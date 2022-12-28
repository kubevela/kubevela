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

package definition

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	addonutils "github.com/oam-dev/kubevela/pkg/utils/addon"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/filters"
)

func TestDefinitionBasicFunctions(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	def := &Definition{Unstructured: unstructured.Unstructured{}}
	def.SetAnnotations(map[string]string{
		UserPrefix + "annotation": "annotation",
		"other":                   "other",
	})
	def.SetLabels(map[string]string{
		UserPrefix + "label": "label",
		"other":              "other",
	})
	def.SetName("test-trait")
	def.SetGVK("TraitDefinition")
	def.SetOwnerReferences([]v1.OwnerReference{{
		Name: addonutils.Addon2AppName("test-addon"),
	}})
	if _type := def.GetType(); _type != "trait" {
		t.Fatalf("set gvk invalid, expected trait got %s", _type)
	}
	if err := def.SetType("abc"); err == nil {
		t.Fatalf("set type should failed due to invalid type, but got no error")
	}
	def.Object["spec"] = GetDefinitionDefaultSpec("TraitDefinition")
	_ = unstructured.SetNestedField(def.Object, "patch: metadata: labels: \"KubeVela-test\": parameter.tag\nparameter: tag: string\n", "spec", "schematic", "cue", "template")
	cueString, err := def.ToCUEString()
	if err != nil {
		t.Fatalf("unexpected error when getting to cue: %v", err)
	}
	trait := &v1beta1.TraitDefinition{}
	s, _ := json.Marshal(def.Object)
	_ = json.Unmarshal(s, trait)
	if err = c.Create(context.Background(), trait); err != nil {
		t.Fatalf("unexpected error when creating new definition with fake client: %v", err)
	}
	if err = def.FromCUEString("abc:]{xa}", nil); err == nil {
		t.Fatalf("should encounter invalid cue string but not found error")
	}
	if err = def.FromCUEString(cueString+"abc: {xa}", nil); err == nil {
		t.Fatalf("should encounter invalid cue string but not found error")
	}
	parts := strings.Split(cueString, "template: ")
	if err = def.FromCUEString(parts[0], nil); err == nil {
		t.Fatalf("should encounter no template found error but not found error")
	}
	if err = def.FromCUEString("template:"+parts[1], nil); err == nil {
		t.Fatalf("should encounter no metadata found error but not found error")
	}
	if err = def.FromCUEString("import \"strconv\"\n"+cueString, nil); err != nil {
		t.Fatalf("should not encounter cue compile error due to useless import")
	}
	if err = def.FromCUEString("abc: {}\n"+cueString, nil); err == nil {
		t.Fatalf("should encounter duplicated object name error but not found error")
	}
	if err = def.FromCUEString(strings.Replace(cueString, "\"trait\"", "\"tr\"", 1), nil); err == nil {
		t.Fatalf("should encounter invalid type error but not found error")
	}
	if err = def.FromCUEString(cueString, nil); err != nil {
		t.Fatalf("unexpected error when setting from cue: %v", err)
	}
	if _cueString, err := def.ToCUEString(); err != nil {
		t.Fatalf("failed to generate cue string: %v", err)
	} else if _cueString != cueString {
		t.Fatalf("the bidirectional conversion of cue string is not idempotent")
	}
	templateString, _, _ := unstructured.NestedString(def.Object, DefinitionTemplateKeys...)
	_ = unstructured.SetNestedField(def.Object, "import \"strconv\"\n"+templateString, DefinitionTemplateKeys...)
	if s, err := def.ToCUEString(); err != nil {
		t.Fatalf("failed to generate cue string: %v", err)
	} else if !strings.Contains(s, "import \"strconv\"\n") {
		t.Fatalf("definition ToCUEString missed import, val: %v", s)
	}
	def = &Definition{}
	if err = def.FromCUEString(cueString, nil); err != nil {
		t.Fatalf("unexpected error when setting from cue for empty def: %v", err)
	}

	// test other definition default spec
	_ = GetDefinitionDefaultSpec("ComponentDefinition")
	_ = GetDefinitionDefaultSpec("WorkloadDefinition")
	_ = ValidDefinitionTypes()

	if _, err = SearchDefinition(c, "", ""); err != nil {
		t.Fatalf("failed to search definition: %v", err)
	}
	if _, err = SearchDefinition(c, "trait", "default"); err != nil {
		t.Fatalf("failed to search definition: %v", err)
	}
	res, err := SearchDefinition(c, "", "", filters.ByOwnerAddon("test-addon"))
	if err != nil {
		t.Fatalf("failed to search definition: %v", err)
	}
	if len(res) < 1 {
		t.Fatalf("failed to search definition with addon filter applied: %s", "no result returned")
	}
	res, err = SearchDefinition(c, "", "", filters.ByName("test-trait"), filters.ByOwnerAddon("test-addon"))
	if err != nil {
		t.Fatalf("failed to search definition: %v", err)
	}
	if len(res) < 1 {
		t.Fatalf("failed to search definition with addon filter applied: %s", "no result returned")
	}
	res, err = SearchDefinition(c, "", "", filters.ByOwnerAddon("this-is-a-non-existent-addon"))
	if err != nil {
		t.Fatalf("failed to search definition: %v", err)
	}
	if len(res) >= 1 {
		t.Fatalf("failed to search definition with addon filter applied: %s", "too many results returned")
	}
}

func TestDefinitionRevisionSearch(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(common.Scheme).Build()

	var err error

	// Load test DefinitionRevisions files into client
	testFiles, err := os.ReadDir("testdata")
	assert.NoError(t, err, "read testdata failed")
	for _, file := range testFiles {
		if !strings.HasSuffix(file.Name(), ".yaml") {
			continue
		}
		content, err := os.ReadFile(filepath.Join("testdata", file.Name()))
		assert.NoError(t, err)
		def := &v1beta1.DefinitionRevision{}
		err = yaml.Unmarshal(content, def)
		assert.NoError(t, err)
		err = c.Create(context.TODO(), def)
		assert.NoError(t, err, "cannot create "+file.Name())
	}

	var defrevs []v1beta1.DefinitionRevision

	// Read with no conditions, should at least have 4 defrevs
	defrevs, err = SearchDefinitionRevisions(context.TODO(), c, "", "", "", 0)
	assert.NoError(t, err)
	assert.Equal(t, true, len(defrevs) >= 4)

	// Restrict namespace
	defrevs, err = SearchDefinitionRevisions(context.TODO(), c, "rev-test-custom-ns", "", "", 0)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(defrevs))

	// Restrict type
	defrevs, err = SearchDefinitionRevisions(context.TODO(), c, "rev-test-ns", "", common2.ComponentType, 0)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(defrevs))

	// Restrict revision
	defrevs, err = SearchDefinitionRevisions(context.TODO(), c, "rev-test-ns", "", "", 1)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(defrevs))

	// Restrict name
	defrevs, err = SearchDefinitionRevisions(context.TODO(), c, "rev-test-ns", "webservice", "", 1)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(defrevs))

	// Test GetDefinitionFromDefinitionRevision
	defrev := defrevs[0]

	// Simulate ComponentDefinition
	defrev.Spec.DefinitionType = common2.ComponentType
	_, err = GetDefinitionFromDefinitionRevision(&defrev)
	assert.NoError(t, err)

	// Simulate TraitDefinition
	defrev.Spec.DefinitionType = common2.TraitType
	_, err = GetDefinitionFromDefinitionRevision(&defrev)
	assert.NoError(t, err)

	// Simulate PolicyDefinition
	defrev.Spec.DefinitionType = common2.PolicyType
	_, err = GetDefinitionFromDefinitionRevision(&defrev)
	assert.NoError(t, err)

	// Simulate WorkflowStepDefinition
	defrev.Spec.DefinitionType = common2.WorkflowStepType
	_, err = GetDefinitionFromDefinitionRevision(&defrev)
	assert.NoError(t, err)
}
