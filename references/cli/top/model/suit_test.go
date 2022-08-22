/*
Copyright 2022 The KubeVela Authors.

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

package model

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	helmapi "github.com/oam-dev/kubevela/pkg/appfile/helm/flux2apis"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

var createObject = func(name string, ns string, value string, kind string) *unstructured.Unstructured {
	o := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"data": map[string]interface{}{
				"key": value,
			},
		},
	}
	o.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind(kind))
	return o
}

var _ = BeforeSuite(func(done Done) {
	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute * 3,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       pointer.BoolPtr(false),
		CRDDirectoryPaths: []string{
			"../../../../charts/vela-core/crds",
		},
	}

	By("start kube test env")
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	By("new kube client")
	cfg.Timeout = time.Minute * 2
	k8sClient, err = client.New(cfg, client.Options{Scheme: common.Scheme})
	Expect(err).Should(BeNil())
	Expect(k8sClient).ToNot(BeNil())

	name, namespace := "first-vela-app", "default"
	testApp := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common2.ApplicationComponent{
				{
					Name: "webservice-test",
					Type: "webservice",
				},
			},
		},
	}
	err = k8sClient.Create(context.TODO(), testApp)
	Expect(err).Should(BeNil())
	testApp.Status = common2.AppStatus{
		Phase: common2.ApplicationRunning,
		AppliedResources: []common2.ClusterObjectReference{
			{
				Cluster: "",
				ObjectReference: corev1.ObjectReference{
					Kind:       "Ingress",
					Namespace:  "default",
					Name:       "ingress-http",
					APIVersion: "networking.k8s.io/v1beta1",
				},
			},
			{
				Cluster: "",
				ObjectReference: corev1.ObjectReference{
					Kind:       "Ingress",
					Namespace:  "default",
					Name:       "ingress-https",
					APIVersion: "networking.k8s.io/v1",
				},
			},
			{
				Cluster: "",
				ObjectReference: corev1.ObjectReference{
					Kind:       "Ingress",
					Namespace:  "default",
					Name:       "ingress-paths",
					APIVersion: "networking.k8s.io/v1",
				},
			},
			{
				Cluster: "",
				ObjectReference: corev1.ObjectReference{
					Kind:       "Service",
					Namespace:  "default",
					Name:       "nodeport",
					APIVersion: "v1",
				},
			},
			{
				Cluster: "",
				ObjectReference: corev1.ObjectReference{
					Kind:       "Service",
					Namespace:  "default",
					Name:       "loadbalancer",
					APIVersion: "v1",
				},
			},
			{
				Cluster: "",
				ObjectReference: corev1.ObjectReference{
					Kind:      helmapi.HelmReleaseGVK.Kind,
					Namespace: "default",
					Name:      "helmRelease",
				},
			},
		},
	}
	err = k8sClient.Status().Update(context.TODO(), testApp)
	Expect(err).Should(BeNil())

	svc := createObject("service1", namespace, "x", "Service")
	svcRaw, err := json.Marshal(svc)
	Expect(err).Should(Succeed())
	dply := createObject("deploy1", namespace, "y", "Deployment")
	dplyRaw, err := json.Marshal(dply)
	Expect(err).Should(Succeed())

	rt := &v1beta1.ResourceTracker{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-v1-%s", name, namespace),
			Labels: map[string]string{
				oam.LabelAppName:      name,
				oam.LabelAppNamespace: namespace,
			},
			Annotations: map[string]string{
				oam.AnnotationPublishVersion: "v1",
			},
		},
		Spec: v1beta1.ResourceTrackerSpec{
			ManagedResources: []v1beta1.ManagedResource{
				{
					ClusterObjectReference: common2.ClusterObjectReference{
						Cluster: "",
						ObjectReference: corev1.ObjectReference{
							APIVersion: "v1",
							Kind:       "Service",
							Namespace:  namespace,
							Name:       "web",
						},
					},
					OAMObjectReference: common2.OAMObjectReference{
						Component: "web",
					},
					Data: &runtime.RawExtension{Raw: svcRaw},
				},
				{
					ClusterObjectReference: common2.ClusterObjectReference{
						Cluster: "",
						ObjectReference: corev1.ObjectReference{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
							Namespace:  namespace,
							Name:       "web",
						},
					},
					OAMObjectReference: common2.OAMObjectReference{
						Component: "web",
					},
					Data: &runtime.RawExtension{Raw: dplyRaw},
				},
			},
			Type: v1beta1.ResourceTrackerTypeVersioned,
		},
	}
	Expect(k8sClient.Create(context.TODO(), rt)).Should(BeNil())

	err = k8sClient.Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: types.DefaultKubeVelaNS,
		},
	})
	Expect(err).Should(BeNil())

	quantityLimitsCPU, _ := resource.ParseQuantity("10m")
	quantityLimitsMemory, _ := resource.ParseQuantity("10Mi")
	quantityRequestsCPU, _ := resource.ParseQuantity("10m")
	quantityRequestsMemory, _ := resource.ParseQuantity("10Mi")
	//quantityUsageCPU, _ := resource.ParseQuantity("8m")
	//quantityUsageMemory, _ := resource.ParseQuantity("20Mi")

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "vela-core", Namespace: "vela-system", Labels: map[string]string{"app.kubernetes.io/name": "vela-core"}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{
				Name:  "vela-core-1",
				Image: "vela",
				Resources: corev1.ResourceRequirements{
					Requests: map[corev1.ResourceName]resource.Quantity{"memory": quantityRequestsMemory, "cpu": quantityRequestsCPU},
					Limits:   map[corev1.ResourceName]resource.Quantity{"memory": quantityLimitsMemory, "cpu": quantityLimitsCPU},
				},
			},
		}},
	}
	Expect(k8sClient.Create(context.TODO(), pod1)).Should(BeNil())
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "vela-core-cluster-gateway", Namespace: "vela-system", Labels: map[string]string{"app.kubernetes.io/name": "vela-core-cluster-gateway"}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{
				Name:  "vela-core-cluster-gateway-1",
				Image: "vela",
				Resources: corev1.ResourceRequirements{
					Requests: map[corev1.ResourceName]resource.Quantity{"memory": quantityRequestsMemory, "cpu": quantityRequestsCPU},
					Limits:   map[corev1.ResourceName]resource.Quantity{"memory": quantityLimitsMemory, "cpu": quantityLimitsCPU},
				},
			},
		}},
	}
	Expect(k8sClient.Create(context.TODO(), pod2)).Should(BeNil())

	close(done)
}, 240)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

func TestModel(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Model Suite")
}
