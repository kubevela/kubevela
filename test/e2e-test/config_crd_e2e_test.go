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

package controllers_test

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
)

const (
	configE2ETimeout      = 30 * time.Second
	configE2EPollInterval = 2 * time.Second

	configE2ECUETemplate = `
template: {
	parameter: {
		username: string
	}
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		stringData: {
			username: parameter.username
		}
	}
}
`
)

func configE2ERawExtension(v interface{}) *runtime.RawExtension {
	data, err := json.Marshal(v)
	Expect(err).ShouldNot(HaveOccurred())
	return &runtime.RawExtension{Raw: data}
}

var _ = Describe("CRD-based config management (config.oam.dev/v1alpha1)", func() {
	ctx := context.Background()

	var namespace string
	var ns corev1.Namespace

	BeforeEach(func() {
		namespace = randomNamespaceName("config-crd-e2e")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, configE2ETimeout, configE2EPollInterval).Should(Succeed())
	})

	AfterEach(func() {
		By("Cleaning up Configs")
		_ = k8sClient.DeleteAllOf(ctx, &configv1alpha1.Config{}, client.InNamespace(namespace))
		By("Cleaning up ConfigTemplates")
		_ = k8sClient.DeleteAllOf(ctx, &configv1alpha1.ConfigTemplate{}, client.InNamespace(namespace))
		By("Cleaning up Secrets")
		_ = k8sClient.DeleteAllOf(ctx, &corev1.Secret{}, client.InNamespace(namespace))
		By("Cleaning up ConfigMaps")
		_ = k8sClient.DeleteAllOf(ctx, &corev1.ConfigMap{}, client.InNamespace(namespace))
		By("Deleting test namespace")
		_ = k8sClient.Delete(ctx, &ns)
	})

	It("ConfigTemplate with valid CUE becomes Available and exposes a JSON schema", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ct-valid",
				Namespace: namespace,
			},
			Spec: configv1alpha1.ConfigTemplateSpec{
				Template: configE2ECUETemplate,
			},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())

		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct); err != nil {
				return ""
			}
			return ct.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))

		Expect(ct.Status.Schema).ShouldNot(BeNil())
		Expect(ct.Status.Schema.Raw).ShouldNot(BeEmpty())
	})

	It("ConfigTemplate with invalid CUE transitions to Error phase", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ct-invalid",
				Namespace: namespace,
			},
			Spec: configv1alpha1.ConfigTemplateSpec{
				Template: `this is not valid cue {{{`,
			},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())

		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct); err != nil {
				return ""
			}
			return ct.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigTemplatePhaseError))
	})

	It("Config with inline spec.properties materializes an owned Secret", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "ct-inline", Namespace: namespace},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: configE2ECUETemplate},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)
			return ct.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))

		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-inline", Namespace: namespace},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{
					Name:      "ct-inline",
					Namespace: namespace,
				},
				Properties: configE2ERawExtension(map[string]string{"username": "alice"}),
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		Eventually(func() configv1alpha1.ConfigPhase {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), cfg); err != nil {
				return ""
			}
			return cfg.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigPhaseAvailable))

		Expect(cfg.Status.SecretRef).ShouldNot(BeNil())
		Expect(cfg.Status.SecretRef.Name).Should(Equal("cfg-inline"))

		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "cfg-inline"}, &secret)).Should(Succeed())
		Expect(string(secret.Data["username"])).Should(Equal("alice"))

		Expect(secret.OwnerReferences).Should(HaveLen(1))
		Expect(secret.OwnerReferences[0].Name).Should(Equal("cfg-inline"))
		Expect(secret.OwnerReferences[0].Kind).Should(Equal("Config"))
		Expect(*secret.OwnerReferences[0].Controller).Should(BeTrue())
		Expect(*secret.OwnerReferences[0].BlockOwnerDeletion).Should(BeTrue())
	})

	It("Config with spec.propertiesFrom.secretRef materializes an owned Secret", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "ct-fromsecret", Namespace: namespace},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: configE2ECUETemplate},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)
			return ct.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))

		propsSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "props-src", Namespace: namespace},
			Data:       map[string][]byte{"properties": []byte(`{"username":"bob"}`)},
		}
		Expect(k8sClient.Create(ctx, propsSecret)).Should(Succeed())

		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-fromsecret", Namespace: namespace},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{
					Name:      "ct-fromsecret",
					Namespace: namespace,
				},
				PropertiesFrom: &configv1alpha1.PropertiesReference{
					SecretRef: configv1alpha1.SecretKeySelector{Name: "props-src"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		Eventually(func() configv1alpha1.ConfigPhase {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), cfg); err != nil {
				return ""
			}
			return cfg.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigPhaseAvailable))

		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "cfg-fromsecret"}, &secret)).Should(Succeed())
		Expect(string(secret.Data["username"])).Should(Equal("bob"))
	})

	It("Config with both spec.properties and spec.propertiesFrom transitions to Error phase", func() {
		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-bad", Namespace: namespace},
			Spec: configv1alpha1.ConfigSpec{
				Properties: configE2ERawExtension(map[string]string{"username": "alice"}),
				PropertiesFrom: &configv1alpha1.PropertiesReference{
					SecretRef: configv1alpha1.SecretKeySelector{Name: "any-secret"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		Eventually(func() configv1alpha1.ConfigPhase {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), cfg); err != nil {
				return ""
			}
			return cfg.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigPhaseError))

		Expect(cfg.Status.GetCondition("Synced").Message).Should(ContainSubstring("mutually exclusive"))
	})

	It("Deleting a Config triggers GC of the owned Secret via ownerRef", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "ct-gc", Namespace: namespace},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: configE2ECUETemplate},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)
			return ct.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))

		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-gc", Namespace: namespace},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{
					Name:      "ct-gc",
					Namespace: namespace,
				},
				Properties: configE2ERawExtension(map[string]string{"username": "carol"}),
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		By("Waiting for Secret to be materialized")
		Eventually(func() error {
			var secret corev1.Secret
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "cfg-gc"}, &secret)
		}, configE2ETimeout, configE2EPollInterval).Should(Succeed())

		By("Deleting the Config")
		Expect(k8sClient.Delete(ctx, cfg)).Should(Succeed())

		By("Waiting for owned Secret to be garbage collected")
		Eventually(func() bool {
			var secret corev1.Secret
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "cfg-gc"}, &secret)
			return apierrors.IsNotFound(err)
		}, configE2ETimeout, configE2EPollInterval).Should(BeTrue())
	})

	It("falls back to a legacy config-template-* ConfigMap when no ConfigTemplate CRD exists", func() {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "config-template-legacy-bar",
				Namespace: namespace,
				Labels:    map[string]string{"config.oam.dev/catalog": "velacore-config"},
			},
			Data: map[string]string{"template": configE2ECUETemplate},
		}
		Expect(k8sClient.Create(ctx, cm)).Should(Succeed())

		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-legacy", Namespace: namespace},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{
					Name:      "legacy-bar",
					Namespace: namespace,
				},
				Properties: configE2ERawExtension(map[string]string{"username": "dave"}),
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		Eventually(func() configv1alpha1.ConfigPhase {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), cfg); err != nil {
				return ""
			}
			return cfg.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigPhaseAvailable))

		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "cfg-legacy"}, &secret)).Should(Succeed())
		Expect(string(secret.Data["username"])).Should(Equal("dave"))
	})
})
