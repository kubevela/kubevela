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

package e2e_multicluster_test

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// nestedApp is a parent Application that deploys, through a topology policy on
// the local cluster, a child Application holding one ConfigMap. childClusters
// is the child's own topology; empty leaves the child without one.
// objectCluster, when set, is the ConfigMap's own app.oam.dev/cluster label.
func nestedApp(namespace, name string, childClusters []string, objectCluster string) *v1beta1.Application {
	cmMeta := map[string]interface{}{"name": "cm-" + name, "namespace": namespace}
	if objectCluster != "" {
		cmMeta["labels"] = map[string]interface{}{oam.LabelAppCluster: objectCluster}
	}
	childSpec := map[string]interface{}{"components": []interface{}{map[string]interface{}{
		"name": "cm", "type": "k8s-objects",
		"properties": map[string]interface{}{"objects": []interface{}{map[string]interface{}{
			"apiVersion": "v1", "kind": "ConfigMap", "metadata": cmMeta, "data": map[string]interface{}{"app": name},
		}}},
	}}}
	if len(childClusters) > 0 {
		childSpec["policies"] = []interface{}{map[string]interface{}{
			"name": "child-topology", "type": "topology",
			"properties": map[string]interface{}{"clusters": childClusters},
		}}
	}
	child, err := json.Marshal(map[string]interface{}{"objects": []interface{}{map[string]interface{}{
		"apiVersion": "core.oam.dev/v1beta1", "kind": "Application",
		"metadata": map[string]interface{}{"name": "child-" + name, "namespace": namespace},
		"spec":     childSpec,
	}}})
	Expect(err).Should(Succeed())
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "parent-" + name},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{
				Name:       "child",
				Type:       "k8s-objects",
				Properties: &runtime.RawExtension{Raw: child},
			}},
			Policies: []v1beta1.AppPolicy{{
				Name:       "parent-on-hub",
				Type:       "topology",
				Properties: &runtime.RawExtension{Raw: []byte(`{"clusters":["local"]}`)},
			}},
		},
	}
}

func configMapExists(ctx context.Context, g Gomega, namespace, name string) bool {
	err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &corev1.ConfigMap{})
	if kerrors.IsNotFound(err) {
		return false
	}
	g.Expect(err).Should(Succeed())
	return true
}

// A parent that deploys a child Application through a topology policy stamps
// app.oam.dev/cluster on the child. The child must not pass that label on to
// its own objects: dispatch only sets the cluster on an object that has none,
// so an inherited value would override the child's own topology.
var _ = Describe("Test multicluster nested Application", func() {
	var namespace string
	var hubCtx context.Context
	var workerCtx context.Context

	BeforeEach(func() {
		hubCtx, workerCtx, namespace = initializeContextAndNamespace()
	})

	AfterEach(func() {
		apps := &v1beta1.ApplicationList{}
		Expect(k8sClient.List(hubCtx, apps)).Should(Succeed())
		for i := range apps.Items {
			if apps.Items[i].Namespace == namespace {
				_ = k8sClient.Delete(hubCtx, apps.Items[i].DeepCopy())
			}
		}
		cleanUpNamespace(hubCtx, workerCtx, namespace)
	})

	It("deploys a nested Application's objects by its own topology, not its parent's cluster", func() {
		Expect(k8sClient.Create(hubCtx, nestedApp(namespace, "own-topology", []string{WorkerClusterName}, ""))).Should(Succeed())

		child := types.NamespacedName{Namespace: namespace, Name: "child-own-topology"}
		Eventually(func(g Gomega) {
			app := &v1beta1.Application{}
			g.Expect(k8sClient.Get(hubCtx, child, app)).Should(Succeed())
			g.Expect(app.Labels[oam.LabelAppCluster]).Should(Equal("local"), "the parent's topology labels the child")
			g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
			g.Expect(configMapExists(workerCtx, g, namespace, "cm-own-topology")).Should(BeTrue(), "the child's topology targets the worker")
		}, 2*time.Minute, time.Second).Should(Succeed())
		Consistently(func(g Gomega) {
			g.Expect(configMapExists(hubCtx, g, namespace, "cm-own-topology")).Should(BeFalse(), "nothing is placed by the parent's label")
		}, 10*time.Second, time.Second).Should(Succeed())
	})

	It("keeps a nested Application without a topology on its own cluster", func() {
		Expect(k8sClient.Create(hubCtx, nestedApp(namespace, "no-topology", nil, ""))).Should(Succeed())

		Eventually(func(g Gomega) {
			app := &v1beta1.Application{}
			g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "child-no-topology"}, app)).Should(Succeed())
			g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
			g.Expect(configMapExists(hubCtx, g, namespace, "cm-no-topology")).Should(BeTrue())
		}, 2*time.Minute, time.Second).Should(Succeed())
		Consistently(func(g Gomega) {
			g.Expect(configMapExists(workerCtx, g, namespace, "cm-no-topology")).Should(BeFalse())
		}, 10*time.Second, time.Second).Should(Succeed())
	})

	It("keeps a cluster label an object sets on itself", func() {
		Expect(k8sClient.Create(hubCtx, nestedApp(namespace, "own-label", nil, WorkerClusterName))).Should(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(configMapExists(workerCtx, g, namespace, "cm-own-label")).Should(BeTrue(), "the object's own label is not replaced by the parent's")
		}, 2*time.Minute, time.Second).Should(Succeed())
		Expect(configMapExists(hubCtx, Default, namespace, "cm-own-label")).Should(BeFalse())
	})
})
