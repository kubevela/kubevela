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

package sources

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/oam-dev/kubevela/pkg/cue/render"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	apitypes "github.com/oam-dev/kubevela/apis/types"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
)

func identityMeta() velaprocess.SourceCacheWriteMeta {
	return velaprocess.SourceCacheWriteMeta{
		SourceDefName:      "tenant-data",
		SourceDefNamespace: "team-a",
		KeyInputs:          []string{"cluster", "namespace", "appLabels[region]"},
		Context: map[string]interface{}{
			"cluster":   "prod-cluster",
			"namespace": "team-a",
			"appLabels": map[string]interface{}{"region": "eu-west"},
		},
		Properties:   map[string]interface{}{"image": "nginx:1.25.0"},
		TemplateHash: "4f2a1c9d",
	}
}

func applied(meta velaprocess.SourceCacheWriteMeta) *corev1.Secret {
	secret := &corev1.Secret{}
	ApplySourceCacheMetadata(secret, "tenant-data", meta)
	return secret
}

// The point of the labels: find every entry for a source on a cluster with a
// selector, rather than listing and filtering by hand.
func TestCacheEntryIsSelectableByItsIdentity(t *testing.T) {
	got := applied(identityMeta()).GetLabels()

	for key, want := range map[string]string{
		apitypes.LabelSourceDefinitionName:            "tenant-data",
		apitypes.LabelSourceContextPrefix + "cluster": "prod-cluster",
		// An indexed read folds the index into the label name, so a specific
		// label value is selectable too.
		apitypes.LabelSourceContextPrefix + "appLabels.region": "eu-west",
	} {
		if got[key] != want {
			t.Errorf("expected label %s=%s, got %q", key, want, got[key])
		}
	}
}

// Every emitted label has to be legal, or the write fails and the entry is never
// persisted at all - a far worse outcome than a missing label.
func TestCacheEntryLabelsAreAlwaysLegal(t *testing.T) {
	meta := identityMeta()
	meta.Context = map[string]interface{}{
		"cluster": "prod-cluster",
		// An index containing a slash would put a second one in the label key.
		"appLabels": map[string]interface{}{
			"example.org/service-name": "checkout",
			// A value legal as a label value but not as one here.
			"owner": "team a/platform",
			"team":  "platform",
		},
		// A struct cannot be rendered into a label value at all.
		"clusterVersion": map[string]interface{}{"major": "1", "minor": "31"},
	}

	labels := applied(meta).GetLabels()
	for k, v := range labels {
		if errs := validation.IsQualifiedName(k); len(errs) > 0 {
			t.Errorf("label key %q is not valid: %v", k, errs)
		}
		if errs := validation.IsValidLabelValue(v); len(errs) > 0 {
			t.Errorf("label value %q (key %q) is not valid: %v", v, k, errs)
		}
	}

	// The legal ones survive; the rest are simply absent.
	if labels[apitypes.LabelSourceContextPrefix+"appLabels.team"] != "platform" {
		t.Error("a label-safe indexed read should still be selectable")
	}
	for _, absent := range []string{"appLabels.example.org/service-name", "appLabels.owner", "clusterVersion"} {
		if _, ok := labels[apitypes.LabelSourceContextPrefix+absent]; ok {
			t.Errorf("%s cannot be a legal label and must be skipped", absent)
		}
	}
}

// Nothing is lost when a value cannot be a label: the annotation carries the
// whole context, which is what makes skipping safe rather than lossy.
func TestCacheEntryAnnotationCarriesTheFullContext(t *testing.T) {
	meta := identityMeta()
	annotations := applied(meta).GetAnnotations()

	var ctx map[string]interface{}
	if err := json.Unmarshal([]byte(annotations[apitypes.AnnotationSourceContext]), &ctx); err != nil {
		t.Fatalf("context annotation is not readable JSON: %v", err)
	}
	labels, _ := ctx["appLabels"].(map[string]interface{})
	if labels["region"] != "eu-west" {
		t.Errorf("expected the indexed read in the annotation, got %v", ctx["appLabels"])
	}

	var inputs []string
	if err := json.Unmarshal([]byte(annotations[apitypes.AnnotationSourceKeyInputs]), &inputs); err != nil {
		t.Fatalf("key-inputs annotation is not readable JSON: %v", err)
	}
	if len(inputs) != 3 || inputs[2] != "appLabels[region]" {
		t.Errorf("expected the key inputs verbatim, got %v", inputs)
	}

	if annotations[apitypes.AnnotationSourceTemplateHash] != "4f2a1c9d" {
		t.Errorf("expected the template fingerprint, got %q", annotations[apitypes.AnnotationSourceTemplateHash])
	}

	var props map[string]interface{}
	if err := json.Unmarshal([]byte(annotations[apitypes.AnnotationSourceProperties]), &props); err != nil {
		t.Fatalf("properties annotation is not readable JSON: %v", err)
	}
	if props["image"] != "nginx:1.25.0" {
		t.Errorf("expected the binding's properties, got %v", props)
	}
}

// Large properties must not blow the object's annotation budget, and a clipped
// value has to announce itself - otherwise it reads as the real thing when
// someone compares two entries.
//
// Crucially the result must still parse. Clipping the finished JSON document
// would satisfy the size cap and leave something no reader can use, which is
// worse than recording nothing.
func TestCacheEntryClampsLargeProperties(t *testing.T) {
	meta := identityMeta()
	meta.Properties = map[string]interface{}{
		"image": "nginx:1.25.0",
		"blob":  strings.Repeat("x", render.MaxAnnotationValueLen*2),
	}

	annotations := applied(meta).GetAnnotations()
	raw := annotations[apitypes.AnnotationSourceProperties]

	if len(raw) > render.MaxAnnotationValueLen {
		t.Fatalf("properties annotation is %d bytes, over the %d cap", len(raw), render.MaxAnnotationValueLen)
	}
	if annotations[apitypes.AnnotationSourcePropertiesTruncated] != "true" {
		t.Error("a clipped value must be marked, or it reads as complete")
	}

	var props map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &props); err != nil {
		t.Fatalf("a clamped annotation must still be readable JSON, got %v for: %s", err, raw)
	}
	// The small property survives intact - only the oversized one is replaced,
	// so the annotation still says what distinguishes this entry.
	if props["image"] != "nginx:1.25.0" {
		t.Errorf("a large property crowded out a small one: %v", props)
	}
	blob, _ := props["blob"].(string)
	if !strings.HasPrefix(blob, "<omitted:") {
		t.Errorf("expected the oversized value to be replaced by a placeholder, got %q", blob)
	}
}

// The other way to exceed the budget: many small properties rather than one
// large one. The names alone are still valid JSON and still informative.
func TestCacheEntryClampsManyProperties(t *testing.T) {
	meta := identityMeta()
	meta.Properties = map[string]interface{}{}
	for i := 0; i < 500; i++ {
		meta.Properties[fmt.Sprintf("property-number-%03d", i)] = "value"
	}

	annotations := applied(meta).GetAnnotations()
	raw := annotations[apitypes.AnnotationSourceProperties]

	if len(raw) > render.MaxAnnotationValueLen {
		t.Fatalf("properties annotation is %d bytes, over the %d cap", len(raw), render.MaxAnnotationValueLen)
	}
	if annotations[apitypes.AnnotationSourcePropertiesTruncated] != "true" {
		t.Error("expected the truncation marker")
	}
	var names []string
	if err := json.Unmarshal([]byte(raw), &names); err != nil {
		t.Fatalf("expected a readable JSON array of names, got %v for: %s", err, raw)
	}
	if len(names) == 0 {
		t.Fatal("expected as many names as fit the cap, got none")
	}
	// Sorted and filled to the cap, so two entries written at different times
	// list the same prefix and can be compared.
	if names[0] != "property-number-000" {
		t.Errorf("expected a stable sorted prefix, got %v", names[:1])
	}
	if !sort.StringsAreSorted(names) {
		t.Error("the recorded names must be sorted, or the annotation churns between writes")
	}
}

// Recording only identity inputs is what makes this correct. A cache entry is
// shared by every binding that resolves to it, so the resolved output - which
// differs per definition version and is what +sensitive protects - stays in the
// Secret's data, where encryption-at-rest covers it. Metadata is not encrypted.
func TestCacheEntryMetadataExcludesResolvedOutput(t *testing.T) {
	secret := applied(identityMeta())

	for k, v := range secret.GetAnnotations() {
		if strings.Contains(v, "resolved-secret-value") {
			t.Errorf("annotation %s leaked resolved output", k)
		}
	}
	if _, ok := secret.GetAnnotations()["sourcedefinition.oam.dev/output"]; ok {
		t.Error("resolved output must stay in the Secret's data, not its metadata")
	}
}

// An entry written before this metadata existed, or by a store that has none of
// it, must still be written rather than rejected.
func TestCacheEntryMetadataIsOptional(t *testing.T) {
	secret := applied(velaprocess.SourceCacheWriteMeta{SourceDefName: "bare"})

	if secret.GetLabels()[apitypes.LabelConfigCatalog] != apitypes.VelaCoreConfig {
		t.Error("the catalog label is what makes an entry findable at all and must always be set")
	}
	for _, key := range []string{
		apitypes.AnnotationSourceKeyInputs,
		apitypes.AnnotationSourceContext,
		apitypes.AnnotationSourceProperties,
	} {
		if _, ok := secret.GetAnnotations()[key]; ok {
			t.Errorf("%s should be absent rather than empty when there is nothing to record", key)
		}
	}
}
