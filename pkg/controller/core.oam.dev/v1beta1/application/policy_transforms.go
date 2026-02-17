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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/kubevela/pkg/cue/cuex"
	monitorContext "github.com/kubevela/pkg/monitor/context"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/config"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"
)

const (
	// SkipGlobalPoliciesAnnotation allows Applications to opt-out of global policies
	SkipGlobalPoliciesAnnotation = "policy.oam.dev/skip-global"
)

// ApplyApplicationScopeTransforms iterates through policies in the Application spec
// and applies transforms from any Application-scoped PolicyDefinitions.
//
// Two-level caching strategy:
//  1. In-memory global cache (globalPolicyCache) - Caches rendered policy results for rapid
//     reconciliations. Invalidated when Application or global policy set changes.
//  2. ConfigMap persistent cache - Stores individual policy results with TTL control:
//     - TTL=-1: Never refresh (deterministic policies)
//     - TTL=0: Never cache (policies with external dependencies)
//     - TTL>0: Refresh after N seconds
//
// It first discovers and applies any global policies (if feature gate enabled),
// then applies explicit policies from the Application spec.
// This modifies the in-memory Application object before it's parsed into an AppFile.
// Returns the updated context with any additionalContext from policies.
func (h *AppHandler) ApplyApplicationScopeTransforms(ctx monitorContext.Context, app *v1beta1.Application) (monitorContext.Context, error) {
	// Clear previous global policy status
	app.Status.AppliedApplicationPolicies = nil

	// CRITICAL: Compute Application hash BEFORE any policies run
	// This hash is based on the original spec (not modified by policies yet)
	// It's used for cache invalidation - only spec changes should invalidate cache
	appHash, err := computeApplicationHash(app)
	if err != nil {
		ctx.Info("Failed to compute Application hash", "error", err)
		appHash = "" // Continue without hash
	}

	// Step 1: Validate explicit policies are not global
	for _, policy := range app.Spec.Policies {
		if err := validateNotGlobalPolicy(ctx, h.Client, policy.Type, app.Namespace); err != nil {
			return ctx, errors.Wrapf(err, "invalid policy reference: %s", policy.Type)
		}
	}

	// Step 2: Handle global policies (if feature gate enabled and not opted out)
	var globalRenderedResults []RenderedPolicyResult
	allPolicyChanges := make(map[string]*PolicyChanges)         // Track full changes for ConfigMap storage
	policyMetadata := make(map[string]*policyConfigMapMetadata) // Track metadata for ConfigMap
	sequence := 1                                               // Track execution order
	// TODO: Remove cascade invalidation code (upstreamOutputs tracking)
	// For now, stub it out to keep code compiling
	var upstreamOutputs []*PolicyOutput

	if !shouldSkipGlobalPolicies(app) && utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableGlobalPolicies) {
		// Try cache first - this returns the RENDERED results, not PolicyDefinitions
		cachedResults, cacheHit, err := applicationPolicyCache.Get(app)
		if err != nil {
			ctx.Info("Cache error, will discover and render policies", "error", err)
		} else if cacheHit {
			ctx.Info("Cache HIT - using pre-rendered policy results", "count", len(cachedResults), "cacheKey", fmt.Sprintf("%s/%s", app.Namespace, app.Name))
			globalRenderedResults = cachedResults
		} else {
			// Cache miss - discover, render, and cache
			ctx.Info("Cache MISS - discovering and rendering global policies")

			var globalPolicies []v1beta1.PolicyDefinition
			var velaSystemPolicies, namespacePolicies []v1beta1.PolicyDefinition

			// Discover from vela-system
			velaSystemPolicies, err = discoverGlobalPolicies(ctx, h.Client, oam.SystemDefinitionNamespace)
			if err != nil {
				ctx.Info("Failed to discover vela-system global policies", "error", err)
			}

			// Discover from application namespace (if different)
			if app.Namespace != oam.SystemDefinitionNamespace {
				namespacePolicies, err = discoverGlobalPolicies(ctx, h.Client, app.Namespace)
				if err != nil {
					ctx.Info("Failed to discover namespace global policies", "error", err)
				}
			}

			// Deduplicate: namespace policies win over vela-system policies
			namespacePolicyNames := make(map[string]bool)
			for _, policy := range namespacePolicies {
				namespacePolicyNames[policy.Name] = true
			}

			// Combine: namespace policies first, then vela-system (deduped)
			globalPolicies = append(globalPolicies, namespacePolicies...)
			for _, policy := range velaSystemPolicies {
				if !namespacePolicyNames[policy.Name] {
					globalPolicies = append(globalPolicies, policy)
				}
			}

			// Now RENDER each global policy (expensive operation we want to cache)
			for _, policy := range globalPolicies {
				ctx.Info("Rendering global policy", "policy", policy.Name, "namespace", policy.Namespace)

				policyRef := v1beta1.AppPolicy{
					Name: policy.Name,
					Type: policy.Name,
					// Global policies don't have parameters from Application spec
				}

				result, err := h.renderPolicy(ctx, app, policyRef, &policy)
				if err != nil {
					ctx.Info("Failed to render global policy, skipping", "policy", policy.Name, "error", err)
					// Store failed result for observability
					result.PolicyName = policy.Name
					result.PolicyNamespace = policy.Namespace
					result.Enabled = false
					result.SkipReason = fmt.Sprintf("render error: %s", err.Error())
				}

				globalRenderedResults = append(globalRenderedResults, result)
			}

			// Cache the rendered results for next time
			if err := applicationPolicyCache.Set(app, globalRenderedResults); err != nil {
				ctx.Info("Failed to update cache", "error", err)
			} else {
				ctx.Info("Cached rendered global policy results", "count", len(globalRenderedResults))
			}
		}

		// Apply the rendered global policy results (either from cache or freshly rendered)
		for _, result := range globalRenderedResults {
			// Check feature gate for Application-scoped policies
			if !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableApplicationScopedPolicies) {
				ctx.Info("Skipping Application-scoped global policy (feature gate disabled)",
					"policy", result.PolicyName,
					"namespace", result.PolicyNamespace,
					"featureGate", "EnableApplicationScopedPolicies")
				continue
			}

			ctx.Info("Applying global policy result", "policy", result.PolicyName, "enabled", result.Enabled, "fromCache", cacheHit, "sequence", sequence)

			// Get priority from the result (stored during render)
			priority := result.Priority

			var policyChanges *PolicyChanges
			ctx, policyChanges, err = h.applyRenderedPolicyResult(ctx, app, result, sequence, priority)
			if err != nil {
				return ctx, errors.Wrapf(err, "failed to apply global policy %s", result.PolicyName)
			}

			// Store changes and metadata for ConfigMap storage
			if policyChanges != nil {
				allPolicyChanges[result.PolicyName] = policyChanges
			}
			if result.Enabled {
				// Compute cascade ID for this policy (hash of all upstream outputs)
				cascadeID, err := computeCascadeID(upstreamOutputs)
				if err != nil {
					ctx.Info("Failed to compute cascade ID", "policy", result.PolicyName, "error", err)
					cascadeID = ""
				}

				policyMetadata[result.PolicyName] = &policyConfigMapMetadata{
					Name:      result.PolicyName,
					Namespace: result.PolicyNamespace,
					Source:    "global",
					Sequence:  sequence,
					Priority:  priority,
					Config:    result.Config,
					CascadeID: cascadeID,
				}
				sequence++ // Increment sequence only if policy was applied (enabled=true)

				// Track this policy's output for cascade invalidation of downstream policies
				if result.Transforms != nil {
					if policyOutput, ok := result.Transforms.(*PolicyOutput); ok {
						upstreamOutputs = append(upstreamOutputs, policyOutput)
					}
				}
			}
		}
	} else if shouldSkipGlobalPolicies(app) {
		ctx.Info("Skipping global policies (opt-out annotation present)")
	} else {
		ctx.Info("Global policies feature is disabled (feature gate not enabled)")
	}

	// Step 3: Apply explicit policies from Application spec
	// Use same render+apply flow as global policies for caching and cascade tracking
	for _, policy := range app.Spec.Policies {
		// Load PolicyDefinition template
		templ, err := appfile.LoadTemplate(ctx, h.Client, policy.Type, types.TypePolicy, app.Annotations)
		if err != nil {
			ctx.Info("Failed to load PolicyDefinition, skipping", "policy", policy.Type, "error", err)
			continue
		}

		// Check if Application-scoped
		if templ.PolicyDefinition == nil || templ.PolicyDefinition.Spec.Scope != v1beta1.ApplicationScope {
			continue
		}

		// Check feature gate for Application-scoped policies
		if !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableApplicationScopedPolicies) {
			ctx.Info("Skipping Application-scoped policy (feature gate disabled)",
				"policy", policy.Type,
				"name", policy.Name,
				"featureGate", "EnableApplicationScopedPolicies")
			continue
		}

		ctx.Info("Applying explicit Application-scoped policy", "policy", policy.Type, "name", policy.Name)

		// Render with upstream tracking (same as global policies)
		result, err := h.renderPolicyWithUpstream(ctx, app, policy, templ.PolicyDefinition, upstreamOutputs)
		if err != nil {
			ctx.Info("Failed to render explicit policy, skipping", "policy", policy.Name, "error", err)
			result.PolicyName = policy.Name
			result.PolicyNamespace = templ.PolicyDefinition.Namespace
			result.Enabled = false
			result.SkipReason = fmt.Sprintf("render error: %s", err.Error())
		}

		// Apply the rendered result
		priority := int32(0) // Explicit policies don't have priority
		var policyChanges *PolicyChanges
		ctx, policyChanges, err = h.applyRenderedPolicyResult(ctx, app, result, sequence, priority)
		if err != nil {
			return ctx, errors.Wrapf(err, "failed to apply explicit policy %s", policy.Name)
		}

		// Store changes and metadata for ConfigMap storage
		if policyChanges != nil {
			allPolicyChanges[policy.Name] = policyChanges
		}
		if result.Enabled {
			// Compute cascade ID for this policy (hash of all upstream outputs)
			cascadeID, err := computeCascadeID(upstreamOutputs)
			if err != nil {
				ctx.Info("Failed to compute cascade ID", "policy", policy.Name, "error", err)
				cascadeID = ""
			}

			policyMetadata[policy.Name] = &policyConfigMapMetadata{
				Name:      policy.Name,
				Namespace: templ.PolicyDefinition.Namespace,
				Source:    "explicit",
				Sequence:  sequence,
				Priority:  priority,
				Config:    result.Config,
				CascadeID: cascadeID,
			}
			sequence++ // Increment sequence only if policy was applied (enabled=true)

			// Track this policy's output for cascade invalidation of downstream policies
			if result.Transforms != nil {
				if policyOutput, ok := result.Transforms.(*PolicyOutput); ok {
					upstreamOutputs = append(upstreamOutputs, policyOutput)
				}
			}
		}
	}

	// Step 4: Store all rendered policy outputs in ConfigMap for reuse and observability
	// This creates a persistent cache with TTL that can be used to avoid re-rendering
	orderedData := make(map[string]string)

	// Note: appHash was computed at the very beginning (before policies ran)
	// This ensures we're hashing the original Application spec, not the modified version

	// Build ConfigMap data from metadata and changes tracked during reconciliation
	for policyName, metadata := range policyMetadata {
		// Get the full policy changes if available (includes output, labels, annotations, context)
		policyChanges, hasPolicyChanges := allPolicyChanges[policyName]
		if !hasPolicyChanges || policyChanges == nil || policyChanges.Output == nil {
			// No output to cache - skip
			continue
		}

		// Use new per-output-type cache record format if config is available
		var policyJSON []byte
		var err error

		if metadata.Config != nil {
			// New format: per-output-type caching with refresh control
			cacheRecord := createPolicyCacheRecord(
				metadata.Name,
				metadata.Namespace,
				int(metadata.Priority),
				metadata.Sequence,
				appHash,
				metadata.CascadeID,
				policyChanges.Output,
				metadata.Config,
			)

			policyJSON, err = json.MarshalIndent(cacheRecord, "", "  ")
		} else {
			// Old format: monolithic cache (for backwards compatibility)
			policyRecord := map[string]interface{}{
				"policy":           metadata.Name,
				"namespace":        metadata.Namespace,
				"source":           metadata.Source,
				"sequence":         metadata.Sequence,
				"priority":         metadata.Priority,
				"rendered_at":      time.Now().Format(time.RFC3339),
				"ttl_seconds":      int32(-1),
				"enabled":          true,
				"application_hash": appHash,
			}

			// Store the output object in reusable format
			outputData := serializeOutputForStorage(policyChanges.Output)
			if len(outputData) > 0 {
				policyRecord["output"] = outputData
			}

			// Add additional context if available
			if policyChanges.AdditionalContext != nil && len(policyChanges.AdditionalContext) > 0 {
				policyRecord["additional_context"] = policyChanges.AdditionalContext
			}

			// Add observability summary
			policyRecord["summary"] = map[string]interface{}{
				"labels_added":      len(policyChanges.AddedLabels),
				"annotations_added": len(policyChanges.AddedAnnotations),
				"spec_modified":     policyChanges.SpecModified,
				"has_context":       len(policyChanges.AdditionalContext) > 0,
			}

			policyJSON, err = json.MarshalIndent(policyRecord, "", "  ")
		}

		if err != nil {
			ctx.Info("Failed to marshal policy record", "policy", metadata.Name, "error", err)
			continue
		}

		key := fmt.Sprintf("%03d-%s", metadata.Sequence, metadata.Name)
		orderedData[key] = string(policyJSON)
	}

	// Create/update ConfigMap if any policies were applied
	if len(orderedData) > 0 {
		err := createOrUpdateDiffsConfigMap(ctx, h.Client, app, orderedData)
		if err != nil {
			ctx.Info("Failed to store policy records in ConfigMap", "error", err)
			// Don't fail reconciliation - observability/caching is optional
		} else {
			app.Status.ApplicationPoliciesConfigMap = fmt.Sprintf("application-policies-%s-%s", app.Namespace, app.Name)
			ctx.Info("Stored policy records in ConfigMap", "configmap", app.Status.ApplicationPoliciesConfigMap, "policies", len(orderedData))
		}
	}

	return ctx, nil
}

// applyPolicyTransform renders the policy's CUE template and applies transforms to the Application.
// Returns the updated context with any additionalContext merged in.
func (h *AppHandler) applyPolicyTransform(ctx monitorContext.Context, app *v1beta1.Application, policyRef v1beta1.AppPolicy, policyDef *v1beta1.PolicyDefinition) (monitorContext.Context, *PolicyChanges, error) {
	// Validate policy has CUE schematic
	if policyDef.Spec.Schematic == nil || policyDef.Spec.Schematic.CUE == nil {
		return ctx, nil, errors.Errorf("Application-scoped policy %s must have a CUE schematic", policyDef.Name)
	}

	// Parse policy parameters
	var policyParams map[string]interface{}
	if policyRef.Properties != nil && len(policyRef.Properties.Raw) > 0 {
		if err := json.Unmarshal(policyRef.Properties.Raw, &policyParams); err != nil {
			return ctx, nil, errors.Wrap(err, "failed to unmarshal policy parameters")
		}
	}

	// Load prior cached result (if any) to pass as context.prior
	// This allows the policy template to access previous rendered values
	var priorResult map[string]interface{}
	if app.Status.ApplicationPoliciesConfigMap != "" {
		priorResult, _ = loadCachedPolicyFromConfigMap(ctx, h.Client, app, policyDef.Name, -1) // Always load regardless of TTL
	}

	// Render the CUE template with context.application and context.prior
	rendered, err := h.renderPolicyCUETemplate(ctx, app, policyParams, policyDef, priorResult)
	if err != nil {
		return ctx, nil, errors.Wrap(err, "failed to render CUE template")
	}

	// Check if the transform should be applied (default: true)
	shouldApply, err := h.extractEnabled(rendered)
	if err != nil {
		return ctx, nil, errors.Wrap(err, "failed to extract enabled")
	}

	if !shouldApply {
		ctx.Info("Skipping transform (enabled=false)", "policy", policyRef.Type)
		// Record in status if this is a global policy
		// Note: This should not happen for explicit policies as we validate earlier
		// that global policies cannot be explicitly referenced
		if policyDef.Spec.Global {
			recordApplicationPolicyStatus(app, policyRef.Name, policyDef.Namespace, "global", 0, policyDef.Spec.Priority, false, "enabled=false", nil)
		}
		return ctx, nil, nil
	}

	// Extract output (new API only)
	output, err := h.extractOutput(rendered)
	if err != nil {
		return ctx, nil, errors.Wrap(err, "failed to extract output")
	}

	// Require output field
	if output == nil {
		return ctx, nil, errors.New("policy must specify 'output' field - see documentation for API reference")
	}

	// Track changes from before transform application
	changes := &PolicyChanges{
		Enabled: true, // We already checked it's enabled above
		Output:  output,
	}

	// Take snapshot of labels and annotations BEFORE applying transform
	labelsBefore := make(map[string]string)
	if app.Labels != nil {
		for k, v := range app.Labels {
			labelsBefore[k] = v
		}
	}
	annotationsBefore := make(map[string]string)
	if app.Annotations != nil {
		for k, v := range app.Annotations {
			annotationsBefore[k] = v
		}
	}

	// Apply output to Application spec
	// Build new spec struct (like old code did) to survive status patch operations
	newSpec := app.Spec.DeepCopy()

	if output.Components != nil {
		newSpec.Components = output.Components
		ctx.Info("Replaced components from output", "policy", policyRef.Type, "count", len(output.Components))
		changes.SpecModified = true
	}
	if output.Workflow != nil {
		newSpec.Workflow = output.Workflow
		ctx.Info("Replaced workflow from output", "policy", policyRef.Type)
		changes.SpecModified = true
	}
	if output.Policies != nil {
		newSpec.Policies = output.Policies
		ctx.Info("Replaced policies from output", "policy", policyRef.Type, "count", len(output.Policies))
		changes.SpecModified = true
	}

	// Replace entire spec (matches old transforms behavior)
	app.Spec = *newSpec

	// Apply labels (always merge)
	if output.Labels != nil && len(output.Labels) > 0 {
		if app.Labels == nil {
			app.Labels = make(map[string]string)
		}
		for k, v := range output.Labels {
			app.Labels[k] = v
		}
		ctx.Info("Merged labels from output", "policy", policyRef.Type, "count", len(output.Labels))
	}

	// Apply annotations (always merge)
	if output.Annotations != nil && len(output.Annotations) > 0 {
		if app.Annotations == nil {
			app.Annotations = make(map[string]string)
		}
		for k, v := range output.Annotations {
			app.Annotations[k] = v
		}
		ctx.Info("Merged annotations from output", "policy", policyRef.Type, "count", len(output.Annotations))
	}

	// Compare AFTER to capture actual changes (works regardless of how CUE modifies the Application)
	labelsAdded := make(map[string]string)
	if app.Labels != nil {
		for k, v := range app.Labels {
			if labelsBefore[k] != v {
				labelsAdded[k] = v
			}
		}
	}
	if len(labelsAdded) > 0 {
		changes.AddedLabels = labelsAdded
	}

	annotationsAdded := make(map[string]string)
	if app.Annotations != nil {
		for k, v := range app.Annotations {
			if annotationsBefore[k] != v {
				annotationsAdded[k] = v
			}
		}
	}
	if len(annotationsAdded) > 0 {
		changes.AddedAnnotations = annotationsAdded
	}

	// Store ctx as additionalContext
	additionalContext := output.Ctx

	if additionalContext != nil {
		ctx = storeAdditionalContextInCtx(ctx, additionalContext)
		ctx.Info("Stored additionalContext in context", "policy", policyRef.Type, "keys", len(additionalContext))
		changes.AdditionalContext = additionalContext
	}

	ctx.Info("Successfully applied transform", "policy", policyRef.Type)
	return ctx, changes, nil
}

// renderPolicy renders a policy's CUE template and extracts the results for caching
// Returns a RenderedPolicyResult that can be cached and reused
func (h *AppHandler) renderPolicy(ctx monitorContext.Context, app *v1beta1.Application, policyRef v1beta1.AppPolicy, policyDef *v1beta1.PolicyDefinition) (RenderedPolicyResult, error) {
	return h.renderPolicyWithUpstream(ctx, app, policyRef, policyDef, nil)
}

// renderPolicyWithUpstream renders a policy with upstream output tracking for cascade invalidation
func (h *AppHandler) renderPolicyWithUpstream(ctx monitorContext.Context, app *v1beta1.Application, policyRef v1beta1.AppPolicy, policyDef *v1beta1.PolicyDefinition, upstreamOutputs []*PolicyOutput) (RenderedPolicyResult, error) {
	result := RenderedPolicyResult{
		PolicyName:      policyDef.Name,
		PolicyNamespace: policyDef.Namespace,
		Priority:        policyDef.Spec.Priority,
		Enabled:         false,
	}

	// Parse policy parameters first (needed for rendering)
	var policyParams map[string]interface{}
	if policyRef.Properties != nil && len(policyRef.Properties.Raw) > 0 {
		if err := json.Unmarshal(policyRef.Properties.Raw, &policyParams); err != nil {
			result.SkipReason = fmt.Sprintf("parameter unmarshal error: %s", err.Error())
			return result, errors.Wrap(err, "failed to unmarshal policy parameters")
		}
	}

	// Load prior cached result (if any) to pass as context.prior
	var priorResult map[string]interface{}
	if app.Status.ApplicationPoliciesConfigMap != "" {
		priorResult, _ = loadCachedPolicyFromConfigMap(ctx, h.Client, app, policyDef.Name, -1)
	}

	// Render the CUE template to extract config (we need config.refresh before checking cache)
	rendered, err := h.renderPolicyCUETemplate(ctx, app, policyParams, policyDef, priorResult)
	if err != nil {
		result.SkipReason = fmt.Sprintf("CUE render error: %s", err.Error())
		return result, errors.Wrap(err, "failed to render CUE template")
	}

	// Extract config (including refresh settings)
	policyConfig, err := h.extractRefreshConfig(rendered)
	if err != nil {
		result.SkipReason = fmt.Sprintf("config extraction error: %s", err.Error())
		return result, errors.Wrap(err, "failed to extract config")
	}
	if policyConfig == nil {
		// No config - use defaults
		policyConfig = applyRefreshDefaults(&PolicyConfig{Enabled: true})
	}

	// Extract enabled field
	enabled, err := h.extractEnabled(rendered)
	if err != nil {
		result.SkipReason = fmt.Sprintf("enabled extraction error: %s", err.Error())
		return result, errors.Wrap(err, "failed to extract enabled")
	}

	result.Enabled = enabled
	if !enabled {
		result.SkipReason = "enabled=false"
		return result, nil
	}

	// Try to load from new cache format with cascade and refresh checks
	cachedRecord, err := loadCachedPolicyRecord(ctx, h.Client, app, policyDef.Name, policyConfig, upstreamOutputs)
	if err != nil {
		ctx.Info("Cache load error, will render", "policy", policyDef.Name, "error", err)
	} else if cachedRecord != nil {
		ctx.Info("Using cached policy result (new format)",
			"policy", policyDef.Name,
			"hasSpec", cachedRecord.Spec != nil,
			"hasLabels", cachedRecord.Labels != nil,
			"hasAnnotations", cachedRecord.Annotations != nil,
			"hasCtx", cachedRecord.Ctx != nil)

		// Reconstruct PolicyOutput from cache
		result.Transforms = reconstructPolicyOutputFromCache(cachedRecord)
		if policyOutput, ok := result.Transforms.(*PolicyOutput); ok {
			result.AdditionalContext = policyOutput.Ctx
		}
		result.Enabled = true

		return result, nil
	}

	// Cache miss or invalid - continue with fresh render (we already have the rendered CUE value)
	// Extract output (new API only)
	output, err := h.extractOutput(rendered)
	if err != nil {
		result.SkipReason = fmt.Sprintf("output extraction error: %s", err.Error())
		return result, errors.Wrap(err, "failed to extract output")
	}

	// Require output field
	if output == nil {
		result.SkipReason = "missing output field"
		return result, errors.New("policy must specify 'output' field - see documentation for API reference")
	}

	// Store output and config
	result.Transforms = output
	result.AdditionalContext = output.Ctx
	result.Config = policyConfig

	return result, nil
}

// applyRenderedPolicyResult applies a cached/rendered policy result to the Application
// This skips all the expensive CUE rendering and just applies the pre-computed transforms
// Returns the updated context and the PolicyChanges
func (h *AppHandler) applyRenderedPolicyResult(ctx monitorContext.Context, app *v1beta1.Application, result RenderedPolicyResult, sequence int, priority int32) (monitorContext.Context, *PolicyChanges, error) {
	if !result.Enabled {
		ctx.Info("Skipping policy (from cache)", "policy", result.PolicyName, "reason", result.SkipReason)
		recordApplicationPolicyStatus(app, result.PolicyName, result.PolicyNamespace, "global", sequence, priority, false, result.SkipReason, nil)
		return ctx, nil, nil
	}

	// Extract PolicyOutput from cached result
	output, ok := result.Transforms.(*PolicyOutput)
	if !ok || output == nil {
		return ctx, nil, errors.Errorf("cached policy has invalid or missing output for policy %s", result.PolicyName)
	}

	// Track what changes we're making
	changes := &PolicyChanges{
		AdditionalContext: result.AdditionalContext,
		Enabled:           result.Enabled,
		Output:            output,
	}

	// Apply output to Application spec
	if output.Components != nil {
		app.Spec.Components = output.Components
		ctx.Info("Replaced components from output", "policy", result.PolicyName, "count", len(output.Components))
		changes.SpecModified = true
	}
	if output.Workflow != nil {
		app.Spec.Workflow = output.Workflow
		ctx.Info("Replaced workflow from output", "policy", result.PolicyName)
		changes.SpecModified = true
	}
	if output.Policies != nil {
		app.Spec.Policies = output.Policies
		ctx.Info("Replaced policies from output", "policy", result.PolicyName, "count", len(output.Policies))
		changes.SpecModified = true
	}

	// Apply labels (always merge)
	if output.Labels != nil && len(output.Labels) > 0 {
		if app.Labels == nil {
			app.Labels = make(map[string]string)
		}
		for k, v := range output.Labels {
			app.Labels[k] = v
		}
		changes.AddedLabels = output.Labels
		ctx.Info("Merged labels from output", "policy", result.PolicyName, "count", len(output.Labels))
	}

	// Apply annotations (always merge)
	if output.Annotations != nil && len(output.Annotations) > 0 {
		if app.Annotations == nil {
			app.Annotations = make(map[string]string)
		}
		for k, v := range output.Annotations {
			app.Annotations[k] = v
		}
		changes.AddedAnnotations = output.Annotations
		ctx.Info("Merged annotations from output", "policy", result.PolicyName, "count", len(output.Annotations))
	}

	// Store additionalContext in context
	if result.AdditionalContext != nil {
		ctx = storeAdditionalContextInCtx(ctx, result.AdditionalContext)
		ctx.Info("Stored cached additionalContext in context", "policy", result.PolicyName, "keys", len(result.AdditionalContext))
	}

	recordApplicationPolicyStatus(app, result.PolicyName, result.PolicyNamespace, "global", sequence, priority, true, "", changes)
	ctx.Info("Successfully applied cached policy result", "policy", result.PolicyName)
	return ctx, changes, nil
}

// renderPolicyCUETemplate renders the policy CUE template with parameter and context.application
// Now includes CueX support by creating a proper process.Context with runtime parameters.
// This enables CueX actions like kube.#Read while preserving all existing functionality:
// - context.application (Full Application CR)
// - context.prior (Previous policy result for incremental policies)
// - parameter (Policy parameters from Application spec)
func (h *AppHandler) renderPolicyCUETemplate(ctx monitorContext.Context, app *v1beta1.Application, params map[string]interface{}, policyDef *v1beta1.PolicyDefinition, priorResult map[string]interface{}) (cue.Value, error) {
	// Create runtime context with KubeClient so kube.#Read and other CueX actions work
	runtimeCtx := oamprovidertypes.WithRuntimeParams(ctx.GetContext(), oamprovidertypes.RuntimeParams{
		KubeClient: h.Client,
		ConfigFactory: config.NewConfigFactoryWithDispatcher(h.Client, func(goCtx context.Context, resources []*unstructured.Unstructured, applyOptions []apply.ApplyOption) error {
			// Policies don't dispatch resources directly, but provide this for CueX consistency
			return nil
		}),
	})

	// Get current ApplicationRevision name for context
	var appRevisionName string
	if h.currentAppRev != nil {
		appRevisionName = h.currentAppRev.Name
	}

	// Create a process.Context with proper runtime parameters embedded for CueX execution
	// This provides explicit fields and filtered metadata for security
	pCtx := velaprocess.NewContext(velaprocess.ContextData{
		Namespace:       app.Namespace,
		AppName:         app.Name,
		CompName:        app.Name,                            // Policy context doesn't have specific component
		AppRevisionName: appRevisionName,                     // Explicit appRevision field
		AppLabels:       filterUserMetadata(app.Labels),      // Filtered labels (security)
		AppAnnotations:  filterUserMetadata(app.Annotations), // Filtered annotations (security)
		AppComponents:   app.Spec.Components,                 // Controlled spec access
		AppWorkflow:     app.Spec.Workflow,                   // Controlled spec access
		AppPolicies:     app.Spec.Policies,                   // Controlled spec access
		Ctx:             runtimeCtx,                          // Use runtime context with CueX providers
	})

	// Build parameter file (as JSON, not type annotation)
	var paramFile string
	if params != nil {
		paramJSON, err := json.Marshal(params)
		if err != nil {
			return cue.Value{}, errors.Wrap(err, "failed to marshal parameters")
		}
		paramFile = fmt.Sprintf("parameter: %s", string(paramJSON))
	} else {
		paramFile = "parameter: {}"
	}

	// Get base context with explicit fields (appName, namespace, appRevision, etc.)
	// and filtered metadata (appLabels, appAnnotations) from process.Context
	baseContext, err := pCtx.BaseContextFile()
	if err != nil {
		return cue.Value{}, errors.Wrap(err, "failed to generate base context")
	}

	// Build additional context fields for context.prior (if available)
	// context.prior: Previous policy result for incremental policies
	var contextFile string
	if priorResult != nil {
		priorJSON, err := json.Marshal(priorResult)
		if err != nil {
			return cue.Value{}, errors.Wrap(err, "failed to marshal prior result")
		}
		contextFile = fmt.Sprintf("context: {\nprior: %s\n}", string(priorJSON))
	}

	// Build CUE source with base context (explicit fields + filtered metadata), parameters, and prior
	// cuex.DefaultCompiler already has all the imports (kube, http, etc.)
	cueSource := strings.Join([]string{
		policyDef.Spec.Schematic.CUE.Template,
		paramFile,
		baseContext,  // Explicit fields (appName, namespace, appLabels, appComponents, etc.) + filtered metadata
		contextFile,  // context.prior (if available)
	}, "\n")

	// Compile with CueX execution enabled (cuex.DefaultCompiler automatically resolves actions)
	val, err := cuex.DefaultCompiler.Get().CompileString(pCtx.GetCtx(), cueSource)
	if err != nil {
		return cue.Value{}, errors.Wrap(err, "failed to compile CUE template")
	}

	// Validate
	if err := val.Validate(); err != nil {
		return cue.Value{}, errors.Wrap(err, "CUE validation failed")
	}

	return val, nil
}

// RefreshMode defines how often a policy output should be refreshed
type RefreshMode string

const (
	RefreshAlways   RefreshMode = "always"   // Re-render on every reconciliation
	RefreshNever    RefreshMode = "never"    // Cache indefinitely (until Application revision changes)
	RefreshPeriodic RefreshMode = "periodic" // Re-render after interval seconds
)

// OutputRefreshConfig configures refresh behavior for a specific output type
type OutputRefreshConfig struct {
	Mode         RefreshMode `json:"mode"`                   // How often to refresh
	Interval     *int32      `json:"interval,omitempty"`     // Seconds between refreshes (for periodic mode)
	ForceRefresh *bool       `json:"forceRefresh,omitempty"` // Dynamic expression to force immediate refresh
}

// PolicyConfig represents policy configuration settings
type PolicyConfig struct {
	Enabled bool `json:"enabled"` // Whether policy is enabled (default: true)

	// Per-output-type refresh control
	Refresh struct {
		Spec        *OutputRefreshConfig `json:"spec,omitempty"`        // components, workflow, policies
		Labels      *OutputRefreshConfig `json:"labels,omitempty"`      // metadata labels
		Annotations *OutputRefreshConfig `json:"annotations,omitempty"` // metadata annotations
		Ctx         *OutputRefreshConfig `json:"ctx,omitempty"`         // additional context
	} `json:"refresh,omitempty"`
}

// extractEnabled extracts the enabled field from rendered CUE (defaults to true)
// Supports both new API (config.enabled) and old API (enabled at root)
func (h *AppHandler) extractEnabled(val cue.Value) (bool, error) {
	// Try new API first: config.enabled
	configVal := val.LookupPath(cue.ParsePath("config"))
	if configVal.Exists() {
		enabledVal := configVal.LookupPath(cue.ParsePath("enabled"))
		if enabledVal.Exists() {
			enabled, err := enabledVal.Bool()
			if err != nil {
				return false, errors.Wrap(err, "failed to decode config.enabled (must be boolean)")
			}
			return enabled, nil
		}
		// config exists but no enabled field - default to true
		return true, nil
	}

	// Fall back to old API: enabled at root level (for backwards compatibility)
	enabledVal := val.LookupPath(cue.ParsePath("enabled"))
	if !enabledVal.Exists() {
		// No enabled field in either location, default to true
		return true, nil
	}

	enabled, err := enabledVal.Bool()
	if err != nil {
		return false, errors.Wrap(err, "failed to decode enabled (must be boolean)")
	}

	return enabled, nil
}

// extractRefreshConfig extracts the refresh configuration from rendered CUE
// Returns PolicyConfig with defaults applied if config doesn't exist
func (h *AppHandler) extractRefreshConfig(val cue.Value) (*PolicyConfig, error) {
	configVal := val.LookupPath(cue.ParsePath("config"))
	if !configVal.Exists() {
		// No config block - return defaults
		return applyRefreshDefaults(&PolicyConfig{Enabled: true}), nil
	}

	var config PolicyConfig
	if err := configVal.Decode(&config); err != nil {
		return nil, errors.Wrap(err, "failed to decode config")
	}

	// Apply defaults for any missing refresh configs
	config = *applyRefreshDefaults(&config)

	// Validate: periodic mode requires interval > 0
	validationErrors := []string{}
	for name, cfg := range map[string]*OutputRefreshConfig{
		"spec":        config.Refresh.Spec,
		"labels":      config.Refresh.Labels,
		"annotations": config.Refresh.Annotations,
		"ctx":         config.Refresh.Ctx,
	} {
		if cfg != nil && cfg.Mode == RefreshPeriodic {
			if cfg.Interval == nil || *cfg.Interval <= 0 {
				validationErrors = append(validationErrors,
					fmt.Sprintf("config.refresh.%s.mode='periodic' requires interval > 0", name))
			}
		}
	}

	if len(validationErrors) > 0 {
		return nil, errors.New(strings.Join(validationErrors, "; "))
	}

	return &config, nil
}

// applyRefreshDefaults applies default values to refresh configuration
func applyRefreshDefaults(config *PolicyConfig) *PolicyConfig {
	if config.Refresh.Spec == nil {
		config.Refresh.Spec = &OutputRefreshConfig{Mode: RefreshNever}
	}
	if config.Refresh.Labels == nil {
		config.Refresh.Labels = &OutputRefreshConfig{Mode: RefreshNever}
	}
	if config.Refresh.Annotations == nil {
		config.Refresh.Annotations = &OutputRefreshConfig{Mode: RefreshNever}
	}
	if config.Refresh.Ctx == nil {
		config.Refresh.Ctx = &OutputRefreshConfig{Mode: RefreshNever}
	}
	return config
}

// TransformOperationType defines the type of operation for a transform
type TransformOperationType string

const (
	TransformReplace TransformOperationType = "replace"
	TransformMerge   TransformOperationType = "merge"
)

// Transform represents a typed transformation operation
type Transform struct {
	Type  TransformOperationType `json:"type"`
	Value interface{}            `json:"value"`
}

// PolicyTransforms represents the allowed transformation operations (old API)
type PolicyTransforms struct {
	Spec        *Transform `json:"spec,omitempty"`
	Labels      *Transform `json:"labels,omitempty"`
	Annotations *Transform `json:"annotations,omitempty"`
}

// PolicyOutput represents the new simplified output structure (new API)
type PolicyOutput struct {
	Components  []common.ApplicationComponent `json:"components,omitempty"`
	Workflow    *v1beta1.Workflow             `json:"workflow,omitempty"`
	Policies    []v1beta1.AppPolicy           `json:"policies,omitempty"`
	Labels      map[string]string             `json:"labels,omitempty"`
	Annotations map[string]string             `json:"annotations,omitempty"`
	Ctx         map[string]interface{}        `json:"ctx,omitempty"`
}

// CachedOutputData represents cached data for a specific output type with refresh metadata
type CachedOutputData struct {
	RenderedAt      string      `json:"rendered_at"`              // RFC3339 timestamp
	RefreshMode     RefreshMode `json:"refresh_mode"`             // "always", "never", "periodic"
	RefreshInterval *int32      `json:"refresh_interval,omitempty"` // seconds (for periodic mode)
	Data            interface{} `json:"data,omitempty"`           // The actual output data
}

// PolicyCacheRecord represents the complete cache entry for a policy in ConfigMap
// This is the new per-output-type structure with cascade tracking
type PolicyCacheRecord struct {
	PolicyName      string `json:"policy_name"`
	PolicyNamespace string `json:"policy_namespace"`
	Priority        int    `json:"priority"`
	Sequence        int    `json:"sequence"`
	ApplicationHash string `json:"application_hash"` // For revision change detection
	LastCascadeID   string `json:"last_cascade_id"`  // Tracks upstream policy changes

	// Per-output-type cached data
	Spec        *CachedOutputData `json:"spec,omitempty"`        // components, workflow, policies
	Labels      *CachedOutputData `json:"labels,omitempty"`      // metadata labels
	Annotations *CachedOutputData `json:"annotations,omitempty"` // metadata annotations
	Ctx         *CachedOutputData `json:"ctx,omitempty"`         // additional context
}

// extractOutput extracts the output field from rendered CUE (new API)
// Returns nil if output doesn't exist (old API being used or no output specified)
func (h *AppHandler) extractOutput(val cue.Value) (*PolicyOutput, error) {
	outputVal := val.LookupPath(cue.ParsePath("output"))
	if !outputVal.Exists() {
		return nil, nil // No output field
	}

	var output PolicyOutput
	if err := outputVal.Decode(&output); err != nil {
		return nil, errors.Wrap(err, "failed to decode output")
	}

	// Validate structure: only allowed fields
	iter, err := outputVal.Fields()
	if err != nil {
		return nil, errors.Wrap(err, "failed to iterate output fields")
	}

	allowedFields := map[string]bool{
		"components":  true,
		"workflow":    true,
		"policies":    true,
		"labels":      true,
		"annotations": true,
		"ctx":         true,
	}

	for iter.Next() {
		fieldName := iter.Selector().String()
		if !allowedFields[fieldName] {
			return nil, errors.Errorf("output.%s is not allowed; only 'components', 'workflow', 'policies', 'labels', 'annotations', and 'ctx' are permitted", fieldName)
		}
	}

	return &output, nil
}

// validateTransformType ensures the transform type is valid
// allowReplace controls whether 'replace' operation is permitted
func validateTransformType(opType TransformOperationType, fieldName string, allowReplace bool) error {
	if opType == TransformMerge {
		return nil
	}
	if opType == TransformReplace {
		if !allowReplace {
			return errors.Errorf("transforms.%s.type='replace' is not allowed; only 'merge' is permitted for %s", fieldName, fieldName)
		}
		return nil
	}
	return errors.Errorf("transforms.%s.type must be 'replace' or 'merge', got: %s", fieldName, opType)
}

// extractAdditionalContext extracts the additionalContext field from rendered CUE
func (h *AppHandler) extractAdditionalContext(val cue.Value) (map[string]interface{}, error) {
	contextVal := val.LookupPath(cue.ParsePath("additionalContext"))
	if !contextVal.Exists() {
		// No additionalContext field, that's OK
		return nil, nil
	}

	var additionalContext map[string]interface{}
	if err := contextVal.Decode(&additionalContext); err != nil {
		return nil, errors.Wrap(err, "failed to decode additionalContext")
	}

	return additionalContext, nil
}

// applyTransformsToApplication applies transforms to the in-memory Application object
// Handles both replace (complete replacement) and merge (deep merge) operations
func (h *AppHandler) applyTransformsToApplication(ctx monitorContext.Context, app *v1beta1.Application, transforms *PolicyTransforms) error {
	// Apply spec transform
	if transforms.Spec != nil {
		if transforms.Spec.Type == TransformReplace {
			ctx.Info("Replacing Application spec", "operation", "spec.replace")
			// Convert spec to proper type and replace
			specBytes, err := json.Marshal(transforms.Spec.Value)
			if err != nil {
				return errors.Wrap(err, "failed to marshal spec value")
			}
			var newSpec v1beta1.ApplicationSpec
			if err := json.Unmarshal(specBytes, &newSpec); err != nil {
				return errors.Wrap(err, "failed to unmarshal spec value into ApplicationSpec")
			}
			app.Spec = newSpec
		} else if transforms.Spec.Type == TransformMerge {
			ctx.Info("Merging Application spec", "operation", "spec.merge")
			// Convert both to unstructured for deep merge
			appUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(app)
			if err != nil {
				return errors.Wrap(err, "failed to convert Application to unstructured")
			}

			// Deep merge spec
			if appSpec, ok := appUnstructured["spec"].(map[string]interface{}); ok {
				specValue, ok := transforms.Spec.Value.(map[string]interface{})
				if !ok {
					return errors.New("spec.value must be an object for merge operation")
				}
				mergedSpec := deepMerge(appSpec, specValue)
				appUnstructured["spec"] = mergedSpec

				// Convert back to Application
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(appUnstructured, app); err != nil {
					return errors.Wrap(err, "failed to convert unstructured back to Application")
				}
			}
		}
	}

	// Apply labels transform (merge only)
	if transforms.Labels != nil {
		ctx.Info("Merging labels", "operation", "labels.merge")
		if app.Labels == nil {
			app.Labels = make(map[string]string)
		}
		labelsMap, ok := transforms.Labels.Value.(map[string]interface{})
		if !ok {
			return errors.New("labels.value must be an object")
		}
		for k, v := range labelsMap {
			if strVal, ok := v.(string); ok {
				app.Labels[k] = strVal
			}
		}
	}

	// Apply annotations transform (merge only)
	if transforms.Annotations != nil {
		ctx.Info("Merging annotations", "operation", "annotations.merge")
		if app.Annotations == nil {
			app.Annotations = make(map[string]string)
		}
		annotationsMap, ok := transforms.Annotations.Value.(map[string]interface{})
		if !ok {
			return errors.New("annotations.value must be an object")
		}
		for k, v := range annotationsMap {
			if strVal, ok := v.(string); ok {
				app.Annotations[k] = strVal
			}
		}
	}

	return nil
}

// deepMerge recursively merges source into target
func deepMerge(target, source map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy target
	for k, v := range target {
		result[k] = v
	}

	// Merge source
	for key, sourceValue := range source {
		if targetValue, exists := result[key]; exists {
			// If both are maps, merge recursively
			if targetMap, targetIsMap := targetValue.(map[string]interface{}); targetIsMap {
				if sourceMap, sourceIsMap := sourceValue.(map[string]interface{}); sourceIsMap {
					result[key] = deepMerge(targetMap, sourceMap)
					continue
				}
			}
		}
		// Otherwise, replace/set the value from source
		result[key] = sourceValue
	}

	return result
}

// policyAdditionalContextKeyString is the string key for storing additionalContext in Go context
// We use a string key to avoid type mismatches when accessing from different packages (e.g., pkg/cue/process)
const policyAdditionalContextKeyString = "kubevela.oam.dev/policy-additional-context"

// storeAdditionalContextInCtx stores additional policy context in the Go context
// This context will be available in workflow steps as context.custom
func storeAdditionalContextInCtx(ctx monitorContext.Context, additionalContext map[string]interface{}) monitorContext.Context {
	// Get existing additional context if any
	existing := getAdditionalContextFromCtx(ctx)
	if existing == nil {
		existing = make(map[string]interface{})
	}

	// Merge new context into existing
	merged := deepMerge(existing, additionalContext)

	// Store back in context using a string key (avoids type mismatches across packages)
	// We need to extract the underlying context.Context, add our value, and wrap it back
	baseCtx := context.WithValue(ctx.GetContext(), policyAdditionalContextKeyString, merged)
	ctx.SetContext(baseCtx)
	return ctx
}

// getAdditionalContextFromCtx retrieves additional policy context from the Go context
func getAdditionalContextFromCtx(ctx monitorContext.Context) map[string]interface{} {
	if val := ctx.GetContext().Value(policyAdditionalContextKeyString); val != nil {
		if contextMap, ok := val.(map[string]interface{}); ok {
			return contextMap
		}
	}
	return nil
}

// shouldSkipGlobalPolicies checks if the Application has opted out of global policies
func shouldSkipGlobalPolicies(app *v1beta1.Application) bool {
	if app.Annotations == nil {
		return false
	}
	return app.Annotations[SkipGlobalPoliciesAnnotation] == "true"
}

// discoverGlobalPolicies discovers and returns Application-scoped global policies from a namespace
// Policies are sorted by Priority (descending) then Name (alphabetical)
func discoverGlobalPolicies(ctx monitorContext.Context, cli client.Client, namespace string) ([]v1beta1.PolicyDefinition, error) {
	// List all PolicyDefinitions in namespace
	policyList := &v1beta1.PolicyDefinitionList{}
	if err := cli.List(ctx.GetContext(), policyList, client.InNamespace(namespace)); err != nil {
		return nil, errors.Wrapf(err, "failed to list PolicyDefinitions in namespace %s", namespace)
	}

	// Filter for Global=true and Scope=Application
	var globalPolicies []v1beta1.PolicyDefinition
	for _, policy := range policyList.Items {
		if policy.Spec.Global && policy.Spec.Scope == v1beta1.ApplicationScope {
			globalPolicies = append(globalPolicies, policy)
		}
	}

	// Sort by Priority (higher first), then by Name (alphabetical)
	sort.Slice(globalPolicies, func(i, j int) bool {
		if globalPolicies[i].Spec.Priority != globalPolicies[j].Spec.Priority {
			return globalPolicies[i].Spec.Priority > globalPolicies[j].Spec.Priority // Higher priority first
		}
		return globalPolicies[i].Name < globalPolicies[j].Name // Alphabetical for same priority
	})

	ctx.Info("Discovered global policies", "namespace", namespace, "count", len(globalPolicies))
	return globalPolicies, nil
}

// validateNotGlobalPolicy validates that a policy is not marked as Global
// Returns error if the policy is Global (cannot be explicitly referenced)
func validateNotGlobalPolicy(ctx monitorContext.Context, cli client.Client, policyName string, namespace string) error {
	// Load policy using LoadTemplate (handles 2-step lookup)
	templ, err := appfile.LoadTemplate(ctx.GetContext(), cli, policyName, types.TypePolicy, nil)
	if err != nil {
		// Policy not found or error loading - skip validation
		return nil
	}

	if templ.PolicyDefinition != nil && templ.PolicyDefinition.Spec.Global {
		return errors.Errorf("policy '%s' is marked as Global and cannot be explicitly referenced in Application spec", policyName)
	}

	return nil
}

// recordApplicationPolicyStatus records the application status of an Application-scoped policy
// (global or explicit)
func recordApplicationPolicyStatus(app *v1beta1.Application, policyName, policyNamespace, source string, sequence int, priority int32, applied bool, reason string, changes *PolicyChanges) {
	entry := common.AppliedApplicationPolicy{
		Name:      policyName,
		Namespace: policyNamespace,
		Applied:   applied,
		Reason:    reason,
		Source:    source, // "global" or "explicit"
	}

	// Record summary counts of what was changed (if policy was applied)
	// Full details are stored in the ApplicationPoliciesConfigMap
	if applied && changes != nil {
		entry.SpecModified = changes.SpecModified
		entry.LabelsCount = len(changes.AddedLabels)
		entry.AnnotationsCount = len(changes.AddedAnnotations)
		entry.HasContext = changes.AdditionalContext != nil && len(changes.AdditionalContext) > 0
	}

	app.Status.AppliedApplicationPolicies = append(app.Status.AppliedApplicationPolicies, entry)
}

// PolicyChanges tracks what a policy modified
type PolicyChanges struct {
	AddedLabels       map[string]string
	AddedAnnotations  map[string]string
	AdditionalContext map[string]interface{}
	SpecModified      bool

	// Full rendered output for caching/reuse
	Enabled    bool
	Transforms *PolicyTransforms // Old transforms API
	Output     *PolicyOutput     // New output API
}

// policyConfigMapMetadata tracks metadata needed for ConfigMap storage
type policyConfigMapMetadata struct {
	Name       string
	Namespace  string
	Source     string // "global" or "explicit"
	Sequence   int
	Priority   int32
	Config     *PolicyConfig // Refresh configuration from policy template
	CascadeID  string        // Hash of upstream policy outputs for cascade invalidation
}

// serializeTransformsForStorage converts PolicyTransforms to a format suitable for storage and reuse
func serializeTransformsForStorage(transforms *PolicyTransforms) map[string]interface{} {
	if transforms == nil {
		return nil
	}

	result := make(map[string]interface{})

	if transforms.Labels != nil {
		result["labels"] = map[string]interface{}{
			"type":  transforms.Labels.Type,
			"value": transforms.Labels.Value,
		}
	}

	if transforms.Annotations != nil {
		result["annotations"] = map[string]interface{}{
			"type":  transforms.Annotations.Type,
			"value": transforms.Annotations.Value,
		}
	}

	if transforms.Spec != nil {
		result["spec"] = map[string]interface{}{
			"type":  transforms.Spec.Type,
			"value": transforms.Spec.Value,
		}
	}

	return result
}

// serializeOutputForStorage converts PolicyOutput to a format suitable for storage and reuse
func serializeOutputForStorage(output *PolicyOutput) map[string]interface{} {
	if output == nil {
		return nil
	}

	result := make(map[string]interface{})

	if output.Components != nil {
		result["components"] = output.Components
	}

	if output.Workflow != nil {
		result["workflow"] = output.Workflow
	}

	if output.Policies != nil {
		result["policies"] = output.Policies
	}

	if output.Labels != nil {
		result["labels"] = output.Labels
	}

	if output.Annotations != nil {
		result["annotations"] = output.Annotations
	}

	if output.Ctx != nil {
		result["ctx"] = output.Ctx
	}

	return result
}

// deserializeOutputFromStorage converts stored output back to PolicyOutput
func deserializeOutputFromStorage(outputData map[string]interface{}) *PolicyOutput {
	if outputData == nil {
		return nil
	}

	output := &PolicyOutput{}

	// Decode components
	if componentsData, ok := outputData["components"]; ok {
		jsonBytes, err := json.Marshal(componentsData)
		if err != nil {
			klog.Errorf("Failed to marshal components data: %v", err)
		} else {
			if err := json.Unmarshal(jsonBytes, &output.Components); err != nil {
				klog.Errorf("Failed to unmarshal components: %v", err)
			}
		}
	}

	// Decode workflow
	if workflowData, ok := outputData["workflow"]; ok {
		jsonBytes, err := json.Marshal(workflowData)
		if err != nil {
			klog.Errorf("Failed to marshal workflow data: %v", err)
		} else {
			if err := json.Unmarshal(jsonBytes, &output.Workflow); err != nil {
				klog.Errorf("Failed to unmarshal workflow: %v", err)
			}
		}
	}

	// Decode policies
	if policiesData, ok := outputData["policies"]; ok {
		jsonBytes, err := json.Marshal(policiesData)
		if err != nil {
			klog.Errorf("Failed to marshal policies data: %v", err)
		} else {
			if err := json.Unmarshal(jsonBytes, &output.Policies); err != nil {
				klog.Errorf("Failed to unmarshal policies: %v", err)
			}
		}
	}

	// Decode labels
	if labelsData, ok := outputData["labels"].(map[string]interface{}); ok {
		output.Labels = make(map[string]string)
		for k, v := range labelsData {
			if strVal, ok := v.(string); ok {
				output.Labels[k] = strVal
			}
		}
	}

	// Decode annotations
	if annotationsData, ok := outputData["annotations"].(map[string]interface{}); ok {
		output.Annotations = make(map[string]string)
		for k, v := range annotationsData {
			if strVal, ok := v.(string); ok {
				output.Annotations[k] = strVal
			}
		}
	}

	// Decode ctx
	if ctxData, ok := outputData["ctx"].(map[string]interface{}); ok {
		output.Ctx = ctxData
	}

	return output
}

// createPolicyCacheRecord creates a new PolicyCacheRecord with per-output-type caching
func createPolicyCacheRecord(
	policyName, policyNamespace string,
	priority, sequence int,
	appHash, cascadeID string,
	output *PolicyOutput,
	config *PolicyConfig,
) *PolicyCacheRecord {
	now := time.Now().Format(time.RFC3339)

	record := &PolicyCacheRecord{
		PolicyName:      policyName,
		PolicyNamespace: policyNamespace,
		Priority:        priority,
		Sequence:        sequence,
		ApplicationHash: appHash,
		LastCascadeID:   cascadeID,
	}

	// Create spec cache entry (components, workflow, policies)
	if config.Refresh.Spec != nil {
		specData := make(map[string]interface{})
		if output.Components != nil {
			specData["components"] = output.Components
		}
		if output.Workflow != nil {
			specData["workflow"] = output.Workflow
		}
		if output.Policies != nil {
			specData["policies"] = output.Policies
		}

		if len(specData) > 0 {
			record.Spec = &CachedOutputData{
				RenderedAt:      now,
				RefreshMode:     config.Refresh.Spec.Mode,
				RefreshInterval: config.Refresh.Spec.Interval,
				Data:            specData,
			}
		}
	}

	// Create labels cache entry
	if config.Refresh.Labels != nil && output.Labels != nil && len(output.Labels) > 0 {
		record.Labels = &CachedOutputData{
			RenderedAt:      now,
			RefreshMode:     config.Refresh.Labels.Mode,
			RefreshInterval: config.Refresh.Labels.Interval,
			Data:            output.Labels,
		}
	}

	// Create annotations cache entry
	if config.Refresh.Annotations != nil && output.Annotations != nil && len(output.Annotations) > 0 {
		record.Annotations = &CachedOutputData{
			RenderedAt:      now,
			RefreshMode:     config.Refresh.Annotations.Mode,
			RefreshInterval: config.Refresh.Annotations.Interval,
			Data:            output.Annotations,
		}
	}

	// Create ctx cache entry
	if config.Refresh.Ctx != nil && output.Ctx != nil && len(output.Ctx) > 0 {
		record.Ctx = &CachedOutputData{
			RenderedAt:      now,
			RefreshMode:     config.Refresh.Ctx.Mode,
			RefreshInterval: config.Refresh.Ctx.Interval,
			Data:            output.Ctx,
		}
	}

	return record
}

// reconstructPolicyOutputFromCache reconstructs a PolicyOutput from a PolicyCacheRecord
func reconstructPolicyOutputFromCache(record *PolicyCacheRecord) *PolicyOutput {
	if record == nil {
		return nil
	}

	output := &PolicyOutput{}

	// Reconstruct spec (components, workflow, policies)
	if record.Spec != nil && record.Spec.Data != nil {
		if specData, ok := record.Spec.Data.(map[string]interface{}); ok {
			// Decode components
			if componentsData, ok := specData["components"]; ok {
				jsonBytes, err := json.Marshal(componentsData)
				if err == nil {
					json.Unmarshal(jsonBytes, &output.Components)
				}
			}

			// Decode workflow
			if workflowData, ok := specData["workflow"]; ok {
				jsonBytes, err := json.Marshal(workflowData)
				if err == nil {
					json.Unmarshal(jsonBytes, &output.Workflow)
				}
			}

			// Decode policies
			if policiesData, ok := specData["policies"]; ok {
				jsonBytes, err := json.Marshal(policiesData)
				if err == nil {
					json.Unmarshal(jsonBytes, &output.Policies)
				}
			}
		}
	}

	// Reconstruct labels
	if record.Labels != nil && record.Labels.Data != nil {
		if labelsData, ok := record.Labels.Data.(map[string]interface{}); ok {
			output.Labels = make(map[string]string)
			for k, v := range labelsData {
				if strVal, ok := v.(string); ok {
					output.Labels[k] = strVal
				}
			}
		}
	}

	// Reconstruct annotations
	if record.Annotations != nil && record.Annotations.Data != nil {
		if annotationsData, ok := record.Annotations.Data.(map[string]interface{}); ok {
			output.Annotations = make(map[string]string)
			for k, v := range annotationsData {
				if strVal, ok := v.(string); ok {
					output.Annotations[k] = strVal
				}
			}
		}
	}

	// Reconstruct ctx
	if record.Ctx != nil && record.Ctx.Data != nil {
		if ctxData, ok := record.Ctx.Data.(map[string]interface{}); ok {
			output.Ctx = ctxData
		}
	}

	return output
}

// shouldRefreshOutput determines if a cached output needs to be refreshed
// based on the refresh mode, interval, and forceRefresh flag
func shouldRefreshOutput(cachedData *CachedOutputData, config *OutputRefreshConfig) bool {
	if cachedData == nil || config == nil {
		return true // No cache or config - must render
	}

	// Check forceRefresh flag first
	if config.ForceRefresh != nil && *config.ForceRefresh {
		return true // Force refresh requested
	}

	// Check refresh mode
	switch config.Mode {
	case RefreshAlways:
		return true // Always refresh

	case RefreshNever:
		return false // Never refresh (cache is valid)

	case RefreshPeriodic:
		// Check if enough time has elapsed
		if config.Interval == nil || *config.Interval <= 0 {
			// Invalid interval - treat as "never"
			return false
		}

		renderedAt, err := time.Parse(time.RFC3339, cachedData.RenderedAt)
		if err != nil {
			// Invalid timestamp - must refresh
			return true
		}

		elapsed := time.Since(renderedAt)
		interval := time.Duration(*config.Interval) * time.Second
		return elapsed >= interval

	default:
		// Unknown mode - be safe and refresh
		return true
	}
}

// loadCachedPolicyRecord loads a PolicyCacheRecord from ConfigMap with cascade and refresh checks
// Returns the cached record if valid, nil if cache miss or invalidated
func loadCachedPolicyRecord(
	ctx context.Context,
	cli client.Client,
	app *v1beta1.Application,
	policyName string,
	config *PolicyConfig,
	upstreamOutputs []*PolicyOutput,
) (*PolicyCacheRecord, error) {
	if app.Status.ApplicationPoliciesConfigMap == "" {
		return nil, nil // No ConfigMap exists yet
	}

	// Get the ConfigMap
	cm := &corev1.ConfigMap{}
	cmName := app.Status.ApplicationPoliciesConfigMap
	if err := cli.Get(ctx, client.ObjectKey{Name: cmName, Namespace: app.Namespace}, cm); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, nil // ConfigMap doesn't exist
		}
		return nil, err
	}

	// Find the entry for this policy
	var cachedData string
	for key, value := range cm.Data {
		// Keys are formatted as "001-policy-name"
		if strings.HasSuffix(key, "-"+policyName) {
			cachedData = value
			break
		}
	}

	if cachedData == "" {
		return nil, nil // Policy not in ConfigMap
	}

	// Parse the cached record
	var record PolicyCacheRecord
	if err := json.Unmarshal([]byte(cachedData), &record); err != nil {
		// Try old format for backwards compatibility
		return nil, nil // Can't parse - treat as cache miss
	}

	// Check if Application state has changed (cache invalidation)
	currentHash, err := computeApplicationHash(app)
	if err == nil && currentHash != "" {
		if record.ApplicationHash != currentHash {
			// Application changed - cache is invalid
			return nil, nil
		}
	}

	// Check cascade ID - if upstream policies changed, this cache is stale
	currentCascadeID, err := computeCascadeID(upstreamOutputs)
	if err == nil {
		if record.LastCascadeID != currentCascadeID {
			// Upstream policies changed - cascade invalidation
			return nil, nil
		}
	}

	// Check if any output type needs refresh
	needsRefresh := false

	if config != nil && config.Refresh.Spec != nil {
		if shouldRefreshOutput(record.Spec, config.Refresh.Spec) {
			needsRefresh = true
		}
	}

	if config != nil && config.Refresh.Labels != nil {
		if shouldRefreshOutput(record.Labels, config.Refresh.Labels) {
			needsRefresh = true
		}
	}

	if config != nil && config.Refresh.Annotations != nil {
		if shouldRefreshOutput(record.Annotations, config.Refresh.Annotations) {
			needsRefresh = true
		}
	}

	if config != nil && config.Refresh.Ctx != nil {
		if shouldRefreshOutput(record.Ctx, config.Refresh.Ctx) {
			needsRefresh = true
		}
	}

	if needsRefresh {
		return nil, nil // Cache expired or force refresh
	}

	// Cache is valid
	return &record, nil
}

// loadCachedPolicyFromConfigMap attempts to load a cached policy result from the ConfigMap
// Returns the cached result if found and valid according to TTL and Application state, nil otherwise
// DEPRECATED: This is the old format loader kept for backwards compatibility
func loadCachedPolicyFromConfigMap(ctx context.Context, cli client.Client, app *v1beta1.Application, policyName string, ttlSeconds int32) (map[string]interface{}, error) {
	if app.Status.ApplicationPoliciesConfigMap == "" {
		return nil, nil // No ConfigMap exists yet
	}

	// Get the ConfigMap
	cm := &corev1.ConfigMap{}
	cmName := app.Status.ApplicationPoliciesConfigMap
	if err := cli.Get(ctx, client.ObjectKey{Name: cmName, Namespace: app.Namespace}, cm); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, nil // ConfigMap doesn't exist
		}
		return nil, err
	}

	// Find the entry for this policy
	var cachedData string
	for key, value := range cm.Data {
		// Keys are formatted as "001-policy-name"
		if strings.HasSuffix(key, "-"+policyName) {
			cachedData = value
			break
		}
	}

	if cachedData == "" {
		return nil, nil // Policy not in ConfigMap
	}

	// Parse the cached record
	var record map[string]interface{}
	if err := json.Unmarshal([]byte(cachedData), &record); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal cached policy record")
	}

	// Check if Application state has changed (cache invalidation)
	currentHash, err := computeApplicationHash(app)
	if err == nil && currentHash != "" {
		cachedHash, _ := record["application_hash"].(string)
		if cachedHash != currentHash {
			// Application changed - cache is invalid even if TTL hasn't expired
			return nil, nil
		}
	}

	// Check TTL
	if ttlSeconds == -1 {
		// Never refresh - always use cached (if Application hasn't changed)
		return record, nil
	}

	if ttlSeconds == 0 {
		// Never cache - always re-render
		return nil, nil
	}

	// Check if cache is still valid by time
	renderedAtStr, ok := record["rendered_at"].(string)
	if !ok {
		return nil, nil // Invalid format
	}

	renderedAt, err := time.Parse(time.RFC3339, renderedAtStr)
	if err != nil {
		return nil, nil // Invalid timestamp
	}

	elapsed := time.Since(renderedAt)
	ttl := time.Duration(ttlSeconds) * time.Second

	if elapsed < ttl {
		// Cache is still valid (both time and Application state)
		return record, nil
	}

	// Cache expired
	return nil, nil
}

// computeApplicationHash computes a hash of the Application state that affects policy rendering
// This ONLY includes spec, following KubeVela's standard revision hash approach.
// Labels and annotations are NOT included because:
// - Policies can modify labels/annotations, which would invalidate downstream caches
// - Only spec changes should trigger cache invalidation
// - Per-output-type refresh control handles metadata changes independently
func computeApplicationHash(app *v1beta1.Application) (string, error) {
	// Use apply.ComputeSpecHash which is KubeVela's standard approach
	// This only hashes the spec, not labels or annotations
	return apply.ComputeSpecHash(&app.Spec)
}

// computeCascadeID computes a hash of all upstream policy outputs
// This is used for cascade invalidation - if upstream policies change, downstream caches must be invalidated
// upstreamOutputs should contain the PolicyOutput from all policies that executed before this one
func computeCascadeID(upstreamOutputs []*PolicyOutput) (string, error) {
	if len(upstreamOutputs) == 0 {
		// First policy in the chain - no upstream dependencies
		return "", nil
	}

	// Extract ONLY spec fields from upstream outputs
	// Labels, annotations, and ctx changes should NOT trigger cascade invalidation
	// Only spec changes (components, workflow, policies) affect downstream policy behavior
	type specOnly struct {
		Components []common.ApplicationComponent `json:"components,omitempty"`
		Workflow   *v1beta1.Workflow             `json:"workflow,omitempty"`
		Policies   []v1beta1.AppPolicy           `json:"policies,omitempty"`
	}

	upstreamSpecs := make([]specOnly, 0, len(upstreamOutputs))
	for _, output := range upstreamOutputs {
		upstreamSpecs = append(upstreamSpecs, specOnly{
			Components: output.Components,
			Workflow:   output.Workflow,
			Policies:   output.Policies,
		})
	}

	// Hash only the spec fields
	jsonBytes, err := json.Marshal(upstreamSpecs)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal upstream specs for cascade ID")
	}

	// Compute SHA256 hash
	hash := sha256.Sum256(jsonBytes)
	return hex.EncodeToString(hash[:]), nil
}

// deserializeTransformsFromStorage converts stored transforms back to PolicyTransforms
func deserializeTransformsFromStorage(transformsData map[string]interface{}) *PolicyTransforms {
	if transformsData == nil {
		return nil
	}

	transforms := &PolicyTransforms{}

	if labelsData, ok := transformsData["labels"].(map[string]interface{}); ok {
		typeStr, _ := labelsData["type"].(string)
		transforms.Labels = &Transform{
			Type:  TransformOperationType(typeStr),
			Value: labelsData["value"],
		}
	}

	if annotationsData, ok := transformsData["annotations"].(map[string]interface{}); ok {
		typeStr, _ := annotationsData["type"].(string)
		transforms.Annotations = &Transform{
			Type:  TransformOperationType(typeStr),
			Value: annotationsData["value"],
		}
	}

	if specData, ok := transformsData["spec"].(map[string]interface{}); ok {
		typeStr, _ := specData["type"].(string)
		transforms.Spec = &Transform{
			Type:  TransformOperationType(typeStr),
			Value: specData["value"],
		}
	}

	return transforms
}

// computeCurrentGlobalPolicyHash discovers current global policies and computes their hash
// This is used for cache validation
// deepCopyAppSpec creates a deep copy of the Application spec for diff computation
func deepCopyAppSpec(spec *v1beta1.ApplicationSpec) (*v1beta1.ApplicationSpec, error) {
	// Marshal to JSON and unmarshal back to create a deep copy
	data, err := json.Marshal(spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal Application spec")
	}

	var copy v1beta1.ApplicationSpec
	if err := json.Unmarshal(data, &copy); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal Application spec")
	}

	return &copy, nil
}

// computeJSONPatch computes a JSON Merge Patch (RFC 7386) diff between two Application specs
// This is simpler than JSON Patch (RFC 6902) but sufficient for our observability needs
func computeJSONPatch(before, after *v1beta1.ApplicationSpec) ([]byte, error) {
	// Convert both specs to JSON
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal before spec")
	}

	afterJSON, err := json.Marshal(after)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal after spec")
	}

	// Create JSON Merge Patch (RFC 7386)
	// This shows what changed in a more human-readable format than RFC 6902
	patch, err := jsonpatch.CreateMergePatch(beforeJSON, afterJSON)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create merge patch")
	}

	return patch, nil
}

// createOrUpdateDiffsConfigMap creates or updates a ConfigMap containing all policy records
func createOrUpdateDiffsConfigMap(ctx context.Context, cli client.Client, app *v1beta1.Application, orderedData map[string]string) error {
	cmName := fmt.Sprintf("application-policies-%s-%s", app.Namespace, app.Name)

	// Create the ConfigMap
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: app.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					// Use hardcoded values since TypeMeta is cleared by k8s client after Create/Get
					APIVersion:         v1beta1.SchemeGroupVersion.String(),
					Kind:               v1beta1.ApplicationKind,
					Name:               app.Name,
					UID:                app.UID,
					Controller:         ptrBool(true),
					BlockOwnerDeletion: ptrBool(true),
				},
			},
		},
		Data: orderedData,
	}

	// Add standard KubeVela labels (following ResourceTracker pattern)
	meta.AddLabels(cm, map[string]string{
		oam.LabelAppName:                   app.Name,
		oam.LabelAppNamespace:              app.Namespace,
		oam.LabelAppUID:                    string(app.UID),
		"app.oam.dev/application-policies": "true", // Identify this as an application-policies ConfigMap
	})

	// Add annotations to track update time
	meta.AddAnnotations(cm, map[string]string{
		oam.AnnotationLastAppliedTime: time.Now().Format(time.RFC3339),
	})

	// Try to create the ConfigMap
	err := cli.Create(ctx, cm)
	if err != nil {
		// If it already exists, update it
		if client.IgnoreAlreadyExists(err) == nil {
			// Get existing ConfigMap
			existing := &corev1.ConfigMap{}
			if getErr := cli.Get(ctx, client.ObjectKey{Name: cmName, Namespace: app.Namespace}, existing); getErr != nil {
				return errors.Wrap(getErr, "failed to get existing ConfigMap")
			}

			// Update data and annotations
			existing.Data = orderedData
			meta.AddAnnotations(existing, map[string]string{
				oam.AnnotationLastAppliedTime: time.Now().Format(time.RFC3339),
			})
			if updateErr := cli.Update(ctx, existing); updateErr != nil {
				return errors.Wrap(updateErr, "failed to update ConfigMap")
			}
		} else {
			return errors.Wrap(err, "failed to create ConfigMap")
		}
	}

	return nil
}

// ptrBool returns a pointer to a bool value
func ptrBool(b bool) *bool {
	return &b
}

// cleanApplicationForPolicyContext removes server-generated fields from the Application
// before exposing it to policy templates via context.application.
// This ensures policies only see user-provided fields from the original manifest.

// Internal/system metadata prefixes to exclude from policy context
// Using a map for O(1) lookup instead of O(n) slice iteration
var internalMetadataPrefixes = map[string]struct{}{
	"app.oam.dev/":           {},
	"oam.dev/":               {},
	"kubectl.kubernetes.io/": {},
	"kubernetes.io/":         {},
	"k8s.io/":                {},
	"helm.sh/":               {},
	"app.kubernetes.io/":     {},
}

// filterUserMetadata filters out internal/system labels and annotations
// to prevent policies from accessing sensitive internal metadata.
// Returns a new map with only user metadata. Returns nil for empty results.
// Optimized for hot path - runs on every reconciliation with policies.
func filterUserMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}

	// Pre-allocate for common case where most metadata is user-provided
	filtered := make(map[string]string, len(metadata))

	for k, v := range metadata {
		// Extract prefix (everything before first "/" if present)
		// Most keys have prefixes, so check for "/" first
		slashIdx := strings.IndexByte(k, '/')
		if slashIdx > 0 {
			prefix := k[:slashIdx+1] // Include the trailing "/"
			if _, isInternal := internalMetadataPrefixes[prefix]; isInternal {
				continue // Skip internal metadata
			}
		}

		// Include user-provided metadata
		filtered[k] = v
	}

	// Return nil instead of empty map to avoid unnecessary allocations
	if len(filtered) == 0 {
		return nil
	}

	return filtered
}
