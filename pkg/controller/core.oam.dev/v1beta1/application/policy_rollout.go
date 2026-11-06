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
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	monitorContext "github.com/kubevela/pkg/monitor/context"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
)

// policySetting is one of the rollout choices an Application-scoped policy change is subject
// to. Each resolves, per policy, from the app's annotation, then the policy template's config,
// then the controller-flag default.
type policySetting int

const (
	// settingAutoRevision: policy output that rewrites the spec cuts a new ApplicationRevision.
	settingAutoRevision policySetting = iota
	// settingApplyPolicyUpdates: an app adopts a new version of a policy it runs.
	settingApplyPolicyUpdates
	// settingApplyDataChanges: an app takes changed output from an unchanged policy version.
	settingApplyDataChanges

	policySettingCount
)

// policySettingSources says where each setting is read: the app annotation, the field under the
// policy template's config, and the controller-flag default.
var policySettingSources = [policySettingCount]struct {
	annotation   string
	configField  string
	defaultValue *bool
}{
	settingAutoRevision:       {oam.AnnotationAutoRevision, "autoRevision", &common2.ApplicationDefaultAutoRevision},
	settingApplyPolicyUpdates: {oam.AnnotationApplyPolicyUpdates, "applyPolicyUpdates", &common2.ApplicationDefaultApplyPolicyUpdates},
	settingApplyDataChanges:   {oam.AnnotationApplyDataChanges, "applyDataChanges", &common2.ApplicationDefaultApplyDataChanges},
}

var allPolicySettings = []policySetting{settingAutoRevision, settingApplyPolicyUpdates, settingApplyDataChanges}

func (s policySetting) annotation() string  { return policySettingSources[s].annotation }
func (s policySetting) configField() string { return policySettingSources[s].configField }
func (s policySetting) defaultValue() bool  { return *policySettingSources[s].defaultValue }

// policySettings are the settings a policy's rendered template sets under config, by setting;
// nil is unset.
type policySettings [policySettingCount]*bool

// extractPolicySettings reads the rollout settings from a rendered policy template's config,
// next to config.enabled. They are evaluated with the app's context, so a template can decide
// per app. A value that is not a boolean is left unset and described in the returned warnings:
// a rollout setting never fails the policy.
func extractPolicySettings(rendered cue.Value) (policySettings, []string) {
	var settings policySettings
	var warnings []string
	config := rendered.LookupPath(cue.ParsePath("config"))
	if !config.Exists() {
		return settings, nil
	}
	for _, s := range allPolicySettings {
		v := config.LookupPath(cue.ParsePath(s.configField()))
		if !v.Exists() {
			continue
		}
		b, err := v.Bool()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("config.%s is not a boolean; it is ignored", s.configField()))
			continue
		}
		settings[s] = &b
	}
	return settings, warnings
}

// policySelection is a parsed setting annotation: "true" (every policy), "false" (none), or a
// comma-separated list of PolicyDefinition names (only those).
type policySelection struct {
	all   bool
	names map[string]bool
}

func (p policySelection) includes(policyName string) bool {
	return p.all || p.names[policyName]
}

func (p policySelection) any() bool {
	return p.all || len(p.names) > 0
}

// parsePolicySelection parses a setting annotation. It is not valid when the value is empty or
// names something that cannot be a PolicyDefinition; the setting then resolves from each policy's
// template, then the controller default.
func parsePolicySelection(value string) (policySelection, bool) {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "true":
		return policySelection{all: true}, true
	case "false":
		return policySelection{}, true
	}
	names := map[string]bool{}
	for _, name := range strings.Split(value, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if len(validation.IsDNS1123Subdomain(name)) > 0 {
			return policySelection{}, false
		}
		names[name] = true
	}
	if len(names) == 0 {
		return policySelection{}, false
	}
	return policySelection{names: names}, true
}

// appPolicySelection returns the app's selection for s, and false when the annotation is absent
// or not valid.
func appPolicySelection(annotations map[string]string, s policySetting) (policySelection, bool) {
	value, ok := annotations[s.annotation()]
	if !ok {
		return policySelection{}, false
	}
	return parsePolicySelection(value)
}

// policyDefinitionName is the name a setting list refers to a policy by: its PolicyDefinition
// name, without any @version.
func policyDefinitionName(policyType string) string {
	if idx := strings.Index(policyType, "@"); idx > 0 {
		return policyType[:idx]
	}
	return policyType
}

// policySettingEnabledWith resolves s for one policy of app: the app's annotation, then the
// policy template's value (fromPolicy, nil when unset), then the controller default.
func policySettingEnabledWith(app *v1beta1.Application, s policySetting, policyType string, fromPolicy *bool) bool {
	if sel, ok := appPolicySelection(app.Annotations, s); ok {
		return sel.includes(policyDefinitionName(policyType))
	}
	if fromPolicy != nil {
		return *fromPolicy
	}
	return s.defaultValue()
}

// resultSettingEnabled resolves s for the policy that produced result.
func resultSettingEnabled(app *v1beta1.Application, s policySetting, result RenderedPolicyResult) bool {
	return policySettingEnabledWith(app, s, result.PolicyType, result.Settings[s])
}

// effectivePolicySettings is what each setting resolves to for the policy that produced result,
// as recorded for vela policy view.
func effectivePolicySettings(app *v1beta1.Application, result RenderedPolicyResult) map[string]bool {
	effective := map[string]bool{}
	for _, s := range allPolicySettings {
		effective[s.configField()] = resultSettingEnabled(app, s, result)
	}
	return effective
}

// templateSettings are the settings a template sets, by config field, as recorded in the policy
// ConfigMap.
func templateSettings(settings policySettings) map[string]bool {
	own := map[string]bool{}
	for _, s := range allPolicySettings {
		if settings[s] != nil {
			own[s.configField()] = *settings[s]
		}
	}
	return own
}

// policySettingsFromConfig reads recorded template settings back, ignoring unknown fields.
func policySettingsFromConfig(own map[string]bool) policySettings {
	var settings policySettings
	for _, s := range allPolicySettings {
		if v, ok := own[s.configField()]; ok {
			settings[s] = &v
		}
	}
	return settings
}

// policyConfigWarnings describes the rollout settings in policy templates that were ignored.
func policyConfigWarnings(results []RenderedPolicyResult) []string {
	var warnings []string
	for _, result := range results {
		for _, w := range result.SettingWarnings {
			warnings = append(warnings, fmt.Sprintf("policy %s: %s", policyDefinitionName(result.PolicyType), w))
		}
	}
	return warnings
}

// anyPolicySettingEnabled reports whether s can be on for any policy of an app with these
// annotations, for callers that act on the app as a whole.
func anyPolicySettingEnabled(annotations map[string]string, s policySetting) bool {
	if sel, ok := appPolicySelection(annotations, s); ok {
		return sel.any()
	}
	return s.defaultValue()
}

// autoRevisionForResults reports whether the spec rewrite in these rendered results cuts a new
// revision: some policy that rewrote the spec has auto-revision on.
func autoRevisionForResults(app *v1beta1.Application, results []RenderedPolicyResult) bool {
	for _, result := range results {
		output, ok := result.Transforms.(*PolicyOutput)
		if !result.Enabled || !ok || output == nil {
			continue
		}
		if len(output.Components) > 0 || output.Workflow != nil || len(output.Policies) > 0 {
			if resultSettingEnabled(app, settingAutoRevision, result) {
				return true
			}
		}
	}
	return false
}

// policySettingWarnings describes setting annotations that are ignored, in whole or in part:
// values that cannot be parsed, and listed names that match none of appliedPolicies (policy
// types or PolicyDefinition names of the policies applied to the app).
func policySettingWarnings(app *v1beta1.Application, appliedPolicies []string) []string {
	applied := map[string]bool{}
	for _, p := range appliedPolicies {
		applied[policyDefinitionName(p)] = true
	}
	var warnings []string
	for _, s := range allPolicySettings {
		value, ok := app.Annotations[s.annotation()]
		if !ok {
			continue
		}
		sel, valid := parsePolicySelection(value)
		if !valid {
			warnings = append(warnings, fmt.Sprintf(
				"annotation %s=%q is not \"true\", \"false\" or a list of policy names; it is ignored, and each policy's own setting, or the controller default, applies",
				s.annotation(), value))
			continue
		}
		var unknown []string
		for name := range sel.names {
			if !applied[name] {
				unknown = append(unknown, fmt.Sprintf("%q", name))
			}
		}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			warnings = append(warnings, fmt.Sprintf("annotation %s names %s, which match no policy applied to this application",
				s.annotation(), strings.Join(unknown, ", ")))
		}
	}
	return warnings
}

// dataChangeMetadata returns the labels and annotations of rendered policy output that may reach
// the app's resources without a new revision: for each key, the value from the last policy in
// render order that sets it, when that policy's apply-data-changes setting is on.
func dataChangeMetadata(app *v1beta1.Application, results []RenderedPolicyResult) (map[string]string, map[string]string) {
	labels, labelOwner := map[string]string{}, map[string]int{}
	annotations, annotationOwner := map[string]string{}, map[string]int{}
	for i, result := range results {
		output, ok := result.Transforms.(*PolicyOutput)
		if !result.Enabled || !ok || output == nil {
			continue
		}
		for k, v := range output.Labels {
			labels[k], labelOwner[k] = v, i
		}
		for k, v := range output.Annotations {
			annotations[k], annotationOwner[k] = v, i
		}
	}
	keepOwned := func(values map[string]string, owners map[string]int) map[string]string {
		for k := range values {
			if !resultSettingEnabled(app, settingApplyDataChanges, results[owners[k]]) {
				delete(values, k)
			}
		}
		return values
	}
	return keepOwned(labels, labelOwner), keepOwned(annotations, annotationOwner)
}

// contextHashKey identifies a rendered policy instance: its version key
// (global:<name> for a global, the spec.policies name for an explicit policy), so two policies of
// one type are told apart. Results without one fall back to the PolicyDefinition name.
func contextHashKey(result RenderedPolicyResult) string {
	if result.VersionKey != "" {
		return result.VersionKey
	}
	return policyDefinitionName(result.PolicyType)
}

// appliedPolicyOutput is the data a policy last contributed to the app's components: its context
// and its labels and annotations.
type appliedPolicyOutput struct {
	Context     map[string]interface{} `json:"context,omitempty"`
	Labels      map[string]string      `json:"labels,omitempty"`
	Annotations map[string]string      `json:"annotations,omitempty"`
}

// policyAppliedOutputsKey is the policy observability ConfigMap entry for each policy's applied
// output, by contextHashKey.
const policyAppliedOutputsKey = "applied_outputs"

// appliedOutputOf is the data result contributes.
func appliedOutputOf(result RenderedPolicyResult) appliedPolicyOutput {
	out := appliedPolicyOutput{Context: result.AdditionalContext}
	if output, ok := result.Transforms.(*PolicyOutput); ok && output != nil {
		out.Labels, out.Annotations = output.Labels, output.Annotations
	}
	return out
}

// appliedOutputs is the data each enabled policy in results contributes, by contextHashKey.
func appliedOutputs(results []RenderedPolicyResult) map[string]appliedPolicyOutput {
	applied := map[string]appliedPolicyOutput{}
	for _, result := range results {
		if result.Enabled {
			applied[contextHashKey(result)] = appliedOutputOf(result)
		}
	}
	return applied
}

func encodeAppliedOutputs(applied map[string]appliedPolicyOutput) string {
	raw, _ := json.Marshal(applied)
	return string(raw)
}

func decodeAppliedOutputs(value string) map[string]appliedPolicyOutput {
	applied := map[string]appliedPolicyOutput{}
	_ = json.Unmarshal([]byte(value), &applied)
	return applied
}

// sameAppliedOutput compares two outputs by their recorded form, so an empty map and a missing
// one are equal.
func sameAppliedOutput(a, b appliedPolicyOutput) bool {
	ra, errA := json.Marshal(a)
	rb, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(ra, rb)
}

// holdPolicyData returns results with each policy whose apply-data-changes setting is off
// contributing the output last applied (applied, by contextHashKey) in place of a changed one,
// marked DataPending. A new revision takes every policy's output, as does a policy with nothing
// applied yet. results are shared with the policy cache, so held entries are copies.
func holdPolicyData(app *v1beta1.Application, results []RenderedPolicyResult, applied map[string]appliedPolicyOutput, newRevision bool) []RenderedPolicyResult {
	if newRevision || len(applied) == 0 {
		return results
	}
	var out []RenderedPolicyResult
	for i, result := range results {
		if !result.Enabled || resultSettingEnabled(app, settingApplyDataChanges, result) {
			continue
		}
		prev, ok := applied[contextHashKey(result)]
		if !ok || sameAppliedOutput(prev, appliedOutputOf(result)) {
			continue
		}
		if out == nil {
			out = append([]RenderedPolicyResult(nil), results...)
		}
		held := result
		held.AdditionalContext = prev.Context
		output := &PolicyOutput{}
		if current, ok := result.Transforms.(*PolicyOutput); ok && current != nil {
			copied := *current
			output = &copied
		}
		output.Labels, output.Annotations, output.Ctx = prev.Labels, prev.Annotations, prev.Context
		held.Transforms = output
		held.DataPending = true
		out[i] = held
	}
	if out == nil {
		return results
	}
	return out
}

// recordedKey is a label or annotation the policies set, as last recorded: its value and the
// PolicyDefinition that set it.
type recordedKey struct {
	value string
	owner string
}

// policyMetadataRecord is the policy metadata last applied to an app, read from its policy
// observability ConfigMap before this reconcile rewrites it. It is how a key the policies stop
// setting is found, so the ConfigMap is load-bearing, not only for vela policy view.
type policyMetadataRecord struct {
	labels      map[string]recordedKey
	annotations map[string]recordedKey
	// settings are the rollout settings each policy's template set, by PolicyDefinition name.
	settings map[string]policySettings
	// applied is the output each policy last contributed to the app's components, by
	// contextHashKey.
	applied map[string]appliedPolicyOutput
}

// decodePolicyMetadataRecord reads a policy observability ConfigMap's data: the merged
// "metadata" entry for values, and the per-policy entries for which policy set each key.
func decodePolicyMetadataRecord(data map[string]string) policyMetadataRecord {
	record := policyMetadataRecord{labels: map[string]recordedKey{}, annotations: map[string]recordedKey{}, settings: map[string]policySettings{}}
	if applied, ok := data[policyAppliedOutputsKey]; ok {
		record.applied = decodeAppliedOutputs(applied)
	}
	var merged struct {
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	}
	if err := json.Unmarshal([]byte(data["metadata"]), &merged); err != nil {
		return record
	}
	labelOwner, annotationOwner := map[string]string{}, map[string]string{}
	var entries []string
	for key := range data {
		if isPolicyEntryKey(key) {
			entries = append(entries, key)
		}
	}
	sort.Strings(entries) // render order: a later policy's value is the one applied
	for _, key := range entries {
		var entry struct {
			Name             string          `json:"name"`
			Type             string          `json:"type"`
			TemplateSettings map[string]bool `json:"templateSettings"`
			Output           struct {
				Labels      map[string]string `json:"labels"`
				Annotations map[string]string `json:"annotations"`
			} `json:"output"`
		}
		if json.Unmarshal([]byte(data[key]), &entry) != nil {
			continue
		}
		owner := entry.Type
		if owner == "" {
			owner = entry.Name // an entry without a type names the policy only
		}
		owner = policyDefinitionName(owner)
		if len(entry.TemplateSettings) > 0 {
			record.settings[owner] = policySettingsFromConfig(entry.TemplateSettings)
		}
		for k := range entry.Output.Labels {
			labelOwner[k] = owner
		}
		for k := range entry.Output.Annotations {
			annotationOwner[k] = owner
		}
	}
	for k, v := range merged.Labels {
		record.labels[k] = recordedKey{value: v, owner: labelOwner[k]}
	}
	for k, v := range merged.Annotations {
		record.annotations[k] = recordedKey{value: v, owner: annotationOwner[k]}
	}
	return record
}

// isPolicyEntryKey reports whether a policy observability ConfigMap key is a per-policy entry
// ("001-<policy name>").
func isPolicyEntryKey(key string) bool {
	if len(key) < 5 || key[3] != '-' {
		return false
	}
	for _, c := range key[:3] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// removedPolicyMetadata returns the recorded keys the rendered policies no longer set, and that
// still have the recorded value on app: a key someone has changed since is not the policies' to
// remove.
func removedPolicyMetadata(app *v1beta1.Application, record policyMetadataRecord, results []RenderedPolicyResult) policyMetadataRecord {
	rendered := extractRenderedMetadata(results)
	labels, _ := rendered["labels"].(map[string]string)
	annotations, _ := rendered["annotations"].(map[string]string)
	removed := func(recorded map[string]recordedKey, current, onApp map[string]string) map[string]recordedKey {
		out := map[string]recordedKey{}
		for k, rec := range recorded {
			if _, stillSet := current[k]; stillSet {
				continue
			}
			if got, ok := onApp[k]; ok && got == rec.value {
				out[k] = rec
			}
		}
		return out
	}
	return policyMetadataRecord{
		labels:      removed(record.labels, labels, app.Labels),
		annotations: removed(record.annotations, annotations, app.Annotations),
	}
}

// removalOf converts the removed keys whose owner keep accepts to a resource keeper removal.
func removalOf(removed policyMetadataRecord, keep func(owner string) bool) resourcekeeper.MetadataRemoval {
	pick := func(keys map[string]recordedKey) map[string]string {
		out := map[string]string{}
		for k, rec := range keys {
			if keep(rec.owner) {
				out[k] = rec.value
			}
		}
		return out
	}
	return resourcekeeper.MetadataRemoval{Labels: pick(removed.labels), Annotations: pick(removed.annotations)}
}

// removeFromApp takes removed keys off app, which then neither carries nor passes them down.
func removeFromApp(app *v1beta1.Application, removed policyMetadataRecord) {
	for k := range removed.labels {
		delete(app.Labels, k)
	}
	for k := range removed.annotations {
		delete(app.Annotations, k)
	}
}

// previousPolicyMetadata reads the policy metadata last applied to app from its policy
// observability ConfigMap, and keeps the ConfigMap for this reconcile's write. The read is
// skipped only for an app whose status names no ConfigMap and whose policies render nothing. A
// missing ConfigMap, or one not labelled with app's uid, records nothing. Any other
// read error is returned: the rewrite carries the record forward, so it must have read it.
func (h *AppHandler) previousPolicyMetadata(ctx monitorContext.Context, app *v1beta1.Application, results []RenderedPolicyResult) (policyMetadataRecord, error) {
	h.policyConfigMap = nil
	if app.Status.ApplicationPoliciesConfigMap == "" && len(results) == 0 {
		return decodePolicyMetadataRecord(nil), nil
	}
	cm := &corev1.ConfigMap{}
	if err := h.Client.Get(ctx.GetContext(), client.ObjectKey{Name: policyConfigMapName(app.Namespace, app.Name), Namespace: app.Namespace}, cm); err != nil {
		if kerrors.IsNotFound(err) {
			return decodePolicyMetadataRecord(nil), nil
		}
		return policyMetadataRecord{}, errors.Wrap(err, "failed to read the policy ConfigMap")
	}
	h.policyConfigMap = cm
	if cm.Labels[oam.LabelAppUID] != string(app.UID) {
		return decodePolicyMetadataRecord(nil), nil
	}
	return decodePolicyMetadataRecord(cm.Data), nil
}

// removeStoppedPolicyMetadata takes the keys the policies set last time (record) and no longer do
// off app, and keeps them for the Application patch and the data refresh.
func (h *AppHandler) removeStoppedPolicyMetadata(app *v1beta1.Application, record policyMetadataRecord, results []RenderedPolicyResult) {
	removed := removedPolicyMetadata(app, record, results)
	removeFromApp(app, removed)
	h.metadataRemovedFromApp = removed
	h.dataRemovedMetadata = removalOf(removed, func(owner string) bool {
		// A policy that no longer renders decides by the settings its template last set.
		fromPolicy := record.settings[owner][settingApplyDataChanges]
		for _, result := range results {
			if policyDefinitionName(result.PolicyType) == owner {
				fromPolicy = result.Settings[settingApplyDataChanges]
			}
		}
		return policySettingEnabledWith(app, settingApplyDataChanges, owner, fromPolicy)
	})
}
