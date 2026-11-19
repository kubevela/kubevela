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
	"slices"
	"strconv"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex"
	pkgmulticluster "github.com/kubevela/pkg/multicluster"

	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
	"github.com/oam-dev/kubevela/pkg/sources"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1beta1/sourcedefinition"
)

// SourceInput is what a #SourceExec case resolves a source with.
type SourceInput struct {
	// Consumer is the surface reading the source, which decides the context
	// fields it can read.
	Consumer string
	// Context is the case's context, as the source reads it: its fields are
	// those a source reads rather than a component's, so it is kept as given.
	Context map[string]any
}

// testBinding names the binding a case resolves when its context names none.
const testBinding = "test"

// sourceConsumers are the surfaces that resolve a source, with the context
// registry schema giving each field's type.
var sourceConsumers = map[string]propexpr.ContextSchema{
	sources.SurfaceComponent:      propexpr.ComponentContext,
	sources.SurfaceTrait:          propexpr.TraitContext,
	sources.SurfaceWorkflowStep:   propexpr.WorkflowStepContext,
	sources.SurfacePolicyRendered: propexpr.RenderedPolicyContext,
}

// sourceProviders are a source's when it is resolved. Reads of the cluster
// run for real, against the test cluster; what reaches outside it must be
// mocked.
var sourceProviders = &providerSet{
	packages: velacuex.SourcePackages,
	unmockable: map[string]string{
		"vela/base64": "has no side effects",
		"vela/cue":    "has no side effects",
	},
	unmatched: func(path, def, _ string, params []byte) error {
		if path == "vela/http" || path == "vela/registry" {
			return fmt.Errorf("unmocked call %s.%s with $params %s: it reaches outside the cluster, so a source test must mock it", path, def, params)
		}
		return nil
	},
}

// checkSourceContext checks a source case's context and consumer: the
// consumer must be one that resolves sources and that the definition allows,
// and each context field one a source it consumes can read, of the type the
// consumer's surface gives it.
func checkSourceContext(v cue.Value, consumer string, s Subject) error {
	surface, ok := sourceConsumers[consumer]
	if !ok {
		return fmt.Errorf("consumer %q does not resolve sources; one of %v", consumer, sortedKeys(sourceConsumers))
	}
	allowed, err := sourcedefinition.ParseConsumableFrom(s.Template)
	if err != nil {
		return fmt.Errorf("consumableFrom: %w", err)
	}
	if allowed != nil && !slices.Contains(allowed, consumer) {
		return fmt.Errorf("%s is consumable from %s only, not %s", s.Name, strings.Join(allowed, ", "), consumer)
	}
	if !v.Exists() {
		return nil
	}
	fields, err := sources.ContextFields(consumer)
	if err != nil {
		return err
	}
	it, err := v.Fields()
	if err != nil {
		return fmt.Errorf("context: %w", err)
	}
	for it.Next() {
		field := it.Selector().Unquoted()
		if !slices.Contains(fields, field) {
			return fmt.Errorf("context.%s is not readable by a source consumed from %s", field, consumer)
		}
		want, _ := surface.FieldValue(field)
		var got any
		if err := it.Value().Decode(&got); err != nil {
			return fmt.Errorf("context.%s: %w", field, err)
		}
		if err := want.Unify(want.Context().Encode(got)).Validate(cue.Concrete(true)); err != nil {
			return fmt.Errorf("context.%s: %w", field, err)
		}
	}
	return nil
}

// ResolvedSource is what resolving a source produced.
type ResolvedSource struct {
	Output map[string]any
	// Calls are the provider calls made, real or mocked.
	Calls []Call
	// Storage is how the value would be cached; nil when the policy itself
	// could not be computed, which the resolve reports.
	Storage *sources.CachePolicy
}

// ResolveSource resolves a source definition through the controller's own
// source engine, as consumer would, against the shared test cluster in ns.
// The engine runs with no cache store, so every resolve fetches; the cache
// policy is computed alongside, as the resolver computes it.
func ResolveSource(s Subject, in SourceInput, parameter map[string]any, mocks Mocks, cl *TestCluster, ns string) (*ResolvedSource, error) {
	goCtx := cl.runtimeContext(context.Background())
	rec := &recorder{}
	pkgs, err := sourceProviders.wrap(mocks, rec)
	if err != nil {
		return nil, err
	}
	values := map[string]any{
		"namespace": ns,
		"appName":   "test-app",
		"cluster":   pkgmulticluster.Local,
	}
	for k, v := range in.Context {
		values[k] = v
	}
	binding := testBinding
	if name, ok := values["name"].(string); ok {
		binding = name
	}
	delete(values, "name")
	engine, err := sources.NewSourceEngine(sources.SourceEngineOptions{
		Surface:   or(in.Consumer, sources.SurfaceComponent),
		Context:   values,
		Bindings:  map[string]map[string]any{binding: parameter},
		Types:     map[string]string{binding: s.Name},
		Templates: map[string]string{s.Name: s.Template},
		Compiler:  cuex.NewCompilerWithInternalPackages(pkgs...),
	})
	if err != nil {
		return nil, err
	}
	r := &ResolvedSource{}
	if policy, err := engine.CachePolicy(goCtx, binding); err == nil {
		r.Storage = &policy
	}
	res, err := engine.Resolve(goCtx, map[string]any{"output": "$(source[" + strconv.Quote(binding) + "])"})
	r.Calls = rec.calls
	if err != nil {
		return r, err
	}
	props, _ := res.Properties.(map[string]any)
	r.Output, _ = props["output"].(map[string]any)
	return r, nil
}

func (c *Case) sourceResult(cl *TestCluster, ns string) (map[string]any, error) {
	r, err := ResolveSource(c.Subject, *c.Source, c.Input.Parameter, c.Input.Mocks, cl, ns)
	if r == nil {
		return map[string]any{}, err
	}
	result := map[string]any{"calls": callsResult(r.Calls)}
	if r.Output != nil {
		result["output"] = r.Output
	}
	if p := r.Storage; p != nil {
		result["storage"] = map[string]any{
			"ttl":            c.expectedDuration("storage.ttl", p.TTL),
			"onStaleFailure": p.OnStaleFailure,
			"keyInputs":      append([]string{}, p.KeyInputs...),
			"key":            p.Key,
		}
	}
	return result, err
}

// expectedDuration is d as the case writes it at path, when the case expects
// the same duration however spelled, so "90s" matches 1m30s; otherwise d.
func (c *Case) expectedDuration(path string, d time.Duration) string {
	want, err := c.Expect.LookupPath(cue.ParsePath(path)).String()
	if err != nil {
		return d.String()
	}
	if parsed, err := time.ParseDuration(want); err == nil && parsed == d {
		return want
	}
	return d.String()
}
