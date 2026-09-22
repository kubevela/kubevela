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
	"strings"
	"testing"

	"github.com/oam-dev/kubevela/pkg/definition/cachekey"
)

func inputs(template string, props, ctx map[string]interface{}) identityInputs {
	return identityInputs{Template: template, Properties: props, Context: ctx}
}

// The readable prefix is cosmetic; the hash carries uniqueness. So the hash is
// always present, even for a source that reads nothing and takes no properties -
// the definition's own template is always a contributor.
func TestCacheIdentityAlwaysHashes(t *testing.T) {
	got, err := cacheIdentity("cluster-lookup-prod", inputs("t1", nil, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, "cluster-lookup-prod-") {
		t.Fatalf("the readable prefix must lead: got %q", got)
	}
	if got == "cluster-lookup-prod" {
		t.Fatal("the hash must always be appended, or uniqueness has nowhere to live")
	}
	if err := cachekey.ValidateCacheKey(got); err != nil {
		t.Fatalf("the identity must be a legal object name: %v", err)
	}
}

// The case that drove this design: a template may branch on whether a label is
// set, so absent, present-but-empty and set-to-a-value are three different
// inputs and must be three different identities.
func TestCacheIdentityDistinguishesAbsentFromEmpty(t *testing.T) {
	absent, err := cacheIdentity("svc", inputs("t1", nil, map[string]interface{}{
		"appLabels": map[string]interface{}{"team": nil},
	}))
	if err != nil {
		t.Fatal(err)
	}
	empty, err := cacheIdentity("svc", inputs("t1", nil, map[string]interface{}{
		"appLabels": map[string]interface{}{"team": ""},
	}))
	if err != nil {
		t.Fatal(err)
	}
	set, err := cacheIdentity("svc", inputs("t1", nil, map[string]interface{}{
		"appLabels": map[string]interface{}{"team": "platform"},
	}))
	if err != nil {
		t.Fatal(err)
	}

	if absent == empty {
		t.Error("an absent label and an empty one produce different output; they must not share an entry")
	}
	if empty == set {
		t.Error("an empty label and a set one must not share an entry")
	}
	if absent == set {
		t.Error("an absent label and a set one must not share an entry")
	}
}

// A property and a context value of the same name must not be confusable, which
// is why the hash covers a structured document rather than concatenated text.
func TestCacheIdentityKeepsPropertiesAndContextApart(t *testing.T) {
	asProperty, err := cacheIdentity("svc", inputs("t1", map[string]interface{}{"team": "platform"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	asContext, err := cacheIdentity("svc", inputs("t1", nil, map[string]interface{}{"team": "platform"}))
	if err != nil {
		t.Fatal(err)
	}
	if asProperty == asContext {
		t.Fatal("a property named like a context field must not collide with it")
	}
}

// Editing a definition - its schema or its fetch logic - must orphan the entries
// resolved by the previous version, since cached values are served without
// re-validation. That is finding #11, closed by the template being an input.
func TestCacheIdentityChangesWithTheTemplate(t *testing.T) {
	before, err := cacheIdentity("svc", inputs("t1", nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	after, err := cacheIdentity("svc", inputs("t2", nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("a changed template must produce a different identity")
	}
}

// The two built-in ConfigMap sources generate prefixes that can land on the same
// string: configmap-local in cluster "local" namespace "default" reads
// configmap-local-local-default, and so does configmap in cluster "local"
// namespace "local-default". Their parameters can also match, since
// configmap-local's `name` is the only one configmap requires.
//
// The templates differ, and that is the whole reason the prefix is allowed to be
// cosmetic. Pinning it here because the pair now ships together, so a future
// change to how prefixes are built has a shipped collision to answer for.
func TestCacheIdentitySeparatesTheTwoConfigMapSources(t *testing.T) {
	props := map[string]interface{}{"name": "app-config"}
	local, err := cacheIdentity("configmap-local-local-default", inputs("configmap-local-template", props, nil))
	if err != nil {
		t.Fatal(err)
	}
	generic, err := cacheIdentity("configmap-local-local-default", inputs("configmap-template", props, nil))
	if err != nil {
		t.Fatal(err)
	}
	if local == generic {
		t.Fatal("configmap and configmap-local must not share a cache entry when their prefixes collide")
	}
}

func TestCacheIdentityIsStable(t *testing.T) {
	// Map iteration order must not leak into the hash, or the same binding would
	// address a different entry on each reconcile.
	in := inputs("t1",
		map[string]interface{}{"zebra": "z", "alpha": "a", "middle": 3},
		map[string]interface{}{"appLabels": map[string]interface{}{"b": 2, "a": 1}, "cluster": "prod"},
	)
	first, err := cacheIdentity("svc", in)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		again, err := cacheIdentity("svc", in)
		if err != nil {
			t.Fatal(err)
		}
		if again != first {
			t.Fatalf("identity is not stable across calls: %q then %q", first, again)
		}
	}
}

// Uniqueness lives in the hash, so an over-long identity trims the cosmetic
// prefix and keeps the hash intact. Re-hashing the whole thing would discard
// readability exactly when the name is longest.
func TestCacheIdentityTruncatesThePrefixNotTheHash(t *testing.T) {
	long := strings.Repeat("a", cachekey.MaxCacheKeyLen+50)

	got, err := cacheIdentity(long, inputs("t1", map[string]interface{}{"x": "y"}, nil))
	if err != nil {
		t.Fatalf("an over-long prefix must be trimmed, not rejected: %v", err)
	}
	if len(got) > cachekey.MaxCacheKeyLen {
		t.Fatalf("identity is %d characters, over the %d limit", len(got), cachekey.MaxCacheKeyLen)
	}
	if err := cachekey.ValidateCacheKey(got); err != nil {
		t.Fatalf("the trimmed identity must still be legal: %v", err)
	}

	// The hash survives intact, so two over-long prefixes with different inputs
	// stay distinct even though their visible parts are identical.
	other, err := cacheIdentity(long, inputs("t1", map[string]interface{}{"x": "z"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if got == other {
		t.Fatal("trimming must not collapse distinct inputs")
	}
}
