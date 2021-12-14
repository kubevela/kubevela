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

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/version"
)

var _ = Describe("Test ResourceKeeper garbage collection", func() {

	var namespace string

	BeforeEach(func() {
		namespace = "test-ns-" + utils.RandomString(4)
		Expect(testClient.Create(context.Background(), &v1.Namespace{ObjectMeta: v12.ObjectMeta{Name: namespace}})).Should(Succeed())
	})

	AfterEach(func() {
		ns := &v1.Namespace{}
		Expect(testClient.Get(context.Background(), types.NamespacedName{Name: namespace}, ns)).Should(Succeed())
		Expect(testClient.Delete(context.Background(), ns)).Should(Succeed())
	})

	It("Test gcHandler garbage collect legacy RT", func() {
		if version.VelaVersion == "UNKNOWN" {
			version.VelaVersion = velaVersionNumberToUpgradeResourceTracker
		}
		ctx := context.Background()
		cli := multicluster.NewFakeClient(testClient)
		cli.AddCluster("worker", workerClient)
		cli.AddCluster("worker-2", workerClient)
		app := &v1beta1.Application{ObjectMeta: v12.ObjectMeta{Name: "gc-app", Namespace: namespace}}
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
		Expect(cli.Create(ctx, rt5)).Should(Succeed())
		Expect(h.GarbageCollectLegacyResourceTrackers(ctx)).Should(Succeed())
		checkRTExists(ctx, rt5.GetName(), true)

		meta.AddAnnotations(app, map[string]string{oam.AnnotationKubeVelaVersion: "UNKNOWN"})
		Expect(h.GarbageCollectLegacyResourceTrackers(ctx)).Should(Succeed())
		checkRTExists(ctx, rt5.GetName(), true)
	})

})
