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
)

// MaxCacheKeyLen is the Kubernetes object-name limit. A resolved storage key
// becomes the name of the backing Config object, so it must fit.
const MaxCacheKeyLen = 253

// ValidateCacheKey checks that a resolved SourceDefinition storage key can be
// used as the name of the backing Config object.
//
// Keys are not sanitised automatically: an interpolated value that produces an
// illegal name is a definition-authoring error, and silently rewriting it would
// change the cache identity — and therefore which Applications share an entry.
//
// The admission webhook applies this to statically-known keys; the resolver
// applies it again once interpolation has produced a concrete value.
func ValidateCacheKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("storage.key must not be empty")
	}
	if len(key) > MaxCacheKeyLen {
		return fmt.Errorf("storage.key is %d characters, exceeding the %d-character limit", len(key), MaxCacheKeyLen)
	}
	if bad := InvalidCacheKeyChars(key); bad != "" {
		return fmt.Errorf("storage.key %q contains characters not allowed in a cache key (%s); only lowercase letters, digits and '-' are permitted", key, bad)
	}
	return nil
}

// InvalidCacheKeyChars returns a comma-separated list of the distinct disallowed
// characters in s, or "" when every character is permitted.
func InvalidCacheKeyChars(s string) string {
	seen := map[rune]bool{}
	var bad []string
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		if seen[r] {
			continue
		}
		seen[r] = true
		bad = append(bad, fmt.Sprintf("%q", r))
	}
	return strings.Join(bad, ", ")
}
