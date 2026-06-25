/*
Copyright 2026 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package upgrade

import (
	"testing"
)

func entry(requiresUpgrade bool, upgraded string) compatEntry {
	return compatEntry{requiresUpgrade: requiresUpgrade, upgraded: upgraded}
}

func TestLRUCacheBasic(t *testing.T) {
	c := newLRUCache(3)

	c.put("a", entry(false, "1"))
	c.put("b", entry(false, "2"))
	c.put("c", entry(false, "3"))

	if v, ok := c.get("a"); !ok || v.upgraded != "1" {
		t.Errorf("expected a.upgraded=1, got %q %v", v.upgraded, ok)
	}

	// Adding a 4th entry should evict the LRU — "b" (least recently used after "a" was promoted)
	c.put("d", entry(false, "4"))
	if _, ok := c.get("b"); ok {
		t.Error("expected b to be evicted")
	}
	if v, ok := c.get("d"); !ok || v.upgraded != "4" {
		t.Errorf("expected d.upgraded=4, got %q %v", v.upgraded, ok)
	}
}

func TestLRUCacheUpdate(t *testing.T) {
	c := newLRUCache(2)
	c.put("a", entry(false, "1"))
	c.put("a", entry(true, "2"))
	if v, ok := c.get("a"); !ok || v.upgraded != "2" || !v.requiresUpgrade {
		t.Errorf("expected updated value a.upgraded=2 requiresUpgrade=true, got %q %v %v", v.upgraded, v.requiresUpgrade, ok)
	}
	if c.len() != 1 {
		t.Errorf("expected len 1, got %d", c.len())
	}
}

func TestLRUCacheCapacity(t *testing.T) {
	capacity := 10
	c := newLRUCache(capacity)
	for i := range capacity * 2 {
		c.put(string(rune('a'+i)), entry(false, ""))
	}
	if c.len() != capacity {
		t.Errorf("expected len %d, got %d", capacity, c.len())
	}
}

func TestEnsureCueVersionCompatibilityCacheHit(t *testing.T) {
	compatCache.Store(newLRUCache(512))

	input := `
list1: [1, 2, 3]
list2: [4, 5, 6]
combined: list1 + list2
`
	// First call — cache miss, runs upgrade
	result1, _ := EnsureCueVersionCompatibility(input, "test-def", ComponentKind, TemplateAreaMain)

	const sentinel = "CACHE_HIT_SENTINEL"
	key := templateHash(input)
	compatCache.Load().put(key, compatEntry{requiresUpgrade: true, upgraded: sentinel})

	// Second call — must return the sentinel from cache, not a freshly computed value
	result2, _ := EnsureCueVersionCompatibility(input, "test-def", ComponentKind, TemplateAreaMain)

	if result2 != sentinel {
		t.Errorf("second call did not use cache: expected sentinel %q, got %q (first result was %q)", sentinel, result2, result1)
	}
	if compatCache.Load().len() != 1 {
		t.Errorf("expected 1 cache entry, got %d", compatCache.Load().len())
	}
}

func TestEnsureCueVersionCompatibilityAlreadyCompatible(t *testing.T) {
	compatCache.Store(newLRUCache(512))

	// Use canonical CUE formatting (no leading newline, blank line after import).
	input := `import "list"

list1: [1, 2, 3]
list2: [4, 5, 6]
combined: list.Concat([list1, list2])`
	result1, _ := EnsureCueVersionCompatibility(input, "test-def", ComponentKind, TemplateAreaMain)
	if result1 != input {
		t.Errorf("already-compatible template should be returned unchanged on first call, got %q", result1)
	}
	const sentinel = "CACHE_HIT_SENTINEL"
	key := templateHash(input)
	compatCache.Load().put(key, compatEntry{requiresUpgrade: true, upgraded: sentinel})

	result2, _ := EnsureCueVersionCompatibility(input, "test-def", ComponentKind, TemplateAreaMain)
	if result2 != sentinel {
		t.Errorf("second call did not use cache: expected sentinel %q, got %q", sentinel, result2)
	}
	if compatCache.Load().len() != 1 {
		t.Errorf("expected 1 cache entry, got %d", compatCache.Load().len())
	}
}
