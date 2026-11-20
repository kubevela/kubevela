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
	"encoding/json"
	"fmt"

	upstreamcuex "github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	appctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/application"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// RenderedApplicationPolicy is what an Application-scoped policy did.
type RenderedApplicationPolicy struct {
	Enabled bool
	// Output is what the template emitted.
	Output map[string]any
	// Application is the Application with the output applied: its metadata
	// and spec.
	Application map[string]any
	Calls       []Call
}

// scopedPolicyProviders are an Application-scoped policy's: the CueX default
// compiler's internal packages, taken from it so the two cannot drift. As in
// a render, every side-effecting function is mocked.
var scopedPolicyProviders = &providerSet{
	packages: func() []cuexruntime.Package {
		return upstreamcuex.NewCompilerWithDefaultInternalPackages().PackageManager.GetPackages()
	},
	unmockable: map[string]string{
		"vela/base64": "has no side effects",
		"vela/cue":    "has no side effects",
		"vela/util":   "has no side effects",
	},
	unmatched: workloadProviders.unmatched,
}

// RenderApplicationPolicy renders an Application-scoped policy definition
// through the controller's own RenderApplicationPolicy: given the
// Application's identity from the context and its spec from in.Spec, with
// the policy's output applied as the next policy would see it.
func RenderApplicationPolicy(s Subject, in Input) (*RenderedApplicationPolicy, error) {
	if s.Kind != KindApplicationPolicy {
		return nil, fmt.Errorf("%q is a %s definition, not an Application-scoped policy", s.Name, s.Kind)
	}
	rec := &recorder{}
	pkgs, err := scopedPolicyProviders.wrap(in.Mocks, rec)
	if err != nil {
		return nil, err
	}
	// The controller compiles with CueX's default compiler.
	if err := requireOffline(); err != nil {
		return nil, err
	}
	real := upstreamcuex.DefaultCompiler.Get()
	upstreamcuex.DefaultCompiler.Set(upstreamcuex.NewCompilerWithInternalPackages(pkgs...))
	defer upstreamcuex.DefaultCompiler.Set(real)

	def := &v1beta1.PolicyDefinition{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(definitionObject(s).Object, def); err != nil {
		return nil, fmt.Errorf("reading the definition: %w", err)
	}
	c := in.Context
	app := application(c)
	if in.Spec != nil {
		raw, err := json.Marshal(in.Spec)
		if err != nil {
			return nil, fmt.Errorf("spec: %w", err)
		}
		if err := json.Unmarshal(raw, &app.Spec); err != nil {
			return nil, fmt.Errorf("spec: %w", err)
		}
	}
	ctx := context.Background()
	if c.Custom != nil {
		ctx = context.WithValue(ctx, oam.PolicyAdditionalContextKey, c.Custom)
	}
	props, err := rawJSON(in.Parameter)
	if err != nil {
		return nil, err
	}
	restoreVersion := useClusterVersion(c.ClusterVersion)
	defer restoreVersion()

	out, result, err := appctrl.RenderApplicationPolicy(ctx, fake.NewClientBuilder().Build(), appctrl.ApplicationPolicyRender{
		App:         app,
		AppRevision: or(c.AppRevision, app.Name+"-v1"),
		Policy:      v1beta1.AppPolicy{Name: or(c.PolicyName, "test-policy"), Type: s.Name, Properties: props},
		Definition:  def,
		Version: &v1beta1.PolicyVersionMetadata{
			DefinitionRevisionName: c.PolicyRevisionName,
			Revision:               c.PolicyRevision,
			RevisionHash:           c.PolicyRevisionHash,
		},
	})
	if err != nil {
		return nil, err
	}
	r := &RenderedApplicationPolicy{Enabled: result.Enabled, Calls: rec.calls}
	if r.Output, err = toMap(result.Transforms); err != nil {
		return nil, fmt.Errorf("reading the output: %w", err)
	}
	if r.Application, err = toMap(map[string]any{
		"metadata": map[string]any{"name": out.Name, "namespace": out.Namespace, "labels": out.Labels, "annotations": out.Annotations},
		"spec":     out.Spec,
	}); err != nil {
		return nil, fmt.Errorf("reading the Application: %w", err)
	}
	return r, nil
}
