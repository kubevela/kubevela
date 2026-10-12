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
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
)

// legacyTemplateCUE is a template with a same-namespace output object and one in
// another namespace, the shape 1.11.0 allowed.
func legacyTemplateCUE(name, scope string, sensitive bool, otherNS string) string {
	outputs := ""
	if otherNS != "" {
		outputs = fmt.Sprintf(`
	outputs: {
		local: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {name: context.name + "-local", namespace: context.namespace}
			data: value: parameter.value
		}
		remote: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {name: context.name + "-remote", namespace: %q}
			data: value: parameter.value
		}
	}`, otherNS)
	}
	return fmt.Sprintf(`
metadata: {
	name:        %q
	alias:       "alias-%s"
	description: "legacy template"
	scope:       %q
	sensitive:   %t
}
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
	}%s
}
`, name, name, scope, sensitive, outputs)
}

var _ = Describe("Migrating legacy templates and configs", func() {
	ctx := context.Background()
	var fac Factory
	var migrator *Migrator

	ensureNamespace := func(name string) {
		err := k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})
		Expect(client.IgnoreAlreadyExists(err)).Should(Succeed())
	}

	writeLegacyTemplate := func(ns, name, cue string) *corev1.ConfigMap {
		t, err := fac.ParseTemplate(ctx, name, []byte(cue))
		Expect(err).Should(Succeed())
		Expect(fac.CreateOrUpdateConfigTemplate(ctx, ns, t)).Should(Succeed())
		var cm corev1.ConfigMap
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: TemplateConfigMapNamePrefix + name}, &cm)).Should(Succeed())
		return &cm
	}

	writeLegacyConfig := func(tplNS, tplName, ns, name string) *corev1.Secret {
		item, err := fac.ParseConfig(ctx, NamespacedName{Namespace: tplNS, Name: tplName}, Metadata{
			NamespacedName: NamespacedName{Namespace: ns, Name: name},
			Alias:          "alias-" + name,
			Description:    "legacy config",
			Properties:     map[string]interface{}{"value": "v-" + name},
		})
		Expect(err).Should(Succeed())
		Expect(fac.CreateOrUpdateConfig(ctx, item, ns)).Should(Succeed())
		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &secret)).Should(Succeed())
		return &secret
	}

	getSecret := func(ns, name string) *corev1.Secret {
		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &secret)).Should(Succeed())
		return &secret
	}
	getConfig := func(ns, name string) *configv1alpha1.Config {
		var cfg configv1alpha1.Config
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &cfg)).Should(Succeed())
		return &cfg
	}
	getTemplateCR := func(ns, name string) *configv1alpha1.ConfigTemplate {
		var ct configv1alpha1.ConfigTemplate
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &ct)).Should(Succeed())
		return &ct
	}
	controllerOf := func(obj metav1.Object) *metav1.OwnerReference {
		return metav1.GetControllerOf(obj)
	}
	resultFor := func(report MigrationReport, kind, ns, name string) MigrationResult {
		for _, res := range report.Results {
			if res.Kind == kind && res.Namespace == ns && res.Name == name {
				return res
			}
		}
		Fail(fmt.Sprintf("no result for %s %s/%s in %+v", kind, ns, name, report.Results))
		return MigrationResult{}
	}

	BeforeEach(func() {
		fac = NewConfigFactory(k8sClient)
		migrator = NewMigrator(k8sClient)
		ensureNamespace(types.DefaultKubeVelaNS)
	})

	// Other suites in this package list templates and configs across all namespaces
	// and count them, and envtest has no garbage collector, so every object written
	// here is removed again. The scenario namespaces are this file's own; in
	// vela-system only this file's names are touched.
	AfterEach(func() {
		for _, ns := range []string{"mig-one", "mig-one-other", "mig-team-a", "mig-team-b", "mig-cfg", "mig-cfg-all", "mig-all"} {
			for _, obj := range []client.Object{&configv1alpha1.Config{}, &configv1alpha1.ConfigTemplate{}, &v1beta1.Application{}, &corev1.Secret{}, &corev1.ConfigMap{}} {
				Expect(client.IgnoreNotFound(k8sClient.DeleteAllOf(ctx, obj, client.InNamespace(ns)))).Should(Succeed())
			}
		}
		for _, name := range []string{"shared", "shared2", "shared-all", "denied"} {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: types.DefaultKubeVelaNS, Name: TemplateConfigMapNamePrefix + name}}))).Should(Succeed())
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &configv1alpha1.ConfigTemplate{ObjectMeta: metav1.ObjectMeta{Namespace: types.DefaultKubeVelaNS, Name: name}}))).Should(Succeed())
		}
		for _, name := range []string{"sys-denied"} {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: types.DefaultKubeVelaNS, Name: name}}))).Should(Succeed())
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Namespace: types.DefaultKubeVelaNS, Name: name}}))).Should(Succeed())
		}
	})

	Context("one template with its configs", func() {
		const ns = "mig-one"
		const other = "mig-one-other"
		BeforeEach(func() {
			ensureNamespace(ns)
			ensureNamespace(other)
		})

		It("adopts the template, its configs, same-namespace outputs and the distribution", func() {
			cm := writeLegacyTemplate(ns, "plain", legacyTemplateCUE("plain", "project", false, other))
			secret := writeLegacyConfig(ns, "plain", ns, "cfg-a")
			Expect(fac.CreateOrUpdateDistribution(ctx, ns, DefaultDistributionName("cfg-a"), &CreateDistributionSpec{
				Targets: []*ClusterTarget{{ClusterName: types.ClusterLocalName, Namespace: other}},
				Configs: []*NamespacedName{{Namespace: ns, Name: "cfg-a"}},
			})).Should(Succeed())
			var localOut, remoteOut corev1.ConfigMap
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "cfg-a-local"}, &localOut)).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: other, Name: "cfg-a-remote"}, &remoteOut)).Should(Succeed())

			recorder := record.NewFakeRecorder(20)
			report, err := migrator.MigrateTemplate(ctx, ns, "plain", MigrationOptions{Recorder: recorder})
			Expect(err).Should(Succeed())
			Expect(report.Results).Should(HaveLen(2))
			Expect(report.Results[0]).Should(MatchFields(IgnoreExtras, Fields{
				"Kind": Equal("ConfigMap"), "Name": Equal("plain"), "Outcome": Equal(MigrationAdopted), "Reason": Equal("ConfigTemplate plain created"),
			}))
			Expect(report.Results[1]).Should(MatchFields(IgnoreExtras, Fields{
				"Kind": Equal("Secret"), "Name": Equal("cfg-a"), "Outcome": Equal(MigrationAdopted),
				"Reason": Equal("Config cfg-a created; 1 output object(s) outside mig-one left unowned"),
			}))

			By("the template CR")
			ct := getTemplateCR(ns, "plain")
			Expect(ct.Labels).Should(HaveKeyWithValue(types.LabelConfigCreatedBy, types.ConfigCreatedByMigration))
			Expect(ct.Spec.Scope).Should(Equal(configv1alpha1.ConfigTemplateScopeNamespace))
			Expect(ct.Spec.Alias).Should(Equal("alias-plain"))
			// ParseTemplate has never copied metadata.description (same in v1.11.0), so legacy
			// ConfigMaps carry an empty one; migration carries over whatever is there.
			Expect(ct.Spec.Description).Should(Equal(cm.Annotations[types.AnnotationConfigDescription]))
			Expect(ct.Spec.Template).Should(Equal(cm.Data[SaveTemplateKey]))
			var cmAfter corev1.ConfigMap
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), &cmAfter)).Should(Succeed())
			Expect(cmAfter.Annotations).Should(HaveKeyWithValue(types.AnnotationConfigMigratedTo, string(ct.UID)))

			By("the Config CR")
			cfg := getConfig(ns, "cfg-a")
			Expect(cfg.Labels).Should(HaveKeyWithValue(types.LabelConfigCreatedBy, types.ConfigCreatedByMigration))
			Expect(cfg.Spec.TemplateRef).Should(Equal(&configv1alpha1.ConfigTemplateReference{Name: "plain", Namespace: ns}))
			Expect(cfg.Spec.Alias).Should(Equal("alias-cfg-a"))
			Expect(cfg.Spec.Description).Should(Equal("legacy config"))
			Expect(cfg.Spec.PropertiesFrom).Should(BeNil())
			var props map[string]interface{}
			Expect(json.Unmarshal(cfg.Spec.Properties.Raw, &props)).Should(Succeed())
			Expect(props).Should(Equal(map[string]interface{}{"value": "v-cfg-a"}))

			By("the Secret: same UID and data, owned, annotated")
			after := getSecret(ns, "cfg-a")
			Expect(after.UID).Should(Equal(secret.UID))
			Expect(after.Data).Should(Equal(secret.Data))
			Expect(after.Type).Should(Equal(secret.Type))
			Expect(controllerOf(after)).ShouldNot(BeNil())
			Expect(controllerOf(after).UID).Should(Equal(cfg.UID))
			Expect(after.Annotations).Should(HaveKeyWithValue(types.AnnotationConfigMigratedTo, string(cfg.UID)))

			By("the same-namespace output is owned, the remote one untouched")
			var localAfter, remoteAfter corev1.ConfigMap
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&localOut), &localAfter)).Should(Succeed())
			Expect(localAfter.UID).Should(Equal(localOut.UID))
			Expect(controllerOf(&localAfter).UID).Should(Equal(cfg.UID))
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&remoteOut), &remoteAfter)).Should(Succeed())
			Expect(remoteAfter.ResourceVersion).Should(Equal(remoteOut.ResourceVersion))
			Expect(remoteAfter.OwnerReferences).Should(BeEmpty())

			By("the distribution Application is owned")
			var app v1beta1.Application
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: DefaultDistributionName("cfg-a")}, &app)).Should(Succeed())
			Expect(controllerOf(&app).UID).Should(Equal(cfg.UID))

			By("Events")
			var events []string
			for len(recorder.Events) > 0 {
				events = append(events, <-recorder.Events)
			}
			Expect(events).Should(ContainElement(ContainSubstring("Normal MigrationAdopted ConfigTemplate plain created")))
			Expect(events).Should(ContainElement(ContainSubstring("Normal MigrationAdopted Config cfg-a created")))
			Expect(events).Should(ContainElement(And(
				ContainSubstring("Warning MigrationOutputsUnowned"),
				ContainSubstring("ConfigMap mig-one-other/cfg-a-remote"))))

			By("a second run skips everything and writes nothing")
			rv := after.ResourceVersion
			report, err = migrator.MigrateTemplate(ctx, ns, "plain", MigrationOptions{Recorder: recorder})
			Expect(err).Should(Succeed())
			Expect(report.Count(MigrationSkipped)).Should(Equal(2))
			Expect(report.Count(MigrationAdopted)).Should(Equal(0))
			for _, res := range report.Results {
				Expect(res.Reason).Should(Equal("already migrated"))
			}
			Expect(getSecret(ns, "cfg-a").ResourceVersion).Should(Equal(rv))
			Expect(recorder.Events).Should(BeEmpty())
		})

		It("maps every legacy scope word", func() {
			for word, want := range map[string]configv1alpha1.ConfigTemplateScope{
				"system": configv1alpha1.ConfigTemplateScopeSystem,
				"global": configv1alpha1.ConfigTemplateScopeNamespace,
				"env":    configv1alpha1.ConfigTemplateScopeNamespace,
			} {
				name := "scope-" + word
				writeLegacyTemplate(ns, name, legacyTemplateCUE(name, word, false, ""))
				report, err := migrator.MigrateTemplate(ctx, ns, name, MigrationOptions{})
				Expect(err).Should(Succeed())
				Expect(report.Count(MigrationAdopted)).Should(Equal(1))
				Expect(getTemplateCR(ns, name).Spec.Scope).Should(Equal(want), word)
			}
		})

		It("puts a sensitive config's input in a companion Secret", func() {
			writeLegacyTemplate(ns, "secret-tpl", legacyTemplateCUE("secret-tpl", "namespace", true, ""))
			before := writeLegacyConfig(ns, "secret-tpl", ns, "cfg-s")
			report, err := migrator.MigrateTemplate(ctx, ns, "secret-tpl", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Count(MigrationAdopted)).Should(Equal(2))

			Expect(getTemplateCR(ns, "secret-tpl").Spec.Sensitive).Should(BeTrue())
			cfg := getConfig(ns, "cfg-s")
			Expect(cfg.Spec.Properties).Should(BeNil())
			Expect(cfg.Spec.PropertiesFrom).Should(Equal(&configv1alpha1.PropertiesReference{SecretRef: configv1alpha1.SecretKeySelector{Name: "cfg-s-properties"}}))
			props := getSecret(ns, "cfg-s-properties")
			Expect(props.Data[propertiesSecretKey]).Should(Equal(before.Data[SaveInputPropertiesKey]))
			Expect(props.Labels).Should(HaveKeyWithValue(types.LabelConfigCreatedBy, types.ConfigCreatedByMigration))
			Expect(controllerOf(props).UID).Should(Equal(cfg.UID))
			Expect(getSecret(ns, "cfg-s").Data).Should(Equal(before.Data))
		})

		It("patches the config Secret last", func() {
			writeLegacyTemplate(ns, "order", legacyTemplateCUE("order", "namespace", false, other))
			writeLegacyConfig(ns, "order", ns, "cfg-o")
			var writes []string
			watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
			Expect(err).Should(Succeed())
			recording := interceptor.NewClient(watchClient, interceptor.Funcs{
				Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					writes = append(writes, obj.GetName())
					return c.Patch(ctx, obj, patch, opts...)
				},
				Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
					writes = append(writes, obj.GetName())
					return c.Update(ctx, obj, opts...)
				},
			})
			report, err := NewMigrator(recording).MigrateConfig(ctx, ns, "cfg-o", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Count(MigrationAdopted)).Should(Equal(1))
			Expect(writes).Should(Equal([]string{"cfg-o-local", "cfg-o"}))
			Expect(controllerOf(getSecret(ns, "cfg-o"))).ShouldNot(BeNil())
		})

		It("reports a conflict and writes nothing when a hand-made CR holds the name", func() {
			writeLegacyTemplate(ns, "clash", legacyTemplateCUE("clash", "namespace", false, ""))
			secret := writeLegacyConfig(ns, "clash", ns, "cfg-c")
			handmade := &configv1alpha1.Config{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "cfg-c"},
				Spec:       configv1alpha1.ConfigSpec{Alias: "mine"},
			}
			Expect(k8sClient.Create(ctx, handmade)).Should(Succeed())

			recorder := record.NewFakeRecorder(10)
			report, err := migrator.MigrateTemplate(ctx, ns, "clash", MigrationOptions{Recorder: recorder})
			Expect(err).Should(Succeed())
			Expect(report.HasConflict()).Should(BeTrue())
			res := resultFor(report, "Secret", ns, "cfg-c")
			Expect(res.Outcome).Should(Equal(MigrationConflict))
			Expect(res.Reason).Should(Equal("Config cfg-c exists and was not created by migration"))
			Expect(getSecret(ns, "cfg-c").ResourceVersion).Should(Equal(secret.ResourceVersion))
			Expect(getConfig(ns, "cfg-c").Spec.Alias).Should(Equal("mine"))
			Expect(<-recorder.Events).Should(ContainSubstring("MigrationAdopted ConfigTemplate clash created"))
			Expect(<-recorder.Events).Should(ContainSubstring("Warning MigrationConflict Config cfg-c exists and was not created by migration"))
		})

		It("completes a half-done adoption on the next run", func() {
			writeLegacyTemplate(ns, "resume", legacyTemplateCUE("resume", "namespace", false, ""))
			writeLegacyConfig(ns, "resume", ns, "cfg-r")
			// a Config from an earlier run whose owner patches never landed
			earlier := &configv1alpha1.Config{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "cfg-r", Labels: withMigrationLabel(nil)},
				Spec:       configv1alpha1.ConfigSpec{TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "resume", Namespace: ns}},
			}
			Expect(k8sClient.Create(ctx, earlier)).Should(Succeed())
			// and a ConfigTemplate from that run whose ConfigMap annotation never landed
			earlierCT := &configv1alpha1.ConfigTemplate{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "resume", Labels: withMigrationLabel(nil)},
				Spec:       configv1alpha1.ConfigTemplateSpec{Template: "template: {}"},
			}
			Expect(k8sClient.Create(ctx, earlierCT)).Should(Succeed())

			report, err := migrator.MigrateTemplate(ctx, ns, "resume", MigrationOptions{})
			Expect(err).Should(Succeed())
			for _, res := range report.Results {
				Expect(res.Outcome).Should(Equal(MigrationAdopted))
				Expect(res.Reason).Should(Equal("adoption completed"))
			}
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: TemplateConfigMapNamePrefix + "resume"}, &cm)).Should(Succeed())
			Expect(cm.Annotations).Should(HaveKeyWithValue(types.AnnotationConfigMigratedTo, string(earlierCT.UID)))
			after := getSecret(ns, "cfg-r")
			Expect(controllerOf(after).UID).Should(Equal(earlier.UID))
			Expect(after.Annotations).Should(HaveKeyWithValue(types.AnnotationConfigMigratedTo, string(earlier.UID)))
		})

		It("skips objects that opted out", func() {
			cm := writeLegacyTemplate(ns, "optout", legacyTemplateCUE("optout", "namespace", false, ""))
			secret := writeLegacyConfig(ns, "optout", ns, "cfg-no")
			setAnnotation(secret, types.AnnotationConfigMigrate, "false")
			Expect(k8sClient.Update(ctx, secret)).Should(Succeed())
			report, err := migrator.MigrateTemplate(ctx, ns, "optout", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(resultFor(report, "ConfigMap", ns, "optout").Outcome).Should(Equal(MigrationAdopted))
			res := resultFor(report, "Secret", ns, "cfg-no")
			Expect(res.Outcome).Should(Equal(MigrationSkipped))
			Expect(res.Reason).Should(Equal("opted out"))
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "cfg-no"}, &configv1alpha1.Config{}))).Should(BeTrue())
			Expect(cm).ShouldNot(BeNil())
		})

		It("dry run fills Target and writes nothing", func() {
			cm := writeLegacyTemplate(ns, "dry", legacyTemplateCUE("dry", "system", false, ""))
			secret := writeLegacyConfig(ns, "dry", ns, "cfg-d")
			recorder := record.NewFakeRecorder(10)
			report, err := migrator.MigrateTemplate(ctx, ns, "dry", MigrationOptions{DryRun: true, Recorder: recorder})
			Expect(err).Should(Succeed())
			Expect(report.Count(MigrationAdopted)).Should(Equal(2))
			ct, ok := report.Results[0].Target.(*configv1alpha1.ConfigTemplate)
			Expect(ok).Should(BeTrue())
			Expect(ct.Kind).Should(Equal("ConfigTemplate"))
			Expect(ct.Spec.Scope).Should(Equal(configv1alpha1.ConfigTemplateScopeSystem))
			cfg, ok := report.Results[1].Target.(*configv1alpha1.Config)
			Expect(ok).Should(BeTrue())
			Expect(cfg.Kind).Should(Equal("Config"))
			Expect(cfg.Spec.TemplateRef.Name).Should(Equal("dry"))
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "dry"}, &configv1alpha1.ConfigTemplate{}))).Should(BeTrue())
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "cfg-d"}, &configv1alpha1.Config{}))).Should(BeTrue())
			Expect(getSecret(ns, "cfg-d").ResourceVersion).Should(Equal(secret.ResourceVersion))
			var cmAfter corev1.ConfigMap
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), &cmAfter)).Should(Succeed())
			Expect(cmAfter.Annotations).ShouldNot(HaveKey(types.AnnotationConfigMigratedTo))
			Expect(recorder.Events).Should(BeEmpty())
		})

		It("returns an error for a template that does not exist or is not a template", func() {
			_, err := migrator.MigrateTemplate(ctx, ns, "nope", MigrationOptions{})
			Expect(err).Should(MatchError("the config template nope not found"))
			bogus := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: TemplateConfigMapNamePrefix + "bogus", Labels: map[string]string{"x": "y"}, Annotations: map[string]string{}}, Data: map[string]string{"template": "x"}}
			Expect(k8sClient.Create(ctx, bogus)).Should(Succeed())
			_, err = migrator.MigrateTemplate(ctx, ns, "bogus", MigrationOptions{})
			Expect(err).Should(MatchError("found a configmap but is not a config template"))
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "bogus"}, &configv1alpha1.ConfigTemplate{}))).Should(BeTrue())
		})
	})

	Context("configs in other namespaces", func() {
		sys := types.DefaultKubeVelaNS
		const teamA = "mig-team-a"
		const teamB = "mig-team-b"
		BeforeEach(func() {
			ensureNamespace(teamA)
			ensureNamespace(teamB)
		})

		It("finds the configs of a vela-system template wherever they live", func() {
			writeLegacyTemplate(sys, "shared", legacyTemplateCUE("shared", "system", false, ""))
			writeLegacyConfig(sys, "shared", teamA, "a-shared")
			b := writeLegacyConfig(sys, "shared", teamB, "b-shared")
			// the pre-CRD default: no annotation means vela-system
			delete(b.Annotations, types.AnnotationConfigTemplateNamespace)
			Expect(k8sClient.Update(ctx, b)).Should(Succeed())
			// same type name, but made from a template in another namespace: not ours
			writeLegacyTemplate(teamB, "shared", legacyTemplateCUE("shared", "namespace", false, ""))
			writeLegacyConfig(teamB, "shared", teamB, "b-own")

			report, err := migrator.MigrateTemplate(ctx, sys, "shared", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Results).Should(HaveLen(3))
			Expect(report.Results[0].Name).Should(Equal("shared"))
			Expect(resultFor(report, "Secret", teamA, "a-shared").Outcome).Should(Equal(MigrationAdopted))
			Expect(resultFor(report, "Secret", teamB, "b-shared").Outcome).Should(Equal(MigrationAdopted))
			Expect(getConfig(teamA, "a-shared").Spec.TemplateRef).Should(Equal(&configv1alpha1.ConfigTemplateReference{Name: "shared", Namespace: sys}))
			Expect(getConfig(teamB, "b-shared").Spec.TemplateRef).Should(Equal(&configv1alpha1.ConfigTemplateReference{Name: "shared", Namespace: sys}))
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: teamB, Name: "b-own"}, &configv1alpha1.Config{}))).Should(BeTrue())
		})

		It("falls back to the template namespace and reports when the cluster-wide list is denied", func() {
			writeLegacyTemplate(sys, "denied", legacyTemplateCUE("denied", "system", false, ""))
			writeLegacyConfig(sys, "denied", sys, "sys-denied")
			writeLegacyConfig(sys, "denied", teamA, "a-denied")
			watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
			Expect(err).Should(Succeed())
			forbidden := interceptor.NewClient(watchClient, interceptor.Funcs{
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					var lo client.ListOptions
					lo.ApplyOptions(opts)
					if _, isSecrets := list.(*corev1.SecretList); isSecrets && lo.Namespace == "" {
						return apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "", fmt.Errorf("no cluster-wide list"))
					}
					return c.List(ctx, list, opts...)
				},
			})
			report, err := NewMigrator(forbidden).MigrateTemplate(ctx, sys, "denied", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(resultFor(report, "Secret", sys, "sys-denied").Outcome).Should(Equal(MigrationAdopted))
			star := resultFor(report, "Secret", "*", "*")
			Expect(star.Outcome).Should(Equal(MigrationFailed))
			Expect(star.Reason).Should(HavePrefix("could not list configs in other namespaces: "))
			Expect(star.Reason).Should(ContainSubstring("no cluster-wide list"))
			Expect(report.Results[len(report.Results)-1]).Should(Equal(star))
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: teamA, Name: "a-denied"}, &configv1alpha1.Config{}))).Should(BeTrue())
		})
	})

	Context("config-level migration", func() {
		const ns = "mig-cfg"
		BeforeEach(func() {
			ensureNamespace(ns)
		})

		It("migrates one config while its template stays legacy", func() {
			writeLegacyTemplate(ns, "stay", legacyTemplateCUE("stay", "namespace", false, ""))
			before := writeLegacyConfig(ns, "stay", ns, "cfg-first")
			report, err := migrator.MigrateConfig(ctx, ns, "cfg-first", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Results).Should(HaveLen(1))
			Expect(report.Results[0].Outcome).Should(Equal(MigrationAdopted))
			Expect(report.Results[0].Reason).Should(Equal("Config cfg-first created"))
			cfg := getConfig(ns, "cfg-first")
			Expect(cfg.Spec.TemplateRef).Should(Equal(&configv1alpha1.ConfigTemplateReference{Name: "stay", Namespace: ns}))
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "stay"}, &configv1alpha1.ConfigTemplate{}))).Should(BeTrue())
			Expect(controllerOf(getSecret(ns, "cfg-first")).UID).Should(Equal(cfg.UID))
			Expect(getSecret(ns, "cfg-first").UID).Should(Equal(before.UID))
		})

		It("migrates one config whose template is already a CR", func() {
			writeLegacyTemplate(ns, "moved", legacyTemplateCUE("moved", "namespace", true, ""))
			writeLegacyConfig(ns, "moved", ns, "cfg-moved")
			_, err := migrator.MigrateTemplate(ctx, ns, "moved", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(k8sClient.Delete(ctx, getConfig(ns, "cfg-moved"))).Should(Succeed())
			// the Secret stays, now annotated and owned by a UID that no longer exists
			report, err := migrator.MigrateConfig(ctx, ns, "cfg-moved", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Results[0].Outcome).Should(Equal(MigrationAdopted))
			cfg := getConfig(ns, "cfg-moved")
			Expect(cfg.Spec.PropertiesFrom).ShouldNot(BeNil(), "sensitivity read from the CR")
			Expect(controllerOf(getSecret(ns, "cfg-moved")).UID).Should(Equal(cfg.UID))
		})

		It("fails a config whose template exists in neither form", func() {
			writeLegacyTemplate(ns, "gone", legacyTemplateCUE("gone", "namespace", false, ""))
			writeLegacyConfig(ns, "gone", ns, "cfg-gone")
			Expect(k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: TemplateConfigMapNamePrefix + "gone"}})).Should(Succeed())
			report, err := migrator.MigrateConfig(ctx, ns, "cfg-gone", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Results[0].Outcome).Should(Equal(MigrationFailed))
			Expect(report.Results[0].Reason).Should(Equal("template mig-cfg/gone not found"))
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "cfg-gone"}, &configv1alpha1.Config{}))).Should(BeTrue())
		})

		It("returns an error for a missing or non-config Secret", func() {
			_, err := migrator.MigrateConfig(ctx, ns, "missing", MigrationOptions{})
			Expect(err).Should(MatchError("the config missing not found"))
			Expect(k8sClient.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "plain-secret"}})).Should(Succeed())
			_, err = migrator.MigrateConfig(ctx, ns, "plain-secret", MigrationOptions{})
			Expect(err).Should(MatchError("found a secret but is not a config"))
		})

		It("treats a foreign companion properties Secret as a conflict", func() {
			writeLegacyTemplate(ns, "props-tpl", legacyTemplateCUE("props-tpl", "namespace", true, ""))
			before := writeLegacyConfig(ns, "props-tpl", ns, "cfg-props")
			foreign := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "cfg-props" + propertiesSecretSuffix},
				Data:       map[string][]byte{propertiesSecretKey: []byte(`{"value":"somebody else's"}`)},
			}
			Expect(k8sClient.Create(ctx, foreign)).Should(Succeed())

			for _, dry := range []bool{true, false} {
				report, err := migrator.MigrateConfig(ctx, ns, "cfg-props", MigrationOptions{DryRun: dry})
				Expect(err).Should(Succeed())
				Expect(report.Results[0].Outcome).Should(Equal(MigrationConflict), fmt.Sprintf("dry=%t", dry))
				Expect(report.Results[0].Reason).Should(Equal("Secret cfg-props-properties exists and was not created by migration"))
			}
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "cfg-props"}, &configv1alpha1.Config{}))).Should(BeTrue())
			Expect(getSecret(ns, "cfg-props").ResourceVersion).Should(Equal(before.ResourceVersion))
			var untouched corev1.Secret
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(foreign), &untouched)).Should(Succeed())
			Expect(untouched.OwnerReferences).Should(BeEmpty())
			Expect(untouched.Labels).ShouldNot(HaveKey(types.LabelConfigCreatedBy))

			By("a companion Secret from an interrupted earlier run, same data and label, is reused")
			Expect(k8sClient.Delete(ctx, foreign)).Should(Succeed())
			ours := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "cfg-props" + propertiesSecretSuffix, Labels: withMigrationLabel(nil)},
				Data:       map[string][]byte{propertiesSecretKey: before.Data[SaveInputPropertiesKey]},
			}
			Expect(k8sClient.Create(ctx, ours)).Should(Succeed())
			report, err := migrator.MigrateConfig(ctx, ns, "cfg-props", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Results[0].Outcome).Should(Equal(MigrationAdopted))
			Expect(controllerOf(getSecret(ns, "cfg-props"+propertiesSecretSuffix)).UID).Should(Equal(getConfig(ns, "cfg-props").UID))
		})

		It("fails a config whose objects-reference key is malformed and leaves it unowned", func() {
			writeLegacyTemplate(ns, "bad-ref", legacyTemplateCUE("bad-ref", "namespace", false, ""))
			secret := writeLegacyConfig(ns, "bad-ref", ns, "cfg-bad")
			secret.Data[SaveObjectReferenceKey] = []byte("{not json")
			Expect(k8sClient.Update(ctx, secret)).Should(Succeed())
			report, err := migrator.MigrateConfig(ctx, ns, "cfg-bad", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Results[0].Outcome).Should(Equal(MigrationFailed))
			Expect(report.Results[0].Reason).Should(HavePrefix("objects-reference is not valid JSON: "))
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "cfg-bad"}, &configv1alpha1.Config{}))).Should(BeTrue())
			Expect(getSecret(ns, "cfg-bad").OwnerReferences).Should(BeEmpty())
		})

		It("leaves source cache entries to the application controller", func() {
			writeLegacyTemplate(ns, "cache-tpl", legacyTemplateCUE("cache-tpl", "namespace", false, ""))
			entry := writeLegacyConfig(ns, "cache-tpl", ns, "cache-entry")
			entry.Labels[types.LabelSourceDefinitionName] = "some-source"
			Expect(k8sClient.Update(ctx, entry)).Should(Succeed())
			writeLegacyConfig(ns, "cache-tpl", ns, "cache-sibling")

			report, err := migrator.MigrateTemplate(ctx, ns, "cache-tpl", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Results).Should(HaveLen(2), "the template and the sibling only")
			Expect(resultFor(report, "Secret", ns, "cache-sibling").Outcome).Should(Equal(MigrationAdopted))
			report, err = migrator.MigrateConfigs(ctx, ns, MigrationOptions{})
			Expect(err).Should(Succeed())
			for _, res := range report.Results {
				Expect(res.Name).ShouldNot(Equal("cache-entry"))
			}
			report, err = migrator.MigrateConfig(ctx, ns, "cache-entry", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Results[0].Outcome).Should(Equal(MigrationSkipped))
			Expect(report.Results[0].Reason).Should(Equal("source cache entry managed by the controller"))
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "cache-entry"}, &configv1alpha1.Config{}))).Should(BeTrue())
		})

		It("migrates every legacy config in a namespace, template or not", func() {
			const ns = "mig-cfg-all"
			ensureNamespace(ns)
			writeLegacyTemplate(ns, "all-t", legacyTemplateCUE("all-t", "namespace", false, ""))
			writeLegacyConfig(ns, "all-t", ns, "all-with")
			writeLegacyTemplate(types.DefaultKubeVelaNS, "shared-all", legacyTemplateCUE("shared-all", "system", false, ""))
			writeLegacyConfig(types.DefaultKubeVelaNS, "shared-all", ns, "all-foreign-missing")
			Expect(k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: types.DefaultKubeVelaNS, Name: TemplateConfigMapNamePrefix + "shared-all"}})).Should(Succeed())
			noTpl, err := fac.ParseConfig(ctx, NamespacedName{}, Metadata{
				NamespacedName: NamespacedName{Namespace: ns, Name: "all-none"},
				Properties:     map[string]interface{}{"key": "value"},
			})
			Expect(err).Should(Succeed())
			Expect(fac.CreateOrUpdateConfig(ctx, noTpl, ns)).Should(Succeed())
			_, err = migrator.MigrateConfig(ctx, ns, "all-with", MigrationOptions{})
			Expect(err).Should(Succeed())

			report, err := migrator.MigrateConfigs(ctx, ns, MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(resultFor(report, "Secret", ns, "all-with").Reason).Should(Equal("already migrated"))
			Expect(resultFor(report, "Secret", ns, "all-foreign-missing").Outcome).Should(Equal(MigrationFailed))
			Expect(resultFor(report, "Secret", ns, "all-foreign-missing").Reason).Should(Equal("template vela-system/shared-all not found"))
			none := resultFor(report, "Secret", ns, "all-none")
			Expect(none.Outcome).Should(Equal(MigrationAdopted))
			cfg := getConfig(ns, "all-none")
			Expect(cfg.Spec.TemplateRef).Should(BeNil())
			Expect(string(cfg.Spec.Properties.Raw)).Should(MatchJSON(`{"key":"value"}`))
			Expect(report.Results).Should(HaveLen(3))
		})
	})

	Context("a whole namespace", func() {
		const ns = "mig-all"
		It("migrates two templates, their configs and a template-less config once each", func() {
			ensureNamespace(ns)
			writeLegacyTemplate(ns, "ns-a", legacyTemplateCUE("ns-a", "namespace", false, ""))
			writeLegacyTemplate(ns, "ns-b", legacyTemplateCUE("ns-b", "namespace", false, ""))
			writeLegacyConfig(ns, "ns-a", ns, "a-cfg")
			writeLegacyConfig(ns, "ns-b", ns, "b-cfg")
			noTpl, err := fac.ParseConfig(ctx, NamespacedName{}, Metadata{NamespacedName: NamespacedName{Namespace: ns, Name: "no-tpl"}})
			Expect(err).Should(Succeed())
			Expect(fac.CreateOrUpdateConfig(ctx, noTpl, ns)).Should(Succeed())

			report, err := migrator.MigrateNamespace(ctx, ns, MigrationOptions{})
			Expect(err).Should(Succeed())
			var names []string
			for _, res := range report.Results {
				Expect(res.Outcome).Should(Equal(MigrationAdopted), res.Name)
				names = append(names, res.Kind+"/"+res.Name)
			}
			Expect(strings.Join(names, " ")).Should(Equal("ConfigMap/ns-a Secret/a-cfg ConfigMap/ns-b Secret/b-cfg Secret/no-tpl"))
			Expect(getConfig(ns, "no-tpl").Spec.TemplateRef).Should(BeNil())

			report, err = migrator.MigrateNamespace(ctx, ns, MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Count(MigrationSkipped)).Should(Equal(5))
			Expect(report.Results).Should(HaveLen(5))
		})
	})

	It("leaves a cluster-scoped output alone and rejects a malformed reference key", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "cluster-out"},
			Data:       map[string][]byte{SaveObjectReferenceKey: []byte(`[{"kind":"ClusterRole","apiVersion":"rbac.authorization.k8s.io/v1","name":"x"}]`)},
		}
		refs, err := objectReferences(secret)
		Expect(err).Should(Succeed())
		Expect(outputsOutside(refs, "default")).Should(Equal([]string{"ClusterRole /x"}))
		refs, err = objectReferences(&corev1.Secret{})
		Expect(err).Should(Succeed())
		Expect(refs).Should(BeNil())
		_, err = objectReferences(&corev1.Secret{Data: map[string][]byte{SaveObjectReferenceKey: []byte(`{not json`)}})
		Expect(err).Should(MatchError(HavePrefix("objects-reference is not valid JSON: ")))
	})

	It("ignores an unknown unstructured output that no longer exists", func() {
		u := &unstructured.Unstructured{}
		u.SetAPIVersion("v1")
		u.SetKind("ConfigMap")
		u.SetNamespace("default")
		u.SetName("does-not-exist")
		cfg := &configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "owner", UID: "u"}}
		Expect(apierrors.IsNotFound(migrator.own(ctx, cfg, u, false))).Should(BeTrue())
	})
})
