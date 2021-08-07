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

package common

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestDefinitionBasicFunctions(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	def := &Definition{Unstructured: unstructured.Unstructured{}}
	def.SetAnnotations(map[string]string{
		DefinitionUserPrefix + "annotation": "annotation",
		"other":                             "other",
	})
	def.SetLabels(map[string]string{
		DefinitionUserPrefix + "label": "label",
		"other":                        "other",
	})
	def.SetName("test-trait")
	def.SetGVK("TraitDefinition")
	if _type := def.GetType(); _type != "trait" {
		t.Fatalf("set gvk invalid, expected trait got %s", _type)
	}
	if err := def.SetType("abc"); err == nil {
		t.Fatalf("set type should failed due to invalid type, but got no error")
	}
	def.Object["spec"] = GetDefinitionDefaultSpec("TraitDefinition")
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
	if err = def.FromCUEString(cueString + "abc"); err == nil {
		t.Fatalf("should encounter invalid cue string but not found error")
	}
	parts := strings.Split(cueString, "template: ")
	if err = def.FromCUEString(parts[0]); err == nil {
		t.Fatalf("should encounter no template found error but not found error")
	}
	if err = def.FromCUEString("import \"strconv\"\n" + cueString); err == nil {
		t.Fatalf("should encounter cue compile error due to useless import but not found error")
	}
	if err = def.FromCUEString("abc: {}\n" + cueString); err == nil {
		t.Fatalf("should encounter duplicated object name error but not found error")
	}
	if err = def.FromCUEString(strings.Replace(cueString, "\"trait\"", "\"tr\"", 1)); err == nil {
		t.Fatalf("should encounter invalid type error but not found error")
	}
	if err = def.FromCUEString(cueString); err != nil {
		t.Fatalf("unexpected error when setting from cue: %v", err)
	}
	templateString, _, _ := unstructured.NestedString(def.Object, DefinitionTemplateKeys...)
	_ = unstructured.SetNestedField(def.Object, "import \"strconv\"\n"+templateString, DefinitionTemplateKeys...)
	if s, err := def.ToCUEString(); err != nil {
		t.Fatalf("failed to generate cue string: %v", err)
	} else if !strings.Contains(s, "import \"strconv\"\n") {
		t.Fatalf("definition ToCUEString missed import, val: %v", s)
	}
	def = &Definition{}
	if err = def.FromCUEString(cueString); err != nil {
		t.Fatalf("unexpected error when setting from cue for empty def: %v", err)
	}

	// test other definition default spec
	_ = GetDefinitionDefaultSpec("ComponentDefinition")
	_ = GetDefinitionDefaultSpec("WorkloadDefinition")
	_ = ValidDefinitionTypes()

	if _, err = SearchDefinition("*", c, "", ""); err != nil {
		t.Fatalf("failed to search definition: %v", err)
	}
	if _, err = SearchDefinition("*", c, "trait", "default"); err != nil {
		t.Fatalf("failed to search definition: %v", err)
	}
}
