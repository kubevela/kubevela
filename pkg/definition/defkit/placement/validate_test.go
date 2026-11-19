/*
Copyright 2025 The KubeVela Authors.

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

package placement

import (
	"strings"
	"testing"
)

func TestValidatePlacement(t *testing.T) {
	tests := []struct {
		name        string
		spec        PlacementSpec
		wantErr     bool
		errContains string
	}{
		{
			name:    "empty placement is valid",
			spec:    PlacementSpec{},
			wantErr: false,
		},
		{
			name: "only RunOn is valid",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("cloud-provider").Eq("aws"),
				},
			},
			wantErr: false,
		},
		{
			name: "only NotRunOn is valid",
			spec: PlacementSpec{
				NotRunOn: []Condition{
					Label("environment").Eq("production"),
				},
			},
			wantErr: false,
		},
		{
			name: "non-overlapping conditions are valid",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("cloud-provider").Eq("aws"),
				},
				NotRunOn: []Condition{
					Label("environment").Eq("production"),
				},
			},
			wantErr: false,
		},
		{
			name: "identical condition in RunOn and NotRunOn is invalid",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("cloud-provider").Eq("aws"),
				},
				NotRunOn: []Condition{
					Label("cloud-provider").Eq("aws"),
				},
			},
			wantErr:     true,
			errContains: "identical condition",
		},
		{
			name: "RunOn Eq with NotRunOn Exists on same key is invalid",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("cloud-provider").Eq("aws"),
				},
				NotRunOn: []Condition{
					Label("cloud-provider").Exists(),
				},
			},
			wantErr:     true,
			errContains: "specific value",
		},
		{
			name: "RunOn In with NotRunOn Exists on same key is invalid",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("cloud-provider").In("aws", "gcp"),
				},
				NotRunOn: []Condition{
					Label("cloud-provider").Exists(),
				},
			},
			wantErr:     true,
			errContains: "specific value",
		},
		{
			name: "RunOn Exists with NotRunOn Exists on same key is invalid",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("cloud-provider").Exists(),
				},
				NotRunOn: []Condition{
					Label("cloud-provider").Exists(),
				},
			},
			wantErr:     true,
			errContains: "exists",
		},
		{
			name: "RunOn Eq and NotRunOn In containing that value is invalid",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("cloud-provider").Eq("aws"),
				},
				NotRunOn: []Condition{
					Label("cloud-provider").In("aws", "azure"),
				},
			},
			wantErr:     true,
			errContains: "exclusion list",
		},
		{
			name: "RunOn In all values excluded by NotRunOn In is invalid",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("cloud-provider").In("aws", "gcp"),
				},
				NotRunOn: []Condition{
					Label("cloud-provider").In("aws", "gcp", "azure"),
				},
			},
			wantErr:     true,
			errContains: "all values",
		},
		{
			name: "RunOn In with partial overlap in NotRunOn is valid (some values remain)",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("cloud-provider").In("aws", "gcp", "azure"),
				},
				NotRunOn: []Condition{
					Label("cloud-provider").In("aws"),
				},
			},
			wantErr: false, // gcp and azure are still valid
		},
		{
			name: "different keys don't conflict",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("cloud-provider").Eq("aws"),
				},
				NotRunOn: []Condition{
					Label("region").Eq("us-west-2"),
				},
			},
			wantErr: false,
		},
		{
			name: "RunOn Eq different value from NotRunOn Eq is valid",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("cloud-provider").Eq("aws"),
				},
				NotRunOn: []Condition{
					Label("cloud-provider").Eq("gcp"),
				},
			},
			wantErr: false, // aws != gcp, so no conflict
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePlacement(tt.spec)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidatePlacement() expected error, got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errContains)) {
					t.Errorf("ValidatePlacement() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidatePlacement() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	err := &ValidationError{
		Message:  "test error message",
		RunOn:    "cloud = aws",
		NotRunOn: "cloud = aws",
	}

	if err.Error() != "test error message" {
		t.Errorf("ValidationError.Error() = %q, want %q", err.Error(), "test error message")
	}
}

func TestConditionsConflict_CompositeConditions(t *testing.T) {
	tests := []struct {
		name         string
		runOn        Condition
		notRunOn     Condition
		wantConflict bool
	}{
		{
			name:         "All condition with conflicting inner condition",
			runOn:        All(Label("cloud").Eq("aws"), Label("env").Eq("prod")),
			notRunOn:     Label("cloud").Eq("aws"),
			wantConflict: true,
		},
		{
			name:         "Any condition in NotRunOn with conflicting part",
			runOn:        Label("cloud").Eq("aws"),
			notRunOn:     Any(Label("cloud").Eq("aws"), Label("env").Eq("dev")),
			wantConflict: true,
		},
		{
			name:         "Non-conflicting composite conditions",
			runOn:        All(Label("cloud").Eq("aws"), Label("env").Eq("prod")),
			notRunOn:     Label("region").Eq("eu-west-1"),
			wantConflict: false,
		},
		// Complex nested conditions
		{
			name:         "RunOn Any vs NotRunOn Any with same conditions - conflict",
			runOn:        Any(Label("cloud").Eq("aws"), Label("cloud").Eq("gcp")),
			notRunOn:     Any(Label("cloud").Eq("aws"), Label("cloud").Eq("gcp")),
			wantConflict: true,
		},
		{
			name:         "RunOn Any vs NotRunOn All - no conflict (Any needs one, All needs both)",
			runOn:        Any(Label("cloud").Eq("aws"), Label("cloud").Eq("gcp")),
			notRunOn:     All(Label("cloud").Eq("aws"), Label("cloud").Eq("gcp")),
			wantConflict: false,
		},
		{
			name:         "RunOn All vs NotRunOn Any with overlapping condition - conflict",
			runOn:        All(Label("cloud").Eq("aws"), Label("env").Eq("prod")),
			notRunOn:     Any(Label("cloud").Eq("aws"), Label("region").Eq("us-east-1")),
			wantConflict: true,
		},
		{
			name:         "RunOn All vs NotRunOn All - no conflict (NotRunOn needs both to match)",
			runOn:        All(Label("cloud").Eq("aws"), Label("env").Eq("prod")),
			notRunOn:     All(Label("cloud").Eq("aws"), Label("env").Eq("dev")),
			wantConflict: false,
		},
		{
			name:         "Deeply nested All inside Any - RunOn",
			runOn:        Any(All(Label("cloud").Eq("aws"), Label("region").Eq("us-east-1")), Label("cloud").Eq("gcp")),
			notRunOn:     Label("cloud").Eq("aws"),
			wantConflict: false, // gcp option is still valid
		},
		{
			name:         "Deeply nested - all options excluded",
			runOn:        Any(Label("cloud").Eq("aws"), Label("cloud").Eq("gcp")),
			notRunOn:     Any(Label("cloud").Eq("aws"), Label("cloud").Eq("gcp"), Label("cloud").Eq("azure")),
			wantConflict: true,
		},
		{
			name:         "RunOn with Exists vs NotRunOn with NotExists - conflict",
			runOn:        Label("gpu").Exists(),
			notRunOn:     Label("gpu").NotExists(),
			wantConflict: true,
		},
		{
			name:         "RunOn simple vs NotRunOn nested Any containing it - conflict",
			runOn:        Label("env").Eq("prod"),
			notRunOn:     Any(Label("env").Eq("prod"), Label("env").Eq("staging")),
			wantConflict: true,
		},
		{
			name:         "RunOn simple vs NotRunOn nested All containing it - no conflict",
			runOn:        Label("env").Eq("prod"),
			notRunOn:     All(Label("env").Eq("prod"), Label("cloud").Eq("azure")),
			wantConflict: false, // cloud might not be azure
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflict, reason := conditionsConflict(tt.runOn, tt.notRunOn)
			if conflict != tt.wantConflict {
				t.Errorf("conditionsConflict() = %v (reason: %s), want %v", conflict, reason, tt.wantConflict)
			}
		})
	}
}

func TestComplexPlacementValidation(t *testing.T) {
	tests := []struct {
		name        string
		spec        PlacementSpec
		wantErr     bool
		errContains string
	}{
		{
			name: "Complex valid placement - multi-cloud with exclusions",
			spec: PlacementSpec{
				RunOn: []Condition{
					Any(
						Label("cloud").Eq("aws"),
						Label("cloud").Eq("gcp"),
					),
				},
				NotRunOn: []Condition{
					Label("env").Eq("dev"),
				},
			},
			wantErr: false,
		},
		{
			name: "Complex valid placement - nested All with different NotRunOn",
			spec: PlacementSpec{
				RunOn: []Condition{
					All(
						Label("cloud").Eq("aws"),
						Label("region").In("us-east-1", "us-west-2"),
					),
				},
				NotRunOn: []Condition{
					Label("cluster-type").Eq("vcluster"),
				},
			},
			wantErr: false,
		},
		{
			name: "Complex conflict - Any options all excluded by Any",
			spec: PlacementSpec{
				RunOn: []Condition{
					Any(
						Label("cloud").Eq("aws"),
						Label("cloud").Eq("gcp"),
					),
				},
				NotRunOn: []Condition{
					Any(
						Label("cloud").Eq("aws"),
						Label("cloud").Eq("gcp"),
						Label("cloud").Eq("azure"),
					),
				},
			},
			wantErr:     true,
			errContains: "overlap",
		},
		{
			name: "Complex conflict - All requires something excluded by Any",
			spec: PlacementSpec{
				RunOn: []Condition{
					All(
						Label("cloud").Eq("aws"),
						Label("env").Eq("prod"),
					),
				},
				NotRunOn: []Condition{
					Any(
						Label("cloud").Eq("aws"),
						Label("region").Eq("eu-west-1"),
					),
				},
			},
			wantErr:     true,
			errContains: "cloud = aws", // Required by All but excluded by Any
		},
		{
			name: "Valid - RunOn Any with partial overlap in NotRunOn",
			spec: PlacementSpec{
				RunOn: []Condition{
					Any(
						Label("cloud").Eq("aws"),
						Label("cloud").Eq("gcp"),
						Label("cloud").Eq("azure"),
					),
				},
				NotRunOn: []Condition{
					Label("cloud").Eq("aws"), // Only excludes aws, gcp and azure still valid
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePlacement(tt.spec)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidatePlacement() expected error, got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errContains)) {
					t.Errorf("ValidatePlacement() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidatePlacement() unexpected error = %v", err)
				}
			}
		})
	}
}
