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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/config"
)

var readSensitiveConfigApplication = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: read-from
spec:
  components: []
  workflow:
    steps:
    - name: read
      type: read-config
      properties:
        name: cfg-from
`

var _ = Describe("The sensitive rules", Label("core-config"), func() {
	ctx := context.Background()

	var namespace string

	BeforeEach(func() {
		namespace = randomNamespaceName("config-sensitive-e2e")
		ns := createNamespace(ctx, namespace)
		DeferCleanup(func() {
			_ = k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(namespace))
			_ = k8sClient.DeleteAllOf(ctx, &configv1alpha1.Config{}, client.InNamespace(namespace))
			_ = k8sClient.DeleteAllOf(ctx, &configv1alpha1.ConfigTemplate{}, client.InNamespace(namespace))
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground)))).To(Succeed())
		})
	})

	availableTemplate := func(name string, sensitive bool) *configv1alpha1.ConfigTemplate {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: configE2ECUETemplate, Sensitive: sensitive},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)
			return ct.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))
		return ct
	}

	It("the API server refuses to turn a ConfigTemplate's sensitive off", func() {
		ct := availableTemplate("ct-sensitive", true)

		// the CRD's rule answers: schema validation runs before the validating webhooks, and
		// only the API server's own refusal is Invalid (a webhook's denial is Forbidden)
		patch := client.MergeFrom(ct.DeepCopy())
		ct.Spec.Sensitive = false
		err := k8sClient.Patch(ctx, ct, patch)
		Expect(apierrors.IsInvalid(err)).Should(BeTrue(), "%v", err)
		Expect(err.Error()).Should(ContainSubstring(config.SensitiveTurnedOffMessage))

		var got configv1alpha1.ConfigTemplate
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), &got)).Should(Succeed())
		Expect(got.Spec.Sensitive).Should(BeTrue())
	})

	It("a propertiesFrom Config on a template that is not sensitive cannot be read back by read-config", func() {
		availableTemplate("ct-plain", false)
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "props", Namespace: namespace},
			Data:       map[string][]byte{"properties": []byte(`{"username":"carol"}`)},
		})).Should(Succeed())
		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-from", Namespace: namespace},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef:    &configv1alpha1.ConfigTemplateReference{Name: "ct-plain", Namespace: namespace},
				PropertiesFrom: &configv1alpha1.PropertiesReference{SecretRef: configv1alpha1.SecretKeySelector{Name: "props"}},
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigPhase {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), cfg)
			return cfg.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigPhaseAvailable))

		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "cfg-from"}, &secret)).Should(Succeed())
		Expect(secret.Annotations).Should(HaveKeyWithValue(apitypes.AnnotationConfigSensitive, "true"))

		By("read-config refuses it")
		var app v1beta1.Application
		Expect(yaml.Unmarshal([]byte(readSensitiveConfigApplication), &app)).Should(Succeed())
		app.Namespace = namespace
		Expect(k8sClient.Create(ctx, &app)).Should(Succeed())
		Eventually(func() string {
			var got v1beta1.Application
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&app), &got); err != nil || got.Status.Workflow == nil {
				return ""
			}
			for _, step := range got.Status.Workflow.Steps {
				if step.Name == "read" {
					return step.Message
				}
			}
			return ""
		}, time.Minute, 2*time.Second).Should(ContainSubstring(config.ErrSensitiveConfig.Error()))
	})
})
