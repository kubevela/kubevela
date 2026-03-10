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
	"context"
	"sort"
	"sync"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// PolicyMetadata stores lightweight metadata about a PolicyDefinition
// This allows us to check scope and other properties without fetching the full definition
type PolicyMetadata struct {
	Name            string
	Namespace       string
	Scope           v1beta1.PolicyScope
	Global          bool
	Priority        int32
	ResourceVersion string
}

// PolicyScopeIndex maintains an in-memory index of PolicyDefinition metadata
// This index is eagerly populated at startup and kept synchronized via watch events
// It also supports lazy initialization on first access for testing scenarios
type PolicyScopeIndex struct {
	mu sync.RWMutex

	// Fast lookups by name: namespace → policyName → metadata
	byNamespace map[string]map[string]*PolicyMetadata

	// Pre-filtered lists for global Application-scoped policies
	// namespace → sorted list of global Application-scoped policies
	globalApplicationPolicies map[string][]*PolicyMetadata

	// Track if index has been initialized
	initialized bool
	client      client.Client
}

// NewPolicyScopeIndex creates a new empty index
func NewPolicyScopeIndex() *PolicyScopeIndex {
	return &PolicyScopeIndex{
		byNamespace:               make(map[string]map[string]*PolicyMetadata),
		globalApplicationPolicies: make(map[string][]*PolicyMetadata),
	}
}

// Package-level singleton index instance
var policyScopeIndex = NewPolicyScopeIndex()

// ensureInitialized lazily initializes the index if it hasn't been initialized yet
// This allows the index to work in test scenarios where explicit initialization isn't called
func (idx *PolicyScopeIndex) ensureInitialized(ctx context.Context) {
	idx.mu.RLock()
	if idx.initialized {
		idx.mu.RUnlock()
		return
	}
	idx.mu.RUnlock()

	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.initialized {
		return
	}

	if idx.client != nil {
		klog.V(4).InfoS("Lazy-initializing PolicyScopeIndex")
		if err := idx.initializeLocked(ctx, idx.client); err != nil {
			klog.ErrorS(err, "Failed to lazy-initialize PolicyScopeIndex")
			// Do not mark as initialized — allow retry on next reconcile.
			return
		}
	}

	idx.initialized = true
}

// Initialize populates the index by listing all PolicyDefinitions from the cluster
// This should be called at controller startup
func (idx *PolicyScopeIndex) Initialize(ctx context.Context, cli client.Client) error {
	klog.InfoS("Initializing PolicyScopeIndex")

	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.client = cli

	if err := idx.initializeLocked(ctx, cli); err != nil {
		return err
	}

	idx.initialized = true
	return nil
}

// initializeLocked performs the actual initialization (caller must hold write lock)
func (idx *PolicyScopeIndex) initializeLocked(ctx context.Context, cli client.Client) error {
	// List all PolicyDefinitions across all namespaces
	policyList := &v1beta1.PolicyDefinitionList{}
	if err := cli.List(ctx, policyList); err != nil {
		klog.ErrorS(err, "Failed to list PolicyDefinitions for index initialization")
		return err
	}

	idx.byNamespace = make(map[string]map[string]*PolicyMetadata)
	idx.globalApplicationPolicies = make(map[string][]*PolicyMetadata)

	for i := range policyList.Items {
		policy := &policyList.Items[i]
		idx.addPolicyLocked(policy)
	}

	klog.InfoS("PolicyScopeIndex initialized",
		"totalPolicies", len(policyList.Items),
		"namespaces", len(idx.byNamespace))

	return nil
}

// Get retrieves metadata for a policy, searching in the specified namespace and vela-system
// Returns nil if not found
func (idx *PolicyScopeIndex) Get(policyName, appNamespace string) *PolicyMetadata {
	idx.ensureInitialized(context.Background())

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Try app namespace first
	if nsMap, exists := idx.byNamespace[appNamespace]; exists {
		if metadata, found := nsMap[policyName]; found {
			return metadata
		}
	}

	// Fall back to vela-system
	if appNamespace != oam.SystemDefinitionNamespace {
		if nsMap, exists := idx.byNamespace[oam.SystemDefinitionNamespace]; exists {
			if metadata, found := nsMap[policyName]; found {
				return metadata
			}
		}
	}

	return nil
}

// GetFromNamespace retrieves metadata for a policy in a specific namespace
// Returns nil if not found
func (idx *PolicyScopeIndex) GetFromNamespace(policyName, namespace string) *PolicyMetadata {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if nsMap, exists := idx.byNamespace[namespace]; exists {
		return nsMap[policyName]
	}
	return nil
}

// GetGlobalApplicationPolicies returns pre-filtered global Application-scoped policies
// This eliminates the need for List operations and in-memory filtering
// Returns policies sorted by Priority (asc) then Name (asc): lower priority value runs first
func (idx *PolicyScopeIndex) GetGlobalApplicationPolicies(namespace string) []*PolicyMetadata {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	policies, exists := idx.globalApplicationPolicies[namespace]
	if !exists {
		return nil
	}

	// Return a copy to prevent external modification
	result := make([]*PolicyMetadata, len(policies))
	copy(result, policies)
	return result
}

// GetGlobalApplicationPoliciesDeduped returns deduplicated global policies
// Namespace policies take precedence over vela-system policies
func (idx *PolicyScopeIndex) GetGlobalApplicationPoliciesDeduped(appNamespace string) []*PolicyMetadata {
	idx.ensureInitialized(context.Background())

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var result []*PolicyMetadata
	seenNames := make(map[string]bool)

	// Add namespace policies first (higher precedence)
	if appNamespace != oam.SystemDefinitionNamespace {
		if nsPolicies, exists := idx.globalApplicationPolicies[appNamespace]; exists {
			for _, policy := range nsPolicies {
				result = append(result, policy)
				seenNames[policy.Name] = true
			}
		}
	}

	// Add vela-system policies (skip if already seen)
	if systemPolicies, exists := idx.globalApplicationPolicies[oam.SystemDefinitionNamespace]; exists {
		for _, policy := range systemPolicies {
			if !seenNames[policy.Name] {
				result = append(result, policy)
			}
		}
	}

	return result
}

// AddOrUpdate adds or updates a policy in the index
// This should be called from watch event handlers
func (idx *PolicyScopeIndex) AddOrUpdate(policy *v1beta1.PolicyDefinition) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.addPolicyLocked(policy)

	klog.V(4).InfoS("PolicyScopeIndex updated",
		"policy", policy.Name,
		"namespace", policy.Namespace,
		"scope", policy.Spec.Scope,
		"global", policy.Spec.Global)
}

// Delete removes a policy from the index
// This should be called from watch event handlers
func (idx *PolicyScopeIndex) Delete(policyName, namespace string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Remove from namespace map
	if nsMap, exists := idx.byNamespace[namespace]; exists {
		delete(nsMap, policyName)
		if len(nsMap) == 0 {
			delete(idx.byNamespace, namespace)
		}
	}

	// Rebuild global policies list for this namespace
	idx.rebuildGlobalPoliciesForNamespaceLocked(namespace)

	klog.V(4).InfoS("PolicyScopeIndex deleted",
		"policy", policyName,
		"namespace", namespace)
}

// InvalidateNamespace invalidates all policies in a namespace and rebuilds the index
// This can be used as a fallback if individual updates miss something
func (idx *PolicyScopeIndex) InvalidateNamespace(namespace string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	delete(idx.byNamespace, namespace)
	delete(idx.globalApplicationPolicies, namespace)

	klog.V(4).InfoS("PolicyScopeIndex namespace invalidated", "namespace", namespace)
}

// Size returns the total number of indexed policies
func (idx *PolicyScopeIndex) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	count := 0
	for _, nsMap := range idx.byNamespace {
		count += len(nsMap)
	}
	return count
}

// addPolicyLocked adds a policy to the index (caller must hold write lock)
func (idx *PolicyScopeIndex) addPolicyLocked(policy *v1beta1.PolicyDefinition) {
	namespace := policy.Namespace
	if namespace == "" {
		// For cluster-scoped policies (legacy), treat as vela-system
		namespace = oam.SystemDefinitionNamespace
	}

	// Ensure namespace map exists
	if _, exists := idx.byNamespace[namespace]; !exists {
		idx.byNamespace[namespace] = make(map[string]*PolicyMetadata)
	}

	// Add/update metadata
	metadata := &PolicyMetadata{
		Name:            policy.Name,
		Namespace:       namespace,
		Scope:           policy.Spec.Scope,
		Global:          policy.Spec.Global,
		Priority:        policy.Spec.Priority,
		ResourceVersion: policy.ResourceVersion,
	}
	idx.byNamespace[namespace][policy.Name] = metadata

	// Rebuild global policies list for this namespace
	idx.rebuildGlobalPoliciesForNamespaceLocked(namespace)
}

// rebuildGlobalPoliciesForNamespaceLocked rebuilds the sorted global Application-scoped policy list
// for a namespace (caller must hold write lock)
func (idx *PolicyScopeIndex) rebuildGlobalPoliciesForNamespaceLocked(namespace string) {
	nsMap, exists := idx.byNamespace[namespace]
	if !exists {
		delete(idx.globalApplicationPolicies, namespace)
		return
	}

	// Filter for Global=true and Scope=Application
	var globalPolicies []*PolicyMetadata
	for _, metadata := range nsMap {
		if metadata.Global && metadata.Scope == v1beta1.ApplicationScope {
			globalPolicies = append(globalPolicies, metadata)
		}
	}

	// Sort by Priority (lower value first, matching Kubernetes admission webhook convention),
	// then by Name (alphabetical) for stable ordering within the same priority.
	sort.Slice(globalPolicies, func(i, j int) bool {
		if globalPolicies[i].Priority != globalPolicies[j].Priority {
			return globalPolicies[i].Priority < globalPolicies[j].Priority // Lower value runs first
		}
		return globalPolicies[i].Name < globalPolicies[j].Name // Alphabetical for same priority
	})

	if len(globalPolicies) > 0 {
		idx.globalApplicationPolicies[namespace] = globalPolicies
	} else {
		delete(idx.globalApplicationPolicies, namespace)
	}
}
