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

package cachekey

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// These cases are the frozen behaviour of the current rules. Changing them means
// changing what cache identity a definition gets, which invalidates every
// definition generated against the old rules - so a rules change must add a new
// rules file rather than edit this expectation.
func TestInfer(t *testing.T) {
	cases := []struct {
		name     string
		template string
		want     []string // "field" or "field[index]", in key order
		wantErr  string
	}{
		{
			name: "no context read is a definition-scoped key",
			template: `
schema: {region: string}
output: {region: parameter.region}
parameter: {region: string}
`,
			want: nil,
		},
		{
			name: "a single context read",
			template: `
schema: {region: string}
output: {region: context.cluster}
`,
			want: []string{"cluster"},
		},
		{
			// Declaration order in the template must not affect the key, or the
			// same definition formatted differently would cache differently.
			name: "dimensions are ordered by the rules, not by appearance",
			template: `
output: {
  a: context.name
  b: context.cluster
  c: context.appName
}
`,
			// name is identity, so it leads; the rest follow broad to narrow.
			want: []string{"name", "cluster", "appName"},
		},
		{
			// Scanning only output: would miss this, and the value would still
			// reach the output through the alias.
			name: "an aliased read still counts",
			template: `
_c: context.cluster
output: {region: _c}
`,
			want: []string{"cluster"},
		},
		{
			name: "an indexed read contributes that index",
			template: `
output: {svc: context.appLabels["example.org/service-name"]}
`,
			want: []string{`appLabels[example.org/service-name]`},
		},
		{
			// Two reads of the same map must assemble identically every time.
			name: "indexed reads are ordered by index",
			template: `
output: {
  b: context.appLabels["tier"]
  a: context.appLabels["team"]
}
`,
			want: []string{"appLabels[team]", "appLabels[tier]"},
		},
		{
			name: "the same field read twice contributes once",
			template: `
output: {
  a: context.cluster
  b: context.cluster
}
`,
			want: []string{"cluster"},
		},
		{
			// Over-inclusion is deliberate: a narrower cache is never wrong,
			// and deciding whether a read reaches output: is not tractable.
			name: "a read in errs: still counts",
			template: `
errs: [if context.cluster == "" {"no cluster"}]
output: {region: "us-east-1"}
`,
			want: []string{"cluster"},
		},
		{
			// Caller identity is readable and becomes a key dimension, which is
			// what makes a per-component source possible. It is safe because a
			// source reading it is consumable only where that field exists - the
			// surface-compatibility check enforces that per binding.
			name: "consumer identity is readable and keyed",
			template: `
output: {t: context.componentType, n: context.componentName}
`,
			want: []string{"componentName", "componentType"},
		},
		{
			name: "policy identity is readable and keyed",
			template: `
output: {p: context.policyName}
`,
			want: []string{"policyName"},
		},
		{
			// Hash-only: readable, and it separates cache entries, but it does not
			// lengthen the key - nothing is gained by grepping for a replica.
			name: "a hashed field is readable and keyed without a segment",
			template: `
output: {r: context.revision, k: context.replicaKey}
`,
			// In rules order, not alphabetical - revision is 19, replicaKey 20.
			want: []string{"revision", "replicaKey"},
		},
		{
			// In the registry, but only on policy-app - which renders before the
			// appfile exists and so resolves no sources. Being in the registry is
			// not enough; the surface has to be one a source can resolve on.
			name: "a field whose only surface resolves no sources is rejected",
			template: `
output: {r: context.policyRevisionName}
`,
			wantErr: "policyRevisionName",
		},
		{
			name: "internal plumbing is rejected",
			template: `
output: {p: context.appSourceCacheStore}
`,
			wantErr: "appSourceCacheStore",
		},
		{
			// Fail closed: a context field added later must not silently become
			// an unkeyed dependency.
			name: "an unclassified field is rejected",
			template: `
output: {p: context.somethingNew}
`,
			wantErr: "somethingNew",
		},
		{
			name: "a dynamic index is rejected",
			template: `
parameter: {k: string}
output: {p: context.appLabels[parameter.k]}
`,
			wantErr: "index",
		},
		{
			// storage.key is generated from the reads, so it cannot also be one
			// of them. Scanning it would make a regenerated key depend on the
			// previous key rather than on the resolution logic.
			name: "an existing storage.key is not itself a read",
			template: `
$internal: {
	key:        "old-\(context.appName)"
}
storage: {
	storageTTL: "15m"
}
output: {region: context.cluster}
`,
			want: []string{"cluster"},
		},
		{
			// Only key is generated; the rest of storage: is authored and can
			// legitimately depend on context.
			name: "the rest of storage: is still scanned",
			template: `
$internal: {
	key:        "old-\(context.appName)"
}
storage: {
	storageTTL: context.appLabels["ttl"]
}
output: {region: "us-east-1"}
`,
			want: []string{"appLabels[ttl]"},
		},
		{
			name:     "an unparseable template is an error",
			template: `output: {`,
			wantErr:  "parse",
		},
	}

	rules, err := LoadRules()
	if err != nil {
		t.Fatalf("loading embedded rules: %v", err)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Infer(tc.template, rules)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected an error mentioning %q, got dimensions %v", tc.wantErr, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected an error mentioning %q, got: %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var names []string
			for _, d := range got {
				names = append(names, d.String())
			}
			if len(names) != len(tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, names)
			}
			for i := range names {
				if names[i] != tc.want[i] {
					t.Fatalf("expected %v, got %v", tc.want, names)
				}
			}
		})
	}
}

// The hash identifies the policy, not the file. Editing a comment or the declared
// version must not change it - that would force every stamped definition to be
// regenerated for no behavioural reason. A change to what is readable, or to the
// order it contributes in, must.
func TestPolicyHashCoversBehaviourOnly(t *testing.T) {
	base, err := LoadRules()
	if err != nil {
		t.Fatalf("loading rules: %v", err)
	}

	prose := &Rules{Version: "something else entirely", keyed: base.keyed}
	proseHash, err := prose.policyHash()
	if err != nil {
		t.Fatal(err)
	}
	if proseHash != base.Hash {
		t.Errorf("renaming the version must not change the identity: %q became %q", base.Hash, proseHash)
	}

	// One more readable field.
	added := &Rules{keyed: map[string]keyedField{}}
	for f, e := range base.keyed {
		added.keyed[f] = e
	}
	added.keyed["somethingNew"] = keyedField{Order: 99}
	addedHash, err := added.policyHash()
	if err != nil {
		t.Fatal(err)
	}
	if addedHash == base.Hash {
		t.Error("making another field readable must change the identity")
	}

	// Same fields, different order.
	reordered := &Rules{keyed: map[string]keyedField{}}
	for f, e := range base.keyed {
		e.Order += 100
		reordered.keyed[f] = e
	}
	reorderedHash, err := reordered.policyHash()
	if err != nil {
		t.Fatal(err)
	}
	if reorderedHash == base.Hash {
		t.Error("reordering the key segments must change the identity")
	}
}

// Everything outside the keyed list gets the same rejection, naming the field and
// pointing at properties.
func TestUnsupportedContextIsOneMessage(t *testing.T) {
	rules, err := LoadRules()
	if err != nil {
		t.Fatalf("loading rules: %v", err)
	}
	// Everything the registry offers a source-resolving surface is keyed now, so
	// what remains unsupported is internal plumbing, a render product, a field
	// whose only surface resolves no sources, and one that does not exist at all.
	for _, field := range []string{"appSourceCacheStore", "outputs", "policyRevisionName", "somethingNobodyHasAddedYet"} {
		_, err := Infer("output: {p: context."+field+"}\n", rules)
		if err == nil {
			t.Errorf("context.%s should be unsupported", field)
			continue
		}
		if !strings.Contains(err.Error(), "context."+field) {
			t.Errorf("rejection should name the field; got: %v", err)
		}
		if !strings.Contains(err.Error(), "properties") {
			t.Errorf("rejection should point at properties; got: %v", err)
		}
	}
}

// The stamped hash is a published value, not an internal one.
//
// Every generated SourceDefinition carries definition.oam.dev/cache-key-rules,
// and a definition stamped with one rules version resolving under another is the
// silent failure the annotation exists to prevent. The literal is pinned here so
// an accidental edit to the rules file is a failing test rather than a mismatch
// discovered against a cluster - the other hash tests prove the function is
// stable, not that this particular value has not moved.
func TestStampedRulesHashHasNotMoved(t *testing.T) {
	const stamped = "6ac674fa" // as written into every generated SourceDefinition

	rules, err := LoadRules()
	if err != nil {
		t.Fatalf("loading rules: %v", err)
	}
	got, err := rules.policyHash()
	if err != nil {
		t.Fatalf("hashing rules: %v", err)
	}
	if got != stamped {
		t.Fatalf("the rules hash is now %q, was %q.\n"+
			"If the keyed rules genuinely changed, restamp every generated definition "+
			"(`make manifests`, plus docs/examples/source-expressions-demo/definitions/*.yaml) "+
			"and update this constant. If they did not, something edited "+
			"pkg/definition/cachekey/rules/ by accident", got, stamped)
	}
}

// The resolver hands a template the whole appLabels map, but inference recorded
// nothing for a read of it: only an indexed read produced a dimension. Two
// Applications differing only in their labels therefore resolved to one cache
// key and shared a result that was computed from one of them.
func TestInferRefusesAWholeMapRead(t *testing.T) {
	rules, err := LoadRules()
	require.NoError(t, err)

	for _, tc := range []struct{ name, template string }{
		{"interpolated", "schema: {v: string}\noutput: {v: \"\\(context.appLabels)\"}\n"},
		{"bound to a hidden field", "schema: {v: string}\n_l: context.appAnnotations\noutput: {v: \"x\"}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Infer(tc.template, rules)
			require.ErrorContains(t, err, "literal key")
		})
	}

	// An indexed read is what the field exists for, and still works.
	dims, err := Infer("schema: {v: string}\noutput: {v: context.appLabels[\"team\"]}\n", rules)
	require.NoError(t, err)
	require.Len(t, dims, 1)
	require.Equal(t, "appLabels[team]", dims[0].String())
}
