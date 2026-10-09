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

package appkeeper

import (
	"context"
	"encoding/json"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/kubevela/pkg/util/rand"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/version"
)

// What the pre-v1.2 clean-up does. When the keeper runs it (only once the Application has a
// current tracker, or while it is being deleted) is the keeper's concern, tested there.
var _ = Describe("Legacy (pre-v1.2) ResourceTracker clean-up", func() {

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

	It("removes untyped trackers in every cluster the app touched, then records the upgrade", func() {
		// Restore the gate and version afterwards: they are process-wide, and the next spec
		// expects the gate at its default.
		Expect(utilfeature.DefaultMutableFeatureGate.Set(string(features.LegacyResourceTrackerGC) + "=true")).Should(Succeed())
		DeferCleanup(func() {
			Expect(utilfeature.DefaultMutableFeatureGate.Set(string(features.LegacyResourceTrackerGC) + "=false")).Should(Succeed())
		})
		DeferCleanup(func(v string) { version.VelaVersion = v }, version.VelaVersion)
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
		rt := &v1beta1.ResourceTracker{}
		rt.SetName("gc-app-rt-v1-" + namespace)
		rt.SetLabels(map[string]string{
			oam.LabelAppName:      app.Name,
			oam.LabelAppNamespace: app.Namespace,
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
			oam.LabelAppName:      app.Name,
			oam.LabelAppNamespace: app.Namespace,
		})
		Expect(cli.Create(ctx, rt2)).Should(Succeed())
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

		By("untyped trackers go, in the local cluster, the env-binding cluster and the applied cluster; typed ones stay")
		Expect(garbageCollectLegacyResourceTrackers(ctx, cli, app)).Should(Succeed())
		checkRTExists(ctx, rt.GetName(), false)
		checkRTExists(ctx, rt2.GetName(), true)
		checkRTExists(multicluster.ContextWithClusterName(ctx, "worker"), rt3.GetName(), false)
		checkRTExists(multicluster.ContextWithClusterName(ctx, "worker-2"), rt4.GetName(), false)
		Expect(app.GetAnnotations()[oam.AnnotationKubeVelaVersion]).Should(Equal("v1.2.0"))

		By("a cluster without the ResourceTracker CRD is skipped, and a non-semver build still records v1.2.0")
		crd := &apiextensionsv1.CustomResourceDefinition{}
		Expect(workerClient.Get(ctx, types.NamespacedName{Name: "resourcetrackers.core.oam.dev"}, crd)).Should(Succeed())
		Expect(workerClient.Delete(ctx, crd)).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(workerClient.List(ctx, &v1beta1.ResourceTrackerList{})).ShouldNot(Succeed())
		}, 10*time.Second).Should(Succeed())
		metav1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationKubeVelaVersion, "master")
		version.VelaVersion = "master"
		Expect(cli.Update(ctx, app)).Should(Succeed())
		Expect(garbageCollectLegacyResourceTrackers(ctx, cli, app)).Should(Succeed())
		Expect(app.GetAnnotations()[oam.AnnotationKubeVelaVersion]).Should(Equal("v1.2.0"))

		By("once upgraded, nothing more is removed")
		Expect(cli.Create(ctx, rt5)).Should(Succeed())
		Expect(garbageCollectLegacyResourceTrackers(ctx, cli, app)).Should(Succeed())
		checkRTExists(ctx, rt5.GetName(), true)
	})

	It("does nothing while its feature gate is off", func() {
		ctx := context.Background()
		app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "gc-app", Namespace: namespace,
			Annotations: map[string]string{oam.AnnotationKubeVelaVersion: "v1.1.13"}},
			Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{}}}
		Expect(testClient.Create(ctx, app)).Should(Succeed())
		rt := &v1beta1.ResourceTracker{}
		rt.SetName("gc-app-rt-off-" + namespace)
		rt.SetLabels(map[string]string{oam.LabelAppName: app.Name, oam.LabelAppNamespace: app.Namespace})
		Expect(testClient.Create(ctx, rt)).Should(Succeed())

		Expect(garbageCollectLegacyResourceTrackers(ctx, testClient, app)).Should(Succeed())
		Expect(testClient.Get(ctx, types.NamespacedName{Name: rt.GetName()}, &v1beta1.ResourceTracker{})).Should(Succeed())
		Expect(app.GetAnnotations()[oam.AnnotationKubeVelaVersion]).Should(Equal("v1.1.13"))
	})
})
