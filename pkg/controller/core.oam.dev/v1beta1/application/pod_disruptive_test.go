package application

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestPodDisruptiveTraitDefinition(t *testing.T) {
	tests := []struct {
		name     string
		oldRev   *v1beta1.ApplicationRevision
		newRev   *v1beta1.ApplicationRevision
		expected bool
	}{
		{
			name: "same podDisruptive value should be equal",
			oldRev: &v1beta1.ApplicationRevision{
				Spec: v1beta1.ApplicationRevisionSpec{
					ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
						Application: v1beta1.Application{
							Spec: v1beta1.ApplicationSpec{
								Components: []common.ApplicationComponent{
									{
										Name: "test-comp",
										Type: "worker",
									},
								},
							},
						},
						TraitDefinitions: map[string]*v1beta1.TraitDefinition{
							"configmap": {
								ObjectMeta: metav1.ObjectMeta{
									Name: "configmap",
								},
								Spec: v1beta1.TraitDefinitionSpec{
									PodDisruptive: true,
									Schematic: &common.Schematic{
										CUE: &common.CUE{
											Template: "patch: {}",
										},
									},
								},
							},
						},
					},
				},
			},
			newRev: &v1beta1.ApplicationRevision{
				Spec: v1beta1.ApplicationRevisionSpec{
					ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
						Application: v1beta1.Application{
							Spec: v1beta1.ApplicationSpec{
								Components: []common.ApplicationComponent{
									{
										Name: "test-comp",
										Type: "worker",
									},
								},
							},
						},
						TraitDefinitions: map[string]*v1beta1.TraitDefinition{
							"configmap": {
								ObjectMeta: metav1.ObjectMeta{
									Name: "configmap",
								},
								Spec: v1beta1.TraitDefinitionSpec{
									PodDisruptive: true,
									Schematic: &common.Schematic{
										CUE: &common.CUE{
											Template: "patch: {}",
										},
									},
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "different podDisruptive value should not be equal",
			oldRev: &v1beta1.ApplicationRevision{
				Spec: v1beta1.ApplicationRevisionSpec{
					ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
						Application: v1beta1.Application{
							Spec: v1beta1.ApplicationSpec{
								Components: []common.ApplicationComponent{
									{
										Name: "test-comp",
										Type: "worker",
									},
								},
							},
						},
						TraitDefinitions: map[string]*v1beta1.TraitDefinition{
							"configmap": {
								ObjectMeta: metav1.ObjectMeta{
									Name: "configmap",
								},
								Spec: v1beta1.TraitDefinitionSpec{
									PodDisruptive: false,
									Schematic: &common.Schematic{
										CUE: &common.CUE{
											Template: "patch: {}",
										},
									},
								},
							},
						},
					},
				},
			},
			newRev: &v1beta1.ApplicationRevision{
				Spec: v1beta1.ApplicationRevisionSpec{
					ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
						Application: v1beta1.Application{
							Spec: v1beta1.ApplicationSpec{
								Components: []common.ApplicationComponent{
									{
										Name: "test-comp",
										Type: "worker",
									},
								},
							},
						},
						TraitDefinitions: map[string]*v1beta1.TraitDefinition{
							"configmap": {
								ObjectMeta: metav1.ObjectMeta{
									Name: "configmap",
								},
								Spec: v1beta1.TraitDefinitionSpec{
									PodDisruptive: true,
									Schematic: &common.Schematic{
										CUE: &common.CUE{
											Template: "patch: {}",
										},
									},
								},
							},
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeepEqualRevision(tt.oldRev, tt.newRev)
			assert.Equal(t, tt.expected, result, "DeepEqualRevision() result should match expected")
		})
	}
}
