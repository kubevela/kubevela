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

package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// These tests drive the real reconciler end to end. Watches don't fire in envtest, so a
// global edit is followed by the index/cache refresh the PolicyDefinition watch performs.
var _ = Describe("Test global PolicyDefinition changes reach existing Applications", func() {
	namespace := "global-policy-propagation"
	var ctx context.Context

	labelPolicy := func(name, value string) *v1beta1.PolicyDefinition {
		return &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: v1beta1.PolicyDefinitionSpec{
				Global: true,
				Scope:  v1beta1.ApplicationScope,
				Schematic: &common.Schematic{CUE: &common.CUE{
					Template: fmt.Sprintf("parameter: {}\noutput: labels: {\"gwcp/tag\": %q}\n", value),
				}},
			},
		}
	}

	// editGlobal rewrites the policy template and refreshes the index and cache as the watch would.
	editGlobal := func(name, template string) {
		pd := &v1beta1.PolicyDefinition{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, pd)).Should(Succeed())
		pd.Spec.Schematic.CUE.Template = template
		Expect(k8sClient.Update(ctx, pd)).Should(Succeed())
		Eventually(func() string {
			got := &v1beta1.PolicyDefinition{}
			_ = k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, got)
			return got.Spec.Schematic.CUE.Template
		}, "10s", "100ms").Should(Equal(template))
		policyScopeIndex.AddOrUpdate(pd)
		applicationPolicyCache.InvalidateForNamespace(namespace)
	}

	newConfigMapApp := func(name string, annotations map[string]string) *v1beta1.Application {
		return &v1beta1.Application{
			TypeMeta:   metav1.TypeMeta{APIVersion: v1beta1.SchemeGroupVersion.String(), Kind: v1beta1.ApplicationKind},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Annotations: annotations},
			Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{
				Name: name + "-comp",
				Type: "k8s-objects",
				Properties: &runtime.RawExtension{Raw: []byte(fmt.Sprintf(
					`{"objects":[{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":%q},"data":{"k":"v"}}]}`, name+"-cm"))},
			}}},
		}
	}

	adoptUpdates := map[string]string{oam.AnnotationApplyPolicyUpdates: "true"}

	getApp := func(key types.NamespacedName) *v1beta1.Application {
		app := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).Should(Succeed())
		return app
	}

	cmLabel := func(name string) string {
		cm := &corev1.ConfigMap{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, cm); err != nil {
			return "<" + err.Error() + ">"
		}
		return cm.Labels["gwcp/tag"]
	}

	// reconcileUntil reconciles the app until cond holds, as successive resyncs would.
	reconcileUntil := func(key types.NamespacedName, cond func() bool) {
		GinkgoHelper()
		Eventually(func() error {
			_, _ = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: key})
			if cond() {
				return nil
			}
			app := getApp(key)
			wfRev := ""
			if app.Status.Workflow != nil {
				wfRev = app.Status.Workflow.AppRevision
			}
			cm := &corev1.ConfigMap{}
			_ = k8sClient.Get(ctx, client.ObjectKey{Name: app.Name + "-cm", Namespace: namespace}, cm)
			return fmt.Errorf("phase=%s appLabels=%v latestRevision=%v workflowRevision=%s configMapLabels=%v",
				app.Status.Phase, app.Labels, app.Status.LatestRevision, wfRev, cm.Labels)
		}, 30*time.Second, 200*time.Millisecond).Should(Succeed())
	}

	// deployed reports whether the app has a revision and finished its workflow.
	deployed := func(key types.NamespacedName) bool {
		app := getApp(key)
		return app.Status.Phase == common.ApplicationRunning && app.Status.LatestRevision != nil
	}

	BeforeEach(func() {
		ctx = util.SetNamespaceInCtx(context.Background(), namespace)
		ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		velaSystem := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: oam.SystemDefinitionNamespace}}
		Expect(k8sClient.Create(ctx, &velaSystem)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		k8sObjects := &v1beta1.ComponentDefinition{}
		Expect(yaml.Unmarshal([]byte(k8sObjectsComponentDefinitionYaml), k8sObjects)).Should(Succeed())
		Expect(k8sClient.Create(ctx, k8sObjects)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableGlobalPolicies=true")).ToNot(HaveOccurred())
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableApplicationScopedPolicies=true")).ToNot(HaveOccurred())
		applicationPolicyCache.InvalidateAll()
		policyScopeIndex.client = k8sClient
		policyScopeIndex.initialized = false
	})

	AfterEach(func() {
		apps := &v1beta1.ApplicationList{}
		_ = k8sClient.List(ctx, apps, client.InNamespace(namespace))
		for i := range apps.Items {
			_ = k8sClient.Delete(ctx, &apps.Items[i])
		}
		policies := &v1beta1.PolicyDefinitionList{}
		_ = k8sClient.List(ctx, policies, client.InNamespace(namespace))
		for i := range policies.Items {
			_ = k8sClient.Delete(ctx, &policies.Items[i])
			policyScopeIndex.Delete(policies.Items[i].Name, namespace)
		}
		applicationPolicyCache.InvalidateAll()
	})

	It("an edited metadata-only global reaches the Application and its dispatched resources", func() {
		createPolicyDefAndIndex(ctx, labelPolicy("gwcp-tags", "v1"))
		app := newConfigMapApp("gp-prop-md", adoptUpdates)
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		key := client.ObjectKeyFromObject(app)

		reconcileUntil(key, func() bool { return deployed(key) && cmLabel("gp-prop-md-cm") == "v1" })
		firstRev := getApp(key).Status.LatestRevision.Name

		editGlobal("gwcp-tags", "parameter: {}\noutput: labels: {\"gwcp/tag\": \"v2\"}\n")

		reconcileUntil(key, func() bool {
			return getApp(key).Labels["gwcp/tag"] == "v2" && cmLabel("gp-prop-md-cm") == "v2" && deployed(key)
		})
		Expect(getApp(key).Status.LatestRevision.Name).ShouldNot(Equal(firstRev),
			"the global change must be recorded in a new ApplicationRevision")
	})

	It("an unchanged global does not cut spurious revisions", func() {
		createPolicyDefAndIndex(ctx, labelPolicy("gwcp-steady", "v1"))
		app := newConfigMapApp("gp-prop-steady", nil)
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		key := client.ObjectKeyFromObject(app)

		reconcileUntil(key, func() bool { return deployed(key) && cmLabel("gp-prop-steady-cm") == "v1" })
		firstRev := getApp(key).Status.LatestRevision.Name

		for i := 0; i < 5; i++ {
			applicationPolicyCache.InvalidateForNamespace(namespace)
			_, _ = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: key})
		}
		Expect(getApp(key).Status.LatestRevision.Name).Should(Equal(firstRev))
	})

	It("a publishVersion-pinned Application is not redeployed by a global change", func() {
		createPolicyDefAndIndex(ctx, labelPolicy("gwcp-pinned", "v1"))
		app := newConfigMapApp("gp-prop-pinned", map[string]string{oam.AnnotationPublishVersion: "release-1", oam.AnnotationApplyPolicyUpdates: "true"})
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		key := client.ObjectKeyFromObject(app)

		reconcileUntil(key, func() bool { return deployed(key) && cmLabel("gp-prop-pinned-cm") == "v1" })
		firstRev := getApp(key).Status.LatestRevision.Name

		editGlobal("gwcp-pinned", "parameter: {}\noutput: labels: {\"gwcp/tag\": \"v2\"}\n")
		for i := 0; i < 5; i++ {
			_, _ = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: key})
		}
		Expect(getApp(key).Status.LatestRevision.Name).Should(Equal(firstRev))
		Expect(cmLabel("gp-prop-pinned-cm")).Should(Equal("v1"), "the pin holds until publishVersion is bumped")

		By("bumping publishVersion, as a GitOps commit would, rolls the changed global out")
		pinned := getApp(key)
		pinned.Annotations[oam.AnnotationPublishVersion] = "release-2"
		Expect(k8sClient.Update(ctx, pinned)).Should(Succeed())
		reconcileUntil(key, func() bool {
			return getApp(key).Labels["gwcp/tag"] == "v2" && cmLabel("gp-prop-pinned-cm") == "v2" && deployed(key)
		})
	})

	It("an edited spec-changing global stays pinned even when updates are applied", func() {
		specPolicy := func(image string) string {
			return fmt.Sprintf("parameter: {}\noutput: components: [{name: \"gp-prop-spec-comp\", type: \"k8s-objects\", "+
				"properties: objects: [{apiVersion: \"v1\", kind: \"ConfigMap\", metadata: name: \"gp-prop-spec-cm\", data: image: %q}]}]\n", image)
		}
		pd := labelPolicy("gwcp-spec", "unused")
		pd.Spec.Schematic.CUE.Template = specPolicy("v1")
		createPolicyDefAndIndex(ctx, pd)
		app := newConfigMapApp("gp-prop-spec", adoptUpdates)
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		key := client.ObjectKeyFromObject(app)

		cmImage := func() string {
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: "gp-prop-spec-cm", Namespace: namespace}, cm); err != nil {
				return ""
			}
			return cm.Data["image"]
		}
		reconcileUntil(key, func() bool { return deployed(key) && cmImage() == "v1" })
		firstRev := getApp(key).Status.LatestRevision.Name

		editGlobal("gwcp-spec", specPolicy("v2"))
		for i := 0; i < 5; i++ {
			_, _ = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: key})
		}
		Expect(getApp(key).Status.LatestRevision.Name).Should(Equal(firstRev),
			"spec-changing globals stay pinned without auto-revision; out of scope for this fix")
		Expect(cmImage()).Should(Equal("v1"))
	})
	It("by default an edited global is held and reported as pending", func() {
		createPolicyDefAndIndex(ctx, labelPolicy("gwcp-held", "v1"))
		app := newConfigMapApp("gp-prop-held", nil)
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		key := client.ObjectKeyFromObject(app)

		reconcileUntil(key, func() bool { return deployed(key) && cmLabel("gp-prop-held-cm") == "v1" })
		firstRev := getApp(key).Status.LatestRevision.Name

		editGlobal("gwcp-held", "parameter: {}\noutput: labels: {\"gwcp/tag\": \"v2\"}\n")
		reconcileUntil(key, func() bool {
			for _, p := range getApp(key).Status.AppliedApplicationPolicies {
				if p.Type == "gwcp-held" && strings.Contains(p.Message, "update pending") {
					return true
				}
			}
			return false
		})
		Expect(getApp(key).Status.LatestRevision.Name).Should(Equal(firstRev))
		Expect(cmLabel("gp-prop-held-cm")).Should(Equal("v1"))
	})

	It("a held update is adopted at the app's next revision", func() {
		createPolicyDefAndIndex(ctx, labelPolicy("gwcp-next", "v1"))
		app := newConfigMapApp("gp-prop-next", nil)
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		key := client.ObjectKeyFromObject(app)

		reconcileUntil(key, func() bool { return deployed(key) && cmLabel("gp-prop-next-cm") == "v1" })
		editGlobal("gwcp-next", "parameter: {}\noutput: labels: {\"gwcp/tag\": \"v2\"}\n")

		By("an author edit cuts a new revision, which takes the live policy as before")
		edited := getApp(key)
		edited.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(
			`{"objects":[{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"gp-prop-next-cm"},"data":{"k":"v2"}}]}`)}
		Expect(k8sClient.Update(ctx, edited)).Should(Succeed())
		reconcileUntil(key, func() bool { return cmLabel("gp-prop-next-cm") == "v2" && deployed(key) })
	})

	It("an unchanged spec-rewriting global does not hold another policy's update", func() {
		specGlobal := labelPolicy("gwcp-sidecar", "unused")
		specGlobal.Spec.Schematic.CUE.Template = `parameter: {}
output: components: [{name: "gp-prop-mixed-comp", type: "k8s-objects", properties: objects: [{
  apiVersion: "v1", kind: "ConfigMap", metadata: name: "gp-prop-mixed-cm", data: injected: "yes"}]}]
`
		createPolicyDefAndIndex(ctx, specGlobal)
		createPolicyDefAndIndex(ctx, labelPolicy("gwcp-mixed-tags", "v1"))
		app := newConfigMapApp("gp-prop-mixed", adoptUpdates)
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		key := client.ObjectKeyFromObject(app)
		reconcileUntil(key, func() bool { return deployed(key) && cmLabel("gp-prop-mixed-cm") == "v1" })

		editGlobal("gwcp-mixed-tags", "parameter: {}\noutput: labels: {\"gwcp/tag\": \"v2\"}\n")
		reconcileUntil(key, func() bool { return cmLabel("gp-prop-mixed-cm") == "v2" && deployed(key) })
	})

	It("a new policy version that fails to render is held, not applied", func() {
		explicit := labelPolicy("explicit-broken", "v1")
		explicit.Spec.Global = false
		createPolicyDefAndIndex(ctx, explicit)
		app := newConfigMapApp("gp-prop-broken", adoptUpdates)
		app.Spec.Policies = []v1beta1.AppPolicy{{Name: "tags", Type: "explicit-broken"}}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		key := client.ObjectKeyFromObject(app)
		reconcileUntil(key, func() bool { return deployed(key) && cmLabel("gp-prop-broken-cm") == "v1" })
		firstRev := getApp(key).Status.LatestRevision.Name

		editGlobal("explicit-broken", "parameter: {}\noutput: labels: {\"gwcp/tag\": undefinedReference}\n")
		reconcileUntil(key, func() bool {
			for _, p := range getApp(key).Status.AppliedApplicationPolicies {
				if p.Type == "explicit-broken" && strings.Contains(p.Message, "update pending") {
					return true
				}
			}
			return false
		})
		app2 := getApp(key)
		Expect(app2.Status.Phase).Should(Equal(common.ApplicationRunning), "the pinned version keeps the app reconciling")
		Expect(app2.Status.LatestRevision.Name).Should(Equal(firstRev))
		Expect(cmLabel("gp-prop-broken-cm")).Should(Equal("v1"))
	})

	It("a list applies updates only from the policies it names", func() {
		listed := labelPolicy("gwcp-listed", "v1")
		listed.Spec.Schematic.CUE.Template = "parameter: {}\noutput: labels: {\"gwcp/listed\": \"v1\"}\n"
		unlisted := labelPolicy("gwcp-unlisted", "v1")
		unlisted.Spec.Schematic.CUE.Template = "parameter: {}\noutput: labels: {\"gwcp/unlisted\": \"v1\"}\n"
		createPolicyDefAndIndex(ctx, listed)
		createPolicyDefAndIndex(ctx, unlisted)
		app := newConfigMapApp("gp-prop-list", map[string]string{oam.AnnotationApplyPolicyUpdates: "gwcp-listed"})
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		key := client.ObjectKeyFromObject(app)

		cmLabels := func() map[string]string {
			cm := &corev1.ConfigMap{}
			_ = k8sClient.Get(ctx, client.ObjectKey{Name: "gp-prop-list-cm", Namespace: namespace}, cm)
			return cm.Labels
		}
		reconcileUntil(key, func() bool {
			l := cmLabels()
			return deployed(key) && l["gwcp/listed"] == "v1" && l["gwcp/unlisted"] == "v1"
		})

		editGlobal("gwcp-listed", "parameter: {}\noutput: labels: {\"gwcp/listed\": \"v2\"}\n")
		editGlobal("gwcp-unlisted", "parameter: {}\noutput: labels: {\"gwcp/unlisted\": \"v2\"}\n")
		reconcileUntil(key, func() bool { return cmLabels()["gwcp/listed"] == "v2" && deployed(key) })
		Expect(cmLabels()["gwcp/unlisted"]).Should(Equal("v1"), "the unlisted policy waits for the next revision")
	})

	It("an edited explicit Application-scoped policy reaches the app", func() {
		explicit := labelPolicy("explicit-tags", "v1")
		explicit.Spec.Global = false
		createPolicyDefAndIndex(ctx, explicit)
		app := newConfigMapApp("gp-prop-explicit", adoptUpdates)
		app.Spec.Policies = []v1beta1.AppPolicy{{Name: "my-tags", Type: "explicit-tags"}}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		key := client.ObjectKeyFromObject(app)

		reconcileUntil(key, func() bool { return deployed(key) && cmLabel("gp-prop-explicit-cm") == "v1" })
		editGlobal("explicit-tags", "parameter: {}\noutput: labels: {\"gwcp/tag\": \"v2\"}\n")
		reconcileUntil(key, func() bool { return cmLabel("gp-prop-explicit-cm") == "v2" && deployed(key) })
	})
	Context("a policy whose output comes from cluster data", func() {
		dataPolicy := func(name, source string) *v1beta1.PolicyDefinition {
			pd := labelPolicy(name, "unused")
			pd.Spec.Schematic.CUE.Template = `
import "vela/kube"

parameter: {}
_source: kube.#Get & {
  $params: resource: {
    apiVersion: "v1"
    kind:       "ConfigMap"
    metadata: {name: "` + source + `", namespace: "` + namespace + `"}
  }
}
output: labels: {"gwcp/tag": _source.$returns.data.tag}
`
			return pd
		}
		source := func(name, tag string) {
			cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}, Data: map[string]string{"tag": tag}}
			existing := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), existing); err == nil {
				existing.Data = cm.Data
				Expect(k8sClient.Update(ctx, existing)).Should(Succeed())
				return
			}
			Expect(k8sClient.Create(ctx, cm)).Should(Succeed())
		}

		It("applies changed data to dispatched resources without a new revision or workflow run", func() {
			source("gp-data-src-on", "v1")
			createPolicyDefAndIndex(ctx, dataPolicy("gwcp-data-on", "gp-data-src-on"))
			app := newConfigMapApp("gp-data-on", map[string]string{oam.AnnotationApplyDataChanges: "true"})
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			key := client.ObjectKeyFromObject(app)

			reconcileUntil(key, func() bool { return deployed(key) && cmLabel("gp-data-on-cm") == "v1" })
			before := getApp(key)
			firstRev, firstRun := before.Status.LatestRevision.Name, before.Status.Workflow.StartTime

			source("gp-data-src-on", "v2")
			applicationPolicyCache.InvalidateApplication(namespace, app.Name) // as the cache TTL would
			reconcileUntil(key, func() bool { return cmLabel("gp-data-on-cm") == "v2" })

			after := getApp(key)
			Expect(after.Status.LatestRevision.Name).Should(Equal(firstRev), "a data change is not a new revision")
			Expect(after.Status.Workflow.StartTime).Should(Equal(firstRun), "a metadata data change does not re-run the workflow")
		})

		It("a publishVersion pin holds data changes until it moves", func() {
			source("gp-pin-src", "v1")
			createPolicyDefAndIndex(ctx, dataPolicy("gwcp-pin-md", "gp-pin-src"))
			app := newConfigMapApp("gp-pin-data", map[string]string{
				oam.AnnotationApplyDataChanges: "true", oam.AnnotationPublishVersion: "release-1"})
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			key := client.ObjectKeyFromObject(app)

			reconcileUntil(key, func() bool { return deployed(key) && cmLabel("gp-pin-data-cm") == "v1" })
			source("gp-pin-src", "v2")
			applicationPolicyCache.InvalidateApplication(namespace, app.Name)
			for i := 0; i < 5; i++ {
				_, _ = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: key})
			}
			Expect(cmLabel("gp-pin-data-cm")).Should(Equal("v1"), "metadata held by the pin")
		})

		It("rolling back to an earlier revision renders it with current data", func() {
			source("gp-rollback-src", "data-a")
			createPolicyDefAndIndex(ctx, dataPolicy("gwcp-rollback", "gp-rollback-src"))
			app := newConfigMapApp("gp-rollback", nil)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			key := client.ObjectKeyFromObject(app)
			reconcileUntil(key, func() bool { return deployed(key) && cmLabel("gp-rollback-cm") == "data-a" })
			firstRev := getApp(key).Status.LatestRevision.Name
			firstSpec := getApp(key).Spec.DeepCopy()

			By("an author edit cuts a second revision")
			edited := getApp(key)
			edited.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(
				`{"objects":[{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"gp-rollback-cm"},"data":{"k":"v2"}}]}`)}
			Expect(k8sClient.Update(ctx, edited)).Should(Succeed())
			reconcileUntil(key, func() bool { return getApp(key).Status.LatestRevision.Name != firstRev && deployed(key) })

			By("the data changes, then the app is rolled back to the first revision's spec")
			source("gp-rollback-src", "data-b")
			rolledBack := getApp(key)
			rolledBack.Spec = *firstSpec
			Expect(k8sClient.Update(ctx, rolledBack)).Should(Succeed())
			reconcileUntil(key, func() bool {
				return getApp(key).Status.LatestRevision.Name == firstRev && deployed(key) && cmLabel("gp-rollback-cm") == "data-b"
			})
		})

		It("by default leaves dispatched resources at the old data", func() {
			source("gp-data-src-off", "v1")
			createPolicyDefAndIndex(ctx, dataPolicy("gwcp-data-off", "gp-data-src-off"))
			app := newConfigMapApp("gp-data-off", nil)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			key := client.ObjectKeyFromObject(app)

			reconcileUntil(key, func() bool { return deployed(key) && cmLabel("gp-data-off-cm") == "v1" })
			source("gp-data-src-off", "v2")
			applicationPolicyCache.InvalidateApplication(namespace, app.Name)
			for i := 0; i < 5; i++ {
				_, _ = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: key})
			}
			Expect(cmLabel("gp-data-off-cm")).Should(Equal("v1"))
		})

		It("a policy with apply-data-changes off keeps its applied data through another policy's refresh", func() {
			keyed := func(name, src, key string) *v1beta1.PolicyDefinition {
				pd := dataPolicy(name, src)
				pd.Spec.Schematic.CUE.Template = strings.Replace(pd.Spec.Schematic.CUE.Template, `"gwcp/tag"`, `"`+key+`"`, 1)
				return pd
			}
			source("gp-two-src-a", "v1")
			source("gp-two-src-b", "v1")
			createPolicyDefAndIndex(ctx, keyed("gwcp-two-a", "gp-two-src-a", "gwcp/a"))
			createPolicyDefAndIndex(ctx, keyed("gwcp-two-b", "gp-two-src-b", "gwcp/b"))
			app := newConfigMapApp("gp-two", map[string]string{oam.AnnotationApplyDataChanges: "gwcp-two-a"})
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			key := client.ObjectKeyFromObject(app)
			labels := func() map[string]string {
				cm := &corev1.ConfigMap{}
				_ = k8sClient.Get(ctx, client.ObjectKey{Name: "gp-two-cm", Namespace: namespace}, cm)
				return cm.Labels
			}
			reconcileUntil(key, func() bool { return deployed(key) && labels()["gwcp/a"] == "v1" && labels()["gwcp/b"] == "v1" })

			source("gp-two-src-a", "v2")
			source("gp-two-src-b", "v2")
			applicationPolicyCache.InvalidateApplication(namespace, app.Name)
			reconcileUntil(key, func() bool { return labels()["gwcp/a"] == "v2" })
			for i := 0; i < 3; i++ {
				_, _ = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: key})
			}
			Expect(labels()["gwcp/b"]).Should(Equal("v1"), "the held policy's resource label stays at its applied data")
			Expect(getApp(key).Labels["gwcp/b"]).Should(Equal("v1"), "and so does the app's")
		})

		It("holds an app at its current revision while a mandatory global fails to render", func() {
			common2.AllowSkipGlobalPolicies = false
			DeferCleanup(func() { common2.AllowSkipGlobalPolicies = true })
			source("gp-hold-src", "v1")
			createPolicyDefAndIndex(ctx, dataPolicy("gwcp-hold", "gp-hold-src"))
			app := newConfigMapApp("gp-hold", nil)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			key := client.ObjectKeyFromObject(app)
			reconcileUntil(key, func() bool { return deployed(key) && cmLabel("gp-hold-cm") == "v1" })
			firstRev := getApp(key).Status.LatestRevision.Name
			_, _ = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: key})
			Expect(getApp(key).Status.GetCondition(globalPoliciesCondition).Status).Should(Equal(corev1.ConditionUnknown),
				"an app that was never held has no condition")

			By("the global's source goes missing and the author edits the app")
			Expect(k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "gp-hold-src", Namespace: namespace}})).Should(Succeed())
			edited := getApp(key)
			edited.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(
				`{"objects":[{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"gp-hold-cm-renamed"},"data":{"k":"v"}}]}`)}
			Expect(k8sClient.Update(ctx, edited)).Should(Succeed())
			applicationPolicyCache.InvalidateApplication(namespace, app.Name)

			Eventually(func() error {
				_, _ = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: key})
				c := getApp(key).Status.GetCondition(globalPoliciesCondition)
				if c.Status == corev1.ConditionFalse && strings.Contains(c.Message, "gwcp-hold") {
					return nil
				}
				return fmt.Errorf("no held condition naming the global: %v", getApp(key).Status.Conditions)
			}, 30*time.Second, 200*time.Millisecond).Should(Succeed())
			held := getApp(key)
			Expect(held.Status.Phase).Should(Equal(common.ApplicationRunning), "the current revision keeps running")
			Expect(held.Status.LatestRevision.Name).Should(Equal(firstRev), "no revision is cut while the global fails")
			Expect(cmLabel("gp-hold-cm")).Should(Equal("v1"), "the current revision keeps its resources")
			Expect(cmLabel("gp-hold-cm-renamed")).Should(HavePrefix("<"), "the author's edit waits for the global")
			events, _ := recorder.GetEventsWithName("gp-hold")
			Expect(events).Should(ContainElement(SatisfyAll(
				HaveField("EventType", "Warning"),
				HaveField("Reason", "GlobalPolicyHeld"))))

			By("the source comes back")
			source("gp-hold-src", "v2")
			applicationPolicyCache.InvalidateApplication(namespace, app.Name)
			Eventually(func() error {
				_, _ = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: key})
				a := getApp(key)
				if deployed(key) && cmLabel("gp-hold-cm-renamed") == "v2" {
					return nil
				}
				return fmt.Errorf("renamed=%q appLabels=%v policies=%v", cmLabel("gp-hold-cm-renamed"), a.Labels, a.Status.AppliedApplicationPolicies)
			}, 30*time.Second, 200*time.Millisecond).Should(Succeed())
			Expect(getApp(key).Status.GetCondition(globalPoliciesCondition).Status).ShouldNot(Equal(corev1.ConditionFalse))
		})
	})
	Context("rollout settings in the policy template", func() {
		taggedPolicy := func(name, value, config string) *v1beta1.PolicyDefinition {
			pd := labelPolicy(name, value)
			pd.Spec.Schematic.CUE.Template = config + "\n" + pd.Spec.Schematic.CUE.Template
			return pd
		}
		editTemplate := func(name, value, config string) {
			editGlobal(name, config+"\n"+fmt.Sprintf("parameter: {}\noutput: labels: {\"gwcp/tag\": %q}\n", value))
		}

		It("the live version's applyPolicyUpdates decides whether it is adopted", func() {
			createPolicyDefAndIndex(ctx, taggedPolicy("tpl-live", "v1", ""))
			app := newConfigMapApp("tpl-live", nil)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			key := client.ObjectKeyFromObject(app)
			reconcileUntil(key, func() bool { return deployed(key) && cmLabel("tpl-live-cm") == "v1" })

			editTemplate("tpl-live", "v2", "config: applyPolicyUpdates: true")
			reconcileUntil(key, func() bool { return cmLabel("tpl-live-cm") == "v2" && deployed(key) })
		})

		It("the app annotation overrides the template", func() {
			createPolicyDefAndIndex(ctx, taggedPolicy("tpl-override", "v1", "config: applyPolicyUpdates: true"))
			app := newConfigMapApp("tpl-override", map[string]string{oam.AnnotationApplyPolicyUpdates: "false"})
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			key := client.ObjectKeyFromObject(app)
			reconcileUntil(key, func() bool { return deployed(key) && cmLabel("tpl-override-cm") == "v1" })
			firstRev := getApp(key).Status.LatestRevision.Name

			editTemplate("tpl-override", "v2", "config: applyPolicyUpdates: true")
			for i := 0; i < 5; i++ {
				_, _ = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: key})
			}
			Expect(getApp(key).Status.LatestRevision.Name).Should(Equal(firstRev))
			Expect(cmLabel("tpl-override-cm")).Should(Equal("v1"))
		})

		It("a template can decide per app from its context", func() {
			cfg := `config: applyPolicyUpdates: context.appName == "tpl-ctx-a"`
			createPolicyDefAndIndex(ctx, taggedPolicy("tpl-ctx", "v1", cfg))
			appA, appB := newConfigMapApp("tpl-ctx-a", nil), newConfigMapApp("tpl-ctx-b", nil)
			Expect(k8sClient.Create(ctx, appA)).Should(Succeed())
			Expect(k8sClient.Create(ctx, appB)).Should(Succeed())
			keyA, keyB := client.ObjectKeyFromObject(appA), client.ObjectKeyFromObject(appB)
			reconcileUntil(keyA, func() bool { return deployed(keyA) && cmLabel("tpl-ctx-a-cm") == "v1" })
			reconcileUntil(keyB, func() bool { return deployed(keyB) && cmLabel("tpl-ctx-b-cm") == "v1" })

			editTemplate("tpl-ctx", "v2", cfg)
			reconcileUntil(keyA, func() bool { return cmLabel("tpl-ctx-a-cm") == "v2" && deployed(keyA) })
			for i := 0; i < 5; i++ {
				_, _ = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: keyB})
			}
			Expect(cmLabel("tpl-ctx-b-cm")).Should(Equal("v1"), "the template holds the update for this app")
		})

		It("a template's applyDataChanges applies data changes, and is recorded for vela policy view", func() {
			src := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "tpl-data-src", Namespace: namespace}, Data: map[string]string{"tag": "v1"}}
			Expect(k8sClient.Create(ctx, src)).Should(Succeed())
			pd := labelPolicy("tpl-data", "unused")
			pd.Spec.Schematic.CUE.Template = `
import "vela/kube"

parameter: {}
config: applyDataChanges: true
_source: kube.#Get & {
  $params: resource: {apiVersion: "v1", kind: "ConfigMap", metadata: {name: "tpl-data-src", namespace: "` + namespace + `"}}
}
output: labels: {"gwcp/tag": _source.$returns.data.tag}
`
			createPolicyDefAndIndex(ctx, pd)
			app := newConfigMapApp("tpl-data", nil)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			key := client.ObjectKeyFromObject(app)
			reconcileUntil(key, func() bool { return deployed(key) && cmLabel("tpl-data-cm") == "v1" })

			src.Data["tag"] = "v2"
			Expect(k8sClient.Update(ctx, src)).Should(Succeed())
			applicationPolicyCache.InvalidateApplication(namespace, app.Name)
			reconcileUntil(key, func() bool { return cmLabel("tpl-data-cm") == "v2" })

			Eventually(func() string {
				cms := &corev1.ConfigMapList{}
				_ = k8sClient.List(ctx, cms)
				for _, cm := range cms.Items {
					if cm.Name == policyConfigMapName(namespace, app.Name) {
						for k, v := range cm.Data {
							if strings.HasSuffix(k, "-tpl-data") {
								return v
							}
						}
					}
				}
				return ""
			}, "10s", "200ms").Should(SatisfyAll(
				ContainSubstring(`"settings"`), MatchRegexp(`"applyDataChanges":\s*true`), MatchRegexp(`"applyPolicyUpdates":\s*false`)))
		})
	})
	// The metadata refresh rewrites a policy-owned key on recorded manifests to the policy's value.
	// That is what a re-render produces only because inherited Application labels out-rank labels
	// a trait patches onto the workload. This pins that precedence.
	It("an inherited policy label out-ranks a trait that patches the same key", func() {
		td := &v1beta1.TraitDefinition{}
		Expect(yaml.Unmarshal([]byte(`
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: patch-tag-label
  namespace: vela-system
spec:
  appliesToWorkloads: ["*"]
  schematic:
    cue:
      template: |
        patch: metadata: labels: {"gwcp/tag": parameter.value}
        parameter: value: string
`), td)).Should(Succeed())
		Expect(k8sClient.Create(ctx, td)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		createPolicyDefAndIndex(ctx, labelPolicy("gwcp-precedence", "v1"))
		app := newConfigMapApp("gp-precedence", nil)
		app.Spec.Components[0].Traits = []common.ApplicationTrait{{Type: "patch-tag-label",
			Properties: &runtime.RawExtension{Raw: []byte(`{"value":"from-trait"}`)}}}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		key := client.ObjectKeyFromObject(app)
		reconcileUntil(key, func() bool { return deployed(key) })
		Expect(cmLabel("gp-precedence-cm")).Should(Equal("v1"))
	})
	Context("removing metadata the policies stop setting", func() {
		twoLabels := "parameter: {}\noutput: labels: {\"gwcp/tag\": \"v1\", \"gwcp/extra\": \"x\"}\n"
		oneLabel := "parameter: {}\noutput: labels: {\"gwcp/tag\": \"v1\"}\n"
		appLabel := func(key types.NamespacedName, label string) (string, bool) {
			v, ok := getApp(key).Labels[label]
			return v, ok
		}
		// The deployed resource is a ServiceAccount: KubeVela records no last-applied configuration
		// for ConfigMaps, Secrets and CRDs, so a three-way merge cannot remove keys from those
		// (see the ConfigMap case below).

		// labelOn reports whether the ServiceAccount carries label, and whether it was read.
		labelOn := func(name, label string) (has, found bool) {
			sa := &corev1.ServiceAccount{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, sa); err != nil {
				return false, false
			}
			_, has = sa.Labels[label]
			return has, true
		}
		resourceHas := func(name, label string) bool {
			has, found := labelOn(name, label)
			return found && has
		}
		resourceLacks := func(name, label string) bool {
			has, found := labelOn(name, label)
			return found && !has
		}
		deployWith := func(appName, policyName, template string, annotations map[string]string) types.NamespacedName {
			pd := labelPolicy(policyName, "unused")
			pd.Spec.Schematic.CUE.Template = template
			createPolicyDefAndIndex(ctx, pd)
			app := newConfigMapApp(appName, annotations)
			app.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(fmt.Sprintf(
				`{"objects":[{"apiVersion":"v1","kind":"ServiceAccount","metadata":{"name":%q}}]}`, appName+"-sa"))}
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			key := client.ObjectKeyFromObject(app)
			reconcileUntil(key, func() bool { return deployed(key) && resourceHas(appName+"-sa", "gwcp/extra") })
			return key
		}
		deleteGlobal := func(name string) {
			pd := &v1beta1.PolicyDefinition{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, pd)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, pd)).Should(Succeed())
			policyScopeIndex.Delete(name, namespace)
			applicationPolicyCache.InvalidateForNamespace(namespace)
		}

		It("a key a new policy version stops setting comes off the app and its resources", func() {
			key := deployWith("rm-version", "rm-version", twoLabels, adoptUpdates)
			editGlobal("rm-version", oneLabel)
			reconcileUntil(key, func() bool {
				_, onApp := appLabel(key, "gwcp/extra")
				return !onApp && resourceLacks("rm-version-sa", "gwcp/extra") && deployed(key)
			})
		})

		It("a deleted global's labels come off once the deletion is adopted", func() {
			key := deployWith("rm-deleted", "rm-deleted", twoLabels, adoptUpdates)
			deleteGlobal("rm-deleted")
			reconcileUntil(key, func() bool {
				_, onApp := appLabel(key, "gwcp/extra")
				return !onApp && resourceLacks("rm-deleted-sa", "gwcp/extra") && resourceLacks("rm-deleted-sa", "gwcp/tag") && deployed(key)
			})
		})

		It("a deleted global whose template sets applyPolicyUpdates: true has its deletion adopted", func() {
			key := deployWith("rm-tpl", "rm-tpl", "config: applyPolicyUpdates: true\n"+twoLabels, nil)
			deleteGlobal("rm-tpl")
			reconcileUntil(key, func() bool {
				_, onApp := appLabel(key, "gwcp/extra")
				return !onApp && resourceLacks("rm-tpl-sa", "gwcp/extra") && deployed(key)
			})
		})

		It("by default a deleted global's labels stay until the next revision", func() {
			key := deployWith("rm-held", "rm-held", twoLabels, nil)
			deleteGlobal("rm-held")
			for i := 0; i < 5; i++ {
				_, _ = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: key})
			}
			_, onApp := appLabel(key, "gwcp/extra")
			Expect(onApp).Should(BeTrue())
			Expect(resourceHas("rm-held-sa", "gwcp/extra")).Should(BeTrue())
		})

		It("a key the data no longer yields comes off resources without a revision", func() {
			src := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "rm-data-src", Namespace: namespace},
				Data: map[string]string{"gwcp-tag": "v1", "gwcp-extra": "x"}}
			Expect(k8sClient.Create(ctx, src)).Should(Succeed())
			template := `
import "vela/kube"

parameter: {}
_source: kube.#Get & {
  $params: resource: {apiVersion: "v1", kind: "ConfigMap", metadata: {name: "rm-data-src", namespace: "` + namespace + `"}}
}
output: labels: {
  "gwcp/tag": _source.$returns.data["gwcp-tag"]
  if _source.$returns.data["gwcp-extra"] != _|_ {"gwcp/extra": _source.$returns.data["gwcp-extra"]}
}
`
			key := deployWith("rm-data", "rm-data", template, map[string]string{oam.AnnotationApplyDataChanges: "true"})
			firstRev := getApp(key).Status.LatestRevision.Name

			delete(src.Data, "gwcp-extra")
			Expect(k8sClient.Update(ctx, src)).Should(Succeed())
			applicationPolicyCache.InvalidateApplication(namespace, key.Name)
			reconcileUntil(key, func() bool {
				_, onApp := appLabel(key, "gwcp/extra")
				return !onApp && resourceLacks("rm-data-sa", "gwcp/extra")
			})
			Expect(getApp(key).Status.LatestRevision.Name).Should(Equal(firstRev))
		})

		It("a key someone changed since is left alone", func() {
			key := deployWith("rm-changed", "rm-changed", twoLabels, adoptUpdates)
			edited := getApp(key)
			edited.Labels["gwcp/extra"] = "mine"
			Expect(k8sClient.Update(ctx, edited)).Should(Succeed())
			editGlobal("rm-changed", oneLabel)
			reconcileUntil(key, func() bool { return deployed(key) && getApp(key).Status.LatestRevision.Name != "rm-changed-v1" })
			v, onApp := appLabel(key, "gwcp/extra")
			Expect(onApp).Should(BeTrue())
			Expect(v).Should(Equal("mine"))
		})

		It("a ConfigMap keeps a removed key: KubeVela records no last-applied configuration for it", func() {
			pd := labelPolicy("rm-configmap", "unused")
			pd.Spec.Schematic.CUE.Template = twoLabels
			createPolicyDefAndIndex(ctx, pd)
			app := newConfigMapApp("rm-configmap", adoptUpdates)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			key := client.ObjectKeyFromObject(app)
			hasExtra := func() bool {
				cm := &corev1.ConfigMap{}
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "rm-configmap-cm", Namespace: namespace}, cm)).Should(Succeed())
				_, ok := cm.Labels["gwcp/extra"]
				return ok
			}
			reconcileUntil(key, func() bool { return deployed(key) && hasExtra() })

			editGlobal("rm-configmap", oneLabel)
			reconcileUntil(key, func() bool {
				_, onApp := appLabel(key, "gwcp/extra")
				return !onApp && deployed(key) && getApp(key).Status.LatestRevision.Name != "rm-configmap-v1"
			})
			Expect(hasExtra()).Should(BeTrue(), "known limitation: removed keys stay on ConfigMaps, Secrets and CRDs")
		})

		It("without a record nothing is removed", func() {
			key := deployWith("rm-norecord", "rm-norecord", twoLabels, adoptUpdates)
			Expect(k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name: policyConfigMapName(namespace, key.Name), Namespace: namespace}})).Should(Succeed())
			editGlobal("rm-norecord", oneLabel)
			reconcileUntil(key, func() bool { return deployed(key) && getApp(key).Status.LatestRevision.Name != "rm-norecord-v1" })
			_, onApp := appLabel(key, "gwcp/extra")
			Expect(onApp).Should(BeTrue(), "keys set before the record existed are not known to be the policies'")
		})
	})
	Context("with global policies restricted to vela-system (the default)", func() {
		BeforeEach(func() {
			common2.AllowNamespaceGlobalPolicies = false
			DeferCleanup(func() { common2.AllowNamespaceGlobalPolicies = true })
		})

		It("a global outside vela-system is ignored", func() {
			createPolicyDefAndIndex(ctx, labelPolicy("gwcp-ns-global", "tenant"))
			app := newConfigMapApp("gp-ns-global", nil)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			key := client.ObjectKeyFromObject(app)
			reconcileUntil(key, func() bool { return deployed(key) })
			Expect(getApp(key).Labels).ShouldNot(HaveKey("gwcp/tag"))
			Expect(cmLabel("gp-ns-global-cm")).Should(BeEmpty())
		})

		It("an existing namespace global comes off once restricted, when updates are applied", func() {
			common2.AllowNamespaceGlobalPolicies = true
			createPolicyDefAndIndex(ctx, labelPolicy("gwcp-ns-existing", "tenant"))
			app := newConfigMapApp("gp-ns-existing", map[string]string{oam.AnnotationApplyPolicyUpdates: "true"})
			app.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(
				`{"objects":[{"apiVersion":"v1","kind":"ServiceAccount","metadata":{"name":"gp-ns-existing-sa"}}]}`)}
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			key := client.ObjectKeyFromObject(app)
			saLabel := func() string {
				sa := &corev1.ServiceAccount{}
				_ = k8sClient.Get(ctx, client.ObjectKey{Name: "gp-ns-existing-sa", Namespace: namespace}, sa)
				return sa.Labels["gwcp/tag"]
			}
			reconcileUntil(key, func() bool { return deployed(key) && saLabel() == "tenant" })

			By("the controller now restricts global policies to vela-system")
			common2.AllowNamespaceGlobalPolicies = false
			applicationPolicyCache.InvalidateForNamespace(namespace)
			reconcileUntil(key, func() bool {
				_, onApp := getApp(key).Labels["gwcp/tag"]
				return !onApp && saLabel() == "" && deployed(key)
			})
		})
	})
	It("a skip-global annotation is ignored when global policies are mandatory (the default)", func() {
		common2.AllowSkipGlobalPolicies = false
		DeferCleanup(func() { common2.AllowSkipGlobalPolicies = true })
		createPolicyDefAndIndex(ctx, labelPolicy("gwcp-mandatory", "platform"))
		app := newConfigMapApp("gp-mandatory", map[string]string{oam.AnnotationSkipGlobalPolicies: "true"})
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		key := client.ObjectKeyFromObject(app)
		reconcileUntil(key, func() bool { return deployed(key) && cmLabel("gp-mandatory-cm") == "platform" })
	})
	It("a mandatory global that fails to render creates no untagged resources", func() {
		common2.AllowSkipGlobalPolicies = false
		DeferCleanup(func() { common2.AllowSkipGlobalPolicies = true })
		broken := labelPolicy("gwcp-broken-mandatory", "unused")
		broken.Spec.Schematic.CUE.Template = "parameter: {}\noutput: labels: {\"gwcp/tag\": undefinedReference}\n"
		createPolicyDefAndIndex(ctx, broken)
		app := newConfigMapApp("gp-broken-mandatory", nil)
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		key := client.ObjectKeyFromObject(app)
		Eventually(func() string {
			_, _ = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: key})
			for _, c := range getApp(key).Status.Conditions {
				if c.Status == corev1.ConditionFalse && strings.Contains(c.Message, "mandatory") {
					return c.Message
				}
			}
			return ""
		}, 30*time.Second, 200*time.Millisecond).ShouldNot(BeEmpty())
		Expect(cmLabel("gp-broken-mandatory-cm")).Should(HavePrefix("<"), "nothing is deployed")
	})
})
