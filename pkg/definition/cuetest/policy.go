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

package cuetest

import (
	"context"
	"fmt"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

// Artifact is one of the Application's rendered components, as a policy
// reads it in context.artifacts.
type Artifact struct {
	Workload map[string]any `json:"workload,omitempty"`
	// Traits are each trait's rendered objects, by trait type then output.
	Traits map[string]map[string]map[string]any `json:"traits,omitempty"`
}

// declaresOutput reports whether a template declares a top-level output.
func declaresOutput(template string) bool {
	f, err := parser.ParseFile("template", template)
	if err != nil {
		// Left for the render to report.
		return true
	}
	for _, decl := range f.Decls {
		if field, ok := decl.(*ast.Field); ok {
			if name, _, _ := ast.LabelName(field.Label); name == "output" {
				return true
			}
		}
	}
	return false
}

// RenderPolicy renders a workload-bearing policy definition as the
// controller does before dispatching it: through the Application's appfile,
// against the given rendered components, with its output and outputs
// defaulted and labelled as they are dispatched.
func RenderPolicy(s Subject, in Input) (*Rendered, error) {
	if s.Kind != KindPolicy {
		return nil, fmt.Errorf("%q is a %s definition, not a workload-bearing policy", s.Name, s.Kind)
	}
	restoreCompiler, calls, err := useMocks(in.Mocks)
	if err != nil {
		return nil, err
	}
	defer restoreCompiler()

	c := in.Context
	app := application(c)
	name := or(c.Name, "test-policy")
	props, err := rawJSON(in.Parameter)
	if err != nil {
		return nil, err
	}
	app.Spec.Policies = []v1beta1.AppPolicy{{Name: name, Type: s.Name, Properties: props}}

	ctx := oamutil.SetNamespaceInCtx(context.Background(), app.Namespace)
	parser := appfile.NewDryRunApplicationParser(fake.NewClientBuilder().Build(), []*unstructured.Unstructured{definitionObject(s)})
	af, err := parser.GenerateAppFileFromApp(ctx, app)
	if err != nil {
		return nil, err
	}
	af.AppRevisionName = or(c.AppRevision, app.Name+"-v1")
	if c.Custom != nil {
		// Where an Application-scoped policy's output.ctx is carried.
		af.Context = context.WithValue(af.Context, oam.PolicyAdditionalContextKey, c.Custom)
	}
	for _, compName := range sortedKeys(in.Artifacts) {
		af.Artifacts = append(af.Artifacts, artifactManifest(compName, in.Artifacts[compName]))
	}
	var policy *appfile.Component
	for _, p := range af.ParsedPolicies {
		if p.Name == name {
			policy = p
		}
	}
	if policy == nil {
		return nil, fmt.Errorf("the application has no policy %q", name)
	}

	restoreVersion := useClusterVersion(c.ClusterVersion)
	defer restoreVersion()
	r, err := af.RenderPolicy(policy)
	if err != nil {
		return nil, err
	}
	out := &Rendered{Output: r.Output.Object, Outputs: map[string]map[string]any{}, Calls: calls.calls}
	for n, o := range r.Outputs {
		out.Outputs[n] = o.Object
	}
	out.Context, out.ContextErr = templateContext(r.Context)
	return out, nil
}

// artifactManifest is an artifact as the appfile carries it: a trait's
// objects are found by the labels a render stamps on them.
func artifactManifest(name string, a Artifact) *types.ComponentManifest {
	cm := &types.ComponentManifest{Name: name}
	if a.Workload != nil {
		cm.ComponentOutput = &unstructured.Unstructured{Object: deepCopy(a.Workload)}
	}
	for _, typ := range sortedKeys(a.Traits) {
		for _, output := range sortedKeys(a.Traits[typ]) {
			obj := &unstructured.Unstructured{Object: deepCopy(a.Traits[typ][output])}
			labels := obj.GetLabels()
			if labels == nil {
				labels = map[string]string{}
			}
			labels[oam.TraitTypeLabel], labels[oam.TraitResource] = typ, output
			obj.SetLabels(labels)
			cm.ComponentOutputsAndTraits = append(cm.ComponentOutputsAndTraits, obj)
		}
	}
	return cm
}

// useClusterVersion sets the control plane's version a render without a
// mutate hook reads, and returns a function putting it back.
func useClusterVersion(v *ClusterVersion) (restore func()) {
	saved := types.ControlPlaneClusterVersion
	types.ControlPlaneClusterVersion = clusterVersion(v)
	return func() { types.ControlPlaneClusterVersion = saved }
}
