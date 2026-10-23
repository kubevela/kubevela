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
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	legacyconfig "github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/velaconfig/reader"
	workflowconfig "github.com/oam-dev/kubevela/pkg/workflow/providers/config"
)

func TestConfigSensitive(t *testing.T) {
	fromSecret := &configv1alpha1.PropertiesReference{SecretRef: configv1alpha1.SecretKeySelector{Name: "props"}}
	cases := map[string]struct {
		propertiesFrom *configv1alpha1.PropertiesReference
		tmpl           *legacyconfig.ResolvedTemplate
		want           bool
	}{
		"inline on a plain template":         {tmpl: &legacyconfig.ResolvedTemplate{}},
		"inline on a sensitive template":     {tmpl: &legacyconfig.ResolvedTemplate{Sensitive: true}, want: true},
		"propertiesFrom on a plain template": {propertiesFrom: fromSecret, tmpl: &legacyconfig.ResolvedTemplate{}, want: true},
		"propertiesFrom on a sensitive one":  {propertiesFrom: fromSecret, tmpl: &legacyconfig.ResolvedTemplate{Sensitive: true}, want: true},
		"template-less inline":               {},
		"template-less propertiesFrom":       {propertiesFrom: fromSecret, want: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := &configv1alpha1.Config{Spec: configv1alpha1.ConfigSpec{PropertiesFrom: tc.propertiesFrom}}
			assert.Equal(t, tc.want, configSensitive(cfg, tc.tmpl))
		})
	}
}

var _ = Describe("Config controller, the sensitive rules", func() {
	ctx := context.Background()

	templateAvailable := func(name string, sensitive bool) *configv1alpha1.ConfigTemplate {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: testCUETemplate, Sensitive: sensitive},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)).Should(Succeed())
			return ct.Status.Phase
		}, 15*time.Second, time.Second).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))
		return ct
	}
	propertiesSecret := func(name, username string) *corev1.Secret {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Data:       map[string][]byte{"properties": []byte(`{"username":"` + username + `"}`)},
		}
		Expect(k8sClient.Create(ctx, s)).Should(Succeed())
		return s
	}
	fromSecret := func(name string) *configv1alpha1.PropertiesReference {
		return &configv1alpha1.PropertiesReference{SecretRef: configv1alpha1.SecretKeySelector{Name: name}}
	}
	templateRef := func(name string) *configv1alpha1.ConfigTemplateReference {
		return &configv1alpha1.ConfigTemplateReference{Name: name, Namespace: "default"}
	}
	// sensitiveMark is the Secret's annotation, "<none>" when it has none
	sensitiveMark := func(name string) func() string {
		return func() string {
			var secret corev1.Secret
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, &secret)).Should(Succeed())
			if v, ok := secret.Annotations[apitypes.AnnotationConfigSensitive]; ok {
				return v
			}
			return "<none>"
		}
	}

	It("marks the Secret of a propertiesFrom Config sensitive on a template that is not", func() {
		templateAvailable("sens-plain", false)
		propertiesSecret("sens-from-properties", "dave")
		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "sens-from", Namespace: "default"},
			Spec:       configv1alpha1.ConfigSpec{TemplateRef: templateRef("sens-plain"), PropertiesFrom: fromSecret("sens-from-properties")},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())
		var gotCfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseAvailable, &gotCfg)
		Expect(sensitiveMark("sens-from")()).Should(Equal("true"))

		By("an inline Config on the same template stays readable")
		inline := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "sens-inline", Namespace: "default"},
			Spec:       configv1alpha1.ConfigSpec{TemplateRef: templateRef("sens-plain"), Properties: rawExtension(map[string]string{"username": "erin"})},
		}
		Expect(k8sClient.Create(ctx, inline)).Should(Succeed())
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(inline), configv1alpha1.ConfigPhaseAvailable, &gotCfg)
		Expect(sensitiveMark("sens-inline")()).Should(Equal("false"))
	})

	It("marks a template-less propertiesFrom Config's Secret sensitive, and leaves a template-less inline one unmarked", func() {
		propertiesSecret("sens-notmpl-from-properties", "frank")
		from := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "sens-notmpl-from", Namespace: "default"},
			Spec:       configv1alpha1.ConfigSpec{PropertiesFrom: fromSecret("sens-notmpl-from-properties")},
		}
		inline := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "sens-notmpl-inline", Namespace: "default"},
			Spec:       configv1alpha1.ConfigSpec{Properties: rawExtension(map[string]string{"key": "value"})},
		}
		Expect(k8sClient.Create(ctx, from)).Should(Succeed())
		Expect(k8sClient.Create(ctx, inline)).Should(Succeed())
		var gotCfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(from), configv1alpha1.ConfigPhaseAvailable, &gotCfg)
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(inline), configv1alpha1.ConfigPhaseAvailable, &gotCfg)
		Expect(sensitiveMark("sens-notmpl-from")()).Should(Equal("true"))
		Expect(sensitiveMark("sens-notmpl-inline")()).Should(Equal("<none>"))
	})

	It("turning sensitive on: the inline Config goes to Error with its Secret marked at once, the propertiesFrom one stays Available", func() {
		ct := templateAvailable("sens-flip", false)
		propertiesSecret("sens-flip-from-properties", "grace")
		inline := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "sens-flip-inline", Namespace: "default"},
			Spec:       configv1alpha1.ConfigSpec{TemplateRef: templateRef("sens-flip"), Properties: rawExtension(map[string]string{"username": "heidi"})},
		}
		from := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "sens-flip-from", Namespace: "default"},
			Spec:       configv1alpha1.ConfigSpec{TemplateRef: templateRef("sens-flip"), PropertiesFrom: fromSecret("sens-flip-from-properties")},
		}
		Expect(k8sClient.Create(ctx, inline)).Should(Succeed())
		Expect(k8sClient.Create(ctx, from)).Should(Succeed())
		var gotCfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(inline), configv1alpha1.ConfigPhaseAvailable, &gotCfg)
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(from), configv1alpha1.ConfigPhaseAvailable, &gotCfg)
		Expect(sensitiveMark("sens-flip-inline")()).Should(Equal("false"))

		patch := client.MergeFrom(ct.DeepCopy())
		ct.Spec.Sensitive = true
		Expect(k8sClient.Patch(ctx, ct, patch)).Should(Succeed())

		eventuallyConfigError(ctx, client.ObjectKeyFromObject(inline), "is sensitive: spec.properties must not be set, use spec.propertiesFrom instead", &gotCfg)
		Eventually(sensitiveMark("sens-flip-inline"), 15*time.Second, time.Second).Should(Equal("true"))
		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "sens-flip-inline"}, &secret)).Should(Succeed())
		Expect(string(secret.Data["username"])).Should(Equal("heidi"), "the mark only: the data stays for the Secret's consumers")

		Consistently(func() configv1alpha1.ConfigPhase {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(from), &gotCfg)).Should(Succeed())
			return gotCfg.Status.Phase
		}, 3*time.Second, time.Second).Should(Equal(configv1alpha1.ConfigPhaseAvailable))
		Expect(sensitiveMark("sens-flip-from")()).Should(Equal("true"))
	})

	It("marks the Secret while the Config is in Error for another reason, as one written by an earlier release", func() {
		templateAvailable("sens-upgrade", false)
		props := propertiesSecret("sens-upgrade-properties", "ivan")
		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "sens-upgrade", Namespace: "default"},
			Spec:       configv1alpha1.ConfigSpec{TemplateRef: templateRef("sens-upgrade"), PropertiesFrom: fromSecret("sens-upgrade-properties")},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())
		var gotCfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseAvailable, &gotCfg)

		Expect(k8sClient.Delete(ctx, props)).Should(Succeed())
		eventuallyConfigError(ctx, client.ObjectKeyFromObject(cfg), "failed to load spec.propertiesFrom secret", &gotCfg)

		By("the Secret as an earlier release wrote it")
		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "sens-upgrade"}, &secret)).Should(Succeed())
		patch := client.MergeFrom(secret.DeepCopy())
		secret.Annotations[apitypes.AnnotationConfigSensitive] = "false"
		Expect(k8sClient.Patch(ctx, &secret, patch)).Should(Succeed())

		Eventually(sensitiveMark("sens-upgrade"), 15*time.Second, time.Second).Should(Equal("true"))
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &gotCfg)).Should(Succeed())
		Expect(gotCfg.Status.Phase).Should(Equal(configv1alpha1.ConfigPhaseError))
	})

	It("marks a propertiesFrom Config's Secret while its template is gone, as one written by an earlier release", func() {
		ct := templateAvailable("sens-gone", false)
		propertiesSecret("sens-gone-properties", "kim")
		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "sens-gone", Namespace: "default"},
			Spec:       configv1alpha1.ConfigSpec{TemplateRef: templateRef("sens-gone"), PropertiesFrom: fromSecret("sens-gone-properties")},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())
		// left in Error it would be rendered again every errorRetryInterval for the rest of the run
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cfg))).Should(Succeed()) })
		var gotCfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseAvailable, &gotCfg)

		Expect(k8sClient.Delete(ctx, ct)).Should(Succeed())
		eventuallyConfigError(ctx, client.ObjectKeyFromObject(cfg), "the template does not exist", &gotCfg)

		By("the Secret as an earlier release wrote it")
		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "sens-gone"}, &secret)).Should(Succeed())
		patch := client.MergeFrom(secret.DeepCopy())
		secret.Annotations[apitypes.AnnotationConfigSensitive] = "false"
		Expect(k8sClient.Patch(ctx, &secret, patch)).Should(Succeed())

		Eventually(sensitiveMark("sens-gone"), 15*time.Second, time.Second).Should(Equal("true"))
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &gotCfg)).Should(Succeed())
		Expect(gotCfg.Status.Phase).Should(Equal(configv1alpha1.ConfigPhaseError))
	})

	It("leaves a Secret the Config does not own as it is", func() {
		templateAvailable("sens-foreign", false)
		propertiesSecret("sens-foreign-properties", "lee")
		foreign := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "sens-foreign",
				Namespace:   "default",
				Annotations: map[string]string{apitypes.AnnotationConfigSensitive: "false"},
			},
			Data: map[string][]byte{"username": []byte("someone-else")},
		}
		Expect(k8sClient.Create(ctx, foreign)).Should(Succeed())
		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "sens-foreign", Namespace: "default"},
			Spec:       configv1alpha1.ConfigSpec{TemplateRef: templateRef("sens-foreign"), PropertiesFrom: fromSecret("sens-foreign-properties")},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cfg))).Should(Succeed()) })
		var gotCfg configv1alpha1.Config
		eventuallyConfigError(ctx, client.ObjectKeyFromObject(cfg), "already exists and is not owned by this Config", &gotCfg)

		Consistently(sensitiveMark("sens-foreign"), 3*time.Second, time.Second).Should(Equal("false"))
		var after corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(foreign), &after)).Should(Succeed())
		Expect(after.OwnerReferences).Should(BeEmpty())
		Expect(after.Data).Should(Equal(foreign.Data))
	})

	It("keeps a Secret marked sensitive when the legacy template's annotation is turned off again", func() {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "config-template-sens-legacy",
				Namespace:   "default",
				Labels:      map[string]string{apitypes.LabelConfigCatalog: apitypes.VelaCoreConfig},
				Annotations: map[string]string{apitypes.AnnotationConfigSensitive: "false"},
			},
			Data: map[string]string{"template": testCUETemplate},
		}
		Expect(k8sClient.Create(ctx, cm)).Should(Succeed())
		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "sens-legacy", Namespace: "default"},
			Spec:       configv1alpha1.ConfigSpec{TemplateRef: templateRef("sens-legacy"), Properties: rawExtension(map[string]string{"username": "judy"})},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())
		var gotCfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseAvailable, &gotCfg)
		Expect(sensitiveMark("sens-legacy")()).Should(Equal("false"))

		setLegacySensitive := func(v string) {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), cm)).Should(Succeed())
			patch := client.MergeFrom(cm.DeepCopy())
			cm.Annotations[apitypes.AnnotationConfigSensitive] = v
			Expect(k8sClient.Patch(ctx, cm, patch)).Should(Succeed())
		}

		By("the annotation turned on: Error, the Secret marked")
		setLegacySensitive("true")
		eventuallyConfigError(ctx, client.ObjectKeyFromObject(cfg), "is sensitive", &gotCfg)
		Eventually(sensitiveMark("sens-legacy"), 15*time.Second, time.Second).Should(Equal("true"))

		By("the annotation turned off again: the Config renders, the Secret stays sensitive")
		setLegacySensitive("false")
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseAvailable, &gotCfg)
		Consistently(sensitiveMark("sens-legacy"), 3*time.Second, time.Second).Should(Equal("true"))
	})

	It("keeps a Secret marked sensitive when its Config moves from propertiesFrom back to inline properties", func() {
		templateAvailable("sens-back", false)
		propertiesSecret("sens-back-properties", "mallory")
		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "sens-back", Namespace: "default"},
			Spec:       configv1alpha1.ConfigSpec{TemplateRef: templateRef("sens-back"), PropertiesFrom: fromSecret("sens-back-properties")},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())
		var gotCfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseAvailable, &gotCfg)
		Expect(sensitiveMark("sens-back")()).Should(Equal("true"))

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), cfg)).Should(Succeed())
		patch := client.MergeFrom(cfg.DeepCopy())
		cfg.Spec.PropertiesFrom = nil
		cfg.Spec.Properties = rawExtension(map[string]string{"username": "niaj"})
		Expect(k8sClient.Patch(ctx, cfg, patch)).Should(Succeed())

		Eventually(func() string {
			var secret corev1.Secret
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "sens-back"}, &secret)).Should(Succeed())
			return string(secret.Data["username"])
		}, 15*time.Second, time.Second).Should(Equal("niaj"))
		Expect(sensitiveMark("sens-back")()).Should(Equal("true"))
	})

	It("hides a propertiesFrom Config on a template that is not sensitive from every reader", func() {
		templateAvailable("sens-readers", false)
		propertiesSecret("sens-readers-from-properties", "olivia")
		from := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "sens-readers-from", Namespace: "default"},
			Spec:       configv1alpha1.ConfigSpec{TemplateRef: templateRef("sens-readers"), PropertiesFrom: fromSecret("sens-readers-from-properties")},
		}
		inline := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "sens-readers-inline", Namespace: "default"},
			Spec:       configv1alpha1.ConfigSpec{TemplateRef: templateRef("sens-readers"), Properties: rawExtension(map[string]string{"username": "peggy"})},
		}
		Expect(k8sClient.Create(ctx, from)).Should(Succeed())
		Expect(k8sClient.Create(ctx, inline)).Should(Succeed())
		var gotCfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(from), configv1alpha1.ConfigPhaseAvailable, &gotCfg)
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(inline), configv1alpha1.ConfigPhaseAvailable, &gotCfg)
		Expect(sensitiveMark("sens-readers-from")()).Should(Equal("true"))

		factory := legacyconfig.NewConfigFactory(k8sClient)
		deps := workflowconfig.Deps{Factory: factory, Client: k8sClient}

		By("vela config and the factory's readers")
		_, err := factory.ReadConfig(ctx, "default", "sens-readers-from")
		Expect(err).Should(MatchError(legacyconfig.ErrSensitiveConfig))
		_, err = factory.GetConfig(ctx, "default", "sens-readers-from", false)
		Expect(err).Should(MatchError(legacyconfig.ErrSensitiveConfig))
		props, err := factory.ReadConfig(ctx, "default", "sens-readers-inline")
		Expect(err).Should(Succeed())
		Expect(props).Should(HaveKeyWithValue("username", "peggy"))

		By("the read-config step")
		_, err = workflowconfig.Read(ctx, deps, legacyconfig.NamespacedName{Namespace: "default", Name: "sens-readers-from"})
		Expect(err).Should(MatchError(legacyconfig.ErrSensitiveConfig))
		props, err = workflowconfig.Read(ctx, deps, legacyconfig.NamespacedName{Namespace: "default", Name: "sens-readers-inline"})
		Expect(err).Should(Succeed())
		Expect(props).Should(HaveKeyWithValue("username", "peggy"))

		By("the list-config step: both listed, only the inline one with its properties")
		listed, err := workflowconfig.List(ctx, deps, workflowconfig.ListVars{Namespace: "default", Template: "sens-readers"})
		Expect(err).Should(Succeed())
		byName := map[string]map[string]any{}
		for _, c := range listed {
			byName[c["name"].(string)] = c
		}
		Expect(byName).Should(HaveLen(2))
		Expect(byName["sens-readers-from"]["config"]).Should(BeNil())
		Expect(byName["sens-readers-inline"]["config"]).Should(HaveKeyWithValue("username", "peggy"))

		By("the config provider of CueX")
		_, err = reader.Config{Client: k8sClient}.ReadConfig(ctx, "default", "sens-readers-from")
		Expect(err).Should(MatchError(legacyconfig.ErrSensitiveConfig))
	})
})
