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

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile/dryrun"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
	// maxApplicationBytes bounds the Application document a plan tool accepts.
	maxApplicationBytes = 1 << 20
	// maxLocalDefinitions bounds the definitions a caller may supply inline.
	maxLocalDefinitions = 20
	// maxRenderedBytes bounds the rendered objects one plan result carries;
	// beyond it objects are listed without content.
	maxRenderedBytes = 1 << 20
	// mcpAnnotationPrefix marks annotations the server writes itself. They
	// are stripped from any input so a record cannot be forged.
	mcpAnnotationPrefix = "mcp.oam.dev/"
)

// PlanInput is the Application a plan tool works on. It is the document a
// user would hand to vela up, as YAML or JSON, so a draft can be checked
// without being saved anywhere.
type PlanInput struct {
	Application string `json:"application" jsonschema:"The Application as YAML or JSON, exactly as it would be applied."`
	Namespace   string `json:"namespace,omitempty" jsonschema:"Namespace to render in when the document has none. The document's own namespace wins."`
	// Definitions lets an author render against definitions that are not
	// on the cluster yet. They take priority over cluster definitions of
	// the same name, as vela dry-run -d does.
	Definitions []string `json:"definitions,omitempty" jsonschema:"Optional local definitions as YAML, each one document, used instead of the cluster's copy of the same name."`
}

// RenderedResource is one object the Application would create.
type RenderedResource struct {
	Component  string `json:"component"`
	Source     string `json:"source"`
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
	// Object is the rendered manifest after redaction, absent when the
	// result exceeded its byte budget.
	Object map[string]any `json:"object,omitempty"`
}

// RenderedPolicy is one object a policy rendered.
type RenderedPolicy struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Name       string         `json:"name"`
	Object     map[string]any `json:"object,omitempty"`
}

// DryRunOutput is the structured result of dryrun_application.
type DryRunOutput struct {
	Application string             `json:"application"`
	Namespace   string             `json:"namespace"`
	Components  []ComponentRef     `json:"components"`
	Resources   []RenderedResource `json:"resources"`
	Policies    []RenderedPolicy   `json:"policies"`
	Evidence    []string           `json:"evidence"`
	Meta        Meta               `json:"meta"`
}

// rendered is a dry-run result keyed for comparison.
type rendered struct {
	app       *v1beta1.Application
	resources []RenderedResource
	objects   map[string]*unstructured.Unstructured // key: kind/namespace/name
	policies  []RenderedPolicy
}

func (d *deps) dryRunApplication(ctx context.Context, _ *sdk.CallToolRequest, in PlanInput) (*sdk.CallToolResult, DryRunOutput, error) {
	out := DryRunOutput{Components: []ComponentRef{}, Resources: []RenderedResource{}, Policies: []RenderedPolicy{}}
	app, defs, err := d.parsePlanInput(in)
	if err != nil {
		return nil, out, err
	}
	out.Application, out.Namespace = app.Name, app.Namespace
	for _, c := range app.Spec.Components {
		out.Components = append(out.Components, ComponentRef{Name: c.Name, Type: c.Type})
	}
	r, err := d.render(ctx, app, defs)
	if err != nil {
		return nil, out, err
	}
	out.Resources = r.resources
	out.Policies = r.policies
	out.Meta = d.meta(ctx)
	out.Meta.Source = SourceRendered
	out.Evidence = []string{
		fmt.Sprintf("rendered Application %s/%s from the submitted document with the cluster's definitions", app.Namespace, app.Name),
	}
	if len(defs) > 0 {
		out.Evidence = append(out.Evidence, fmt.Sprintf("%d local definitions took priority over the cluster's copies", len(defs)))
	}
	if trimRendered(out.Resources, out.Policies) {
		out.Meta.Truncated = true
		out.Meta.Warnings = append(out.Meta.Warnings, fmt.Sprintf("rendered objects exceed %d bytes; later objects are listed without content", maxRenderedBytes))
	}
	return nil, out, nil
}

// parsePlanInput decodes the Application and local definitions, applies the
// namespace rule, checks the allowlist and strips server-owned annotations.
func (d *deps) parsePlanInput(in PlanInput) (*v1beta1.Application, []*unstructured.Unstructured, error) {
	if strings.TrimSpace(in.Application) == "" {
		return nil, nil, fmt.Errorf("application is required")
	}
	if len(in.Application) > maxApplicationBytes {
		return nil, nil, fmt.Errorf("application document is %d bytes, over the %d byte limit", len(in.Application), maxApplicationBytes)
	}
	app := &v1beta1.Application{}
	if err := yaml.UnmarshalStrict([]byte(in.Application), app); err != nil {
		return nil, nil, fmt.Errorf("application does not parse as a KubeVela Application: %w", err)
	}
	if app.Kind != "" && app.Kind != v1beta1.ApplicationKind {
		return nil, nil, fmt.Errorf("document kind is %q, expected Application", app.Kind)
	}
	if app.Name == "" {
		return nil, nil, fmt.Errorf("application has no metadata.name")
	}
	if app.Namespace == "" {
		app.Namespace = in.Namespace
	}
	if app.Namespace == "" {
		return nil, nil, fmt.Errorf("namespace is required: set metadata.namespace or the namespace argument")
	}
	if err := d.namespaceAllowed(app.Namespace); err != nil {
		return nil, nil, err
	}
	for k := range app.Annotations {
		if strings.HasPrefix(k, mcpAnnotationPrefix) {
			delete(app.Annotations, k)
		}
	}
	if len(in.Definitions) > maxLocalDefinitions {
		return nil, nil, fmt.Errorf("%d local definitions supplied, at most %d are accepted", len(in.Definitions), maxLocalDefinitions)
	}
	defs := make([]*unstructured.Unstructured, 0, len(in.Definitions))
	for i, doc := range in.Definitions {
		obj := &unstructured.Unstructured{}
		if err := yaml.Unmarshal([]byte(doc), &obj.Object); err != nil {
			return nil, nil, fmt.Errorf("definitions[%d] does not parse: %w", i, err)
		}
		if obj.GetKind() == "" || obj.GetName() == "" {
			return nil, nil, fmt.Errorf("definitions[%d] needs kind and metadata.name", i)
		}
		if !strings.HasSuffix(obj.GetKind(), "Definition") {
			return nil, nil, fmt.Errorf("definitions[%d] is a %s, only definition kinds are accepted", i, obj.GetKind())
		}
		defs = append(defs, obj)
	}
	return app, defs, nil
}

// render runs the same dry-run vela dry-run uses and returns the objects
// keyed for comparison. Nothing is written: ExecuteDryRun reads definitions
// and policies only.
func (d *deps) render(ctx context.Context, app *v1beta1.Application, defs []*unstructured.Unstructured) (*rendered, error) {
	cli := d.f.Client()
	if cli == nil {
		return nil, errNoConfig
	}
	opt := dryrun.NewDryRunOption(cli, d.f.Config(), defs, false)
	rctx := oamutil.SetNamespaceInCtx(ctx, app.Namespace)
	comps, policies, err := opt.ExecuteDryRun(rctx, app.DeepCopy())
	if err != nil {
		return nil, fmt.Errorf("render %s/%s: %w", app.Namespace, app.Name, err)
	}
	r := &rendered{app: app, objects: map[string]*unstructured.Unstructured{}, resources: []RenderedResource{}, policies: []RenderedPolicy{}}
	for _, comp := range comps {
		if comp.ComponentOutput != nil {
			r.add(comp.Name, "output", app.Namespace, comp.ComponentOutput)
		}
		for _, obj := range comp.ComponentOutputsAndTraits {
			r.add(comp.Name, "outputs or trait", app.Namespace, obj)
		}
	}
	for _, p := range policies {
		if p == nil {
			continue
		}
		r.policies = append(r.policies, RenderedPolicy{APIVersion: p.GetAPIVersion(), Kind: p.GetKind(), Name: p.GetName(), Object: redactObject(p.DeepCopy().Object)})
	}
	return r, nil
}

func (r *rendered) add(component, source, defaultNS string, obj *unstructured.Unstructured) {
	if obj == nil {
		return
	}
	ns := obj.GetNamespace()
	if ns == "" && !clusterScopedKind(obj.GetKind()) {
		ns = defaultNS
	}
	key := resourceKey(obj.GetKind(), ns, obj.GetName())
	r.objects[key] = obj
	r.resources = append(r.resources, RenderedResource{
		Component:  component,
		Source:     source,
		APIVersion: obj.GetAPIVersion(),
		Kind:       obj.GetKind(),
		Namespace:  ns,
		Name:       obj.GetName(),
		Object:     redactObject(obj.DeepCopy().Object),
	})
}

func resourceKey(kind, ns, name string) string {
	return kind + "/" + ns + "/" + name
}

// clusterScopedKind covers the kinds definitions commonly emit without a
// namespace. Anything else without one is assumed namespaced.
func clusterScopedKind(kind string) bool {
	switch kind {
	case "Namespace", "ClusterRole", "ClusterRoleBinding", "PersistentVolume", "StorageClass", "CustomResourceDefinition", "PriorityClass", "ClusterIssuer":
		return true
	}
	return false
}

// trimRendered drops object content past the byte budget, keeping the
// identities so the list stays complete. It reports whether it trimmed.
func trimRendered(resources []RenderedResource, policies []RenderedPolicy) bool {
	used, trimmed := 0, false
	for i := range resources {
		size := jsonSize(resources[i].Object)
		if used+size > maxRenderedBytes {
			resources[i].Object = nil
			trimmed = true
			continue
		}
		used += size
	}
	for i := range policies {
		size := jsonSize(policies[i].Object)
		if used+size > maxRenderedBytes {
			policies[i].Object = nil
			trimmed = true
			continue
		}
		used += size
	}
	return trimmed
}
