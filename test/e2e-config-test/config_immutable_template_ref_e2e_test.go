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

package config_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/config"
)

var _ = Describe("The Config CRD's templateRef rule", Label("core-config"), func() {
	ctx := context.Background()

	var namespace string

	BeforeEach(func() {
		namespace = randomNamespaceName("config-immutable-e2e")
		ns := createNamespace(ctx, namespace)
		DeferCleanup(func() {
			_ = k8sClient.DeleteAllOf(ctx, &configv1alpha1.Config{}, client.InNamespace(namespace))
			_ = k8sClient.DeleteAllOf(ctx, &configv1alpha1.ConfigTemplate{}, client.InNamespace(namespace))
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground)))).To(Succeed())
		})
	})

	availableTemplate := func(name string) {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: configE2ECUETemplate},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)
			return ct.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))
	}

	// the CRD's rule answers, not the webhook: schema validation runs before the validating
	// webhooks, and only the API server's own refusal is Invalid (a webhook's denial is Forbidden)
	refused := func(err error) {
		Expect(apierrors.IsInvalid(err)).Should(BeTrue(), "%v", err)
		Expect(err.Error()).Should(ContainSubstring(config.TemplateRefImmutableMessage))
	}

	It("refuses to change, add or remove a Config's templateRef", func() {
		availableTemplate("ct-a")
		availableTemplate("ct-b")

		withRef := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "with-ref", Namespace: namespace},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "ct-a", Namespace: namespace},
				Properties:  configE2ERawExtension(map[string]string{"username": "ivy"}),
			},
		}
		Expect(k8sClient.Create(ctx, withRef)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigPhase {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(withRef), withRef)
			return withRef.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigPhaseAvailable))

		noRef := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "no-ref", Namespace: namespace},
			Spec: configv1alpha1.ConfigSpec{
				Properties: configE2ERawExtension(map[string]string{"username": "jane"}),
			},
		}
		Expect(k8sClient.Create(ctx, noRef)).Should(Succeed())

		By("changing the template name")
		patched := withRef.DeepCopy()
		patched.Spec.TemplateRef.Name = "ct-b"
		refused(k8sClient.Patch(ctx, patched, client.MergeFrom(withRef)))

		By("changing the template namespace")
		patched = withRef.DeepCopy()
		patched.Spec.TemplateRef.Namespace = "vela-system"
		refused(k8sClient.Patch(ctx, patched, client.MergeFrom(withRef)))

		By("removing it")
		patched = withRef.DeepCopy()
		patched.Spec.TemplateRef = nil
		refused(k8sClient.Patch(ctx, patched, client.MergeFrom(withRef)))

		By("adding it to a Config that has none")
		patched = noRef.DeepCopy()
		patched.Spec.TemplateRef = &configv1alpha1.ConfigTemplateReference{Name: "ct-a", Namespace: namespace}
		refused(k8sClient.Patch(ctx, patched, client.MergeFrom(noRef)))

		By("an update that leaves templateRef as it is")
		patched = withRef.DeepCopy()
		patched.Spec.Properties = configE2ERawExtension(map[string]string{"username": "jane"})
		Expect(k8sClient.Patch(ctx, patched, client.MergeFrom(withRef))).Should(Succeed())
	})
})
