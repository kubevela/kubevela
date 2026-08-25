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
	"fmt"
	"strings"

	"cuelang.org/go/cue"
)

// KeyExpression renders the CUE expression that becomes storage.key.
//
// This is the *readable prefix* of the cache identity, not the whole of it. The
// resolver appends a hash covering every value the template reads, and that hash
// carries uniqueness on its own - so a segment here is cosmetic, and one that
// resolves to nothing can be dropped without two identities colliding.
//
// Only fields the rules mark as segments are inlined: those that always render to
// a Kubernetes name. A struct, a label value that may contain a dot, or a field
// that is legitimately empty contributes to the hash instead, which is what
// removes the need for a sentinel or a default.
//
// Length is not checked here: the expression is static text, and how long it
// resolves to is only knowable once the interpolation has values.
func KeyExpression(definitionName string, dims []Dimension, rules *Rules) (string, error) {
	if strings.TrimSpace(definitionName) == "" {
		return "", fmt.Errorf("definition name is empty; a cache key needs it as a prefix")
	}
	// The prefix is the one part of the key that is fixed at generation time, so
	// it is also the one part that can be checked now.
	if bad := InvalidCacheKeyChars(definitionName); bad != "" {
		return "", fmt.Errorf("definition name %q contains characters not allowed in a cache key (%s); "+
			"only lowercase letters, digits and '-' are permitted", definitionName, bad)
	}

	var b strings.Builder
	b.WriteString(`"`)
	b.WriteString(definitionName)
	for _, d := range dims {
		if entry, ok := rules.keyedEntry(d.Field); !ok || !entry.Segment {
			continue // contributes to the hash, not to the readable prefix
		}
		b.WriteString(`-\(`)
		b.WriteString(contextIdent)
		b.WriteString(".")
		b.WriteString(d.Field)
		if d.Index != "" {
			// A quoted index inside an interpolation is legal CUE, and keeps the
			// expression readable rather than hiding the label behind an alias.
			b.WriteString(`["`)
			b.WriteString(d.Index)
			b.WriteString(`"]`)
		}
		b.WriteString(`)`)
	}
	b.WriteString(`"`)
	return b.String(), nil
}

// KeyInputs names every value the identity hash must cover, in key order.
//
// The resolver hashes exactly this set. Hashing more - every label on the object,
// say - would change the identity whenever GitOps stamped an unrelated annotation
// and leave the cache permanently cold; hashing less would let two bindings that
// differ in an unrecorded input share an entry.
func KeyInputs(dims []Dimension) []string {
	inputs := make([]string, 0, len(dims))
	for _, d := range dims {
		inputs = append(inputs, d.String())
	}
	return inputs
}

// cuePathKey is the path the expression tests evaluate.
func cuePathKey() cue.Path { return cue.ParsePath("key") }
