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

package applicationconfiguration

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
)

var (
	ctx = context.Background()
)

func TestValidateRevisionNameFn(t *testing.T) {
	tests := []struct {
		caseName            string
		validatingAppConfig ValidatingAppConfig
		want                []error
	}{
		{
			caseName: "componentName and revisionName are both assigned",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						appConfigComponent: v1alpha2.ApplicationConfigurationComponent{
							ComponentName: "example-comp",
							RevisionName:  "example-comp-v1",
						},
					},
				},
			},
			want: []error{
				fmt.Errorf(errFmtRevisionName, "example-comp", "example-comp-v1"),
			},
		},
		{
			caseName: "componentName is assigned",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						appConfigComponent: v1alpha2.ApplicationConfigurationComponent{
							ComponentName: "example-comp",
						},
					},
				},
			},
			want: nil,
		},
		{
			caseName: "revisionName is assigned",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						appConfigComponent: v1alpha2.ApplicationConfigurationComponent{
							RevisionName: "example-comp-v1",
						},
					},
				},
			},
			want: nil,
		},
	}

	for _, tc := range tests {
		result := ValidateRevisionNameFn(ctx, tc.validatingAppConfig)
		assert.Equal(t, tc.want, result, fmt.Sprintf("Test case: %q", tc.caseName))
	}
}

func TestValidateWorkloadNameForVersioningFn(t *testing.T) {
	workloadName := "wl-name"
	wlWithName := unstructured.Unstructured{}
	wlWithName.SetName(workloadName)
	paramName := "workloadName"
	paramValue := workloadName

	tests := []struct {
		caseName            string
		validatingAppConfig ValidatingAppConfig
		want                []error
	}{
		{
			caseName: "validation fails for workload name fixed in component",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						compName:        "example-comp",
						workloadContent: wlWithName,
						validatingTraits: []ValidatingTrait{
							{traitDefinition: v1alpha2.TraitDefinition{
								Spec: v1alpha2.TraitDefinitionSpec{RevisionEnabled: true},
							}},
						},
					},
				},
			},
			want: []error{
				fmt.Errorf(errFmtWorkloadNameNotEmpty, workloadName),
			},
		},
		{
			caseName: "validation fails for workload name assigned by parameter",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						compName: "example-comp",
						appConfigComponent: v1alpha2.ApplicationConfigurationComponent{
							ParameterValues: []v1alpha2.ComponentParameterValue{
								{
									Name:  paramName,
									Value: intstr.FromString(paramValue),
								},
							},
						},
						component: v1alpha2.Component{
							Spec: v1alpha2.ComponentSpec{
								Parameters: []v1alpha2.ComponentParameter{
									{
										Name:       paramName,
										FieldPaths: []string{WorkloadNamePath},
									},
								},
							},
						},
						validatingTraits: []ValidatingTrait{
							{traitDefinition: v1alpha2.TraitDefinition{
								Spec: v1alpha2.TraitDefinitionSpec{RevisionEnabled: true},
							}},
						},
					},
				},
			},
			want: []error{
				fmt.Errorf(errFmtWorkloadNameNotEmpty, workloadName),
			},
		},
		{
			caseName: "validation succeeds",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						compName: "example-comp",
						validatingTraits: []ValidatingTrait{
							{traitDefinition: v1alpha2.TraitDefinition{
								Spec: v1alpha2.TraitDefinitionSpec{RevisionEnabled: true},
							}},
						},
					},
				},
			},
			want: nil,
		},
	}

	for _, tc := range tests {
		result := ValidateWorkloadNameForVersioningFn(ctx, tc.validatingAppConfig)
		assert.Equal(t, tc.want, result, fmt.Sprintf("Test case: %q", tc.caseName))
	}

}

func TestValidateTraitAppliableToWorkloadFn(t *testing.T) {
	tests := []struct {
		caseName            string
		validatingAppConfig ValidatingAppConfig
		want                []error
	}{
		{
			caseName: "validate succeed: apply trait to any workload",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						workloadDefinition: v1alpha2.WorkloadDefinition{
							Spec: v1alpha2.WorkloadDefinitionSpec{
								Reference: common.DefinitionReference{
									Name: "TestWorkload",
								},
							},
						},
						validatingTraits: []ValidatingTrait{
							{traitDefinition: v1alpha2.TraitDefinition{
								Spec: v1alpha2.TraitDefinitionSpec{
									AppliesToWorkloads: []string{"*"}, // "*" means apply to any
								},
							}},
							{traitDefinition: v1alpha2.TraitDefinition{
								Spec: v1alpha2.TraitDefinitionSpec{
									AppliesToWorkloads: []string{}, // empty means apply to any
								},
							}},
						},
					},
				},
			},
			want: nil,
		},
		{
			caseName: "validate succeed: apply trait to workload with specific workloadDefinition name",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						workloadDefinition: v1alpha2.WorkloadDefinition{
							ObjectMeta: v1.ObjectMeta{Name: "TestWorkload"}, // matched workload def(type) nmae
						},
						validatingTraits: []ValidatingTrait{
							{traitDefinition: v1alpha2.TraitDefinition{
								Spec: v1alpha2.TraitDefinitionSpec{
									AppliesToWorkloads: []string{"TestWorkload"},
								},
							}},
						},
					},
				},
			},
			want: nil,
		},
		{
			caseName: "validate succeed: apply trait to workload with specific definition reference name",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						workloadDefinition: v1alpha2.WorkloadDefinition{
							Spec: v1alpha2.WorkloadDefinitionSpec{
								Reference: common.DefinitionReference{
									Name: "TestWorkload", // matched CRD name
								},
							},
						},
						validatingTraits: []ValidatingTrait{
							{traitDefinition: v1alpha2.TraitDefinition{
								Spec: v1alpha2.TraitDefinitionSpec{
									AppliesToWorkloads: []string{"TestWorkload"},
								},
							}},
						},
					},
				},
			},
			want: nil,
		},
		{
			caseName: "validate succeed: apply trait to workload with specific group",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						workloadDefinition: v1alpha2.WorkloadDefinition{
							Spec: v1alpha2.WorkloadDefinitionSpec{
								Reference: common.DefinitionReference{
									Name: "testworkloads.example.com", // matched CRD group
								},
							},
						},
						validatingTraits: []ValidatingTrait{
							{traitDefinition: v1alpha2.TraitDefinition{
								Spec: v1alpha2.TraitDefinitionSpec{
									AppliesToWorkloads: []string{"*.example.com"},
								},
							}},
						},
					},
					{
						workloadDefinition: v1alpha2.WorkloadDefinition{
							Spec: v1alpha2.WorkloadDefinitionSpec{
								Reference: common.DefinitionReference{
									Name: "testworkload2s.example.com",
								},
							},
						},
						validatingTraits: []ValidatingTrait{
							{traitDefinition: v1alpha2.TraitDefinition{
								Spec: v1alpha2.TraitDefinitionSpec{
									AppliesToWorkloads: []string{"*.example.com"},
								},
							}},
						},
					},
				},
			},
			want: nil,
		},
		{
			caseName: "validate fail: apply trait to unappliable workload",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						compName: "example-comp",
						workloadDefinition: v1alpha2.WorkloadDefinition{
							ObjectMeta: v1.ObjectMeta{Name: "TestWorkload"},
							Spec: v1alpha2.WorkloadDefinitionSpec{
								Reference: common.DefinitionReference{
									Name: "TestWorkload1.example.foo",
								},
							},
						},
						validatingTraits: []ValidatingTrait{
							{traitDefinition: v1alpha2.TraitDefinition{
								ObjectMeta: v1.ObjectMeta{Name: "TestTrait"},
								Spec: v1alpha2.TraitDefinitionSpec{
									AppliesToWorkloads: []string{"example.com", "TestWorkload2"},
								},
							}},
						},
					},
				},
			},
			want: []error{fmt.Errorf(errFmtUnappliableTrait,
				"TestTrait", "TestWorkload", "example-comp",
				[]string{"example.com", "TestWorkload2"})},
		},
		{
			caseName: "validate fail: applyTo has CRD group but not match workload",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						compName: "example-comp",
						workloadDefinition: v1alpha2.WorkloadDefinition{
							ObjectMeta: v1.ObjectMeta{
								Name: "TestWorkload",
							},
							Spec: v1alpha2.WorkloadDefinitionSpec{
								Reference: common.DefinitionReference{
									Name: "testworkloads.example.foo", // dismatched CRD group
								},
							},
						},
						validatingTraits: []ValidatingTrait{
							{traitDefinition: v1alpha2.TraitDefinition{
								ObjectMeta: v1.ObjectMeta{Name: "TestTrait"},
								Spec: v1alpha2.TraitDefinitionSpec{
									AppliesToWorkloads: []string{"*.example.com"},
								},
							}},
						},
					},
				},
			},
			want: []error{fmt.Errorf(errFmtUnappliableTrait,
				"TestTrait", "TestWorkload", "example-comp",
				[]string{"*.example.com"})},
		},
		{
			caseName: "validate fail: applyTo has CRD name but not match",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						compName: "example-comp",
						workloadDefinition: v1alpha2.WorkloadDefinition{
							ObjectMeta: v1.ObjectMeta{
								Name: "TestWorkload",
							},
							Spec: v1alpha2.WorkloadDefinitionSpec{
								Reference: common.DefinitionReference{
									Name: "bar.example.com", // dismatched CRD name
								},
							},
						},
						validatingTraits: []ValidatingTrait{
							{traitDefinition: v1alpha2.TraitDefinition{
								ObjectMeta: v1.ObjectMeta{Name: "TestTrait"},
								Spec: v1alpha2.TraitDefinitionSpec{
									AppliesToWorkloads: []string{"foo.example.com"},
								},
							}},
						},
					},
				},
			},
			want: []error{fmt.Errorf(errFmtUnappliableTrait,
				"TestTrait", "TestWorkload", "example-comp",
				[]string{"foo.example.com"})},
		},
		{
			caseName: "validate fail: applyTo has definition name but not match",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						compName: "example-comp",
						workloadDefinition: v1alpha2.WorkloadDefinition{
							ObjectMeta: v1.ObjectMeta{
								Name: "bar", // dismatched workload def(type) name
							},
						},
						validatingTraits: []ValidatingTrait{
							{traitDefinition: v1alpha2.TraitDefinition{
								ObjectMeta: v1.ObjectMeta{Name: "TestTrait"},
								Spec: v1alpha2.TraitDefinitionSpec{
									AppliesToWorkloads: []string{"foo"},
								},
							}},
						},
					},
				},
			},
			want: []error{fmt.Errorf(errFmtUnappliableTrait,
				"TestTrait", "bar", "example-comp",
				[]string{"foo"})},
		},
	}

	for _, tc := range tests {
		result := ValidateTraitAppliableToWorkloadFn(ctx, tc.validatingAppConfig)
		assert.Equal(t, tc.want, result, fmt.Sprintf("Test case: %q", tc.caseName))
	}
}

func TestValidateTraitConflictFn(t *testing.T) {
	compName := "testComp"
	traitDefName1 := "testTraitDef1"
	traitDefName2 := "testTraitDef2"
	tests := []struct {
		caseName      string
		conflictRules []string
		traitDef      v1alpha2.TraitDefinition
		want          []error
	}{
		{
			caseName:      "empty conflict rule (no conflict with any other trait)",
			conflictRules: []string{},
			traitDef: v1alpha2.TraitDefinition{
				ObjectMeta: v1.ObjectMeta{
					Name: traitDefName2,
				},
			},
			want: []error{},
		},
		{
			caseName:      "'*' conflict rule (conflict with all other trait)",
			conflictRules: []string{"*"},
			traitDef: v1alpha2.TraitDefinition{
				ObjectMeta: v1.ObjectMeta{
					Name: traitDefName2,
				},
			},
			want: []error{fmt.Errorf(errFmtTraitConflictWithAll, traitDefName1, compName)},
		},
		{
			caseName:      "'*' conflict rule (no conflict if only one trait)",
			conflictRules: []string{"*"},
			traitDef: v1alpha2.TraitDefinition{
				ObjectMeta: v1.ObjectMeta{
					Name: "remove me",
				},
			},
			want: []error{},
		},
		{
			caseName:      "Trait group conflict",
			conflictRules: []string{"*.example.com"},
			traitDef: v1alpha2.TraitDefinition{
				ObjectMeta: v1.ObjectMeta{
					Name: traitDefName2,
				},
				Spec: v1alpha2.TraitDefinitionSpec{
					Reference: common.DefinitionReference{
						Name: "foo.example.com",
					},
				},
			},
			want: []error{fmt.Errorf(errFmtTraitConflict, "*.example.com", traitDefName1, traitDefName2, compName)},
		},
		{
			caseName:      "TraitDefinition name conflict",
			conflictRules: []string{traitDefName2},
			traitDef: v1alpha2.TraitDefinition{
				ObjectMeta: v1.ObjectMeta{
					Name: traitDefName2,
				},
			},
			want: []error{fmt.Errorf(errFmtTraitConflict, traitDefName2, traitDefName1, traitDefName2, compName)},
		},
		{
			caseName:      "CRD name conflict",
			conflictRules: []string{"foo.example.com"},
			traitDef: v1alpha2.TraitDefinition{
				ObjectMeta: v1.ObjectMeta{
					Name: traitDefName2,
				},
				Spec: v1alpha2.TraitDefinitionSpec{
					Reference: common.DefinitionReference{
						Name: "foo.example.com",
					},
				},
			},
			want: []error{fmt.Errorf(errFmtTraitConflict, "foo.example.com", traitDefName1, traitDefName2, compName)},
		},
		{
			caseName:      "LabelSelector conflict",
			conflictRules: []string{"labelSelector:foo=bar"},
			traitDef: v1alpha2.TraitDefinition{
				ObjectMeta: v1.ObjectMeta{
					Name:   traitDefName2,
					Labels: map[string]string{"foo": "bar"},
				},
			},
			want: []error{fmt.Errorf(errFmtTraitConflict, "labelSelector:foo=bar", traitDefName1, traitDefName2, compName)},
		},
		{
			caseName:      "LabelSelector invalid error",
			conflictRules: []string{"labelSelector:,,,"},
			traitDef: v1alpha2.TraitDefinition{
				ObjectMeta: v1.ObjectMeta{
					Name:   traitDefName2,
					Labels: map[string]string{"foo": "bar"},
				},
			},
			want: []error{fmt.Errorf(errFmtInvalidLabelSelector, "labelSelector:,,,",
				fmt.Errorf("found ',', expected: !, identifier, or 'end of string'"))},
		},
	}

	for _, tc := range tests {
		validatingAppConfig := ValidatingAppConfig{
			validatingComps: []ValidatingComponent{
				{
					compName: compName,
					validatingTraits: []ValidatingTrait{
						{
							traitDefinition: v1alpha2.TraitDefinition{
								ObjectMeta: v1.ObjectMeta{Name: traitDefName1},
								Spec: v1alpha2.TraitDefinitionSpec{
									ConflictsWith: tc.conflictRules,
								},
							},
						},
						{traitDefinition: tc.traitDef},
					},
				},
			},
		}
		if len(tc.conflictRules) > 0 && tc.conflictRules[0] == "*" &&
			tc.traitDef.Name == "remove me" {
			// for test case: '*' conflict rule, no conflict if only one trait
			validatingAppConfig.validatingComps[0].validatingTraits =
				validatingAppConfig.validatingComps[0].validatingTraits[:1]
		}
		result := ValidateTraitConflictFn(ctx, validatingAppConfig)
		assert.Equal(t, tc.want, result, fmt.Sprintf("Test case: %q", tc.caseName))
	}
}
