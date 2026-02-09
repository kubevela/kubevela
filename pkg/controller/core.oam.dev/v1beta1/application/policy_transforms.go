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
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/kubevela/pkg/cue/cuex"
	monitorContext "github.com/kubevela/pkg/monitor/context"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const (
	// SkipGlobalPoliciesAnnotation allows Applications to opt-out of global policies
	SkipGlobalPoliciesAnnotation = "policy.oam.dev/skip-global"
)

// ApplyApplicationScopeTransforms iterates through policies in the Application spec
// and applies transforms from any Application-scoped PolicyDefinitions.
// It first discovers and applies any global policies (if feature gate enabled),
// then applies explicit policies from the Application spec.
// This modifies the in-memory Application object before it's parsed into an AppFile.
// Returns the updated context with any additionalContext from policies.
func (h *AppHandler) ApplyApplicationScopeTransforms(ctx monitorContext.Context, app *v1beta1.Application) (monitorContext.Context, error) {
	// Clear previous global policy status
	app.Status.AppliedGlobalPolicies = nil

	// Step 1: Validate explicit policies are not global
	for _, policy := range app.Spec.Policies {
		if err := validateNotGlobalPolicy(ctx, h.Client, policy.Type, app.Namespace); err != nil {
			return ctx, errors.Wrapf(err, "invalid policy reference: %s", policy.Type)
		}
	}

	// Step 2: Handle global policies (if feature gate enabled and not opted out)
	var globalRenderedResults []RenderedPolicyResult
	allDiffs := make(map[string][]byte) // Track spec diffs: policy-name -> JSON patch
	sequence := 1                        // Track execution order

	if !shouldSkipGlobalPolicies(app) && utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableGlobalPolicies) {
		// Compute current global policy hash for cache validation
		currentGlobalPolicyHash, err := h.computeCurrentGlobalPolicyHash(ctx, app.Namespace)
		if err != nil {
			ctx.Info("Failed to compute global policy hash, proceeding without cache", "error", err)
			currentGlobalPolicyHash = ""
		}

		// Try cache first - this returns the RENDERED results, not PolicyDefinitions
		cachedResults, cacheHit, err := globalPolicyCache.Get(app, currentGlobalPolicyHash)
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
			if err := globalPolicyCache.Set(app, globalRenderedResults, currentGlobalPolicyHash); err != nil {
				ctx.Info("Failed to update cache", "error", err)
			} else {
				ctx.Info("Cached rendered global policy results", "count", len(globalRenderedResults))
			}
		}

		// Apply the rendered global policy results (either from cache or freshly rendered)
		for _, result := range globalRenderedResults {
			ctx.Info("Applying global policy result", "policy", result.PolicyName, "enabled", result.Enabled, "fromCache", cacheHit, "sequence", sequence)

			// Get priority from the result (stored during render)
			priority := result.Priority

			var diffBytes []byte
			ctx, diffBytes, err = h.applyRenderedPolicyResult(ctx, app, result, sequence, priority)
			if err != nil {
				return ctx, errors.Wrapf(err, "failed to apply global policy %s", result.PolicyName)
			}

			// Store diff if policy modified spec
			if diffBytes != nil && len(diffBytes) > 0 {
				allDiffs[result.PolicyName] = diffBytes
			}

			// Increment sequence only if policy was applied (enabled=true)
			if result.Enabled {
				sequence++
			}
		}
	} else if shouldSkipGlobalPolicies(app) {
		ctx.Info("Skipping global policies (opt-out annotation present)")
	} else {
		ctx.Info("Global policies feature is disabled (feature gate not enabled)")
	}

	// Step 3: Apply explicit policies from Application spec
	// These are NOT cached as they may have parameters specific to this Application
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

		ctx.Info("Applying explicit Application-scoped policy", "policy", policy.Type, "name", policy.Name)

		// Render and apply (not cached - explicit policies can have unique parameters)
		ctx, err = h.applyPolicyTransform(ctx, app, policy, templ.PolicyDefinition)
		if err != nil {
			return ctx, errors.Wrapf(err, "failed to apply transform from policy %s", policy.Type)
		}
	}

	// Step 4: Store all spec diffs in a single ConfigMap (if any policies modified spec)
	if len(allDiffs) > 0 {
		// Build ConfigMap data with sequence-prefixed keys
		orderedData := make(map[string]string)
		for _, appliedPolicy := range app.Status.AppliedGlobalPolicies {
			if diff, ok := allDiffs[appliedPolicy.Name]; ok {
				key := fmt.Sprintf("%03d-%s", appliedPolicy.Sequence, appliedPolicy.Name)
				orderedData[key] = string(diff)
			}
		}

		// Create/update ConfigMap
		if len(orderedData) > 0 {
			err := createOrUpdateDiffsConfigMap(ctx, h.Client, app, orderedData)
			if err != nil {
				ctx.Info("Failed to store policy diffs in ConfigMap", "error", err)
				// Don't fail reconciliation - observability is optional
			} else {
				app.Status.PolicyDiffsConfigMap = fmt.Sprintf("%s-policy-diffs", app.Name)
				ctx.Info("Stored policy diffs in ConfigMap", "configmap", app.Status.PolicyDiffsConfigMap, "count", len(orderedData))
			}
		}
	}

	return ctx, nil
}

// applyPolicyTransform renders the policy's CUE template and applies transforms to the Application.
// Returns the updated context with any additionalContext merged in.
func (h *AppHandler) applyPolicyTransform(ctx monitorContext.Context, app *v1beta1.Application, policyRef v1beta1.AppPolicy, policyDef *v1beta1.PolicyDefinition) (monitorContext.Context, error) {
	// Validate policy has CUE schematic
	if policyDef.Spec.Schematic == nil || policyDef.Spec.Schematic.CUE == nil {
		return ctx, errors.Errorf("Application-scoped policy %s must have a CUE schematic", policyDef.Name)
	}

	// Parse policy parameters
	var policyParams map[string]interface{}
	if policyRef.Properties != nil && len(policyRef.Properties.Raw) > 0 {
		if err := json.Unmarshal(policyRef.Properties.Raw, &policyParams); err != nil {
			return ctx, errors.Wrap(err, "failed to unmarshal policy parameters")
		}
	}

	// Render the CUE template with context.application
	rendered, err := h.renderPolicyCUETemplate(ctx, app, policyParams, policyDef)
	if err != nil {
		return ctx, errors.Wrap(err, "failed to render CUE template")
	}

	// Check if the transform should be applied (default: true)
	shouldApply, err := h.extractEnabled(rendered)
	if err != nil {
		return ctx, errors.Wrap(err, "failed to extract enabled")
	}

	if !shouldApply {
		ctx.Info("Skipping transform (enabled=false)", "policy", policyRef.Type)
		// Record in status if this is a global policy
		// Note: This should not happen for explicit policies as we validate earlier
		// that global policies cannot be explicitly referenced
		if policyDef.Spec.Global {
			recordGlobalPolicyStatus(app, policyRef.Name, policyDef.Namespace, 0, policyDef.Spec.Priority, false, "enabled=false", nil)
		}
		return ctx, nil
	}

	// Extract transforms field
	transforms, err := h.extractTransforms(rendered)
	if err != nil {
		return ctx, errors.Wrap(err, "failed to extract transforms")
	}

	// Apply transforms to the in-memory Application
	if transforms != nil {
		if err := h.applyTransformsToApplication(ctx, app, transforms); err != nil {
			return ctx, errors.Wrap(err, "failed to apply transforms")
		}
		ctx.Info("Applied transforms to Application", "policy", policyRef.Type)
	}

	// Extract and store additionalContext
	additionalContext, err := h.extractAdditionalContext(rendered)
	if err != nil {
		return ctx, errors.Wrap(err, "failed to extract additionalContext")
	}

	if additionalContext != nil {
		ctx = storeAdditionalContextInCtx(ctx, additionalContext)
		ctx.Info("Stored additionalContext in context", "policy", policyRef.Type, "keys", len(additionalContext))
	}

	ctx.Info("Successfully applied transform", "policy", policyRef.Type)
	return ctx, nil
}

// renderPolicy renders a policy's CUE template and extracts the results for caching
// Returns a RenderedPolicyResult that can be cached and reused
func (h *AppHandler) renderPolicy(ctx monitorContext.Context, app *v1beta1.Application, policyRef v1beta1.AppPolicy, policyDef *v1beta1.PolicyDefinition) (RenderedPolicyResult, error) {
	result := RenderedPolicyResult{
		PolicyName:      policyDef.Name,
		PolicyNamespace: policyDef.Namespace,
		Priority:        policyDef.Spec.Priority,
		Enabled:         false,
	}

	// Validate policy has CUE schematic
	if policyDef.Spec.Schematic == nil || policyDef.Spec.Schematic.CUE == nil {
		result.SkipReason = "no CUE schematic"
		return result, errors.Errorf("Application-scoped policy %s must have a CUE schematic", policyDef.Name)
	}

	// Parse policy parameters
	var policyParams map[string]interface{}
	if policyRef.Properties != nil && len(policyRef.Properties.Raw) > 0 {
		if err := json.Unmarshal(policyRef.Properties.Raw, &policyParams); err != nil {
			result.SkipReason = fmt.Sprintf("parameter unmarshal error: %s", err.Error())
			return result, errors.Wrap(err, "failed to unmarshal policy parameters")
		}
	}

	// Render the CUE template with context.application
	rendered, err := h.renderPolicyCUETemplate(ctx, app, policyParams, policyDef)
	if err != nil {
		result.SkipReason = fmt.Sprintf("CUE render error: %s", err.Error())
		return result, errors.Wrap(err, "failed to render CUE template")
	}

	// Extract enabled field (default: true)
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

	// Extract transforms field
	transforms, err := h.extractTransforms(rendered)
	if err != nil {
		result.SkipReason = fmt.Sprintf("transforms extraction error: %s", err.Error())
		return result, errors.Wrap(err, "failed to extract transforms")
	}
	result.Transforms = transforms

	// Extract additionalContext
	additionalContext, err := h.extractAdditionalContext(rendered)
	if err != nil {
		result.SkipReason = fmt.Sprintf("additionalContext extraction error: %s", err.Error())
		return result, errors.Wrap(err, "failed to extract additionalContext")
	}
	result.AdditionalContext = additionalContext

	return result, nil
}

// applyRenderedPolicyResult applies a cached/rendered policy result to the Application
// This skips all the expensive CUE rendering and just applies the pre-computed transforms
// Returns the diff bytes if spec was modified
func (h *AppHandler) applyRenderedPolicyResult(ctx monitorContext.Context, app *v1beta1.Application, result RenderedPolicyResult, sequence int, priority int32) (monitorContext.Context, []byte, error) {
	if !result.Enabled {
		ctx.Info("Skipping policy (from cache)", "policy", result.PolicyName, "reason", result.SkipReason)
		recordGlobalPolicyStatus(app, result.PolicyName, result.PolicyNamespace, sequence, priority, false, result.SkipReason, nil)
		return ctx, nil, nil
	}

	// Track what changes we're making
	changes := &PolicyChanges{
		AdditionalContext: result.AdditionalContext,
	}

	// Take snapshot of spec BEFORE applying transforms (for diff tracking)
	specBefore, err := deepCopyAppSpec(&app.Spec)
	if err != nil {
		ctx.Info("Failed to copy spec for diff tracking", "error", err)
		specBefore = nil // Continue without diff tracking
	}

	var diffBytes []byte

	// Apply transforms to the in-memory Application
	if result.Transforms != nil {
		// Cast from interface{} back to *PolicyTransforms
		transforms, ok := result.Transforms.(*PolicyTransforms)
		if !ok {
			return ctx, nil, errors.Errorf("cached transforms have invalid type for policy %s", result.PolicyName)
		}

		// Capture what labels/annotations/spec will be changed
		if transforms.Labels != nil && transforms.Labels.Value != nil {
			if labelMap, ok := transforms.Labels.Value.(map[string]interface{}); ok {
				changes.AddedLabels = make(map[string]string)
				for k, v := range labelMap {
					if str, ok := v.(string); ok {
						changes.AddedLabels[k] = str
					}
				}
			}
		}
		if transforms.Annotations != nil && transforms.Annotations.Value != nil {
			if annotationMap, ok := transforms.Annotations.Value.(map[string]interface{}); ok {
				changes.AddedAnnotations = make(map[string]string)
				for k, v := range annotationMap {
					if str, ok := v.(string); ok {
						changes.AddedAnnotations[k] = str
					}
				}
			}
		}
		if transforms.Spec != nil {
			changes.SpecModified = true
		}

		if err := h.applyTransformsToApplication(ctx, app, transforms); err != nil {
			return ctx, nil, errors.Wrap(err, "failed to apply transforms")
		}
		ctx.Info("Applied cached transforms to Application", "policy", result.PolicyName)

		// Compute diff if spec was modified
		if specBefore != nil && changes.SpecModified {
			// Check if spec actually changed
			if !reflect.DeepEqual(specBefore, &app.Spec) {
				diff, diffErr := computeJSONPatch(specBefore, &app.Spec)
				if diffErr != nil {
					ctx.Info("Failed to compute diff", "policy", result.PolicyName, "error", diffErr)
				} else {
					diffBytes = diff
					ctx.Info("Computed spec diff", "policy", result.PolicyName, "size", len(diff))
				}
			}
		}
	}

	// Store additionalContext in context
	if result.AdditionalContext != nil {
		ctx = storeAdditionalContextInCtx(ctx, result.AdditionalContext)
		ctx.Info("Stored cached additionalContext in context", "policy", result.PolicyName, "keys", len(result.AdditionalContext))
	}

	recordGlobalPolicyStatus(app, result.PolicyName, result.PolicyNamespace, sequence, priority, true, "", changes)
	ctx.Info("Successfully applied cached policy result", "policy", result.PolicyName)
	return ctx, diffBytes, nil
}

// policyTransformSchema provides type-safe schema for Application-scoped policy transforms
const policyTransformSchema = `
parameter: {[string]: _}
enabled: *true | bool
transforms?: {
	spec?: {
		type: "replace" | "merge"
		value: {...}
	}
	labels?: {
		type: "merge"
		value: {[string]: string}
	}
	annotations?: {
		type: "merge"
		value: {[string]: string}
	}
}
additionalContext?: {...}
context: {
	application: {...}
}
`

// renderPolicyCUETemplate renders the policy CUE template with parameter and context.application
func (h *AppHandler) renderPolicyCUETemplate(ctx monitorContext.Context, app *v1beta1.Application, params map[string]interface{}, policyDef *v1beta1.PolicyDefinition) (cue.Value, error) {
	// Build CUE source with parameter and context
	var cueSources []string

	// Add type safety schema
	cueSources = append(cueSources, policyTransformSchema)

	// Add the policy template
	cueSources = append(cueSources, policyDef.Spec.Schematic.CUE.Template)

	// Add parameter
	if params != nil {
		paramJSON, err := json.Marshal(params)
		if err != nil {
			return cue.Value{}, errors.Wrap(err, "failed to marshal parameters")
		}
		cueSources = append(cueSources, fmt.Sprintf("parameter: %s", string(paramJSON)))
	} else {
		cueSources = append(cueSources, "parameter: {}")
	}

	// Add context.application (convert Application to JSON)
	appJSON, err := json.Marshal(app)
	if err != nil {
		return cue.Value{}, errors.Wrap(err, "failed to marshal Application")
	}
	cueSources = append(cueSources, fmt.Sprintf("context: application: %s", string(appJSON)))

	// Compile the CUE using the default CueX compiler
	cueSource := strings.Join(cueSources, "\n")
	val, err := cuex.DefaultCompiler.Get().CompileString(ctx.GetContext(), cueSource)

	if err != nil {
		return cue.Value{}, errors.Wrap(err, "failed to compile CUE template")
	}

	// Validate
	if err := val.Validate(); err != nil {
		return cue.Value{}, errors.Wrap(err, "CUE validation failed")
	}

	return val, nil
}

// extractEnabled extracts the enabled field from rendered CUE (defaults to true)
func (h *AppHandler) extractEnabled(val cue.Value) (bool, error) {
	enabledVal := val.LookupPath(cue.ParsePath("enabled"))
	if !enabledVal.Exists() {
		// No enabled field, default to true
		return true, nil
	}

	enabled, err := enabledVal.Bool()
	if err != nil {
		return false, errors.Wrap(err, "failed to decode enabled (must be boolean)")
	}

	return enabled, nil
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

// PolicyTransforms represents the allowed transformation operations
type PolicyTransforms struct {
	Spec        *Transform `json:"spec,omitempty"`
	Labels      *Transform `json:"labels,omitempty"`
	Annotations *Transform `json:"annotations,omitempty"`
}

// extractTransforms extracts the transforms field from rendered CUE
// Only spec, labels, and annotations with type+value structure are permitted
func (h *AppHandler) extractTransforms(val cue.Value) (*PolicyTransforms, error) {
	transformsVal := val.LookupPath(cue.ParsePath("transforms"))
	if !transformsVal.Exists() {
		// No transforms field, that's OK
		return nil, nil
	}

	var transforms PolicyTransforms
	if err := transformsVal.Decode(&transforms); err != nil {
		return nil, errors.Wrap(err, "failed to decode transforms")
	}

	// Validate structure: only 'spec', 'labels', and 'annotations' are allowed
	iter, err := transformsVal.Fields()
	if err != nil {
		return nil, errors.Wrap(err, "failed to iterate transforms fields")
	}

	allowedFields := map[string]bool{
		"spec":        true,
		"labels":      true,
		"annotations": true,
	}

	for iter.Next() {
		fieldName := iter.Selector().String()
		if !allowedFields[fieldName] {
			return nil, errors.Errorf("transforms.%s is not allowed; only 'spec', 'labels', and 'annotations' are permitted", fieldName)
		}
	}

	// Validate each transform has correct type
	if transforms.Spec != nil {
		if err := validateTransformType(transforms.Spec.Type, "spec", true); err != nil {
			return nil, err
		}
	}
	if transforms.Labels != nil {
		// Labels only support merge for safety
		if err := validateTransformType(transforms.Labels.Type, "labels", false); err != nil {
			return nil, err
		}
	}
	if transforms.Annotations != nil {
		// Annotations only support merge for safety
		if err := validateTransformType(transforms.Annotations.Type, "annotations", false); err != nil {
			return nil, err
		}
	}

	return &transforms, nil
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

	// Store back in context using the PolicyAdditionalContextKey from application_controller.go
	// We need to extract the underlying context.Context, add our value, and wrap it back
	baseCtx := context.WithValue(ctx.GetContext(), PolicyAdditionalContextKey, merged)
	ctx.SetContext(baseCtx)
	return ctx
}

// getAdditionalContextFromCtx retrieves additional policy context from the Go context
func getAdditionalContextFromCtx(ctx monitorContext.Context) map[string]interface{} {
	if val := ctx.GetContext().Value(PolicyAdditionalContextKey); val != nil {
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

// recordGlobalPolicyStatus records the application status of a global policy
func recordGlobalPolicyStatus(app *v1beta1.Application, policyName, policyNamespace string, sequence int, priority int32, applied bool, reason string, changes *PolicyChanges) {
	entry := common.AppliedGlobalPolicy{
		Name:      policyName,
		Namespace: policyNamespace,
		Applied:   applied,
		Reason:    reason,
		Sequence:  sequence,
		Priority:  priority,
	}

	// Record what was changed (if policy was applied)
	if applied && changes != nil {
		entry.AddedLabels = changes.AddedLabels
		entry.AddedAnnotations = changes.AddedAnnotations
		entry.AdditionalContext = changes.AdditionalContext
		entry.SpecModified = changes.SpecModified
	}

	app.Status.AppliedGlobalPolicies = append(app.Status.AppliedGlobalPolicies, entry)
}

// PolicyChanges tracks what a policy modified
type PolicyChanges struct {
	AddedLabels       map[string]string
	AddedAnnotations  map[string]string
	AdditionalContext map[string]interface{}
	SpecModified      bool
}

// computeCurrentGlobalPolicyHash discovers current global policies and computes their hash
// This is used for cache validation
func (h *AppHandler) computeCurrentGlobalPolicyHash(ctx monitorContext.Context, appNamespace string) (string, error) {
	var allPolicies []v1beta1.PolicyDefinition

	// Get policies from vela-system
	velaSystemPolicies, err := discoverGlobalPolicies(ctx, h.Client, oam.SystemDefinitionNamespace)
	if err != nil {
		return "", errors.Wrap(err, "failed to discover vela-system policies for hash")
	}
	allPolicies = append(allPolicies, velaSystemPolicies...)

	// Get policies from app namespace (if different)
	if appNamespace != oam.SystemDefinitionNamespace {
		namespacePolicies, err := discoverGlobalPolicies(ctx, h.Client, appNamespace)
		if err != nil {
			return "", errors.Wrap(err, "failed to discover namespace policies for hash")
		}
		allPolicies = append(allPolicies, namespacePolicies...)
	}

	return computeGlobalPolicyHash(allPolicies)
}

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

// createOrUpdateDiffsConfigMap creates or updates a ConfigMap containing all policy diffs
func createOrUpdateDiffsConfigMap(ctx context.Context, cli client.Client, app *v1beta1.Application, orderedData map[string]string) error {
	cmName := fmt.Sprintf("%s-policy-diffs", app.Name)

	// Create the ConfigMap
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: app.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         app.APIVersion,
					Kind:               app.Kind,
					Name:               app.Name,
					UID:                app.UID,
					Controller:         ptrBool(true),
					BlockOwnerDeletion: ptrBool(true),
				},
			},
		},
		Data: orderedData,
	}

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

			// Update data
			existing.Data = orderedData
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
