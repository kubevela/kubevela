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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/oam-dev/kubevela/pkg/definition/cachekey"
)

// identityHashLen is long enough that a collision is not a practical concern,
// and short enough to leave the readable prefix visible.
//
// 64 bits. A collision means one binding served another binding's data, so the
// margin is worth more than the eight characters of prefix it costs: at 32 bits
// the birthday bound puts an even chance of one at around 77,000 identities
// sharing a prefix, which a large platform reaches.
const identityHashLen = 16

// identityInputs is everything a cache entry's identity depends on.
//
// It is hashed as a structured document rather than concatenated text, so a
// property cannot be confused with a context field of the same name.
type identityInputs struct {
	// Template fingerprints the definition that produced the value. Cached values
	// are served without re-validation, so a definition whose schema or fetch
	// logic changed must not keep addressing entries resolved by the old one.
	Template string `json:"template"`
	// Properties are the binding's inputs.
	Properties map[string]interface{} `json:"properties,omitempty"`
	// Context holds the values the template reads: nil for a field that is absent,
	// "" for one that is present and empty. A template may branch on the
	// difference, so the identity has to draw it too.
	Context map[string]interface{} `json:"context,omitempty"`
}

// cacheIdentity assembles the name of the backing Config object.
//
// The prefix is cosmetic - it exists so an operator can grep - and the hash
// carries uniqueness on its own. That split is what lets a value be left out of
// the prefix without consequence: two identities differing only in an omitted
// segment still differ in the hash. It is also why nothing here needs a sentinel
// for an absent or empty value, or a character rule for a value that cannot be
// rendered into a name.
func cacheIdentity(prefix string, in identityInputs) (string, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("hashing cache identity inputs: %w", err)
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])[:identityHashLen]

	identity := prefix + "-" + hash
	if len(identity) <= cachekey.MaxCacheKeyLen {
		return identity, nil
	}

	// Trim the prefix, never the hash: uniqueness lives in the hash, so a shorter
	// prefix cannot cause a collision, whereas re-hashing the whole name would
	// throw away readability exactly when it is longest.
	room := cachekey.MaxCacheKeyLen - len(hash) - 1
	if room < 1 {
		return hash, nil
	}
	return trimTrailingSeparator(prefix[:room]) + "-" + hash, nil
}

// trimTrailingSeparator avoids a doubled separator where the cut lands on one.
func trimTrailingSeparator(s string) string {
	for len(s) > 0 && s[len(s)-1] == '-' {
		s = s[:len(s)-1]
	}
	return s
}

// templateFingerprint identifies the definition a value was resolved by.
//
// Cached values are served without re-validation, so a definition whose schema or
// fetch logic has changed must stop addressing the entries its previous version
// produced - otherwise a changed URL with an unchanged output shape keeps serving
// data fetched by the old logic.
func templateFingerprint(template string) string {
	sum := sha256.Sum256([]byte(template))
	return hex.EncodeToString(sum[:])[:identityHashLen]
}
