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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
	// Derived is set when the status was computed from the object's pods.
	Derived bool `json:"derived,omitempty" jsonschema:"True when this status was computed from the object's pods, because its own health check reported Healthy while some of its pods were not."`
}

// deriveFromPods corrects a managed object whose own health check reports
// Healthy while pods under it are failing. KubeVela's resource tree has no
// rule for most workload kinds and defaults them to Healthy, so without
// this a crash-looping Deployment reads as healthy at the top line. All
// pods failing is UnHealthy; some failing is Progressing. The original
// verdict is kept in the message.
func deriveFromPods(nodes []ResourceNode) []ResourceNode {
	if len(nodes) == 0 || nodes[0].Health.Status != string(querytypes.HealthStatusHealthy) {
		return nodes
	}
	pods, failing := 0, 0
	for _, n := range nodes[1:] {
		if n.Kind != podKind {
			continue
		}
		pods++
		if n.Health.Status != string(querytypes.HealthStatusHealthy) {
			failing++
		}
	}
	if failing == 0 {
		return nodes
	}
	status := string(querytypes.HealthStatusProgressing)
	if failing == pods {
		status = string(querytypes.HealthStatusUnHealthy)
	}
	nodes[0].Health = ResourceHealth{
		Status:  status,
		Reason:  fmt.Sprintf("%d of %d pods are not healthy", failing, pods),
		Message: "the object's own health check reported Healthy",
		Derived: true,
	}
	return nodes
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
	Resources   []ManagedResource `json:"resources" jsonschema:"Managed objects with cluster, health and the live resource tree. A single match also carries the live object."`
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

	applied, info, err := d.listAppliedResources(ctx, app, in.Component, in.Cluster)
	if err != nil {
		return nil, out, err
	}
	clusters := map[string]bool{}
	for i := range applied {
		clusters[applied[i].Cluster] = true
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
	out.Meta = d.meta(ctx)
	out.Meta.Warnings = append(out.Meta.Warnings, withheldWarning(info.withheld)...)
	out.Meta.Warnings = append(out.Meta.Warnings, info.notes...)
	if len(info.untraced) > 0 {
		out.Meta.Truncated = true
	}
	if w := d.remoteReadWarning(ctx, clusters); w != "" {
		out.Meta.Warnings = append(out.Meta.Warnings, w)
	}
	for i := range out.Resources {
		if len(out.Resources[i].Tree) > maxTreeNodes {
			out.Resources[i].Tree = out.Resources[i].Tree[:maxTreeNodes]
			out.Meta.Truncated = true
			out.Meta.Warnings = append(out.Meta.Warnings, fmt.Sprintf("%s %s has more than %d descendants; only the first %d are listed", out.Resources[i].Kind, out.Resources[i].Name, maxTreeNodes, maxTreeNodes))
		}
	}
	if len(out.Resources) == 1 && (in.ResourceName != "" || in.Kind != "") {
		d.attachLiveObject(ctx, &out.Resources[0], &out)
	}
	return nil, out, nil
}

// withheldWarning explains resources dropped by the namespace allowlist.
func withheldWarning(withheld int) []string {
	if withheld == 0 {
		return nil
	}
	return []string{fmt.Sprintf("%d managed resources are outside this server's allowed namespaces and were withheld", withheld)}
}

// listAppliedResources runs the same collector VelaQL's resource views use,
// so the tree and health here match vela status --tree. When the server
// has a namespace allowlist, resources recorded in other namespaces are
// dropped before anything is read from them and the count is returned:
// an Application can record objects anywhere a topology or read-only
// policy points, and the allowlist must hold for those too. Cluster-scoped
// objects have no namespace and are withheld under an allowlist.
// appliedInfo is what listAppliedResources knows beyond the resources
// themselves: how many it withheld, notes for the caller to surface, and
// which resources it listed without tracing their tree.
type appliedInfo struct {
	withheld int
	notes    []string
	untraced map[string]bool
}

// appliedKey identifies a managed resource for appliedInfo.untraced.
func appliedKey(r querytypes.AppliedResource) string {
	return clusterName(r.Cluster) + "/" + r.Namespace + "/" + r.Kind + "/" + r.Name
}

func (d *deps) listAppliedResources(ctx context.Context, app *v1beta1.Application, component, cluster string) ([]querytypes.AppliedResource, appliedInfo, error) {
	cli := d.f.Client()
	if cli == nil {
		return nil, appliedInfo{}, errNoConfig
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
	if len(d.opts.Namespaces) > 0 {
		// Without a tree the collector only reads ResourceTrackers, so the
		// allowlist can be applied before any object in a foreign namespace
		// is touched; the tree is then built for the survivors only.
		opt.WithTree = false
		recorded, err := query.NewAppCollector(cli, opt).ListApplicationResources(ctx, app)
		if err != nil {
			return nil, appliedInfo{}, fmt.Errorf("list managed resources of %s/%s: %w", app.Namespace, app.Name, err)
		}
		withheld := 0
		allowed := []querytypes.AppliedResource{}
		for i := range recorded {
			if recorded[i].Namespace == "" || d.namespaceAllowed(recorded[i].Namespace) != nil {
				withheld++
				continue
			}
			allowed = append(allowed, recorded[i])
		}
		if len(allowed) == 0 {
			return allowed, appliedInfo{withheld: withheld}, nil
		}
		// One tree pass per allowed cluster and namespace: the collector's
		// filter then skips every other recorded object before reading it.
		type scope struct{ cluster, namespace string }
		var scopes []scope
		seen := map[scope]bool{}
		for _, r := range allowed {
			sc := scope{r.Cluster, r.Namespace}
			if sc.cluster == "" {
				sc.cluster = localCluster
			}
			if !seen[sc] {
				seen[sc] = true
				scopes = append(scopes, sc)
			}
		}
		// Each scope is a collector pass, so they are bounded. Resources in
		// scopes past the bound are listed without a tree and reported.
		var kept []querytypes.AppliedResource
		info := appliedInfo{withheld: withheld, untraced: map[string]bool{}}
		if len(scopes) > maxTreeScopes {
			skipped := map[scope]bool{}
			for _, sc := range scopes[maxTreeScopes:] {
				skipped[sc] = true
			}
			for _, r := range allowed {
				sc := scope{r.Cluster, r.Namespace}
				if sc.cluster == "" {
					sc.cluster = localCluster
				}
				if skipped[sc] {
					kept = append(kept, r)
					info.untraced[appliedKey(r)] = true
				}
			}
			info.notes = append(info.notes, fmt.Sprintf("the Application spans %d cluster and namespace pairs; this server traces resource trees in the first %d, so %d resources in further pairs are listed but not traced. Narrow by cluster or component to trace them", len(scopes), maxTreeScopes, len(kept)))
			scopes = scopes[:maxTreeScopes]
		}
		for _, sc := range scopes {
			o := opt
			o.WithTree = true
			o.Filter.Cluster, o.Filter.ClusterNamespace = sc.cluster, sc.namespace
			applied, err := query.NewAppCollector(cli, o).ListApplicationResources(ctx, app)
			if err != nil {
				return nil, info, fmt.Errorf("list managed resources of %s/%s: %w", app.Namespace, app.Name, err)
			}
			kept = append(kept, applied...)
		}
		return kept, info, nil
	}
	applied, err := query.NewAppCollector(cli, opt).ListApplicationResources(ctx, app)
	if err != nil {
		return nil, appliedInfo{}, fmt.Errorf("list managed resources of %s/%s: %w", app.Namespace, app.Name, err)
	}
	return applied, appliedInfo{}, nil
}

// attachLiveObject reads the selected object from its cluster. A Secret is
// read as metadata only, so its data never reaches this process.
func (d *deps) attachLiveObject(ctx context.Context, res *ManagedResource, out *GetManagedResourceOutput) {
	key := client.ObjectKey{Namespace: res.Namespace, Name: res.Name}
	cctx := ctx
	if res.Cluster != "" && res.Cluster != localCluster {
		cctx = multicluster.ContextWithClusterName(ctx, res.Cluster)
	}
	if res.Kind == secretKind {
		d.attachSecretMetadata(cctx, key, res, out)
		return
	}
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(res.APIVersion)
	obj.SetKind(res.Kind)
	if err := d.f.Client().Get(cctx, key, obj); err != nil {
		res.ObjectNote = "live read failed: " + redactLine(err.Error())
		return
	}
	out.Evidence = append(out.Evidence, fmt.Sprintf("%s %s/%s in cluster %s resourceVersion %s", res.Kind, res.Namespace, res.Name, res.Cluster, obj.GetResourceVersion()))
	content := redactObject(obj.DeepCopy().Object)
	if res.Kind == "ConfigMap" {
		res.ObjectNote = "ConfigMap values are not returned, only their keys; read values through the Kubernetes MCP under its own policy"
	}
	if size := jsonSize(content); size > maxObjectBytes {
		res.ObjectNote = fmt.Sprintf("live object is %d bytes, over the %d byte limit; read it with the Kubernetes MCP or kubectl", size, maxObjectBytes)
		out.Meta.Truncated = true
		return
	}
	res.Object = content
}

// jsonSize is the encoded size of v, or 0 when it cannot be encoded.
func jsonSize(v any) int {
	raw, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return len(raw)
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
		m.Tree = deriveFromPods(flattenTree(res.ResourceTree, 0, "", nil))
	}
	return m
}

// attachSecretMetadata reads a Secret through the metadata-only API, so
// the request itself never returns data or type. The Factory's client is an
// uncached controller-runtime client, which routes PartialObjectMetadata to
// its metadata client; that client asks the API server for
// "as=PartialObjectMetadata", so the response carries no Secret data.
func (d *deps) attachSecretMetadata(ctx context.Context, key client.ObjectKey, res *ManagedResource, out *GetManagedResourceOutput) {
	pm := &metav1.PartialObjectMetadata{}
	pm.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind(secretKind))
	if err := d.f.Client().Get(ctx, key, pm); err != nil {
		res.ObjectNote = "live read failed: " + redactLine(err.Error())
		return
	}
	out.Evidence = append(out.Evidence, fmt.Sprintf("%s %s/%s in cluster %s resourceVersion %s, metadata only", res.Kind, res.Namespace, res.Name, res.Cluster, pm.GetResourceVersion()))
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pm)
	if err != nil {
		res.ObjectNote = "Secret metadata could not be read: " + redactLine(err.Error())
		return
	}
	meta := redactObject(map[string]any{"apiVersion": "v1", "kind": secretKind, "metadata": raw["metadata"]})
	if size := jsonSize(meta); size > maxObjectBytes {
		res.ObjectNote = fmt.Sprintf("Secret metadata is %d bytes, over the %d byte limit; content is never returned", size, maxObjectBytes)
		out.Meta.Truncated = true
		return
	}
	res.Object = meta
	res.ObjectNote = "Secret content is never read or returned; metadata only"
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
			Reason:  text(n.HealthStatus.Reason),
			Message: text(n.HealthStatus.Message),
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
