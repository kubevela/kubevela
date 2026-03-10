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

package application

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

const (
	// ApplicationPolicyCacheTTL defines how long cache entries are valid
	ApplicationPolicyCacheTTL = 1 * time.Minute

	// Policy source values for RenderedPolicyResult.Source
	PolicySourceGlobal   = "global"
	PolicySourceExplicit = "explicit"

	// Cache miss reasons returned by GetWithReason
	CacheMissNotFound        = "not_found"
	CacheMissSpecChanged     = "spec_changed"
	CacheMissRevisionChanged = "revision_changed"
	CacheMissTTLExpired      = "ttl_expired"
)

// RenderedPolicyResult stores the output of a rendered policy CUE template.
type RenderedPolicyResult struct {
	PolicyName        string
	PolicyType        string
	PolicyNamespace   string
	Priority          int32
	Enabled           bool
	Source            string                 // PolicySourceGlobal or PolicySourceExplicit
	Transforms        interface{}            // *PolicyOutput
	AdditionalContext map[string]interface{} // output.ctx
	SkipReason        string
	IsError           bool // true when SkipReason is due to an error, false for config.enabled=false

	// VersionKey is the namespaced key used in the handler's policyVersions map.
	// For globals: "global:<defName>". For explicit: "<policy.Name>".
	// Stored on the result so cache-restore can rebuild the map with correct keys.
	VersionKey string

	// Version tracking for status observability.
	DefinitionRevisionName string
	Revision               int64
	RevisionHash           string
	PolicyDefinitionUsed   *v1beta1.PolicyDefinition

	// Per-policy spec snapshots for ConfigMap audit trail; nil when spec was not modified.
	SpecBefore *v1beta1.ApplicationSpec
	SpecAfter  *v1beta1.ApplicationSpec
}

// ApplicationPolicyCacheEntry holds rendered results for one Application.
// Invalidated when spec hash changes, ApplicationRevision changes, or TTL expires.
type ApplicationPolicyCacheEntry struct {
	appSpecHash     string
	appRevisionHash string
	renderedResults []RenderedPolicyResult
	timestamp       time.Time
}

// ApplicationPolicyCache caches rendered policy results keyed by namespace/name.
type ApplicationPolicyCache struct {
	mu      sync.RWMutex
	entries map[string]*ApplicationPolicyCacheEntry
}

func NewApplicationPolicyCache() *ApplicationPolicyCache {
	return &ApplicationPolicyCache{entries: make(map[string]*ApplicationPolicyCacheEntry)}
}

var applicationPolicyCache = NewApplicationPolicyCache()

func computeApplicationPolicyCacheKey(app *v1beta1.Application) string {
	return fmt.Sprintf("%s/%s", app.Namespace, app.Name)
}

func computeApplicationSpecHash(app *v1beta1.Application) (string, error) {
	return apply.ComputeSpecHash(app.Spec)
}

// Get retrieves cached results. Returns (results, hit, error).
func (c *ApplicationPolicyCache) Get(app *v1beta1.Application) ([]RenderedPolicyResult, bool, error) {
	results, hit, _, err := c.GetWithReason(app)
	return results, hit, err
}

// GetWithReason retrieves cached results with a miss reason for diagnostics.
// missReason: CacheMissNotFound | CacheMissSpecChanged | CacheMissRevisionChanged | CacheMissTTLExpired | "" (hit)
func (c *ApplicationPolicyCache) GetWithReason(app *v1beta1.Application) ([]RenderedPolicyResult, bool, string, error) {
	cacheKey := computeApplicationPolicyCacheKey(app)
	appSpecHash, err := computeApplicationSpecHash(app)
	if err != nil {
		return nil, false, "error", errors.Wrap(err, "failed to compute app spec hash")
	}

	currentAppRevHash := ""
	if app.Status.LatestRevision != nil {
		currentAppRevHash = app.Status.LatestRevision.RevisionHash
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, found := c.entries[cacheKey]
	if !found {
		return nil, false, CacheMissNotFound, nil // Cache miss - first time
	}

	if entry.appSpecHash != appSpecHash {
		return nil, false, CacheMissSpecChanged, nil
	}
	if entry.appRevisionHash != currentAppRevHash {
		return nil, false, CacheMissRevisionChanged, nil
	}
	if time.Since(entry.timestamp) > ApplicationPolicyCacheTTL {
		return nil, false, CacheMissTTLExpired, nil
	}
	return entry.renderedResults, true, "", nil
}

// Set stores rendered policy results in the cache
func (c *ApplicationPolicyCache) Set(app *v1beta1.Application, results []RenderedPolicyResult) error {
	cacheKey := computeApplicationPolicyCacheKey(app)

	appSpecHash, err := computeApplicationSpecHash(app)
	if err != nil {
		return errors.Wrap(err, "failed to compute app spec hash")
	}

	appRevisionHash := ""
	if app.Status.LatestRevision != nil {
		appRevisionHash = app.Status.LatestRevision.RevisionHash
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[cacheKey] = &ApplicationPolicyCacheEntry{
		appSpecHash:     appSpecHash,
		appRevisionHash: appRevisionHash,
		renderedResults: results,
		timestamp:       time.Now(),
	}

	return nil
}

// InvalidateForNamespace removes all cache entries for Applications in the given namespace.
func (c *ApplicationPolicyCache) InvalidateForNamespace(namespace string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := namespace + "/"
	for key := range c.entries {
		if strings.HasPrefix(key, prefix) {
			delete(c.entries, key)
		}
	}
}

// InvalidateAll clears the entire cache. Call when global (vela-system) policies change.
func (c *ApplicationPolicyCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*ApplicationPolicyCacheEntry)
}

func (c *ApplicationPolicyCache) InvalidateApplication(namespace, name string) {
	cacheKey := fmt.Sprintf("%s/%s", namespace, name)

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, cacheKey)
}

func (c *ApplicationPolicyCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.entries)
}

// CleanupStale removes entries older than ApplicationPolicyCacheTTL and returns the count removed.
func (c *ApplicationPolicyCache) CleanupStale() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := 0
	now := time.Now()

	for key, entry := range c.entries {
		if now.Sub(entry.timestamp) > ApplicationPolicyCacheTTL {
			delete(c.entries, key)
			removed++
		}
	}

	return removed
}
