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

package cli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	v1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestFilterPolicies(t *testing.T) {
	policies := []common.AppliedApplicationPolicy{
		{Name: "add-env-labels"},
		{Name: "add-env-annotations"},
		{Name: "governance"},
		{Name: "tenant-context"},
	}

	t.Run("exact match", func(t *testing.T) {
		got := filterPolicies(policies, "governance")
		require.Len(t, got, 1)
		assert.Equal(t, "governance", got[0].Name)
	})

	t.Run("glob prefix", func(t *testing.T) {
		got := filterPolicies(policies, "add-env-*")
		require.Len(t, got, 2)
		assert.Equal(t, "add-env-labels", got[0].Name)
		assert.Equal(t, "add-env-annotations", got[1].Name)
	})

	t.Run("no match returns empty", func(t *testing.T) {
		got := filterPolicies(policies, "nonexistent")
		assert.Empty(t, got)
	})

	t.Run("empty pattern matches nothing via path.Match", func(t *testing.T) {
		// path.Match("", x) only matches empty string
		got := filterPolicies(policies, "")
		assert.Empty(t, got)
	})

	t.Run("wildcard matches all", func(t *testing.T) {
		got := filterPolicies(policies, "*")
		assert.Len(t, got, len(policies))
	})
}

func TestBuildPolicyDetailsFromConfigMap(t *testing.T) {
	t.Run("nil configmap returns nil", func(t *testing.T) {
		assert.Nil(t, buildPolicyDetailsFromConfigMap(nil))
	})

	t.Run("empty configmap returns empty map", func(t *testing.T) {
		cm := &corev1.ConfigMap{}
		got := buildPolicyDetailsFromConfigMap(cm)
		assert.Empty(t, got)
	})

	t.Run("skips reserved keys", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				"info":          `{"rendered_at":"2026-01-01T00:00:00Z"}`,
				"rendered_spec": `{}`,
				"applied_spec":  `{}`,
				"metadata":      `{}`,
			},
		}
		got := buildPolicyDetailsFromConfigMap(cm)
		assert.Empty(t, got)
	})

	t.Run("parses policy entry with labels and context", func(t *testing.T) {
		entry := map[string]any{
			"name":                   "governance",
			"namespace":              "vela-system",
			"priority":               int32(5),
			"definitionRevisionName": "governance-v1",
			"revision":               int64(1),
			"revisionHash":           "abc123",
			"output": map[string]any{
				"labels":      map[string]string{"env": "prod"},
				"annotations": map[string]string{"team": "platform"},
				"ctx":         map[string]any{"tenant": "acme"},
			},
		}
		raw, _ := json.Marshal(entry)
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				"001-governance": string(raw),
			},
		}

		got := buildPolicyDetailsFromConfigMap(cm)
		require.Contains(t, got, "governance")
		d := got["governance"]

		assert.Equal(t, "governance-v1", d["definitionRevisionName"])

		output, ok := d["output"].(map[string]interface{})
		require.True(t, ok)

		labels, ok := output["labels"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "prod", labels["env"])

		annotations, ok := output["annotations"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "platform", annotations["team"])

		ctx, ok := output["ctx"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "acme", ctx["tenant"])
	})

	t.Run("parses spec before/after", func(t *testing.T) {
		beforeRaw := json.RawMessage(`{"image":"nginx:v1"}`)
		afterRaw := json.RawMessage(`{"image":"nginx:v2"}`)
		entry := map[string]any{
			"name":      "patch-policy",
			"namespace": "default",
			"output": map[string]any{
				"spec": map[string]any{
					"before": &beforeRaw,
					"after":  &afterRaw,
				},
			},
		}
		raw, _ := json.Marshal(entry)
		cm := &corev1.ConfigMap{
			Data: map[string]string{"001-patch-policy": string(raw)},
		}

		got := buildPolicyDetailsFromConfigMap(cm)
		require.Contains(t, got, "patch-policy")
		output := got["patch-policy"]["output"].(map[string]interface{})
		spec, ok := output["spec"].(map[string]interface{})
		require.True(t, ok)
		assert.NotNil(t, spec["before"])
		assert.NotNil(t, spec["after"])
	})

	t.Run("skips malformed JSON entries", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				"001-bad-policy": `{not valid json`,
			},
		}
		got := buildPolicyDetailsFromConfigMap(cm)
		assert.Empty(t, got)
	})
}

func TestExtractOutcomeFromConfigMap(t *testing.T) {
	fallbackSpec := v1beta1.ApplicationSpec{}
	fallbackLabels := map[string]string{"controller-label": "yes"}
	fallbackAnnotations := map[string]string{"controller-annotation": "yes"}

	t.Run("nil configmap uses fallbacks", func(t *testing.T) {
		spec, labels, annotations, ctx := extractOutcomeFromConfigMap(nil, fallbackSpec, fallbackLabels, fallbackAnnotations)
		assert.Equal(t, fallbackSpec, spec)
		assert.Equal(t, fallbackLabels, labels)
		assert.Equal(t, fallbackAnnotations, annotations)
		assert.Nil(t, ctx)
	})

	t.Run("metadata present uses policy-contributed values not fallback", func(t *testing.T) {
		meta := map[string]any{
			"labels":      map[string]string{"policy-label": "true"},
			"annotations": map[string]string{"policy-annotation": "true"},
			"context":     map[string]any{"tenant": "acme"},
		}
		metaJSON, _ := json.Marshal(meta)
		cm := &corev1.ConfigMap{
			Data: map[string]string{"metadata": string(metaJSON)},
		}

		_, labels, annotations, ctx := extractOutcomeFromConfigMap(cm, fallbackSpec, fallbackLabels, fallbackAnnotations)
		assert.Equal(t, map[string]string{"policy-label": "true"}, labels)
		assert.Equal(t, map[string]string{"policy-annotation": "true"}, annotations)
		assert.Equal(t, "acme", ctx["tenant"])
		// Must NOT include controller-added values
		assert.NotContains(t, labels, "controller-label")
		assert.NotContains(t, annotations, "controller-annotation")
	})

	t.Run("empty metadata labels/annotations returns empty maps not fallback", func(t *testing.T) {
		meta := map[string]any{
			"labels":      map[string]string{},
			"annotations": map[string]string{},
			"context":     map[string]any{},
		}
		metaJSON, _ := json.Marshal(meta)
		cm := &corev1.ConfigMap{
			Data: map[string]string{"metadata": string(metaJSON)},
		}

		_, labels, annotations, ctx := extractOutcomeFromConfigMap(cm, fallbackSpec, fallbackLabels, fallbackAnnotations)
		assert.Empty(t, labels)
		assert.Empty(t, annotations)
		assert.Nil(t, ctx)
	})

	t.Run("missing metadata key uses fallbacks", func(t *testing.T) {
		cm := &corev1.ConfigMap{Data: map[string]string{}}
		_, labels, annotations, _ := extractOutcomeFromConfigMap(cm, fallbackSpec, fallbackLabels, fallbackAnnotations)
		assert.Equal(t, fallbackLabels, labels)
		assert.Equal(t, fallbackAnnotations, annotations)
	})
}

func TestBuildPolicyOutput(t *testing.T) {
	policies := []common.AppliedApplicationPolicy{
		{Name: "p1", Type: "governance", Applied: true, LabelsCount: 2, SpecModified: true},
		{Name: "p2", Type: "tenant-context", Applied: false},
	}
	spec := v1beta1.ApplicationSpec{}
	labels := map[string]string{"k": "v"}
	annotations := map[string]string{"a": "b"}

	t.Run("summary counts are correct", func(t *testing.T) {
		out := buildPolicyOutput("my-app", "default", policies, spec, labels, annotations, nil, false, nil, nil)
		summary := out["summary"].(map[string]any)
		assert.Equal(t, 1, summary["enabled"])
		assert.Equal(t, 1, summary["disabled"])
		assert.Equal(t, 2, summary["labelsAdded"])
		assert.Equal(t, 1, summary["specModifications"])
	})

	t.Run("outcome block absent when outcome=false", func(t *testing.T) {
		out := buildPolicyOutput("my-app", "default", policies, spec, labels, annotations, nil, false, nil, nil)
		assert.NotContains(t, out, "outcome")
	})

	t.Run("outcome block present when outcome=true", func(t *testing.T) {
		out := buildPolicyOutput("my-app", "default", policies, spec, labels, annotations, nil, true, nil, nil)
		require.Contains(t, out, "outcome")
		outcome := out["outcome"].(map[string]any)
		assert.Equal(t, labels, outcome["labels"])
		assert.Equal(t, annotations, outcome["annotations"])
	})

	t.Run("outcome block includes context when non-empty", func(t *testing.T) {
		finalCtx := map[string]interface{}{"tenant": "acme"}
		out := buildPolicyOutput("my-app", "default", policies, spec, labels, annotations, finalCtx, true, nil, nil)
		outcome := out["outcome"].(map[string]any)
		assert.Equal(t, finalCtx, outcome["context"])
	})

	t.Run("errors field present when non-empty", func(t *testing.T) {
		out := buildPolicyOutput("my-app", "default", policies, spec, labels, annotations, nil, false, []string{"something failed"}, nil)
		require.Contains(t, out, "errors")
		assert.Equal(t, []string{"something failed"}, out["errors"])
	})

	t.Run("policyDetails merged into policy entry", func(t *testing.T) {
		details := map[string]map[string]any{
			"p1": {
				"priority": int32(3),
				"output":   map[string]any{"ctx": map[string]any{"tenant": "acme"}},
			},
		}
		out := buildPolicyOutput("my-app", "default", policies, spec, labels, annotations, nil, false, nil, details)
		merged := out["policies"].([]map[string]any)
		p1 := merged[0]
		assert.Equal(t, int32(3), p1["priority"])
		assert.NotNil(t, p1["output"])
	})
}
