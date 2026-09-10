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
	"fmt"
	"sort"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcomm "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var _ = Describe("SourceDefinition e2e", func() {
	ctx := context.Background()

	var namespaceName string
	var ns corev1.Namespace

	BeforeEach(func() {
		namespaceName = randomNamespaceName("source-definition-e2e")
		ns = createNamespace(ctx, namespaceName)
	})

	AfterEach(func() {
		// Delete the test namespace FIRST so its Applications and SourceDefinitions
		// are gone before we clean vela-system. Otherwise a still-running
		// Application would re-write its source cache entry on the next reconcile,
		// racing (and defeating) the vela-system cleanup below.
		By("Clean up source-definition e2e namespace")
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(BeNil())

		// SourceDefinitions live in the test namespace, but the ConfigTemplates
		// (config-template-source-* ConfigMaps) and source cache entries
		// (source-cache-* Secrets) they generate live in vela-system and are only
		// reclaimed asynchronously by the periodic GC sweep. Delete everything
		// stamped with this test's namespace via the owning-SourceDefinition label
		// so runs do not leak into vela-system. Retry: the label-carrying objects
		// may still be settling as the namespace tears down.
		By("Clean up ConfigTemplates and source cache entries created in vela-system")
		nsLabel := client.MatchingLabels{"sourcedefinition.oam.dev/namespace": namespaceName}
		Eventually(func() error {
			if err := k8sClient.DeleteAllOf(ctx, &corev1.ConfigMap{},
				client.InNamespace("vela-system"), nsLabel); err != nil {
				return err
			}
			if err := k8sClient.DeleteAllOf(ctx, &corev1.Secret{},
				client.InNamespace("vela-system"), nsLabel); err != nil {
				return err
			}
			// Confirm nothing labelled for this namespace remains.
			var cms corev1.ConfigMapList
			if err := k8sClient.List(ctx, &cms, client.InNamespace("vela-system"), nsLabel); err != nil {
				return err
			}
			var secrets corev1.SecretList
			if err := k8sClient.List(ctx, &secrets, client.InNamespace("vela-system"), nsLabel); err != nil {
				return err
			}
			if len(cms.Items)+len(secrets.Items) > 0 {
				return fmt.Errorf("still %d configmaps and %d secrets labelled for %s in vela-system",
					len(cms.Items), len(secrets.Items), namespaceName)
			}
			return nil
		}, 30*time.Second, time.Second).Should(Succeed())
	})

	It("resolves a source read and reconciles updated source properties", func() {
		sourceDef := &v1beta1.SourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "image-source",
				Namespace: namespaceName,
			},
			Spec: v1beta1.SourceDefinitionSpec{
				Schematic: &oamcomm.Schematic{
					CUE: &oamcomm.CUE{
						Template: `
schema: {
  image: string
}
// Generated: this source reads no context, so the key is bare. The image is a
// property, and properties are hashed into the cache identity by the resolver -
// which is why a value containing ':' and '.' needs no normalising here.
$internal: {
  key: "image-source"
  keyInputs: []
}
output: {
  // +sensitive
  image: parameter.image
}
parameter: {
  image: string
}
`,
					},
				},
			},
		}
		applyDefinition(ctx, sourceDef)

		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "source-image-app",
				Namespace: namespaceName,
			},
			Spec: v1beta1.ApplicationSpec{
				Sources: []v1beta1.ApplicationSource{
					{
						Name: "img",
						Type: "image-source",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"image":"nginx:1.25.0"}`),
						},
						StatusPolicy: &v1beta1.ApplicationSourceStatusPolicy{
							ExposeConsumedValues: ptr.To(true),
						},
					},
				},
				Components: []oamcomm.ApplicationComponent{
					{
						Name: "web",
						Type: "webservice",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{
  "image":"$(source.img.image)",
  "port":80
}`),
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, optIn(app))).Should(Succeed())

		verifyApplicationPhase(ctx, namespaceName, app.Name, oamcomm.ApplicationRunning)
		Eventually(func() (string, error) {
			deploy := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "web"}, deploy); err != nil {
				return "", err
			}
			if len(deploy.Spec.Template.Spec.Containers) == 0 {
				return "", fmt.Errorf("deployment has no containers")
			}
			return deploy.Spec.Template.Spec.Containers[0].Image, nil
		}, 90*time.Second, time.Second).Should(Equal("nginx:1.25.0"))

		Eventually(func() error {
			latest := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), latest); err != nil {
				return err
			}
			latest.Spec.Sources[0].Properties = &runtime.RawExtension{
				Raw: []byte(`{"image":"nginx:1.25.1"}`),
			}
			return k8sClient.Update(ctx, latest)
		}, 20*time.Second, time.Second).Should(Succeed())

		Eventually(func() (string, error) {
			deploy := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "web"}, deploy); err != nil {
				return "", err
			}
			if len(deploy.Spec.Template.Spec.Containers) == 0 {
				return "", fmt.Errorf("deployment has no containers")
			}
			return deploy.Spec.Template.Spec.Containers[0].Image, nil
		}, 90*time.Second, time.Second).Should(Equal("nginx:1.25.1"))

		Eventually(func() (string, error) {
			latest := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), latest); err != nil {
				return "", err
			}
			return consumedValue(latest, "img", "image")
		}, 60*time.Second, time.Second).Should(Equal("***"))
	})

	// A resource whose name derives from source data is renamed when that value
	// changes, and a rename dispatches a second object rather than updating the
	// first. Garbage collection cannot reap the original: it recycles whole
	// ResourceTrackers, and a tracker retires only when a new ApplicationRevision
	// supersedes it. A source refresh mints none, so without an explicit prune the
	// old object stays tracked and running.
	//
	// The value therefore has to move WITHOUT touching the Application. Editing
	// spec.sources[].properties would be a spec change, which mints a revision and
	// lets ordinary GC do the reaping - a spec written that way passes whether the
	// prune works or not. Here the source reads a ConfigMap and the ConfigMap is
	// edited instead, so the Application and its definitions are untouched.
	It("reaps a resource the component no longer renders after a source change", func() {
		naming := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "naming-input", Namespace: namespaceName},
			Data:       map[string]string{"suffix": "one"},
		}
		Expect(k8sClient.Create(ctx, naming)).Should(Succeed())

		Expect(k8sClient.Create(ctx, &v1beta1.SourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "naming-source", Namespace: namespaceName},
			Spec: v1beta1.SourceDefinitionSpec{
				Schematic: &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: `
import "vela/kube"

schema: {
  data: [string]: string
}
$internal: {
  key: "naming-source-\(context.cluster)-\(context.namespace)"
  keyInputs: ["cluster", "namespace"]
}
storage: {
  // Short, so the edit below is noticed inside the spec's patience rather than
  // at the default TTL.
  storageTTL: "5s"
}
_cm: kube.#Get & {
  $params: {
    cluster: context.cluster
    resource: {
      apiVersion: "v1"
      kind:       "ConfigMap"
      metadata: {
        name:      parameter.name
        namespace: context.namespace
      }
    }
  }
}
output: data: _cm.$returns.data
parameter: {
  name: string
}
`}},
			},
		})).Should(Succeed())

		// The component's resource name comes from a parameter, so a source value
		// reaches metadata.name - the case that renames rather than updates.
		Expect(k8sClient.Create(ctx, exprComponentDefinition(namespaceName, "named-cm", `
parameter: {name: string}
output: {
  apiVersion: "v1"
  kind:       "ConfigMap"
  metadata: name: parameter.name
  data: {marker: "from-source"}
}
`))).Should(Succeed())

		autoUpdate := true
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "source-prune-app", Namespace: namespaceName},
			Spec: v1beta1.ApplicationSpec{
				Sources: []v1beta1.ApplicationSource{{
					Name:       "naming",
					Type:       "naming-source",
					AutoUpdate: &autoUpdate,
					Properties: &runtime.RawExtension{Raw: []byte(`{"name":"naming-input"}`)},
				}},
				Components: []oamcomm.ApplicationComponent{
					{
						Name:       "renamed",
						Type:       "named-cm",
						Properties: &runtime.RawExtension{Raw: []byte(`{"name":"cm-$(has(source.naming.data.suffix) ? source.naming.data.suffix : \"none\")"}`)},
					},
					// A sibling that reads nothing. Pruning is attributed per
					// component, so this must survive untouched.
					{
						Name:       "bystander",
						Type:       "named-cm",
						Properties: &runtime.RawExtension{Raw: []byte(`{"name":"cm-bystander"}`)},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, optIn(app))).Should(Succeed())
		verifyApplicationPhase(ctx, namespaceName, app.Name, oamcomm.ApplicationRunning)

		cmExists := func(name string) func() bool {
			return func() bool {
				cm := &corev1.ConfigMap{}
				return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: name}, cm) == nil
			}
		}
		Eventually(cmExists("cm-one"), 90*time.Second, time.Second).Should(BeTrue())
		Eventually(cmExists("cm-bystander"), 90*time.Second, time.Second).Should(BeTrue())

		revisionBefore := func() string {
			latest := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), latest); err != nil {
				return ""
			}
			if latest.Status.LatestRevision == nil {
				return ""
			}
			return latest.Status.LatestRevision.Name
		}()
		Expect(revisionBefore).ShouldNot(BeEmpty())

		// Move the value the source reads. Nothing about the Application changes.
		Eventually(func() error {
			live := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(naming), live); err != nil {
				return err
			}
			live.Data["suffix"] = "two"
			return k8sClient.Update(ctx, live)
		}, 20*time.Second, time.Second).Should(Succeed())

		Eventually(cmExists("cm-two"), 180*time.Second, 2*time.Second).Should(BeTrue(),
			"autoUpdate must re-dispatch the component under its new name")

		// The point of the spec: the object the component stopped rendering is
		// gone, and no new revision retired the tracker that held it.
		Eventually(cmExists("cm-one"), 180*time.Second, 2*time.Second).Should(BeFalse(),
			"the renamed-away ConfigMap must be reaped by the prune, not left running")
		Expect(revisionBefore).Should(Equal(func() string {
			latest := &v1beta1.Application{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(app), latest)).Should(Succeed())
			return latest.Status.LatestRevision.Name
		}()), "no new ApplicationRevision: the reaping must be the prune, not ordinary GC")

		// ...and pruning stayed inside the component that changed.
		Consistently(cmExists("cm-bystander"), 10*time.Second, 2*time.Second).Should(BeTrue(),
			"a sibling component's resource must survive a prune")
	})

	// A component's own source kept auto-updating when one of its traits read a
	// different source.
	//
	// A component and every one of its traits render against a single
	// process.Context, one pass each, and each pass built its status map from
	// scratch before pushing it. The push replaces, so the trait's pass discarded
	// the component's record. Nothing looked wrong: the values still substituted,
	// so the first render was correct. But resolvedSourceHashes stamps a
	// resolved-hash only for the bindings it finds in that final map, so the
	// component's own source lost its hash and stopped re-dispatching.
	//
	// The unit test pins the map. This pins what the map is for - a value moving
	// in the cluster, and the workload following it - which is the only thing that
	// proves the hash reached the dispatched object.
	It("keeps a component auto-updating when its trait reads a different source", func() {
		compInput := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "trait-case-comp-input", Namespace: namespaceName},
			Data:       map[string]string{"value": "first"},
		}
		Expect(k8sClient.Create(ctx, compInput)).Should(Succeed())

		traitInput := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "trait-case-trait-input", Namespace: namespaceName},
			Data:       map[string]string{"value": "trait-value"},
		}
		Expect(k8sClient.Create(ctx, traitInput)).Should(Succeed())

		// One definition, two bindings of it. The binding name is in the cache
		// key, so they resolve independently.
		Expect(k8sClient.Create(ctx, &v1beta1.SourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "trait-case-source", Namespace: namespaceName},
			Spec: v1beta1.SourceDefinitionSpec{
				Schematic: &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: `
import "vela/kube"

schema: {
  data: [string]: string
}
$internal: {
  key: "trait-case-source-\(context.cluster)-\(context.namespace)"
  keyInputs: ["cluster", "namespace"]
}
storage: storageTTL: "1s"
parameter: {
  name: string
}
_cm: kube.#Get & {
  $params: {
    cluster: context.cluster
    resource: {
      apiVersion: "v1"
      kind:       "ConfigMap"
      metadata: {
        name:      parameter.name
        namespace: context.namespace
      }
    }
  }
}
output: data: _cm.$returns.data
`}},
			},
		})).Should(Succeed())

		Expect(k8sClient.Create(ctx, exprComponentDefinition(namespaceName, "trait-case-cm", `
parameter: {value: string}
output: {
  apiVersion: "v1"
  kind:       "ConfigMap"
  metadata: name: "trait-case-result"
  data: {value: parameter.value}
}
`))).Should(Succeed())

		// The trait reads the OTHER source. That is the whole condition: without
		// it the component's statuses were never overwritten.
		Expect(k8sClient.Create(ctx, exprTraitDefinition(namespaceName, "trait-case-labeller", `
parameter: {label: string}
patch: metadata: labels: "trait-case/label": parameter.label
`))).Should(Succeed())

		autoUpdateOn := true
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "trait-case-app", Namespace: namespaceName},
			Spec: v1beta1.ApplicationSpec{
				Sources: []v1beta1.ApplicationSource{
					{
						Name:       "forcomp",
						Type:       "trait-case-source",
						AutoUpdate: &autoUpdateOn,
						Properties: &runtime.RawExtension{Raw: []byte(`{"name":"trait-case-comp-input"}`)},
					},
					{
						Name:       "fortrait",
						Type:       "trait-case-source",
						AutoUpdate: &autoUpdateOn,
						Properties: &runtime.RawExtension{Raw: []byte(`{"name":"trait-case-trait-input"}`)},
					},
				},
				Components: []oamcomm.ApplicationComponent{{
					Name: "app",
					Type: "trait-case-cm",
					Properties: &runtime.RawExtension{Raw: []byte(
						`{"value":"$(has(source.forcomp.data.value) ? source.forcomp.data.value : \"absent\")"}`)},
					Traits: []oamcomm.ApplicationTrait{{
						Type: "trait-case-labeller",
						Properties: &runtime.RawExtension{Raw: []byte(
							`{"label":"$(has(source.fortrait.data.value) ? source.fortrait.data.value : \"absent\")"}`)},
					}},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, optIn(app))).Should(Succeed())
		verifyApplicationPhase(ctx, namespaceName, app.Name, oamcomm.ApplicationRunning)

		renderedValue := func() string {
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKey{
				Namespace: namespaceName, Name: "trait-case-result"}, cm); err != nil {
				return ""
			}
			return cm.Data["value"]
		}
		Eventually(renderedValue, 90*time.Second, time.Second).Should(Equal("first"))

		// Both bindings must be reported. Losing one here is the bug in its
		// visible form, before it reaches the hash.
		Eventually(func() []string {
			latest := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), latest); err != nil {
				return nil
			}
			names := []string{}
			for _, s := range latest.Status.Sources {
				names = append(names, s.Name)
			}
			sort.Strings(names)
			return names
		}, 90*time.Second, time.Second).Should(Equal([]string{"forcomp", "fortrait"}),
			"both bindings must appear in status.sources[]; the trait's pass must not drop the component's")

		// Move the value the COMPONENT reads. The Application is untouched, so
		// only the resolved-hash can drive a re-dispatch.
		Eventually(func() error {
			live := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(compInput), live); err != nil {
				return err
			}
			live.Data["value"] = "second"
			return k8sClient.Update(ctx, live)
		}, 20*time.Second, time.Second).Should(Succeed())

		Eventually(renderedValue, 180*time.Second, 2*time.Second).Should(Equal("second"),
			"the component's own source must still drive auto-update when a trait reads another one")
	})

	// A list-valued parameter declared with a default - the ordinary way to write
	// an optional list - was rejected at admission whenever an Application supplied
	// more than the default:
	//
	//	"spec.sources[0].properties.paths[0]": property "paths.0" is not declared
	//	in the parameter schema of SourceDefinition "list-param-source"
	//
	// Properties are flattened to dotted leaves before being checked, so the list
	// arrives as paths.0 and paths.1, and neither resolved through the
	// disjunction the default creates.
	It("accepts a list-valued source parameter declared with a default", func() {
		Expect(k8sClient.Create(ctx, &v1beta1.SourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "list-param-source", Namespace: namespaceName},
			Spec: v1beta1.SourceDefinitionSpec{
				Schematic: &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: `
import "strings"

schema: {
  joined: string
}
$internal: {
  key: "list-param-source"
  keyInputs: []
}
output: {
  joined: strings.Join(parameter.paths, ",")
}
parameter: {
  paths: [...string] | *["app.yaml"]
}
`}},
			},
		})).Should(Succeed())

		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "list-param-app", Namespace: namespaceName},
			Spec: v1beta1.ApplicationSpec{
				Sources: []v1beta1.ApplicationSource{{
					Name: "cfg",
					Type: "list-param-source",
					// More entries than the default, which is what made this fail.
					Properties: &runtime.RawExtension{Raw: []byte(`{"paths":["one.yaml","two.yaml"]}`)},
				}},
				Components: []oamcomm.ApplicationComponent{{
					Name:       "web",
					Type:       "webservice",
					Properties: &runtime.RawExtension{Raw: []byte(`{"image":"nginx:1.25.0","env":[{"name":"PATHS","value":"$(source.cfg.joined)"}]}`)},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, optIn(app))).Should(Succeed(),
			"a list parameter with a default must be admitted, not rejected as undeclared")

		verifyApplicationPhase(ctx, namespaceName, app.Name, oamcomm.ApplicationRunning)
		Eventually(func() (string, error) {
			deploy := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "web"}, deploy); err != nil {
				return "", err
			}
			for _, e := range deploy.Spec.Template.Spec.Containers[0].Env {
				if e.Name == "PATHS" {
					return e.Value, nil
				}
			}
			return "", fmt.Errorf("PATHS env not set yet")
		}, 90*time.Second, time.Second).Should(Equal("one.yaml,two.yaml"),
			"both list entries must reach the template, not just the default")
	})

	It("creates source cache using storage key policy", func() {
		sourceDef := &v1beta1.SourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "stale-image-source",
				Namespace: namespaceName,
			},
			Spec: v1beta1.SourceDefinitionSpec{
				Schematic: &oamcomm.Schematic{
					CUE: &oamcomm.CUE{
						Template: `
schema: {
  image: string
}
// Generated: the template reads no context, so the key is the definition name.
// The $internal block is not itself scanned, so a key mentioning
// context.namespace would not make namespace a dimension.
$internal: {
  key: "stale-image-source"
  keyInputs: []
}
storage: {
  storageTTL: "1h"
  onStaleFailure: "use-stale"
}
output: {
  image: parameter.image
}
parameter: {
  image: string
}
`,
					},
				},
			},
		}
		applyDefinition(ctx, sourceDef)
		// SourceDefinition controller should publish deterministic ConfigTemplate reference.
		Eventually(func() error {
			latest := &v1beta1.SourceDefinition{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "stale-image-source"}, latest); err != nil {
				return err
			}
			if latest.Status.ConfigTemplateRef == nil || latest.Status.ConfigTemplateRef.Name == "" {
				return fmt.Errorf("configTemplateRef not ready")
			}
			if latest.Status.ConfigTemplateRef.SchemaHash == "" {
				return fmt.Errorf("configTemplateRef.schemaHash not ready")
			}
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKey{
				Namespace: "vela-system",
				Name:      "config-template-" + latest.Status.ConfigTemplateRef.Name,
			}, cm); err != nil {
				return err
			}
			if cm.Labels["config.oam.dev/catalog"] != "velacore-config" {
				return fmt.Errorf("unexpected config catalog label: %q", cm.Labels["config.oam.dev/catalog"])
			}
			if cm.Data["schema"] == "" {
				return fmt.Errorf("missing schema in config template")
			}
			return nil
		}, 60*time.Second, time.Second).Should(Succeed())

		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "source-stale-app",
				Namespace: namespaceName,
			},
			Spec: v1beta1.ApplicationSpec{
				Sources: []v1beta1.ApplicationSource{
					{
						Name: "img",
						Type: "stale-image-source",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"image":"nginx:1.25.0"}`),
						},
					},
				},
				Components: []oamcomm.ApplicationComponent{
					{
						Name: "web-stale",
						Type: "webservice",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{
  "image":"$(source.img.image)",
  "port":80
}`),
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, optIn(app))).Should(Succeed())
		verifyApplicationPhase(ctx, namespaceName, app.Name, oamcomm.ApplicationRunning)

		// The cache entry is named <storage.key>-<propertiesHash>: the generated key
		// leads so it stays greppable, and the hash discriminates bindings that
		// differ only in their properties. The hash is not knowable here, so match
		// on the prefix rather than pinning a name.
		Eventually(func() error {
			var secrets corev1.SecretList
			if err := k8sClient.List(ctx, &secrets, client.InNamespace("vela-system")); err != nil {
				return err
			}
			var secret *corev1.Secret
			for i := range secrets.Items {
				if strings.HasPrefix(secrets.Items[i].Name, "stale-image-source-") {
					secret = &secrets.Items[i]
					break
				}
			}
			if secret == nil {
				return fmt.Errorf("no source cache entry with prefix %q in vela-system", "stale-image-source-")
			}
			sourceDef := &v1beta1.SourceDefinition{}
			if err := k8sClient.Get(ctx, client.ObjectKey{
				Namespace: namespaceName,
				Name:      "stale-image-source",
			}, sourceDef); err != nil {
				return err
			}
			if secret.Labels["config.oam.dev/catalog"] != "velacore-config" {
				return fmt.Errorf("unexpected config catalog label: %q", secret.Labels["config.oam.dev/catalog"])
			}
			if sourceDef.Status.ConfigTemplateRef == nil || sourceDef.Status.ConfigTemplateRef.Name == "" {
				return fmt.Errorf("source definition configTemplateRef missing")
			}
			if secret.Labels["config.oam.dev/type"] != sourceDef.Status.ConfigTemplateRef.Name {
				return fmt.Errorf("unexpected config type label: %q", secret.Labels["config.oam.dev/type"])
			}
			if len(secret.Data["input-properties"]) == 0 {
				return fmt.Errorf("missing input-properties in source cache secret")
			}
			var got map[string]interface{}
			if err := json.Unmarshal(secret.Data["input-properties"], &got); err != nil {
				return err
			}
			image, _ := got["image"].(string)
			if image != "nginx:1.25.0" {
				return fmt.Errorf("unexpected cached image: %v", got["image"])
			}
			return nil
		}, 60*time.Second, time.Second).Should(Succeed())
	})

	// An edited definition must stop addressing the entries its previous version
	// resolved. Cached values are served without re-validation, so a definition
	// whose fetch logic changed would otherwise keep serving data the old logic
	// produced - for as long as the TTL allows, which here is an hour.
	//
	// The change below is deliberately invisible to every other signal: the key is
	// unchanged (no context is read), the schema is unchanged, and the output shape
	// is unchanged. Only the value the template computes differs, which is the
	// shape of a changed URL behind a stable schema. Nothing but the template
	// itself being part of the cache identity can catch it.
	It("orphans cached entries when the definition is edited", func() {
		const defName = "edited-source"

		// Real tags: the resolved value becomes a container image, and an
		// unpullable one would leave the app short of running for reasons that
		// have nothing to do with caching.
		templateFor := func(tag string) string {
			return fmt.Sprintf(`
schema: {
  image: string
}
$internal: {
  key: "%s"
  keyInputs: []
}
storage: {
  // Long enough that a stale entry would certainly be served if the identity
  // did not move.
  storageTTL: "1h"
}
output: {
  image: parameter.image + ":%s"
}
parameter: {
  image: string
}
`, defName, tag)
		}

		sourceDef := &v1beta1.SourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: defName, Namespace: namespaceName},
			Spec: v1beta1.SourceDefinitionSpec{
				Schematic: &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: templateFor("1.25.0")}},
			},
		}
		applyDefinition(ctx, sourceDef)

		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "edited-source-app", Namespace: namespaceName},
			Spec: v1beta1.ApplicationSpec{
				Sources: []v1beta1.ApplicationSource{{
					Name:       "img",
					Type:       defName,
					Properties: &runtime.RawExtension{Raw: []byte(`{"image":"nginx"}`)},
				}},
				Components: []oamcomm.ApplicationComponent{{
					Name: "web-edited",
					Type: "webservice",
					Properties: &runtime.RawExtension{
						Raw: []byte(`{"image":"$(source.img.image)","port":80}`),
					},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, optIn(app))).Should(Succeed())

		// The app's own status reports both facts this test needs: which cache
		// entry the binding resolved against, and what it resolved to. Reading
		// them there rather than from the Deployment keeps the assertion about
		// caching instead of about pod scheduling.
		resolved := func() (config string, image string, err error) {
			latest := &v1beta1.Application{}
			if err = k8sClient.Get(ctx, client.ObjectKey{
				Namespace: namespaceName, Name: app.Name,
			}, latest); err != nil {
				return "", "", err
			}
			key, err := storageKeyOf(latest, "img")
			if err != nil {
				return "", "", err
			}
			got, err := consumedValue(latest, "img", "image")
			if err != nil {
				return "", "", err
			}
			return key, got, nil
		}

		var firstConfig string
		Eventually(func() error {
			config, image, err := resolved()
			if err != nil {
				return err
			}
			if config == "" {
				return fmt.Errorf("no cache entry recorded yet")
			}
			if image != "nginx:1.25.0" {
				return fmt.Errorf("expected the v1 template's value, got %q", image)
			}
			firstConfig = config
			return nil
		}, 90*time.Second, time.Second).Should(Succeed())

		// Edit the definition. Same key, same schema, same output shape - only the
		// value the template computes differs, which is the shape of a changed URL
		// behind a stable schema. Nothing but the template being part of the cache
		// identity can catch it.
		Eventually(func() error {
			latest := &v1beta1.SourceDefinition{}
			if err := k8sClient.Get(ctx, client.ObjectKey{
				Namespace: namespaceName, Name: defName,
			}, latest); err != nil {
				return err
			}
			latest.Spec.Schematic.CUE.Template = templateFor("1.25.2")
			return k8sClient.Update(ctx, latest)
		}, 30*time.Second, time.Second).Should(Succeed())

		// The binding must move to a different entry and serve the new value. With
		// an hour of TTL left on the old one, serving 1.25.0 here would mean the
		// edit had been ignored.
		Eventually(func() error {
			config, image, err := resolved()
			if err != nil {
				return err
			}
			if config == firstConfig {
				return fmt.Errorf("still addressing the pre-edit cache entry %q", firstConfig)
			}
			if image != "nginx:1.25.2" {
				return fmt.Errorf("the pre-edit value is still being served: %q", image)
			}
			return nil
		}, 180*time.Second, 2*time.Second).Should(Succeed())
	})

	It("resolves chained nested sources where second source depends on first", func() {
		sourceA := &v1beta1.SourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-source",
				Namespace: namespaceName,
			},
			Spec: v1beta1.SourceDefinitionSpec{
				Schematic: &oamcomm.Schematic{
					CUE: &oamcomm.CUE{
						Template: `
schema: {
  nested: {
    image: {
      repo: string
      tag:  string
    }
    meta: {
      region: string
    }
  }
}
$internal: {
  key: "cluster-source"
  keyInputs: []
}
output: {
  nested: {
    image: {
      repo: parameter.repo
      tag:  parameter.tag
    }
    meta: {
      region: parameter.region
    }
  }
}
parameter: {
  repo:   string
  tag:    string
  region: string
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sourceA)).Should(Succeed())

		sourceB := &v1beta1.SourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "render-source",
				Namespace: namespaceName,
			},
			Spec: v1beta1.SourceDefinitionSpec{
				Schematic: &oamcomm.Schematic{
					CUE: &oamcomm.CUE{
						Template: `
schema: {
  resolved: {
    image:  string
    region: string
  }
}
$internal: {
  key: "render-source"
  keyInputs: []
}
output: {
  resolved: {
    image: "\(parameter.repo):\(parameter.tag)"
    region: parameter.region
  }
}
parameter: {
  repo:   string
  tag:    string
  region: string
}
`,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sourceB)).Should(Succeed())

		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "source-chain-app",
				Namespace: namespaceName,
			},
			Spec: v1beta1.ApplicationSpec{
				Sources: []v1beta1.ApplicationSource{
					{
						Name: "clusterInfo",
						Type: "cluster-source",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"repo":"nginx","tag":"1.25.2","region":"us-east-1"}`),
						},
					},
					{
						Name: "rendered",
						Type: "render-source",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{
  "repo":"$(source.clusterInfo.nested.image.repo)",
  "tag":"$(source.clusterInfo.nested.image.tag)",
  "region":"$(source.clusterInfo.nested.meta.region)"
}`),
						},
					},
				},
				Components: []oamcomm.ApplicationComponent{
					{
						Name: "web-chain",
						Type: "webservice",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{
  "image":"$(source.rendered.resolved.image)",
  "port":80
}`),
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, optIn(app))).Should(Succeed())

		verifyApplicationPhase(ctx, namespaceName, app.Name, oamcomm.ApplicationRunning)
		Eventually(func() (string, error) {
			deploy := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "web-chain"}, deploy); err != nil {
				return "", err
			}
			if len(deploy.Spec.Template.Spec.Containers) == 0 {
				return "", fmt.Errorf("deployment has no containers")
			}
			return deploy.Spec.Template.Spec.Containers[0].Image, nil
		}, 90*time.Second, time.Second).Should(Equal("nginx:1.25.2"))
	})

	It("resolves source values in trait properties", func() {
		sourceDef := &v1beta1.SourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "scale-source",
				Namespace: namespaceName,
			},
			Spec: v1beta1.SourceDefinitionSpec{
				Schematic: &oamcomm.Schematic{
					CUE: &oamcomm.CUE{
						Template: `
schema: {
  scale: {
    replicas: int
  }
}
$internal: {
  key: "scale-source"
  keyInputs: []
}
output: {
  scale: {
    replicas: parameter.replicas
  }
}
parameter: {
  replicas: int
}
`,
					},
				},
			},
		}
		applyDefinition(ctx, sourceDef)

		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "source-trait-app",
				Namespace: namespaceName,
			},
			Spec: v1beta1.ApplicationSpec{
				Sources: []v1beta1.ApplicationSource{
					{
						Name: "scaleData",
						Type: "scale-source",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"replicas":3}`),
						},
					},
				},
				Components: []oamcomm.ApplicationComponent{
					{
						Name: "web-trait",
						Type: "webservice",
						Properties: &runtime.RawExtension{
							Raw: []byte(`{
  "image":"nginx:1.25.0",
  "port":80
}`),
						},
						Traits: []oamcomm.ApplicationTrait{
							{
								Type: "scaler",
								Properties: &runtime.RawExtension{
									Raw: []byte(`{
  "replicas":"$(source.scaleData.scale.replicas)"
}`),
								},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, optIn(app))).Should(Succeed())

		verifyApplicationPhase(ctx, namespaceName, app.Name, oamcomm.ApplicationRunning)
		Eventually(func() (int32, error) {
			deploy := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "web-trait"}, deploy); err != nil {
				return 0, err
			}
			if deploy.Spec.Replicas == nil {
				return 0, fmt.Errorf("deployment replicas is nil")
			}
			return *deploy.Spec.Replicas, nil
		}, 90*time.Second, time.Second).Should(Equal(int32(3)))
	})

	// Negative cases: verify the admission webhook is wired and rejects known-bad
	// source usage at kubectl apply time, with an intelligible message. These
	// complement the unit tests in pkg/webhook (which test the logic directly) by
	// proving the deny path reaches the client end-to-end.
	// A source is a cached, shared read, so it compiles against SourceCompiler
	// rather than WorkloadCompiler. helm.#Render calls installOrUpgradeChart
	// outside dry-run and the render path sets none, so a source importing
	// vela/helm would install a real release on its first cache miss. It is
	// refused when the definition is applied rather than at that point.
	It("refuses a SourceDefinition importing a package that acts", func() {
		for _, pkg := range []string{"vela/helm", "vela/addon"} {
			def := &v1beta1.SourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "acting-" + strings.ReplaceAll(pkg, "/", "-"),
					Namespace: namespaceName,
				},
				Spec: v1beta1.SourceDefinitionSpec{
					Schematic: &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: `
import "` + pkg + `"
schema: {name: string}
$internal: {key: "acting", keyInputs: []}
parameter: {}
output: name: "x"
`}},
				},
			}
			err := k8sClient.Create(ctx, def)
			Expect(err).To(HaveOccurred(), "a source must not be able to import %s", pkg)
			Expect(err.Error()).To(ContainSubstring(pkg))
		}
	})

	Context("rejects invalid source usage at admission", func() {
		// A SourceDefinition with a required string field, an optional field, and
		// an int parameter, used by the negative cases below.
		applyTypedSource := func() {
			sourceDef := &v1beta1.SourceDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "typed-source", Namespace: namespaceName},
				Spec: v1beta1.SourceDefinitionSpec{
					Schematic: &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: `
schema: {
  image:  string
  vpcId?: string
}
$internal: {
  key: "typed-source"
  keyInputs: []
}
output: {
  image: parameter.image
}
parameter: {
  image:    string
  replicas: int
}
`}},
				},
			}
			applyDefinition(ctx, sourceDef)
		}

		newApp := func(name string, sources []v1beta1.ApplicationSource, comps []oamcomm.ApplicationComponent) *v1beta1.Application {
			return &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespaceName},
				Spec:       v1beta1.ApplicationSpec{Sources: sources, Components: comps},
			}
		}

		// spec.components is structurally required by the Application CRD, so
		// cases that exercise only source-level validation still need a valid
		// component to get past structural admission and reach the webhook.
		minimalComp := func() []oamcomm.ApplicationComponent {
			return []oamcomm.ApplicationComponent{
				{Name: "web", Type: "webservice", Properties: &runtime.RawExtension{Raw: []byte(`{"image":"nginx:1.25.0","port":80}`)}},
			}
		}

		It("denies a source path not declared in the source schema", func() {
			applyTypedSource()
			app := newApp("bad-path", []v1beta1.ApplicationSource{
				{Name: "s", Type: "typed-source", Properties: &runtime.RawExtension{Raw: []byte(`{"image":"nginx","replicas":1}`)}},
			}, []oamcomm.ApplicationComponent{
				{Name: "web", Type: "webservice", Properties: &runtime.RawExtension{Raw: []byte(`{"image":"$(source.s.doesNotExist)","port":80}`)}},
			})
			err := k8sClient.Create(ctx, optIn(app))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("is not declared in schema of SourceDefinition"))
		})

		It("denies an optional schema field consumed without a default", func() {
			applyTypedSource()
			app := newApp("no-default", []v1beta1.ApplicationSource{
				{Name: "s", Type: "typed-source", Properties: &runtime.RawExtension{Raw: []byte(`{"image":"nginx","replicas":1}`)}},
			}, []oamcomm.ApplicationComponent{
				{Name: "web", Type: "webservice", Properties: &runtime.RawExtension{Raw: []byte(`{"image":"$(source.s.vpcId)","port":80}`)}},
			})
			err := k8sClient.Create(ctx, optIn(app))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("guard it with"))
		})

		It("denies a forward source dependency", func() {
			applyTypedSource()
			// source at index 0 references a source declared at index 1.
			app := newApp("forward-ref", []v1beta1.ApplicationSource{
				{Name: "first", Type: "typed-source", Properties: &runtime.RawExtension{Raw: []byte(`{"image":"$(source.second.image)","replicas":1}`)}},
				{Name: "second", Type: "typed-source", Properties: &runtime.RawExtension{Raw: []byte(`{"image":"nginx","replicas":1}`)}},
			}, minimalComp())
			err := k8sClient.Create(ctx, optIn(app))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("can only depend on prior sources"))
		})

		It("denies an undeclared source property", func() {
			applyTypedSource()
			app := newApp("unknown-prop", []v1beta1.ApplicationSource{
				{Name: "s", Type: "typed-source", Properties: &runtime.RawExtension{Raw: []byte(`{"image":"nginx","replicas":1,"bogus":"x"}`)}},
			}, minimalComp())
			err := k8sClient.Create(ctx, optIn(app))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("is not declared in the parameter schema of SourceDefinition"))
		})

		It("denies a source property with an incompatible type", func() {
			applyTypedSource()
			app := newApp("bad-type", []v1beta1.ApplicationSource{
				{Name: "s", Type: "typed-source", Properties: &runtime.RawExtension{Raw: []byte(`{"image":"nginx","replicas":"three"}`)}},
			}, minimalComp())
			err := k8sClient.Create(ctx, optIn(app))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("type mismatch for parameter"))
		})
	})

	// $internal.key is derived from the context a template reads, generated by
	// vela def and re-derived at admission. These cases cover the paths vela def
	// cannot guard - a stored object edited in place, or YAML written by hand -
	// which is where a wrong key would otherwise reach the cluster and quietly
	// change which Applications share a cache entry.
	Context("rejects a SourceDefinition whose cache key was not derived", func() {
		newSource := func(name, template string, annotations map[string]string) *v1beta1.SourceDefinition {
			return &v1beta1.SourceDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespaceName, Annotations: annotations},
				Spec: v1beta1.SourceDefinitionSpec{
					Schematic: &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: template}},
				},
			}
		}

		// Reads context.cluster, so the derived key is "<name>-\(context.cluster)"
		// and the single input is "cluster".
		const readsCluster = `
schema: {region: string}
$internal: {key: "%s", keyInputs: ["cluster"]}
output: {region: context.cluster}
parameter: {}
`

		It("accepts the derived key", func() {
			def := newSource("derived-ok", fmt.Sprintf(readsCluster, `derived-ok-\(context.cluster)`), nil)
			Expect(k8sClient.Create(ctx, def)).Should(Succeed())
		})

		It("rejects a hand-written key, naming the one expected", func() {
			def := newSource("derived-bad", fmt.Sprintf(readsCluster, "whatever-i-like"), nil)
			err := k8sClient.Create(ctx, def)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("whatever-i-like"))
			Expect(err.Error()).To(ContainSubstring(`derived-bad-\(context.cluster)`))
		})

		It("rejects a key that ignores context the template reads", func() {
			// The subtle one: legal, readable, and wrong. Every cluster would
			// share one entry and each would serve the first cluster's data.
			def := newSource("derived-flat", fmt.Sprintf(readsCluster, "derived-flat"), nil)
			err := k8sClient.Create(ctx, def)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not match"))
		})

		It("rejects rules this build does not have", func() {
			// Validating against different rules than the ones that generated the
			// key would defeat recording which were used.
			def := newSource("derived-unknown-rules",
				fmt.Sprintf(readsCluster, `derived-unknown-rules-\(context.cluster)`),
				map[string]string{"definition.oam.dev/cache-key-rules": "deadbeef"})
			err := k8sClient.Create(ctx, def)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("deadbeef"))
		})
	})

})
