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

	"github.com/kubevela/workflow/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	helmapi "github.com/oam-dev/kubevela/pkg/appfile/helm/flux2apis"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

var _ = BeforeSuite(func(done Done) {
	// env init
	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute * 3,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       pointer.Bool(false),
		CRDDirectoryPaths: []string{
			"../../../../charts/vela-core/crds",
		},
	}
	// env start
	By("start kube test env")
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())
	// client start
	By("new kube client")
	cfg.Timeout = time.Minute * 2
	k8sClient, err = client.New(cfg, client.Options{Scheme: common.Scheme})
	Expect(err).Should(BeNil())
	Expect(k8sClient).ToNot(BeNil())

	// create namespace
	By("create namespace")
	_ = k8sClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	})
	// create app
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
	err = k8sClient.Create(context.Background(), testApp)
	Expect(err).Should(BeNil())
	testApp.Status = common2.AppStatus{
		Phase: common2.ApplicationRunning,
		Workflow: &common2.WorkflowStatus{
			AppRevision:    "",
			Mode:           "DAG",
			Phase:          "",
			Message:        "",
			Suspend:        false,
			SuspendState:   "",
			Terminated:     false,
			Finished:       false,
			ContextBackend: nil,
			Steps:          []v1alpha1.WorkflowStepStatus{},
			StartTime:      metav1.Time{},
			EndTime:        metav1.Time{},
		},
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
	err = k8sClient.Status().Update(context.Background(), testApp)
	Expect(err).Should(BeNil())
	// create service
	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service1",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Port: 2002}},
		},
	}
	svcRaw, err := json.Marshal(svc)
	Expect(err).Should(Succeed())
	Expect(k8sClient.Create(context.Background(), svc)).Should(BeNil())
	// create deploy
	dply := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deploy1",
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: namespace, Labels: map[string]string{"app": "test"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{
						Name:  "vela-core-1",
						Image: "vela",
					},
				}},
			},
		},
	}
	dplyRaw, err := json.Marshal(dply)
	Expect(err).Should(Succeed())
	Expect(k8sClient.Create(context.Background(), dply)).Should(BeNil())
	//create replicaSet
	var rsNum int32 = 2
	rs := &appsv1.ReplicaSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ReplicaSet",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rs1",
			Namespace: namespace,
			Labels:    map[string]string{"app": "test"},
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: &rsNum,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: namespace, Labels: map[string]string{"app": "test"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{
						Name:  "vela-core-1",
						Image: "vela",
					},
				}},
			},
		},
	}
	rsRaw, err := json.Marshal(rs)
	Expect(err).Should(Succeed())
	Expect(k8sClient.Create(context.Background(), rs)).Should(BeNil())
	// create pod
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod1",
			Namespace: namespace,
			Labels:    map[string]string{"app": "test"},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{
				Name:  "vela-core-1",
				Image: "vela",
			},
		}},
	}
	podRaw, err := json.Marshal(pod)
	Expect(err).Should(Succeed())
	Expect(k8sClient.Create(context.Background(), pod)).Should(BeNil())
	// create resourceTracker
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
							Name:       "service1",
						},
					},
					OAMObjectReference: common2.OAMObjectReference{
						Component: "service1",
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
							Name:       "deploy1",
						},
					},
					OAMObjectReference: common2.OAMObjectReference{
						Component: "deploy1",
					},
					Data: &runtime.RawExtension{Raw: dplyRaw},
				},
				{
					ClusterObjectReference: common2.ClusterObjectReference{
						Cluster: "",
						ObjectReference: corev1.ObjectReference{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Namespace:  namespace,
							Name:       "rs1",
						},
					},
					OAMObjectReference: common2.OAMObjectReference{
						Component: "rs1",
					},
					Data: &runtime.RawExtension{Raw: rsRaw},
				},
				{
					ClusterObjectReference: common2.ClusterObjectReference{
						Cluster: "",
						ObjectReference: corev1.ObjectReference{
							APIVersion: "v1",
							Kind:       "Pod",
							Namespace:  namespace,
							Name:       "pod1",
						},
					},
					OAMObjectReference: common2.OAMObjectReference{
						Component: "pod1",
					},
					Data: &runtime.RawExtension{Raw: podRaw},
				},
			},
			Type: v1beta1.ResourceTrackerTypeVersioned,
		},
	}
	Expect(k8sClient.Create(context.Background(), rt)).Should(BeNil())

	err = k8sClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: types.DefaultKubeVelaNS,
		},
	})
	Expect(err).Should(BeNil())

	quantityLimitsCPU, _ := resource.ParseQuantity("10m")
	quantityLimitsMemory, _ := resource.ParseQuantity("10Mi")
	quantityRequestsCPU, _ := resource.ParseQuantity("10m")
	quantityRequestsMemory, _ := resource.ParseQuantity("10Mi")

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
	Expect(k8sClient.Create(context.Background(), pod1)).Should(BeNil())
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
	Expect(k8sClient.Create(context.Background(), pod2)).Should(BeNil())

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
