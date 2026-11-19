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
	"regexp"
	"strings"
	"testing"
)

// cue fmt aligns values within a struct, so an assertion on generated text has to
// ignore the padding rather than encode whichever alignment happened to apply.
var spaceRun = regexp.MustCompile(`[ \t]+`)

func normalise(s string) string { return spaceRun.ReplaceAllString(s, " ") }

func TestStamp(t *testing.T) {
	cases := []struct {
		name     string
		defName  string
		template string
		wantKey  string
		wantKeep []string // fragments that must survive stamping
		wantErr  string
	}{
		{
			name:    "adds a storage block when there is none",
			defName: "cluster-lookup",
			template: `
schema: {region: string}
output: {region: context.cluster}
`,
			wantKey:  `key: "cluster-lookup-\(context.cluster)"`,
			wantKeep: []string{"schema:", "output:", "context.cluster"},
		},
		{
			name:    "adds the key to an existing storage block, preserving its other fields",
			defName: "tenant-data",
			template: `
schema: {tenant: string}
storage: {
  storageTTL:     "15m"
  onStaleFailure: "fail"
}
output: {tenant: context.namespace}
`,
			wantKey:  `key: "tenant-data-\(context.namespace)"`,
			wantKeep: []string{`storageTTL:`, `"15m"`, `onStaleFailure:`, `"fail"`},
		},
		{
			// vela def get emits the stored template, key included, so a
			// round-trip through get/edit/apply has to be accepted.
			name:    "accepts a key that matches what inference produces",
			defName: "cluster-lookup",
			template: `
$internal: {
	key:       "cluster-lookup-\(context.cluster)"
	keyInputs: ["cluster"]
}
storage: {
	storageTTL: "15m"
}
output: {region: context.cluster}
`,
			wantKey:  `key: "cluster-lookup-\(context.cluster)"`,
			wantKeep: []string{`storageTTL:`},
		},
		{
			// Silently rewriting it would hide that the author believed something
			// different about how their source is cached.
			name:    "rejects a key that does not match",
			defName: "cluster-lookup",
			template: `
$internal: {key: "hand-written"}
storage: {storageTTL: "15m"}
output: {region: context.cluster}
`,
			wantErr: "hand-written",
		},
		{
			name:    "a source reading no context gets a bare key",
			defName: "backstage-component",
			template: `
schema: {owner: string}
output: {owner: parameter.ref}
parameter: {ref: string}
`,
			wantKey: `key: "backstage-component"`,
		},
		{
			name:    "a forbidden read is reported rather than stamped",
			defName: "bad",
			template: `
output: {p: context.appSourceCacheStore}
`,
			wantErr: "appSourceCacheStore",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, rules, err := Stamp(tc.defName, tc.template)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected an error mentioning %q, got:\n%s", tc.wantErr, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected an error mentioning %q, got: %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rules == nil || rules.Hash == "" || rules.Version == "" {
				t.Fatal("expected the rules to be returned, so hash and version can be recorded on the object")
			}
			if !strings.Contains(normalise(got), normalise(tc.wantKey)) {
				t.Fatalf("expected the template to contain:\n  %s\ngot:\n%s", tc.wantKey, got)
			}
			for _, keep := range tc.wantKeep {
				if !strings.Contains(normalise(got), normalise(keep)) {
					t.Fatalf("stamping dropped %q from the template:\n%s", keep, got)
				}
			}
		})
	}
}

// Stamping twice must not change the result, or a re-apply of an unchanged
// definition would show a diff and GitOps would never converge.
func TestStampIsIdempotent(t *testing.T) {
	const template = `
schema: {region: string}
storage: {storageTTL: "15m"}
output: {region: context.cluster, svc: context.appLabels["team"]}
`
	once, rules1, err := Stamp("cluster-lookup", template)
	if err != nil {
		t.Fatalf("first stamp: %v", err)
	}
	twice, rules2, err := Stamp("cluster-lookup", once)
	if err != nil {
		t.Fatalf("second stamp: %v", err)
	}
	if once != twice {
		t.Fatalf("stamping is not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}
	if rules1.Hash != rules2.Hash {
		t.Fatalf("rules hash changed between stamps: %q then %q", rules1.Hash, rules2.Hash)
	}
}

// The stamped template must still be valid CUE - it is what the controller
// compiles at resolve time.
func TestStampedTemplateParses(t *testing.T) {
	const template = `
schema: {region: string}
storage: {storageTTL: "15m"}
output: {region: context.cluster}
`
	got, _, err := Stamp("cluster-lookup", template)
	if err != nil {
		t.Fatalf("stamping: %v", err)
	}
	rules, err := LoadRules()
	if err != nil {
		t.Fatalf("loading rules: %v", err)
	}
	// Re-inferring the stamped output must give the same dimensions: proof the
	// generated key did not become an input to its own regeneration.
	dims, err := Infer(got, rules)
	if err != nil {
		t.Fatalf("stamped template no longer infers: %v\n%s", err, got)
	}
	if len(dims) != 1 || dims[0].Field != "cluster" {
		t.Fatalf("expected [cluster] after stamping, got %v", dims)
	}
}

// A mismatch has to name the value that was expected, or the only way to fix it
// is to delete the line and guess.
func TestStampMismatchNamesTheExpectedKey(t *testing.T) {
	const template = `
$internal: {key: "hand-written"}
output: {region: context.cluster}
`
	_, _, err := Stamp("cluster-lookup", template)
	if err == nil {
		t.Fatal("expected a mismatched key to be rejected")
	}
	for _, want := range []string{"hand-written", `cluster-lookup-\(context.cluster)`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error should mention %q so the fix is obvious; got: %v", want, err)
		}
	}
}

// Verify is the admission-side half of the contract: the CLI writes the key, and
// this re-derives it to check nothing edited the artifact afterwards.
func TestVerify(t *testing.T) {
	current, err := LoadRules()
	if err != nil {
		t.Fatalf("loading rules: %v", err)
	}

	const good = `
$internal: {key: "cluster-lookup-\(context.cluster)", keyInputs: ["cluster"]}
storage: {storageTTL: "15m"}
output: {region: context.cluster}
`

	t.Run("a matching key with the rules that produced it", func(t *testing.T) {
		if err := Verify("cluster-lookup", good, current.Hash); err != nil {
			t.Fatalf("expected acceptance, got: %v", err)
		}
	})

	t.Run("a matching key with no recorded rules falls back to the current ones", func(t *testing.T) {
		// Hand-written YAML carries no annotation. It still has to match, just
		// against today's policy rather than a pinned one.
		if err := Verify("cluster-lookup", good, ""); err != nil {
			t.Fatalf("expected acceptance, got: %v", err)
		}
	})

	t.Run("a tampered key is rejected", func(t *testing.T) {
		const tampered = `
$internal: {key: "something-else"}
output: {region: context.cluster}
`
		err := Verify("cluster-lookup", tampered, current.Hash)
		if err == nil {
			t.Fatal("expected a tampered key to be rejected")
		}
		for _, want := range []string{"something-else", `cluster-lookup-\(context.cluster)`} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error should name %q; got: %v", want, err)
			}
		}
	})

	t.Run("a missing key is rejected", func(t *testing.T) {
		const noKey = `
storage: {storageTTL: "15m"}
output: {region: context.cluster}
`
		if err := Verify("cluster-lookup", noKey, current.Hash); err == nil {
			t.Fatal("expected a template with no storage.key to be rejected")
		}
	})

	t.Run("rules that this build does not have are rejected", func(t *testing.T) {
		// Better to refuse than to validate against different rules than the ones
		// that generated the key - that would defeat recording it.
		err := Verify("cluster-lookup", good, "deadbeef")
		if err == nil {
			t.Fatal("expected an unknown rules hash to be rejected")
		}
		if !strings.Contains(err.Error(), "deadbeef") {
			t.Fatalf("error should name the missing hash; got: %v", err)
		}
	})

	// keyInputs needs checking on its own account. Only some fields are inlined
	// into the key, so a hashed-only one can be removed while storage.key still
	// matches perfectly - and every value that field distinguished then collapses
	// onto a single cache entry.
	t.Run("dropping a hashed-only input is rejected even though the key still matches", func(t *testing.T) {
		const poisoned = `
$internal: {key: "svc-\(context.cluster)", keyInputs: ["cluster"]}
output: {
  region: context.cluster
  team:   context.appLabels["team"]
}
`
		// The premise: the label contributes nothing to the key expression, so the
		// key alone cannot detect its removal.
		if !strings.Contains(poisoned, `key: "svc-\(context.cluster)"`) {
			t.Fatal("fixture no longer exercises a key that is unaffected by the dropped input")
		}

		err := Verify("svc", poisoned, current.Hash)
		if err == nil {
			t.Fatal("a keyInputs list missing a value the template reads must be rejected: " +
				"every value of that label would share one cache entry")
		}
		if !strings.Contains(err.Error(), "appLabels[team]") {
			t.Fatalf("error should name the missing input; got: %v", err)
		}
	})

	t.Run("an extra input is rejected", func(t *testing.T) {
		// The mirror image: hashing a field the template never reads fragments the
		// cache and makes the identity depend on data nobody consumes.
		const extra = `
$internal: {key: "cluster-lookup-\(context.cluster)", keyInputs: ["cluster", "namespace"]}
output: {region: context.cluster}
`
		if err := Verify("cluster-lookup", extra, current.Hash); err == nil {
			t.Fatal("expected an input the template does not read to be rejected")
		}
	})

	t.Run("a missing keyInputs is rejected", func(t *testing.T) {
		const noInputs = `
$internal: {key: "cluster-lookup-\(context.cluster)"}
storage: {storageTTL: "15m"}
output: {region: context.cluster}
`
		err := Verify("cluster-lookup", noInputs, current.Hash)
		if err == nil {
			t.Fatal("expected a template with no storage.keyInputs to be rejected")
		}
		if !strings.Contains(err.Error(), KeyInputsField) {
			t.Fatalf("error should name the missing field; got: %v", err)
		}
	})

	t.Run("a malformed keyInputs is rejected rather than read as absent", func(t *testing.T) {
		const malformed = `
$internal: {key: "cluster-lookup-\(context.cluster)", keyInputs: "cluster"}
output: {region: context.cluster}
`
		if err := Verify("cluster-lookup", malformed, current.Hash); err == nil {
			t.Fatal("a keyInputs that is not a list of strings must not be waved through")
		}
	})

	t.Run("order is part of the contract", func(t *testing.T) {
		// The resolver hashes a structured document, so order does not change the
		// identity - but a reordered list is still not what inference produces, and
		// accepting it would mean the stored artifact no longer round-trips.
		const reordered = `
$internal: {key: "svc-\(context.cluster)-\(context.namespace)", keyInputs: ["namespace", "cluster"]}
output: {
  region: context.cluster
  ns:     context.namespace
}
`
		if err := Verify("svc", reordered, current.Hash); err == nil {
			t.Fatal("expected a reordered keyInputs to be rejected")
		}
	})
}

// Stamp rejects a hand-edited keyInputs rather than silently correcting it, for
// the same reason it rejects a mismatched key: the author's belief about how
// their source is cached should not be quietly overwritten.
func TestStampRejectsMismatchedKeyInputs(t *testing.T) {
	// The key is correct, so only keyInputs is wrong - otherwise this would be
	// testing the key check instead.
	const template = `
$internal: {key: "svc-\(context.cluster)", keyInputs: ["cluster"]}
output: {
  region: context.cluster
  team:   context.appLabels["team"]
}
`
	_, _, err := Stamp("svc", template)
	if err == nil {
		t.Fatal("expected a mismatched keyInputs to be rejected")
	}
	if !strings.Contains(err.Error(), "appLabels[team]") {
		t.Fatalf("error should name what is missing; got: %v", err)
	}
}
