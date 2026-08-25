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

package sources

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// The point of the engine: a caller that can name its bindings and its surface
// resolves expressions without knowing anything about process.Context, the
// render pipeline, or how caching works.
func demoEngineOptions() SourceEngineOptions {
	return SourceEngineOptions{
		Surface: SurfaceComponent,
		Context: map[string]interface{}{
			velaprocess.ContextNamespace: "team-a",
			velaprocess.ContextAppName:   "checkout",
		},
		Bindings: map[string]map[string]interface{}{"cfg": {}},
		Types:    map[string]string{"cfg": "demo"},
		Templates: map[string]string{"demo": `
schema: {region: string, tier: string}
$internal: {key: "demo", keyInputs: []}
output: {region: "eu-west", tier: "gold"}
`},
	}
}

func TestSourceEngineResolves(t *testing.T) {
	r := require.New(t)
	engine, err := NewSourceEngine(demoEngineOptions())
	r.NoError(err)

	res, err := engine.Resolve(context.Background(), map[string]interface{}{
		"image": "acme/web",
		"where": "$(source.cfg.region)",
		"nested": map[string]interface{}{
			"label": "tier-$(source.cfg.tier)",
		},
		"list": []interface{}{"$(source.cfg.region)"},
	})
	r.NoError(err)

	out := res.Properties.(map[string]interface{})
	r.Equal("acme/web", out["image"], "a property with no expression is untouched")
	r.Equal("eu-west", out["where"])
	// Expressions resolve at any depth, in objects and in list entries alike.
	r.Equal("tier-gold", out["nested"].(map[string]interface{})["label"])
	r.Equal("eu-west", out["list"].([]interface{})[0])

	// The caller gets enough to report status without knowing how resolution works.
	r.Contains(res.Statuses, "cfg")
	r.Equal("Resolved", res.Statuses["cfg"].Phase)
}

// An unknown surface has to be an error. availableFields fails open - an
// unrecognised surface is treated as offering everything - so a typo would
// silently widen what a source may read rather than failing.
func TestSourceEngineRejectsAnUnknownSurface(t *testing.T) {
	r := require.New(t)
	opts := demoEngineOptions()
	opts.Surface = "compnent"
	_, err := NewSourceEngine(opts)
	r.Error(err)
	r.Contains(err.Error(), "unknown surface")
	r.Contains(err.Error(), "compnent")

	opts.Surface = ""
	_, err = NewSourceEngine(opts)
	r.Error(err)
}

// A render populates only the context fields that have values, so requiring the
// caller to supply every field a surface declares would reject the Application's
// own calls.
func TestSourceEngineAcceptsPartialContext(t *testing.T) {
	opts := demoEngineOptions()
	opts.Context = map[string]interface{}{}
	_, err := NewSourceEngine(opts)
	require.NoError(t, err)
}

// A caller that supplies templates and forgets the sensitive paths would get
// silent under-redaction - a credential in plain text where the definition asked
// for none. Deriving them removes the chance.
func TestSourceEngineDerivesSensitivePaths(t *testing.T) {
	r := require.New(t)
	opts := demoEngineOptions()
	opts.Templates = map[string]string{"demo": `
schema: {
  region: string
  // +sensitive
  token: string
}
$internal: {key: "demo", keyInputs: []}
output: {region: "eu-west", token: "s3cret"}
`}
	// Deliberately not supplied.
	opts.Sensitive = nil

	engine, err := NewSourceEngine(opts)
	r.NoError(err)

	res, err := engine.Resolve(context.Background(), map[string]interface{}{
		"where": "$(source.cfg.region)",
		"creds": "$(source.cfg.token)",
	})
	r.NoError(err)
	r.Equal("s3cret", res.Properties.(map[string]interface{})["creds"],
		"the rendered resource still receives the real value")

	// The paths were derived from the template, not supplied.
	paths := res.Statuses["cfg"].SensitivePaths
	r.Contains(paths, "token")
	r.NotContains(paths, "region")

	// ...and a reporter redacts against them. ConsumedFields deliberately holds
	// real values, because the render needs them.
	masks := map[string]struct{}{}
	for _, p := range paths {
		masks[p] = struct{}{}
	}
	consumed := res.Statuses["cfg"].ConsumedFields
	r.Equal("eu-west", RedactValue("region", consumed["region"], masks))
	r.Equal("***", RedactValue("token", consumed["token"], masks))
	r.Equal("s3cret", consumed["token"], "the status still holds the real value")
}

// A caller may add a path the template does not declare, without restating the
// ones it does.
func TestSourceEngineSensitivePathsAreAdditive(t *testing.T) {
	r := require.New(t)
	opts := demoEngineOptions()
	opts.Templates = map[string]string{"demo": `
schema: {
  region: string
  // +sensitive
  token: string
}
$internal: {key: "demo", keyInputs: []}
output: {region: "eu-west", token: "s3cret"}
`}
	opts.Sensitive = map[string][]string{"demo": {"region"}}

	engine, err := NewSourceEngine(opts)
	r.NoError(err)
	_, err = engine.Resolve(context.Background(), map[string]interface{}{"a": "$(source.cfg.region)"})
	r.NoError(err)
	r.ElementsMatch([]string{"token", "region"}, engine.opts.Sensitive["demo"])
}

// Every Surface constant has to be declared in the context registry.
//
// Two things go wrong quietly otherwise. NewSourceEngine rejects an undeclared
// surface, so a constant that drifted would fail every render using it at
// runtime. And availableFields treats an unrecognised surface as offering
// everything, so before that check it would have silently widened what a source
// may read.
func TestEverySurfaceConstantIsDeclared(t *testing.T) {
	for _, s := range []string{
		SurfaceComponent, SurfaceTrait, SurfaceWorkflowStep,
		SurfacePolicy, SurfacePolicyApp, SurfacePolicyRendered,
	} {
		if !propexpr.SurfaceDeclared(s) {
			t.Errorf("surface %q is not declared in the context registry; declared: %v",
				s, propexpr.SurfaceNames())
		}
	}
}

// Check is what makes the engine usable before anything has been fetched, and
// what a caller reaches for instead of resolving and hoping.
func TestSourceEngineCheck(t *testing.T) {
	r := require.New(t)
	engine, err := NewSourceEngine(demoEngineOptions())
	r.NoError(err)

	r.Empty(engine.Check(map[string]interface{}{
		"a": "$(source.cfg.region)",
		"b": "tier-$(source.cfg.tier)",
		"c": "no expression",
	}), "valid expressions produce no findings")

	found := engine.Check(map[string]interface{}{
		"bad":     "$(source.cfg.nosuchfield)",
		"unknown": "$(source.nosuchbinding.x)",
		"nested":  map[string]interface{}{"deep": "$(source.cfg.region)"},
		"list":    []interface{}{"$(source.cfg.alsomissing)"},
	})
	// Every problem, not the first: a caller validating a blob wants them all.
	r.Len(found, 3)

	byProp := map[string]CheckError{}
	for _, f := range found {
		byProp[f.Property] = f
	}
	// The path says where, including through lists.
	r.Contains(byProp, "bad")
	r.Contains(byProp, "unknown")
	r.Contains(byProp, "list[0]")
	r.NotContains(byProp, "nested.deep", "a valid expression at depth is not reported")
	r.Contains(byProp["bad"].Error(), "bad")
}

// The typed environment is the point. The permissive one types every source read
// as dyn, so a string feeding an int passes unnoticed; Check is what relies on
// the difference.
func TestSourceEngineTypesAgainstTheDeclaredSchema(t *testing.T) {
	r := require.New(t)
	engine, err := NewSourceEngine(demoEngineOptions())
	r.NoError(err)

	r.Empty(engine.Check(map[string]interface{}{"ok": "$(source.cfg.region)"}),
		"a declared path types cleanly")

	bad := engine.Check(map[string]interface{}{"no": "$(source.cfg.nosuchfield)"})
	r.Len(bad, 1, "an undeclared path is a type error, not a dyn")
	r.Equal("no", bad[0].Property)
}

// A caller with no admission step of its own opts in and gets the check without
// having to know to call Check separately.
func TestSourceEngineValidateOnResolve(t *testing.T) {
	r := require.New(t)
	bad := map[string]interface{}{"x": "$(source.cfg.nosuchfield)"}

	// Off by default: the Application checks at admission, and a type error at
	// render is a component that will not reconcile rather than a rejected apply.
	loose, err := NewSourceEngine(demoEngineOptions())
	r.NoError(err)
	_, err = loose.Resolve(context.Background(), bad)
	r.Error(err, "the read still fails, but as a resolution error rather than a type check")
	r.NotContains(err.Error(), "did not type check")

	opts := demoEngineOptions()
	opts.Validate = true
	strict, err := NewSourceEngine(opts)
	r.NoError(err)
	_, err = strict.Resolve(context.Background(), bad)
	r.Error(err)
	r.Contains(err.Error(), "did not type check")

	// A CheckError unwraps, so a caller can match on the underlying cause.
	var ce CheckError
	r.True(errors.As(err, &ce))
	r.Equal("x", ce.Property)

	// ...and valid properties still resolve with validation on.
	res, err := strict.Resolve(context.Background(), map[string]interface{}{"x": "$(source.cfg.region)"})
	r.NoError(err)
	r.Equal("eu-west", res.Properties.(map[string]interface{})["x"])
}
