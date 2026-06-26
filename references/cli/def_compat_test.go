/*
Copyright 2024 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

// ── parseHints ────────────────────────────────────────────────────────────────

func TestParseHints(t *testing.T) {
	tests := []struct {
		name      string
		reasons   []string
		wantHints []CompatibilityHint
	}{
		{
			name:      "empty",
			reasons:   nil,
			wantHints: nil,
		},
		{
			name:    "single hint",
			reasons: []string{"[v1.11] contains deprecated list operators"},
			wantHints: []CompatibilityHint{
				{Introduced: "v1.11", Reason: "contains deprecated list operators"},
			},
		},
		{
			name: "multiple hints",
			reasons: []string{
				"[v1.11] contains deprecated list operators",
				"[v1.11] contains field named 'error'",
			},
			wantHints: []CompatibilityHint{
				{Introduced: "v1.11", Reason: "contains deprecated list operators"},
				{Introduced: "v1.11", Reason: "contains field named 'error'"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseHints(tt.reasons)
			if len(got) != len(tt.wantHints) {
				t.Fatalf("parseHints() len = %d, want %d", len(got), len(tt.wantHints))
			}
			for i, h := range got {
				if h != tt.wantHints[i] {
					t.Errorf("hint[%d] = %+v, want %+v", i, h, tt.wantHints[i])
				}
			}
		})
	}
}

// ── parseSelectorMap ──────────────────────────────────────────────────────────

func TestParseSelectorMap(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      map[string]string
		wantError bool
	}{
		{name: "empty", input: "", want: nil},
		{
			name:  "single pair",
			input: "app=foo",
			want:  map[string]string{"app": "foo"},
		},
		{
			name:  "multiple pairs",
			input: "app=foo,env=prod",
			want:  map[string]string{"app": "foo", "env": "prod"},
		},
		{
			name:  "value with equals sign",
			input: "key=a=b",
			want:  map[string]string{"key": "a=b"},
		},
		{
			name:      "missing value separator",
			input:     "badkey",
			wantError: true,
		},
		{
			name:      "empty key",
			input:     "=value",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSelectorMap(tt.input)
			if tt.wantError {
				if err == nil {
					t.Errorf("parseSelectorMap(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSelectorMap(%q) unexpected error: %v", tt.input, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseSelectorMap(%q) len = %d, want %d", tt.input, len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseSelectorMap(%q)[%q] = %q, want %q", tt.input, k, got[k], v)
				}
			}
		})
	}
}

// ── matchesSelector ───────────────────────────────────────────────────────────

func TestMatchesSelector(t *testing.T) {
	tests := []struct {
		name string
		meta map[string]string
		sel  map[string]string
		want bool
	}{
		{name: "nil selector matches anything", meta: map[string]string{"a": "b"}, sel: nil, want: true},
		{name: "empty selector matches anything", meta: map[string]string{"a": "b"}, sel: map[string]string{}, want: true},
		{name: "exact match", meta: map[string]string{"app": "foo"}, sel: map[string]string{"app": "foo"}, want: true},
		{name: "subset match", meta: map[string]string{"app": "foo", "env": "prod"}, sel: map[string]string{"app": "foo"}, want: true},
		{name: "value mismatch", meta: map[string]string{"app": "foo"}, sel: map[string]string{"app": "bar"}, want: false},
		{name: "missing key", meta: map[string]string{"app": "foo"}, sel: map[string]string{"env": "prod"}, want: false},
		{name: "nil meta", meta: nil, sel: map[string]string{"app": "foo"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesSelector(tt.meta, tt.sel); got != tt.want {
				t.Errorf("matchesSelector() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ── extractCUETemplate ────────────────────────────────────────────────────────

func TestExtractCUETemplate(t *testing.T) {
	tests := []struct {
		name     string
		obj      map[string]any
		wantTmpl string
		wantOk   bool
	}{
		{
			name:   "missing spec",
			obj:    map[string]any{},
			wantOk: false,
		},
		{
			name:   "missing schematic",
			obj:    map[string]any{"spec": map[string]any{}},
			wantOk: false,
		},
		{
			name: "missing cue",
			obj: map[string]any{"spec": map[string]any{
				"schematic": map[string]any{},
			}},
			wantOk: false,
		},
		{
			name: "template present",
			obj: map[string]any{"spec": map[string]any{
				"schematic": map[string]any{
					"cue": map[string]any{
						"template": "output: {}",
					},
				},
			}},
			wantTmpl: "output: {}",
			wantOk:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractCUETemplate(tt.obj)
			if ok != tt.wantOk {
				t.Errorf("extractCUETemplate() ok = %v, want %v", ok, tt.wantOk)
			}
			if got != tt.wantTmpl {
				t.Errorf("extractCUETemplate() = %q, want %q", got, tt.wantTmpl)
			}
		})
	}
}

// ── defNameFromRevision ───────────────────────────────────────────────────────

func TestDefNameFromRevision(t *testing.T) {
	tests := []struct {
		name     string
		drName   string
		drType   oamcommon.DefinitionType
		labels   map[string]string
		wantName string
	}{
		{
			name:     "name from label",
			drName:   "worker-v3",
			drType:   oamcommon.ComponentType,
			labels:   map[string]string{"componentdefinition.oam.dev/name": "worker"},
			wantName: "worker",
		},
		{
			name:     "fallback strip -vN",
			drName:   "my-trait-v12",
			drType:   oamcommon.TraitType,
			labels:   map[string]string{},
			wantName: "my-trait",
		},
		{
			name:     "fallback no -vN suffix",
			drName:   "standalone",
			drType:   oamcommon.ComponentType,
			labels:   map[string]string{},
			wantName: "standalone",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dr := v1beta1.DefinitionRevision{
				ObjectMeta: metav1.ObjectMeta{
					Name:   tt.drName,
					Labels: tt.labels,
				},
			}
			dr.Spec.DefinitionType = tt.drType
			got := defNameFromRevision(dr)
			if got != tt.wantName {
				t.Errorf("defNameFromRevision() = %q, want %q", got, tt.wantName)
			}
		})
	}
}

// ── extractDefRevTemplate ─────────────────────────────────────────────────────

func TestExtractDefRevTemplate(t *testing.T) {
	tmpl := "output: {}"

	tests := []struct {
		name string
		dr   v1beta1.DefinitionRevision
		want string
	}{
		{
			name: "component",
			dr: v1beta1.DefinitionRevision{Spec: v1beta1.DefinitionRevisionSpec{
				DefinitionType: oamcommon.ComponentType,
				ComponentDefinition: v1beta1.ComponentDefinition{Spec: v1beta1.ComponentDefinitionSpec{
					Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: tmpl}},
				}},
			}},
			want: tmpl,
		},
		{
			name: "trait",
			dr: v1beta1.DefinitionRevision{Spec: v1beta1.DefinitionRevisionSpec{
				DefinitionType: oamcommon.TraitType,
				TraitDefinition: v1beta1.TraitDefinition{Spec: v1beta1.TraitDefinitionSpec{
					Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: tmpl}},
				}},
			}},
			want: tmpl,
		},
		{
			name: "no schematic",
			dr: v1beta1.DefinitionRevision{Spec: v1beta1.DefinitionRevisionSpec{
				DefinitionType: oamcommon.ComponentType,
			}},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDefRevTemplate(tt.dr)
			if got != tt.want {
				t.Errorf("extractDefRevTemplate() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ── printDefCompatReport ──────────────────────────────────────────────────────

func TestPrintDefCompatReport(t *testing.T) {
	report := DefCompatReport{
		Total: 1,
		Definitions: []DefCompatEntry{
			{
				Name:      "worker",
				Kind:      "ComponentDefinition",
				Namespace: "vela-system",
				Issues: []DefIssueEntry{
					{
						Introduced: "v1.11",
						Reason:     "contains deprecated list operators",
						Revisions:  []string{"v1", "v2"},
					},
				},
			},
		},
	}

	t.Run("table output", func(t *testing.T) {
		var buf bytes.Buffer
		streams := util.IOStreams{Out: &buf}
		if err := printDefCompatReport(streams, report, "table"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "worker") {
			t.Error("expected definition name 'worker' in output")
		}
		if !strings.Contains(out, "v1.11") {
			t.Error("expected introduced version 'v1.11' in output")
		}
		if !strings.Contains(out, "v1, v2") {
			t.Error("expected revisions 'v1, v2' in output")
		}
	})

	t.Run("empty table output", func(t *testing.T) {
		var buf bytes.Buffer
		streams := util.IOStreams{Out: &buf}
		empty := DefCompatReport{Total: 0, Definitions: []DefCompatEntry{}}
		if err := printDefCompatReport(streams, empty, "table"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "compatible") {
			t.Error("expected 'compatible' message for empty report")
		}
	})

	t.Run("json output", func(t *testing.T) {
		var buf bytes.Buffer
		streams := util.IOStreams{Out: &buf}
		if err := printDefCompatReport(streams, report, "json"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var decoded DefCompatReport
		if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
			t.Fatalf("json output not valid: %v", err)
		}
		if decoded.Total != 1 {
			t.Errorf("json total = %d, want 1", decoded.Total)
		}
		if decoded.Definitions[0].Issues[0].Introduced != "v1.11" {
			t.Errorf("json introduced = %q, want v1.11", decoded.Definitions[0].Issues[0].Introduced)
		}
	})

	t.Run("yaml output", func(t *testing.T) {
		var buf bytes.Buffer
		streams := util.IOStreams{Out: &buf}
		if err := printDefCompatReport(streams, report, "yaml"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "introduced: v1.11") {
			t.Errorf("yaml output missing introduced field, got:\n%s", out)
		}
		if !strings.Contains(out, "- v1") {
			t.Errorf("yaml output missing revision v1, got:\n%s", out)
		}
	})
}

// ── printAppCompatReport ──────────────────────────────────────────────────────

func TestPrintAppCompatReport(t *testing.T) {
	report := AppCompatReport{
		Total: 1,
		Applications: []AppCompatEntry{
			{
				Application: "my-app",
				Namespace:   "default",
				Revision:    "my-app-v1",
				Issues: []AppIssueEntry{
					{
						Introduced: "v1.11",
						Reason:     "contains deprecated list operators",
						Components: []string{"worker"},
						Traits:     []string{"scaler"},
					},
				},
			},
		},
	}

	t.Run("table output", func(t *testing.T) {
		var buf bytes.Buffer
		streams := util.IOStreams{Out: &buf}
		if err := printAppCompatReport(streams, report, "table"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "my-app") {
			t.Error("expected app name in output")
		}
		if !strings.Contains(out, "my-app-v1") {
			t.Error("expected revision in output")
		}
		if !strings.Contains(out, "components: worker") {
			t.Error("expected components in output")
		}
		if !strings.Contains(out, "traits: scaler") {
			t.Error("expected traits in output")
		}
	})

	t.Run("empty table output", func(t *testing.T) {
		var buf bytes.Buffer
		streams := util.IOStreams{Out: &buf}
		empty := AppCompatReport{Total: 0, Applications: []AppCompatEntry{}}
		if err := printAppCompatReport(streams, empty, "table"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "compatible") {
			t.Error("expected 'compatible' message for empty report")
		}
	})

	t.Run("json output", func(t *testing.T) {
		var buf bytes.Buffer
		streams := util.IOStreams{Out: &buf}
		if err := printAppCompatReport(streams, report, "json"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var decoded AppCompatReport
		if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
			t.Fatalf("json output not valid: %v", err)
		}
		if decoded.Total != 1 {
			t.Errorf("json total = %d, want 1", decoded.Total)
		}
		issue := decoded.Applications[0].Issues[0]
		if issue.Introduced != "v1.11" {
			t.Errorf("json introduced = %q, want v1.11", issue.Introduced)
		}
		if len(issue.Components) != 1 || issue.Components[0] != "worker" {
			t.Errorf("json components = %v, want [worker]", issue.Components)
		}
	})

	t.Run("yaml output", func(t *testing.T) {
		var buf bytes.Buffer
		streams := util.IOStreams{Out: &buf}
		if err := printAppCompatReport(streams, report, "yaml"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "application: my-app") {
			t.Errorf("yaml output missing application field, got:\n%s", out)
		}
		if !strings.Contains(out, "components:") {
			t.Errorf("yaml output missing components field, got:\n%s", out)
		}
	})

	t.Run("omitempty hides empty kind fields", func(t *testing.T) {
		var buf bytes.Buffer
		streams := util.IOStreams{Out: &buf}
		noTraits := AppCompatReport{
			Total: 1,
			Applications: []AppCompatEntry{{
				Application: "app",
				Namespace:   "default",
				Revision:    "app-v1",
				Issues: []AppIssueEntry{{
					Introduced: "v1.11",
					Reason:     "some issue",
					Components: []string{"worker"},
				}},
			}},
		}
		if err := printAppCompatReport(streams, noTraits, "json"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(buf.String(), "traits") {
			t.Error("expected traits to be omitted when empty")
		}
	})
}
