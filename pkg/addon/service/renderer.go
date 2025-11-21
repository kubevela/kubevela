/*
Copyright 2025 The KubeVela Authors.

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

package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/kubevela/pkg/util/singleton"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/addon"
	addonprovider "github.com/oam-dev/kubevela/pkg/cue/cuex/providers/addon"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils"
)

// cacheEntry holds a cached result with expiry time
type cacheEntry struct {
	result    *addonprovider.AddonResult
	expiresAt time.Time
}

// AddonRendererImpl implements the AddonRenderer interface
type AddonRendererImpl struct {
	cache      map[string]*cacheEntry
	cacheMutex sync.RWMutex
	cacheTTL   time.Duration
}

// NewAddonRenderer creates a new addon renderer implementation
func NewAddonRenderer() addonprovider.AddonRenderer {
	return &AddonRendererImpl{
		cache:    make(map[string]*cacheEntry),
		cacheTTL: 5 * time.Minute,
	}
}

// generateCacheKey creates a cache key based on request parameters
func generateCacheKey(req *addonprovider.AddonRequest) string {
	// Create a deterministic key from all parameters that affect the result
	key := struct {
		Addon      string
		Version    string
		Registry   string
		Properties map[string]interface{}
		Include    *addonprovider.IncludeOptions
	}{
		Addon:      req.Addon,
		Version:    req.Version,
		Registry:   req.Registry,
		Properties: req.Properties,
		Include:    req.Include,
	}
	
	// Serialize to JSON for hashing
	data, _ := json.Marshal(key)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

// RenderAddon implements the AddonRenderer interface
func (r *AddonRendererImpl) RenderAddon(ctx context.Context, req *addonprovider.AddonRequest) (*addonprovider.AddonResult, error) {
	start := time.Now()
	
	// Log performance metrics at the end
	defer func() {
		duration := time.Since(start)
		klog.Infof("Addon service render took %v for addon: %s", duration, req.Addon)
	}()
	
	// Generate cache key
	cacheKey := generateCacheKey(req)
	
	// Check cache
	r.cacheMutex.RLock()
	if entry, exists := r.cache[cacheKey]; exists {
		if time.Now().Before(entry.expiresAt) {
			r.cacheMutex.RUnlock()
			klog.V(2).Infof("Cache hit for addon %s, key %s", req.Addon, cacheKey[:8])
			return entry.result, nil
		}
	}
	r.cacheMutex.RUnlock()
	
	klog.V(2).Infof("Cache miss for addon %s, key %s", req.Addon, cacheKey[:8])
	
	// Get singleton k8s client
	k8sClient := singleton.KubeClient.Get()

	// Get registries from datastore
	registryStart := time.Now()
	registryDS := addon.NewRegistryDataStore(k8sClient)
	registries, err := registryDS.ListRegistries(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list registries")
	}
	klog.V(4).Infof("Registry list took %v", time.Since(registryStart))

	// Parse registry name from addon parameter
	registryName, addonName, err := splitSpecifyRegistry(req.Addon)
	if err != nil {
		return nil, err
	}

	// If registry specified in params, use that instead
	if req.Registry != "" {
		registryName = req.Registry
	}

	// Find addon in registries
	findStart := time.Now()
	installPkg, resolvedVersion, selectedRegistry, err := findAndLoadAddon(ctx, addonName, req.Version, registryName, registries)
	if err != nil {
		return nil, err
	}
	klog.V(4).Infof("Find and load addon took %v", time.Since(findStart))

	// Render the addon using existing infrastructure
	renderStart := time.Now()
	app, resources, err := addon.RenderApp(ctx, installPkg, nil, req.Properties)
	if err != nil {
		return nil, errors.Wrap(err, "failed to render addon")
	}
	klog.V(4).Infof("RenderApp took %v", time.Since(renderStart))

	// Start with base resources or empty slice based on include options
	var allResources []*unstructured.Unstructured
	
	// Determine what to include (default to true if include is nil)
	includeDefinitions := req.Include == nil || req.Include.Definitions
	includeConfigTemplates := req.Include == nil || req.Include.ConfigTemplates
	includeViews := req.Include == nil || req.Include.Views
	includeResources := req.Include == nil || req.Include.Resources

	// Add base resources if included
	if includeResources {
		allResources = append(allResources, resources...)
	}

	// Render addon definitions (ComponentDefinitions, TraitDefinitions, etc.) if included
	if includeDefinitions {
		k8sConfig := ctrl.GetConfigOrDie()
		definitions, err := addon.RenderDefinitions(installPkg, k8sConfig)
		if err != nil {
			return nil, errors.Wrap(err, "failed to render addon definitions")
		}
		allResources = append(allResources, definitions...)
	}

	// Render config templates if included
	if includeConfigTemplates {
		configTemplates, err := addon.RenderConfigTemplates(ctx, installPkg, k8sClient)
		if err != nil {
			return nil, errors.Wrap(err, "failed to render addon config templates")
		}
		allResources = append(allResources, configTemplates...)
	}

	// Render views (VelaQL views) if included
	if includeViews {
		views, err := addon.RenderViews(ctx, installPkg)
		if err != nil {
			return nil, errors.Wrap(err, "failed to render addon views")
		}
		allResources = append(allResources, views...)
	}

	// Extract registry metadata
	actualRegistryName := selectedRegistry.Name
	registryType, registryURL := getRegistryTypeAndURL(selectedRegistry)

	// Add addon labels (for selection/identification)
	if app.Labels == nil {
		app.Labels = make(map[string]string)
	}
	app.Labels["addons.oam.dev/name"] = addonName
	
	// Add addon metadata as annotations (descriptive metadata)
	if app.Annotations == nil {
		app.Annotations = make(map[string]string)
	}
	app.Annotations["addons.oam.dev/version"] = resolvedVersion
	app.Annotations["addons.oam.dev/registry"] = actualRegistryName
	app.Annotations["addons.oam.dev/registry-type"] = registryType
	if registryURL != "" {
		app.Annotations["addons.oam.dev/registry-url"] = registryURL
	}

	// Convert Application to map[string]interface{}
	appMap, err := applicationToMap(app)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert application to map")
	}

	// Add addon labels to all auxiliary resources and convert to maps
	resourceMaps := make([]map[string]interface{}, len(allResources))
	for i, resource := range allResources {
		// Add addon labels (for selection/identification)
		if resource.GetLabels() == nil {
			resource.SetLabels(make(map[string]string))
		}
		labels := resource.GetLabels()
		labels["addons.oam.dev/name"] = addonName
		resource.SetLabels(labels)
		
		// Add addon metadata as annotations (descriptive metadata)
		if resource.GetAnnotations() == nil {
			resource.SetAnnotations(make(map[string]string))
		}
		annotations := resource.GetAnnotations()
		annotations["addons.oam.dev/version"] = resolvedVersion
		annotations["addons.oam.dev/registry"] = actualRegistryName
		annotations["addons.oam.dev/registry-type"] = registryType
		if registryURL != "" {
			annotations["addons.oam.dev/registry-url"] = registryURL
		}
		resource.SetAnnotations(annotations)

		resourceMap, err := unstructuredToMap(resource)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert resource %d to map", i)
		}
		resourceMaps[i] = resourceMap
	}

	result := &addonprovider.AddonResult{
		ResolvedVersion: resolvedVersion,
		Registry:        actualRegistryName,
		Application:     appMap,
		Resources:       resourceMaps,
	}
	
	// Store in cache
	r.cacheMutex.Lock()
	r.cache[cacheKey] = &cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(r.cacheTTL),
	}
	r.cacheMutex.Unlock()
	
	klog.V(2).Infof("Cached result for addon %s, key %s, expires at %v", req.Addon, cacheKey[:8], time.Now().Add(r.cacheTTL).Format(time.RFC3339))
	
	return result, nil
}

// splitSpecifyRegistry parses registry/addon format
func splitSpecifyRegistry(name string) (string, string, error) {
	res := strings.Split(name, "/")
	switch len(res) {
	case 2:
		return res[0], res[1], nil
	case 1:
		return "", res[0], nil
	default:
		return "", "", fmt.Errorf("invalid addon name, you should specify name only <addonName> or with registry as prefix <registryName>/<addonName>")
	}
}

// findAndLoadAddon finds and loads an addon from available registries
func findAndLoadAddon(ctx context.Context, addonName, version, registryName string, registries []addon.Registry) (*addon.InstallPackage, string, *addon.Registry, error) {
	for _, registry := range registries {
		// Skip if specific registry requested and this isn't it
		if registryName != "" && registryName != registry.Name {
			continue
		}

		// Handle versioned registries
		if addon.IsVersionRegistry(registry) {
			vr, err := addon.ToVersionedRegistry(registry)
			if err != nil {
				continue
			}

			// Get available versions
			versions, err := vr.GetAddonAvailableVersion(addonName)
			if err != nil {
				continue
			}

			// Choose version based on version string (can be exact or constraint)
			targetVersion, err := resolveVersion(version, versions)
			if err != nil {
				continue
			}

			// Load install package
			installPkg, err := vr.GetAddonInstallPackage(ctx, addonName, targetVersion)
			if err != nil {
				continue
			}

			return installPkg, targetVersion, &registry, nil
		} else {
			// Handle non-versioned registries
			metas, err := registry.ListAddonMeta()
			if err != nil {
				continue
			}

			meta, ok := metas[addonName]
			if !ok {
				continue
			}

			uiData, err := registry.GetUIData(&meta, addon.UIMetaOptions)
			if err != nil {
				continue
			}

			installPkg, err := registry.GetInstallPackage(&meta, uiData)
			if err != nil {
				continue
			}

			return installPkg, uiData.Meta.Version, &registry, nil
		}
	}

	return nil, "", nil, fmt.Errorf("addon %s not found in any registry", addonName)
}

// isVersionConstraint checks if a version string contains constraint operators
func isVersionConstraint(version string) bool {
	// Check for common semver constraint operators
	return strings.ContainsAny(version, "><~^=|")
}

// resolveVersion chooses the appropriate version based on version string
// The version string can be:
// - Empty: returns latest stable version
// - Exact version: "1.2.3" or "v1.2.3"
// - Constraint: ">=1.0.0", "~1.2.0", "^1.0.0", etc.
func resolveVersion(version string, versions []*repo.ChartVersion) (string, error) {
	// If version specified
	if version != "" {
		// Check if it's a constraint
		if isVersionConstraint(version) {
			constraint, err := semver.NewConstraint(version)
			if err != nil {
				return "", errors.Wrap(err, "invalid version constraint")
			}

			// Find the latest version that satisfies the constraint
			for _, v := range versions {
				semVersion, err := semver.NewVersion(v.Version)
				if err != nil {
					continue
				}
				if constraint.Check(semVersion) {
					return v.Version, nil
				}
			}
			return "", fmt.Errorf("no version found matching constraint %s", version)
		} else {
			// Treat as exact version
			for _, v := range versions {
				if utils.IgnoreVPrefix(v.Version) == utils.IgnoreVPrefix(version) {
					return v.Version, nil
				}
			}
			return "", fmt.Errorf("version %s not found", version)
		}
	}

	// Otherwise, find latest stable version (no prerelease)
	for _, v := range versions {
		version, err := semver.NewVersion(v.Version)
		if err != nil {
			continue
		}
		if len(version.Prerelease()) == 0 {
			return v.Version, nil
		}
	}

	// If no stable version, use the first one
	if len(versions) > 0 {
		return versions[0].Version, nil
	}

	return "", fmt.Errorf("no versions available")
}

// unstructuredToMap converts an unstructured object to map[string]interface{}
func unstructuredToMap(obj *unstructured.Unstructured) (map[string]interface{}, error) {
	if obj == nil {
		return nil, nil
	}
	// Clean up undefined/null fields that could cause CUE issues
	cleanMap := cleanUndefinedFields(obj.Object)
	return cleanMap, nil
}

// applicationToMap converts an Application to map[string]interface{}
func applicationToMap(app *v1beta1.Application) (map[string]interface{}, error) {
	if app == nil {
		return nil, nil
	}
	unstructuredApp, err := util.Object2Unstructured(app)
	if err != nil {
		return nil, err
	}

	// Clean up undefined/null fields that could cause CUE issues
	cleanMap := cleanUndefinedFields(unstructuredApp.Object)
	return cleanMap, nil
}

// cleanUndefinedFields recursively removes undefined, null, or problematic fields
func cleanUndefinedFields(obj map[string]interface{}) map[string]interface{} {
	cleaned := make(map[string]interface{})

	for key, value := range obj {
		if value == nil {
			continue // Skip nil values
		}

		switch v := value.(type) {
		case map[string]interface{}:
			cleanedNested := cleanUndefinedFields(v)
			if len(cleanedNested) > 0 {
				cleaned[key] = cleanedNested
			}
		case []interface{}:
			var cleanedArray []interface{}
			for _, item := range v {
				if item != nil {
					if nestedMap, ok := item.(map[string]interface{}); ok {
						cleanedItem := cleanUndefinedFields(nestedMap)
						if len(cleanedItem) > 0 {
							cleanedArray = append(cleanedArray, cleanedItem)
						}
					} else {
						cleanedArray = append(cleanedArray, item)
					}
				}
			}
			if len(cleanedArray) > 0 {
				cleaned[key] = cleanedArray
			}
		default:
			// Include non-nil scalar values
			cleaned[key] = value
		}
	}

	return cleaned
}

// getRegistryTypeAndURL extracts the registry type and URL from a registry object
func getRegistryTypeAndURL(registry *addon.Registry) (string, string) {
	if registry.Helm != nil {
		return "helm", registry.Helm.URL
	}
	if registry.Git != nil {
		return "git", registry.Git.URL
	}
	if registry.OSS != nil {
		return "oss", "" // OSS doesn't have a simple URL field
	}
	if registry.Gitee != nil {
		return "gitee", registry.Gitee.URL
	}
	if registry.Gitlab != nil {
		return "gitlab", registry.Gitlab.URL
	}
	return "unknown", ""
}
