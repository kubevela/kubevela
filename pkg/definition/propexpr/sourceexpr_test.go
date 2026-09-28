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

package propexpr

import (
	"context"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// An expression sees what the definition it feeds sees, at the moment that
// definition is rendered. This builds a real component render context and
// requires every field in it to be classified - readable with a type, or
// explicitly not readable with a reason.
//
// The point is drift: a field added to the render context upstream must force a
// decision here rather than silently becoming unavailable, which would be
// indistinguishable from a typo in an author's expression.
func TestContextTypesMatchTheRenderContext(t *testing.T) {
	v := cuecontext.New().CompileString(componentRenderContext(t))
	if v.Err() != nil {
		t.Fatalf("compiling the render context: %v", v.Err())
	}
	iter, err := v.LookupPath(cue.ParsePath("context")).Fields(cue.All())
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	for iter.Next() {
		field := iter.Selector().Unquoted()
		seen[field] = true
		_, readable := ComponentContext.field(field)
		excluded := knownField(field)
		switch {
		case readable && registry.excluded[field] != "":
			t.Errorf("%q is both readable and globally excluded; it must be one or the other", field)
		case !readable && !excluded:
			t.Errorf("the render context carries %q (%s) but the registry has never heard of it - "+
				"add it to a group in context.cue, or to excluded with a reason",
				field, iter.Value().IncompleteKind())
		}
	}

	// A type declared for a field the render context does not carry would type
	// cleanly at admission and fail at render.
	for _, field := range ComponentContext.readable() {
		if !seen[field] {
			t.Errorf("ComponentContext declares %q, which the render context does not carry", field)
		}
	}
}

// The same drift guard as the component context, against the surface that
// actually differs. Its point is that a field a surface never populates must not
// be declared readable: it would type cleanly at admission and hand the author an
// empty value at render.
func TestScopedPolicyContextMatchesTheRender(t *testing.T) {
	v := cuecontext.New().CompileString(scopedPolicyContext(t))
	if v.Err() != nil {
		t.Fatalf("compiling: %v", v.Err())
	}
	iter, err := v.LookupPath(cue.ParsePath("context")).Fields(cue.All())
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	for iter.Next() {
		field := iter.Selector().Unquoted()
		seen[field] = true
		_, readable := ScopedPolicyContext.field(field)
		excluded := knownField(field)
		switch {
		case readable && registry.excluded[field] != "":
			t.Errorf("%q is both readable and globally excluded", field)
		case !readable && !excluded:
			t.Errorf("the scoped policy context carries %q (%s) but the registry has never heard of it - "+
				"add it to a group in context.cue, or to excluded with a reason",
				field, iter.Value().IncompleteKind())
		}

		// A readable field must actually be populated. An always-empty one would
		// type at admission and be useless at render.
		if readable {
			if s, err := iter.Value().String(); err == nil && s == "" {
				t.Errorf("%q is declared readable but is empty in a real scoped policy render", field)
			}
		}
	}

	for _, field := range ScopedPolicyContext.readable() {
		if !seen[field] {
			t.Errorf("ScopedPolicyContext declares %q, which the scoped render does not carry", field)
		}
	}
}

// The subset is the point: fields the component context has, that a scoped
// policy never receives, must be absent there.
//
// Asked of the registry rather than of a type checker, so this holds regardless
// of the expression language - it is a statement about what the render supplies.
func TestScopedPolicyContextIsASubset(t *testing.T) {
	for _, field := range []string{"appName", "namespace", "appLabels", "policyName"} {
		if !ScopedPolicyContext.Offers(field) {
			t.Errorf("a scoped policy should offer context.%s", field)
		}
	}
	for _, tc := range []struct{ field, why string }{
		{"cluster", "that render targets no cluster"},
		{"workflowName", "always empty there"},
		{"publishVersion", "always empty there"},
		{"name", "ambiguous across the two policy paths"},
	} {
		if ScopedPolicyContext.Offers(tc.field) {
			t.Errorf("a scoped policy must not offer context.%s (%s)", tc.field, tc.why)
		}
	}
}

// A surface's or template's declared context must unify with the context it
// really renders against.
//
// This is the whole point of declaring the registry in CUE. The membership tests
// either side of this one check that every field is classified and every declared
// field exists; unification adds the other half - that a field's declared *type*
// is the type the render actually holds. Without it, a field typed string that is
// really an int passes every test, types cleanly at admission, and fails at
// render.
//
// CUE reports the conflict itself, naming the field and both types, so this needs
// no per-kind mapping to keep in step.
func TestSurfaceTypesUnifyWithTheRenderContext(t *testing.T) {
	for _, tc := range []struct {
		surface string
		schema  ContextSchema
		render  string
	}{
		{"component", ComponentContext, componentRenderContext(t)},
		{"application-scoped policy", ScopedPolicyContext, scopedPolicyContext(t)},
		{"workflow step template", WorkflowStepTemplateContext, workflowStepTemplateContext(t)},
	} {
		t.Run(tc.surface, func(t *testing.T) {
			real := registryContext.CompileString(tc.render).LookupPath(cue.ParsePath("context"))
			if real.Err() != nil {
				t.Fatalf("compiling the render context: %v", real.Err())
			}
			// Only the declared fields: the render context carries more, and what
			// it may carry beyond them is the membership tests' business.
			for _, field := range tc.schema.readable() {
				declared, _ := tc.schema.field(field)
				actual := real.LookupPath(cue.MakePath(cue.Str(field)))
				if !actual.Exists() {
					continue // reported by the membership test
				}
				if err := declared.Unify(actual).Validate(); err != nil {
					t.Errorf("context.%s does not unify with the %s render context: %v",
						field, tc.surface, err)
				}
			}
		})
	}
}

// componentRenderContext builds the context a ComponentDefinition renders
// against, matching what TestContextTypesMatchTheRenderContext constructs.
func componentRenderContext(t *testing.T) string {
	t.Helper()
	ctx := velaprocess.NewContext(velaprocess.ContextData{
		AppName: "checkout", CompName: "web", Namespace: "team-a",
		AppRevisionName: "checkout-v3", WorkflowName: "deploy", PublishVersion: "v1",
		Cluster:        "prod",
		AppLabels:      map[string]string{"team": "payments"},
		AppAnnotations: map[string]string{"note": "x"},
		Ctx:            publishedContext(),
	})
	// Mirrors PrepareProcessContext, which pushes the component's identity before
	// its template - and therefore before any source consumed there resolves.
	ctx.PushData(velaprocess.ContextComponentName, "web")
	ctx.PushData(velaprocess.ContextComponentType, "webservice")
	base, err := ctx.BaseContextFile()
	if err != nil {
		t.Fatalf("building the render context: %v", err)
	}
	return base
}

// scopedPolicyContext mirrors renderPolicyCUETemplate's construction, so the
// schema is checked against what that render really carries.
func scopedPolicyContext(t *testing.T) string {
	t.Helper()
	pCtx := velaprocess.NewContext(velaprocess.ContextData{
		Namespace:       "team-a",
		AppName:         "checkout",
		CompName:        "checkout",
		AppRevisionName: "checkout-v3",
		AppLabels:       map[string]string{"team": "payments"},
		AppAnnotations:  map[string]string{"note": "x"},
		// A later scoped policy sees what an earlier one published, because
		// storeAdditionalContextInCtx merges rather than replaces.
		Ctx: publishedContext(),
	})
	pCtx.PushData(velaprocess.ContextAppComponents, []string{})
	pCtx.PushData(velaprocess.ContextAppWorkflow, nil)
	pCtx.PushData(velaprocess.ContextAppPolicies, []string{})
	pCtx.PushData(velaprocess.ContextPolicyName, "my-policy")
	pCtx.PushData(velaprocess.ContextPolicyType, "my-type")
	pCtx.PushData(velaprocess.ContextPolicyRevisionName, "rev-1")
	pCtx.PushData(velaprocess.ContextPolicyRevision, int64(1))
	pCtx.PushData(velaprocess.ContextPolicyRevisionHash, "abc")

	base, err := pCtx.BaseContextFile()
	if err != nil {
		t.Fatalf("building the scoped policy context: %v", err)
	}
	return base
}

// publishedContext carries what an Application-scoped policy emitted via
// output.ctx, which NewContext wraps as context.custom.
//
// The drift tests need it: declaring `custom` readable without a render context
// that produces it would assert availability rather than measure it.
func publishedContext() context.Context {
	return context.WithValue(context.Background(), oam.PolicyAdditionalContextKey,
		map[string]interface{}{"region": "eu-west"})
}
