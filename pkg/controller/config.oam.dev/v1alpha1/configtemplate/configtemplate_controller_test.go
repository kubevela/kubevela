/*
Copyright 2026 The KubeVela Authors.

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

package configtemplate

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
)

var _ = Describe("ConfigTemplate controller", func() {
	ctx := context.Background()

	It("should extract a schema and set Phase=Available for a valid CUE template", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "valid-template", Namespace: "default"},
			Spec: configv1alpha1.ConfigTemplateSpec{
				Template: `
template: {
	parameter: {
		username: string
	}
	output: {
		apiVersion: "v1"
		kind: "Secret"
		stringData: {
			username: parameter.username
		}
	}
}
`,
			},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())

		var got configv1alpha1.ConfigTemplate
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), &got)).Should(Succeed())
			return got.Status.Phase
		}, 15*time.Second, time.Second).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))
		Expect(got.Status.Schema).ShouldNot(BeNil())
		Expect(got.Status.Schema.Raw).ShouldNot(BeEmpty())
		Expect(got.Status.GetCondition(condition.TypeSynced).Status).Should(BeEquivalentTo("True"))
	})

	It("should set Phase=Error for an invalid CUE template", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "invalid-template", Namespace: "default"},
			Spec: configv1alpha1.ConfigTemplateSpec{
				Template: `this is not valid cue {{{`,
			},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())

		var got configv1alpha1.ConfigTemplate
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), &got)).Should(Succeed())
			return got.Status.Phase
		}, 15*time.Second, time.Second).Should(Equal(configv1alpha1.ConfigTemplatePhaseError))
		Expect(got.Status.GetCondition(condition.TypeSynced).Message).ShouldNot(BeEmpty())
	})
})
