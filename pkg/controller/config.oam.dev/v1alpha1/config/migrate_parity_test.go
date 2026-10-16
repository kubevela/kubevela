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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/types"
	legacyconfig "github.com/oam-dev/kubevela/pkg/config"
)

// parityTemplateCUE is a legacy template with a same-namespace output object.
func parityTemplateCUE(name string, sensitive bool) string {
	return fmt.Sprintf(`
metadata: {
	name:      %q
	alias:     "parity"
	scope:     "namespace"
	sensitive: %t
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
	}
	outputs: {
		local: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {name: context.name + "-out", namespace: context.namespace}
			data: value: parameter.value
		}
	}
}
`, name, sensitive)
}

// An adopted legacy config must come out of its first reconcile as the Config
// controller would have written it: same Secret, same data, now owned.
var _ = Describe("Migration parity with the running Config controller", func() {
	ctx := context.Background()
	const ns = "parity"
	var fac legacyconfig.Factory
	var migrator *legacyconfig.Migrator

	BeforeEach(func() {
		err := k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
		Expect(client.IgnoreAlreadyExists(err)).Should(Succeed())
		fac = legacyconfig.NewConfigFactory(k8sClient)
		migrator = legacyconfig.NewMigrator(k8sClient)
	})

	seed := func(tpl, cfgName string, sensitive bool) (*corev1.Secret, *corev1.ConfigMap) {
		t, err := fac.ParseTemplate(ctx, tpl, []byte(parityTemplateCUE(tpl, sensitive)))
		Expect(err).Should(Succeed())
		Expect(fac.CreateOrUpdateConfigTemplate(ctx, ns, t)).Should(Succeed())
		item, err := fac.ParseConfig(ctx, legacyconfig.NamespacedName{Namespace: ns, Name: tpl}, legacyconfig.Metadata{
			NamespacedName: legacyconfig.NamespacedName{Namespace: ns, Name: cfgName},
			Alias:          "a",
			Properties:     map[string]interface{}{"value": "v-" + cfgName},
		})
		Expect(err).Should(Succeed())
		Expect(fac.CreateOrUpdateConfig(ctx, item, ns)).Should(Succeed())
		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: cfgName}, &secret)).Should(Succeed())
		var out corev1.ConfigMap
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: cfgName + "-out"}, &out)).Should(Succeed())
		Expect(secret.OwnerReferences).Should(BeEmpty())
		return &secret, &out
	}

	// assertUnchanged compares the Secret after adoption and the first reconcile with
	// the legacy snapshot: identity, type, data and labels identical; annotations
	// identical plus the migrated-to key; and the Config as controller owner. Exact
	// equality, so anything else the controller stamps shows up here.
	assertUnchanged := func(before *corev1.Secret, cfg *configv1alpha1.Config) {
		var after corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(before), &after)).Should(Succeed())
		Expect(after.UID).Should(Equal(before.UID))
		Expect(after.Type).Should(Equal(before.Type))
		Expect(after.Data).Should(Equal(before.Data))
		Expect(after.Labels).Should(Equal(before.Labels))
		wantAnnotations := map[string]string{types.AnnotationConfigMigratedTo: string(cfg.UID)}
		for k, v := range before.Annotations {
			wantAnnotations[k] = v
		}
		Expect(after.Annotations).Should(Equal(wantAnnotations))
		Expect(metav1.GetControllerOf(&after)).ShouldNot(BeNil())
		Expect(metav1.GetControllerOf(&after).UID).Should(Equal(cfg.UID))
	}

	It("keeps an adopted Secret and its output object unchanged through the first reconcile", func() {
		before, outBefore := seed("parity-plain", "parity-cfg", false)
		report, err := migrator.MigrateTemplate(ctx, ns, "parity-plain", legacyconfig.MigrationOptions{})
		Expect(err).Should(Succeed())
		Expect(report.Count(legacyconfig.MigrationAdopted)).Should(Equal(2))

		var cfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKey{Namespace: ns, Name: "parity-cfg"}, configv1alpha1.ConfigPhaseAvailable, &cfg)
		assertUnchanged(before, &cfg)

		var outAfter corev1.ConfigMap
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(outBefore), &outAfter)).Should(Succeed())
		Expect(outAfter.UID).Should(Equal(outBefore.UID))
		Expect(outAfter.Data).Should(Equal(outBefore.Data))
		Expect(metav1.GetControllerOf(&outAfter).UID).Should(Equal(cfg.UID))
		// envtest runs no garbage collector, so "deleted with the Config" is shown by
		// the manual scenarios; the owner reference above is what makes it happen.
	})

	// configFor is the Config the migration library builds for a seeded legacy config,
	// written by hand so the test, not the library, decides when the owner reference lands.
	configFor := func(tpl, cfgName string, migrationCreated bool) *configv1alpha1.Config {
		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: cfgName, Namespace: ns},
			Spec: configv1alpha1.ConfigSpec{
				Alias:       "a",
				TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: tpl, Namespace: ns},
				Properties:  &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"value":%q}`, "v-"+cfgName))},
			},
		}
		if migrationCreated {
			cfg.Labels = map[string]string{types.LabelConfigCreatedBy: types.ConfigCreatedByMigration}
		}
		return cfg
	}

	// markFor writes what the library writes right before it creates the Config: the
	// adopt-by mark on the legacy Secret, the controller's proof.
	markFor := func(secret *corev1.Secret, cfgName string) {
		orig := secret.DeepCopy()
		if secret.Annotations == nil {
			secret.Annotations = map[string]string{}
		}
		secret.Annotations[types.AnnotationConfigAdoptBy] = cfgName
		Expect(k8sClient.Patch(ctx, secret, client.MergeFrom(orig))).Should(Succeed())
	}

	It("adopts the legacy Secret and its output itself on the migration's mark, before the library's owner patch", func() {
		// The library marks the Secret, creates the Config and then patches the owner
		// references; a reconcile in between used to find them unowned and report Error
		// until the next one. Here the patch never comes, so Available means the controller
		// adopted them on its own.
		before, outBefore := seed("parity-race", "parity-race-cfg", false)
		markFor(before, "parity-race-cfg")
		cfg := configFor("parity-race", "parity-race-cfg", true)
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cfg))).Should(Succeed()) })

		var got configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseAvailable, &got)
		var after corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(before), &after)).Should(Succeed())
		Expect(after.UID).Should(Equal(before.UID))
		Expect(after.Data).Should(Equal(before.Data))
		Expect(after.Annotations).ShouldNot(HaveKey(types.AnnotationConfigAdoptBy), "the mark is spent")
		Expect(metav1.GetControllerOf(&after)).ShouldNot(BeNil())
		Expect(metav1.GetControllerOf(&after).UID).Should(Equal(got.UID))
		var outAfter corev1.ConfigMap
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(outBefore), &outAfter)).Should(Succeed())
		Expect(outAfter.UID).Should(Equal(outBefore.UID))
		Expect(metav1.GetControllerOf(&outAfter)).ShouldNot(BeNil())
		Expect(metav1.GetControllerOf(&outAfter).UID).Should(Equal(got.UID))
	})

	It("still refuses the legacy Secret for a Config nothing marked it for, migration label or not", func() {
		// The label is the Config author's to set; the mark takes write access to the Secret.
		before, _ := seed("parity-hand", "parity-hand-cfg", false)
		cfg := configFor("parity-hand", "parity-hand-cfg", true)
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())
		// left in Error it would be rendered again every errorRetryInterval for the rest of the run
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cfg))).Should(Succeed()) })

		var got configv1alpha1.Config
		eventuallyConfigError(ctx, client.ObjectKeyFromObject(cfg), "already exists and is not owned by this Config", &got)
		var after corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(before), &after)).Should(Succeed())
		Expect(after.OwnerReferences).Should(BeEmpty())
		Expect(after.Data).Should(Equal(before.Data))
	})

	It("keeps an adopted sensitive Secret unchanged, with its input in the companion Secret", func() {
		before, _ := seed("parity-secret", "parity-sensitive", true)
		report, err := migrator.MigrateTemplate(ctx, ns, "parity-secret", legacyconfig.MigrationOptions{})
		Expect(err).Should(Succeed())
		Expect(report.Count(legacyconfig.MigrationAdopted)).Should(Equal(2))

		var cfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKey{Namespace: ns, Name: "parity-sensitive"}, configv1alpha1.ConfigPhaseAvailable, &cfg)
		Expect(cfg.Spec.Properties).Should(BeNil())
		Expect(cfg.Spec.PropertiesFrom.SecretRef.Name).Should(Equal("parity-sensitive-properties"))
		assertUnchanged(before, &cfg)
	})

	It("reaches Available for a config migrated while its template stays a legacy ConfigMap", func() {
		before, _ := seed("parity-stay", "parity-first", false)
		report, err := migrator.MigrateConfig(ctx, ns, "parity-first", legacyconfig.MigrationOptions{})
		Expect(err).Should(Succeed())
		Expect(report.Results).Should(HaveLen(1))
		Expect(report.Results[0].Outcome).Should(Equal(legacyconfig.MigrationAdopted))

		var cfg configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKey{Namespace: ns, Name: "parity-first"}, configv1alpha1.ConfigPhaseAvailable, &cfg)
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "parity-stay"}, &configv1alpha1.ConfigTemplate{})).ShouldNot(Succeed())
		assertUnchanged(before, &cfg)
	})
})
