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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLabelCondition_Equals(t *testing.T) {
	tests := []struct {
		name     string
		cond     *LabelCondition
		labels   map[string]string
		expected bool
	}{
		{
			name:     "equals matches",
			cond:     Label("provider").Eq("aws"),
			labels:   map[string]string{"provider": "aws"},
			expected: true,
		},
		{
			name:     "equals does not match different value",
			cond:     Label("provider").Eq("aws"),
			labels:   map[string]string{"provider": "gcp"},
			expected: false,
		},
		{
			name:     "equals does not match missing label",
			cond:     Label("provider").Eq("aws"),
			labels:   map[string]string{},
			expected: false,
		},
		{
			name:     "equals with nil labels",
			cond:     Label("provider").Eq("aws"),
			labels:   nil,
			expected: false,
		},
		{
			name:     "equals with empty value",
			cond:     Label("provider").Eq(""),
			labels:   map[string]string{"provider": ""},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cond.Evaluate(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLabelCondition_NotEquals(t *testing.T) {
	tests := []struct {
		name     string
		cond     *LabelCondition
		labels   map[string]string
		expected bool
	}{
		{
			name:     "not equals matches when different",
			cond:     Label("provider").Ne("aws"),
			labels:   map[string]string{"provider": "gcp"},
			expected: true,
		},
		{
			name:     "not equals does not match when same",
			cond:     Label("provider").Ne("aws"),
			labels:   map[string]string{"provider": "aws"},
			expected: false,
		},
		{
			name:     "not equals matches when label missing",
			cond:     Label("provider").Ne("aws"),
			labels:   map[string]string{},
			expected: true,
		},
		{
			name:     "not equals matches with nil labels",
			cond:     Label("provider").Ne("aws"),
			labels:   nil,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cond.Evaluate(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLabelCondition_In(t *testing.T) {
	tests := []struct {
		name     string
		cond     *LabelCondition
		labels   map[string]string
		expected bool
	}{
		{
			name:     "in matches first value",
			cond:     Label("provider").In("aws", "gcp", "azure"),
			labels:   map[string]string{"provider": "aws"},
			expected: true,
		},
		{
			name:     "in matches middle value",
			cond:     Label("provider").In("aws", "gcp", "azure"),
			labels:   map[string]string{"provider": "gcp"},
			expected: true,
		},
		{
			name:     "in matches last value",
			cond:     Label("provider").In("aws", "gcp", "azure"),
			labels:   map[string]string{"provider": "azure"},
			expected: true,
		},
		{
			name:     "in does not match value not in set",
			cond:     Label("provider").In("aws", "gcp", "azure"),
			labels:   map[string]string{"provider": "on-prem"},
			expected: false,
		},
		{
			name:     "in does not match missing label",
			cond:     Label("provider").In("aws", "gcp"),
			labels:   map[string]string{},
			expected: false,
		},
		{
			name:     "in with single value",
			cond:     Label("provider").In("aws"),
			labels:   map[string]string{"provider": "aws"},
			expected: true,
		},
		{
			name:     "in with empty values list",
			cond:     Label("provider").In(),
			labels:   map[string]string{"provider": "aws"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cond.Evaluate(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLabelCondition_NotIn(t *testing.T) {
	tests := []struct {
		name     string
		cond     *LabelCondition
		labels   map[string]string
		expected bool
	}{
		{
			name:     "not in matches when value not in set",
			cond:     Label("provider").NotIn("aws", "gcp"),
			labels:   map[string]string{"provider": "azure"},
			expected: true,
		},
		{
			name:     "not in does not match when value in set",
			cond:     Label("provider").NotIn("aws", "gcp"),
			labels:   map[string]string{"provider": "aws"},
			expected: false,
		},
		{
			name:     "not in matches when label missing",
			cond:     Label("provider").NotIn("aws", "gcp"),
			labels:   map[string]string{},
			expected: true,
		},
		{
			name:     "not in with empty values fails closed",
			cond:     Label("provider").NotIn(),
			labels:   map[string]string{"provider": "aws"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cond.Evaluate(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLabelCondition_Exists(t *testing.T) {
	tests := []struct {
		name     string
		cond     *LabelCondition
		labels   map[string]string
		expected bool
	}{
		{
			name:     "exists matches when label present",
			cond:     Label("gpu").Exists(),
			labels:   map[string]string{"gpu": "true"},
			expected: true,
		},
		{
			name:     "exists matches when label has empty value",
			cond:     Label("gpu").Exists(),
			labels:   map[string]string{"gpu": ""},
			expected: true,
		},
		{
			name:     "exists does not match when label missing",
			cond:     Label("gpu").Exists(),
			labels:   map[string]string{"cpu": "true"},
			expected: false,
		},
		{
			name:     "exists does not match empty labels",
			cond:     Label("gpu").Exists(),
			labels:   map[string]string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cond.Evaluate(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLabelCondition_NotExists(t *testing.T) {
	tests := []struct {
		name     string
		cond     *LabelCondition
		labels   map[string]string
		expected bool
	}{
		{
			name:     "not exists matches when label missing",
			cond:     Label("deprecated").NotExists(),
			labels:   map[string]string{"active": "true"},
			expected: true,
		},
		{
			name:     "not exists does not match when label present",
			cond:     Label("deprecated").NotExists(),
			labels:   map[string]string{"deprecated": "true"},
			expected: false,
		},
		{
			name:     "not exists does not match even with empty value",
			cond:     Label("deprecated").NotExists(),
			labels:   map[string]string{"deprecated": ""},
			expected: false,
		},
		{
			name:     "not exists matches empty labels",
			cond:     Label("deprecated").NotExists(),
			labels:   map[string]string{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cond.Evaluate(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAllCondition(t *testing.T) {
	tests := []struct {
		name     string
		cond     *AllCondition
		labels   map[string]string
		expected bool
	}{
		{
			name: "all conditions match",
			cond: All(
				Label("provider").Eq("aws"),
				Label("cluster-type").Eq("eks"),
			),
			labels:   map[string]string{"provider": "aws", "cluster-type": "eks"},
			expected: true,
		},
		{
			name: "first condition fails",
			cond: All(
				Label("provider").Eq("aws"),
				Label("cluster-type").Eq("eks"),
			),
			labels:   map[string]string{"provider": "gcp", "cluster-type": "eks"},
			expected: false,
		},
		{
			name: "second condition fails",
			cond: All(
				Label("provider").Eq("aws"),
				Label("cluster-type").Eq("eks"),
			),
			labels:   map[string]string{"provider": "aws", "cluster-type": "gke"},
			expected: false,
		},
		{
			name: "all conditions fail",
			cond: All(
				Label("provider").Eq("aws"),
				Label("cluster-type").Eq("eks"),
			),
			labels:   map[string]string{"provider": "gcp", "cluster-type": "gke"},
			expected: false,
		},
		{
			name:     "empty all condition matches",
			cond:     All(),
			labels:   map[string]string{"provider": "aws"},
			expected: true,
		},
		{
			name: "single condition all",
			cond: All(
				Label("provider").Eq("aws"),
			),
			labels:   map[string]string{"provider": "aws"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cond.Evaluate(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAnyCondition(t *testing.T) {
	tests := []struct {
		name     string
		cond     *AnyCondition
		labels   map[string]string
		expected bool
	}{
		{
			name: "first condition matches",
			cond: Any(
				Label("provider").Eq("aws"),
				Label("provider").Eq("gcp"),
			),
			labels:   map[string]string{"provider": "aws"},
			expected: true,
		},
		{
			name: "second condition matches",
			cond: Any(
				Label("provider").Eq("aws"),
				Label("provider").Eq("gcp"),
			),
			labels:   map[string]string{"provider": "gcp"},
			expected: true,
		},
		{
			name: "no conditions match",
			cond: Any(
				Label("provider").Eq("aws"),
				Label("provider").Eq("gcp"),
			),
			labels:   map[string]string{"provider": "azure"},
			expected: false,
		},
		{
			name:     "empty any condition does not match",
			cond:     Any(),
			labels:   map[string]string{"provider": "aws"},
			expected: false,
		},
		{
			name: "single condition any",
			cond: Any(
				Label("provider").Eq("aws"),
			),
			labels:   map[string]string{"provider": "aws"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cond.Evaluate(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNotCondition(t *testing.T) {
	tests := []struct {
		name     string
		cond     *NotCondition
		labels   map[string]string
		expected bool
	}{
		{
			name:     "not negates true to false",
			cond:     Not(Label("provider").Eq("aws")),
			labels:   map[string]string{"provider": "aws"},
			expected: false,
		},
		{
			name:     "not negates false to true",
			cond:     Not(Label("provider").Eq("aws")),
			labels:   map[string]string{"provider": "gcp"},
			expected: true,
		},
		{
			name:     "not with nil condition returns true",
			cond:     &NotCondition{Condition: nil},
			labels:   map[string]string{"provider": "aws"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cond.Evaluate(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNestedConditions(t *testing.T) {
	tests := []struct {
		name     string
		cond     Condition
		labels   map[string]string
		expected bool
	}{
		{
			name: "nested all in any - first branch matches",
			cond: Any(
				All(
					Label("provider").Eq("aws"),
					Label("cluster-type").Eq("eks"),
				),
				All(
					Label("provider").Eq("gcp"),
					Label("cluster-type").Eq("gke"),
				),
			),
			labels:   map[string]string{"provider": "aws", "cluster-type": "eks"},
			expected: true,
		},
		{
			name: "nested all in any - second branch matches",
			cond: Any(
				All(
					Label("provider").Eq("aws"),
					Label("cluster-type").Eq("eks"),
				),
				All(
					Label("provider").Eq("gcp"),
					Label("cluster-type").Eq("gke"),
				),
			),
			labels:   map[string]string{"provider": "gcp", "cluster-type": "gke"},
			expected: true,
		},
		{
			name: "nested all in any - no branch matches",
			cond: Any(
				All(
					Label("provider").Eq("aws"),
					Label("cluster-type").Eq("eks"),
				),
				All(
					Label("provider").Eq("gcp"),
					Label("cluster-type").Eq("gke"),
				),
			),
			labels:   map[string]string{"provider": "azure", "cluster-type": "aks"},
			expected: false,
		},
		{
			name: "all with nested any",
			cond: All(
				Any(
					Label("provider").Eq("aws"),
					Label("provider").Eq("gcp"),
				),
				Label("environment").Eq("production"),
			),
			labels:   map[string]string{"provider": "aws", "environment": "production"},
			expected: true,
		},
		{
			name: "all with nested any - any fails",
			cond: All(
				Any(
					Label("provider").Eq("aws"),
					Label("provider").Eq("gcp"),
				),
				Label("environment").Eq("production"),
			),
			labels:   map[string]string{"provider": "azure", "environment": "production"},
			expected: false,
		},
		{
			name: "not nested in all",
			cond: All(
				Label("provider").Eq("aws"),
				Not(Label("cluster-type").Eq("vcluster")),
			),
			labels:   map[string]string{"provider": "aws", "cluster-type": "eks"},
			expected: true,
		},
		{
			name: "not nested in all - negation fails",
			cond: All(
				Label("provider").Eq("aws"),
				Not(Label("cluster-type").Eq("vcluster")),
			),
			labels:   map[string]string{"provider": "aws", "cluster-type": "vcluster"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cond.Evaluate(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPlacementSpec_Evaluate(t *testing.T) {
	tests := []struct {
		name                 string
		spec                 PlacementSpec
		labels               map[string]string
		expectEligible       bool
		expectReasonContains string
	}{
		{
			name:                 "empty spec is eligible everywhere",
			spec:                 PlacementSpec{},
			labels:               map[string]string{"provider": "aws"},
			expectEligible:       true,
			expectReasonContains: "no placement constraints",
		},
		{
			name: "runOn matches",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("provider").Eq("aws"),
				},
			},
			labels:               map[string]string{"provider": "aws"},
			expectEligible:       true,
			expectReasonContains: "provider = aws",
		},
		{
			name: "runOn does not match",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("provider").Eq("aws"),
				},
			},
			labels:               map[string]string{"provider": "gcp"},
			expectEligible:       false,
			expectReasonContains: "runOn conditions not satisfied",
		},
		{
			name: "notRunOn excludes",
			spec: PlacementSpec{
				NotRunOn: []Condition{
					Label("cluster-type").Eq("vcluster"),
				},
			},
			labels:               map[string]string{"cluster-type": "vcluster"},
			expectEligible:       false,
			expectReasonContains: "excluded by notRunOn",
		},
		{
			name: "notRunOn does not exclude",
			spec: PlacementSpec{
				NotRunOn: []Condition{
					Label("cluster-type").Eq("vcluster"),
				},
			},
			labels:               map[string]string{"cluster-type": "eks"},
			expectEligible:       true,
			expectReasonContains: "not excluded",
		},
		{
			name: "runOn matches and notRunOn does not exclude",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("provider").Eq("aws"),
				},
				NotRunOn: []Condition{
					Label("cluster-type").Eq("vcluster"),
				},
			},
			labels:         map[string]string{"provider": "aws", "cluster-type": "eks"},
			expectEligible: true,
		},
		{
			name: "runOn matches but notRunOn excludes",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("provider").Eq("aws"),
				},
				NotRunOn: []Condition{
					Label("cluster-type").Eq("vcluster"),
				},
			},
			labels:               map[string]string{"provider": "aws", "cluster-type": "vcluster"},
			expectEligible:       false,
			expectReasonContains: "excluded by notRunOn",
		},
		{
			name: "runOn does not match (notRunOn not checked)",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("provider").Eq("aws"),
				},
				NotRunOn: []Condition{
					Label("cluster-type").Eq("vcluster"),
				},
			},
			labels:               map[string]string{"provider": "gcp", "cluster-type": "vcluster"},
			expectEligible:       false,
			expectReasonContains: "runOn conditions not satisfied",
		},
		{
			name: "multiple runOn conditions - all must match",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("provider").Eq("aws"),
					Label("environment").Eq("production"),
				},
			},
			labels:         map[string]string{"provider": "aws", "environment": "production"},
			expectEligible: true,
		},
		{
			name: "multiple runOn conditions - one fails",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("provider").Eq("aws"),
					Label("environment").Eq("production"),
				},
			},
			labels:         map[string]string{"provider": "aws", "environment": "staging"},
			expectEligible: false,
		},
		{
			name:           "nil labels treated as empty",
			spec:           PlacementSpec{},
			labels:         nil,
			expectEligible: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Evaluate(tt.spec, tt.labels)
			assert.Equal(t, tt.expectEligible, result.Eligible)
			if tt.expectReasonContains != "" {
				assert.Contains(t, result.Reason, tt.expectReasonContains)
			}
		})
	}
}

func TestPlacementSpec_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		spec     PlacementSpec
		expected bool
	}{
		{
			name:     "empty spec",
			spec:     PlacementSpec{},
			expected: true,
		},
		{
			name: "only runOn",
			spec: PlacementSpec{
				RunOn: []Condition{Label("provider").Eq("aws")},
			},
			expected: false,
		},
		{
			name: "only notRunOn",
			spec: PlacementSpec{
				NotRunOn: []Condition{Label("cluster-type").Eq("vcluster")},
			},
			expected: false,
		},
		{
			name: "both runOn and notRunOn",
			spec: PlacementSpec{
				RunOn:    []Condition{Label("provider").Eq("aws")},
				NotRunOn: []Condition{Label("cluster-type").Eq("vcluster")},
			},
			expected: false,
		},
		{
			name: "empty slices",
			spec: PlacementSpec{
				RunOn:    []Condition{},
				NotRunOn: []Condition{},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.spec.IsEmpty()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCondition_String(t *testing.T) {
	tests := []struct {
		name     string
		cond     Condition
		expected string
	}{
		{
			name:     "equals",
			cond:     Label("provider").Eq("aws"),
			expected: "provider = aws",
		},
		{
			name:     "not equals",
			cond:     Label("provider").Ne("aws"),
			expected: "provider != aws",
		},
		{
			name:     "in",
			cond:     Label("provider").In("aws", "gcp"),
			expected: "provider in (aws, gcp)",
		},
		{
			name:     "not in",
			cond:     Label("provider").NotIn("azure"),
			expected: "provider not in (azure)",
		},
		{
			name:     "exists",
			cond:     Label("gpu").Exists(),
			expected: "gpu exists",
		},
		{
			name:     "not exists",
			cond:     Label("deprecated").NotExists(),
			expected: "deprecated not exists",
		},
		{
			name: "all condition",
			cond: All(
				Label("provider").Eq("aws"),
				Label("env").Eq("prod"),
			),
			expected: "all(provider = aws AND env = prod)",
		},
		{
			name: "any condition",
			cond: Any(
				Label("provider").Eq("aws"),
				Label("provider").Eq("gcp"),
			),
			expected: "any(provider = aws OR provider = gcp)",
		},
		{
			name:     "not condition",
			cond:     Not(Label("env").Eq("dev")),
			expected: "not(env = dev)",
		},
		{
			name:     "empty all",
			cond:     All(),
			expected: "all()",
		},
		{
			name:     "empty any",
			cond:     Any(),
			expected: "any()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cond.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRealWorldScenarios(t *testing.T) {
	// Test realistic placement scenarios based on the KEP

	awsEKSCluster := map[string]string{
		"provider":     "aws",
		"cluster-type": "eks",
		"environment":  "production",
		"region":       "us-east-1",
	}

	gcpGKECluster := map[string]string{
		"provider":     "gcp",
		"cluster-type": "gke",
		"environment":  "production",
		"region":       "us-central1",
	}

	vclusterDev := map[string]string{
		"provider":     "aws",
		"cluster-type": "vcluster",
		"environment":  "dev",
	}

	tests := []struct {
		name           string
		spec           PlacementSpec
		cluster        map[string]string
		expectEligible bool
	}{
		{
			name: "AWS ALB controller on EKS",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("provider").Eq("aws"),
					Label("cluster-type").In("eks", "self-managed"),
				},
				NotRunOn: []Condition{
					Label("cluster-type").Eq("vcluster"),
				},
			},
			cluster:        awsEKSCluster,
			expectEligible: true,
		},
		{
			name: "AWS ALB controller on GKE - should fail",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("provider").Eq("aws"),
				},
			},
			cluster:        gcpGKECluster,
			expectEligible: false,
		},
		{
			name: "AWS ALB controller on vcluster - should be excluded",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("provider").Eq("aws"),
				},
				NotRunOn: []Condition{
					Label("cluster-type").Eq("vcluster"),
				},
			},
			cluster:        vclusterDev,
			expectEligible: false,
		},
		{
			name: "Multi-cloud load balancer",
			spec: PlacementSpec{
				RunOn: []Condition{
					Any(
						Label("provider").Eq("aws"),
						Label("provider").Eq("gcp"),
						Label("provider").Eq("azure"),
					),
				},
			},
			cluster:        gcpGKECluster,
			expectEligible: true,
		},
		{
			name: "Dev-only component on vcluster",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("cluster-type").Eq("vcluster"),
				},
			},
			cluster:        vclusterDev,
			expectEligible: true,
		},
		{
			name: "Dev-only component on production EKS - should fail",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("cluster-type").Eq("vcluster"),
				},
			},
			cluster:        awsEKSCluster,
			expectEligible: false,
		},
		{
			name: "Production-only policy",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("environment").Eq("production"),
				},
			},
			cluster:        awsEKSCluster,
			expectEligible: true,
		},
		{
			name: "Production-only policy on dev vcluster - should fail",
			spec: PlacementSpec{
				RunOn: []Condition{
					Label("environment").Eq("production"),
				},
			},
			cluster:        vclusterDev,
			expectEligible: false,
		},
		{
			name: "Exclude staging environments",
			spec: PlacementSpec{
				NotRunOn: []Condition{
					Label("environment").Eq("staging"),
				},
			},
			cluster:        awsEKSCluster,
			expectEligible: true,
		},
		{
			name: "Complex: (AWS EKS OR GCP GKE) AND production",
			spec: PlacementSpec{
				RunOn: []Condition{
					All(
						Any(
							All(
								Label("provider").Eq("aws"),
								Label("cluster-type").Eq("eks"),
							),
							All(
								Label("provider").Eq("gcp"),
								Label("cluster-type").Eq("gke"),
							),
						),
						Label("environment").Eq("production"),
					),
				},
			},
			cluster:        awsEKSCluster,
			expectEligible: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Evaluate(tt.spec, tt.cluster)
			assert.Equal(t, tt.expectEligible, result.Eligible,
				"Expected eligible=%v but got %v. Reason: %s",
				tt.expectEligible, result.Eligible, result.Reason)
		})
	}
}

func TestGetEffectivePlacement(t *testing.T) {
	modulePlacement := PlacementSpec{
		RunOn: []Condition{
			Label("provider").Eq("aws"),
		},
		NotRunOn: []Condition{
			Label("cluster-type").Eq("vcluster"),
		},
	}

	definitionPlacement := PlacementSpec{
		RunOn: []Condition{
			Label("provider").Eq("gcp"),
		},
	}

	tests := []struct {
		name       string
		module     PlacementSpec
		definition PlacementSpec
		expectLen  int    // expected len of RunOn in result
		expectFrom string // "module" or "definition"
	}{
		{
			name:       "definition overrides module when definition has placement",
			module:     modulePlacement,
			definition: definitionPlacement,
			expectLen:  1,
			expectFrom: "definition",
		},
		{
			name:       "uses module when definition is empty",
			module:     modulePlacement,
			definition: PlacementSpec{},
			expectLen:  1,
			expectFrom: "module",
		},
		{
			name:       "both empty returns empty",
			module:     PlacementSpec{},
			definition: PlacementSpec{},
			expectLen:  0,
			expectFrom: "none",
		},
		{
			name:   "definition with only notRunOn still overrides",
			module: modulePlacement,
			definition: PlacementSpec{
				NotRunOn: []Condition{Label("env").Eq("dev")},
			},
			expectLen:  0, // definition has no runOn
			expectFrom: "definition",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetEffectivePlacement(tt.module, tt.definition)
			assert.Len(t, result.RunOn, tt.expectLen)

			switch tt.expectFrom {
			case "definition":
				// Result should be definition's placement
				if !tt.definition.IsEmpty() {
					assert.Equal(t, len(tt.definition.RunOn), len(result.RunOn))
					assert.Equal(t, len(tt.definition.NotRunOn), len(result.NotRunOn))
				}
			case "module":
				// Result should be module's placement
				assert.Equal(t, len(tt.module.RunOn), len(result.RunOn))
				assert.Equal(t, len(tt.module.NotRunOn), len(result.NotRunOn))
			case "none":
				assert.True(t, result.IsEmpty())
			}
		})
	}
}

func TestOperator_IsValid(t *testing.T) {
	tests := []struct {
		operator Operator
		valid    bool
	}{
		{OperatorEquals, true},
		{OperatorNotEquals, true},
		{OperatorIn, true},
		{OperatorNotIn, true},
		{OperatorExists, true},
		{OperatorNotExists, true},
		{Operator("Eq"), true},
		{Operator("Ne"), true},
		{Operator("In"), true},
		{Operator("NotIn"), true},
		{Operator("Exists"), true},
		{Operator("NotExists"), true},
		{Operator("Invalid"), false},
		{Operator("Equals"), false},   // common typo
		{Operator("NotEqual"), false}, // common typo
		{Operator(""), false},
		{Operator("eq"), false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(string(tt.operator), func(t *testing.T) {
			assert.Equal(t, tt.valid, tt.operator.IsValid())
		})
	}
}

func TestValidOperators(t *testing.T) {
	ops := ValidOperators()
	assert.Len(t, ops, 6)
	assert.Contains(t, ops, OperatorEquals)
	assert.Contains(t, ops, OperatorNotEquals)
	assert.Contains(t, ops, OperatorIn)
	assert.Contains(t, ops, OperatorNotIn)
	assert.Contains(t, ops, OperatorExists)
	assert.Contains(t, ops, OperatorNotExists)
}

// TestLabelCondition_EmptyValues tests fail-closed behavior for empty values.
// Following Kubernetes label selector semantics, Eq/Ne/In/NotIn with empty
// values are invalid configurations that should fail closed (return false).
func TestLabelCondition_EmptyValues(t *testing.T) {
	labels := map[string]string{"provider": "aws"}

	tests := []struct {
		name     string
		cond     *LabelCondition
		expected bool
	}{
		{
			name: "Equals with empty values returns false",
			cond: &LabelCondition{
				Key:      "provider",
				Operator: OperatorEquals,
				Values:   []string{},
			},
			expected: false,
		},
		{
			name: "NotEquals with empty values returns false (fail closed)",
			cond: &LabelCondition{
				Key:      "provider",
				Operator: OperatorNotEquals,
				Values:   []string{},
			},
			expected: false,
		},
		{
			name: "In with empty values returns false",
			cond: &LabelCondition{
				Key:      "provider",
				Operator: OperatorIn,
				Values:   []string{},
			},
			expected: false,
		},
		{
			name: "NotIn with empty values returns false (fail closed)",
			cond: &LabelCondition{
				Key:      "provider",
				Operator: OperatorNotIn,
				Values:   []string{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cond.Evaluate(labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}
