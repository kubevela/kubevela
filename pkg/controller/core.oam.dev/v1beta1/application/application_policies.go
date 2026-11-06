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
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	celengine "github.com/kubevela/pkg/cel"
	"github.com/kubevela/pkg/cel/template"
	"github.com/kubevela/pkg/cue/cuex"
	monitorContext "github.com/kubevela/pkg/monitor/context"
	"github.com/pkg/errors"
	jsonpatch "gomodules.xyz/jsonpatch/v2"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	wfprocess "github.com/kubevela/workflow/pkg/cue/process"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/config"
	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/cue/upgrade"
	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/policy/appscope"
	"github.com/oam-dev/kubevela/pkg/sources"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"
)

// SkipGlobalPoliciesAnnotation is an alias for oam.AnnotationSkipGlobalPolicies.
const SkipGlobalPoliciesAnnotation = oam.AnnotationSkipGlobalPolicies

// globalPoliciesCondition is the Application condition that reports a mandatory global policy
// holding the app at its current revision.
const globalPoliciesCondition = "GlobalPolicies"

// ApplyApplicationScopeTransforms applies Application-scoped policy transforms to the in-memory
// Application before it is parsed into an AppFile. Global policies are applied first,
// then explicit spec.policies, in priority order. Results are cached (see ApplicationPolicyCache)
// and recorded in the app's policy ConfigMap, which `vela policy view` reads.
func (h *AppHandler) ApplyApplicationScopeTransforms(ctx monitorContext.Context, app *v1beta1.Application) (monitorContext.Context, error) {
	appliedBefore := app.Status.AppliedApplicationPolicies
	app.Status.AppliedApplicationPolicies = nil

	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableApplicationScopedPolicies) {
		logDebug(ctx, "Application-scoped policies disabled by feature gate")
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
		h.pendingPolicyUpdates = pendingUpdates(cachedResults)
		h.restorePolicyDefinitions(cachedResults)
	} else {
		// The render runs before PrepareCurrentAppRevision, so it decides isNewRevision early: a
		// new revision renders the live policies, and between revisions the pinned ones render.
		// PrepareCurrentAppRevision decides it again for the revision it cuts.
		if revisionLooksNew(app, previousAppRev) {
			h.isNewRevision = true
		}
		klog.V(4).InfoS("Cache MISS - rendering all policies", "reason", cacheMissReason, "isNewRevision", h.isNewRevision)
		var renderErr error
		renderedResults, renderErr = h.renderWithLivePolicies(ctx, app, previousAppRev)
		var mandatory *mandatoryGlobalError
		if errors.As(renderErr, &mandatory) && previousAppRev != nil {
			app.Status.AppliedApplicationPolicies = appliedBefore
			return h.holdForGlobalPolicy(ctx, app, previousAppRev, renderErr)
		}
		if renderErr != nil {
			return ctx, renderErr
		}
		// A render that adopts policy updates is not cached: until its revision is cut, the next
		// reconcile must adopt them again.
		if !h.policyUpdatesAdopted {
			if err := applicationPolicyCache.Set(app, renderedResults); err != nil {
				klog.V(4).InfoS("Failed to cache policy results", "error", err)
			}
		}
	}

	// The record of what the policies last applied is read before this reconcile rewrites it.
	record, err := h.previousPolicyMetadata(ctx, app, renderedResults)
	if err != nil {
		return ctx, err
	}
	renderedResults = holdPolicyData(app, renderedResults, record.applied, h.isNewRevision)
	if cap, ok := ctx.GetContext().Value(dryRunCaptureKey).(*dryRunCapture); ok && cap != nil {
		cap.results = renderedResults
	}

	renderedSpec := extractRenderedSpec(renderedResults)
	renderedMetadata := extractRenderedMetadata(renderedResults)
	h.dataLabels, h.dataAnnotations = dataChangeMetadata(app, renderedResults)
	h.contextHashes = policyContextHashes(renderedResults)
	h.policyResults = renderedResults

	applyMetadataToApp(app, renderedMetadata)

	// Keys the policies set last time and no longer do come off the app, and from there off its
	// resources.
	h.removeStoppedPolicyMetadata(app, record, renderedResults)

	if policyContext, ok := renderedMetadata["context"].(map[string]interface{}); ok && len(policyContext) > 0 {
		ctx = storeAdditionalContextInCtx(ctx, policyContext)
	}

	// A rendered spec rewrite applies in memory. Without autoRevision, an app past its first
	// revision keeps the components, workflow and policies stored in its latest revision, so a
	// rewrite cuts no revision until the app's own next one.
	hasSpecChanges := rewritesSpec(renderedSpec)
	autoRevision := autoRevisionForResults(app, renderedResults)
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
	if app.Status.GetCondition(globalPoliciesCondition).Status == corev1.ConditionFalse {
		app.Status.SetConditions(condition.ReadyCondition(globalPoliciesCondition))
	}

	// Write results to ConfigMap for `vela policy show`.
	// Also written when no policies remain, so the record says the policies set nothing.
	if len(renderedResults) > 0 || len(record.labels) > 0 || len(record.annotations) > 0 || !h.dataRemovedMetadata.Empty() {
		h.writePolicyObservabilityConfigMap(ctx, app, renderedResults, renderedSpec, renderedMetadata, autoRevision, cacheHit)
	}

	logDebug(ctx, "Policy transforms completed",
		"total", len(renderedResults),
		"enabled", countAppliedPolicies(app.Status.AppliedApplicationPolicies),
		"autoRevision", autoRevision)

	return ctx, nil
}

// restorePolicyDefinitions sets the handler's PolicyDefinitions and version metadata from cached
// results, so PrepareCurrentAppRevision includes them in the ApplicationRevision on a cache hit.
func (h *AppHandler) restorePolicyDefinitions(results []RenderedPolicyResult) {
	if len(results) == 0 {
		return
	}
	h.applicationScopedPolicyDefs = make(map[string]*v1beta1.PolicyDefinition)
	h.policyVersions = make(map[string]v1beta1.PolicyVersionMetadata)
	for _, result := range results {
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

// unchangedSinceRevision reports whether the author has not edited app's spec since rev was
// written: the generation recorded on the revision is the app's. A revision stores the spec after
// policy transforms, so a spec-rewriting policy makes the two specs differ with no author edit.
// Revisions that recorded no generation are not judged.
func unchangedSinceRevision(app *v1beta1.Application, rev *v1beta1.ApplicationRevision) bool {
	recorded, ok := rev.GetAnnotations()[oam.AnnotationAppGeneration]
	return ok && recorded == strconv.FormatInt(app.Generation, 10)
}

// rewritesSpec reports whether rendered policy output replaces part of the Application spec.
func rewritesSpec(spec *v1beta1.ApplicationSpec) bool {
	return len(spec.Components) > 0 || spec.Workflow != nil || len(spec.Policies) > 0
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
				Message:                pendingUpdateMessage(result),
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

// pendingUpdateMessage is the status message of an applied policy whose newer version or changed
// data is held.
func pendingUpdateMessage(result RenderedPolicyResult) string {
	var messages []string
	switch {
	case result.UpdatePending && result.UpdateFailsToRender:
		messages = append(messages, "update pending: a newer version of this policy fails to render, so this version "+
			"keeps applying; the application's next revision takes the newer version")
	case result.UpdatePending && result.UpdateRewritesSpec:
		messages = append(messages, "update pending: a newer version of this policy rewrites the application spec, so this "+
			"version keeps applying; the application's next revision takes the newer version")
	case result.UpdatePending:
		messages = append(messages, "update pending: a newer version of this policy is held by "+oam.AnnotationApplyPolicyUpdates+
			" or a publishVersion pin; it applies at the application's next revision")
	}
	if result.DataPending {
		messages = append(messages, "data change pending: this policy's changed output is held by "+oam.AnnotationApplyDataChanges+
			"; it applies at the application's next revision")
	}
	return strings.Join(messages, "; ")
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
	if !h.dataRemovedMetadata.Empty() {
		configMapData[policyPendingRemovalsKey] = encodePendingRemovals(h.dataRemovedMetadata)
	}
	if applied := appliedOutputs(results); len(applied) > 0 {
		configMapData[policyAppliedOutputsKey] = encodeAppliedOutputs(applied)
	}

	sequence := 1
	for _, result := range results {
		if !result.Enabled {
			continue
		}
		policyData := buildPolicyObservabilityData(app, result)
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
		if err := createOrUpdateDiffsConfigMap(ctx, h.Client, app, configMapData, h.policyConfigMap); err != nil {
			ctx.Info("Failed to store policy ConfigMap", "error", err)
		} else {
			app.Status.ApplicationPoliciesConfigMap = policyConfigMapName(app.Namespace, app.Name)
		}
	}
}

// buildPolicyObservabilityData constructs the per-policy data map written to the observability ConfigMap.
func buildPolicyObservabilityData(app *v1beta1.Application, result RenderedPolicyResult) map[string]interface{} {
	policyData := map[string]interface{}{
		"name":      result.PolicyName,
		"type":      result.PolicyType,
		"namespace": result.PolicyNamespace,
		"enabled":   result.Enabled,
		"priority":  result.Priority,
		"settings":  effectivePolicySettings(app, result),
	}
	if own := templateSettings(result.Settings); len(own) > 0 {
		policyData["templateSettings"] = own
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
	removed := h.metadataRemovedFromApp
	hasPolicyMetadata := len(removed.labels) > 0 || len(removed.annotations) > 0
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

	for k, rec := range removed.labels {
		if got, ok := currentApp.Labels[k]; ok && got == rec.value {
			delete(currentApp.Labels, k)
			hasActualChanges = true
		}
	}
	for k, rec := range removed.annotations {
		if got, ok := currentApp.Annotations[k]; ok && got == rec.value {
			delete(currentApp.Annotations, k)
			hasActualChanges = true
		}
	}

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
	result.Settings, result.SettingWarnings = extractPolicySettings(rendered)

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

	if err := checkCtxSchema(rendered); err != nil {
		result.SkipReason = fmt.Sprintf("context schema error: %s", err.Error())
		result.IsError = true
		return result, err
	}

	additionalContext, err := ctxWithSchemaDefaults(rendered, output.Ctx)
	if err != nil {
		result.SkipReason = fmt.Sprintf("context schema error: %s", err.Error())
		result.IsError = true
		return result, err
	}

	result.Transforms = output
	result.AdditionalContext = additionalContext
	return result, nil
}

// renderPolicyCUETemplate compiles and executes the policy CUE template.
// newPolicyProcessContext builds the context an Application-scoped policy template renders with.
// Admission builds its view of that context from the same function (policyContextShape).
func newPolicyProcessContext(goCtx context.Context, app *v1beta1.Application, appRevisionName, policyName, policyType string, versionMetadata *v1beta1.PolicyVersionMetadata) wfprocess.Context {
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
		Ctx:             goCtx,
	})
	pCtx.PushData(velaprocess.ContextAppComponents, app.Spec.Components)
	pCtx.PushData(velaprocess.ContextAppWorkflow, app.Spec.Workflow)
	pCtx.PushData(velaprocess.ContextAppPolicies, app.Spec.Policies)
	pCtx.PushData(velaprocess.ContextPolicyName, policyName)
	pCtx.PushData(velaprocess.ContextPolicyType, policyType)
	pCtx.PushData(velaprocess.ContextPolicyRevisionName, revisionName)
	pCtx.PushData(velaprocess.ContextPolicyRevision, revision)
	pCtx.PushData(velaprocess.ContextPolicyRevisionHash, revisionHash)
	return pCtx
}

// renderPolicyCUETemplate renders a policy template with the app's context (name, namespace,
// components, policy version metadata) and parameters, through the CueX compiler, so templates
// can call providers such as kube.#Get.
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

	pCtx := newPolicyProcessContext(runtimeCtx, app, appRevisionName, policyName, policyType, versionMetadata)

	// Substitute $(context...) expressions in the policy's properties before the
	// template compiles.
	//
	// Context only, and deliberately: this render happens before the appfile
	// exists (the controller orders ApplyApplicationScopeTransforms ahead of
	// GenerateAppFile), so there is no parsed spec.sources[] to resolve against.
	// Context has no such dependency - it is built right here - so the half of
	// the feature that can work does, and reading a source is refused with a
	// reason rather than left inert.
	if params != nil && sources.ExpressionsEnabledFor(app.GetAnnotations()) {
		substituted, serr := substituteScopedPolicyExpressions(pCtx, params)
		if serr != nil {
			return cue.Value{}, errors.Wrap(serr, "failed to resolve context expressions in policy properties")
		}
		params = substituted
	}

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

	template, _ := upgrade.EnsureCueVersionCompatibility(policyDef.Spec.Schematic.CUE.Template, policyDef.Name, upgrade.PolicyKind, upgrade.TemplateAreaMain)
	cueSource := strings.Join([]string{
		template,
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
	return common2.AllowSkipGlobalPolicies && asksToSkipGlobalPolicies(app)
}

// asksToSkipGlobalPolicies reports whether app carries the skip-global annotation, whether or not
// the controller permits skips (--allow-skip-global-policies).
func asksToSkipGlobalPolicies(app *v1beta1.Application) bool {
	return appscope.AsksToSkipGlobals(app)
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

func (p policyToRender) ref() appscope.Ref {
	return appscope.Ref{Name: p.policyRef.Name, Priority: p.priority, Global: p.source == PolicySourceGlobal, SpecOrder: p.specOrder}
}

// revisionLooksNew is the policy render's early view of whether this reconcile cuts a new
// revision: the first one, an author's spec change, or a publishVersion bump. A publishVersion
// bump takes the live policies as an author edit does.
func revisionLooksNew(app *v1beta1.Application, previousAppRev *v1beta1.ApplicationRevision) bool {
	if previousAppRev == nil {
		return true // first reconcile
	}
	prevSpecHash, _ := apply.ComputeSpecHash(previousAppRev.Spec.Application.Spec)
	currSpecHash, _ := apply.ComputeSpecHash(app.Spec)
	if prevSpecHash != currSpecHash && !unchangedSinceRevision(app, previousAppRev) {
		return true
	}
	return metav1.HasAnnotation(app.ObjectMeta, oam.AnnotationPublishVersion) &&
		oam.GetPublishVersion(app) != oam.GetPublishVersion(previousAppRev)
}

// renderWithLivePolicies renders the app's policies. A new revision takes the live policies.
// Otherwise the pinned ones render, except for policies whose live version changed and is adopted:
// it renders in the sequence and its apply-policy-updates setting is on. Adopting marks the
// reconcile to cut a new revision. Updates held by the setting, by a render failure, or by a
// publishVersion pin that has not moved are marked pending and apply at the app's next revision.
// A changed policy that rewrites the spec stays pinned until the next revision.
func (h *AppHandler) renderWithLivePolicies(ctx monitorContext.Context, app *v1beta1.Application, previousAppRev *v1beta1.ApplicationRevision) ([]RenderedPolicyResult, error) {
	if h.isNewRevision {
		return h.renderAllPolicies(ctx, app, previousAppRev)
	}
	changed, live := h.changedPolicies(ctx, app, previousAppRev)
	if len(changed) == 0 {
		return h.renderAllPolicies(ctx, app, previousAppRev)
	}
	pinnedByPublishVersion := metav1.HasAnnotation(app.ObjectMeta, oam.AnnotationPublishVersion) &&
		oam.GetPublishVersion(app) == oam.GetPublishVersion(previousAppRev)
	var candidates []string
	if !pinnedByPublishVersion {
		candidates = changed
	}
	var rewriting []string
	results, adopt, failing, err := h.renderAdopting(ctx, app, previousAppRev, candidates, live)
	for err == nil {
		rewriters := adoptedSpecRewriters(results, adopt)
		if len(rewriters) == 0 {
			break
		}
		logDebug(ctx, "Changed policies rewrite the spec; keeping their pinned versions until the next revision", "policies", rewriters)
		rewriting = append(rewriting, rewriters...)
		candidates = without(candidates, rewriters)
		results, adopt, failing, err = h.renderAdopting(ctx, app, previousAppRev, candidates, live)
	}
	if err != nil {
		return nil, err
	}
	held := without(changed, adopt)
	if len(adopt) > 0 {
		ctx.Info("Applying policy updates and cutting a new revision", "policies", adopt, "revision", previousAppRev.Name)
		h.isNewRevision = true
		h.policyUpdatesAdopted = true
	}
	h.pendingPolicyUpdates = held
	markPendingUpdates(results, held, failing, rewriting)
	return results, nil
}

// renderAdopting renders app with the live versions of candidates over the versions pinned in
// rev. A candidate is held, and the sequence rendered again without it, when its
// apply-policy-updates setting is off or the sequence fails to render with it (see
// failingCandidates); a candidate already known to be held is not tried. It returns the render,
// the candidates it kept, and the held candidates whose live version fails to render, or the
// error of the pinned sequence itself.
func (h *AppHandler) renderAdopting(ctx monitorContext.Context, app *v1beta1.Application, rev *v1beta1.ApplicationRevision, candidates []string, live map[string]*v1beta1.PolicyDefinition) ([]RenderedPolicyResult, []string, []string, error) {
	var unknown, failing []string
	for _, name := range candidates {
		verdict, known := heldUpdates.held(app, rev, name, live[name])
		switch {
		case !known:
			unknown = append(unknown, name)
		case verdict.failsToRender:
			failing = append(failing, name)
		}
	}
	candidates = unknown
	hold := func(name string, failsToRender bool) {
		heldUpdates.hold(app, rev, name, live[name], heldVerdict{failsToRender: failsToRender})
		candidates = without(candidates, []string{name})
		if failsToRender {
			failing = append(failing, name)
		}
	}
	for {
		results, err := h.renderOverlay(ctx, app, rev, candidates, live)
		if err != nil {
			if len(candidates) == 0 {
				return nil, nil, nil, err
			}
			failed := h.failingCandidates(ctx, app, rev, candidates, live, err)
			if len(failed) == 0 {
				return nil, nil, nil, err
			}
			for _, name := range failed {
				ctx.Info("Holding a policy update whose new version fails to render", "policy", name, "error", err)
				hold(name, true)
			}
			continue
		}
		kept := len(candidates)
		for _, name := range candidates {
			if !h.adoptable(ctx, app, rev, name, live[name], results) {
				hold(name, !renderedCleanly(name, results))
			}
		}
		if len(candidates) == kept {
			return results, candidates, failing, nil
		}
	}
}

// renderedCleanly reports whether the named policy's result in results rendered without error.
func renderedCleanly(name string, results []RenderedPolicyResult) bool {
	for _, result := range results {
		if policyDefinitionName(result.PolicyType) == name && result.IsError {
			return false
		}
	}
	return true
}

// failingCandidates names the candidates to hold after rendering them together failed with err.
// A failure in a candidate's own live version names it. When the pinned sequence fails without
// any candidate, it names none. Otherwise it names the candidates that fail when each is rendered
// alone over the pinned sequence, or, when none does and the cause lies in how they combine, the
// last candidate.
func (h *AppHandler) failingCandidates(ctx monitorContext.Context, app *v1beta1.Application, rev *v1beta1.ApplicationRevision, candidates []string, live map[string]*v1beta1.PolicyDefinition, err error) []string {
	if failed := failingPolicy(err); slices.Contains(candidates, failed) {
		return []string{failed}
	}
	if _, pinnedErr := h.renderOverlay(ctx, app, rev, nil, live); pinnedErr != nil {
		return nil
	}
	var failing []string
	for _, name := range candidates {
		if _, soloErr := h.renderOverlay(ctx, app, rev, []string{name}, live); soloErr != nil {
			failing = append(failing, name)
		}
	}
	if len(failing) == 0 {
		failing = candidates[len(candidates)-1:]
	}
	return failing
}

// renderOverlay renders app with the live versions of names over the versions pinned in rev.
func (h *AppHandler) renderOverlay(ctx monitorContext.Context, app *v1beta1.Application, rev *v1beta1.ApplicationRevision, names []string, live map[string]*v1beta1.PolicyDefinition) ([]RenderedPolicyResult, error) {
	h.applicationScopedPolicyDefs = nil
	h.policyVersions = nil
	return h.renderPoliciesFromStoredRevision(ctx, app, overlayPolicies(app, rev, names, live))
}

// adoptable reports whether the rendered live version of the named policy is adopted: it rendered
// without error and its apply-policy-updates setting, from its template, is on. A removed policy
// (no live version) reads the setting from the template of the version pinned in rev.
func (h *AppHandler) adoptable(ctx monitorContext.Context, app *v1beta1.Application, rev *v1beta1.ApplicationRevision, name string, liveDef *v1beta1.PolicyDefinition, results []RenderedPolicyResult) bool {
	if liveDef == nil {
		ref := v1beta1.AppPolicy{Name: name, Type: name}
		for _, policy := range app.Spec.Policies {
			if policyDefinitionName(policy.Type) == name {
				ref = policy
				break
			}
		}
		return policySettingEnabledWith(app, settingApplyPolicyUpdates, name, h.pinnedPolicySetting(ctx, app, ref, rev, settingApplyPolicyUpdates))
	}
	var fromPolicy *bool
	for _, result := range results {
		if policyDefinitionName(result.PolicyType) != name {
			continue
		}
		if result.IsError {
			return false
		}
		fromPolicy = result.Settings[settingApplyPolicyUpdates]
	}
	return policySettingEnabledWith(app, settingApplyPolicyUpdates, name, fromPolicy)
}

// failingPolicy names the policy definition an error from rendering the policy sequence comes
// from, or "" when it names none.
func failingPolicy(err error) string {
	var mandatory *mandatoryGlobalError
	if errors.As(err, &mandatory) {
		return mandatory.policy
	}
	var explicit *explicitPolicyError
	if errors.As(err, &explicit) {
		return explicit.policy
	}
	return ""
}

// without returns names less those in drop.
func without(names, drop []string) []string {
	var out []string
	for _, name := range names {
		if !slices.Contains(drop, name) {
			out = append(out, name)
		}
	}
	return out
}

// pinnedPolicySetting is s as set by the template of the policy version pinned in rev, rendered
// for app: what decides for a removed policy, which has no live version. It is nil when the
// template leaves s unset, or when there is no pinned version or it fails to render.
func (h *AppHandler) pinnedPolicySetting(ctx monitorContext.Context, app *v1beta1.Application, ref v1beta1.AppPolicy, rev *v1beta1.ApplicationRevision, s policySetting) *bool {
	if rev == nil {
		return nil
	}
	pinnedDef, ok := rev.Spec.PolicyDefinitions[policyDefinitionName(ref.Type)]
	if !ok {
		return nil
	}
	result, err := h.renderPolicy(ctx, app.DeepCopy(), ref, &pinnedDef, nil)
	if err != nil || result.IsError {
		return nil
	}
	return result.Settings[s]
}

// adoptedSpecRewriters names the adopted policies whose own output rewrites the spec.
func adoptedSpecRewriters(results []RenderedPolicyResult, adopt []string) []string {
	var rewriters []string
	for _, result := range results {
		name := policyDefinitionName(result.PolicyType)
		if slices.Contains(adopt, name) && !slices.Contains(rewriters, name) &&
			rewritesSpec(extractRenderedSpec([]RenderedPolicyResult{result})) {
			rewriters = append(rewriters, name)
		}
	}
	return rewriters
}

// liveGlobalPolicy reads the global policy the index lists. When a namespace one is gone (the index
// can lag a delete), the same-named vela-system global applies instead, as at render time.
func (h *AppHandler) liveGlobalPolicy(ctx monitorContext.Context, metadata *PolicyMetadata) (*v1beta1.PolicyDefinition, error) {
	pd := &v1beta1.PolicyDefinition{}
	err := h.Client.Get(ctx.GetContext(), client.ObjectKey{Name: metadata.Name, Namespace: metadata.Namespace}, pd)
	if !kerrors.IsNotFound(err) || metadata.Namespace == oam.SystemDefinitionNamespace {
		return pd, err
	}
	sysMeta := policyScopeIndex.GetFromNamespace(metadata.Name, oam.SystemDefinitionNamespace)
	if sysMeta == nil || !sysMeta.Global || sysMeta.Scope != v1beta1.ApplicationScope {
		return pd, err
	}
	err = h.Client.Get(ctx.GetContext(), client.ObjectKey{Name: sysMeta.Name, Namespace: sysMeta.Namespace}, pd)
	return pd, err
}

// overlayPolicies returns a copy of rev whose pinned policy definitions have the named policies
// replaced by their live versions (removed when live has none).
func overlayPolicies(app *v1beta1.Application, rev *v1beta1.ApplicationRevision, names []string, live map[string]*v1beta1.PolicyDefinition) *v1beta1.ApplicationRevision {
	overlay := rev.DeepCopy()
	if overlay.Spec.PolicyDefinitions == nil {
		overlay.Spec.PolicyDefinitions = map[string]v1beta1.PolicyDefinition{}
	}
	for _, name := range names {
		if pd := live[name]; pd != nil {
			overlay.Spec.PolicyDefinitions[name] = *pd
		} else {
			delete(overlay.Spec.PolicyDefinitions, name)
		}
		// The pinned version's metadata no longer applies; the render looks up the live one.
		delete(overlay.Spec.PolicyVersions, name)
		delete(overlay.Spec.PolicyVersions, "global:"+name)
		for _, policy := range app.Spec.Policies {
			if policyDefinitionName(policy.Type) == name {
				delete(overlay.Spec.PolicyVersions, policy.Name)
			}
		}
	}
	return overlay
}

// pendingUpdates names the policies whose results are marked as holding a newer live version.
func pendingUpdates(results []RenderedPolicyResult) []string {
	var pending []string
	for _, result := range results {
		if name := policyDefinitionName(result.PolicyType); result.UpdatePending && !slices.Contains(pending, name) {
			pending = append(pending, name)
		}
	}
	return pending
}

// markPendingUpdates flags the results of policies whose live version is held back, and of those
// held because the live version fails to render.
func markPendingUpdates(results []RenderedPolicyResult, held, failing, rewriting []string) {
	for i := range results {
		name := policyDefinitionName(results[i].PolicyType)
		results[i].UpdatePending = slices.Contains(held, name)
		results[i].UpdateFailsToRender = results[i].UpdatePending && slices.Contains(failing, name)
		results[i].UpdateRewritesSpec = results[i].UpdatePending && slices.Contains(rewriting, name)
	}
}

// changedPolicies returns the names of the Application-scoped policies whose live definition
// differs from the one pinned in latestRev (added, removed or edited), with the live definitions
// by name (nil for a removed policy). Explicit policies referenced with @version are pinned by
// that reference and never count as changed.
func (h *AppHandler) changedPolicies(ctx monitorContext.Context, app *v1beta1.Application, latestRev *v1beta1.ApplicationRevision) ([]string, map[string]*v1beta1.PolicyDefinition) {
	if latestRev == nil || !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableApplicationScopedPolicies) {
		return nil, nil
	}
	pinned := map[string]string{}
	live := map[string]*v1beta1.PolicyDefinition{}
	liveHash := map[string]string{}
	hashOf := func(pd v1beta1.PolicyDefinition) string {
		hash, _ := apply.ComputeSpecHash(pd.Spec)
		return hash
	}

	if !shouldSkipGlobalPolicies(app) && utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableGlobalPolicies) {
		for name, pd := range latestRev.Spec.PolicyDefinitions {
			if pd.Spec.Global {
				pinned[name] = hashOf(pd)
			}
		}
		for _, metadata := range policyScopeIndex.GetGlobalApplicationPoliciesDeduped(app.Namespace) {
			pd, err := h.liveGlobalPolicy(ctx, metadata)
			if kerrors.IsNotFound(err) {
				// Gone, though the index still lists it: a removed policy.
				continue
			}
			if err != nil {
				// Unreadable globals are skipped at render time too; don't count them as a change.
				if hash, ok := pinned[metadata.Name]; ok {
					liveHash[metadata.Name] = hash
				}
				continue
			}
			live[pd.Name], liveHash[pd.Name] = pd, hashOf(*pd)
		}
	}

	for _, policy := range app.Spec.Policies {
		if strings.Contains(policy.Type, "@") {
			continue
		}
		stored, ok := latestRev.Spec.PolicyDefinitions[policy.Type]
		if !ok || stored.Spec.Global || stored.Spec.Scope != v1beta1.ApplicationScope {
			// Not pinned: a newly referenced policy is a spec change, which cuts a revision anyway.
			continue
		}
		pinned[policy.Type] = hashOf(stored)
		templ, err := appfile.LoadTemplate(ctx, h.Client, policy.Type, types.TypePolicy, app.Annotations)
		if err != nil || templ.PolicyDefinition == nil {
			// Explicit policies that fail to load fail the render; leave that to it.
			liveHash[policy.Type] = pinned[policy.Type]
			continue
		}
		live[policy.Type], liveHash[policy.Type] = templ.PolicyDefinition, hashOf(*templ.PolicyDefinition)
	}

	var changed []string
	for name, hash := range liveHash {
		if pinned[name] != hash {
			changed = append(changed, name)
		}
	}
	for name := range pinned {
		if _, ok := liveHash[name]; !ok {
			changed = append(changed, name)
			live[name] = nil
		}
	}
	sort.Strings(changed)
	return changed, live
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
				// The index can briefly lag the informer cache after a delete. Fall back to the
				// same-named vela-system policy without touching the index: the watch handler owns
				// eviction, and evicting here could drop a policy that was just recreated.
				lookupNamespace := metadata.Namespace
				if kerrors.IsNotFound(err) && metadata.Namespace != oam.SystemDefinitionNamespace {
					if sysMeta := policyScopeIndex.GetFromNamespace(metadata.Name, oam.SystemDefinitionNamespace); sysMeta != nil && sysMeta.Global && sysMeta.Scope == v1beta1.ApplicationScope {
						lookupNamespace = sysMeta.Namespace
						err = h.Client.Get(ctx.GetContext(), client.ObjectKey{
							Name:      sysMeta.Name,
							Namespace: sysMeta.Namespace,
						}, policyDef)
					}
				}
				if err != nil {
					if !kerrors.IsNotFound(err) && !common2.AllowSkipGlobalPolicies {
						return nil, &mandatoryGlobalError{policy: metadata.Name, err: errors.Wrapf(err,
							"failed to load global policy %s/%s, which is mandatory", lookupNamespace, metadata.Name)}
					}
					ctx.Info("Failed to load global PolicyDefinition from index",
						"policy", metadata.Name,
						"namespace", lookupNamespace,
						"error", err)
					continue
				}
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
				// Revisions store global versions under their version key; older ones by name.
				if versionInfo, ok := currentAppRev.Spec.PolicyVersions[versionKey]; ok {
					h.policyVersions[versionKey] = versionInfo
				} else if versionInfo, ok := currentAppRev.Spec.PolicyVersions[name]; ok {
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
// An explicit policy that fails to render fails the render. A global one is skipped when skips are
// permitted, and is otherwise a mandatoryGlobalError.
func (h *AppHandler) renderPoliciesInSequence(ctx monitorContext.Context, app *v1beta1.Application, policiesToRender []policyToRender) ([]RenderedPolicyResult, error) {
	var allResults []RenderedPolicyResult
	workingApp := app.DeepCopy()

	// Lower priority value runs first (Kubernetes convention).
	// At equal priority: globals before explicit; globals sort alphabetically; explicit preserve declaration order.
	// Admission orders them the same way (appscope.Less), to judge an
	// Application by the context they will supply.
	sort.SliceStable(policiesToRender, func(i, j int) bool {
		return appscope.Less(policiesToRender[i].ref(), policiesToRender[j].ref())
	})

	for _, p := range policiesToRender {
		logDebug(ctx, "Rendering policy", "policy", p.policyRef.Name, "priority", p.priority, "source", p.source)

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
				return nil, &explicitPolicyError{policy: policyDefinitionName(p.policyRef.Type), err: errors.Wrapf(err,
					"failed to render explicit policy %q", p.policyRef.Name)}
			}
			if !common2.AllowSkipGlobalPolicies {
				// A mandatory global is never dropped from a render: what it adds, such as
				// identity labels, is part of every resource deployed.
				return nil, &mandatoryGlobalError{policy: p.policyRef.Name, err: errors.Wrapf(err,
					"failed to render global policy %q, which is mandatory", p.policyRef.Name)}
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
		applyPolicyResult(workingApp, result)

		// Store spec snapshots only when this policy changed the spec (for ConfigMap audit trail).
		specAfter := workingApp.Spec.DeepCopy()
		if !apiequality.Semantic.DeepEqual(specBefore, specAfter) {
			allResults[len(allResults)-1].SpecBefore = specBefore
			allResults[len(allResults)-1].SpecAfter = specAfter
		}
	}

	return allResults, nil
}

// applyPolicyResult applies an enabled policy's output to app: labels and
// annotations merged, components, workflow and policies replaced where given.
func applyPolicyResult(app *v1beta1.Application, result RenderedPolicyResult) {
	if !result.Enabled || result.Transforms == nil {
		return
	}
	policyOutput, ok := result.Transforms.(*PolicyOutput)
	if !ok {
		return
	}
	if len(policyOutput.Labels) > 0 {
		if app.Labels == nil {
			app.Labels = make(map[string]string)
		}
		for k, v := range policyOutput.Labels {
			app.Labels[k] = v
		}
	}
	if len(policyOutput.Annotations) > 0 {
		if app.Annotations == nil {
			app.Annotations = make(map[string]string)
		}
		for k, v := range policyOutput.Annotations {
			app.Annotations[k] = v
		}
	}
	if len(policyOutput.Components) > 0 {
		app.Spec.Components = policyOutput.Components
	}
	if policyOutput.Workflow != nil {
		app.Spec.Workflow = policyOutput.Workflow
	}
	if policyOutput.Policies != nil {
		app.Spec.Policies = policyOutput.Policies
	}
}

// ApplicationPolicyRender is one Application-scoped policy to render.
type ApplicationPolicyRender struct {
	// App is the Application as the policy receives it; it is required.
	App *v1beta1.Application
	// AppRevision is the revision the template reads as context.appRevision.
	AppRevision string
	Policy      v1beta1.AppPolicy
	// Definition is the policy's definition; it is required.
	Definition *v1beta1.PolicyDefinition
	// Version is the definition revision the template reads as
	// context.policyRevision and the rest; nil leaves them empty.
	Version *v1beta1.PolicyVersionMetadata
}

// RenderApplicationPolicy renders one Application-scoped policy as
// ApplyApplicationScopeTransforms renders each in turn, and returns the
// Application as that policy leaves it, beside the result. cli is what the
// template's providers read through, and may be nil for a template that uses
// none.
func RenderApplicationPolicy(ctx context.Context, cli client.Client, r ApplicationPolicyRender) (*v1beta1.Application, RenderedPolicyResult, error) {
	if r.App == nil {
		return nil, RenderedPolicyResult{}, errors.New("application is required")
	}
	if r.Definition == nil {
		return nil, RenderedPolicyResult{}, errors.New("policy definition is required")
	}
	h := &AppHandler{Client: cli}
	if r.AppRevision != "" {
		h.currentAppRev = &v1beta1.ApplicationRevision{ObjectMeta: metav1.ObjectMeta{Name: r.AppRevision}}
	}
	app := r.App.DeepCopy()
	result, err := h.renderPolicy(monitorContext.NewTraceContext(ctx, ""), app, r.Policy, r.Definition, r.Version)
	if err != nil {
		return nil, result, err
	}
	if r.Version != nil {
		result.DefinitionRevisionName = r.Version.DefinitionRevisionName
		result.Revision = r.Version.Revision
		result.RevisionHash = r.Version.RevisionHash
	}
	result.PolicyDefinitionUsed = r.Definition
	applyPolicyResult(app, result)
	return app, result, nil
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

// createOrUpdateDiffsConfigMap writes the app's policy ConfigMap. existing is the ConfigMap as
// read this reconcile, updated in place; without one it is created, and updated when it exists
// after all.
func createOrUpdateDiffsConfigMap(ctx context.Context, cli client.Client, app *v1beta1.Application, orderedData map[string]string, existing *corev1.ConfigMap) error {
	cmName := policyConfigMapName(app.Namespace, app.Name)
	ownerRefs := []metav1.OwnerReference{
		{
			// Use hardcoded values since TypeMeta is cleared by k8s client after Create/Get
			APIVersion:         v1beta1.SchemeGroupVersion.String(),
			Kind:               v1beta1.ApplicationKind,
			Name:               app.Name,
			UID:                app.UID,
			Controller:         ptrBool(true),
			BlockOwnerDeletion: ptrBool(true),
		},
	}
	update := func(cm *corev1.ConfigMap) error {
		cm = cm.DeepCopy()
		cm.Data = orderedData
		cm.OwnerReferences = ownerRefs
		meta.AddLabels(cm, map[string]string{
			oam.LabelAppUID: string(app.UID),
		})
		meta.AddAnnotations(cm, map[string]string{
			oam.AnnotationLastAppliedTime: time.Now().Format(time.RFC3339),
		})
		return cli.Update(ctx, cm)
	}

	if existing != nil {
		err := update(existing)
		if err == nil {
			return nil
		}
		if !kerrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to update ConfigMap")
		}
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            cmName,
			Namespace:       app.Namespace,
			OwnerReferences: ownerRefs,
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
	if err == nil {
		return nil
	}
	if !kerrors.IsAlreadyExists(err) {
		return errors.Wrap(err, "failed to create ConfigMap")
	}
	current := &corev1.ConfigMap{}
	if getErr := cli.Get(ctx, client.ObjectKey{Name: cmName, Namespace: app.Namespace}, current); getErr != nil {
		return errors.Wrap(getErr, "failed to get existing ConfigMap")
	}
	if updateErr := update(current); updateErr != nil {
		return errors.Wrap(updateErr, "failed to update ConfigMap")
	}
	return nil
}

func ptrBool(b bool) *bool { return &b }

// substituteScopedPolicyExpressions evaluates $(context...) expressions in an
// Application-scoped policy's properties.
//
// The context values are pulled from the render's own process context, for
// exactly the fields ScopedPolicyContext declares readable - so what an
// expression can see is what this render actually carries, rather than what a
// component render would have.
func substituteScopedPolicyExpressions(pCtx wfprocess.Context, params map[string]interface{}) (map[string]interface{}, error) {
	if !template.HasExpression(params) {
		return params, nil
	}

	ctxValues := map[string]interface{}{}
	for _, field := range propexpr.ScopedPolicyContext.ReadableFields() {
		if v := pCtx.GetData(field); v != nil {
			ctxValues[field] = v
		}
	}

	goCtx := pCtx.GetCtx()
	if goCtx == nil {
		goCtx = context.Background()
	}
	resolved, err := celexpr.Vela.EvalTree(goCtx, params, map[string]celengine.Resolver{
		propexpr.ContextIdent: celengine.Static(ctxValues),
	}, celengine.TreeOptions{})
	if err != nil {
		return nil, celexpr.ExplainFaults(err)
	}
	out, ok := resolved.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("policy properties resolved to %T, expected an object", resolved)
	}
	return out, nil
}
