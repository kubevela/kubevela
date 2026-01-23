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
	"strings"
	"testing"

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
			got, err := Upgrade(tt.input, "1.11") // Explicitly provide version for tests
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

	got, err := Upgrade(input, "1.11") // Explicitly provide version for tests
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

	got, err := Upgrade(input, "1.11") // Explicitly provide version for tests
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
	result, err := Upgrade(input, "1.11") // Provide explicit version for test
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}
	if !strings.Contains(result, "list.Concat") {
		t.Errorf("Default upgrade should apply 1.11 list concatenation upgrade, got = %v", result)
	}
	
	// Test explicit version 1.11
	result, err = Upgrade(input, "1.11")
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}
	if !strings.Contains(result, "list.Concat") {
		t.Errorf("1.11 upgrade should apply list concatenation upgrade, got = %v", result)
	}
	
	// Test future version (should still apply 1.11 upgrades)
	result, err = Upgrade(input, "1.12")
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
	found := false
	for _, v := range versions {
		if v == "1.11" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 1.11 to be in supported versions, got %v", versions)
	}
}

func TestGetCurrentKubeVelaMinorVersion(t *testing.T) {
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
			name:        "unknown version error",
			mockVersion: "UNKNOWN",
			expectError: true,
		},
		{
			name:        "empty version error",
			mockVersion: "",
			expectError: true,
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
			// Mock the version
			version.VelaVersion = tt.mockVersion
			
			got, err := getCurrentKubeVelaMinorVersion()
			if tt.expectError {
				if err == nil {
					t.Errorf("getCurrentKubeVelaMinorVersion() expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("getCurrentKubeVelaMinorVersion() unexpected error: %v", err)
				return
			}
			
			if got != tt.expectedVersion {
				t.Errorf("getCurrentKubeVelaMinorVersion() = %v, want %v", got, tt.expectedVersion)
			}
		})
	}
}

func TestRequiresUpgrade(t *testing.T) {
	tests := []struct {
		name         string
		input        string
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
			needsUpgrade, reasons, err := RequiresUpgrade(tt.input, "1.11")
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

func TestUpgradeWithUnknownVersionError(t *testing.T) {
	// Save original version
	originalVersion := version.VelaVersion
	defer func() {
		version.VelaVersion = originalVersion
	}()
	
	// Mock unknown version
	version.VelaVersion = "UNKNOWN"
	
	input := `
list1: [1, 2, 3]
list2: [4, 5, 6]
combined: list1 + list2
`
	
	// Should return error when no version is specified and VelaVersion is UNKNOWN
	_, err := Upgrade(input)
	if err == nil {
		t.Errorf("Upgrade() expected error when version is UNKNOWN but got none")
	}
	
	// Should contain helpful message
	expectedMsg := "Please specify the target version explicitly using --target-version=1.11"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Error message should contain guidance about using --target-version flag, got: %v", err.Error())
	}
	
	// Should work when explicit version is provided
	result, err := Upgrade(input, "1.11")
	if err != nil {
		t.Errorf("Upgrade() with explicit version should work even when VelaVersion is UNKNOWN, got error: %v", err)
	}
	if !strings.Contains(result, "list.Concat") {
		t.Errorf("Upgrade() with explicit version should still apply transformations")
	}
}