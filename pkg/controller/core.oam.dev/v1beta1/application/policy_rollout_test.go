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
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	monitorContext "github.com/kubevela/pkg/monitor/context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
)

// setRolloutDefaults sets the three controller-flag defaults for the duration of t.
func setRolloutDefaults(t *testing.T, autoRevision, policyUpdates, dataChanges bool) {
	t.Helper()
	orig := []bool{common2.ApplicationDefaultAutoRevision, common2.ApplicationDefaultApplyPolicyUpdates, common2.ApplicationDefaultApplyDataChanges}
	t.Cleanup(func() {
		common2.ApplicationDefaultAutoRevision, common2.ApplicationDefaultApplyPolicyUpdates, common2.ApplicationDefaultApplyDataChanges = orig[0], orig[1], orig[2]
	})
	common2.ApplicationDefaultAutoRevision, common2.ApplicationDefaultApplyPolicyUpdates, common2.ApplicationDefaultApplyDataChanges = autoRevision, policyUpdates, dataChanges
}

func appWithAnnotations(annotations map[string]string) *v1beta1.Application {
	return &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "ns", Annotations: annotations}}
}

func TestParsePolicySelection(t *testing.T) {
	cases := []struct {
		value     string
		want      policySelection
		wantValid bool
	}{
		{"true", policySelection{all: true}, true},
		{"TRUE", policySelection{all: true}, true},
		{" false ", policySelection{}, true},
		{"gwcp-tags,cluster-info", policySelection{names: map[string]bool{"gwcp-tags": true, "cluster-info": true}}, true},
		{"gwcp-tags, cluster-info ,", policySelection{names: map[string]bool{"gwcp-tags": true, "cluster-info": true}}, true},
		{"yes", policySelection{names: map[string]bool{"yes": true}}, true},
		{"", policySelection{}, false},
		{" , ", policySelection{}, false},
		{"gwcp-tags,Not_A_Name", policySelection{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			got, valid := parsePolicySelection(tc.value)
			if valid != tc.wantValid {
				t.Fatalf("parsePolicySelection(%q) valid = %v, want %v", tc.value, valid, tc.wantValid)
			}
			if valid && !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parsePolicySelection(%q) = %+v, want %+v", tc.value, got, tc.want)
			}
		})
	}
}

func TestPolicySettingEnabled(t *testing.T) {
	settings := map[string]struct {
		setting    policySetting
		annotation string
	}{
		"auto-revision":        {settingAutoRevision, oam.AnnotationAutoRevision},
		"apply-policy-updates": {settingApplyPolicyUpdates, oam.AnnotationApplyPolicyUpdates},
		"apply-data-changes":   {settingApplyDataChanges, oam.AnnotationApplyDataChanges},
	}
	cases := []struct {
		name        string
		value       *string
		flagDefault bool
		want        bool
	}{
		{"absent uses the default (off)", nil, false, false},
		{"absent uses the default (on)", nil, true, true},
		{"true enables every policy", ptrString("true"), false, true},
		{"false disables every policy", ptrString("false"), true, false},
		{"a list enables the named policy", ptrString("other,gwcp-tags"), false, true},
		{"a list disables policies it does not name", ptrString("other"), true, false},
		{"an invalid value falls back to the default", ptrString("Not_A_Name"), true, true},
	}
	for settingName, s := range settings {
		for _, tc := range cases {
			t.Run(settingName+"/"+tc.name, func(t *testing.T) {
				setRolloutDefaults(t, tc.flagDefault, tc.flagDefault, tc.flagDefault)
				var annotations map[string]string
				if tc.value != nil {
					annotations = map[string]string{s.annotation: *tc.value}
				}
				if got := policySettingEnabledWith(appWithAnnotations(annotations), s.setting, "gwcp-tags", nil); got != tc.want {
					t.Fatalf("policySettingEnabled = %v, want %v", got, tc.want)
				}
			})
		}
	}

	t.Run("a versioned policy type matches by definition name", func(t *testing.T) {
		setRolloutDefaults(t, false, false, false)
		app := appWithAnnotations(map[string]string{oam.AnnotationApplyDataChanges: "gwcp-tags"})
		if !policySettingEnabledWith(app, settingApplyDataChanges, "gwcp-tags@v2", nil) {
			t.Fatal("expected gwcp-tags@v2 to match the gwcp-tags list entry")
		}
	})
}

func TestPolicySettingWarnings(t *testing.T) {
	applied := []string{"gwcp-tags", "cluster-info"}
	cases := []struct {
		name        string
		annotations map[string]string
		wantParts   []string
	}{
		{"no annotations", nil, nil},
		{"valid values", map[string]string{oam.AnnotationAutoRevision: "true", oam.AnnotationApplyDataChanges: "gwcp-tags"}, nil},
		{"unknown policy name", map[string]string{oam.AnnotationApplyPolicyUpdates: "gwcp-tags,gwcp-tag"},
			[]string{oam.AnnotationApplyPolicyUpdates, `"gwcp-tag"`}},
		{"invalid value", map[string]string{oam.AnnotationApplyDataChanges: "Not_A_Name"},
			[]string{oam.AnnotationApplyDataChanges, `"Not_A_Name"`, "ignored", "each policy's own setting, or the controller default, applies"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			warnings := policySettingWarnings(appWithAnnotations(tc.annotations), applied)
			if len(tc.wantParts) == 0 {
				if len(warnings) != 0 {
					t.Fatalf("expected no warnings, got %v", warnings)
				}
				return
			}
			if len(warnings) != 1 {
				t.Fatalf("expected one warning, got %v", warnings)
			}
			for _, part := range tc.wantParts {
				if !strings.Contains(warnings[0], part) {
					t.Fatalf("warning %q does not mention %q", warnings[0], part)
				}
			}
		})
	}
}

func TestAutoRevisionForResults(t *testing.T) {
	results := []RenderedPolicyResult{
		{PolicyName: "rewrites-spec", PolicyType: "rewrites-spec", Enabled: true,
			Transforms: &PolicyOutput{Components: []common.ApplicationComponent{{Name: "c"}}}},
		{PolicyName: "tags-only", PolicyType: "tags-only", Enabled: true,
			Transforms: &PolicyOutput{Labels: map[string]string{"k": "v"}}},
	}
	cases := []struct {
		name        string
		annotations map[string]string
		flagDefault bool
		want        bool
	}{
		{"default off", nil, false, false},
		{"default on", nil, true, true},
		{"listed policy rewrites the spec", map[string]string{oam.AnnotationAutoRevision: "rewrites-spec"}, false, true},
		{"listed policy only sets tags", map[string]string{oam.AnnotationAutoRevision: "tags-only"}, true, false},
		{"false overrides the default", map[string]string{oam.AnnotationAutoRevision: "false"}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setRolloutDefaults(t, tc.flagDefault, false, false)
			if got := autoRevisionForResults(appWithAnnotations(tc.annotations), results); got != tc.want {
				t.Fatalf("autoRevisionForResults = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAnyPolicySettingEnabled(t *testing.T) {
	cases := []struct {
		name        string
		annotations map[string]string
		flagDefault bool
		want        bool
	}{
		{"absent uses the default (off)", nil, false, false},
		{"absent uses the default (on)", nil, true, true},
		{"true", map[string]string{oam.AnnotationAutoRevision: "true"}, false, true},
		{"false", map[string]string{oam.AnnotationAutoRevision: "false"}, true, false},
		{"a list enables some policies", map[string]string{oam.AnnotationAutoRevision: "gwcp-tags"}, false, true},
		{"an invalid value falls back to the default", map[string]string{oam.AnnotationAutoRevision: "Not_A_Name"}, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setRolloutDefaults(t, tc.flagDefault, false, false)
			if got := anyPolicySettingEnabled(tc.annotations, settingAutoRevision); got != tc.want {
				t.Fatalf("anyPolicySettingEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestChangedPolicies(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableApplicationScopedPolicies, true)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableGlobalPolicies, true)
	allowNamespaceGlobals(t)
	useTestPolicyScopeIndex(t)

	policy := func(name, namespace string, global bool, value string) *v1beta1.PolicyDefinition {
		return &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope:     v1beta1.ApplicationScope,
				Global:    global,
				Schematic: &common.Schematic{CUE: &common.CUE{Template: `parameter: {}, output: labels: {"v": "` + value + `"}`}},
			},
		}
	}
	const ns = "my-ns"
	sys := oam.SystemDefinitionNamespace
	live := []*v1beta1.PolicyDefinition{
		policy("edited", ns, true, "v2"),
		policy("same", ns, true, "v1"),
		policy("added", ns, true, "v1"),
		policy("explicit", sys, false, "v2"),
		policy("versioned", sys, false, "v2"),
	}
	pinned := map[string]v1beta1.PolicyDefinition{
		"edited":    *policy("edited", ns, true, "v1"),
		"same":      *policy("same", ns, true, "v1"),
		"removed":   *policy("removed", ns, true, "v1"),
		"explicit":  *policy("explicit", sys, false, "v1"),
		"versioned": *policy("versioned", sys, false, "v1"),
	}
	builder := fake.NewClientBuilder().WithScheme(newTestScheme(t))
	for _, pd := range live {
		builder = builder.WithObjects(pd)
		policyScopeIndex.AddOrUpdate(pd)
	}
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns},
		Spec: v1beta1.ApplicationSpec{Policies: []v1beta1.AppPolicy{
			{Name: "e", Type: "explicit"},
			{Name: "v", Type: "versioned@v1"},
		}},
	}
	latestRev := &v1beta1.ApplicationRevision{ObjectMeta: metav1.ObjectMeta{Name: "app-v1", Namespace: ns}}
	latestRev.Spec.PolicyDefinitions = pinned

	h := &AppHandler{Client: builder.Build(), app: app}
	got, liveDefs := h.changedPolicies(monitorContext.NewTraceContext(context.Background(), "test"), app, latestRev)
	want := []string{"added", "edited", "explicit", "removed"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("changedPolicies = %v, want %v (a versioned reference is a pin, an unchanged policy is not a change)", got, want)
	}
	if liveDefs["removed"] != nil || liveDefs["edited"] == nil {
		t.Fatalf("expected live definitions for changed policies and nil for a removed one, got %v", liveDefs)
	}
}

func TestChangedPoliciesStaleIndexEntry(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableApplicationScopedPolicies, true)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableGlobalPolicies, true)
	allowNamespaceGlobals(t)
	policy := func(name, namespace, value string) *v1beta1.PolicyDefinition {
		return &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: v1beta1.PolicyDefinitionSpec{Scope: v1beta1.ApplicationScope, Global: true,
				Schematic: &common.Schematic{CUE: &common.CUE{Template: `parameter: {}, output: labels: {"v": "` + value + `"}`}}},
		}
	}
	const ns = "my-ns"
	sys := oam.SystemDefinitionNamespace
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns}}
	monCtx := monitorContext.NewTraceContext(context.Background(), "test")

	t.Run("a deleted global still in the index is removed", func(t *testing.T) {
		useTestPolicyScopeIndex(t)
		policyScopeIndex.AddOrUpdate(policy("deleted", sys, "v1")) // the delete event has not reached the index
		latestRev := &v1beta1.ApplicationRevision{}
		latestRev.Spec.PolicyDefinitions = map[string]v1beta1.PolicyDefinition{"deleted": *policy("deleted", sys, "v1")}
		h := &AppHandler{Client: fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build(), app: app}
		changed, live := h.changedPolicies(monCtx, app, latestRev)
		if !reflect.DeepEqual(changed, []string{"deleted"}) || live["deleted"] != nil {
			t.Fatalf("changed = %v, live = %v: a global that is gone must count as removed", changed, live)
		}
	})

	t.Run("a deleted namespace global falls back to the same-named vela-system one", func(t *testing.T) {
		useTestPolicyScopeIndex(t)
		system := policy("shared", sys, "system")
		policyScopeIndex.AddOrUpdate(system)
		policyScopeIndex.AddOrUpdate(policy("shared", ns, "namespace")) // stale: deleted from the cluster
		latestRev := &v1beta1.ApplicationRevision{}
		latestRev.Spec.PolicyDefinitions = map[string]v1beta1.PolicyDefinition{"shared": *policy("shared", ns, "namespace")}
		h := &AppHandler{Client: fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(system).Build(), app: app}
		changed, live := h.changedPolicies(monCtx, app, latestRev)
		if !reflect.DeepEqual(changed, []string{"shared"}) || live["shared"] == nil || live["shared"].Namespace != sys {
			t.Fatalf("changed = %v, live = %v: the vela-system policy now applies", changed, live)
		}
	})
}

func TestEmitPendingPolicyUpdates(t *testing.T) {
	rec := &recordingRecorder{}
	r := &Reconciler{Recorder: rec}
	r.emitPendingPolicyUpdates(&v1beta1.Application{}, []string{"gwcp-tags"})
	if len(rec.events) != 1 || rec.events[0].Reason != "PolicyUpdatePending" || !strings.Contains(rec.events[0].Message, "gwcp-tags") {
		t.Fatalf("expected one PolicyUpdatePending event naming gwcp-tags, got %+v", rec.events)
	}
	if rec.events[0].Type != event.TypeNormal {
		t.Fatalf("PolicyUpdatePending should be a normal event, got %q", rec.events[0].Type)
	}
}

func TestDataChangeMetadata(t *testing.T) {
	setRolloutDefaults(t, false, false, false)
	results := []RenderedPolicyResult{
		{PolicyType: "first", Enabled: true, Transforms: &PolicyOutput{
			Labels: map[string]string{"shared": "from-first", "first-only": "1"}, Annotations: map[string]string{"note": "first"}}},
		{PolicyType: "second", Enabled: true, Transforms: &PolicyOutput{
			Labels: map[string]string{"shared": "from-second", "second-only": "2"}}},
		{PolicyType: "disabled", Enabled: false, Transforms: &PolicyOutput{Labels: map[string]string{"off": "x"}}},
	}
	cases := []struct {
		name            string
		value           string
		wantLabels      map[string]string
		wantAnnotations map[string]string
	}{
		{"all policies", "true",
			map[string]string{"shared": "from-second", "first-only": "1", "second-only": "2"}, map[string]string{"note": "first"}},
		{"a key belongs to the last policy that sets it", "first",
			map[string]string{"first-only": "1"}, map[string]string{"note": "first"}},
		{"none", "false", map[string]string{}, map[string]string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := appWithAnnotations(map[string]string{oam.AnnotationApplyDataChanges: tc.value})
			labels, annotations := dataChangeMetadata(app, results)
			if !reflect.DeepEqual(labels, tc.wantLabels) || !reflect.DeepEqual(annotations, tc.wantAnnotations) {
				t.Fatalf("dataChangeMetadata = %v, %v; want %v, %v", labels, annotations, tc.wantLabels, tc.wantAnnotations)
			}
		})
	}
}

func TestPolicyContextHashes(t *testing.T) {
	results := func(region string) []RenderedPolicyResult {
		return []RenderedPolicyResult{
			{PolicyType: "region@v2", Enabled: true, AdditionalContext: map[string]interface{}{"region": region}},
			{PolicyType: "tier", Enabled: true, AdditionalContext: map[string]interface{}{"tier": "gold"}},
			{PolicyType: "labels-only", Enabled: true},
			{PolicyType: "disabled", Enabled: false, AdditionalContext: map[string]interface{}{"x": "y"}},
		}
	}
	eu, us := policyContextHashes(results("eu")), policyContextHashes(results("us"))
	if len(eu) != 2 || eu["region"] == "" || eu["tier"] == "" {
		t.Fatalf("expected a hash per policy that sets context, by definition name, got %v", eu)
	}
	if eu["region"] == us["region"] || eu["tier"] != us["tier"] {
		t.Fatalf("only the policy whose context changed should change hash: eu=%v us=%v", eu, us)
	}
}

func TestContextRefreshNeeded(t *testing.T) {
	recorded := map[string]string{"region": "h1", "tier": "h2"}
	cases := []struct {
		name    string
		value   string
		current map[string]string
		want    bool
	}{
		{"unchanged", "true", map[string]string{"region": "h1", "tier": "h2"}, false},
		{"changed and enabled", "true", map[string]string{"region": "h9", "tier": "h2"}, true},
		{"changed but not enabled", "tier", map[string]string{"region": "h9", "tier": "h2"}, false},
		{"added and enabled", "true", map[string]string{"region": "h1", "tier": "h2", "new": "h3"}, true},
		{"removed and enabled", "true", map[string]string{"region": "h1"}, true},
		{"setting off", "false", map[string]string{"region": "h9"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setRolloutDefaults(t, false, false, false)
			app := appWithAnnotations(map[string]string{oam.AnnotationApplyDataChanges: tc.value})
			if got := contextRefreshNeeded(app, recorded, tc.current, nil, nil); got != tc.want {
				t.Fatalf("contextRefreshNeeded = %v, want %v", got, tc.want)
			}
		})
	}
}

// Two policies of one type each get their own hash, so a change in either is seen, and each
// instance's own template setting decides.
func TestPolicyContextHashesPerInstance(t *testing.T) {
	setRolloutDefaults(t, false, false, false)
	app := appWithAnnotations(nil)
	results := func(p1Region string) []RenderedPolicyResult {
		return []RenderedPolicyResult{
			{PolicyType: "tenant-ctx", VersionKey: "p1", Enabled: true, Settings: policySettings{settingApplyDataChanges: ptrBool(true)},
				AdditionalContext: map[string]interface{}{"region": p1Region}},
			{PolicyType: "tenant-ctx", VersionKey: "p2", Enabled: true, Settings: policySettings{settingApplyDataChanges: ptrBool(false)},
				AdditionalContext: map[string]interface{}{"region": "us"}},
		}
	}
	before, after := policyContextHashes(results("eu")), policyContextHashes(results("ap"))
	if len(before) != 2 {
		t.Fatalf("expected a hash per policy instance, got %v", before)
	}
	if !contextRefreshNeeded(app, before, after, results("ap"), nil) {
		t.Fatal("a context change in the first of two same-typed policies should refresh")
	}
	held := results("eu")
	held[0].Settings, held[1].Settings = held[1].Settings, held[0].Settings
	if contextRefreshNeeded(app, before, after, held, nil) {
		t.Fatal("the changed instance's own setting is off, so it should not refresh")
	}
}

// A global that no longer renders is matched to its settings by its PolicyDefinition name.
func TestContextRefreshNeededForRemovedGlobal(t *testing.T) {
	setRolloutDefaults(t, false, false, false)
	app := appWithAnnotations(map[string]string{oam.AnnotationApplyDataChanges: "region"})
	if !contextRefreshNeeded(app, map[string]string{"global:region": "h1"}, map[string]string{}, nil, nil) {
		t.Fatal("a removed global whose definition name is listed should refresh")
	}
}

// A removed policy's own template settings are recorded with its output, so they still decide
// once it no longer renders.
func TestRecordedTemplateSettings(t *testing.T) {
	result := RenderedPolicyResult{PolicyName: "tags", PolicyType: "tags", Enabled: true,
		Settings:   policySettings{settingApplyDataChanges: ptrBool(true)},
		Transforms: &PolicyOutput{Labels: map[string]string{"k": "v"}}}
	entry, err := json.Marshal(buildPolicyObservabilityData(appWithAnnotations(nil), result))
	if err != nil {
		t.Fatal(err)
	}
	record := decodePolicyMetadataRecord(map[string]string{"metadata": `{"labels": {"k": "v"}}`, "001-tags": string(entry)})
	got := record.settings["tags"][settingApplyDataChanges]
	if got == nil || !*got {
		t.Fatalf("the template's applyDataChanges should survive the ConfigMap round trip, got %v", record.settings)
	}
	if record.settings["tags"][settingApplyPolicyUpdates] != nil {
		t.Fatal("a setting the template leaves unset stays unset")
	}
}

func TestContextRefreshNeededForRemovedGlobalFromItsTemplate(t *testing.T) {
	setRolloutDefaults(t, false, false, false)
	app := appWithAnnotations(nil)
	recorded := map[string]policySettings{"region": {settingApplyDataChanges: ptrBool(true)}}
	if !contextRefreshNeeded(app, map[string]string{"global:region": "h1"}, map[string]string{}, nil, recorded) {
		t.Fatal("a removed global whose template set applyDataChanges: true should refresh")
	}
	if contextRefreshNeeded(app, map[string]string{"global:region": "h1"}, map[string]string{}, nil, nil) {
		t.Fatal("without a recorded setting the default (off) decides")
	}
}

func TestRemovedPolicyKeysFollowItsTemplate(t *testing.T) {
	setRolloutDefaults(t, false, false, false)
	app := appWithAnnotations(nil)
	app.Labels = map[string]string{"gone/tag": "v1"}
	app.Status.ApplicationPoliciesConfigMap = policyConfigMapName(app.Namespace, app.Name)
	entry, err := json.Marshal(buildPolicyObservabilityData(app, RenderedPolicyResult{PolicyName: "gone", PolicyType: "gone", Enabled: true,
		Settings:   policySettings{settingApplyDataChanges: ptrBool(true)},
		Transforms: &PolicyOutput{Labels: map[string]string{"gone/tag": "v1"}}}))
	if err != nil {
		t.Fatal(err)
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: app.Status.ApplicationPoliciesConfigMap, Namespace: app.Namespace},
		Data:       map[string]string{"metadata": `{"labels": {"gone/tag": "v1"}}`, "001-gone": string(entry)},
	}
	h := &AppHandler{Client: fake.NewClientBuilder().WithObjects(cm).Build()}
	record, err := h.previousPolicyMetadata(monitorContext.NewTraceContext(context.Background(), ""), app, nil)
	if err != nil {
		t.Fatal(err)
	}
	h.removeStoppedPolicyMetadata(app, record, nil)
	if h.dataRemovedMetadata.Labels["gone/tag"] != "v1" {
		t.Fatalf("a removed policy whose template set applyDataChanges: true takes its keys off resources, got %+v", h.dataRemovedMetadata)
	}
}

func TestWithPendingRemovals(t *testing.T) {
	results := []RenderedPolicyResult{{PolicyType: "tags", Enabled: true,
		Transforms: &PolicyOutput{Labels: map[string]string{"back": "v2"}}}}
	pending := resourcekeeper.MetadataRemoval{Labels: map[string]string{"gone": "x", "back": "v1"}, Annotations: map[string]string{"note": "y"}}
	got := withPendingRemovals(resourcekeeper.MetadataRemoval{Labels: map[string]string{"new": "z"}}, pending, results)
	want := resourcekeeper.MetadataRemoval{Labels: map[string]string{"new": "z", "gone": "x"}, Annotations: map[string]string{"note": "y"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("a pending removal is carried unless a policy sets the key again: got %v, want %v", got, want)
	}
	if round := decodePendingRemovals(encodePendingRemovals(want)); !reflect.DeepEqual(round, want) {
		t.Fatalf("pending removals should survive the ConfigMap round trip: %v", round)
	}
}

func TestExtractPolicySettings(t *testing.T) {
	cases := []struct {
		name         string
		template     string
		want         policySettings
		wantWarnings int
	}{
		{"no config", `output: {}`, policySettings{}, 0},
		{"config without settings", `config: enabled: true`, policySettings{}, 0},
		{"all settings", `config: {autoRevision: true, applyPolicyUpdates: false, applyDataChanges: true}`,
			policySettings{settingAutoRevision: ptrBool(true), settingApplyPolicyUpdates: ptrBool(false), settingApplyDataChanges: ptrBool(true)}, 0},
		{"computed from context", `_ns: "prod"
config: applyPolicyUpdates: _ns != "prod"`, policySettings{settingApplyPolicyUpdates: ptrBool(false)}, 0},
		{"not a boolean is ignored with a warning", `config: {applyDataChanges: "yes", autoRevision: true}`,
			policySettings{settingAutoRevision: ptrBool(true)}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			val := cuecontext.New().CompileString(tc.template)
			if val.Err() != nil {
				t.Fatalf("bad test template: %v", val.Err())
			}
			got, warnings := extractPolicySettings(val)
			if !reflect.DeepEqual(got, tc.want) || len(warnings) != tc.wantWarnings {
				t.Fatalf("extractPolicySettings = %+v, %v; want %+v with %d warnings", got, warnings, tc.want, tc.wantWarnings)
			}
		})
	}
}

func TestPolicySettingPrecedence(t *testing.T) {
	cases := []struct {
		name        string
		annotation  *string
		fromPolicy  *bool
		flagDefault bool
		want        bool
	}{
		{"the policy's value beats the default", nil, ptrBool(true), false, true},
		{"the policy can turn a default off", nil, ptrBool(false), true, false},
		{"the annotation beats the policy", ptrString("false"), ptrBool(true), false, false},
		{"an annotation list beats the policy", ptrString("gwcp-tags"), ptrBool(false), false, true},
		{"an invalid annotation falls back to the policy", ptrString("Not_A_Name"), ptrBool(true), false, true},
		{"nothing set uses the default", nil, nil, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setRolloutDefaults(t, tc.flagDefault, tc.flagDefault, tc.flagDefault)
			var annotations map[string]string
			if tc.annotation != nil {
				annotations = map[string]string{oam.AnnotationApplyDataChanges: *tc.annotation}
			}
			if got := policySettingEnabledWith(appWithAnnotations(annotations), settingApplyDataChanges, "gwcp-tags", tc.fromPolicy); got != tc.want {
				t.Fatalf("policySettingEnabledWith = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTemplateSettingsReachResolution(t *testing.T) {
	setRolloutDefaults(t, false, false, false)
	app := appWithAnnotations(nil)
	results := []RenderedPolicyResult{
		{PolicyType: "rewrites", Enabled: true, Settings: policySettings{settingAutoRevision: ptrBool(true)},
			Transforms: &PolicyOutput{Components: []common.ApplicationComponent{{Name: "c"}}}},
		{PolicyType: "tags", Enabled: true, Settings: policySettings{settingApplyDataChanges: ptrBool(true)},
			Transforms: &PolicyOutput{Labels: map[string]string{"k": "v"}}},
		{PolicyType: "held-tags", Enabled: true, Transforms: &PolicyOutput{Labels: map[string]string{"held": "v"}}},
	}
	if !autoRevisionForResults(app, results) {
		t.Fatal("a spec-rewriting policy whose template sets autoRevision: true should cut a revision")
	}
	labels, _ := dataChangeMetadata(app, results)
	if !reflect.DeepEqual(labels, map[string]string{"k": "v"}) {
		t.Fatalf("only the policy whose template sets applyDataChanges: true should pass its labels, got %v", labels)
	}
	if !contextRefreshNeeded(app, map[string]string{"tags": "h1"}, map[string]string{"tags": "h2"}, results, nil) {
		t.Fatal("a context change from a policy whose template sets applyDataChanges: true should refresh")
	}
	if contextRefreshNeeded(app, map[string]string{"held-tags": "h1"}, map[string]string{"held-tags": "h2"}, results, nil) {
		t.Fatal("a context change from a policy without the setting should not refresh")
	}
}

func TestPolicyVersionMetadata(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableApplicationScopedPolicies, true)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableGlobalPolicies, true)
	useTestPolicyScopeIndex(t)

	const ns = "my-ns"
	policy := func(value string, revision int64) *v1beta1.PolicyDefinition {
		pd := &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "gp", Namespace: ns},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope, Global: true,
				Schematic: &common.Schematic{CUE: &common.CUE{Template: `parameter: {}, output: labels: {"v": "` + value + `"}`}},
			},
		}
		pd.Status.LatestRevision = &common.Revision{Name: fmt.Sprintf("gp-v%d", revision), Revision: revision}
		return pd
	}
	live := policy("v4", 4)
	policyScopeIndex.AddOrUpdate(live)
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns}}
	latestRev := &v1beta1.ApplicationRevision{ObjectMeta: metav1.ObjectMeta{Name: "app-v1", Namespace: ns}}
	// A revision stores definitions with their status cleared, and versions keyed as rendered.
	pinned := func(pd *v1beta1.PolicyDefinition) v1beta1.PolicyDefinition {
		pd.Status = v1beta1.PolicyDefinitionStatus{}
		return *pd
	}
	explicitPinned, explicitLive := policy("v3", 3), policy("v4", 4)
	for _, pd := range []*v1beta1.PolicyDefinition{explicitPinned, explicitLive} {
		pd.Name, pd.Namespace, pd.Spec.Global = "ep", oam.SystemDefinitionNamespace, false
		pd.Status.LatestRevision.Name = strings.Replace(pd.Status.LatestRevision.Name, "gp", "ep", 1)
	}
	app.Spec.Policies = []v1beta1.AppPolicy{{Name: "my-ep", Type: "ep"}}
	latestRev.Spec.PolicyDefinitions = map[string]v1beta1.PolicyDefinition{"gp": pinned(policy("v3", 3)), "ep": pinned(explicitPinned)}
	latestRev.Spec.PolicyVersions = map[string]v1beta1.PolicyVersionMetadata{
		"global:gp": {DefinitionRevisionName: "gp-v3", Revision: 3},
		"my-ep":     {DefinitionRevisionName: "ep-v3", Revision: 3},
	}
	policyScopeIndex.AddOrUpdate(explicitLive)
	cli := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(live, explicitLive).Build()
	monCtx := monitorContext.NewTraceContext(context.Background(), "test")

	revisionOf := func(results []RenderedPolicyResult, policyType string) int64 {
		for _, r := range results {
			if r.PolicyType == policyType {
				return r.Revision
			}
		}
		return -1
	}
	// labelOf is the "v" label the policy rendered, which names the version that rendered it.
	labelOf := func(results []RenderedPolicyResult, policyType string) string {
		for _, r := range results {
			if output, ok := r.Transforms.(*PolicyOutput); ok && output != nil && r.PolicyType == policyType {
				return output.Labels["v"]
			}
		}
		return ""
	}

	t.Run("a pinned global reports its pinned version", func(t *testing.T) {
		h := &AppHandler{Client: cli, app: app}
		results, err := h.renderPoliciesFromStoredRevision(monCtx, app, latestRev)
		if err != nil {
			t.Fatal(err)
		}
		if got := revisionOf(results, "gp"); got != 3 {
			t.Fatalf("pinned global rendered with revision %d, want 3", got)
		}
		if got := labelOf(results, "gp"); got != "v3" {
			t.Fatalf("pinned global rendered %q, want the pinned v3 template", got)
		}
		if got := revisionOf(results, "ep"); got != 3 {
			t.Fatalf("pinned explicit policy rendered with revision %d, want 3", got)
		}
		if got := labelOf(results, "ep"); got != "v3" {
			t.Fatalf("pinned explicit policy rendered %q, want the pinned v3 template", got)
		}
	})

	t.Run("an adopted update reports its new version", func(t *testing.T) {
		h := &AppHandler{Client: cli, app: app}
		overlay := overlayPolicies(app, latestRev, []string{"gp", "ep"}, map[string]*v1beta1.PolicyDefinition{"gp": live, "ep": explicitLive})
		results, err := h.renderPoliciesFromStoredRevision(monCtx, app, overlay)
		if err != nil {
			t.Fatal(err)
		}
		if got := revisionOf(results, "gp"); got != 4 {
			t.Fatalf("adopted global rendered with revision %d, want 4", got)
		}
		if got := labelOf(results, "gp"); got != "v4" {
			t.Fatalf("adopted global rendered %q, want the live v4 template", got)
		}
		if got := revisionOf(results, "ep"); got != 4 {
			t.Fatalf("adopted explicit policy rendered with revision %d, want 4", got)
		}
		if got := labelOf(results, "ep"); got != "v4" {
			t.Fatalf("adopted explicit policy rendered %q, want the live v4 template", got)
		}
	})
}

func ptrString(s string) *string { return &s }

func TestUnchangedSinceRevision(t *testing.T) {
	rev := func(generation int64) *v1beta1.ApplicationRevision {
		r := &v1beta1.ApplicationRevision{}
		if generation != 0 {
			r.SetAnnotations(map[string]string{oam.AnnotationAppGeneration: fmt.Sprint(generation)})
		}
		return r
	}
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Generation: 3}}
	if !unchangedSinceRevision(app, rev(3)) {
		t.Fatal("same generation: the spec difference comes from policy transforms")
	}
	if unchangedSinceRevision(app, rev(2)) {
		t.Fatal("an older generation means the author edited the spec")
	}
	if unchangedSinceRevision(app, rev(0)) {
		t.Fatal("a revision without a generation is not judged")
	}
}

func TestDecodePolicyMetadataRecord(t *testing.T) {
	data := map[string]string{
		"metadata":       `{"labels": {"gwcp/tag": "v1", "team": "platform"}, "annotations": {"note": "a"}, "context": {}}`,
		"001-gwcp-tags":  `{"name": "gwcp-tags", "type": "gwcp-tags", "output": {"labels": {"gwcp/tag": "v1"}, "annotations": {"note": "a"}}}`,
		"002-team":       `{"name": "my-team", "type": "team-labels@v2", "output": {"labels": {"team": "platform"}}}`,
		"003-older-form": `{"name": "older", "output": {"labels": {"older": "x"}}}`,
		"rendered_spec":  `{}`,
	}
	record := decodePolicyMetadataRecord(data)
	if !reflect.DeepEqual(record.labels, map[string]recordedKey{
		"gwcp/tag": {value: "v1", owner: "gwcp-tags"},
		"team":     {value: "platform", owner: "team-labels"},
	}) {
		t.Fatalf("labels = %+v", record.labels)
	}
	if !reflect.DeepEqual(record.annotations, map[string]recordedKey{"note": {value: "a", owner: "gwcp-tags"}}) {
		t.Fatalf("annotations = %+v", record.annotations)
	}
	if empty := decodePolicyMetadataRecord(map[string]string{}); len(empty.labels) != 0 || len(empty.annotations) != 0 {
		t.Fatalf("a missing metadata entry records nothing, got %+v", empty)
	}
}

func TestRemovedPolicyMetadata(t *testing.T) {
	record := policyMetadataRecord{
		labels: map[string]recordedKey{
			"dropped":         {value: "v1", owner: "p"},
			"kept":            {value: "v1", owner: "p"},
			"changed-by-hand": {value: "v1", owner: "p"},
		},
		annotations: map[string]recordedKey{"note": {value: "a", owner: "q"}},
	}
	current := []RenderedPolicyResult{
		{PolicyType: "p", Enabled: true, Transforms: &PolicyOutput{Labels: map[string]string{"kept": "v2"}}},
	}
	app := appWithAnnotations(nil)
	app.Labels = map[string]string{"dropped": "v1", "kept": "v1", "changed-by-hand": "mine"}
	app.Annotations = map[string]string{"note": "a"}

	removed := removedPolicyMetadata(app, record, current)
	if !reflect.DeepEqual(removed.labels, map[string]recordedKey{"dropped": {value: "v1", owner: "p"}}) {
		t.Fatalf("removed labels = %+v: only a key the policies no longer set, still at their value, is removed", removed.labels)
	}
	if !reflect.DeepEqual(removed.annotations, map[string]recordedKey{"note": {value: "a", owner: "q"}}) {
		t.Fatalf("removed annotations = %+v", removed.annotations)
	}
}

// allowNamespaceGlobals lets global policies outside vela-system apply for the duration of t.
func allowNamespaceGlobals(t *testing.T) {
	t.Helper()
	orig := common2.AllowNamespaceGlobalPolicies
	t.Cleanup(func() { common2.AllowNamespaceGlobalPolicies = orig })
	common2.AllowNamespaceGlobalPolicies = true
}

func TestGlobalsOnlyFromSystemNamespace(t *testing.T) {
	global := func(name, namespace string) *v1beta1.PolicyDefinition {
		return &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       v1beta1.PolicyDefinitionSpec{Scope: v1beta1.ApplicationScope, Global: true},
		}
	}
	sys := oam.SystemDefinitionNamespace
	names := func(policies []*PolicyMetadata) []string {
		var out []string
		for _, p := range policies {
			out = append(out, p.Namespace+"/"+p.Name)
		}
		sort.Strings(out)
		return out
	}
	setup := func(t *testing.T) {
		useTestPolicyScopeIndex(t)
		for _, pd := range []*v1beta1.PolicyDefinition{global("tags", sys), global("tags", "tenant"), global("extra", "tenant")} {
			policyScopeIndex.AddOrUpdate(pd)
		}
	}

	t.Run("by default only vela-system globals apply", func(t *testing.T) {
		orig := common2.AllowNamespaceGlobalPolicies
		t.Cleanup(func() { common2.AllowNamespaceGlobalPolicies = orig })
		common2.AllowNamespaceGlobalPolicies = false
		setup(t)
		if got := names(policyScopeIndex.GetGlobalApplicationPoliciesDeduped("tenant")); !reflect.DeepEqual(got, []string{sys + "/tags"}) {
			t.Fatalf("applied = %v", got)
		}
		if got := names(policyScopeIndex.IgnoredNamespaceGlobals("tenant")); !reflect.DeepEqual(got, []string{"tenant/extra", "tenant/tags"}) {
			t.Fatalf("ignored = %v", got)
		}
	})

	t.Run("allowed namespace globals apply and override as before", func(t *testing.T) {
		allowNamespaceGlobals(t)
		setup(t)
		if got := names(policyScopeIndex.GetGlobalApplicationPoliciesDeduped("tenant")); !reflect.DeepEqual(got, []string{"tenant/extra", "tenant/tags"}) {
			t.Fatalf("applied = %v", got)
		}
		if got := policyScopeIndex.IgnoredNamespaceGlobals("tenant"); len(got) != 0 {
			t.Fatalf("nothing is ignored when namespace globals are allowed, got %v", names(got))
		}
	})
}

func TestEmitIgnoredNamespaceGlobals(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableGlobalPolicies, true)
	orig := common2.AllowNamespaceGlobalPolicies
	t.Cleanup(func() { common2.AllowNamespaceGlobalPolicies = orig })
	common2.AllowNamespaceGlobalPolicies = false
	useTestPolicyScopeIndex(t)
	policyScopeIndex.AddOrUpdate(&v1beta1.PolicyDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "tags", Namespace: "tenant"},
		Spec:       v1beta1.PolicyDefinitionSpec{Scope: v1beta1.ApplicationScope, Global: true},
	})
	rec := &recordingRecorder{}
	r := &Reconciler{Recorder: rec}
	r.emitIgnoredNamespaceGlobals(&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "tenant"}})
	if len(rec.events) != 1 || rec.events[0].Reason != "GlobalPolicyIgnored" || rec.events[0].Type != event.TypeWarning ||
		!strings.Contains(rec.events[0].Message, "tenant/tags") || !strings.Contains(rec.events[0].Message, oam.SystemDefinitionNamespace) {
		t.Fatalf("expected one GlobalPolicyIgnored warning naming tenant/tags, got %+v", rec.events)
	}
}

func TestSkipGlobalPoliciesPermission(t *testing.T) {
	skipping := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "tenant",
		Annotations: map[string]string{SkipGlobalPoliciesAnnotation: "true"}}}
	set := func(t *testing.T, allowed bool) {
		orig := common2.AllowSkipGlobalPolicies
		t.Cleanup(func() { common2.AllowSkipGlobalPolicies = orig })
		common2.AllowSkipGlobalPolicies = allowed
	}

	t.Run("the annotation is ignored when skips are not permitted", func(t *testing.T) {
		set(t, false)
		if shouldSkipGlobalPolicies(skipping) {
			t.Fatal("global policies are mandatory: the skip annotation must be ignored")
		}
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableGlobalPolicies, true)
		rec := &recordingRecorder{}
		(&Reconciler{Recorder: rec}).emitIgnoredGlobalSkip(skipping)
		if len(rec.events) != 1 || rec.events[0].Reason != "GlobalPolicySkipIgnored" || rec.events[0].Type != event.TypeWarning {
			t.Fatalf("expected one GlobalPolicySkipIgnored warning, got %+v", rec.events)
		}
	})

	t.Run("the annotation skips when permitted", func(t *testing.T) {
		set(t, true)
		if !shouldSkipGlobalPolicies(skipping) {
			t.Fatal("a permitted skip must skip global policies")
		}
		rec := &recordingRecorder{}
		(&Reconciler{Recorder: rec}).emitIgnoredGlobalSkip(skipping)
		if len(rec.events) != 0 {
			t.Fatalf("no warning when the skip is permitted, got %+v", rec.events)
		}
	})
}

func TestMandatoryGlobalRenderFailure(t *testing.T) {
	broken := &v1beta1.PolicyDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "gwcp-metadata", Namespace: oam.SystemDefinitionNamespace},
		Spec: v1beta1.PolicyDefinitionSpec{Scope: v1beta1.ApplicationScope, Global: true,
			Schematic: &common.Schematic{CUE: &common.CUE{Template: "parameter: {}\noutput: labels: {tenant: undefinedReference}\n"}}},
	}
	render := func(t *testing.T, skipsAllowed bool) ([]RenderedPolicyResult, error) {
		orig := common2.AllowSkipGlobalPolicies
		t.Cleanup(func() { common2.AllowSkipGlobalPolicies = orig })
		common2.AllowSkipGlobalPolicies = skipsAllowed
		app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "tenant"}}
		h := &AppHandler{Client: fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build(), app: app}
		return h.renderPoliciesInSequence(monitorContext.NewTraceContext(context.Background(), "test"), app, []policyToRender{{
			policyDef: broken, policyRef: v1beta1.AppPolicy{Name: broken.Name, Type: broken.Name},
			source: PolicySourceGlobal, versionKey: "global:" + broken.Name,
		}})
	}

	t.Run("a mandatory global that fails to render fails the reconcile", func(t *testing.T) {
		_, err := render(t, false)
		if err == nil || !strings.Contains(err.Error(), "gwcp-metadata") || !strings.Contains(err.Error(), "mandatory") {
			t.Fatalf("expected an error naming the mandatory global, got %v", err)
		}
	})

	t.Run("when skips are permitted it is skipped as before", func(t *testing.T) {
		results, err := render(t, true)
		if err != nil || len(results) != 1 || !results[0].IsError || results[0].Enabled {
			t.Fatalf("expected a skipped, errored result and no error, got %+v, %v", results, err)
		}
	})
}

func TestCheckPolicyContextReads(t *testing.T) {
	cases := map[string]struct {
		template string
		unknown  string
	}{
		"supplied fields":         {template: `output: labels: {a: context.appName, b: context.appLabels["team"], c: context["namespace"]}`},
		"custom":                  {template: `output: labels: a: context.custom.team`},
		"a provider reading one":  {template: "import \"vela/kube\"\n_ns: kube.#Get & {$params: resource: metadata: name: context.namespace}\noutput: labels: t: _ns.$returns.metadata.labels.t"},
		"another value's context": {template: `_x: context: appNmae: "a"` + "\noutput: labels: a: _x.context.appNmae"},
		"a misspelled field":      {template: `output: labels: a: context.appNmae`, unknown: "context.appNmae"},
		"a misspelled index":      {template: `output: labels: a: context["nmaespace"]`, unknown: "context.nmaespace"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := checkPolicyContextReads(tc.template)
			if tc.unknown == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.unknown) {
				t.Fatalf("expected an error naming %s, got %v", tc.unknown, err)
			}
		})
	}
}
