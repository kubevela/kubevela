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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/kubevela/pkg/cue/cuex"
	jsonpatch "gomodules.xyz/jsonpatch/v2"
	monitorContext "github.com/kubevela/pkg/monitor/context"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"
)

// SkipGlobalPoliciesAnnotation is an alias for oam.AnnotationSkipGlobalPolicies.
const SkipGlobalPoliciesAnnotation = oam.AnnotationSkipGlobalPolicies

// annotationValueTrue is the canonical "true" string used in annotation comparisons.
const annotationValueTrue = "true"

// ApplyApplicationScopeTransforms applies Application-scoped policy transforms to the in-memory
// Application before it is parsed into an AppFile. Global policies (vela-system) are applied first,
// then explicit spec.policies, in priority order. Results are cached with a 1-minute TTL and written
// to a ConfigMap for observability (backing store for `vela policy show`).
func (h *AppHandler) ApplyApplicationScopeTransforms(ctx monitorContext.Context, app *v1beta1.Application) (monitorContext.Context, error) {
	app.Status.AppliedApplicationPolicies = nil

	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableApplicationScopedPolicies) {
		ctx.Info("Application-scoped policies disabled by feature gate")
		return ctx, nil
	}

	for _, policy := range app.Spec.Policies {
		if err := validateNotGlobalPolicy(ctx, h.Client, policy.Type); err != nil {
			return ctx, errors.Wrapf(err, "invalid policy reference: %s", policy.Type)
		}
	}

	var previousAppRev *v1beta1.ApplicationRevision
	if app.Status.LatestRevision != nil && app.Status.LatestRevision.Name != "" {
		previousAppRev = &v1beta1.ApplicationRevision{}
		err := h.Client.Get(ctx, client.ObjectKey{
			Name:      app.Status.LatestRevision.Name,
			Namespace: app.Namespace,
		}, previousAppRev)
		if err != nil {
			ctx.Info("Failed to fetch previous ApplicationRevision, will treat as new Application",
				"revisionName", app.Status.LatestRevision.Name,
				"error", err)
			previousAppRev = nil
		}
	}

	cachedResults, cacheHit, cacheMissReason, err := applicationPolicyCache.GetWithReason(app)
	var renderedResults []RenderedPolicyResult

	if err != nil {
		ctx.Info("Cache error, will render policies", "error", err)
		cacheHit = false
		cacheMissReason = "error"
	}

	if cacheHit {
		klog.V(4).InfoS("Cache HIT - using cached policy results", "count", len(cachedResults))
		renderedResults = cachedResults

		// Restore handler maps so PrepareCurrentAppRevision can include PolicyDefinitions
		// in the ApplicationRevision even on a cache hit.
		if len(cachedResults) > 0 {
			h.applicationScopedPolicyDefs = make(map[string]*v1beta1.PolicyDefinition)
			h.policyVersions = make(map[string]v1beta1.PolicyVersionMetadata)
			for _, result := range cachedResults {
				if result.PolicyDefinitionUsed != nil {
					h.applicationScopedPolicyDefs[result.PolicyDefinitionUsed.Name] = result.PolicyDefinitionUsed
				}
				if result.DefinitionRevisionName != "" {
					key := result.VersionKey
					if key == "" {
						key = result.PolicyName
					}
					h.policyVersions[key] = v1beta1.PolicyVersionMetadata{
						DefinitionRevisionName: result.DefinitionRevisionName,
						Revision:               result.Revision,
						RevisionHash:           result.RevisionHash,
					}
				}
			}
		}
	} else {
		// Ordering contract for h.isNewRevision (dual ownership):
		// ApplyApplicationScopeTransforms runs BEFORE PrepareCurrentAppRevision.
		// We pre-compute it here so renderAllPolicies chooses the correct path
		// (fresh cluster lookup vs stored version-pinned PolicyDefinitions).
		// PrepareCurrentAppRevision independently recomputes it via currentAppRevIsNew(),
		// which is intentional — that authoritative check may also catch non-spec triggers
		// (e.g., PublishVersion bump). The only interaction: if policy transforms change
		// app.Spec and autoRevision=true, the next reconcile will see a changed spec and
		// set isNewRevision=true, which is the desired behaviour.
		if previousAppRev != nil {
			prevSpecHash, _ := apply.ComputeSpecHash(previousAppRev.Spec.Application.Spec)
			currSpecHash, _ := apply.ComputeSpecHash(app.Spec)
			if prevSpecHash != currSpecHash {
				h.isNewRevision = true
			}
		} else {
			h.isNewRevision = true // first reconcile
		}
		klog.V(4).InfoS("Cache MISS - rendering all policies", "reason", cacheMissReason, "isNewRevision", h.isNewRevision)
		var renderErr error
		renderedResults, renderErr = h.renderAllPolicies(ctx, app, previousAppRev)
		if renderErr != nil {
			return ctx, renderErr
		}
		if err := applicationPolicyCache.Set(app, renderedResults); err != nil {
			klog.V(4).InfoS("Failed to cache policy results", "error", err)
		}
		// Dry-run capture: if a dryRunCapture was placed in the context, stash results.
		if cap, ok := ctx.GetContext().Value(dryRunCaptureKey).(*dryRunCapture); ok && cap != nil {
			cap.results = renderedResults
		}
	}

	renderedSpec := extractRenderedSpec(renderedResults)
	renderedMetadata := extractRenderedMetadata(renderedResults)

	applyMetadataToApp(app, renderedMetadata)

	if policyContext, ok := renderedMetadata["context"].(map[string]interface{}); ok && len(policyContext) > 0 {
		ctx = storeAdditionalContextInCtx(ctx, policyContext)
	}

	// For observability we always render, but on subsequent reconciliations we restore from
	// the ApplicationRevision to avoid creating spurious new revisions.
	hasSpecChanges := len(renderedSpec.Components) > 0 || renderedSpec.Workflow != nil || len(renderedSpec.Policies) > 0
	autoRevision := shouldAutoCreateRevision(app)
	isFirstRevision := app.Status.LatestRevision == nil || app.Status.LatestRevision.Name == ""

	if hasSpecChanges {
		// Apply rendered spec to Application (in-memory only)
		applySpecToApp(app, renderedSpec)

		if !isFirstRevision && !autoRevision {
			latestRev := &v1beta1.ApplicationRevision{}
			revName := app.Status.LatestRevision.Name
			err := h.Client.Get(ctx, client.ObjectKey{
				Name:      revName,
				Namespace: app.Namespace,
			}, latestRev)

			if err == nil && latestRev.Spec.Application.Spec.Components != nil {
				app.Spec.Components = latestRev.Spec.Application.Spec.Components
				if latestRev.Spec.Application.Spec.Workflow != nil {
					app.Spec.Workflow = latestRev.Spec.Application.Spec.Workflow
				}
				if len(latestRev.Spec.Application.Spec.Policies) > 0 {
					app.Spec.Policies = latestRev.Spec.Application.Spec.Policies
				}
				ctx.Info("Restored spec from ApplicationRevision (autoRevision disabled)", "revision", revName)
			} else if err != nil {
				ctx.Error(err, "Failed to load ApplicationRevision, using rendered spec", "revision", revName)
			}
		}
	}

	recordPolicyStatuses(app, renderedResults)

	// Write results to ConfigMap for `vela policy show`.
	if len(renderedResults) > 0 {
		h.writePolicyObservabilityConfigMap(ctx, app, renderedResults, renderedSpec, renderedMetadata, autoRevision, cacheHit)
	}

	ctx.Info("Policy transforms completed",
		"total", len(renderedResults),
		"enabled", countAppliedPolicies(app.Status.AppliedApplicationPolicies),
		"autoRevision", autoRevision)

	return ctx, nil
}

// countAppliedPolicies returns the number of policies with Applied=true.
func countAppliedPolicies(policies []common.AppliedApplicationPolicy) int {
	count := 0
	for _, p := range policies {
		if p.Applied {
			count++
		}
	}
	return count
}

// recordPolicyStatuses appends applied/skipped entries to app.Status.AppliedApplicationPolicies.
func recordPolicyStatuses(app *v1beta1.Application, results []RenderedPolicyResult) {
	for _, result := range results {
		if result.Enabled {
			specModified := false
			labelsCount := 0
			annotationsCount := 0

			if policyOutput, ok := result.Transforms.(*PolicyOutput); ok {
				specModified = len(policyOutput.Components) > 0 || policyOutput.Workflow != nil || len(policyOutput.Policies) > 0
				labelsCount = len(policyOutput.Labels)
				annotationsCount = len(policyOutput.Annotations)
			}

			app.Status.AppliedApplicationPolicies = append(app.Status.AppliedApplicationPolicies, common.AppliedApplicationPolicy{
				Name:                   result.PolicyName,
				Type:                   result.PolicyType,
				Namespace:              result.PolicyNamespace,
				Applied:                true,
				Source:                 result.Source,
				SpecModified:           specModified,
				LabelsCount:            labelsCount,
				AnnotationsCount:       annotationsCount,
				HasContext:             len(result.AdditionalContext) > 0,
				DefinitionRevisionName: result.DefinitionRevisionName,
				Revision:               result.Revision,
				RevisionHash:           result.RevisionHash,
			})
		} else if result.PolicyName != "" {
			app.Status.AppliedApplicationPolicies = append(app.Status.AppliedApplicationPolicies, common.AppliedApplicationPolicy{
				Name:                   result.PolicyName,
				Type:                   result.PolicyType,
				Namespace:              result.PolicyNamespace,
				Applied:                false,
				Error:                  result.IsError,
				Message:                result.SkipReason,
				Source:                 result.Source,
				DefinitionRevisionName: result.DefinitionRevisionName,
				Revision:               result.Revision,
				RevisionHash:           result.RevisionHash,
			})
		}
	}
}

// writePolicyObservabilityConfigMap persists rendered policy data to a ConfigMap for `vela policy show`.
func (h *AppHandler) writePolicyObservabilityConfigMap(ctx monitorContext.Context, app *v1beta1.Application, results []RenderedPolicyResult, renderedSpec *v1beta1.ApplicationSpec, renderedMetadata map[string]interface{}, autoRevision, cacheHit bool) {
	configMapData := make(map[string]string)

	if renderedSpecJSON, err := json.MarshalIndent(renderedSpec, "", "  "); err == nil {
		configMapData["rendered_spec"] = string(renderedSpecJSON)
	}
	if appliedSpecJSON, err := json.MarshalIndent(app.Spec, "", "  "); err == nil {
		configMapData["applied_spec"] = string(appliedSpecJSON)
	}
	if metadataJSON, err := json.MarshalIndent(renderedMetadata, "", "  "); err == nil {
		configMapData["metadata"] = string(metadataJSON)
	}

	sequence := 1
	for _, result := range results {
		if !result.Enabled {
			continue
		}
		policyData := buildPolicyObservabilityData(result)
		if policyJSON, err := json.MarshalIndent(policyData, "", "  "); err == nil {
			configMapData[fmt.Sprintf("%03d-%s", sequence, result.PolicyName)] = string(policyJSON)
			sequence++
		}
	}

	appHash, _ := computeApplicationSpecHash(app)
	infoData := map[string]interface{}{
		"rendered_at":      time.Now().Format(time.RFC3339),
		"auto_revision":    autoRevision,
		"total_policies":   len(results),
		"enabled_policies": countAppliedPolicies(app.Status.AppliedApplicationPolicies),
		"cache_hit":        cacheHit,
		"application_hash": appHash,
	}
	if infoJSON, err := json.MarshalIndent(infoData, "", "  "); err == nil {
		configMapData["info"] = string(infoJSON)
	}

	if len(configMapData) > 0 {
		if err := createOrUpdateDiffsConfigMap(ctx, h.Client, app, configMapData); err != nil {
			ctx.Info("Failed to store policy ConfigMap", "error", err)
		} else {
			app.Status.ApplicationPoliciesConfigMap = policyConfigMapName(app.Namespace, app.Name)
		}
	}
}

// buildPolicyObservabilityData constructs the per-policy data map written to the observability ConfigMap.
func buildPolicyObservabilityData(result RenderedPolicyResult) map[string]interface{} {
	policyData := map[string]interface{}{
		"name":      result.PolicyName,
		"namespace": result.PolicyNamespace,
		"enabled":   result.Enabled,
		"priority":  result.Priority,
	}
	if result.DefinitionRevisionName != "" {
		policyData["definitionRevisionName"] = result.DefinitionRevisionName
	}
	if result.Revision > 0 {
		policyData["revision"] = result.Revision
	}
	if result.RevisionHash != "" {
		policyData["revisionHash"] = result.RevisionHash
	}

	if result.Transforms != nil {
		if policyOutput, ok := result.Transforms.(*PolicyOutput); ok {
			output := make(map[string]interface{})
			if len(policyOutput.Components) > 0 {
				output["components"] = policyOutput.Components
			}
			if policyOutput.Workflow != nil {
				output["workflow"] = policyOutput.Workflow
			}
			if len(policyOutput.Policies) > 0 {
				output["policies"] = policyOutput.Policies
			}
			if len(policyOutput.Labels) > 0 {
				output["labels"] = policyOutput.Labels
			}
			if len(policyOutput.Annotations) > 0 {
				output["annotations"] = policyOutput.Annotations
			}
			if len(policyOutput.Ctx) > 0 {
				output["ctx"] = policyOutput.Ctx
			}
			if result.SpecBefore != nil && result.SpecAfter != nil {
				specBeforeJSON, beforeErr := json.Marshal(result.SpecBefore)
				specAfterJSON, afterErr := json.Marshal(result.SpecAfter)
				if beforeErr == nil && afterErr == nil {
					specBlock := map[string]interface{}{
						"before": json.RawMessage(specBeforeJSON),
						"after":  json.RawMessage(specAfterJSON),
					}
					if ops, err := jsonpatch.CreatePatch(specBeforeJSON, specAfterJSON); err == nil && len(ops) > 0 {
						specBlock["diff"] = ops
					}
					output["spec"] = specBlock
				}
			}
			if len(output) > 0 {
				policyData["output"] = output
			}
		}
	}
	return policyData
}

// UpdateApplicationMetadata persists policy-applied labels and annotations to the Application.
// Uses MergePatch to avoid clobbering concurrent spec changes. Safe because ApplicationRevision
// hashes only cover Application.Spec, not metadata.
func (h *AppHandler) UpdateApplicationMetadata(ctx monitorContext.Context, app *v1beta1.Application) error {
	hasPolicyMetadata := false
	for _, policy := range app.Status.AppliedApplicationPolicies {
		if policy.LabelsCount > 0 || policy.AnnotationsCount > 0 {
			hasPolicyMetadata = true
			break
		}
	}
	if !hasPolicyMetadata {
		return nil
	}

	currentApp := &v1beta1.Application{}
	if err := h.Client.Get(ctx, client.ObjectKey{Name: app.Name, Namespace: app.Namespace}, currentApp); err != nil {
		return errors.Wrap(err, "failed to fetch current Application for metadata update")
	}

	base := currentApp.DeepCopy()
	hasActualChanges := false

	if len(app.Labels) > 0 {
		if currentApp.Labels == nil {
			currentApp.Labels = make(map[string]string)
		}
		for k, v := range app.Labels {
			if existingValue, exists := currentApp.Labels[k]; !exists || existingValue != v {
				currentApp.Labels[k] = v
				hasActualChanges = true
			}
		}
	}

	if len(app.Annotations) > 0 {
		if currentApp.Annotations == nil {
			currentApp.Annotations = make(map[string]string)
		}
		for k, v := range app.Annotations {
			if existingValue, exists := currentApp.Annotations[k]; !exists || existingValue != v {
				currentApp.Annotations[k] = v
				hasActualChanges = true
			}
		}
	}

	if !hasActualChanges {
		return nil
	}

	if err := h.Client.Patch(ctx, currentApp, client.MergeFrom(base)); err != nil {
		return errors.Wrap(err, "failed to patch Application metadata")
	}
	app.ResourceVersion = currentApp.ResourceVersion
	return nil
}

// renderPolicy renders a policy's CUE template and returns a cacheable result.
func (h *AppHandler) renderPolicy(ctx monitorContext.Context, app *v1beta1.Application, policyRef v1beta1.AppPolicy, policyDef *v1beta1.PolicyDefinition, versionMetadata *v1beta1.PolicyVersionMetadata) (RenderedPolicyResult, error) {
	result := RenderedPolicyResult{
		PolicyName:      policyRef.Name,
		PolicyType:      policyRef.Type,
		PolicyNamespace: policyDef.Namespace,
		Priority:        policyDef.Spec.Priority,
		Enabled:         false,
	}

	var policyParams map[string]interface{}
	if policyRef.Properties != nil && len(policyRef.Properties.Raw) > 0 {
		if err := json.Unmarshal(policyRef.Properties.Raw, &policyParams); err != nil {
			result.SkipReason = fmt.Sprintf("parameter unmarshal error: %s", err.Error())
			result.IsError = true
			return result, errors.Wrap(err, "failed to unmarshal policy parameters")
		}
	}

	if versionMetadata == nil {
		versionMetadata = &v1beta1.PolicyVersionMetadata{}
	}

	rendered, err := h.renderPolicyCUETemplate(ctx, app, policyParams, policyDef, policyRef.Name, policyRef.Type, versionMetadata)
	if err != nil {
		result.SkipReason = fmt.Sprintf("CUE render error: %s", err.Error())
		result.IsError = true
		return result, errors.Wrap(err, "failed to render CUE template")
	}

	enabled, err := h.extractEnabled(rendered)
	if err != nil {
		result.SkipReason = fmt.Sprintf("enabled extraction error: %s", err.Error())
		result.IsError = true
		return result, errors.Wrap(err, "failed to extract enabled")
	}

	result.Enabled = enabled
	if !enabled {
		result.SkipReason = "enabled=false"
		return result, nil
	}

	output, err := h.extractOutput(rendered)
	if err != nil {
		result.SkipReason = fmt.Sprintf("output extraction error: %s", err.Error())
		result.IsError = true
		return result, errors.Wrap(err, "failed to extract output")
	}
	if output == nil {
		result.SkipReason = "missing output field"
		result.IsError = true
		return result, errors.New("policy must specify 'output' field")
	}

	result.Transforms = output
	result.AdditionalContext = output.Ctx
	return result, nil
}

// renderPolicyCUETemplate compiles and executes the policy CUE template.
// Injects context (appName, namespace, components, policy version metadata) and parameters
// via a process.Context, enabling CueX providers (kube.#Read, etc.).
func (h *AppHandler) renderPolicyCUETemplate(ctx monitorContext.Context, app *v1beta1.Application, params map[string]interface{}, policyDef *v1beta1.PolicyDefinition, policyName, policyType string, versionMetadata *v1beta1.PolicyVersionMetadata) (cue.Value, error) {
	runtimeCtx := oamprovidertypes.WithRuntimeParams(ctx.GetContext(), oamprovidertypes.RuntimeParams{
		KubeClient: h.Client,
		ConfigFactory: config.NewConfigFactoryWithDispatcher(h.Client, func(goCtx context.Context, resources []*unstructured.Unstructured, applyOptions []apply.ApplyOption) error {
			return nil // policies don't dispatch resources directly
		}),
	})

	var appRevisionName string
	if h.currentAppRev != nil {
		appRevisionName = h.currentAppRev.Name
	}

	var revisionName string
	var revision int64
	var revisionHash string
	if versionMetadata != nil {
		revisionName = versionMetadata.DefinitionRevisionName
		revision = versionMetadata.Revision
		revisionHash = versionMetadata.RevisionHash
	}

	pCtx := velaprocess.NewContext(velaprocess.ContextData{
		Namespace:       app.Namespace,
		AppName:         app.Name,
		CompName:        app.Name,
		AppRevisionName: appRevisionName,
		AppLabels:       oam.FilterInternalMetadata(app.Labels),
		AppAnnotations:  oam.FilterInternalMetadata(app.Annotations),
		Ctx:             runtimeCtx,
	})
	pCtx.PushData(velaprocess.ContextAppComponents, app.Spec.Components)
	pCtx.PushData(velaprocess.ContextAppWorkflow, app.Spec.Workflow)
	pCtx.PushData(velaprocess.ContextAppPolicies, app.Spec.Policies)
	pCtx.PushData(velaprocess.ContextPolicyName, policyName)
	pCtx.PushData(velaprocess.ContextPolicyType, policyType)
	pCtx.PushData(velaprocess.ContextPolicyRevisionName, revisionName)
	pCtx.PushData(velaprocess.ContextPolicyRevision, revision)
	pCtx.PushData(velaprocess.ContextPolicyRevisionHash, revisionHash)

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

	baseContext, err := pCtx.BaseContextFile()
	if err != nil {
		return cue.Value{}, errors.Wrap(err, "failed to generate base context")
	}

	cueSource := strings.Join([]string{
		policyDef.Spec.Schematic.CUE.Template,
		paramFile,
		baseContext,
	}, "\n")

	val, err := cuex.DefaultCompiler.Get().CompileString(pCtx.GetCtx(), cueSource)
	if err != nil {
		return cue.Value{}, errors.Wrap(err, "failed to compile CUE template")
	}
	if err := val.Validate(); err != nil {
		return cue.Value{}, errors.Wrap(err, "CUE validation failed")
	}

	return val, nil
}

// extractEnabled reads the enabled flag from rendered CUE, defaulting to true.
// Checks config.enabled first, then root-level enabled for backwards compatibility.
func (h *AppHandler) extractEnabled(val cue.Value) (bool, error) {
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
		return true, nil
	}

	enabledVal := val.LookupPath(cue.ParsePath("enabled"))
	if !enabledVal.Exists() {
		return true, nil
	}
	return enabledVal.Bool()
}

// TransformOperationType defines the type of operation for a transform.
type TransformOperationType string

const (
	TransformReplace TransformOperationType = "replace"
	TransformMerge   TransformOperationType = "merge"
)

// Transform represents a typed transformation operation.
type Transform struct {
	Type  TransformOperationType `json:"type"`
	Value interface{}            `json:"value"`
}

// PolicyTransforms is the old-API transform structure (kept for backwards compatibility).
type PolicyTransforms struct {
	Spec        *Transform `json:"spec,omitempty"`
	Labels      *Transform `json:"labels,omitempty"`
	Annotations *Transform `json:"annotations,omitempty"`
}

// PolicyOutput is the output structure returned by policy CUE templates.
type PolicyOutput struct {
	Components  []common.ApplicationComponent `json:"components,omitempty"`
	Workflow    *v1beta1.Workflow             `json:"workflow,omitempty"`
	Policies    []v1beta1.AppPolicy           `json:"policies,omitempty"`
	Labels      map[string]string             `json:"labels,omitempty"`
	Annotations map[string]string             `json:"annotations,omitempty"`
	Ctx         map[string]interface{}        `json:"ctx,omitempty"`
}

// extractOutput decodes the output field from rendered CUE. Returns nil if absent.
func (h *AppHandler) extractOutput(val cue.Value) (*PolicyOutput, error) {
	outputVal := val.LookupPath(cue.ParsePath("output"))
	if !outputVal.Exists() {
		return nil, nil
	}

	var output PolicyOutput
	if err := outputVal.Decode(&output); err != nil {
		return nil, errors.Wrap(err, "failed to decode output")
	}

	allowedFields := map[string]bool{
		"components":  true,
		"workflow":    true,
		"policies":    true,
		"labels":      true,
		"annotations": true,
		"ctx":         true,
	}

	iter, err := outputVal.Fields()
	if err != nil {
		return nil, errors.Wrap(err, "failed to iterate output fields")
	}
	for iter.Next() {
		if fieldName := iter.Selector().String(); !allowedFields[fieldName] {
			return nil, errors.Errorf("output.%s is not allowed; permitted fields: components, workflow, policies, labels, annotations, ctx", fieldName)
		}
	}

	return &output, nil
}

// deepMerge recursively merges source into target (source wins on conflict).
func deepMerge(target, source map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range target {
		result[k] = v
	}
	for key, sourceValue := range source {
		if targetValue, exists := result[key]; exists {
			if targetMap, ok := targetValue.(map[string]interface{}); ok {
				if sourceMap, ok := sourceValue.(map[string]interface{}); ok {
					result[key] = deepMerge(targetMap, sourceMap)
					continue
				}
			}
		}
		result[key] = sourceValue
	}
	return result
}

// policyAdditionalContextKey is a plain string so that pkg/cue/process can retrieve it
// without an import cycle (Go context.Value equality requires identical type+value).
const policyAdditionalContextKey = oam.PolicyAdditionalContextKey

// storeAdditionalContextInCtx merges additionalContext into the Go context (available as context.custom in workflows).
func storeAdditionalContextInCtx(ctx monitorContext.Context, additionalContext map[string]interface{}) monitorContext.Context {
	existing := getAdditionalContextFromCtx(ctx)
	if existing == nil {
		existing = make(map[string]interface{})
	}
	merged := deepMerge(existing, additionalContext)
	ctx.SetContext(context.WithValue(ctx.GetContext(), policyAdditionalContextKey, merged))
	return ctx
}

func getAdditionalContextFromCtx(ctx monitorContext.Context) map[string]interface{} {
	if val := ctx.GetContext().Value(policyAdditionalContextKey); val != nil {
		if contextMap, ok := val.(map[string]interface{}); ok {
			return contextMap
		}
	}
	return nil
}

func shouldSkipGlobalPolicies(app *v1beta1.Application) bool {
	return app.Annotations[SkipGlobalPoliciesAnnotation] == annotationValueTrue
}

// shouldAutoCreateRevision reports whether policy-rendered spec changes should trigger new ApplicationRevisions.
func shouldAutoCreateRevision(app *v1beta1.Application) bool {
	return app.Annotations[oam.AnnotationAutoRevision] == annotationValueTrue
}

// validateNotGlobalPolicy returns an error if the named PolicyDefinition is marked Global.
// Global policies are security enforcement controls and cannot be explicitly referenced.
// NotFound is treated as a pass (the error surfaces later during render).
func validateNotGlobalPolicy(ctx monitorContext.Context, cli client.Client, policyName string) error {
	templ, err := appfile.LoadTemplate(ctx.GetContext(), cli, policyName, types.TypePolicy, nil)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to load PolicyDefinition '%s' for global-policy validation", policyName)
	}
	if templ.PolicyDefinition != nil && templ.PolicyDefinition.Spec.Global {
		return errors.Errorf("policy '%s' is marked as Global and cannot be explicitly referenced in Application spec", policyName)
	}
	return nil
}

// extractRenderedSpec collects the last-write-wins spec changes from all enabled policy results.
func extractRenderedSpec(results []RenderedPolicyResult) *v1beta1.ApplicationSpec {
	spec := &v1beta1.ApplicationSpec{}

	for _, result := range results {
		if !result.Enabled || result.Transforms == nil {
			continue
		}

		policyOutput, ok := result.Transforms.(*PolicyOutput)
		if !ok {
			continue
		}

		if len(policyOutput.Components) > 0 {
			spec.Components = policyOutput.Components
		}

		if policyOutput.Workflow != nil {
			spec.Workflow = policyOutput.Workflow
		}

		if policyOutput.Policies != nil {
			spec.Policies = policyOutput.Policies
		}
	}

	return spec
}

// extractRenderedMetadata merges labels, annotations, and context from all enabled policy results.
func extractRenderedMetadata(results []RenderedPolicyResult) map[string]interface{} {
	metadata := map[string]interface{}{
		"labels":      make(map[string]string),
		"annotations": make(map[string]string),
		"context":     make(map[string]interface{}),
	}

	labels := metadata["labels"].(map[string]string)
	annotations := metadata["annotations"].(map[string]string)
	ctx := metadata["context"].(map[string]interface{})

	for _, result := range results {
		if !result.Enabled || result.Transforms == nil {
			continue
		}

		policyOutput, ok := result.Transforms.(*PolicyOutput)
		if !ok {
			continue
		}

		for k, v := range policyOutput.Labels {
			labels[k] = v
		}
		for k, v := range policyOutput.Annotations {
			annotations[k] = v
		}
		for k, v := range result.AdditionalContext {
			ctx[k] = v
		}
	}

	return metadata
}

func applyMetadataToApp(app *v1beta1.Application, metadata map[string]interface{}) {
	if labels, ok := metadata["labels"].(map[string]string); ok {
		if app.Labels == nil {
			app.Labels = make(map[string]string)
		}
		for k, v := range labels {
			app.Labels[k] = v
		}
	}

	if annotations, ok := metadata["annotations"].(map[string]string); ok {
		if app.Annotations == nil {
			app.Annotations = make(map[string]string)
		}
		for k, v := range annotations {
			app.Annotations[k] = v
		}
	}
}

func applySpecToApp(app *v1beta1.Application, renderedSpec *v1beta1.ApplicationSpec) {
	if renderedSpec.Components != nil {
		app.Spec.Components = renderedSpec.Components
	}
	if renderedSpec.Workflow != nil {
		app.Spec.Workflow = renderedSpec.Workflow
	}
	if renderedSpec.Policies != nil {
		app.Spec.Policies = renderedSpec.Policies
	}
}

// populateVersionInfo resolves the DefinitionRevision for a policy and stores it on the result.
// instanceName is used as the policyVersions map key (must be policyRef.Name, not policyDef.Name).
// Resolution order: pre-loaded from ApplicationRevision → Status.LatestRevision → versioned name lookup → list scan.
func (h *AppHandler) populateVersionInfo(ctx monitorContext.Context, result *RenderedPolicyResult, policyDef *v1beta1.PolicyDefinition, instanceName string) {
	result.PolicyDefinitionUsed = policyDef

	// 1. Pre-loaded from stored ApplicationRevision (version pinning).
	if h.policyVersions != nil {
		if versionInfo, ok := h.policyVersions[instanceName]; ok {
			result.DefinitionRevisionName = versionInfo.DefinitionRevisionName
			result.Revision = versionInfo.Revision
			result.RevisionHash = versionInfo.RevisionHash
			return
		}
	}

	// 2. PolicyDefinition.Status.LatestRevision (set by definition controller).
	if policyDef.Status.LatestRevision != nil {
		result.DefinitionRevisionName = policyDef.Status.LatestRevision.Name
		result.Revision = policyDef.Status.LatestRevision.Revision
		result.RevisionHash = policyDef.Status.LatestRevision.RevisionHash
		return
	}

	// 3. Versioned name (e.g., "my-policy-v3" or "my-policy@v3").
	if strings.Contains(policyDef.Name, "-v") || strings.Contains(policyDef.Name, "@") {
		result.DefinitionRevisionName = policyDef.Name
		defRevName, err := oamutil.ConvertDefinitionRevName(policyDef.Name)
		if err == nil {
			defRev := &v1beta1.DefinitionRevision{}
			if err := h.Client.Get(ctx, client.ObjectKey{Namespace: policyDef.Namespace, Name: defRevName}, defRev); err == nil {
				result.Revision = defRev.Spec.Revision
				result.RevisionHash = defRev.Spec.RevisionHash
				return
			}
		}
	}

	// 4. List all DefinitionRevisions and pick the highest revision number.
	defRevList := &v1beta1.DefinitionRevisionList{}
	if err := h.Client.List(ctx, defRevList, client.InNamespace(policyDef.Namespace)); err == nil {
		var latestDefRev *v1beta1.DefinitionRevision
		var latestRevision int64
		for i := range defRevList.Items {
			defRev := &defRevList.Items[i]
			if defRev.Spec.DefinitionType == common.PolicyType && strings.HasPrefix(defRev.Name, policyDef.Name+"-v") {
				if defRev.Spec.Revision > latestRevision {
					latestRevision = defRev.Spec.Revision
					latestDefRev = defRev
				}
			}
		}
		if latestDefRev != nil {
			result.DefinitionRevisionName = latestDefRev.Name
			result.Revision = latestDefRev.Spec.Revision
			result.RevisionHash = latestDefRev.Spec.RevisionHash
		}
	}
}

type policyToRender struct {
	policyDef  *v1beta1.PolicyDefinition
	policyRef  v1beta1.AppPolicy
	priority   int32
	source     string
	specOrder  int    // position in spec.policies; preserves declaration order for explicit policies
	versionKey string // key used in h.policyVersions; namespaced to avoid global/explicit collisions
}

// renderAllPolicies dispatches to the appropriate rendering path based on whether we're creating
// a new ApplicationRevision (cluster lookup) or re-reconciling an existing one (version-pinned).
func (h *AppHandler) renderAllPolicies(ctx monitorContext.Context, app *v1beta1.Application, currentAppRev *v1beta1.ApplicationRevision) ([]RenderedPolicyResult, error) {
	if h.isNewRevision {
		return h.renderPoliciesForNewRevision(ctx, app)
	}
	return h.renderPoliciesFromStoredRevision(ctx, app, currentAppRev)
}

// renderPoliciesForNewRevision discovers global and explicit policies from the cluster.
func (h *AppHandler) renderPoliciesForNewRevision(ctx monitorContext.Context, app *v1beta1.Application) ([]RenderedPolicyResult, error) {
	var policiesToRender []policyToRender

	if !shouldSkipGlobalPolicies(app) && utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableGlobalPolicies) {
		globalPolicyMetadata := policyScopeIndex.GetGlobalApplicationPoliciesDeduped(app.Namespace)

		for _, metadata := range globalPolicyMetadata {
			policyDef := &v1beta1.PolicyDefinition{}
			err := h.Client.Get(ctx.GetContext(), client.ObjectKey{
				Name:      metadata.Name,
				Namespace: metadata.Namespace,
			}, policyDef)
			if err != nil {
				ctx.Info("Failed to load global PolicyDefinition from index",
					"policy", metadata.Name,
					"namespace", metadata.Namespace,
					"error", err)
				continue
			}

			policyDefCopy := policyDef.DeepCopy()
			if h.applicationScopedPolicyDefs == nil {
				h.applicationScopedPolicyDefs = make(map[string]*v1beta1.PolicyDefinition)
			}
			if h.policyVersions == nil {
				h.policyVersions = make(map[string]v1beta1.PolicyVersionMetadata)
			}
			h.applicationScopedPolicyDefs[policyDefCopy.Name] = policyDefCopy
			policiesToRender = append(policiesToRender, policyToRender{
				policyDef:  policyDefCopy,
				policyRef:  v1beta1.AppPolicy{Name: policyDef.Name, Type: policyDef.Name},
				priority:   policyDef.Spec.Priority,
				source:     PolicySourceGlobal,
				versionKey: "global:" + policyDef.Name,
			})
		}
	}

	for specIdx, policy := range app.Spec.Policies {
		// Use index to skip non-Application-scoped policies without a full LoadTemplate call.
		policyName := policy.Type
		if idx := strings.Index(policyName, "@"); idx > 0 {
			policyName = policyName[:idx]
		}
		metadata := policyScopeIndex.Get(policyName, app.Namespace)
		if metadata == nil || metadata.Scope != v1beta1.ApplicationScope {
			continue
		}

		templ, err := appfile.LoadTemplate(ctx, h.Client, policy.Type, types.TypePolicy, app.Annotations)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to load PolicyDefinition for explicit policy %q", policy.Type)
		}
		policyDef := templ.PolicyDefinition
		if policyDef == nil {
			return nil, errors.Errorf("PolicyDefinition not found for explicit policy %q", policy.Type)
		}
		if policyDef.Spec.Scope != v1beta1.ApplicationScope {
			continue
		}

		if h.applicationScopedPolicyDefs == nil {
			h.applicationScopedPolicyDefs = make(map[string]*v1beta1.PolicyDefinition)
		}
		if h.policyVersions == nil {
			h.policyVersions = make(map[string]v1beta1.PolicyVersionMetadata)
		}
		h.applicationScopedPolicyDefs[policyDef.Name] = policyDef
		policiesToRender = append(policiesToRender, policyToRender{
			policyDef: policyDef,
			policyRef: policy,
			priority:  0, // explicit policies run after all globals
			specOrder: specIdx,
			source:    PolicySourceExplicit,
		})
	}

	return h.renderPoliciesInSequence(ctx, app, policiesToRender)
}

// renderPoliciesFromStoredRevision renders using version-pinned PolicyDefinitions from the stored ApplicationRevision.
func (h *AppHandler) renderPoliciesFromStoredRevision(ctx monitorContext.Context, app *v1beta1.Application, currentAppRev *v1beta1.ApplicationRevision) ([]RenderedPolicyResult, error) {
	if currentAppRev == nil || currentAppRev.Spec.PolicyDefinitions == nil {
		return h.renderPoliciesForNewRevision(ctx, app)
	}

	var policiesToRender []policyToRender

	if !shouldSkipGlobalPolicies(app) && utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableGlobalPolicies) {
		for name, storedPolicyDef := range currentAppRev.Spec.PolicyDefinitions {
			if !storedPolicyDef.Spec.Global {
				continue
			}
			policyDefCopy := storedPolicyDef.DeepCopy()
			if h.applicationScopedPolicyDefs == nil {
				h.applicationScopedPolicyDefs = make(map[string]*v1beta1.PolicyDefinition)
			}
			if h.policyVersions == nil {
				h.policyVersions = make(map[string]v1beta1.PolicyVersionMetadata)
			}
			h.applicationScopedPolicyDefs[policyDefCopy.Name] = policyDefCopy
			versionKey := "global:" + name
			if currentAppRev.Spec.PolicyVersions != nil {
				if versionInfo, ok := currentAppRev.Spec.PolicyVersions[name]; ok {
					h.policyVersions[versionKey] = versionInfo
				}
			}
			policiesToRender = append(policiesToRender, policyToRender{
				policyDef:  policyDefCopy,
				policyRef:  v1beta1.AppPolicy{Name: name, Type: name},
				priority:   storedPolicyDef.Spec.Priority,
				source:     PolicySourceGlobal,
				versionKey: versionKey,
			})
		}
	}

	for _, policy := range app.Spec.Policies {
		storedPolicyDef, found := currentAppRev.Spec.PolicyDefinitions[policy.Type]
		if !found {
			policyName := policy.Type
			if idx := strings.Index(policyName, "@"); idx > 0 {
				policyName = policyName[:idx]
			}
			if meta := policyScopeIndex.Get(policyName, app.Namespace); meta == nil || meta.Scope != v1beta1.ApplicationScope {
				continue
			}
			templ, err := appfile.LoadTemplate(ctx, h.Client, policy.Type, types.TypePolicy, app.Annotations)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to load PolicyDefinition for explicit policy %q", policy.Type)
			}
			if templ.PolicyDefinition == nil {
				return nil, errors.Errorf("PolicyDefinition not found for explicit policy %q", policy.Type)
			}
			storedPolicyDef = *templ.PolicyDefinition
		}

		if storedPolicyDef.Spec.Scope != v1beta1.ApplicationScope {
			continue
		}

		policyDefCopy := storedPolicyDef.DeepCopy()
		if h.applicationScopedPolicyDefs == nil {
			h.applicationScopedPolicyDefs = make(map[string]*v1beta1.PolicyDefinition)
		}
		if h.policyVersions == nil {
			h.policyVersions = make(map[string]v1beta1.PolicyVersionMetadata)
		}
		h.applicationScopedPolicyDefs[policyDefCopy.Name] = policyDefCopy
		if currentAppRev.Spec.PolicyVersions != nil {
			if versionInfo, ok := currentAppRev.Spec.PolicyVersions[policy.Name]; ok {
				h.policyVersions[policy.Name] = versionInfo
			}
		}
		policiesToRender = append(policiesToRender, policyToRender{
			policyDef: policyDefCopy,
			policyRef: policy,
			priority:  0,
			source:    PolicySourceExplicit,
		})
	}

	return h.renderPoliciesInSequence(ctx, app, policiesToRender)
}

// renderPoliciesInSequence sorts policies by priority and renders them in order, each seeing
// the Application as modified by all previous policies (chaining).
// Explicit policy render failures are fatal — they block reconciliation.
// Global policy render failures are soft — they are skipped with a log warning.
func (h *AppHandler) renderPoliciesInSequence(ctx monitorContext.Context, app *v1beta1.Application, policiesToRender []policyToRender) ([]RenderedPolicyResult, error) {
	var allResults []RenderedPolicyResult
	workingApp := app.DeepCopy()

	// Lower priority value runs first (Kubernetes convention).
	// At equal priority: globals before explicit; globals sort alphabetically; explicit preserve declaration order.
	sort.SliceStable(policiesToRender, func(i, j int) bool {
		pi, pj := policiesToRender[i], policiesToRender[j]
		if pi.priority != pj.priority {
			return pi.priority < pj.priority
		}
		// Same priority: globals always run before explicit policies
		if pi.source != pj.source {
			return pi.source == PolicySourceGlobal
		}
		// Both global: sort alphabetically for determinism
		if pi.source == PolicySourceGlobal {
			return pi.policyRef.Name < pj.policyRef.Name
		}
		// Both explicit: preserve spec declaration order
		return pi.specOrder < pj.specOrder
	})

	for _, p := range policiesToRender {
		ctx.Info("Rendering policy", "policy", p.policyRef.Name, "priority", p.priority, "source", p.source)

		// Resolve version metadata before rendering so it can be injected into the CUE context.
		// Use versionKey (namespaced) for map operations; fall back to policyRef.Name for explicit policies.
		versionKey := p.versionKey
		if versionKey == "" {
			versionKey = p.policyRef.Name
		}
		var versionMetadata *v1beta1.PolicyVersionMetadata
		if h.policyVersions != nil {
			if versionInfo, ok := h.policyVersions[versionKey]; ok {
				versionMetadata = &versionInfo
			}
		}
		if versionMetadata == nil {
			var tempResult RenderedPolicyResult
			h.populateVersionInfo(ctx, &tempResult, p.policyDef, p.policyRef.Name)
			if h.policyVersions == nil {
				h.policyVersions = make(map[string]v1beta1.PolicyVersionMetadata)
			}
			h.policyVersions[versionKey] = v1beta1.PolicyVersionMetadata{
				DefinitionRevisionName: tempResult.DefinitionRevisionName,
				Revision:               tempResult.Revision,
				RevisionHash:           tempResult.RevisionHash,
			}
			versionMetadata = &v1beta1.PolicyVersionMetadata{
				DefinitionRevisionName: tempResult.DefinitionRevisionName,
				Revision:               tempResult.Revision,
				RevisionHash:           tempResult.RevisionHash,
			}
		}

		specBefore := workingApp.Spec.DeepCopy()

		// Render policy with version metadata
		result, err := h.renderPolicy(ctx, workingApp, p.policyRef, p.policyDef, versionMetadata)
		if err != nil {
			if p.source == PolicySourceExplicit {
				return nil, errors.Wrapf(err, "failed to render explicit policy %q", p.policyRef.Name)
			}
			ctx.Info("Failed to render global policy, skipping", "policy", p.policyRef.Name, "error", err)
			result.PolicyName = p.policyRef.Name
			result.PolicyNamespace = p.policyDef.Namespace
			result.Enabled = false
			result.SkipReason = fmt.Sprintf("render error: %s", err.Error())
			result.IsError = true
		}
		result.Priority = p.priority
		result.Source = p.source
		result.VersionKey = versionKey
		result.DefinitionRevisionName = versionMetadata.DefinitionRevisionName
		result.Revision = versionMetadata.Revision
		result.RevisionHash = versionMetadata.RevisionHash
		result.PolicyDefinitionUsed = p.policyDef

		allResults = append(allResults, result)

		// Chain: apply this policy's output to workingApp so the next policy sees it.
		if result.Enabled && result.Transforms != nil {
			if policyOutput, ok := result.Transforms.(*PolicyOutput); ok {
				if len(policyOutput.Labels) > 0 {
					if workingApp.Labels == nil {
						workingApp.Labels = make(map[string]string)
					}
					for k, v := range policyOutput.Labels {
						workingApp.Labels[k] = v
					}
				}
				if len(policyOutput.Annotations) > 0 {
					if workingApp.Annotations == nil {
						workingApp.Annotations = make(map[string]string)
					}
					for k, v := range policyOutput.Annotations {
						workingApp.Annotations[k] = v
					}
				}
				if len(policyOutput.Components) > 0 {
					workingApp.Spec.Components = policyOutput.Components
				}
				if policyOutput.Workflow != nil {
					workingApp.Spec.Workflow = policyOutput.Workflow
				}
				if policyOutput.Policies != nil {
					workingApp.Spec.Policies = policyOutput.Policies
				}
			}
		}

		// Store spec snapshots only when this policy changed the spec (for ConfigMap audit trail).
		specAfter := workingApp.Spec.DeepCopy()
		if !apiequality.Semantic.DeepEqual(specBefore, specAfter) {
			allResults[len(allResults)-1].SpecBefore = specBefore
			allResults[len(allResults)-1].SpecAfter = specAfter
		}
	}

	return allResults, nil
}

// policyConfigMapName returns the ConfigMap name for policy results, capped at 253 chars.
// Uses a hash suffix when truncation is needed to avoid collisions.
func policyConfigMapName(namespace, appName string) string {
	const prefix = "application-policies-"
	const maxLen = 253
	name := prefix + namespace + "-" + appName
	if len(name) <= maxLen {
		return name
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(namespace+"/"+appName)))[:8]
	available := maxLen - len(prefix) - 1 - 8
	truncated := (namespace + "-" + appName)[:available]
	return prefix + truncated + "-" + hash
}

func createOrUpdateDiffsConfigMap(ctx context.Context, cli client.Client, app *v1beta1.Application, orderedData map[string]string) error {
	cmName := policyConfigMapName(app.Namespace, app.Name)

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

	meta.AddLabels(cm, map[string]string{
		oam.LabelAppName:                   app.Name,
		oam.LabelAppNamespace:              app.Namespace,
		oam.LabelAppUID:                    string(app.UID),
		"app.oam.dev/application-policies": "true",
	})
	meta.AddAnnotations(cm, map[string]string{
		oam.AnnotationLastAppliedTime: time.Now().Format(time.RFC3339),
	})

	err := cli.Create(ctx, cm)
	if err != nil {
		if client.IgnoreAlreadyExists(err) == nil {
			existing := &corev1.ConfigMap{}
			if getErr := cli.Get(ctx, client.ObjectKey{Name: cmName, Namespace: app.Namespace}, existing); getErr != nil {
				return errors.Wrap(getErr, "failed to get existing ConfigMap")
			}

			existing.Data = orderedData
			existing.OwnerReferences = cm.OwnerReferences
			meta.AddLabels(existing, map[string]string{
				oam.LabelAppUID: string(app.UID),
			})
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

func ptrBool(b bool) *bool { return &b }
