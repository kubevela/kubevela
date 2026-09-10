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

// Walks every definition vela-core ships and validates it through the validator
// its kind is routed to. This is the regression surface for the routing itself:
// the workload compiler rejects 26 of the 36 bundled WorkflowStepDefinitions
// with `builtin package "vela/op" undefined`, so a kind sent to the wrong
// validator, or a compiler that loses a package, breaks admission for templates
// that ship in the chart. Failing here is cheaper than failing on a user's
// cluster after upgrade.
func TestValidateCuexTemplate_BundledDefinitions(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("..", "..", "..", "charts", "vela-core", "templates", "defwithtemplate")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("bundled definitions not readable at %s: %v", dir, err)
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
			// A non-CUE manifest unmarshals cleanly with zero values, so an error
			// here means helmTemplateExpr missed some Helm templating and this
			// file silently dropped out of the coverage below.
			t.Errorf("%s: not valid YAML after stripping Helm templating: %v", entry.Name(), err)
			continue
		}
		if def.Spec.Schematic.CUE.Template == "" {
			continue
		}

		// Explicit rather than defaulted: routing a kind to the other validator
		// is the mistake this test exists to catch.
		var validate func(context.Context, string) error
		switch def.Kind {
		case "WorkflowStepDefinition":
			validate = ValidateWorkflowStepCuexTemplate
		case "ComponentDefinition", "TraitDefinition":
			validate = ValidateCuexTemplate
		default:
			// PolicyDefinition still uses the plain ValidateCueTemplate.
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
