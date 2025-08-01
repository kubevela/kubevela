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

package assemble

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var _ = Describe("Test PrepareBeforeApply", func() {
	Context("Test disableAllComponentRevision parameter", func() {
		var (
			comp     *types.ComponentManifest
			appRev   *v1beta1.ApplicationRevision
			revHash  string
			compName string
			revName  string
		)

		BeforeEach(func() {
			revHash = "test-hash"
			compName = "test-component"
			revName = "test-revision"

			// initialize ComponentManifest
			comp = &types.ComponentManifest{
				Name:         compName,
				RevisionName: revName,
				ComponentOutput: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]interface{}{
							"name": compName,
						},
						"spec": map[string]interface{}{
							"selector": map[string]interface{}{
								"matchLabels": map[string]interface{}{
									"app": compName,
								},
							},
							"template": map[string]interface{}{
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										"app": compName,
									},
								},
								"spec": map[string]interface{}{
									"containers": []interface{}{
										map[string]interface{}{
											"name":  "nginx",
											"image": "nginx:latest",
										},
									},
								},
							},
						},
					},
				},
				ComponentOutputsAndTraits: []*unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"apiVersion": "core.oam.dev/v1alpha2",
							"kind":       "TraitDefinition",
							"metadata": map[string]interface{}{
								"name": "test-trait",
							},
							"spec": map[string]interface{}{
								"appliesToWorkloads": []interface{}{"deployments.apps"},
							},
						},
					},
				},
			}

			// initialize ApplicationRevision
			appRev = &v1beta1.ApplicationRevision{
				Spec: v1beta1.ApplicationRevisionSpec{
					ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
						ComponentDefinitions: map[string]*v1beta1.ComponentDefinition{},
						TraitDefinitions: map[string]*v1beta1.TraitDefinition{
							"test-trait": {
								Spec: v1beta1.TraitDefinitionSpec{
									ManageWorkload: false,
								},
							},
						},
					},
				},
			}
			appRev.SetLabels(map[string]string{
				oam.LabelAppRevisionHash: revHash,
			})
		})

		It("should add component revision label when disableAllComponentRevision is false", func() {
			wl, traits, err := PrepareBeforeApply(comp, appRev, false)
			Expect(err).To(BeNil())
			Expect(wl).NotTo(BeNil())
			Expect(traits).To(HaveLen(1))

			// Verify workload labels
			labels := wl.GetLabels()
			Expect(labels).To(HaveKeyWithValue(oam.LabelAppRevisionHash, revHash))
			Expect(labels).To(HaveKeyWithValue(oam.LabelAppComponentRevision, revName))

			// Verify trait labels
			traitLabels := traits[0].GetLabels()
			Expect(traitLabels).To(HaveKeyWithValue(oam.LabelAppRevisionHash, revHash))
			Expect(traitLabels).To(HaveKeyWithValue(oam.LabelAppComponentRevision, revName))
		})

		It("should not add component revision label when disableAllComponentRevision is true", func() {
			wl, traits, err := PrepareBeforeApply(comp, appRev, true)
			Expect(err).To(BeNil())
			Expect(wl).NotTo(BeNil())
			Expect(traits).To(HaveLen(1))

			// Verify workload labels
			labels := wl.GetLabels()
			Expect(labels).To(HaveKeyWithValue(oam.LabelAppRevisionHash, revHash))
			Expect(labels).NotTo(HaveKey(oam.LabelAppComponentRevision))

			// Verify trait labels
			traitLabels := traits[0].GetLabels()
			Expect(traitLabels).To(HaveKeyWithValue(oam.LabelAppRevisionHash, revHash))
			Expect(traitLabels).NotTo(HaveKey(oam.LabelAppComponentRevision))
		})

		It("should return nil when component output is nil", func() {
			comp.ComponentOutput = nil
			wl, traits, err := PrepareBeforeApply(comp, appRev, false)
			Expect(err).To(BeNil())
			Expect(wl).To(BeNil())
			Expect(traits).To(BeNil())
		})

		It("should return nil when component output has empty apiVersion and kind", func() {
			comp.ComponentOutput = &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": compName,
					},
				},
			}
			wl, traits, err := PrepareBeforeApply(comp, appRev, false)
			Expect(err).To(BeNil())
			Expect(wl).To(BeNil())
			Expect(traits).To(BeNil())
		})

		It("should handle empty component revision name", func() {
			comp.RevisionName = ""
			wl, traits, err := PrepareBeforeApply(comp, appRev, false)
			Expect(err).To(BeNil())
			Expect(wl).NotTo(BeNil())
			Expect(traits).To(HaveLen(1))

			// Verify workload labels
			labels := wl.GetLabels()
			Expect(labels).To(HaveKeyWithValue(oam.LabelAppRevisionHash, revHash))
			Expect(labels).To(HaveKeyWithValue(oam.LabelAppComponentRevision, ""))

			// Verify trait labels
			traitLabels := traits[0].GetLabels()
			Expect(traitLabels).To(HaveKeyWithValue(oam.LabelAppRevisionHash, revHash))
			Expect(traitLabels).To(HaveKeyWithValue(oam.LabelAppComponentRevision, ""))
		})
	})
})
