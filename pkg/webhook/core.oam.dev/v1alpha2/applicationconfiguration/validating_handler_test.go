package applicationconfiguration

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
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

func TestValidateTraitObjectFn(t *testing.T) {
	traitWithName := unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	unstructured.SetNestedField(traitWithName.Object, "test", TraitTypeField)

	traitWithProperties := unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	unstructured.SetNestedField(traitWithProperties.Object, "test", TraitSpecField)

	traitWithoutGVK := unstructured.Unstructured{}
	traitWithoutGVK.SetAPIVersion("")
	traitWithoutGVK.SetKind("")

	tests := []struct {
		caseName     string
		traitContent unstructured.Unstructured
		want         string
	}{
		{
			caseName:     "the trait contains 'name' info that should be mutated to GVK",
			traitContent: traitWithName,
			want:         "the trait contains 'name' info",
		},
		{
			caseName:     "the trait contains 'properties' info that should be mutated to spec",
			traitContent: traitWithProperties,
			want:         "the trait contains 'properties' info",
		},
		{
			caseName:     "the trait data missing GVK",
			traitContent: traitWithoutGVK,
			want:         "the trait data missing GVK",
		},
	}

	for _, tc := range tests {
		vAppConfig := ValidatingAppConfig{
			validatingComps: []ValidatingComponent{
				{
					validatingTraits: []ValidatingTrait{
						{
							traitContent: tc.traitContent,
						},
					},
				},
			},
		}
		allErrs := ValidateTraitObjectFn(ctx, vAppConfig)
		result := utilerrors.NewAggregate(allErrs).Error()
		assert.Contains(t, result, tc.want, fmt.Sprintf("Test case: %q", tc.caseName))
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
			caseName: "apply trait to any workload",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						workloadDefinition: v1alpha2.WorkloadDefinition{
							Spec: v1alpha2.WorkloadDefinitionSpec{
								Reference: v1alpha2.DefinitionReference{
									Name: "TestWorkload",
								},
							},
						},
						validatingTraits: []ValidatingTrait{
							{traitDefinition: v1alpha2.TraitDefinition{
								Spec: v1alpha2.TraitDefinitionSpec{
									AppliesToWorkloads: []string{"*"},
								},
							}},
							{traitDefinition: v1alpha2.TraitDefinition{
								Spec: v1alpha2.TraitDefinitionSpec{
									AppliesToWorkloads: []string{},
								},
							}},
						},
					},
				},
			},
			want: nil,
		},
		{
			caseName: "apply trait to workload with specific type",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						component: v1alpha2.Component{ObjectMeta: v1.ObjectMeta{
							Labels: map[string]string{oam.WorkloadTypeLabel: "TestWorkload"},
						}},
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
			caseName: "apply trait to workload with specific definition reference name",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						workloadDefinition: v1alpha2.WorkloadDefinition{
							Spec: v1alpha2.WorkloadDefinitionSpec{
								Reference: v1alpha2.DefinitionReference{
									Name: "TestWorkload",
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
			caseName: "apply trait to workload with specific group",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						workloadDefinition: v1alpha2.WorkloadDefinition{
							TypeMeta: v1.TypeMeta{
								APIVersion: "example.com/v1",
								Kind:       "TestWorkload",
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
			caseName: "apply trait to unappliable workload",
			validatingAppConfig: ValidatingAppConfig{
				validatingComps: []ValidatingComponent{
					{
						compName: "example-comp",
						component: v1alpha2.Component{ObjectMeta: v1.ObjectMeta{
							Labels: map[string]string{oam.WorkloadTypeLabel: "TestWorkload0"},
						}},
						workloadDefinition: v1alpha2.WorkloadDefinition{
							TypeMeta: v1.TypeMeta{
								APIVersion: "unknown.group/v1",
								Kind:       "TestWorkload1",
							},
							Spec: v1alpha2.WorkloadDefinitionSpec{
								Reference: v1alpha2.DefinitionReference{
									Name: "TestWorkload2",
								},
							},
						},
						validatingTraits: []ValidatingTrait{
							{traitDefinition: v1alpha2.TraitDefinition{
								TypeMeta: v1.TypeMeta{
									APIVersion: "example.com/v1",
									Kind:       "TestTrait",
								},
								Spec: v1alpha2.TraitDefinitionSpec{
									AppliesToWorkloads: []string{"example.com", "TestWorkload"},
								},
							}},
						},
					},
				},
			},
			want: []error{fmt.Errorf(errFmtUnappliableTrait,
				"example.com/v1, Kind=TestTrait", "unknown.group/v1, Kind=TestWorkload1", "example-comp",
				[]string{"example.com", "TestWorkload"})},
		},
	}

	for _, tc := range tests {
		result := ValidateTraitAppliableToWorkloadFn(ctx, tc.validatingAppConfig)
		assert.Equal(t, tc.want, result, fmt.Sprintf("Test case: %q", tc.caseName))
	}
}
