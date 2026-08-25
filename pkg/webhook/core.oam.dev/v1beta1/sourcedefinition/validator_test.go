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
	"strings"
	"testing"

	"github.com/oam-dev/kubevela/pkg/sources"

	"github.com/stretchr/testify/require"
)

func TestValidateSourceStorage(t *testing.T) {
	cases := []struct {
		name     string
		template string
		wantErr  string // substring; "" means the template must be accepted
	}{
		{
			name: "accept literal key",
			template: `
$internal: {key: "cluster-config-reader"}
output: { region: "us-east-1" }
`,
		},
		{
			name: "accept interpolated key",
			template: `
$internal: {
	key:        "cluster-config-reader-\(context.cluster)"
}
storage: {
	storageTTL: "15m"
}
output: { region: "us-east-1" }
`,
		},
		{
			name: "reject missing $internal block",
			template: `
schema: { region: string }
output: { region: "us-east-1" }
`,
			wantErr: "must declare a $internal block",
		},
		{
			// storage: on its own is fine now - it holds only authored fields. What
			// is missing is the generated block, which admission cannot invent.
			name: "reject an authored storage block with no generated one",
			template: `
storage: {
  storageTTL: "15m"
}
`,
			wantErr: "must declare a $internal block",
		},
		{
			name: "reject empty key",
			template: `
$internal: {key: ""}
`,
			wantErr: "must not be empty",
		},
		{
			name: "reject blank key",
			template: `
$internal: {key: "   "}
`,
			wantErr: "must not be empty",
		},
		{
			name: "reject uppercase in literal key",
			template: `
$internal: {key: "Cluster-Config"}
`,
			wantErr: "not allowed in a cache key",
		},
		{
			name: "reject illegal punctuation in literal key",
			template: `
$internal: {key: "component:default/api"}
`,
			wantErr: "not allowed in a cache key",
		},
		{
			name: "reject illegal literal segment of an interpolated key",
			template: `
$internal: {key: "backstage:\(parameter.entityRef)"}
`,
			wantErr: "literal segment",
		},
		{
			name: "reject non-string key",
			template: `
$internal: {key: 42}
`,
			wantErr: "must be a string",
		},
		{
			name:     "reject empty template",
			template: "   ",
			wantErr:  "must declare a cue template",
		},
		{
			name: "reject key over the length limit",
			template: `
$internal: {key: "` + strings.Repeat("a", 254) + `"}
`,
			wantErr: "exceeding the 253-character limit",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSourceStorage(tc.template)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("expected template to be accepted, got error: %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateSourceSchema(t *testing.T) {
	cases := []struct {
		name     string
		template string
		wantErr  string // substring; "" means the template must be accepted
	}{
		{
			name: "accept a declared schema",
			template: `
schema: {
  region: string
}
$internal: {key: "k"}
`,
		},
		{
			name: "accept optional and required fields",
			template: `
schema: {
  region:  string
  vpcId?:  string
  account!: string
}
`,
		},
		{
			name: "reject missing schema",
			template: `
$internal: {key: "k"}
output: {region: "us-east-1"}
`,
			wantErr: "must declare a schema: block",
		},
		{
			name: "reject empty schema",
			template: `
schema: {}
$internal: {key: "k"}
`,
			wantErr: "at least one field",
		},
		{
			name: "reject non-struct schema",
			template: `
schema: "not-a-struct"
`,
			wantErr: "must be a struct",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSourceSchema(tc.template)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("expected template to be accepted, got error: %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestParseConsumableFrom(t *testing.T) {
	cases := []struct {
		name     string
		template string
		want     []string
		wantErr  string
	}{
		{
			name:     "absent means unrestricted",
			template: "schema: {a: string}\nstorage: {key: \"k\"}\n",
			want:     nil,
		},
		{
			name:     "single surface",
			template: "consumableFrom: [\"component\"]\nschema: {a: string}\n",
			want:     []string{"component"},
		},
		{
			name:     "both surfaces",
			template: "consumableFrom: [\"component\", \"trait\"]\nschema: {a: string}\n",
			want:     []string{"component", "trait"},
		},
		{
			// Workflow steps resolve via the pre-pass in
			// generateWorkflowInstance, so a definition may name them.
			name:     "accept a workflow step",
			template: "consumableFrom: [\"workflowstep\"]\nschema: {a: string}\n",
			want:     []string{"workflowstep"},
		},
		{
			// Policy properties still carry the directive inert, so a definition
			// cannot claim to be consumable there.
			name:     "reject a surface that does not resolve",
			template: "consumableFrom: [\"policy\"]\nschema: {a: string}\n",
			wantErr:  "not a surface that supports a source read",
		},
		{
			name:     "reject empty list",
			template: "consumableFrom: []\nschema: {a: string}\n",
			wantErr:  "must not be empty",
		},
		{
			// Absence is the only way to say "unrestricted"; there is no literal
			// catch-all value to get subtly wrong.
			name:     "reject a bare string",
			template: "consumableFrom: \"all\"\nschema: {a: string}\n",
			wantErr:  "must be a list of surfaces",
		},
		{
			name:     "reject non-string entries",
			template: "consumableFrom: [1]\nschema: {a: string}\n",
			wantErr:  "must be strings",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseConsumableFrom(tc.template)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("expected %v, got %v", tc.want, got)
				}
			}
		})
	}
}

func TestSurfaceAllowed(t *testing.T) {
	if !SurfaceAllowed(nil, sources.SurfaceComponent) {
		t.Fatal("unrestricted source must be allowed from a component")
	}
	if !SurfaceAllowed(nil, sources.SurfaceTrait) {
		t.Fatal("unrestricted source must be allowed from a trait")
	}
	if !SurfaceAllowed([]string{sources.SurfaceComponent}, sources.SurfaceComponent) {
		t.Fatal("component-only source must be allowed from a component")
	}
	if SurfaceAllowed([]string{sources.SurfaceComponent}, sources.SurfaceTrait) {
		t.Fatal("component-only source must not be allowed from a trait")
	}
}

// context.componentName and context.revision are offered by components and
// traits but not by workflow steps or rendered policies, so the check has real
// work to do rather than only pinning the shape of an answer.
func TestValidateSurfaceCompatibility(t *testing.T) {
	// The read has to sit outside $internal: inference strips the generated
	// block before scanning, so a context read that appears only in the stamped
	// key is not a read at all.
	const keyedOnCluster = `
schema: {host: string}
output: {host: "x-\(context.cluster)"}
`
	const keyedOnComponentName = `
schema: {host: string}
output: {host: "x-\(context.componentName)"}
`
	cases := []struct {
		name       string
		template   string
		consumable []string
		wantErr    string
	}{
		{
			name:     "a template reading no context is usable anywhere",
			template: "schema: {host: string}\noutput: {host: \"x\"}",
		},
		{
			name:     "a template that will not parse is left to the cache-key check",
			template: `output: {this is not cue`,
		},
		{
			name:     "cluster is offered by every surface that resolves sources",
			template: keyedOnCluster,
		},
		{
			name:       "and by each of them named explicitly",
			template:   keyedOnCluster,
			consumable: []string{"component"},
		},
		{
			name:       "an unrestricted definition is judged against every surface",
			template:   keyedOnCluster,
			consumable: nil,
		},
		{
			// Declaring a surface is an assertion about it, so one surface
			// working does not excuse another that cannot.
			name:       "every declared surface has to supply what the template reads",
			template:   keyedOnComponentName,
			consumable: []string{"component", "workflowstep"},
			wantErr:    "context.componentName",
		},
		{
			name:       "and the surfaces that do supply it are still accepted",
			template:   keyedOnComponentName,
			consumable: []string{"component", "trait"},
		},
		{
			// Unrestricted asserts nothing, so the narrower check at bind time
			// is what refuses the surfaces that cannot.
			name:       "an unrestricted definition needs only one surface to work",
			template:   keyedOnComponentName,
			consumable: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSurfaceCompatibility(tc.template, tc.consumable)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected acceptance, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// Every surface a source may be consumed from has a plural name, because the
// refusal message lists them and a blank there reads as a bug.
func TestPluraliseNamesEverySurface(t *testing.T) {
	got := pluralise(sources.ConsumableSurfaces)
	if len(got) != len(sources.ConsumableSurfaces) {
		t.Fatalf("pluralise dropped a surface: %v", got)
	}
	for i, name := range got {
		if strings.TrimSpace(name) == "" {
			t.Errorf("%s has no plural form", sources.ConsumableSurfaces[i])
		}
	}
}

// `schema: {...}` passed the non-empty check while declaring nothing, so the
// definition was admitted and then could not be read from: admission has no
// path to validate a read against, and close(schema) admits no output field.
func TestValidateSourceSchemaRefusesAnOpenSchemaThatDeclaresNothing(t *testing.T) {
	require.Error(t, ValidateSourceSchema("schema: {...}\noutput: {anything: \"x\"}\n"))
	require.Error(t, ValidateSourceSchema("schema: {}\noutput: {}\n"))
	// Open alongside a declaration is redundant, not wrong.
	require.NoError(t, ValidateSourceSchema("schema: {host: string, ...}\noutput: {host: \"x\"}\n"))
	require.NoError(t, ValidateSourceSchema("schema: {host: string}\noutput: {host: \"x\"}\n"))
}
