/*
Copyright 2021 The KubeVela Authors.

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

package application

import (
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

const (
	// GlobalPolicyCacheTTL defines how long cache entries are valid
	GlobalPolicyCacheTTL = 1 * time.Minute
)

// RenderedPolicyResult stores the complete rendered output of a policy's CUE template
type RenderedPolicyResult struct {
	PolicyName        string                 // Name of the policy
	PolicyNamespace   string                 // Namespace of the policy
	Priority          int32                  // Priority of the policy (for execution order)
	Enabled           bool                   // Whether the policy should be applied
	Transforms        interface{}            // The transforms (*PolicyTransforms) from CUE template
	AdditionalContext map[string]interface{} // The additionalContext field from CUE template
	SkipReason        string                 // Reason for skipping (if enabled=false or error)
	Config            *PolicyConfig          // Policy configuration including refresh settings
}

// GlobalPolicyCacheEntry represents cached rendered results for an Application
type GlobalPolicyCacheEntry struct {
	appSpecHash      string                 // Hash of Application.Spec
	globalPolicyHash string                 // Hash of global policy specs
	renderedResults  []RenderedPolicyResult // Cached rendering results
	timestamp        time.Time              // For TTL
}

// GlobalPolicyCache caches discovered global policies with smart invalidation
type GlobalPolicyCache struct {
	mu      sync.RWMutex
	entries map[string]*GlobalPolicyCacheEntry
}

// NewGlobalPolicyCache creates a new cache instance
func NewGlobalPolicyCache() *GlobalPolicyCache {
	return &GlobalPolicyCache{
		entries: make(map[string]*GlobalPolicyCacheEntry),
	}
}

// Package-level singleton cache instance
var globalPolicyCache = NewGlobalPolicyCache()

// computeGlobalPolicyCacheKey generates a cache key for an Application
func computeGlobalPolicyCacheKey(app *v1beta1.Application) string {
	return fmt.Sprintf("%s/%s", app.Namespace, app.Name)
}

// computeAppSpecHash computes a hash of the Application spec
func computeAppSpecHash(app *v1beta1.Application) (string, error) {
	return apply.ComputeSpecHash(app.Spec)
}

// computeGlobalPolicyHash computes a hash of global policy specs
func computeGlobalPolicyHash(policies []v1beta1.PolicyDefinition) (string, error) {
	if len(policies) == 0 {
		return "empty", nil
	}

	// Hash all policy specs to detect changes
	specs := make([]v1beta1.PolicyDefinitionSpec, len(policies))
	for i, p := range policies {
		specs[i] = p.Spec
	}
	return apply.ComputeSpecHash(specs)
}

// Get retrieves cached rendered policy results if valid
// Returns (renderedResults, cacheHit, error)
func (c *GlobalPolicyCache) Get(app *v1beta1.Application, currentGlobalPolicyHash string) ([]RenderedPolicyResult, bool, error) {
	cacheKey := computeGlobalPolicyCacheKey(app)
	appSpecHash, err := computeAppSpecHash(app)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to compute app spec hash")
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, found := c.entries[cacheKey]
	if !found {
		return nil, false, nil // Cache miss
	}

	// Check if Application spec changed
	if entry.appSpecHash != appSpecHash {
		return nil, false, nil // Spec changed, cache invalid
	}

	// Check if global policies changed
	if entry.globalPolicyHash != currentGlobalPolicyHash {
		return nil, false, nil // Global policies changed, cache invalid
	}

	// Check TTL
	if time.Since(entry.timestamp) > GlobalPolicyCacheTTL {
		return nil, false, nil // Stale, recompute
	}

	// Cache hit!
	return entry.renderedResults, true, nil
}

// Set stores rendered policy results in the cache
func (c *GlobalPolicyCache) Set(app *v1beta1.Application, results []RenderedPolicyResult, globalPolicyHash string) error {
	cacheKey := computeGlobalPolicyCacheKey(app)

	appSpecHash, err := computeAppSpecHash(app)
	if err != nil {
		return errors.Wrap(err, "failed to compute app spec hash")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[cacheKey] = &GlobalPolicyCacheEntry{
		appSpecHash:      appSpecHash,
		renderedResults:  results,
		globalPolicyHash: globalPolicyHash,
		timestamp:        time.Now(),
	}

	return nil
}

// InvalidateForNamespace invalidates all cache entries that might be affected by changes in a namespace
func (c *GlobalPolicyCache) InvalidateForNamespace(namespace string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Invalidate all entries for Applications in this namespace
	for key := range c.entries {
		delete(c.entries, key)
	}
}

// InvalidateAll clears the entire cache
// Used when vela-system global policies change (affects all namespaces)
func (c *GlobalPolicyCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*GlobalPolicyCacheEntry)
}

// InvalidateApplication invalidates cache for a specific Application
func (c *GlobalPolicyCache) InvalidateApplication(namespace, name string) {
	cacheKey := fmt.Sprintf("%s/%s", namespace, name)

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, cacheKey)
}

// Size returns the number of cached entries
func (c *GlobalPolicyCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.entries)
}

// CleanupStale removes stale entries older than TTL
func (c *GlobalPolicyCache) CleanupStale() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := 0
	now := time.Now()

	for key, entry := range c.entries {
		if now.Sub(entry.timestamp) > GlobalPolicyCacheTTL {
			delete(c.entries, key)
			removed++
		}
	}

	return removed
}
