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

package config

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
)

const testCUETemplate = `
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
`

func rawExtension(v interface{}) *runtime.RawExtension {
	data, err := json.Marshal(v)
	Expect(err).ShouldNot(HaveOccurred())
	return &runtime.RawExtension{Raw: data}
}

// eventuallyConfigPhase polls the Config until its status phase is observed, driving
// the assertion off the live background manager rather than a synchronous Reconcile
// call (which would race the manager's own informer cache).
func eventuallyConfigPhase(ctx context.Context, key client.ObjectKey, got *configv1alpha1.Config) configv1alpha1.ConfigPhase {
	var phase configv1alpha1.ConfigPhase
	Eventually(func() configv1alpha1.ConfigPhase {
		Expect(k8sClient.Get(ctx, key, got)).Should(Succeed())
		phase = got.Status.Phase
		return phase
	}, 15*time.Second, time.Second).Should(Or(Equal(configv1alpha1.ConfigPhaseAvailable), Equal(configv1alpha1.ConfigPhaseError)))
	return phase
}

var _ = Describe("Config controller", func() {
	ctx := context.Background()

	It("materializes a Secret from inline spec.properties and a CRD ConfigTemplate", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "tmpl-inline", Namespace: "default"},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: testCUETemplate},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)).Should(Succeed())
			return ct.Status.Phase
		}, 15*time.Second, time.Second).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))

		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-inline", Namespace: "default"},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "tmpl-inline", Namespace: "default"},
				Properties:  rawExtension(map[string]string{"username": "alice"}),
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		var gotCfg configv1alpha1.Config
		Expect(eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), &gotCfg)).Should(Equal(configv1alpha1.ConfigPhaseAvailable))
		Expect(gotCfg.Status.SecretRef).ShouldNot(BeNil())
		Expect(gotCfg.Status.SecretRef.Name).Should(Equal("cfg-inline"))

		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "cfg-inline"}, &secret)).Should(Succeed())
		Expect(string(secret.Data["username"])).Should(Equal("alice"))

		Expect(secret.OwnerReferences).Should(HaveLen(1))
		Expect(secret.OwnerReferences[0].Name).Should(Equal("cfg-inline"))
		Expect(secret.OwnerReferences[0].Kind).Should(Equal("Config"))
		Expect(*secret.OwnerReferences[0].Controller).Should(BeTrue())
	})

	It("materializes a Secret from spec.propertiesFrom.secretRef", func() {
		propsSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds-secret", Namespace: "default"},
			Data:       map[string][]byte{"properties": []byte(`{"username":"bob"}`)},
		}
		Expect(k8sClient.Create(ctx, propsSecret)).Should(Succeed())

		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-fromsecret", Namespace: "default"},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef:    &configv1alpha1.ConfigTemplateReference{Name: "tmpl-inline", Namespace: "default"},
				PropertiesFrom: &configv1alpha1.PropertiesReference{SecretRef: configv1alpha1.SecretKeySelector{Name: "creds-secret"}},
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		var gotCfg configv1alpha1.Config
		Expect(eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), &gotCfg)).Should(Equal(configv1alpha1.ConfigPhaseAvailable))

		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "cfg-fromsecret"}, &secret)).Should(Succeed())
		Expect(string(secret.Data["username"])).Should(Equal("bob"))
	})

	It("rejects a Config with both properties and propertiesFrom set", func() {
		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-bad", Namespace: "default"},
			Spec: configv1alpha1.ConfigSpec{
				Properties:     rawExtension(map[string]string{"username": "alice"}),
				PropertiesFrom: &configv1alpha1.PropertiesReference{SecretRef: configv1alpha1.SecretKeySelector{Name: "creds-secret"}},
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		var gotCfg configv1alpha1.Config
		Expect(eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), &gotCfg)).Should(Equal(configv1alpha1.ConfigPhaseError))
		Expect(gotCfg.Status.GetCondition("Synced").Message).Should(ContainSubstring("mutually exclusive"))
	})

	It("falls back to a legacy config-template-* ConfigMap when no ConfigTemplate CRD exists", func() {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "config-template-legacy-foo",
				Namespace: "default",
				Labels:    map[string]string{"config.oam.dev/catalog": "velacore-config"},
			},
			Data: map[string]string{"template": testCUETemplate},
		}
		Expect(k8sClient.Create(ctx, cm)).Should(Succeed())

		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-legacy", Namespace: "default"},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "legacy-foo", Namespace: "default"},
				Properties:  rawExtension(map[string]string{"username": "carol"}),
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		var gotCfg configv1alpha1.Config
		Expect(eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), &gotCfg)).Should(Equal(configv1alpha1.ConfigPhaseAvailable))

		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "cfg-legacy"}, &secret)).Should(Succeed())
		Expect(string(secret.Data["username"])).Should(Equal("carol"))
	})
})
