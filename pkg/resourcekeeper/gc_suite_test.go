/*
Copyright 2021 The KubeVela Authors.

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

package resourcekeeper

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/util/rand"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/version"
)

var _ = Describe("Test ResourceKeeper garbage collection", func() {

	var namespace string

	BeforeEach(func() {
		namespace = "test-ns-" + rand.RandomString(4)
		Expect(testClient.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).Should(Succeed())
	})

	AfterEach(func() {
		ns := &corev1.Namespace{}
		Expect(testClient.Get(context.Background(), types.NamespacedName{Name: namespace}, ns)).Should(Succeed())
		Expect(testClient.Delete(context.Background(), ns)).Should(Succeed())
	})

	It("Test gcHandler garbage collect legacy RT", func() {
		defer featuregatetesting.SetFeatureGateDuringTest(&testing.T{}, utilfeature.DefaultFeatureGate, features.LegacyResourceTrackerGC, true)()
		version.VelaVersion = velaVersionNumberToUpgradeResourceTracker
		ctx := context.Background()
		cli := multicluster.NewFakeClient(testClient)
		cli.AddCluster("worker", workerClient)
		cli.AddCluster("worker-2", workerClient)
		app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "gc-app", Namespace: namespace}}
		bs, err := json.Marshal(&v1alpha1.EnvBindingSpec{
			Envs: []v1alpha1.EnvConfig{{
				Placement: v1alpha1.EnvPlacement{ClusterSelector: &common.ClusterSelector{Name: "worker"}},
			}},
		})
		Expect(err).Should(Succeed())
		meta.AddAnnotations(app, map[string]string{oam.AnnotationKubeVelaVersion: "v1.1.13"})
		app.Spec = v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{},
			Policies: []v1beta1.AppPolicy{{
				Type:       v1alpha1.EnvBindingPolicyType,
				Properties: &runtime.RawExtension{Raw: bs},
			}},
		}
		app.Status.AppliedResources = []common.ClusterObjectReference{{
			Cluster: "worker-2",
		}}
		Expect(cli.Create(ctx, app)).Should(Succeed())
		keeper := &resourceKeeper{Client: cli, app: app}
		h := gcHandler{resourceKeeper: keeper}
		rt := &v1beta1.ResourceTracker{}
		rt.SetName("gc-app-rt-v1-" + namespace)
		rt.SetLabels(map[string]string{
			oam.LabelAppName:      h.app.Name,
			oam.LabelAppNamespace: h.app.Namespace,
		})
		rt3 := rt.DeepCopy()
		rt4 := rt.DeepCopy()
		rt5 := rt.DeepCopy()
		rt4.SetName("gc-app-rt-v2-" + namespace)
		Expect(cli.Create(ctx, rt)).Should(Succeed())
		rt2 := &v1beta1.ResourceTracker{}
		rt2.Spec.Type = v1beta1.ResourceTrackerTypeVersioned
		rt2.SetName("gc-app-rt-v2-" + namespace)
		rt2.SetLabels(map[string]string{
			oam.LabelAppName:      h.app.Name,
			oam.LabelAppNamespace: h.app.Namespace,
		})
		Expect(cli.Create(ctx, rt2)).Should(Succeed())
		Expect(h.GarbageCollectLegacyResourceTrackers(ctx)).Should(Succeed())
		Expect(cli.Create(multicluster.ContextWithClusterName(ctx, "worker"), rt3)).Should(Succeed())
		Expect(cli.Create(multicluster.ContextWithClusterName(ctx, "worker-2"), rt4)).Should(Succeed())

		checkRTExists := func(_ctx context.Context, name string, exists bool) {
			_rt := &v1beta1.ResourceTracker{}
			err := cli.Get(_ctx, types.NamespacedName{Name: name}, _rt)
			if exists {
				Expect(err).Should(Succeed())
			} else {
				Expect(errors.IsNotFound(err)).Should(BeTrue())
			}
		}

		Expect(h.GarbageCollectLegacyResourceTrackers(ctx)).Should(Succeed())
		checkRTExists(ctx, rt.GetName(), true)
		checkRTExists(ctx, rt2.GetName(), true)
		checkRTExists(multicluster.ContextWithClusterName(ctx, "worker"), rt3.GetName(), true)
		checkRTExists(multicluster.ContextWithClusterName(ctx, "worker-2"), rt4.GetName(), true)

		h.resourceKeeper._currentRT = rt2
		Expect(h.GarbageCollectLegacyResourceTrackers(ctx)).Should(Succeed())
		checkRTExists(ctx, rt.GetName(), false)
		checkRTExists(ctx, rt2.GetName(), true)
		checkRTExists(multicluster.ContextWithClusterName(ctx, "worker"), rt3.GetName(), false)
		checkRTExists(multicluster.ContextWithClusterName(ctx, "worker-2"), rt4.GetName(), false)
		Expect(app.GetAnnotations()[oam.AnnotationKubeVelaVersion]).Should(Equal("v1.2.0"))

		crd := &apiextensionsv1.CustomResourceDefinition{}
		Expect(workerClient.Get(ctx, types.NamespacedName{Name: "resourcetrackers.core.oam.dev"}, crd)).Should(Succeed())
		Expect(workerClient.Delete(ctx, crd)).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(workerClient.List(ctx, &v1beta1.ResourceTrackerList{})).ShouldNot(Succeed())
		}, 10*time.Second).Should(Succeed())
		metav1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationKubeVelaVersion, "master")
		version.VelaVersion = "master"
		Expect(cli.Update(ctx, app)).Should(Succeed())
		Expect(h.GarbageCollectLegacyResourceTrackers(ctx)).Should(Succeed())
		Expect(app.GetAnnotations()[oam.AnnotationKubeVelaVersion]).Should(Equal("v1.2.0"))

		Expect(cli.Create(ctx, rt5)).Should(Succeed())
		Expect(h.GarbageCollectLegacyResourceTrackers(ctx)).Should(Succeed())
		checkRTExists(ctx, rt5.GetName(), true)
	})

	It("Test gcHandler garbage collect shared resources", func() {
		ctx := context.Background()
		cli := testClient
		app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: namespace}}

		keeper := &resourceKeeper{
			Client:     cli,
			app:        app,
			applicator: apply.NewAPIApplicator(cli),
			cache:      newResourceCache(cli, app),
		}
		h := gcHandler{resourceKeeper: keeper, cfg: newGCConfig()}
		h._currentRT = &v1beta1.ResourceTracker{}
		t := metav1.Now()
		h._currentRT.SetDeletionTimestamp(&t)
		h._currentRT.SetFinalizers([]string{resourcetracker.Finalizer})
		createResource := func(name, appName, appNs, sharedBy string) *unstructured.Unstructured {
			return &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":        name,
					"namespace":   namespace,
					"annotations": map[string]interface{}{oam.AnnotationAppSharedBy: sharedBy},
					"labels": map[string]interface{}{
						oam.LabelAppName:      appName,
						oam.LabelAppNamespace: appNs,
					},
				},
			}}
		}
		By("Test delete normal resource")
		o1 := createResource("o1", "app", namespace, "")
		h._currentRT.AddManagedResource(o1, false, false, "test")
		Expect(cli.Create(ctx, o1)).Should(Succeed())
		h.cache.registerResourceTrackers(h._currentRT)
		Expect(h.Finalize(ctx)).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(cli.Get(ctx, client.ObjectKeyFromObject(o1), o1)).Should(Satisfy(errors.IsNotFound))
		}, 5*time.Second).Should(Succeed())

		By("Test delete resource shared by others")
		o2 := createResource("o2", "app", namespace, fmt.Sprintf("%s/app,x/y", namespace))
		h._currentRT.AddManagedResource(o2, false, false, "test")
		Expect(cli.Create(ctx, o2)).Should(Succeed())
		h.cache.registerResourceTrackers(h._currentRT)
		Expect(h.Finalize(ctx)).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(cli.Get(ctx, client.ObjectKeyFromObject(o2), o2)).Should(Succeed())
			g.Expect(o2.GetAnnotations()[oam.AnnotationAppSharedBy]).Should(Equal("x/y"))
			g.Expect(o2.GetLabels()[oam.LabelAppNamespace]).Should(Equal("x"))
			g.Expect(o2.GetLabels()[oam.LabelAppName]).Should(Equal("y"))
		}, 5*time.Second).Should(Succeed())

		By("Test delete resource shared by self")
		o3 := createResource("o3", "app", namespace, fmt.Sprintf("%s/app", namespace))
		h._currentRT.AddManagedResource(o3, false, false, "test")
		Expect(cli.Create(ctx, o3)).Should(Succeed())
		h.cache.registerResourceTrackers(h._currentRT)
		Expect(h.Finalize(ctx)).Should(Succeed())
		Eventually(func(g Gomega) {
			Expect(cli.Get(ctx, client.ObjectKeyFromObject(o3), o3)).Should(Satisfy(errors.IsNotFound))
		}, 5*time.Second).Should(Succeed())
	})

	It("Test gc same cluster-scoped resource but legacy resource recorded with namespace", func() {
		ctx := context.Background()
		cr := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"name": "test-cluster-scoped-resource",
				"labels": map[string]interface{}{
					oam.LabelAppName:      "app",
					oam.LabelAppNamespace: namespace,
				},
			},
		}}
		Expect(testClient.Create(ctx, cr)).Should(Succeed())
		app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: namespace}}
		keeper := &resourceKeeper{
			Client:     testClient,
			app:        app,
			applicator: apply.NewAPIApplicator(testClient),
			cache:      newResourceCache(testClient, app),
		}
		h := gcHandler{resourceKeeper: keeper, cfg: newGCConfig()}
		h._currentRT = &v1beta1.ResourceTracker{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster-scoped-resource-v2"}}
		Expect(testClient.Create(ctx, h._currentRT)).Should(Succeed())
		h._historyRTs = []*v1beta1.ResourceTracker{{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster-scoped-resource-v1"}}}
		t := metav1.Now()
		h._historyRTs[0].SetDeletionTimestamp(&t)
		h._historyRTs[0].SetFinalizers([]string{resourcetracker.Finalizer})
		h._currentRT.AddManagedResource(cr, true, false, "")
		_cr := cr.DeepCopy()
		_cr.SetNamespace(namespace)
		h._historyRTs[0].AddManagedResource(_cr, true, false, "")
		h.Init()
		Expect(h.Finalize(ctx)).Should(Succeed())
		Expect(testClient.Get(ctx, client.ObjectKeyFromObject(cr), &rbacv1.ClusterRole{})).Should(Succeed())
		h._currentRT.Spec.ManagedResources[0].Name = "not-equal"
		keeper.cache = newResourceCache(testClient, app)
		h.Init()
		Expect(h.Finalize(ctx)).Should(Succeed())
		Expect(testClient.Get(ctx, client.ObjectKeyFromObject(cr), &rbacv1.ClusterRole{})).Should(Satisfy(errors.IsNotFound))
	})

})
