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

package upgrade

import (
	"slices"
	"strings"
	"testing"

	cueast "cuelang.org/go/cue/ast"
	dto "github.com/prometheus/client_model/go"

	"github.com/oam-dev/kubevela/version"
)

func TestUpgrade(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name: "simple list concatenation",
			input: `
myList1: [1, 2, 3]
myList2: [4, 5, 6]
combined: myList1 + myList2
`,
			expected: "list.Concat",
			wantErr:  false,
		},
		{
			name: "list concatenation in object",
			input: `
object: {
	items: baseItems + extraItems
	baseItems: ["a", "b"]
	extraItems: ["c", "d"]
}
`,
			expected: "list.Concat",
			wantErr:  false,
		},
		{
			name: "non-list addition should not be transformed",
			input: `
number1: 5
number2: 10
sum: number1 + number2
`,
			expected: "number1 + number2", // Should remain as is
			wantErr:  false,
		},
		{
			name: "mixed with existing imports",
			input: `
import "strings"

myList1: [1, 2, 3]
myList2: [4, 5, 6]
combined: myList1 + myList2
`,
			expected: "list.Concat",
			wantErr:  false,
		},
		{
			name: "simple list repeat",
			input: `
myList: ["a", "b"]
repeated: myList * 3
`,
			expected: "list.Repeat",
			wantErr:  false,
		},
		{
			name: "reverse list repeat",
			input: `
myList: ["x", "y", "z"]
repeated: 2 * myList
`,
			expected: "list.Repeat",
			wantErr:  false,
		},
		{
			name: "list repeat with field references",
			input: `
parameter: {
	items: ["item1", "item2"]
	count: 5
	repeated1: items * 2
	repeated2: 3 * items
}
`,
			expected: "list.Repeat",
			wantErr:  false,
		},
		{
			name: "mixed concatenation and repeat",
			input: `
list1: ["a", "b"]
list2: ["c", "d"]
concatenated: list1 + list2
repeated: concatenated * 2
`,
			expected: "list.Concat",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Upgrade(tt.input, Version{Major: 1, Minor: 11}) // Explicitly provide version for tests
			if (err != nil) != tt.wantErr {
				t.Errorf("Upgrade() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Check if the expected transformation occurred
			if tt.expected == "list.Concat" || tt.expected == "list.Repeat" {
				if !strings.Contains(got, tt.expected) {
					t.Errorf("Upgrade() did not transform to %s, got = %v", tt.expected, got)
				}
				if !strings.Contains(got, `import "list"`) {
					t.Errorf("Upgrade() did not add list import, got = %v", got)
				}
			} else {
				// Check that the expected string is still present (not transformed)
				if !strings.Contains(got, tt.expected) {
					t.Errorf("Upgrade() unexpectedly transformed non-list operation, got = %v", got)
				}
			}
		})
	}
}

func TestUpgradeErrorFieldLabel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
		absent   string
	}{
		{
			name: "top-level error field",
			input: `
error: "something went wrong"
output: {name: "test"}
`,
			contains: `"error": "something went wrong"`,
			absent:   "error: \"something went wrong\"",
		},
		{
			name: "nested error field",
			input: `
template: {
	error: "bad"
	output: {}
}
`,
			contains: `"error": "bad"`,
		},
		{
			name: "error field not confused with error() call",
			input: `
result: error("something")
`,
			// error() call should be left alone — it's not a field label
			contains: `error("something")`,
		},
		{
			name: "already quoted error field unchanged",
			input: `
"error": "already quoted"
`,
			contains: `"error": "already quoted"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Upgrade(tt.input, Version{Major: 1, Minor: 11})
			if err != nil {
				t.Fatalf("Upgrade() error = %v", err)
			}
			if tt.contains != "" && !strings.Contains(got, tt.contains) {
				t.Errorf("expected output to contain %q, got:\n%s", tt.contains, got)
			}
			if tt.absent != "" && strings.Contains(got, tt.absent) {
				t.Errorf("expected output NOT to contain %q, got:\n%s", tt.absent, got)
			}
			t.Logf("output:\n%s", got)
		})
	}
}

func TestRequiresUpgradeErrorField(t *testing.T) {
	input := `
template: {
	error: "something"
	output: {}
}
`
	needsUpgrade, reasons, err := RequiresUpgrade(input, Version{Major: 1, Minor: 11})
	if err != nil {
		t.Fatalf("RequiresUpgrade() error = %v", err)
	}
	if !needsUpgrade {
		t.Error("expected upgrade required for error field label")
	}
	if len(reasons) != 1 {
		t.Errorf("expected 1 reason, got %d: %v", len(reasons), reasons)
	}
}

func TestUpgradeChainedListConcat(t *testing.T) {
	// Chained a + b + c + d must become list.Concat([a, b, c, d]) in a single pass,
	// not nested list.Concat(list.Concat(...)) calls.
	// This is the real-world pattern from vela-templates/definitions/internal/component/cron-task.cue
	common := `mountsArray: {
	pvc:       [{mountPath: "/pvc"}]
	configMap: [{mountPath: "/cm"}]
	secret:    [{mountPath: "/secret"}]
	emptyDir:  [{mountPath: "/empty"}]
	hostPath:  [{mountPath: "/host"}]
}`
	want := "list.Concat([mountsArray.pvc, mountsArray.configMap, mountsArray.secret, mountsArray.emptyDir, mountsArray.hostPath])"

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "fresh chain",
			input: "volumeMounts: mountsArray.pvc + mountsArray.configMap + mountsArray.secret + mountsArray.emptyDir + mountsArray.hostPath\n" + common,
		},
		{
			// Simulates a partially-upgraded file (previous upgrade pass converted only the first pair).
			name:  "partially upgraded chain",
			input: "volumeMounts: list.Concat([mountsArray.pvc, mountsArray.configMap]) + mountsArray.secret + mountsArray.emptyDir + mountsArray.hostPath\n" + common,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := EnsureCueVersionCompatibility(tt.input, "test", ComponentKind, TemplateAreaMain)
			if strings.Contains(result, "list.Concat([list.Concat(") {
				t.Errorf("got nested list.Concat instead of flat: %s", result)
			}
			if !strings.Contains(result, want) {
				t.Errorf("expected flat list.Concat with all 5 operands, got:\n%s", result)
			}
		})
	}
}

func TestRequiresUpgradeErrorFieldNegative(t *testing.T) {
	// Templates that contain "error" but NOT as an unquoted field label should not trigger.
	cases := []string{
		`output: { message: "error occurred" }`,
		`output: { errorMessage: "bad" }`,
		`// handle error cases\noutput: {}`,
		`output: { "error": "already quoted" }`, // already quoted — upgrade would be a no-op, precheck still fires (fine)
	}
	for _, input := range cases {
		if errorFieldLabelRe.MatchString(input) {
			// Only "error:" (field label) should match; none of the above have an unquoted error: label.
			t.Errorf("errorFieldLabelRe false positive on: %q", input)
		}
	}
	// Positive: all unquoted error field label variants must match.
	positives := []string{
		`output: { error: "something" }`,
		`output: { error?: "optional" }`,
		`output: { error!: "required" }`,
	}
	for _, p := range positives {
		if !errorFieldLabelRe.MatchString(p) {
			t.Errorf("errorFieldLabelRe failed to match: %q", p)
		}
	}
}

func TestUpgradeWithOpenListParameter(t *testing.T) {
	// Real-world pattern: parameter declared as open list type `*[] | [...{...}]`
	// The old registry only matched literal list values; this tests the disjunction case.
	input := `
template: {
	envWithDefaults: parameter.env + [{name: "MANAGED_BY", value: "kubevela"}]

	output: {
		spec: containers: [{
			env: envWithDefaults
		}]
	}

	parameter: {
		env: *[] | [...{
			name:   string
			value?: string
		}]
	}
}
`
	got, err := Upgrade(input, Version{Major: 1, Minor: 11})
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}

	if !strings.Contains(got, "list.Concat") {
		t.Errorf("Upgrade() did not transform open list parameter concatenation, got:\n%s", got)
	}

	if !strings.Contains(got, `import "list"`) {
		t.Errorf("Upgrade() did not add list import, got:\n%s", got)
	}

	t.Logf("Transformed:\n%s", got)
}

func TestUpgradeWithComplexTemplate(t *testing.T) {
	// Test with a template similar to what would be used in a workload definition
	input := `
template: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	spec: {
		selector: matchLabels: app: context.name
		template: {
			metadata: labels: app: context.name
			spec: {
				containers: [{
					name: context.name
					image: parameter.image
					env: parameter.env + [{name: "EXTRA", value: "value"}]
				}]
			}
		}
	}
}

parameter: {
	image: string
	env: [...{name: string, value: string}]
}

output: template
`

	got, err := Upgrade(input, Version{Major: 1, Minor: 11}) // Explicitly provide version for tests
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}

	// Check that env concatenation was transformed
	if !strings.Contains(got, "list.Concat") {
		t.Errorf("Upgrade() did not transform env list concatenation")
	}

	// Check that list import was added
	if !strings.Contains(got, `import "list"`) {
		t.Errorf("Upgrade() did not add list import")
	}

	t.Logf("Transformed template:\n%s", got)
}

func TestUpgradeWithStringsJoin(t *testing.T) {
	// Test the specific case from test-component-lists.cue
	input := `
import "strings"

template: {
	output: {
		spec: {
			selector: matchLabels: "app.oam.dev/component": parameter.name
			template: {
				metadata: labels: "app.oam.dev/component": parameter.name
				spec: containers: [{
					name:  parameter.name
					image: parameter.image
				}]
			}
		}
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: {
			name: strings.Join(parameter.list1 + parameter.list2, "-")
		}
	}
	outputs: {}

	parameter: {
		list1: [...string]
		list2: [...string]
		name: string
		image: string
	}
}
`

	got, err := Upgrade(input, Version{Major: 1, Minor: 11}) // Explicitly provide version for tests
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}

	// Check that the concatenation inside strings.Join was transformed
	// list.Concat takes a list of lists as a single argument
	if !strings.Contains(got, "strings.Join(list.Concat([") {
		t.Errorf("Upgrade() did not transform list concatenation inside strings.Join")
		t.Logf("Got:\n%s", got)
	}

	// Check that list import was added
	if !strings.Contains(got, `import "list"`) {
		t.Errorf("Upgrade() did not add list import")
	}

	// The original strings import should still be there
	if !strings.Contains(got, `import "strings"`) {
		t.Errorf("Upgrade() removed the strings import")
	}

	t.Logf("Transformed template:\n%s", got)
}

func TestUpgradeRegistry(t *testing.T) {
	// Test that the registry system works and can handle version-specific upgrades

	// Test with explicit version (should apply 1.11 upgrades)
	input := `
list1: [1, 2, 3]
list2: [4, 5, 6]
result: list1 + list2
`
	result, err := Upgrade(input, Version{Major: 1, Minor: 11}) // Provide explicit version for test
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}
	if !strings.Contains(result, "list.Concat") {
		t.Errorf("Default upgrade should apply 1.11 list concatenation upgrade, got = %v", result)
	}

	// Test explicit version 1.11
	result, err = Upgrade(input, Version{Major: 1, Minor: 11})
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}
	if !strings.Contains(result, "list.Concat") {
		t.Errorf("1.11 upgrade should apply list concatenation upgrade, got = %v", result)
	}

	// Test future version (should still apply 1.11 upgrades)
	result, err = Upgrade(input, Version{Major: 1, Minor: 12})
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}
	if !strings.Contains(result, "list.Concat") {
		t.Errorf("Future version upgrade should still apply 1.11 upgrades, got = %v", result)
	}
}

func TestGetSupportedVersions(t *testing.T) {
	versions := GetSupportedVersions()
	if len(versions) == 0 {
		t.Error("Expected at least one supported version")
	}

	// Should include 1.11 since that's registered in init()
	if !slices.Contains(versions, Version{Major: 1, Minor: 11}) {
		t.Errorf("Expected 1.11 to be in supported versions, got %v", versions)
	}
}

func TestGetCurrentKubeVelaVersion(t *testing.T) {
	tests := []struct {
		name            string
		mockVersion     string
		expectedVersion string
		expectError     bool
	}{
		{
			name:            "full semantic version",
			mockVersion:     "v1.11.2",
			expectedVersion: "1.11",
			expectError:     false,
		},
		{
			name:            "full semantic version without v prefix",
			mockVersion:     "1.12.0",
			expectedVersion: "1.12",
			expectError:     false,
		},
		{
			name:            "dev version",
			mockVersion:     "v1.13.0-alpha.1+dev",
			expectedVersion: "1.13",
			expectError:     false,
		},
		{
			name:            "unknown version uses latest",
			mockVersion:     "UNKNOWN",
			expectedVersion: "1.11",
			expectError:     false,
		},
		{
			name:            "empty version uses latest",
			mockVersion:     "",
			expectedVersion: "1.11",
			expectError:     false,
		},
		{
			name:        "invalid version error",
			mockVersion: "invalid-version",
			expectError: true,
		},
	}

	// Save original version
	originalVersion := version.VelaVersion
	defer func() {
		version.VelaVersion = originalVersion
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version.VelaVersion = tt.mockVersion

			got, err := getCurrentKubeVelaVersion()
			if tt.expectError {
				if err == nil {
					t.Errorf("getCurrentKubeVelaVersion() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("getCurrentKubeVelaVersion() unexpected error: %v", err)
				return
			}

			if got.String() != tt.expectedVersion {
				t.Errorf("getCurrentKubeVelaVersion() = %v, want %v", got, tt.expectedVersion)
			}
		})
	}
}

func TestRequiresUpgrade(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		shouldRequire bool
		expectReasons int
	}{
		{
			name: "requires upgrade - list concatenation",
			input: `
myList1: [1, 2, 3]
myList2: [4, 5, 6]
combined: myList1 + myList2
`,
			shouldRequire: true,
			expectReasons: 1,
		},
		{
			name: "no upgrade needed - already uses list.Concat",
			input: `
import "list"
myList1: [1, 2, 3]
myList2: [4, 5, 6]
combined: list.Concat([myList1, myList2])
`,
			shouldRequire: false,
			expectReasons: 0,
		},
		{
			name: "no upgrade needed - numeric addition",
			input: `
x: 1
y: 2
sum: x + y
`,
			shouldRequire: false,
			expectReasons: 0,
		},
		{
			name: "requires upgrade - nested structure",
			input: `
parameter: {
	env: [...{name: string, value: string}]
	extraEnv: [{name: "DEBUG", value: "true"}]
}
combined: parameter.env + parameter.extraEnv
`,
			shouldRequire: true,
			expectReasons: 1,
		},
		{
			name: "requires upgrade - list repeat",
			input: `
items: ["a", "b", "c"]
repeated: items * 5
`,
			shouldRequire: true,
			expectReasons: 1,
		},
		{
			name: "no upgrade needed - already uses list.Repeat",
			input: `
import "list"
items: ["a", "b", "c"]
repeated: list.Repeat(items, 5)
`,
			shouldRequire: false,
			expectReasons: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needsUpgrade, reasons, err := RequiresUpgrade(tt.input, Version{Major: 1, Minor: 11})
			if err != nil {
				t.Fatalf("RequiresUpgrade() error = %v", err)
			}

			if needsUpgrade != tt.shouldRequire {
				t.Errorf("RequiresUpgrade() = %v, want %v", needsUpgrade, tt.shouldRequire)
			}

			if len(reasons) != tt.expectReasons {
				t.Errorf("RequiresUpgrade() returned %d reasons, want %d. Reasons: %v",
					len(reasons), tt.expectReasons, reasons)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input     string
		wantMajor int
		wantMinor int
		wantErr   bool
	}{
		{"1.11", 1, 11, false},
		{"v1.11", 1, 11, false},
		{"1.11.2", 1, 11, false},
		{"v1.11.2", 1, 11, false},
		{"1.9", 1, 9, false},
		{"2.0", 2, 0, false},
		{"v1.13.0-alpha.1+dev", 1, 13, false},
		{"1.11foo", 0, 0, true},
		{"1.11.2foo", 0, 0, true},
		{"invalid", 0, 0, true},
		{"", 0, 0, true},
		{"v", 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Major != tt.wantMajor || got.Minor != tt.wantMinor {
				t.Errorf("ParseVersion(%q) = {%d,%d}, want {%d,%d}", tt.input, got.Major, got.Minor, tt.wantMajor, tt.wantMinor)
			}
		})
	}
}

func TestVersionString(t *testing.T) {
	tests := []struct {
		v    Version
		want string
	}{
		{Version{1, 11}, "1.11"},
		{Version{2, 0}, "2.0"},
		{Version{0, 9}, "0.9"},
	}
	for _, tt := range tests {
		if got := tt.v.String(); got != tt.want {
			t.Errorf("Version%v.String() = %q, want %q", tt.v, got, tt.want)
		}
	}
}

func TestVersionLess(t *testing.T) {
	tests := []struct {
		a, b Version
		want bool
	}{
		{Version{1, 9}, Version{1, 11}, true},   // minor ordering (not lexicographic)
		{Version{1, 11}, Version{1, 9}, false},  // reverse
		{Version{1, 11}, Version{1, 11}, false}, // equal
		{Version{1, 11}, Version{2, 0}, true},   // major ordering
		{Version{2, 0}, Version{1, 11}, false},
	}
	for _, tt := range tests {
		if got := tt.a.less(tt.b); got != tt.want {
			t.Errorf("Version%v.less(Version%v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestSortedVersionsOrdering(t *testing.T) {
	vs := sortedVersions()
	for i := 1; i < len(vs); i++ {
		if !vs[i-1].less(vs[i]) {
			t.Errorf("sortedVersions() not in ascending order: %v >= %v at index %d", vs[i-1], vs[i], i)
		}
	}
}

func TestEnsureCueVersionCompatibilityDisabled(t *testing.T) {
	original := EnableCUEVersionCompatibility
	defer func() { EnableCUEVersionCompatibility = original }()
	EnableCUEVersionCompatibility = false

	input := `
list1: [1, 2, 3]
list2: [4, 5, 6]
combined: list1 + list2
`
	got, _ := EnsureCueVersionCompatibility(input, "test-def", ComponentKind, TemplateAreaMain)
	if got != input {
		t.Errorf("expected input returned unchanged when compatibility disabled, got %q", got)
	}
}

func TestUpgradeWithUnknownVersion(t *testing.T) {
	// Save original version
	originalVersion := version.VelaVersion
	defer func() {
		version.VelaVersion = originalVersion
	}()

	// Mock unknown version (dev build)
	version.VelaVersion = "UNKNOWN"

	input := `
list1: [1, 2, 3]
list2: [4, 5, 6]
combined: list1 + list2
`

	// Should apply all upgrades (treating UNKNOWN as latest) rather than erroring
	result, err := Upgrade(input)
	if err != nil {
		t.Errorf("Upgrade() should not error on UNKNOWN version, got: %v", err)
	}
	if !strings.Contains(result, "list.Concat") {
		t.Errorf("Upgrade() with UNKNOWN version should still apply all upgrades, got: %v", result)
	}
}

func TestUpgradeFuncIDRequired(t *testing.T) {
	// RegisterUpgrade must panic if ID is empty.
	defer func() {
		if r := recover(); r == nil {
			t.Error("RegisterUpgrade with empty ID should panic")
		}
	}()
	RegisterUpgrade(KubeVelaUpgradeFunc{
		ID:          "", // intentionally empty
		VelaVersion: Version{Major: 99, Minor: 0},
		Reason:      "test",
		Upgrade:     func(s string, f *cueast.File) (string, error) { return s, nil },
	})
}

func TestUpgradeFuncIDsPresent(t *testing.T) {
	// All registered upgrade functions must have non-empty IDs.
	for v, funcs := range upgradeRegistry {
		for i, u := range funcs {
			if u.id() == "" {
				t.Errorf("upgradeRegistry[%s][%d] has empty ID", v, i)
			}
		}
	}
}

func TestRequiresUpgradeReasonsContainID(t *testing.T) {
	// Reasons returned by RequiresUpgrade must include the upgrade ID.
	input := `
list1: [1, 2, 3]
list2: [4, 5, 6]
combined: list1 + list2
`
	_, reasons, err := RequiresUpgrade(input, Version{Major: 1, Minor: 11})
	if err != nil {
		t.Fatalf("RequiresUpgrade() error = %v", err)
	}
	if len(reasons) == 0 {
		t.Fatal("expected at least one reason")
	}
	for _, r := range reasons {
		if !strings.Contains(r, "[cue@0.14] [list-arithmetic]") {
			t.Errorf("reason %q does not contain expected prefix '[cue@0.14] [list-arithmetic]'", r)
		}
	}
}

func TestEnsureCueVersionCompatibilityIncrementsMetric(t *testing.T) {
	// Reset cache so the upgrade path actually runs.
	compatCache.Store(newLRUCache(512))

	// Reset the counter before the test.
	CUECompatRewriteTotal.Reset()

	input := `
list1: [1, 2, 3]
list2: [4, 5, 6]
combined: list1 + list2
`
	EnsureCueVersionCompatibility(input, "test-def", ComponentKind, TemplateAreaMain)

	// Gather the metric value for the expected label triple.
	mf, err := CUECompatRewriteTotal.GetMetricWithLabelValues("list-arithmetic", "1.11", string(ComponentKind), string(TemplateAreaMain))
	if err != nil {
		t.Fatalf("failed to get metric: %v", err)
	}
	m := &dto.Metric{}
	if err := mf.Write(m); err != nil {
		t.Fatalf("failed to write metric: %v", err)
	}
	if m.Counter == nil || m.Counter.GetValue() < 1 {
		t.Errorf("expected counter >= 1 for list-arithmetic/1.11/Component, got %v", m)
	}
}
