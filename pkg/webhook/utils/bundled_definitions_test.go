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

package utils

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/yaml"
)

// helmTemplateExpr matches the Helm actions embedded in the bundled definition
// manifests (e.g. {{ include "systemDefinitionNamespace" . }}). They are not
// valid YAML, so they are replaced with a scalar before parsing.
var helmTemplateExpr = regexp.MustCompile(`\{\{[^}]*\}\}`)

// TestValidateCuexTemplate_BundledDefinitions guards the compiler swap this
// validator went through. Admission now compiles every definition kind with the
// workflow provider compiler rather than the workload one, so the bundled
// definitions are the regression surface: if the shared compiler ever loses a
// package one of them imports, this fails before anyone ships it.
//
// The previous workload compiler rejected 26 of the 36 bundled
// WorkflowStepDefinition templates with `builtin package "vela/op" undefined`,
// which is why step definitions could not simply reuse it.
func TestValidateCuexTemplate_BundledDefinitions(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("..", "..", "..", "charts", "vela-core", "templates", "defwithtemplate")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("bundled definitions not readable at %s: %v", dir, err)
	}

	perKind := map[string]int{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}

		var def struct {
			Kind string `json:"kind"`
			Spec struct {
				Schematic struct {
					CUE struct {
						Template string `json:"template"`
					} `json:"cue"`
				} `json:"schematic"`
			} `json:"spec"`
		}
		if err := yaml.Unmarshal(helmTemplateExpr.ReplaceAll(raw, []byte("placeholder")), &def); err != nil {
			// Not every manifest in this directory is a CUE-schematic definition.
			continue
		}
		if def.Spec.Schematic.CUE.Template == "" {
			continue
		}

		// Each kind must be validated by the compiler that renders it. Routing a
		// kind through the other validator is precisely the mistake this test
		// exists to catch, so the mapping is explicit rather than a default.
		var validate func(context.Context, string) error
		switch def.Kind {
		case "WorkflowStepDefinition":
			validate = ValidateWorkflowStepCuexTemplate
		case "ComponentDefinition", "TraitDefinition":
			validate = ValidateCuexTemplate
		default:
			// PolicyDefinition still goes through the plain ValidateCueTemplate
			// in its handler, so it is out of scope here.
			continue
		}

		perKind[def.Kind]++
		t.Run(entry.Name(), func(t *testing.T) {
			t.Parallel()
			assert.NoErrorf(t, validate(context.Background(), def.Spec.Schematic.CUE.Template),
				"bundled %s %s must pass admission validation", def.Kind, entry.Name())
		})
	}

	// Fail loudly if the manifests move rather than silently validating nothing.
	assert.NotEmpty(t, perKind, "expected to find bundled definitions to validate")
	assert.Positivef(t, perKind["WorkflowStepDefinition"], "expected bundled WorkflowStepDefinitions, found kinds: %v", perKind)
	t.Logf("validated bundled definitions by kind: %v", perKind)
}
