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
	"encoding/base64"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/config"
)

const (
	gitOpsFieldManager = "kustomize-controller"
	gitOpsSettleTime   = 15 * time.Second
)

// What a GitOps tool does with these objects, played with server-side apply under Flux's field
// manager and labels: applying, comparing on every sync, pruning. The job has no real tool; these
// are properties of field ownership and owner references.
var _ = Describe("Config objects under a GitOps tool", Label("core-config", "gitops"), func() {
	ctx := context.Background()

	var namespace string

	BeforeEach(func() {
		namespace = randomNamespaceName("config-gitops-e2e")
		ns := createNamespace(ctx, namespace)
		DeferCleanup(func() {
			_ = k8sClient.DeleteAllOf(ctx, &configv1alpha1.Config{}, client.InNamespace(namespace))
			_ = k8sClient.DeleteAllOf(ctx, &configv1alpha1.ConfigTemplate{}, client.InNamespace(namespace))
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground)))).To(Succeed())
		})
	})

	manifest := func(apiVersion, kind, name string, fields map[string]interface{}) *unstructured.Unstructured {
		object := map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels":    map[string]interface{}{"kustomize.toolkit.fluxcd.io/name": "configs"},
			},
		}
		for k, v := range fields {
			object[k] = v
		}
		return &unstructured.Unstructured{Object: object}
	}

	configTemplate := func(name, template string) *unstructured.Unstructured {
		return manifest(configv1alpha1.SchemeGroupVersion.String(), configv1alpha1.ConfigTemplateKind, name,
			map[string]interface{}{"spec": map[string]interface{}{"template": template}})
	}

	configCR := func(name string, spec map[string]interface{}) *unstructured.Unstructured {
		return manifest(configv1alpha1.SchemeGroupVersion.String(), configv1alpha1.ConfigKind, name,
			map[string]interface{}{"spec": spec})
	}

	apply := func(obj *unstructured.Unstructured) {
		// the Config webhook reads a template through the manager's cache, which may not have
		// seen one applied a moment ago: a sync retries the same way
		Eventually(func() error {
			return k8sClient.Patch(ctx, obj.DeepCopy(), client.Apply, client.FieldOwner(gitOpsFieldManager), client.ForceOwnership)
		}, configE2ETimeout, configE2EPollInterval).Should(Succeed())
	}

	// withoutVolatile is what a sync compares: everything but the status and the resource version.
	withoutVolatile := func(obj *unstructured.Unstructured) map[string]interface{} {
		object := obj.DeepCopy().Object
		delete(object, "status")
		unstructured.RemoveNestedField(object, "metadata", "resourceVersion")
		return object
	}

	// expectNoDiff holds for a while what a tool checks on every sync: applying the manifest again,
	// without forcing, conflicts with nobody and changes nothing, fields and their owners alike.
	expectNoDiff := func(manifest *unstructured.Unstructured) {
		Consistently(func(g Gomega) {
			live := &unstructured.Unstructured{}
			live.SetGroupVersionKind(manifest.GroupVersionKind())
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(manifest), live)).To(Succeed())
			dry := manifest.DeepCopy()
			g.Expect(k8sClient.Patch(ctx, dry, client.Apply, client.FieldOwner(gitOpsFieldManager), client.DryRunAll)).To(Succeed())
			g.Expect(withoutVolatile(dry)).To(Equal(withoutVolatile(live)))
		}, gitOpsSettleTime, 3*time.Second).Should(Succeed(), "%s %s", manifest.GetKind(), manifest.GetName())
	}

	expectPhase := func(name string, phase configv1alpha1.ConfigPhase) {
		Eventually(func() configv1alpha1.ConfigPhase {
			var cfg configv1alpha1.Config
			_ = k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &cfg)
			return cfg.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(phase), "Config %s", name)
	}

	expectTemplateAvailable := func(name string) {
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			var ct configv1alpha1.ConfigTemplate
			_ = k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &ct)
			return ct.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable), "ConfigTemplate %s", name)
	}

	expectGone := func(obj client.Object, name string) {
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, obj))
		}, configE2ETimeout, configE2EPollInterval).Should(BeTrue(), "%T %s", obj, name)
	}

	It("converges, shows no diff, and pruning removes everything", func() {
		template := configTemplate("ct-outputs", configE2EOutputsTemplate)
		cfg := configCR("cfg-outputs", map[string]interface{}{
			"templateRef": map[string]interface{}{"name": "ct-outputs", "namespace": namespace},
			"properties":  map[string]interface{}{"username": "admin"},
		})
		apply(template)
		expectTemplateAvailable("ct-outputs")
		apply(cfg)
		expectPhase("cfg-outputs", configv1alpha1.ConfigPhaseAvailable)
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "cfg-outputs"}, &corev1.Secret{})).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "cfg-outputs-extra"}, &corev1.ConfigMap{})).To(Succeed())

		expectNoDiff(template)
		expectNoDiff(cfg)

		By("pruning what was applied")
		Expect(k8sClient.Delete(ctx, cfg.DeepCopy())).To(Succeed())
		Expect(k8sClient.Delete(ctx, template.DeepCopy())).To(Succeed())
		expectGone(&configv1alpha1.Config{}, "cfg-outputs")
		expectGone(&configv1alpha1.ConfigTemplate{}, "ct-outputs")
		expectGone(&corev1.Secret{}, "cfg-outputs")
		expectGone(&corev1.ConfigMap{}, "cfg-outputs-extra")
	})

	It("syncs a template with a Nacos block and shows no diff", func() {
		addr, podHost := os.Getenv("NACOS_ADDR"), os.Getenv("NACOS_POD_HOST")
		if addr == "" || podHost == "" {
			Skip("NACOS_ADDR and NACOS_POD_HOST are not set")
		}
		endpoint := configCR("nacos-plain", map[string]interface{}{
			"properties": map[string]interface{}{
				"servers": []interface{}{map[string]interface{}{"ipAddr": podHost, "port": int64(8848)}},
			},
		})
		template := configTemplate("ct-nacos", nacosE2ETemplate("nacos-plain", namespace))
		cfg := configCR("cfg-db", map[string]interface{}{
			"templateRef": map[string]interface{}{"name": "ct-nacos", "namespace": namespace},
			"properties": map[string]interface{}{
				"dataId":  namespace + "-db",
				"group":   "E2E_GROUP",
				"content": map[string]interface{}{"mysqlHost": "db", "mysqlPort": int64(3306)},
			},
		})
		apply(endpoint)
		apply(template)
		expectTemplateAvailable("ct-nacos")
		apply(cfg)
		Eventually(func() string {
			var got configv1alpha1.Config
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &got); err != nil {
				return err.Error()
			}
			c := got.Status.GetCondition(configv1alpha1.ConditionNacosSynced)
			return string(c.Status) + "/" + string(c.Reason)
		}, nacosE2ETimeout, configE2EPollInterval).Should(HavePrefix("True/"))
		Eventually(func() string { return nacosE2EContent(addr, namespace+"-db", "E2E_GROUP") }, configE2ETimeout, configE2EPollInterval).
			Should(Equal("mysqlHost = db\nmysqlPort = 3306\n"))

		expectNoDiff(endpoint)
		expectNoDiff(template)
		expectNoDiff(cfg)
	})

	It("shows no diff on a legacy Secret after migration adopts it", func() {
		factory := config.NewConfigFactory(k8sClient)
		parsed, err := factory.ParseTemplate(ctx, "lg-gitops", []byte(`
metadata: {
	name:      "lg-gitops"
	scope:     "namespace"
	sensitive: false
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
}
`))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(factory.CreateOrUpdateConfigTemplate(ctx, namespace, parsed)).To(Succeed())
		item, err := factory.ParseConfig(ctx, config.NamespacedName{Namespace: namespace, Name: "lg-gitops"}, config.Metadata{
			NamespacedName: config.NamespacedName{Namespace: namespace, Name: "cfg-legacy"},
			Properties:     map[string]interface{}{"value": "from-git"},
		})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(factory.CreateOrUpdateConfig(ctx, item, namespace)).To(Succeed())

		By("the tool, not the CLI, owning the Secret: its manifest is what the CLI wrote")
		var written corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "cfg-legacy"}, &written)).To(Succeed())
		encoded := map[string]string{}
		for k, v := range written.Data {
			encoded[k] = base64.StdEncoding.EncodeToString(v)
		}
		secretManifest := manifest("v1", "Secret", "cfg-legacy", map[string]interface{}{"type": string(written.Type)})
		Expect(unstructured.SetNestedStringMap(secretManifest.Object, encoded, "data")).To(Succeed())
		for k, v := range written.Labels {
			Expect(unstructured.SetNestedField(secretManifest.Object, v, "metadata", "labels", k)).To(Succeed())
		}
		annotations := map[string]string{}
		for k, v := range written.Annotations {
			annotations[k] = v
		}
		Expect(unstructured.SetNestedStringMap(secretManifest.Object, annotations, "metadata", "annotations")).To(Succeed())
		Expect(k8sClient.Delete(ctx, &written)).To(Succeed())
		expectGone(&corev1.Secret{}, "cfg-legacy")
		apply(secretManifest)

		report, err := config.NewMigrator(k8sClient).MigrateTemplate(ctx, namespace, "lg-gitops", config.MigrationOptions{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(report.Count(config.MigrationAdopted)).To(Equal(2), "%+v", report)
		expectPhase("cfg-legacy", configv1alpha1.ConfigPhaseAvailable)

		var adopted corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "cfg-legacy"}, &adopted)).To(Succeed())
		Expect(adopted.Data).To(Equal(written.Data))
		expectNoDiff(secretManifest)
	})
})
