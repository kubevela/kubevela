/*
Copyright 2022 The KubeVela Authors.

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

package application

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
)

var _ = Describe("Test dispatch stage", func() {
	BeforeEach(func() {
		traitDefinition := v1beta1.TraitDefinition{
			ObjectMeta: v1.ObjectMeta{
				Name:      "kruise-rollout",
				Namespace: velatypes.DefaultKubeVelaNS,
			},
			Spec: v1beta1.TraitDefinitionSpec{
				Stage: v1beta1.PreDispatch,
			},
		}
		Expect(k8sClient.Create(context.Background(), &traitDefinition)).Should(BeNil())
	})

	It("Test get dispatch stage from trait", func() {
		appRev := v1beta1.ApplicationRevision{
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					TraitDefinitions: map[string]*v1beta1.TraitDefinition{
						"gateway": {
							Spec: v1beta1.TraitDefinitionSpec{
								Stage: v1beta1.PostDispatch,
							},
						},
						"hpa": {
							Spec: v1beta1.TraitDefinitionSpec{},
						},
					},
				},
			},
		}
		var annotations = make(map[string]string)
		stage, err := getTraitDispatchStage(k8sClient, "kruise-rollout", &appRev, annotations)
		Expect(err).Should(BeNil())
		Expect(stage).Should(BeEquivalentTo(PreDispatch))
		stage, err = getTraitDispatchStage(k8sClient, "gateway", &appRev, annotations)
		Expect(err).Should(BeNil())
		Expect(stage).Should(BeEquivalentTo(PostDispatch))
		stage, err = getTraitDispatchStage(k8sClient, "hpa", &appRev, annotations)
		Expect(err).Should(BeNil())
		Expect(stage).Should(BeEquivalentTo(DefaultDispatch))
		stage, err = getTraitDispatchStage(k8sClient, "not-exist", &appRev, annotations)
		Expect(err).ShouldNot(BeNil())
		Expect(stage).Should(BeEquivalentTo(DefaultDispatch))
	})
})

var _ = Describe("Test componentPropertiesChanged", func() {
	It("should return true when component not in revision (first deployment)", func() {
		comp := &appfile.Component{
			Name: "test-component",
			Type: "webservice",
			Params: map[string]interface{}{
				"image": "nginx:latest",
			},
		}
		appRev := &v1beta1.ApplicationRevision{
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					Application: v1beta1.Application{
						Spec: v1beta1.ApplicationSpec{
							Components: []common.ApplicationComponent{},
						},
					},
				},
			},
		}
		Expect(componentPropertiesChanged(comp, appRev)).Should(BeTrue())
	})

	It("should return false when component properties unchanged", func() {
		properties := map[string]interface{}{
			"image": "nginx:latest",
			"port":  80,
		}
		propertiesJSON, _ := json.Marshal(properties)

		comp := &appfile.Component{
			Name:   "test-component",
			Type:   "webservice",
			Params: properties,
		}
		appRev := &v1beta1.ApplicationRevision{
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					Application: v1beta1.Application{
						Spec: v1beta1.ApplicationSpec{
							Components: []common.ApplicationComponent{
								{
									Name: "test-component",
									Type: "webservice",
									Properties: &runtime.RawExtension{
										Raw: propertiesJSON,
									},
								},
							},
						},
					},
				},
			},
		}
		Expect(componentPropertiesChanged(comp, appRev)).Should(BeFalse())
	})

	It("should return true when component properties changed", func() {
		oldProperties := map[string]interface{}{
			"image": "nginx:1.0",
			"port":  80,
		}
		oldPropertiesJSON, _ := json.Marshal(oldProperties)

		newProperties := map[string]interface{}{
			"image": "nginx:2.0",
			"port":  80,
		}

		comp := &appfile.Component{
			Name:   "test-component",
			Type:   "webservice",
			Params: newProperties,
		}
		appRev := &v1beta1.ApplicationRevision{
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					Application: v1beta1.Application{
						Spec: v1beta1.ApplicationSpec{
							Components: []common.ApplicationComponent{
								{
									Name: "test-component",
									Type: "webservice",
									Properties: &runtime.RawExtension{
										Raw: oldPropertiesJSON,
									},
								},
							},
						},
					},
				},
			},
		}
		Expect(componentPropertiesChanged(comp, appRev)).Should(BeTrue())
	})

	It("should return true when component type changed", func() {
		properties := map[string]interface{}{
			"image": "nginx:latest",
		}
		propertiesJSON, _ := json.Marshal(properties)

		comp := &appfile.Component{
			Name:   "test-component",
			Type:   "worker",
			Params: properties,
		}
		appRev := &v1beta1.ApplicationRevision{
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					Application: v1beta1.Application{
						Spec: v1beta1.ApplicationSpec{
							Components: []common.ApplicationComponent{
								{
									Name: "test-component",
									Type: "webservice",
									Properties: &runtime.RawExtension{
										Raw: propertiesJSON,
									},
								},
							},
						},
					},
				},
			},
		}
		Expect(componentPropertiesChanged(comp, appRev)).Should(BeTrue())
	})

	It("should return false when both properties are nil", func() {
		comp := &appfile.Component{
			Name:   "test-component",
			Type:   "webservice",
			Params: nil,
		}
		appRev := &v1beta1.ApplicationRevision{
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					Application: v1beta1.Application{
						Spec: v1beta1.ApplicationSpec{
							Components: []common.ApplicationComponent{
								{
									Name:       "test-component",
									Type:       "webservice",
									Properties: nil,
								},
							},
						},
					},
				},
			},
		}
		Expect(componentPropertiesChanged(comp, appRev)).Should(BeFalse())
	})

	It("should return true when properties removed (nil current, non-empty previous)", func() {
		comp := &appfile.Component{
			Name:   "test-component",
			Type:   "webservice",
			Params: nil, // Properties removed
		}
		appRev := &v1beta1.ApplicationRevision{
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					Application: v1beta1.Application{
						Spec: v1beta1.ApplicationSpec{
							Components: []common.ApplicationComponent{
								{
									Name: "test-component",
									Type: "webservice",
									Properties: &runtime.RawExtension{
										Raw: []byte(`{"image":"nginx:1.0","port":80}`),
									},
								},
							},
						},
					},
				},
			},
		}
		Expect(componentPropertiesChanged(comp, appRev)).Should(BeTrue())
	})

	It("should return true on JSON unmarshal error (conservative)", func() {
		comp := &appfile.Component{
			Name: "test-component",
			Type: "webservice",
			Params: map[string]interface{}{
				"image": "nginx:latest",
			},
		}
		appRev := &v1beta1.ApplicationRevision{
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					Application: v1beta1.Application{
						Spec: v1beta1.ApplicationSpec{
							Components: []common.ApplicationComponent{
								{
									Name: "test-component",
									Type: "webservice",
									Properties: &runtime.RawExtension{
										Raw: []byte("invalid json"),
									},
								},
							},
						},
					},
				},
			},
		}
		Expect(componentPropertiesChanged(comp, appRev)).Should(BeTrue())
	})
})
