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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

// eventuallyConfigPhase polls the Config until it reaches want, tolerating transient phases
// in between (e.g. Error while a just-Available ConfigTemplate is still propagating to the
// controller's cache) instead of stopping at the first terminal-looking phase it sees.
func eventuallyConfigPhase(ctx context.Context, key client.ObjectKey, want configv1alpha1.ConfigPhase, got *configv1alpha1.Config) {
	Eventually(func() configv1alpha1.ConfigPhase {
		Expect(k8sClient.Get(ctx, key, got)).Should(Succeed())
		return got.Status.Phase
	}, 15*time.Second, time.Second).Should(Equal(want))
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
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseAvailable, &gotCfg)
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
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseAvailable, &gotCfg)

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
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseError, &gotCfg)
		Expect(gotCfg.Status.GetCondition("Synced").Message).Should(ContainSubstring("mutually exclusive"))
	})

	It("does nothing (no-op) when Config has DeletionTimestamp set", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "tmpl-del-cfg", Namespace: "default"},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: testCUETemplate},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)).Should(Succeed())
			return ct.Status.Phase
		}, 15*time.Second, time.Second).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))

		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "cfg-deleting",
				Namespace:  "default",
				Finalizers: []string{"test.oam.dev/protect"},
			},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "tmpl-del-cfg", Namespace: "default"},
				Properties:  rawExtension(map[string]string{"username": "alice"}),
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())
		var gotCfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseAvailable, &gotCfg)

		Expect(k8sClient.Delete(ctx, cfg)).Should(Succeed())
		Eventually(func() bool {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &gotCfg)).Should(Succeed())
			return gotCfg.DeletionTimestamp != nil
		}, 15*time.Second, time.Second).Should(BeTrue())
		Expect(gotCfg.Status.Phase).Should(Equal(configv1alpha1.ConfigPhaseAvailable))

		patch := client.MergeFrom(gotCfg.DeepCopy())
		gotCfg.Finalizers = nil
		Expect(k8sClient.Patch(ctx, &gotCfg, patch)).Should(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &gotCfg))
		}, 15*time.Second, time.Second).Should(BeTrue())
	})

	It("transitions through waiting then Available when ConfigTemplate is initially not ready", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "tmpl-waiting", Namespace: "default"},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: `this is not valid cue {{{`},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)).Should(Succeed())
			return ct.Status.Phase
		}, 15*time.Second, time.Second).Should(Equal(configv1alpha1.ConfigTemplatePhaseError))

		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-waiting", Namespace: "default"},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "tmpl-waiting", Namespace: "default"},
				Properties:  rawExtension(map[string]string{"username": "eve"}),
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		var gotCfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseError, &gotCfg)
		Expect(gotCfg.Status.GetCondition("Synced").Message).Should(ContainSubstring("not Available yet"))

		patch := client.MergeFrom(ct.DeepCopy())
		ct.Spec.Template = testCUETemplate
		Expect(k8sClient.Patch(ctx, ct, patch)).Should(Succeed())

		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)).Should(Succeed())
			return ct.Status.Phase
		}, 15*time.Second, time.Second).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))

		Eventually(func() configv1alpha1.ConfigPhase {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &gotCfg)).Should(Succeed())
			return gotCfg.Status.Phase
		}, 15*time.Second, time.Second).Should(Equal(configv1alpha1.ConfigPhaseAvailable))
	})

	It("creates a minimal Secret when no TemplateRef is set", func() {
		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-notmpl", Namespace: "default"},
			Spec: configv1alpha1.ConfigSpec{
				Properties: rawExtension(map[string]string{"key": "value"}),
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		var gotCfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseAvailable, &gotCfg)

		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "cfg-notmpl"}, &secret)).Should(Succeed())
		Expect(secret.Labels["config.oam.dev/catalog"]).Should(Equal("velacore-config"))
		Expect(secret.Data["input-properties"]).ShouldNot(BeEmpty())
	})

	It("sets Phase=Error when the referenced ConfigTemplate and ConfigMap do not exist", func() {
		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-notfound", Namespace: "default"},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "nonexistent-tmpl", Namespace: "default"},
				Properties:  rawExtension(map[string]string{"username": "alice"}),
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		var gotCfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseError, &gotCfg)
	})

	It("reads properties from a Secret using an explicit key in spec.propertiesFrom.secretRef", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "tmpl-custkey", Namespace: "default"},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: testCUETemplate},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)).Should(Succeed())
			return ct.Status.Phase
		}, 15*time.Second, time.Second).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))

		propsSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds-custkey", Namespace: "default"},
			Data:       map[string][]byte{"my-key": []byte(`{"username":"frank"}`)},
		}
		Expect(k8sClient.Create(ctx, propsSecret)).Should(Succeed())

		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-custkey", Namespace: "default"},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "tmpl-custkey", Namespace: "default"},
				PropertiesFrom: &configv1alpha1.PropertiesReference{
					SecretRef: configv1alpha1.SecretKeySelector{Name: "creds-custkey", Key: "my-key"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		var gotCfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseAvailable, &gotCfg)

		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "cfg-custkey"}, &secret)).Should(Succeed())
		Expect(string(secret.Data["username"])).Should(Equal("frank"))
	})

	It("also materializes template.outputs objects alongside the main Secret", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "tmpl-outputs", Namespace: "default"},
			Spec: configv1alpha1.ConfigTemplateSpec{Template: `
context: {
	name:      string
	namespace: string
}
template: {
	parameter: value: string
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {name: context.name, namespace: context.namespace}
		stringData: value: parameter.value
	}
	outputs: {
		"companion-cm": {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: name: "\(context.name)-extra"
			data: value: parameter.value
		}
		"companion-cm-2": {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: name: "\(context.name)-extra-2"
			data: value: parameter.value
		}
		"companion-secret": {
			apiVersion: "v1"
			kind:       "Secret"
			metadata: name: "\(context.name)-extra-secret"
			stringData: value: parameter.value
		}
	}
}
`},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)).Should(Succeed())
			return ct.Status.Phase
		}, 15*time.Second, time.Second).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))

		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-outputs", Namespace: "default"},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "tmpl-outputs", Namespace: "default"},
				Properties:  rawExtension(map[string]string{"value": "hello"}),
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		var gotCfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseAvailable, &gotCfg)

		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "cfg-outputs"}, &secret)).Should(Succeed())
		Expect(string(secret.Data["value"])).Should(Equal("hello"))

		var cm corev1.ConfigMap
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "cfg-outputs-extra"}, &cm)
		}, 15*time.Second, time.Second).Should(Succeed())
		Expect(cm.Data["value"]).Should(Equal("hello"))
		// GC on Config delete relies on this; envtest has no controller-manager to verify it directly.
		Expect(cm.OwnerReferences).Should(HaveLen(1))
		Expect(cm.OwnerReferences[0].Name).Should(Equal("cfg-outputs"))
		Expect(*cm.OwnerReferences[0].Controller).Should(BeTrue())

		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "cfg-outputs-extra-2"}, &corev1.ConfigMap{})).Should(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "cfg-outputs-extra-secret"}, &corev1.Secret{})).Should(Succeed())

		// the Secret must record all three companion objects (not just the first), matching
		// the legacy Factory.ParseConfig convention so addons/tooling reading objects-reference
		// off the Secret still discover every output, not just one.
		var refs []corev1.ObjectReference
		Expect(json.Unmarshal(secret.Data["objects-reference"], &refs)).Should(Succeed())
		Expect(refs).Should(ConsistOf(
			corev1.ObjectReference{Kind: "ConfigMap", APIVersion: "v1", Namespace: "default", Name: "cfg-outputs-extra"},
			corev1.ObjectReference{Kind: "ConfigMap", APIVersion: "v1", Namespace: "default", Name: "cfg-outputs-extra-2"},
			corev1.ObjectReference{Kind: "Secret", APIVersion: "v1", Namespace: "default", Name: "cfg-outputs-extra-secret"},
		))
	})

	It("denies materializing when a sensitive ConfigTemplate is used with inline spec.properties", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "ct-sensitive", Namespace: "default"},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: testCUETemplate, Sensitive: true},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)).Should(Succeed())
			return ct.Status.Phase
		}, 15*time.Second, time.Second).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))

		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "leaky", Namespace: "default"},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "ct-sensitive", Namespace: "default"},
				Properties:  rawExtension(map[string]string{"username": "PLACEHOLDER-TOKEN"}),
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		var gotCfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseError, &gotCfg)
		Expect(gotCfg.Status.GetCondition("Synced").Message).Should(ContainSubstring("is sensitive"))

		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "leaky"}, &corev1.Secret{})).ShouldNot(Succeed())
	})

	It("materializes a sensitive ConfigTemplate via spec.propertiesFrom with no credential inlined", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "ct-sensitive-ok", Namespace: "default"},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: testCUETemplate, Sensitive: true},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)).Should(Succeed())
			return ct.Status.Phase
		}, 15*time.Second, time.Second).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))

		propsSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "leaky-ok-properties", Namespace: "default"},
			Data:       map[string][]byte{"properties": []byte(`{"username":"PLACEHOLDER-TOKEN"}`)},
		}
		Expect(k8sClient.Create(ctx, propsSecret)).Should(Succeed())

		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "leaky-ok", Namespace: "default"},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef:    &configv1alpha1.ConfigTemplateReference{Name: "ct-sensitive-ok", Namespace: "default"},
				PropertiesFrom: &configv1alpha1.PropertiesReference{SecretRef: configv1alpha1.SecretKeySelector{Name: "leaky-ok-properties"}},
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		var gotCfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseAvailable, &gotCfg)
		Expect(gotCfg.Spec.Properties).Should(BeNil())

		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "leaky-ok"}, &secret)).Should(Succeed())
		Expect(string(secret.Data["username"])).Should(Equal("PLACEHOLDER-TOKEN"))
	})

	It("fails to Error, not Available, when a Config with inline properties outlives admission and its template later becomes a sensitive Available template", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "ct-race", Namespace: "default"},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: `this is not valid cue {{{`},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)).Should(Succeed())
			return ct.Status.Phase
		}, 15*time.Second, time.Second).Should(Equal(configv1alpha1.ConfigTemplatePhaseError))

		By("Config with inline properties is admitted while its template isn't Available yet")
		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "leaky-race", Namespace: "default"},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "ct-race", Namespace: "default"},
				Properties:  rawExtension(map[string]string{"username": "PLACEHOLDER-TOKEN"}),
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		var gotCfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseError, &gotCfg)

		By("the template later becomes Available and sensitive")
		patch := client.MergeFrom(ct.DeepCopy())
		ct.Spec.Template = testCUETemplate
		ct.Spec.Sensitive = true
		Expect(k8sClient.Patch(ctx, ct, patch)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)).Should(Succeed())
			return ct.Status.Phase
		}, 15*time.Second, time.Second).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))

		By("the Config must stay in Error, never Available, once the template resolves as sensitive")
		Consistently(func() configv1alpha1.ConfigPhase {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &gotCfg)).Should(Succeed())
			return gotCfg.Status.Phase
		}, 5*time.Second, time.Second).Should(Equal(configv1alpha1.ConfigPhaseError))
		Expect(gotCfg.Status.GetCondition("Synced").Message).Should(ContainSubstring("is sensitive"))
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
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseAvailable, &gotCfg)

		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "cfg-legacy"}, &secret)).Should(Succeed())
		Expect(string(secret.Data["username"])).Should(Equal("carol"))
	})
})
