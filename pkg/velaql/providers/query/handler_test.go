/*
 Copyright 2021. The KubeVela Authors.

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

package query

import (
	"context"
	"fmt"
	"os"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/kubevela/pkg/util/singleton"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	monitorContext "github.com/kubevela/pkg/monitor/context"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	helmapi "github.com/oam-dev/kubevela/pkg/appfile/helm/flux2apis"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	verrors "github.com/oam-dev/kubevela/pkg/utils/errors"
	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
)

type AppResourcesList struct {
	List []Resource  `json:"list,omitempty"`
	App  interface{} `json:"app"`
	Err  string      `json:"err,omitempty"`
}

type PodList struct {
	List    []*unstructured.Unstructured `json:"list"`
	Value   interface{}                  `json:"value"`
	Cluster string                       `json:"cluster"`
}

var _ = Describe("Test Query Provider", func() {
	var baseDeploy *v1.Deployment
	var baseService *corev1.Service
	var basePod *corev1.Pod

	BeforeEach(func() {
		baseDeploy = new(v1.Deployment)
		Expect(yaml.Unmarshal([]byte(deploymentYaml), baseDeploy)).Should(BeNil())

		baseService = new(corev1.Service)
		Expect(yaml.Unmarshal([]byte(serviceYaml), baseService)).Should(BeNil())

		basePod = new(corev1.Pod)
		Expect(yaml.Unmarshal([]byte(podYaml), basePod)).Should(BeNil())
	})

	Context("Test ListResourcesInApp", func() {
		It("Test list latest resources created by application", func() {
			namespace := "test"
			ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())

			app := v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
					Annotations: map[string]string{
						oam.AnnotationKubeVelaVersion: "v1.3.1",
						oam.AnnotationPublishVersion:  "v1",
					},
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name: "web",
						Type: "webservice",
						Properties: util.Object2RawExtension(map[string]string{
							"image": "busybox",
						}),
						Traits: []common.ApplicationTrait{{
							Type: "expose",
							Properties: util.Object2RawExtension(map[string]interface{}{
								"ports": []int{8000},
							}),
						}},
					}},
				},
			}

			Expect(k8sClient.Create(ctx, &app)).Should(BeNil())
			oldApp := new(v1beta1.Application)
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&app), oldApp)).Should(BeNil())
			oldApp.Status.LatestRevision = &common.Revision{
				Revision: 1,
			}
			oldApp.Status.AppliedResources = []common.ClusterObjectReference{{
				Cluster: "",
				Creator: "workflow",
				ObjectReference: corev1.ObjectReference{
					APIVersion: "v1",
					Kind:       "Service",
					Namespace:  namespace,
					Name:       "web",
				},
			}, {
				Cluster: "",
				Creator: "workflow",
				ObjectReference: corev1.ObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Namespace:  namespace,
					Name:       "web",
				},
			}}
			Eventually(func() error {
				err := k8sClient.Status().Update(ctx, oldApp)
				if err != nil {
					return err
				}
				return nil
			}, 300*time.Microsecond, 3*time.Second).Should(BeNil())

			appDeploy := baseDeploy.DeepCopy()
			appDeploy.SetName("web")
			appDeploy.SetNamespace(namespace)
			appDeploy.SetLabels(map[string]string{
				oam.LabelAppComponent: "web",
				oam.LabelAppRevision:  "test-v1",
			})
			Expect(k8sClient.Create(ctx, appDeploy)).Should(BeNil())

			appService := baseService.DeepCopy()
			appService.SetName("web")
			appService.SetNamespace(namespace)
			appService.SetLabels(map[string]string{
				oam.LabelAppComponent: "web",
				oam.LabelAppRevision:  "test-v1",
			})
			Expect(k8sClient.Create(ctx, appService)).Should(BeNil())

			rt := &v1beta1.ResourceTracker{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("%s-v1-%s", oldApp.Name, oldApp.Namespace),
					Labels: map[string]string{
						oam.LabelAppName:      oldApp.Name,
						oam.LabelAppNamespace: oldApp.Namespace,
					},
					Annotations: map[string]string{
						oam.AnnotationPublishVersion: "v1",
					},
				},
				Spec: v1beta1.ResourceTrackerSpec{
					ManagedResources: []v1beta1.ManagedResource{
						{
							ClusterObjectReference: common.ClusterObjectReference{
								Cluster: "",
								ObjectReference: corev1.ObjectReference{
									APIVersion: "v1",
									Kind:       "Service",
									Namespace:  namespace,
									Name:       "web",
								},
							},
							OAMObjectReference: common.OAMObjectReference{
								Component: "web",
							},
						},
						{
							ClusterObjectReference: common.ClusterObjectReference{
								Cluster: "",
								ObjectReference: corev1.ObjectReference{
									APIVersion: "apps/v1",
									Kind:       "Deployment",
									Namespace:  namespace,
									Name:       "web",
								},
							},
							OAMObjectReference: common.OAMObjectReference{
								Component: "web",
							},
						},
					},
					Type: v1beta1.ResourceTrackerTypeVersioned,
				},
			}
			Expect(k8sClient.Create(ctx, rt)).Should(BeNil())

			opt := `app: {
				name: "test"
				namespace: "test"
				filter: {
					cluster: "",
					clusterNamespace: "test",
					components: ["web"]
				}
			}`
			v := cuecontext.New().CompileString(opt)
			err := v.Err()
			Expect(err).Should(BeNil())
			logCtx := monitorContext.NewTraceContext(ctx, "")
			v, err = ListResourcesInApp(logCtx, v)
			Expect(err).Should(BeNil())

			appResList := new(AppResourcesList)
			Expect(v.Decode(appResList)).Should(BeNil())
			if appResList.Err != "" {
				klog.Error(appResList.Err)
			}

			Expect(len(appResList.List)).Should(Equal(2))

			Expect(appResList.List[0].Object.GroupVersionKind()).Should(Equal(oldApp.Status.AppliedResources[0].GroupVersionKind()))
			Expect(appResList.List[1].Object.GroupVersionKind()).Should(Equal(oldApp.Status.AppliedResources[1].GroupVersionKind()))

			updateApp := new(v1beta1.Application)
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&app), updateApp)).Should(BeNil())

			updateApp.ObjectMeta.Annotations = map[string]string{
				oam.AnnotationKubeVelaVersion: "v1.1.0",
			}
			Expect(k8sClient.Update(ctx, updateApp)).Should(BeNil())
			newValue := cuecontext.New().CompileString(opt)
			Expect(newValue.Err()).Should(BeNil())
			_, err = ListResourcesInApp(logCtx, newValue)
			Expect(err).Should(BeNil())
			newAppResList := new(AppResourcesList)
			Expect(v.Decode(newAppResList)).Should(BeNil())
			Expect(len(newAppResList.List)).Should(Equal(2))
			Expect(newAppResList.List[0].Object.GroupVersionKind()).Should(Equal(updateApp.Status.AppliedResources[0].GroupVersionKind()))
			Expect(newAppResList.List[1].Object.GroupVersionKind()).Should(Equal(updateApp.Status.AppliedResources[1].GroupVersionKind()))
		})

		It("Test list resource with incomplete parameter", func() {
			optWithoutApp := ""
			newV := cuecontext.New().CompileString(optWithoutApp)
			err := newV.Err()
			Expect(err).Should(BeNil())
			logCtx := monitorContext.NewTraceContext(ctx, "")
			_, err = ListResourcesInApp(logCtx, newV)
			Expect(err).ShouldNot(BeNil())
			Expect(verrors.IsCuePathNotFound(err)).Should(BeTrue())
		})
	})

	Context("Test ListAppliedResources", func() {
		It("Test list applied resources created by application", func() {
			// create test app
			app := v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-applied",
					Namespace: "default",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name: "web",
						Type: "webservice",
						Properties: util.Object2RawExtension(map[string]string{
							"image": "busybox",
						}),
						Traits: []common.ApplicationTrait{{
							Type: "expose",
							Properties: util.Object2RawExtension(map[string]interface{}{
								"ports": []int{8000},
							}),
						}},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, &app)).Should(BeNil())
			// create RT
			rt := &v1beta1.ResourceTracker{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-applied",
					Namespace: "default",
					Labels: map[string]string{
						oam.LabelAppName:      app.Name,
						oam.LabelAppNamespace: app.Namespace,
					},
				},
				Spec: v1beta1.ResourceTrackerSpec{
					Type: v1beta1.ResourceTrackerTypeRoot,
					ManagedResources: []v1beta1.ManagedResource{
						{
							ClusterObjectReference: common.ClusterObjectReference{
								Cluster: "",
								ObjectReference: corev1.ObjectReference{
									Kind:       "Deployment",
									APIVersion: "apps/v1",
									Namespace:  "default",
									Name:       "web",
								},
							},
							OAMObjectReference: common.OAMObjectReference{
								Component: "web",
							},
						},
						{
							ClusterObjectReference: common.ClusterObjectReference{
								Cluster: "",
								ObjectReference: corev1.ObjectReference{
									Kind:       "Service",
									APIVersion: "v1",
									Namespace:  "default",
									Name:       "web",
								},
							},
							OAMObjectReference: common.OAMObjectReference{
								Trait:     "expose",
								Component: "web",
							},
						},
					},
				},
			}
			err := k8sClient.Create(context.TODO(), rt)
			Expect(err).Should(BeNil())
			opt := `app: {
				name: "test-applied"
				namespace: "default"
				filter: {
					components: ["web"]
				}
			}`
			v := cuecontext.New().CompileString(opt)
			err = v.Err()
			Expect(err).Should(BeNil())
			logCtx := monitorContext.NewTraceContext(ctx, "")
			Expect(ListAppliedResources(logCtx, v)).Should(BeNil())
			type Res struct {
				List []v1beta1.ManagedResource `json:"list"`
			}
			var res Res
			err = v.Decode(&res)
			Expect(err).Should(BeNil())
			Expect(len(res.List)).Should(Equal(2))

			By("test filter with the apiVersion and kind")
			optWithVersion := `app: {
				name: "test-applied"
				namespace: "default"
				filter: {
					components: ["web"]
					apiVersion: "apps/v1"
				}
			}`
			valueWithVersion := cuecontext.New().CompileString(optWithVersion)
			err = valueWithVersion.Err()
			Expect(err).Should(BeNil())
			Expect(ListAppliedResources(logCtx, valueWithVersion)).Should(BeNil())
			var res2 Res
			err = valueWithVersion.Decode(&res2)
			Expect(err).Should(BeNil())
			Expect(len(res2.List)).Should(Equal(1))
			Expect(res2.List[0].Kind).Should(Equal("Deployment"))

			optWithKind := `app: {
				name: "test-applied"
				namespace: "default"
				filter: {
					components: ["web"]
					kind: "Service"
				}
			}`
			valueWithKind := cuecontext.New().CompileString(optWithKind)
			err = valueWithKind.Err()
			Expect(err).Should(BeNil())
			Expect(ListAppliedResources(logCtx, valueWithKind)).Should(BeNil())
			var res3 Res
			err = valueWithKind.Decode(&res3)
			Expect(err).Should(BeNil())
			Expect(len(res3.List)).Should(Equal(1))
			Expect(res3.List[0].Kind).Should(Equal("Service"))
		})
	})

	Context("Test search event from k8s object", func() {
		It("Test search event with incomplete parameter", func() {
			emptyOpt := ""
			v := cuecontext.New().CompileString(emptyOpt)
			err := v.Err()
			Expect(err).Should(BeNil())
			logCtx := monitorContext.NewTraceContext(ctx, "")
			_, err = SearchEvents(logCtx, v)
			Expect(err).ShouldNot(BeNil())
			Expect(verrors.IsCuePathNotFound(err)).Should(BeTrue())

			optWithoutCluster := `value: {}`
			v = cuecontext.New().CompileString(optWithoutCluster)
			err = v.Err()
			Expect(err).Should(BeNil())
			_, err = SearchEvents(logCtx, v)
			Expect(err).ShouldNot(BeNil())
			Expect(verrors.IsCuePathNotFound(err)).Should(BeTrue())

			optWithWrongValue := `value: {}
cluster: "test"`
			v = cuecontext.New().CompileString(optWithWrongValue)
			err = v.Err()
			Expect(err).Should(BeNil())
			_, err = SearchEvents(logCtx, v)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("Test CollectLogsInPod", func() {
		It("Test CollectLogsInPod with specified container", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "hello-world", Namespace: "default"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main", Image: "busybox"}},
				}}
			Expect(k8sClient.Create(ctx, pod)).Should(Succeed())
			logCtx := monitorContext.NewTraceContext(ctx, "")

			v := cuecontext.New().CompileString("")
			err := v.Err()
			Expect(err).Should(Succeed())
			_, err = CollectLogsInPod(logCtx, v)
			Expect(err).ShouldNot(BeNil())
			Expect(verrors.IsCuePathNotFound(err)).Should(BeTrue())

			v = cuecontext.New().CompileString(`cluster: "local"`)
			err = v.Err()
			Expect(err).Should(Succeed())
			_, err = CollectLogsInPod(logCtx, v)
			Expect(err).ShouldNot(BeNil())
			Expect(verrors.IsCuePathNotFound(err)).Should(BeTrue())

			v = cuecontext.New().CompileString(`cluster: "local"
namespace: "default"`)
			err = v.Err()
			Expect(err).Should(Succeed())
			_, err = CollectLogsInPod(logCtx, v)
			Expect(err).ShouldNot(BeNil())
			Expect(verrors.IsCuePathNotFound(err)).Should(BeTrue())

			v = cuecontext.New().CompileString(`cluster: "local"
namespace: "default"
pod: "hello-world"`)
			err = v.Err()
			Expect(err).Should(Succeed())
			_, err = CollectLogsInPod(logCtx, v)
			Expect(err).ShouldNot(BeNil())
			Expect(verrors.IsCuePathNotFound(err)).Should(BeTrue())

			v = cuecontext.New().CompileString(`cluster: "local"
namespace: "default"
pod: "hello-world"
options: {
  container: 1
}`)
			err = v.Err()
			Expect(err).Should(Succeed())
			_, err = CollectLogsInPod(logCtx, v)
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).Should(ContainSubstring("invalid log options content"))

			v = cuecontext.New().CompileString(`cluster: "local"
namespace: "default"
pod: "hello-world"
options: {
  container: "main"
  previous: true
  sinceSeconds: 100
  tailLines: 50
}`)
			err = v.Err()
			Expect(err).Should(Succeed())
			v, err = CollectLogsInPod(logCtx, v)
			Expect(err).Should(Succeed())
			_, err = v.LookupPath(cue.ParsePath("outputs.logs")).String()
			Expect(err).Should(Succeed())
		})
	})

	It("Test generator service endpoints", func() {
		appsts := common.AppStatus{
			AppliedResources: []common.ClusterObjectReference{
				{
					Cluster: "",
					ObjectReference: corev1.ObjectReference{
						Kind:       "Ingress",
						Namespace:  "default",
						Name:       "ingress-http",
						APIVersion: "networking.k8s.io/v1",
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
						APIVersion: "v1",
						Kind:       "Service",
						Namespace:  "default",
						Name:       "nodeport",
					},
				},
				{
					Cluster: "",
					ObjectReference: corev1.ObjectReference{
						APIVersion: "v1",
						Kind:       "Service",
						Namespace:  "default",
						Name:       "loadbalancer",
					},
				},
				{
					Cluster: "",
					ObjectReference: corev1.ObjectReference{
						APIVersion: "helm.toolkit.fluxcd.io/v2beta1",
						Kind:       helmapi.HelmReleaseGVK.Kind,
						Namespace:  "default",
						Name:       "helm-release",
					},
				},
				{
					Cluster: "",
					ObjectReference: corev1.ObjectReference{
						APIVersion: "machinelearning.seldon.io/v1",
						Kind:       "SeldonDeployment",
						Namespace:  "default",
						Name:       "sdep",
					},
				},
				{
					Cluster: "",
					ObjectReference: corev1.ObjectReference{
						APIVersion: "gateway.networking.k8s.io/v1beta1",
						Kind:       "HTTPRoute",
						Namespace:  "default",
						Name:       "http-test-route",
					},
				},
				{
					Cluster: "",
					ObjectReference: corev1.ObjectReference{
						APIVersion: "gateway.networking.k8s.io/v1beta1",
						Kind:       "HTTPRoute",
						Namespace:  "default",
						Name:       "velaux-ssl",
					},
				},
			},
		}
		testApp := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "endpoints-app",
				Namespace: "default",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: "endpoints-test",
						Type: "webservice",
					},
				},
			},
			Status: appsts,
		}
		err := k8sClient.Create(context.TODO(), testApp)
		Expect(err).Should(BeNil())

		var gtapp v1beta1.Application
		Expect(k8sClient.Get(context.TODO(), client.ObjectKey{Name: "endpoints-app", Namespace: "default"}, &gtapp)).Should(BeNil())
		gtapp.Status = appsts
		Expect(k8sClient.Status().Update(ctx, &gtapp)).Should(BeNil())
		var mr []v1beta1.ManagedResource
		for _, ar := range appsts.AppliedResources {
			smr := v1beta1.ManagedResource{
				ClusterObjectReference: ar,
			}
			smr.Component = "endpoints-test"
			mr = append(mr, smr)
		}
		rt := &v1beta1.ResourceTracker{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "endpoints-app",
				Namespace: "default",
				Labels: map[string]string{
					oam.LabelAppName:      testApp.Name,
					oam.LabelAppNamespace: testApp.Namespace,
				},
			},
			Spec: v1beta1.ResourceTrackerSpec{
				Type:             v1beta1.ResourceTrackerTypeRoot,
				ManagedResources: mr,
			},
		}
		err = k8sClient.Create(context.TODO(), rt)
		Expect(err).Should(BeNil())

		helmRelease := &unstructured.Unstructured{}
		helmRelease.SetName("helm-release")
		helmRelease.SetNamespace("default")
		helmRelease.SetGroupVersionKind(helmapi.HelmReleaseGVK)
		err = k8sClient.Create(context.TODO(), helmRelease)
		Expect(err).Should(BeNil())

		testServiceList := []map[string]interface{}{
			{
				"name": "clusterip",
				"ports": []corev1.ServicePort{
					{Port: 80, TargetPort: intstr.FromInt(80), Name: "80port"},
					{Port: 81, TargetPort: intstr.FromInt(81), Name: "81port"},
				},
				"type": corev1.ServiceTypeClusterIP,
			},
			{
				"name": "nodeport",
				"ports": []corev1.ServicePort{
					{Port: 80, TargetPort: intstr.FromInt(80), NodePort: 30229},
				},
				"type": corev1.ServiceTypeNodePort,
			},
			{
				"name": "loadbalancer",
				"ports": []corev1.ServicePort{
					{Port: 80, TargetPort: intstr.FromInt(80), Name: "80port", NodePort: 30080},
					{Port: 81, TargetPort: intstr.FromInt(81), Name: "81port", NodePort: 30081},
				},
				"type": corev1.ServiceTypeLoadBalancer,
				"status": corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{
								IP: "10.10.10.10",
							},
							{
								Hostname: "text.example.com",
							},
						},
					},
				},
			},
			{
				"name": "helm1",
				"ports": []corev1.ServicePort{
					{Port: 80, NodePort: 30002, TargetPort: intstr.FromInt(80)},
				},
				"type": corev1.ServiceTypeNodePort,
				"labels": map[string]string{
					"helm.toolkit.fluxcd.io/name":      "helm-release",
					"helm.toolkit.fluxcd.io/namespace": "default",
				},
			},
			{
				"name": "seldon-ambassador",
				"ports": []corev1.ServicePort{
					{Port: 80, TargetPort: intstr.FromInt(80), Name: "80port", NodePort: 30011},
				},
				"type": corev1.ServiceTypeLoadBalancer,
				"status": corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{
								IP: "1.1.1.1",
							},
						},
					},
				},
			},
		}
		err = k8sClient.Create(context.TODO(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-system",
			},
		})
		Expect(err).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		for _, s := range testServiceList {
			ns := "default"
			if s["namespace"] != nil {
				ns = s["namespace"].(string)
			}
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      s["name"].(string),
					Namespace: ns,
				},
				Spec: corev1.ServiceSpec{
					Ports: s["ports"].([]corev1.ServicePort),
					Type:  s["type"].(corev1.ServiceType),
				},
			}

			if s["labels"] != nil {
				service.Labels = s["labels"].(map[string]string)
			}
			err := k8sClient.Create(context.TODO(), service)
			Expect(err).Should(BeNil())
			if s["status"] != nil {
				service.Status = s["status"].(corev1.ServiceStatus)
				err := k8sClient.Status().Update(context.TODO(), service)
				Expect(err).Should(BeNil())
			}
		}

		var prefixbeta = networkv1.PathTypePrefix
		testIngress := []client.Object{
			&networkv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-http",
					Namespace: "default",
				},
				Spec: networkv1.IngressSpec{
					Rules: []networkv1.IngressRule{
						{
							Host: "ingress.domain",
							IngressRuleValue: networkv1.IngressRuleValue{
								HTTP: &networkv1.HTTPIngressRuleValue{
									Paths: []networkv1.HTTPIngressPath{
										{
											Path: "/",
											Backend: networkv1.IngressBackend{
												Service: &networkv1.IngressServiceBackend{
													Name: "clusterip",
													Port: networkv1.ServiceBackendPort{
														Number: 80,
													},
												},
											},
											PathType: &prefixbeta,
										},
									},
								},
							},
						},
					},
				},
			},
			&networkv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-https",
					Namespace: "default",
				},
				Spec: networkv1.IngressSpec{
					TLS: []networkv1.IngressTLS{
						{
							SecretName: "https-secret",
						},
					},
					Rules: []networkv1.IngressRule{
						{
							Host: "ingress.domain.https",
							IngressRuleValue: networkv1.IngressRuleValue{
								HTTP: &networkv1.HTTPIngressRuleValue{
									Paths: []networkv1.HTTPIngressPath{
										{
											Path: "/",
											Backend: networkv1.IngressBackend{
												Service: &networkv1.IngressServiceBackend{
													Name: "clusterip",
													Port: networkv1.ServiceBackendPort{
														Number: 80,
													},
												},
											},
											PathType: &prefixbeta,
										},
									},
								},
							},
						},
					},
				},
			},
			&networkv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-paths",
					Namespace: "default",
				},
				Spec: networkv1.IngressSpec{
					TLS: []networkv1.IngressTLS{
						{
							SecretName: "https-secret",
						},
					},
					Rules: []networkv1.IngressRule{
						{
							Host: "ingress.domain.path",
							IngressRuleValue: networkv1.IngressRuleValue{
								HTTP: &networkv1.HTTPIngressRuleValue{
									Paths: []networkv1.HTTPIngressPath{
										{
											Path: "/test",
											Backend: networkv1.IngressBackend{
												Service: &networkv1.IngressServiceBackend{
													Name: "clusterip",
													Port: networkv1.ServiceBackendPort{
														Number: 80,
													},
												},
											},
											PathType: &prefixbeta,
										},
										{
											Path: "/test2",
											Backend: networkv1.IngressBackend{
												Service: &networkv1.IngressServiceBackend{
													Name: "clusterip",
													Port: networkv1.ServiceBackendPort{
														Number: 80,
													},
												},
											},
											PathType: &prefixbeta,
										},
									},
								},
							},
						},
					},
				},
			},
			&networkv1.Ingress{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "networking.k8s.io/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-helm",
					Namespace: "default",
					Labels: map[string]string{
						"helm.toolkit.fluxcd.io/name":      "helm-release",
						"helm.toolkit.fluxcd.io/namespace": "default",
					},
				},
				Spec: networkv1.IngressSpec{
					Rules: []networkv1.IngressRule{
						{
							Host: "ingress.domain.helm",
							IngressRuleValue: networkv1.IngressRuleValue{
								HTTP: &networkv1.HTTPIngressRuleValue{
									Paths: []networkv1.HTTPIngressPath{
										{
											Path: "/",
											Backend: networkv1.IngressBackend{
												Service: &networkv1.IngressServiceBackend{
													Name: "clusterip",
													Port: networkv1.ServiceBackendPort{
														Number: 80,
													},
												},
											},
											PathType: &prefixbeta,
										},
									},
								},
							},
						},
					},
				},
			},
		}

		for _, ing := range testIngress {
			err := k8sClient.Create(context.TODO(), ing)
			Expect(err).Should(BeNil())
		}

		obj := &unstructured.Unstructured{}
		obj.SetName("sdep")
		obj.SetNamespace("default")
		obj.SetAnnotations(map[string]string{
			annoAmbassadorServiceName:      "seldon-ambassador",
			annoAmbassadorServiceNamespace: "default",
		})
		obj.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "machinelearning.seldon.io",
			Version: "v1",
			Kind:    "SeldonDeployment",
		})
		err = k8sClient.Create(context.TODO(), obj)
		Expect(err).Should(BeNil())

		// Create the HTTPRoute for test
		resources := []string{
			"./testdata/gateway/http-route.yaml",
			"./testdata/gateway/gateway.yaml",
			"./testdata/gateway/gateway-tls.yaml",
			"./testdata/gateway/https-route.yaml",
		}
		var objects []client.Object
		for _, resource := range resources {
			data, err := os.ReadFile(resource)
			Expect(err).Should(BeNil())
			var route unstructured.Unstructured
			err = yaml.Unmarshal(data, &route)
			Expect(err).Should(BeNil())
			objects = append(objects, &route)
		}

		for _, res := range objects {
			err := k8sClient.Create(context.TODO(), res)
			Expect(err).Should(BeNil())
		}

		// Prepare nodes in test environment
		masterNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				Labels: map[string]string{
					"node-role.kubernetes.io/master": "true",
				},
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "internal-ip-1",
					},
				},
			},
		}
		workerNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
				Labels: map[string]string{
					"node-role.kubernetes.io/worker": "true",
				},
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "internal-ip-2",
					},
					{
						Type:    corev1.NodeExternalIP,
						Address: "external-ip-2",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, masterNode)).Should(BeNil())
		Expect(k8sClient.Create(ctx, workerNode)).Should(BeNil())

		opt := `app: {
			name: "endpoints-app"
			namespace: "default"
			filter: {
				cluster: "",
				clusterNamespace: "default",
			}
			withTree: true
		}`
		v := cuecontext.New().CompileString(opt)
		err = v.Err()
		Expect(err).Should(BeNil())
		singleton.KubeClient.Set(k8sClient)
		logCtx := monitorContext.NewTraceContext(ctx, "")
		v, err = CollectServiceEndpoints(logCtx, v)
		Expect(err).Should(BeNil())
		gatewayIP := selectorNodeIP(ctx, "", k8sClient)
		Expect(gatewayIP).Should(Equal("external-ip-2"))
		urls := []string{
			"http://ingress.domain",
			"https://ingress.domain.https",
			"https://ingress.domain.path/test",
			"https://ingress.domain.path/test2",
			fmt.Sprintf("http://%s:30229", gatewayIP),
			"http://10.10.10.10",
			"http://text.example.com",
			"10.10.10.10:81",
			"text.example.com:81",
			fmt.Sprintf("http://%s:30002", gatewayIP),
			"http://ingress.domain.helm",
			"http://1.1.1.1/seldon/default/sdep",
			"http://gateway.domain",
			"http://gateway.domain/api",
			"https://demo.kubevela.net",
		}
		endValue := v.LookupPath(cue.ParsePath("list"))
		err = endValue.Err()
		Expect(err).Should(BeNil())
		var endpoints []querytypes.ServiceEndpoint
		err = endValue.Decode(&endpoints)
		Expect(err).Should(BeNil())
		Expect(len(urls)).Should(Equal(len(endpoints)))
		for i, e := range endpoints {
			fmt.Println(e.String())
			Expect(urls[i]).Should(Equal(e.String()))
		}
	})
})

var deploymentYaml = `
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.oam.dev/app-revision-hash: ee69f7ed168cd8fa
    app.oam.dev/appRevision: first-vela-app-v1
    app.oam.dev/component: express-server
    app.oam.dev/name: first-vela-app
    app.oam.dev/resourceType: WORKLOAD
    app.oam.dev/revision: express-server-v1
    oam.dev/render-hash: ee2d39b553b6ef03
    workload.oam.dev/type: webservice
  name: express-server
  namespace: default
spec:
  replicas: 2
  selector:
    matchLabels:
      app.oam.dev/component: express-server
  template:
    metadata:
      labels:
        app.oam.dev/component: express-server
    spec:
      containers:
      - image: crccheck/hello-world
        imagePullPolicy: Always
        name: express-server
        ports:
        - containerPort: 8000
          protocol: TCP
`

var serviceYaml = `
apiVersion: v1
kind: Service
metadata:
  labels:
    app.oam.dev/app-revision-hash: ee69f7ed168cd8fa
    app.oam.dev/appRevision: first-vela-app-v1
    app.oam.dev/component: express-server
    app.oam.dev/name: first-vela-app
    app.oam.dev/resourceType: TRAIT
    app.oam.dev/revision: express-server-v1
    oam.dev/render-hash: bebe99ac3e9607d0
    trait.oam.dev/resource: service
    trait.oam.dev/type: ingress-1-20
  name: express-server
  namespace: default
spec:
  ports:
  - port: 8000
    protocol: TCP
    targetPort: 8000
  selector:
    app.oam.dev/component: express-server
`

var podYaml = `
apiVersion: v1
kind: Pod
metadata:
  labels:
    app.oam.dev/component: express-server
  name: express-server-b77f4476b-4mt5m
  namespace: default
spec:
  containers:
  - image: crccheck/hello-world
    imagePullPolicy: Always
    name: express-server-1
    ports:
    - containerPort: 8000
      protocol: TCP
`
