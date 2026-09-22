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

package sourcedefinition

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/definition/cachekey"
)

// TestRepoSourceDefinitionsAreAccepted guards against the storage validator
// becoming stricter than the definitions actually shipped in this repo.
//
// It drives the real `vela def` conversion path (FromCUEString) so the string
// under test is exactly what lands in spec.schematic.cue.template, rather than a
// hand-sliced approximation of it.
func TestRepoSourceDefinitionsAreAccepted(t *testing.T) {
	const demoDefs = "../../../../../docs/examples/source-definition-demo/definitions/*.cue"

	files, err := filepath.Glob(demoDefs)
	if err != nil {
		t.Fatalf("glob %s: %v", demoDefs, err)
	}
	if len(files) == 0 {
		t.Fatalf("no definitions matched %s — the path is stale, fix the glob rather than skipping", demoDefs)
	}

	checked := 0
	for _, f := range files {
		name := filepath.Base(f)
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		def := &definition.Definition{Unstructured: unstructured.Unstructured{Object: map[string]interface{}{}}}
		if err := def.FromCUEString(string(raw), nil); err != nil {
			t.Fatalf("%s: definition in-repo no longer converts: %v", name, err)
		}
		if def.GetKind() != "SourceDefinition" {
			continue // other definition types carry no storage: block
		}

		template, _, err := unstructured.NestedString(def.Object, "spec", "schematic", "cue", "template")
		if err != nil {
			t.Fatalf("%s: reading template: %v", name, err)
		}
		if err := ValidateSourceStorage(template); err != nil {
			t.Errorf("%s: rejected by ValidateSourceStorage: %v", name, err)
		}
		if err := ValidateSourceSchema(template); err != nil {
			t.Errorf("%s: rejected by ValidateSourceSchema: %v", name, err)
		}
		checked++
	}

	if checked == 0 {
		t.Fatal("no SourceDefinitions were checked — this test is no longer covering anything")
	}
}

// TestRepoYAMLSourceDefinitionsPassAdmission covers the manifests that never go
// through `vela def` at all.
//
// The .cue definitions above are stamped during conversion, so their keys are
// correct by construction. A YAML manifest applied with kubectl - the GitOps
// path - gets no such help: it carries whatever key its author wrote, and
// admission is the only thing standing between a wrong one and a poisoned cache.
// These are the examples users copy, so they have to survive the real check.
func TestRepoYAMLSourceDefinitionsPassAdmission(t *testing.T) {
	const demoManifests = "../../../../../docs/examples/source-definition-demo/*.yaml"

	files, err := filepath.Glob(demoManifests)
	if err != nil {
		t.Fatalf("glob %s: %v", demoManifests, err)
	}
	if len(files) == 0 {
		t.Fatalf("no manifests matched %s — the path is stale, fix the glob rather than skipping", demoManifests)
	}

	checked := 0
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s: %v", filepath.Base(f), err)
		}
		for _, chunk := range strings.Split(string(raw), "\n---\n") {
			obj := map[string]interface{}{}
			if err := yaml.Unmarshal([]byte(chunk), &obj); err != nil {
				continue // not a manifest we can read; other tests cover parsing
			}
			u := unstructured.Unstructured{Object: obj}
			if u.GetKind() != "SourceDefinition" {
				continue
			}

			name := u.GetName()
			template, _, err := unstructured.NestedString(u.Object, "spec", "schematic", "cue", "template")
			if err != nil {
				t.Fatalf("%s: reading template: %v", name, err)
			}

			if err := ValidateSourceStorage(template); err != nil {
				t.Errorf("%s (%s): rejected by ValidateSourceStorage: %v", name, filepath.Base(f), err)
			}
			// The check that matters here: the key and the inputs it hashes are
			// re-derived from the template, exactly as the webhook does it.
			if err := cachekey.Verify(name, template, u.GetAnnotations()[cachekey.RulesAnnotation]); err != nil {
				t.Errorf("%s (%s): would be rejected at admission: %v", name, filepath.Base(f), err)
			}
			checked++
		}
	}

	if checked == 0 {
		t.Fatal("no SourceDefinitions were checked — this test is no longer covering anything")
	}
}
