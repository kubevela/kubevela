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
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// legacyTemplateCUE is a template with a same-namespace output object and, when
// otherNS is set, one in another namespace, the shape 1.11.0 allowed.
func legacyTemplateCUE(name, scope string, sensitive bool, otherNS string) string {
	remote := ""
	if otherNS != "" {
		remote = fmt.Sprintf(`
		remote: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {name: context.name + "-remote", namespace: %q}
			data: value: parameter.value
		}`, otherNS)
	}
	outputs := fmt.Sprintf(`
	outputs: {
		local: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {name: context.name + "-local", namespace: context.namespace}
			data: value: parameter.value
		}%s
	}`, remote)
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

	// forbiddenClusterList is a client whose cluster-wide Secret list is refused, the
	// way a namespace-scoped user sees it; secretLists counts every Secret list call.
	forbiddenClusterList := func(secretLists *int) client.Client {
		watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).Should(Succeed())
		return interceptor.NewClient(watchClient, interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				var lo client.ListOptions
				lo.ApplyOptions(opts)
				if _, isSecrets := list.(*corev1.SecretList); isSecrets {
					if secretLists != nil {
						*secretLists++
					}
					if lo.Namespace == "" {
						return apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "", fmt.Errorf("no cluster-wide list"))
					}
				}
				return c.List(ctx, list, opts...)
			},
		})
	}

	// failingClusterList is a client whose cluster-wide Secret list fails the way a
	// timed-out or unavailable API server does; namespace lists still work.
	failingClusterList := func() client.Client {
		watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).Should(Succeed())
		return interceptor.NewClient(watchClient, interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				var lo client.ListOptions
				lo.ApplyOptions(opts)
				if _, isSecrets := list.(*corev1.SecretList); isSecrets && lo.Namespace == "" {
					return apierrors.NewInternalError(fmt.Errorf("etcdserver: request timed out"))
				}
				return c.List(ctx, list, opts...)
			},
		})
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
		for _, ns := range []string{"mig-one", "mig-one-other", "mig-team-a", "mig-team-b", "mig-cfg", "mig-cfg-all", "mig-all", "mig-all-1", "mig-all-2", "mig-gitops"} {
			for _, obj := range []client.Object{&configv1alpha1.Config{}, &configv1alpha1.ConfigTemplate{}, &v1beta1.Application{}, &corev1.Secret{}, &corev1.ConfigMap{}} {
				Expect(client.IgnoreNotFound(k8sClient.DeleteAllOf(ctx, obj, client.InNamespace(ns)))).Should(Succeed())
			}
		}
		for _, name := range []string{"shared", "shared2", "shared-all", "denied", "flaky", "ca-sys"} {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: types.DefaultKubeVelaNS, Name: TemplateConfigMapNamePrefix + name}}))).Should(Succeed())
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &configv1alpha1.ConfigTemplate{ObjectMeta: metav1.ObjectMeta{Namespace: types.DefaultKubeVelaNS, Name: name}}))).Should(Succeed())
		}
		for _, name := range []string{"sys-denied", "sys-flaky"} {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: types.DefaultKubeVelaNS, Name: name}}))).Should(Succeed())
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Namespace: types.DefaultKubeVelaNS, Name: name}}))).Should(Succeed())
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: types.DefaultKubeVelaNS, Name: name + "-local"}}))).Should(Succeed())
		}
	})

	Context("one template with its configs", func() {
		const ns = "mig-one"
		const other = "mig-one-other"
		BeforeEach(func() {
			ensureNamespace(ns)
			ensureNamespace(other)
		})

		It("adopts the template, its configs, outputs and the distribution", func() {
			cm := writeLegacyTemplate(ns, "plain", legacyTemplateCUE("plain", "project", false, ""))
			secret := writeLegacyConfig(ns, "plain", ns, "cfg-a")
			Expect(fac.CreateOrUpdateDistribution(ctx, ns, DefaultDistributionName("cfg-a"), &CreateDistributionSpec{
				Targets: []*ClusterTarget{{ClusterName: types.ClusterLocalName, Namespace: other}},
				Configs: []*NamespacedName{{Namespace: ns, Name: "cfg-a"}},
			})).Should(Succeed())
			var localOut corev1.ConfigMap
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "cfg-a-local"}, &localOut)).Should(Succeed())

			recorder := record.NewFakeRecorder(20)
			report, err := migrator.MigrateTemplate(ctx, ns, "plain", MigrationOptions{Recorder: recorder})
			Expect(err).Should(Succeed())
			Expect(report.Results).Should(HaveLen(2))
			Expect(report.Results[0]).Should(MatchFields(IgnoreExtras, Fields{
				"Kind": Equal("ConfigMap"), "Name": Equal("plain"), "Outcome": Equal(MigrationAdopted), "Reason": Equal("ConfigTemplate plain created"),
			}))
			Expect(report.Results[1]).Should(MatchFields(IgnoreExtras, Fields{
				"Kind": Equal("Secret"), "Name": Equal("cfg-a"), "Outcome": Equal(MigrationAdopted),
				"Reason": Equal("Config cfg-a created"),
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

			By("the output object is owned, same UID")
			var localAfter corev1.ConfigMap
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&localOut), &localAfter)).Should(Succeed())
			Expect(localAfter.UID).Should(Equal(localOut.UID))
			Expect(controllerOf(&localAfter).UID).Should(Equal(cfg.UID))

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
			Expect(events).Should(HaveLen(2))

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

		It("releases a config an Application dispatched from it: the app and owner marks leave the Secret and its output", func() {
			writeLegacyTemplate(ns, "dispatched", legacyTemplateCUE("dispatched", "project", false, ""))
			secret := writeLegacyConfig(ns, "dispatched", ns, "cfg-app")
			var out corev1.ConfigMap
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "cfg-app-local"}, &out)).Should(Succeed())
			marks := map[string]string{oam.LabelAppName: "wf-app", oam.LabelAppNamespace: ns, oam.LabelOwnerKind: "Application", oam.LabelOwnerName: "wf-app", oam.LabelOwnerNamespace: ns}
			for _, obj := range []client.Object{secret, &out} {
				labels := obj.GetLabels()
				if labels == nil {
					labels = map[string]string{}
				}
				for k, v := range marks {
					labels[k] = v
				}
				obj.SetLabels(labels)
				Expect(k8sClient.Update(ctx, obj)).Should(Succeed())
			}

			report, err := migrator.MigrateConfig(ctx, ns, "cfg-app", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Count(MigrationAdopted)).Should(Equal(1))

			after := getSecret(ns, "cfg-app")
			var outAfter corev1.ConfigMap
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&out), &outAfter)).Should(Succeed())
			for _, obj := range []client.Object{after, &outAfter} {
				for k := range marks {
					Expect(obj.GetLabels()).ShouldNot(HaveKey(k), "%s: the Config owns it now; the app's GC would delete it otherwise", obj.GetName())
				}
			}
			Expect(after.Labels).Should(HaveKeyWithValue(types.LabelConfigCatalog, types.VelaCoreConfig), "the config labels stay")
			Expect(after.Annotations).ShouldNot(HaveKey(types.AnnotationConfigAdoptBy), "the mark written before the Create is spent by the adoption")
			Expect(after.Annotations).Should(HaveKey(types.AnnotationConfigMigratedTo))
		})

		// #7449 review item 1: the controller renders every output into the Config's
		// namespace, so a config with an output elsewhere is a conflict, never adopted.
		It("refuses a config whose output object lives in another namespace", func() {
			writeLegacyTemplate(ns, "xns", legacyTemplateCUE("xns", "namespace", false, other))
			secret := writeLegacyConfig(ns, "xns", ns, "cfg-x")
			var remoteOut corev1.ConfigMap
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: other, Name: "cfg-x-remote"}, &remoteOut)).Should(Succeed())
			const reason = "1 output object(s) outside mig-one would be recreated there by the controller: ConfigMap mig-one-other/cfg-x-remote"

			recorder := record.NewFakeRecorder(10)
			for _, dry := range []bool{true, false} {
				report, err := migrator.MigrateTemplate(ctx, ns, "xns", MigrationOptions{DryRun: dry, Recorder: recorder})
				Expect(err).Should(Succeed())
				Expect(resultFor(report, "ConfigMap", ns, "xns").Outcome).Should(Equal(MigrationAdopted))
				res := resultFor(report, "Secret", ns, "cfg-x")
				Expect(res.Outcome).Should(Equal(MigrationConflict), fmt.Sprintf("dry=%t", dry))
				Expect(res.Reason).Should(Equal(reason))
				Expect(report.HasConflict()).Should(BeTrue())
			}
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "cfg-x"}, &configv1alpha1.Config{}))).Should(BeTrue())
			Expect(getSecret(ns, "cfg-x").ResourceVersion).Should(Equal(secret.ResourceVersion))
			var remoteAfter corev1.ConfigMap
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&remoteOut), &remoteAfter)).Should(Succeed())
			Expect(remoteAfter.ResourceVersion).Should(Equal(remoteOut.ResourceVersion))
			Expect(remoteAfter.OwnerReferences).Should(BeEmpty())
			var events []string
			for len(recorder.Events) > 0 {
				events = append(events, <-recorder.Events)
			}
			Expect(events).Should(ContainElement(ContainSubstring("Warning MigrationConflict " + reason)))
			Expect(events).ShouldNot(ContainElement(ContainSubstring("OutputsUnowned")))

			By("the operator drops the remote entry from objects-reference and migrates again")
			local, err := json.Marshal([]corev1.ObjectReference{{APIVersion: "v1", Kind: "ConfigMap", Namespace: ns, Name: "cfg-x-local"}})
			Expect(err).Should(Succeed())
			edited := getSecret(ns, "cfg-x")
			edited.Data[SaveObjectReferenceKey] = local
			Expect(k8sClient.Update(ctx, edited)).Should(Succeed())
			report, err := migrator.MigrateConfig(ctx, ns, "cfg-x", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Results[0].Outcome).Should(Equal(MigrationAdopted))
			Expect(report.Results[0].Reason).Should(Equal("Config cfg-x created"))
			var localAfter corev1.ConfigMap
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "cfg-x-local"}, &localAfter)).Should(Succeed())
			Expect(controllerOf(&localAfter).UID).Should(Equal(getConfig(ns, "cfg-x").UID))
		})

		// #7449 review item 2: a config follows its template. A ConfigTemplate that
		// migration did not create, beside the legacy ConfigMap, blocks the configs in
		// --all, in the single-config path and in the dry run.
		It("stops at a template a hand-made ConfigTemplate shadows, in every path", func() {
			writeLegacyTemplate(ns, "clash-t", legacyTemplateCUE("clash-t", "namespace", false, ""))
			secret := writeLegacyConfig(ns, "clash-t", ns, "cfg-ct")
			handmade := &configv1alpha1.ConfigTemplate{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "clash-t"},
				Spec:       configv1alpha1.ConfigTemplateSpec{Template: "template: {}"},
			}
			Expect(k8sClient.Create(ctx, handmade)).Should(Succeed())
			const templateReason = "ConfigTemplate clash-t exists and was not created by migration"
			const configReason = "template mig-one/clash-t: ConfigTemplate exists and was not created by migration"

			recorder := record.NewFakeRecorder(10)
			for _, dry := range []bool{true, false} {
				report, err := migrator.MigrateTemplate(ctx, ns, "clash-t", MigrationOptions{DryRun: dry, Recorder: recorder})
				Expect(err).Should(Succeed())
				Expect(report.Results).Should(HaveLen(2))
				Expect(report.Results[0]).Should(MatchFields(IgnoreExtras, Fields{"Kind": Equal("ConfigMap"), "Outcome": Equal(MigrationConflict), "Reason": Equal(templateReason)}))
				Expect(report.Results[1]).Should(MatchFields(IgnoreExtras, Fields{"Kind": Equal("Secret"), "Name": Equal("cfg-ct"), "Outcome": Equal(MigrationConflict), "Reason": Equal(configReason)}))
			}
			report, err := migrator.MigrateConfig(ctx, ns, "cfg-ct", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Results[0].Outcome).Should(Equal(MigrationConflict))
			Expect(report.Results[0].Reason).Should(Equal(configReason))
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "cfg-ct"}, &configv1alpha1.Config{}))).Should(BeTrue())
			Expect(getSecret(ns, "cfg-ct").ResourceVersion).Should(Equal(secret.ResourceVersion))
			var events []string
			for len(recorder.Events) > 0 {
				events = append(events, <-recorder.Events)
			}
			Expect(events).Should(ConsistOf(
				ContainSubstring("Warning MigrationConflict "+templateReason),
				ContainSubstring("Warning MigrationConflict "+configReason)))

			By("with the legacy ConfigMap gone the CR alone is the template, and the config binds to it")
			Expect(k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: TemplateConfigMapNamePrefix + "clash-t"}})).Should(Succeed())
			report, err = migrator.MigrateConfig(ctx, ns, "cfg-ct", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Results[0].Outcome).Should(Equal(MigrationAdopted))
			Expect(getConfig(ns, "cfg-ct").Spec.TemplateRef).Should(Equal(&configv1alpha1.ConfigTemplateReference{Name: "clash-t", Namespace: ns}))
		})

		// The migrated-to annotation on the ConfigMap is as good as the label on the
		// CR: a CR relabelled by hand or by a controller still stands for the
		// ConfigMap that records it, and its configs adopt.
		It("lets a CR that lost the migration label stand for the ConfigMap that records it", func() {
			writeLegacyTemplate(ns, "relabel-t", legacyTemplateCUE("relabel-t", "namespace", false, ""))
			report, err := migrator.MigrateTemplate(ctx, ns, "relabel-t", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Count(MigrationAdopted)).Should(Equal(1))
			ct := getTemplateCR(ns, "relabel-t")
			delete(ct.Labels, types.LabelConfigCreatedBy)
			Expect(k8sClient.Update(ctx, ct)).Should(Succeed())
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: TemplateConfigMapNamePrefix + "relabel-t"}, &cm)).Should(Succeed())
			Expect(cm.Annotations).Should(HaveKeyWithValue(types.AnnotationConfigMigratedTo, string(ct.UID)))

			writeLegacyConfig(ns, "relabel-t", ns, "cfg-relabel")
			for _, dry := range []bool{true, false} {
				report, err = migrator.MigrateConfig(ctx, ns, "cfg-relabel", MigrationOptions{DryRun: dry})
				Expect(err).Should(Succeed())
				Expect(report.Results).Should(HaveLen(1))
				Expect(report.Results[0].Outcome).Should(Equal(MigrationAdopted), report.Results[0].Reason)
			}
			Expect(getConfig(ns, "cfg-relabel").Spec.TemplateRef).Should(Equal(&configv1alpha1.ConfigTemplateReference{Name: "relabel-t", Namespace: ns}))
		})

		It("keeps the configs of an opted-out template legacy with it", func() {
			cm := writeLegacyTemplate(ns, "opt-t", legacyTemplateCUE("opt-t", "namespace", false, ""))
			writeLegacyConfig(ns, "opt-t", ns, "cfg-ot")
			setAnnotation(cm, types.AnnotationConfigMigrate, "false")
			Expect(k8sClient.Update(ctx, cm)).Should(Succeed())
			const reason = "template mig-one/opt-t opted out"

			recorder := record.NewFakeRecorder(10)
			report, err := migrator.MigrateTemplate(ctx, ns, "opt-t", MigrationOptions{Recorder: recorder})
			Expect(err).Should(Succeed())
			Expect(resultFor(report, "ConfigMap", ns, "opt-t").Reason).Should(Equal("opted out"))
			res := resultFor(report, "Secret", ns, "cfg-ot")
			Expect(res.Outcome).Should(Equal(MigrationSkipped))
			Expect(res.Reason).Should(Equal(reason))
			report, err = migrator.MigrateConfig(ctx, ns, "cfg-ot", MigrationOptions{Recorder: recorder})
			Expect(err).Should(Succeed())
			Expect(report.Results[0].Outcome).Should(Equal(MigrationSkipped))
			Expect(report.Results[0].Reason).Should(Equal(reason))
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "cfg-ot"}, &configv1alpha1.Config{}))).Should(BeTrue())
			Expect(<-recorder.Events).Should(ContainSubstring("Normal MigrationSkipped opted out"))
			Expect(<-recorder.Events).Should(ContainSubstring("Normal MigrationSkipped " + reason))
		})

		// #7449 review item 3: the template may have been flipped to non-sensitive
		// since; the Secret's own annotation still keeps the input out of the CR.
		It("keeps the input out of the CR when only the Secret still says sensitive", func() {
			cm := writeLegacyTemplate(ns, "flip", legacyTemplateCUE("flip", "namespace", true, ""))
			before := writeLegacyConfig(ns, "flip", ns, "cfg-f")
			Expect(before.Annotations).Should(HaveKeyWithValue(types.AnnotationConfigSensitive, "true"))
			cm.Annotations[types.AnnotationConfigSensitive] = "false"
			Expect(k8sClient.Update(ctx, cm)).Should(Succeed())

			report, err := migrator.MigrateTemplate(ctx, ns, "flip", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Count(MigrationAdopted)).Should(Equal(2))
			Expect(getTemplateCR(ns, "flip").Spec.Sensitive).Should(BeFalse(), "adopted as flipped")
			cfg := getConfig(ns, "cfg-f")
			Expect(cfg.Spec.Properties).Should(BeNil())
			Expect(cfg.Spec.PropertiesFrom).Should(Equal(&configv1alpha1.PropertiesReference{SecretRef: configv1alpha1.SecretKeySelector{Name: "cfg-f-properties"}}))
			Expect(getSecret(ns, "cfg-f-properties").Data[DefaultPropertiesSecretKey]).Should(Equal(before.Data[SaveInputPropertiesKey]))
		})

		It("keeps the input out of the CR for a template-less config whose Secret says sensitive", func() {
			noTpl, err := fac.ParseConfig(ctx, NamespacedName{}, Metadata{
				NamespacedName: NamespacedName{Namespace: ns, Name: "cfg-nt-s"},
				Properties:     map[string]interface{}{"token": "t"},
			})
			Expect(err).Should(Succeed())
			Expect(fac.CreateOrUpdateConfig(ctx, noTpl, ns)).Should(Succeed())
			before := getSecret(ns, "cfg-nt-s")
			setAnnotation(before, types.AnnotationConfigSensitive, "true")
			Expect(k8sClient.Update(ctx, before)).Should(Succeed())

			report, err := migrator.MigrateConfig(ctx, ns, "cfg-nt-s", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Results[0].Outcome).Should(Equal(MigrationAdopted))
			cfg := getConfig(ns, "cfg-nt-s")
			Expect(cfg.Spec.TemplateRef).Should(BeNil())
			Expect(cfg.Spec.Properties).Should(BeNil())
			Expect(cfg.Spec.PropertiesFrom).Should(Equal(&configv1alpha1.PropertiesReference{SecretRef: configv1alpha1.SecretKeySelector{Name: "cfg-nt-s-properties"}}))
			Expect(getSecret(ns, "cfg-nt-s-properties").Data[DefaultPropertiesSecretKey]).Should(Equal(before.Data[SaveInputPropertiesKey]))
		})

		It("writes the scope by the ConfigMap's namespace, whatever its label says", func() {
			By("outside vela-system every legacy word, system included, adopts as namespace")
			for _, word := range []string{"system", "global", "env", "namespace"} {
				name := "scope-" + word
				writeLegacyTemplate(ns, name, legacyTemplateCUE(name, word, false, ""))
				report, err := migrator.MigrateTemplate(ctx, ns, name, MigrationOptions{})
				Expect(err).Should(Succeed())
				Expect(report.Count(MigrationAdopted)).Should(Equal(1))
				Expect(getTemplateCR(ns, name).Spec.Scope).Should(Equal(configv1alpha1.ConfigTemplateScopeNamespace), word)
			}
			By("in vela-system every legacy word adopts as system: those templates have always been used from every namespace")
			for _, word := range []string{"namespace", "project"} {
				name := "scope-sys-" + word
				cm := writeLegacyTemplate(types.DefaultKubeVelaNS, name, legacyTemplateCUE(name, word, false, ""))
				DeferCleanup(func() { Expect(k8sClient.Delete(ctx, cm)).Should(Succeed()) }) // vela-system is shared with the factory specs' counts
				report, err := migrator.MigrateTemplate(ctx, types.DefaultKubeVelaNS, name, MigrationOptions{DryRun: true})
				Expect(err).Should(Succeed())
				Expect(report.Count(MigrationAdopted)).Should(Equal(1))
				Expect(report.Results[0].Target.(*configv1alpha1.ConfigTemplate).Spec.Scope).Should(Equal(configv1alpha1.ConfigTemplateScopeSystem), "the dry run shows the scope it would write")
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
			Expect(props.Data[DefaultPropertiesSecretKey]).Should(Equal(before.Data[SaveInputPropertiesKey]))
			Expect(props.Labels).Should(HaveKeyWithValue(types.LabelConfigCreatedBy, types.ConfigCreatedByMigration))
			Expect(controllerOf(props).UID).Should(Equal(cfg.UID))
			Expect(getSecret(ns, "cfg-s").Data).Should(Equal(before.Data))
		})

		It("marks the config Secret first and patches it last", func() {
			// The adopt-by mark goes on before the Config exists (the controller's proof, see
			// migrateConfigSecret); the Secret's own adoption, with the migrated-to annotation
			// that says the migration is complete, is still the last write.
			writeLegacyTemplate(ns, "order", legacyTemplateCUE("order", "namespace", false, ""))
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
			Expect(writes).Should(Equal([]string{"cfg-o", "cfg-o-local", "cfg-o"}))
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
			Expect(ct.Spec.Scope).Should(Equal(configv1alpha1.ConfigTemplateScopeNamespace), "the label says system; outside vela-system the namespace decides")
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

	Context("GitOps-managed legacy objects (issue #7410 section 5)", func() {
		const ns = "mig-gitops"
		BeforeEach(func() {
			ensureNamespace(ns)
		})

		// managedBy returns the field managers of obj's managedFields entries that own key.
		managedBy := func(obj metav1.Object, key string) []string {
			var managers []string
			for _, entry := range obj.GetManagedFields() {
				if entry.FieldsV1 != nil && strings.Contains(string(entry.FieldsV1.Raw), key) {
					managers = append(managers, entry.Manager)
				}
			}
			return managers
		}

		drain := func(recorder *record.FakeRecorder) []string {
			var events []string
			for len(recorder.Events) > 0 {
				events = append(events, <-recorder.Events)
			}
			return events
		}

		It("adopts a Flux-applied template and config under migration's own field manager and says so", func() {
			cm := writeLegacyTemplate(ns, "flux-t", legacyTemplateCUE("flux-t", "namespace", false, ""))
			cm.Labels[fluxKustomizationLabel] = "configs"
			Expect(k8sClient.Update(ctx, cm)).Should(Succeed())
			secret := writeLegacyConfig(ns, "flux-t", ns, "cfg-flux")
			setLabel := func(s *corev1.Secret) { s.Labels[fluxKustomizationLabel] = "configs" }
			setLabel(secret)
			Expect(k8sClient.Update(ctx, secret)).Should(Succeed())
			data := getSecret(ns, "cfg-flux").Data

			recorder := &objectRecorder{}
			report, err := migrator.MigrateTemplate(ctx, ns, "flux-t", MigrationOptions{Recorder: recorder})
			Expect(err).Should(Succeed())
			Expect(report.Count(MigrationAdopted)).Should(Equal(2))

			note := "is managed by Flux (label " + fluxKustomizationLabel + ")"
			Expect(resultFor(report, "ConfigMap", ns, "flux-t").Reason).Should(ContainSubstring("the legacy ConfigMap " + note + ": keep this ConfigTemplate where it is kept instead"))
			Expect(resultFor(report, "Secret", ns, "cfg-flux").Reason).Should(ContainSubstring("the legacy Secret " + note + ": keep this Config where it is kept instead"))
			var gitops []recordedEvent
			for _, e := range recorder.events {
				if e.reason == EventMigratedGitOpsManaged {
					gitops = append(gitops, e)
				}
			}
			Expect(gitops).Should(HaveLen(2))
			Expect(gitops[0].object).Should(BeAssignableToTypeOf(&configv1alpha1.ConfigTemplate{}), "on the new ConfigTemplate")
			Expect(gitops[0].object.GetName()).Should(Equal("flux-t"))
			Expect(gitops[0].message).Should(HavePrefix("the legacy ConfigMap " + note))
			Expect(gitops[1].object).Should(BeAssignableToTypeOf(&configv1alpha1.Config{}), "on the new Config")
			Expect(gitops[1].object.GetName()).Should(Equal("cfg-flux"))
			Expect(gitops[1].message).Should(HavePrefix("the legacy Secret " + note))

			adopted := getSecret(ns, "cfg-flux")
			Expect(adopted.Data).Should(Equal(data), "adoption leaves the data alone")
			Expect(managedBy(adopted, `"f:ownerReferences"`)).Should(ConsistOf("kubevela-config-migration"))
			Expect(managedBy(adopted, `"f:`+types.AnnotationConfigMigratedTo+`"`)).Should(ConsistOf("kubevela-config-migration"))
			Expect(managedBy(adopted, `"f:`+SaveInputPropertiesKey+`"`)).ShouldNot(ContainElement("kubevela-config-migration"), "the data stays its writer's")
			var adoptedCM corev1.ConfigMap
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), &adoptedCM)).Should(Succeed())
			Expect(managedBy(&adoptedCM, `"f:`+types.AnnotationConfigMigratedTo+`"`)).Should(ConsistOf("kubevela-config-migration"))
		})

		It("reports managed-by=Helm as Helm, and app.kubernetes.io/instance on its own as possible", func() {
			writeLegacyTemplate(ns, "inst-t", legacyTemplateCUE("inst-t", "namespace", false, ""))
			helm := writeLegacyConfig(ns, "inst-t", ns, "cfg-helm")
			helm.Labels[managedByLabel] = "Helm"
			Expect(k8sClient.Update(ctx, helm)).Should(Succeed())
			bare := writeLegacyConfig(ns, "inst-t", ns, "cfg-instance")
			bare.Labels[instanceLabel] = "configs"
			Expect(k8sClient.Update(ctx, bare)).Should(Succeed())

			report, err := migrator.MigrateTemplate(ctx, ns, "inst-t", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(resultFor(report, "Secret", ns, "cfg-helm").Reason).Should(ContainSubstring("is managed by Helm (label app.kubernetes.io/managed-by=Helm)"))
			Expect(resultFor(report, "Secret", ns, "cfg-instance").Reason).Should(ContainSubstring("may be managed by Argo CD (label tracking) or another tool (label app.kubernetes.io/instance): if so"))
		})

		It("records no MigratedGitOpsManaged Event for an object no tool marks, and none on a dry run", func() {
			writeLegacyTemplate(ns, "plain-g", legacyTemplateCUE("plain-g", "namespace", false, ""))
			writeLegacyConfig(ns, "plain-g", ns, "cfg-plain-g")
			argo := writeLegacyConfig(ns, "plain-g", ns, "cfg-argo")
			setAnnotation(argo, argoCDTrackingIDAnnotation, "configs:/Secret:mig-gitops/cfg-argo")
			Expect(k8sClient.Update(ctx, argo)).Should(Succeed())

			dry := record.NewFakeRecorder(20)
			report, err := migrator.MigrateTemplate(ctx, ns, "plain-g", MigrationOptions{DryRun: true, Recorder: dry})
			Expect(err).Should(Succeed())
			Expect(resultFor(report, "Secret", ns, "cfg-argo").Reason).Should(ContainSubstring("is managed by Argo CD (annotation argocd.argoproj.io/tracking-id)"), "the dry run's report says it")
			Expect(dry.Events).Should(BeEmpty())

			recorder := record.NewFakeRecorder(20)
			_, err = migrator.MigrateTemplate(ctx, ns, "plain-g", MigrationOptions{Recorder: recorder})
			Expect(err).Should(Succeed())
			events := drain(recorder)
			gitops := 0
			for _, e := range events {
				if strings.Contains(e, EventMigratedGitOpsManaged) {
					gitops++
					Expect(e).Should(ContainSubstring("Argo CD"))
				}
			}
			Expect(gitops).Should(Equal(1), "only cfg-argo: %v", events)
		})
	})

	Context("scope and access (issue #7410 section 6)", func() {
		const teamA = "mig-team-a"
		const teamB = "mig-team-b"
		BeforeEach(func() {
			ensureNamespace(teamA)
			ensureNamespace(teamB)
		})
		// reviewingMigrator answers every SubjectAccessReview with allowed.
		reviewingMigrator := func(allowed bool, reviews *[]authorizationv1.ResourceAttributes) *Migrator {
			watching, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
			Expect(err).Should(Succeed())
			return NewMigrator(interceptor.NewClient(watching, interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if review, ok := obj.(*authorizationv1.SubjectAccessReview); ok {
						*reviews = append(*reviews, *review.Spec.ResourceAttributes)
						review.Status.Allowed = allowed
						return nil
					}
					return c.Create(ctx, obj, opts...)
				},
			}))
		}

		It("records the bindings from other namespaces when it adopts a template, and adopts them with it", func() {
			writeLegacyTemplate(teamB, "team-tpl", legacyTemplateCUE("team-tpl", "project", false, ""))
			writeLegacyConfig(teamB, "team-tpl", teamA, "a-bound")
			writeLegacyConfig(teamB, "team-tpl", teamB, "b-own")

			report, err := migrator.MigrateTemplate(ctx, teamB, "team-tpl", MigrationOptions{})
			Expect(err).Should(Succeed())
			ct := getTemplateCR(teamB, "team-tpl")
			Expect(ct.Spec.Scope).Should(Equal(configv1alpha1.ConfigTemplateScopeNamespace))
			Expect(ct.Annotations).Should(HaveKeyWithValue(AnnotationGrandfatheredConfigs, teamA+"/a-bound"), "only the other namespace's: its own namespace needs no review")
			Expect(resultFor(report, "Secret", teamA, "a-bound").Outcome).Should(Equal(MigrationAdopted))
			Expect(getConfig(teamA, "a-bound").Spec.TemplateRef).Should(Equal(&configv1alpha1.ConfigTemplateReference{Name: "team-tpl", Namespace: teamB}))

			By("a binding made after the adoption: migration has no user to ask, so it is skipped")
			writeLegacyConfig(teamB, "team-tpl", teamA, "a-late")
			report, err = migrator.MigrateConfig(ctx, teamA, "a-late", MigrationOptions{})
			Expect(err).Should(Succeed())
			late := resultFor(report, "Secret", teamA, "a-late")
			Expect(late.Outcome).Should(Equal(MigrationSkipped))
			Expect(late.Reason).Should(ContainSubstring("is namespace-scoped"))
			Expect(late.Reason).Should(ContainSubstring("the binding was not recorded when the template was migrated"))
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: teamA, Name: "a-late"}, &configv1alpha1.Config{}))).Should(BeTrue())

			By("the same binding adopted by a user (the create-config step under the gate): reviewed for that user")
			asAlice := request.WithUser(ctx, &user.DefaultInfo{Name: "alice"})
			var reviews []authorizationv1.ResourceAttributes
			report, err = reviewingMigrator(false, &reviews).MigrateConfig(asAlice, teamA, "a-late", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(resultFor(report, "Secret", teamA, "a-late").Reason).Should(ContainSubstring("alice may not"))
			asked := authorizationv1.ResourceAttributes{Verb: "get", Group: configv1alpha1.Group, Version: configv1alpha1.Version, Resource: "configtemplates", Namespace: teamB, Name: "team-tpl"}
			Expect(reviews).Should(Equal([]authorizationv1.ResourceAttributes{asked}))
			report, err = reviewingMigrator(true, &reviews).MigrateConfig(asAlice, teamA, "a-late", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(resultFor(report, "Secret", teamA, "a-late").Outcome).Should(Equal(MigrationAdopted))
			Expect(reviews).Should(Equal([]authorizationv1.ResourceAttributes{asked, asked}), "the allowed run asked the same question")
		})

		It("skips a legacy template whose nacos block names an endpoint in another namespace", func() {
			cue := strings.Replace(legacyTemplateCUE("far-nacos", "project", false, ""), "template: {", `template: {
	nacos: { endpoint: { name: "nacos", namespace: "mig-team-a" }, format: "json", metadata: { dataId: "d" }, content: {} }`, 1)
			Expect(cue).Should(ContainSubstring("nacos:"), "the fixture gained a nacos block")
			writeLegacyTemplate(teamB, "far-nacos", cue)
			report, err := migrator.MigrateTemplate(ctx, teamB, "far-nacos", MigrationOptions{})
			Expect(err).Should(Succeed())
			res := resultFor(report, "ConfigMap", teamB, "far-nacos")
			Expect(res.Outcome).Should(Equal(MigrationSkipped))
			Expect(res.Reason).Should(ContainSubstring("names the nacos endpoint mig-team-a/nacos in another namespace"))
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: teamB, Name: "far-nacos"}, &configv1alpha1.ConfigTemplate{}))).Should(BeTrue())
		})

		It("retries a Config the webhook refuses while its template is pending", func() {
			saved := pendingBackoff
			pendingBackoff = wait.Backoff{Duration: 10 * time.Millisecond, Factor: 1, Steps: 5}
			DeferCleanup(func() { pendingBackoff = saved })
			refusals := 2
			watching, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
			Expect(err).Should(Succeed())
			flaky := NewMigrator(interceptor.NewClient(watching, interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if _, ok := obj.(*configv1alpha1.Config); ok && refusals > 0 {
						refusals--
						return apierrors.NewForbidden(schema.GroupResource{Group: configv1alpha1.Group, Resource: "configs"}, obj.GetName(),
							PendingTemplateError(nil, teamB, "team-tpl", teamA))
					}
					return c.Create(ctx, obj, opts...)
				},
			}))
			writeLegacyTemplate(teamB, "team-tpl", legacyTemplateCUE("team-tpl", "project", false, ""))
			writeLegacyConfig(teamB, "team-tpl", teamA, "a-pending")
			report, err := flaky.MigrateTemplate(ctx, teamB, "team-tpl", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(resultFor(report, "Secret", teamA, "a-pending").Outcome).Should(Equal(MigrationAdopted))
			Expect(refusals).Should(BeZero())
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
			report, err := NewMigrator(forbiddenClusterList(nil)).MigrateTemplate(ctx, sys, "denied", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(resultFor(report, "Secret", sys, "sys-denied").Outcome).Should(Equal(MigrationAdopted))
			// #7449 review item 6: one skipped row for what could not be seen, no failure
			star := resultFor(report, "Secret", "*", "*")
			Expect(star.Outcome).Should(Equal(MigrationSkipped))
			Expect(star.Reason).Should(HavePrefix("configs in other namespaces were not listed: "))
			Expect(star.Reason).Should(ContainSubstring("no cluster-wide list"))
			Expect(report.HasFailure()).Should(BeFalse())
			Expect(report.Results[len(report.Results)-1]).Should(Equal(star))
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: teamA, Name: "a-denied"}, &configv1alpha1.Config{}))).Should(BeTrue())
		})

		// Only a Forbidden list is a visibility limit. Anything else is this run
		// failing, and it is returned before a single object is written.
		It("returns a transient cluster-wide list error instead of reporting unlisted namespaces", func() {
			cm := writeLegacyTemplate(sys, "flaky", legacyTemplateCUE("flaky", "system", false, ""))
			writeLegacyConfig(sys, "flaky", sys, "sys-flaky")
			flaky := NewMigrator(failingClusterList())
			report, err := flaky.MigrateTemplate(ctx, sys, "flaky", MigrationOptions{})
			Expect(err).Should(MatchError(ContainSubstring("request timed out")))
			Expect(report.Results).Should(BeEmpty())
			var after corev1.ConfigMap
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), &after)).Should(Succeed())
			Expect(after.Annotations).ShouldNot(HaveKey(types.AnnotationConfigMigratedTo))
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: sys, Name: "flaky"}, &configv1alpha1.ConfigTemplate{}))).Should(BeTrue())
			report, err = flaky.MigrateNamespace(ctx, sys, MigrationOptions{})
			Expect(err).Should(MatchError(ContainSubstring("request timed out")))
			Expect(report.Results).Should(BeEmpty())
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

		It("moves the Nacos endpoint marker from the legacy Secret to the Config", func() {
			writeLegacyTemplate(ns, "ep-tpl", legacyTemplateCUE("ep-tpl", "namespace", true, ""))
			marked := writeLegacyConfig(ns, "ep-tpl", ns, "cfg-ep-marked")
			setAnnotation(marked, types.AnnotationConfigNacosEndpoint, NacosEndpointAnnotationValue)
			Expect(k8sClient.Update(ctx, marked)).Should(Succeed())
			writeLegacyConfig(ns, "ep-tpl", ns, "cfg-ep-plain")

			for _, name := range []string{"cfg-ep-marked", "cfg-ep-plain"} {
				report, err := migrator.MigrateConfig(ctx, ns, name, MigrationOptions{})
				Expect(err).Should(Succeed())
				Expect(report.Results[0].Outcome).Should(Equal(MigrationAdopted))
			}
			Expect(NacosEndpointMarked(getConfig(ns, "cfg-ep-marked"))).Should(BeTrue())
			Expect(NacosEndpointMarked(getConfig(ns, "cfg-ep-plain"))).Should(BeFalse())
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
				Data:       map[string][]byte{DefaultPropertiesSecretKey: []byte(`{"value":"somebody else's"}`)},
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
				Data:       map[string][]byte{DefaultPropertiesSecretKey: before.Data[SaveInputPropertiesKey]},
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

		// #7449 review item 4: what is not a legacy config is settled before the Config
		// lookup, so a Config of the same name is no conflict.
		It("skips a Secret a Config wrote for itself, and a source cache entry with a Config, before any lookup", func() {
			fresh := &configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "fresh"}, Spec: configv1alpha1.ConfigSpec{Alias: "mine"}}
			Expect(k8sClient.Create(ctx, fresh)).Should(Succeed())
			written := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "fresh", Labels: map[string]string{types.LabelConfigCatalog: types.VelaCoreConfig, types.LabelConfigType: "x"}},
				Data:       map[string][]byte{"value": []byte("v")},
			}
			Expect(controllerutil.SetControllerReference(fresh, written, k8sClient.Scheme())).Should(Succeed())
			Expect(k8sClient.Create(ctx, written)).Should(Succeed())

			report, err := migrator.MigrateConfig(ctx, ns, "fresh", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Results[0].Outcome).Should(Equal(MigrationSkipped))
			Expect(report.Results[0].Reason).Should(Equal("not a legacy config: written by Config fresh"))
			Expect(report.HasConflict()).Should(BeFalse())
			report, err = migrator.MigrateConfigs(ctx, ns, MigrationOptions{})
			Expect(err).Should(Succeed())
			for _, res := range report.Results {
				Expect(res.Name).ShouldNot(Equal("fresh"))
			}
			Expect(getConfig(ns, "fresh").Spec.Alias).Should(Equal("mine"))

			By("a source cache entry whose Config the application controller already declared")
			writeLegacyTemplate(ns, "cache2", legacyTemplateCUE("cache2", "namespace", false, ""))
			entry := writeLegacyConfig(ns, "cache2", ns, "cache2-entry")
			entry.Labels[types.LabelSourceDefinitionName] = "some-source"
			Expect(k8sClient.Update(ctx, entry)).Should(Succeed())
			Expect(k8sClient.Create(ctx, &configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "cache2-entry"}})).Should(Succeed())
			report, err = migrator.MigrateConfig(ctx, ns, "cache2-entry", MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Results[0].Outcome).Should(Equal(MigrationSkipped))
			Expect(report.Results[0].Reason).Should(Equal("source cache entry managed by the controller"))
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

		// #7449 review item 6: one Secret list per run, and a namespace-scoped user
		// gets one skipped row for the namespaces it cannot see, not a failed row per
		// template.
		It("lists configs once and reports the namespaces it could not see as one skipped row", func() {
			ensureNamespace(ns)
			ensureNamespace("mig-team-a")
			writeLegacyTemplate(ns, "ns-c", legacyTemplateCUE("ns-c", "namespace", false, ""))
			writeLegacyTemplate(ns, "ns-d", legacyTemplateCUE("ns-d", "namespace", false, ""))
			writeLegacyConfig(ns, "ns-c", ns, "c-cfg")
			writeLegacyConfig(ns, "ns-c", "mig-team-a", "a-from-c")

			secretLists := 0
			report, err := NewMigrator(forbiddenClusterList(&secretLists)).MigrateNamespace(ctx, ns, MigrationOptions{})
			Expect(err).Should(Succeed())
			var names []string
			for _, res := range report.Results {
				names = append(names, res.Kind+"/"+res.Name)
			}
			Expect(strings.Join(names, " ")).Should(Equal("ConfigMap/ns-c Secret/c-cfg ConfigMap/ns-d Secret/*"))
			star := report.Results[3]
			Expect(star.Outcome).Should(Equal(MigrationSkipped))
			Expect(star.Reason).Should(HavePrefix("configs in other namespaces were not listed: "))
			Expect(report.HasFailure()).Should(BeFalse())
			Expect(report.Count(MigrationAdopted)).Should(Equal(3))
			Expect(secretLists).Should(Equal(2), "the refused cluster-wide list and the namespace fallback")
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: "mig-team-a", Name: "a-from-c"}, &configv1alpha1.Config{}))).Should(BeTrue())
		})
	})

	Context("the whole cluster", func() {
		const one = "mig-all-1"
		const two = "mig-all-2"
		sys := types.DefaultKubeVelaNS

		// scopedClient keeps MigrateAll inside this test's namespaces: the suite's
		// other files have legacy objects of their own in the cluster, which a real
		// cluster-wide run would adopt. listCalls counts list calls by list type.
		scopedClient := func(listCalls map[string]int) client.Client {
			watchClient, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
			Expect(err).Should(Succeed())
			ours := func(ns, name string) bool {
				return ns == one || ns == two || (ns == sys && name == TemplateConfigMapNamePrefix+"ca-sys")
			}
			return interceptor.NewClient(watchClient, interceptor.Funcs{
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					listCalls[fmt.Sprintf("%T", list)]++
					if err := c.List(ctx, list, opts...); err != nil {
						return err
					}
					switch l := list.(type) {
					case *corev1.ConfigMapList:
						kept := l.Items[:0]
						for _, item := range l.Items {
							if ours(item.Namespace, item.Name) {
								kept = append(kept, item)
							}
						}
						l.Items = kept
					case *corev1.SecretList:
						kept := l.Items[:0]
						for _, item := range l.Items {
							if ours(item.Namespace, item.Name) {
								kept = append(kept, item)
							}
						}
						l.Items = kept
					}
					return nil
				},
			})
		}

		It("adopts every template and config once, templates first, with one list of each kind", func() {
			ensureNamespace(one)
			ensureNamespace(two)
			writeLegacyTemplate(sys, "ca-sys", legacyTemplateCUE("ca-sys", "system", false, ""))
			writeLegacyConfig(sys, "ca-sys", one, "one-sys")
			writeLegacyConfig(sys, "ca-sys", two, "two-sys")
			writeLegacyTemplate(two, "ca-t", legacyTemplateCUE("ca-t", "namespace", false, ""))
			writeLegacyConfig(two, "ca-t", two, "two-t")
			skip := writeLegacyConfig(two, "ca-t", two, "two-skip")
			setAnnotation(skip, types.AnnotationConfigMigrate, "false")
			Expect(k8sClient.Update(ctx, skip)).Should(Succeed())
			cache := writeLegacyConfig(two, "ca-t", one, "one-cache")
			cache.Labels[types.LabelSourceDefinitionName] = "some-source"
			Expect(k8sClient.Update(ctx, cache)).Should(Succeed())
			noTpl, err := fac.ParseConfig(ctx, NamespacedName{}, Metadata{NamespacedName: NamespacedName{Namespace: one, Name: "one-none"}})
			Expect(err).Should(Succeed())
			Expect(fac.CreateOrUpdateConfig(ctx, noTpl, one)).Should(Succeed())

			listCalls := map[string]int{}
			report, err := NewMigrator(scopedClient(listCalls)).MigrateAll(ctx, MigrationOptions{})
			Expect(err).Should(Succeed())
			var names []string
			for _, res := range report.Results {
				names = append(names, res.Kind+"/"+res.Name)
			}
			// templates in namespace then name order, each with its configs wherever they
			// live; then what is left; the source cache entry never appears
			Expect(strings.Join(names, " ")).Should(Equal("ConfigMap/ca-t Secret/two-skip Secret/two-t ConfigMap/ca-sys Secret/one-sys Secret/two-sys Secret/one-none"))
			Expect(resultFor(report, "Secret", two, "two-skip").Reason).Should(Equal("opted out"))
			Expect(report.Count(MigrationAdopted)).Should(Equal(6))
			Expect(report.Count(MigrationSkipped)).Should(Equal(1))
			Expect(listCalls).Should(Equal(map[string]int{"*v1.ConfigMapList": 1, "*v1.SecretList": 1}))
			Expect(getConfig(one, "one-sys").Spec.TemplateRef).Should(Equal(&configv1alpha1.ConfigTemplateReference{Name: "ca-sys", Namespace: sys}))
			Expect(getConfig(one, "one-none").Spec.TemplateRef).Should(BeNil())
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: one, Name: "one-cache"}, &configv1alpha1.Config{}))).Should(BeTrue())

			By("a second run skips everything and lists once more")
			listCalls = map[string]int{}
			report, err = NewMigrator(scopedClient(listCalls)).MigrateAll(ctx, MigrationOptions{})
			Expect(err).Should(Succeed())
			Expect(report.Results).Should(HaveLen(7))
			Expect(report.Count(MigrationSkipped)).Should(Equal(7))
			Expect(listCalls).Should(Equal(map[string]int{"*v1.ConfigMapList": 1, "*v1.SecretList": 1}))
		})

		It("returns the error of a refused cluster-wide list instead of a partial report", func() {
			_, err := NewMigrator(forbiddenClusterList(nil)).MigrateAll(ctx, MigrationOptions{})
			Expect(apierrors.IsForbidden(err)).Should(BeTrue())
		})
	})

	It("leaves a cluster-scoped output alone and rejects a malformed reference key", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "cluster-out"},
			Data:       map[string][]byte{SaveObjectReferenceKey: []byte(`[{"kind":"ClusterRole","apiVersion":"rbac.authorization.k8s.io/v1","name":"x"}]`)},
		}
		refs, err := ObjectReferences(secret)
		Expect(err).Should(Succeed())
		Expect(outputsOutside(refs, "default")).Should(Equal([]string{"ClusterRole /x"}))
		refs, err = ObjectReferences(&corev1.Secret{})
		Expect(err).Should(Succeed())
		Expect(refs).Should(BeNil())
		_, err = ObjectReferences(&corev1.Secret{Data: map[string][]byte{SaveObjectReferenceKey: []byte(`{not json`)}})
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

// objectRecorder keeps each Event with the object it is recorded on; record.FakeRecorder
// names the object by its TypeMeta, which a typed object read back from the API server lacks.
type objectRecorder struct {
	events []recordedEvent
}

type recordedEvent struct {
	reason, message string
	object          client.Object
}

func (r *objectRecorder) Event(object runtime.Object, _, reason, message string) {
	r.events = append(r.events, recordedEvent{reason, message, object.(client.Object)})
}

func (r *objectRecorder) Eventf(object runtime.Object, eventType, reason, messageFmt string, args ...interface{}) {
	r.Event(object, eventType, reason, fmt.Sprintf(messageFmt, args...))
}

func (r *objectRecorder) AnnotatedEventf(object runtime.Object, _ map[string]string, eventType, reason, messageFmt string, args ...interface{}) {
	r.Eventf(object, eventType, reason, messageFmt, args...)
}
