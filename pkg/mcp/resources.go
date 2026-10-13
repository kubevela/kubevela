/*
Copyright 2026 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	querytypes "github.com/oam-dev/kubevela/pkg/utils/types"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/query"
)

const secretKind = "Secret"

// GetManagedResourceInput selects resources an Application manages. With
// only the application set it returns the whole tree; kind and name narrow
// it to one object and include its live content.
type GetManagedResourceInput struct {
	ApplicationRef
	Component string `json:"component,omitempty" jsonschema:"Only resources created by this component."`
	Cluster   string `json:"cluster,omitempty" jsonschema:"Only resources in this cluster. Use local for the control plane."`
	Kind      string `json:"kind,omitempty" jsonschema:"Only resources of this kind, for example Deployment."`
	// ResourceName is distinct from Name, which the embedded ApplicationRef
	// owns; a same-named field would shadow it and drop it from the schema.
	ResourceName string `json:"resourceName,omitempty" jsonschema:"Only the resource with this name. With kind this selects one object and returns its live content."`
}

// ResourceHealth is the health the resource tree computed for one object.
type ResourceHealth struct {
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// ResourceNode is one object in a managed resource's tree. The tree is
// returned flat in depth-first order: Depth is 0 for the managed object
// and Parent names the node above, so the shape survives a schema that
// cannot express recursion.
type ResourceNode struct {
	Depth      int            `json:"depth"`
	Parent     string         `json:"parent,omitempty"`
	Cluster    string         `json:"cluster"`
	APIVersion string         `json:"apiVersion,omitempty"`
	Kind       string         `json:"kind"`
	Namespace  string         `json:"namespace,omitempty"`
	Name       string         `json:"name"`
	Health     ResourceHealth `json:"health"`
	Info       map[string]any `json:"info,omitempty"`
	CreatedAt  *time.Time     `json:"createdAt,omitempty"`
	Deleting   bool           `json:"deleting,omitempty"`
}

// ManagedResource is one object a ResourceTracker records, with its live
// tree and, when it was selected alone, its live content.
type ManagedResource struct {
	Cluster    string `json:"cluster"`
	Component  string `json:"component"`
	Trait      string `json:"trait,omitempty"`
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
	// Latest is false for resources only an outdated revision still holds.
	Latest   bool   `json:"latest"`
	Revision string `json:"revision,omitempty"`
	// Tree is the managed object and everything it owns, depth-first.
	Tree []ResourceNode `json:"tree,omitempty"`
	// Object is the live object without managedFields. It is present only
	// when the input selected exactly one resource. Secrets return metadata
	// only.
	Object map[string]any `json:"object,omitempty"`
	// ObjectNote says why Object is absent or reduced.
	ObjectNote string `json:"objectNote,omitempty"`
}

// GetManagedResourceOutput is the structured result of get_managed_resource.
type GetManagedResourceOutput struct {
	Application string            `json:"application"`
	Namespace   string            `json:"namespace"`
	Resources   []ManagedResource `json:"resources"`
	Evidence    []string          `json:"evidence"`
	Meta        Meta              `json:"meta"`
}

func (d *deps) getManagedResource(ctx context.Context, _ *sdk.CallToolRequest, in GetManagedResourceInput) (*sdk.CallToolResult, GetManagedResourceOutput, error) {
	out := GetManagedResourceOutput{Resources: []ManagedResource{}}
	app, err := d.loadApplication(ctx, in.ApplicationRef)
	if err != nil {
		return nil, out, err
	}
	out.Application, out.Namespace = app.Name, app.Namespace
	out.Evidence = []string{applicationEvidence(app)}

	applied, err := d.listAppliedResources(ctx, app, in.Component, in.Cluster)
	if err != nil {
		return nil, out, err
	}
	for i := range applied {
		res := applied[i]
		if in.Kind != "" && !strings.EqualFold(res.Kind, in.Kind) {
			continue
		}
		if in.ResourceName != "" && res.Name != in.ResourceName {
			continue
		}
		out.Resources = append(out.Resources, viewManagedResource(res))
	}
	if in.ResourceName != "" && len(out.Resources) == 0 {
		return nil, out, fmt.Errorf("%s %q is not managed by application %s/%s; only managed objects are read", kindLabel(in.Kind), in.ResourceName, app.Namespace, app.Name)
	}
	if len(out.Resources) == 1 && (in.ResourceName != "" || in.Kind != "") {
		d.attachLiveObject(ctx, &out.Resources[0], &out)
	}
	out.Meta = d.meta(ctx)
	return nil, out, nil
}

// listAppliedResources runs the same collector VelaQL's resource views use,
// so the tree and health here match vela status --tree.
func (d *deps) listAppliedResources(ctx context.Context, app *v1beta1.Application, component, cluster string) ([]querytypes.AppliedResource, error) {
	cli := d.f.Client()
	if cli == nil {
		return nil, errNoConfig
	}
	opt := query.Option{
		Name:      app.Name,
		Namespace: app.Namespace,
		WithTree:  true,
		Filter:    query.FilterOption{Cluster: cluster},
	}
	if component != "" {
		opt.Filter.Components = []string{component}
	}
	applied, err := query.NewAppCollector(cli, opt).ListApplicationResources(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("list managed resources of %s/%s: %w", app.Namespace, app.Name, err)
	}
	return applied, nil
}

// attachLiveObject reads the selected object from its cluster. Secrets are
// reduced to metadata before anything leaves the handler; the GET itself
// still happens so a missing Secret is reported as missing.
func (d *deps) attachLiveObject(ctx context.Context, res *ManagedResource, out *GetManagedResourceOutput) {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(res.APIVersion)
	obj.SetKind(res.Kind)
	key := client.ObjectKey{Namespace: res.Namespace, Name: res.Name}
	cctx := ctx
	if res.Cluster != "" && res.Cluster != localCluster {
		cctx = multicluster.ContextWithClusterName(ctx, res.Cluster)
	}
	if err := d.f.Client().Get(cctx, key, obj); err != nil {
		res.ObjectNote = "live read failed: " + err.Error()
		return
	}
	out.Evidence = append(out.Evidence, fmt.Sprintf("%s %s/%s in cluster %s resourceVersion %s", res.Kind, res.Namespace, res.Name, res.Cluster, obj.GetResourceVersion()))
	content := obj.DeepCopy().Object
	unstructured.RemoveNestedField(content, "metadata", "managedFields")
	if res.Kind == secretKind {
		res.Object = map[string]any{
			"apiVersion": content["apiVersion"],
			"kind":       content["kind"],
			"metadata":   content["metadata"],
			"type":       content["type"],
		}
		res.ObjectNote = "Secret content is never returned; metadata only"
		return
	}
	res.Object, _ = redactMap(content).(map[string]any)
}

func viewManagedResource(res querytypes.AppliedResource) ManagedResource {
	m := ManagedResource{
		Cluster:    clusterName(res.Cluster),
		Component:  res.Component,
		Trait:      res.Trait,
		APIVersion: res.APIVersion,
		Kind:       res.Kind,
		Namespace:  res.Namespace,
		Name:       res.Name,
		Latest:     res.Latest,
		Revision:   res.Revision,
	}
	if res.ResourceTree != nil {
		m.Tree = flattenTree(res.ResourceTree, 0, "", nil)
	}
	return m
}

// flattenTree appends n and its descendants depth-first.
func flattenTree(n *querytypes.ResourceTreeNode, depth int, parent string, out []ResourceNode) []ResourceNode {
	if n == nil {
		return out
	}
	node := viewResourceNode(n, depth, parent)
	out = append(out, node)
	self := node.Kind + "/" + node.Name
	for _, leaf := range n.LeafNodes {
		out = flattenTree(leaf, depth+1, self, out)
	}
	return out
}

func viewResourceNode(n *querytypes.ResourceTreeNode, depth int, parent string) ResourceNode {
	node := ResourceNode{
		Depth:      depth,
		Parent:     parent,
		Cluster:    clusterName(n.Cluster),
		APIVersion: n.APIVersion,
		Kind:       n.Kind,
		Namespace:  n.Namespace,
		Name:       n.Name,
		Health: ResourceHealth{
			Status:  string(n.HealthStatus.Status),
			Reason:  n.HealthStatus.Reason,
			Message: n.HealthStatus.Message,
		},
		CreatedAt: timePtr(n.CreationTimestamp),
		Deleting:  !n.DeletionTimestamp.IsZero(),
	}
	if len(n.AdditionalInfo) > 0 {
		node.Info, _ = redactMap(n.AdditionalInfo).(map[string]any)
	}
	return node
}

func kindLabel(kind string) string {
	if kind == "" {
		return "resource"
	}
	return kind
}
