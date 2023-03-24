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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
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

		stage, err := getTraitDispatchStage(k8sClient, "kruise-rollout", &appRev)
		Expect(err).Should(BeNil())
		Expect(stage).Should(BeEquivalentTo(PreDispatch))
		stage, err = getTraitDispatchStage(k8sClient, "gateway", &appRev)
		Expect(err).Should(BeNil())
		Expect(stage).Should(BeEquivalentTo(PostDispatch))
		stage, err = getTraitDispatchStage(k8sClient, "hpa", &appRev)
		Expect(err).Should(BeNil())
		Expect(stage).Should(BeEquivalentTo(DefaultDispatch))
		stage, err = getTraitDispatchStage(k8sClient, "not-exist", &appRev)
		Expect(err).ShouldNot(BeNil())
		Expect(stage).Should(BeEquivalentTo(DefaultDispatch))
	})
})
