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
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	istiov1beta1 "istio.io/api/networking/v1beta1"
	istio "istio.io/client-go/pkg/apis/networking/v1beta1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkv1beta1 "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	helmapi "github.com/oam-dev/kubevela/pkg/appfile/helm/flux2apis"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
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
						"oam.dev/kubevela-version": "v1.2.0-beta.2",
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

			prd := provider{cli: k8sClient}
			opt := `app: {
				name: "test"
				namespace: "test"
				filter: {
					cluster: "",
					clusterNamespace: "test",
					components: ["web"]
				}
			}`
			v, err := value.NewValue(opt, nil, "")
			Expect(err).Should(BeNil())
			Expect(prd.ListResourcesInApp(nil, v, nil)).Should(BeNil())

			appResList := new(AppResourcesList)
			Expect(v.UnmarshalTo(appResList)).Should(BeNil())

			Expect(len(appResList.List)).Should(Equal(2))

			Expect(appResList.List[0].Object.GroupVersionKind()).Should(Equal(oldApp.Status.AppliedResources[0].GroupVersionKind()))
			Expect(appResList.List[1].Object.GroupVersionKind()).Should(Equal(oldApp.Status.AppliedResources[1].GroupVersionKind()))

			updateApp := new(v1beta1.Application)
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&app), updateApp)).Should(BeNil())

			updateApp.ObjectMeta.Annotations = map[string]string{
				"oam.dev/kubevela-version": "master",
			}
			Expect(k8sClient.Update(ctx, updateApp)).Should(BeNil())
			newValue, err := value.NewValue(opt, nil, "")
			Expect(err).Should(BeNil())
			Expect(prd.ListResourcesInApp(nil, newValue, nil)).Should(BeNil())
			newAppResList := new(AppResourcesList)
			Expect(v.UnmarshalTo(newAppResList)).Should(BeNil())
			Expect(len(newAppResList.List)).Should(Equal(2))
			Expect(newAppResList.List[0].Object.GroupVersionKind()).Should(Equal(updateApp.Status.AppliedResources[0].GroupVersionKind()))
			Expect(newAppResList.List[1].Object.GroupVersionKind()).Should(Equal(updateApp.Status.AppliedResources[1].GroupVersionKind()))
		})

		It("Test list resource with incomplete parameter", func() {
			optWithoutApp := ""
			prd := provider{cli: k8sClient}
			newV, err := value.NewValue(optWithoutApp, nil, "")
			Expect(err).Should(BeNil())
			err = prd.ListResourcesInApp(nil, newV, nil)
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).Should(Equal("var(path=app) not exist"))
		})
	})

	Context("Test CollectPods", func() {
		It("Test collect pod from workload deployment", func() {
			deploy := baseDeploy.DeepCopy()
			deploy.SetName("test-collect-pod")
			deploy.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					oam.LabelAppComponent: "test",
				},
			}
			deploy.Spec.Template.ObjectMeta.SetLabels(map[string]string{
				oam.LabelAppComponent: "test",
			})
			Expect(k8sClient.Create(ctx, deploy)).Should(BeNil())
			for i := 1; i <= 5; i++ {
				pod := basePod.DeepCopy()
				pod.SetName(fmt.Sprintf("test-collect-pod-%d", i))
				pod.SetLabels(map[string]string{
					oam.LabelAppComponent: "test",
				})
				Expect(k8sClient.Create(ctx, pod)).Should(BeNil())
			}

			prd := provider{cli: k8sClient}
			unstructuredDeploy, err := util.Object2Unstructured(deploy)
			Expect(err).Should(BeNil())
			unstructuredDeploy.SetGroupVersionKind((&corev1.ObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			}).GroupVersionKind())

			deployJson, err := json.Marshal(unstructuredDeploy)
			Expect(err).Should(BeNil())
			opt := fmt.Sprintf(`value: %s
cluster: ""`, deployJson)
			v, err := value.NewValue(opt, nil, "")
			Expect(err).Should(BeNil())
			Expect(prd.CollectPods(nil, v, nil)).Should(BeNil())

			podList := new(PodList)
			Expect(v.UnmarshalTo(podList)).Should(BeNil())
			Expect(len(podList.List)).Should(Equal(5))
			for _, pod := range podList.List {
				Expect(pod.GroupVersionKind()).Should(Equal((&corev1.ObjectReference{
					APIVersion: "v1",
					Kind:       "Pod",
				}).GroupVersionKind()))
			}
		})

		It("Test collect pod with incomplete parameter", func() {
			emptyOpt := ""
			prd := provider{cli: k8sClient}
			v, err := value.NewValue(emptyOpt, nil, "")
			Expect(err).Should(BeNil())
			err = prd.CollectPods(nil, v, nil)
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).Should(Equal("var(path=value) not exist"))

			optWithoutCluster := `value: {}`
			v, err = value.NewValue(optWithoutCluster, nil, "")
			Expect(err).Should(BeNil())
			err = prd.CollectPods(nil, v, nil)
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).Should(Equal("var(path=cluster) not exist"))

			optWithWrongValue := `value: {test: 1}
cluster: "test"`
			v, err = value.NewValue(optWithWrongValue, nil, "")
			Expect(err).Should(BeNil())
			err = prd.CollectPods(nil, v, nil)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("Test search event from k8s object", func() {
		It("Test search event with incomplete parameter", func() {
			emptyOpt := ""
			prd := provider{cli: k8sClient}
			v, err := value.NewValue(emptyOpt, nil, "")
			Expect(err).Should(BeNil())
			err = prd.SearchEvents(nil, v, nil)
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).Should(Equal("var(path=value) not exist"))

			optWithoutCluster := `value: {}`
			v, err = value.NewValue(optWithoutCluster, nil, "")
			Expect(err).Should(BeNil())
			err = prd.SearchEvents(nil, v, nil)
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).Should(Equal("var(path=cluster) not exist"))

			optWithWrongValue := `value: {}
cluster: "test"`
			v, err = value.NewValue(optWithWrongValue, nil, "")
			Expect(err).Should(BeNil())
			err = prd.SearchEvents(nil, v, nil)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("Test CollectLogsInPod", func() {
		It("Test CollectLogsInPod with specified container", func() {
			prd := provider{cli: k8sClient, cfg: cfg}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "hello-world", Namespace: "default"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main", Image: "busybox"}},
				}}
			Expect(k8sClient.Create(ctx, pod)).Should(Succeed())

			v, err := value.NewValue(``, nil, "")
			Expect(err).Should(Succeed())
			err = prd.CollectLogsInPod(nil, v, nil)
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).Should(ContainSubstring("var(path=cluster) not exist"))

			v, err = value.NewValue(`cluster: "local"`, nil, "")
			Expect(err).Should(Succeed())
			err = prd.CollectLogsInPod(nil, v, nil)
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).Should(ContainSubstring("var(path=namespace) not exist"))

			v, err = value.NewValue(`cluster: "local"
namespace: "default"`, nil, "")
			Expect(err).Should(Succeed())
			err = prd.CollectLogsInPod(nil, v, nil)
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).Should(ContainSubstring("var(path=pod) not exist"))

			v, err = value.NewValue(`cluster: "local"
namespace: "default"
pod: "hello-world"`, nil, "")
			Expect(err).Should(Succeed())
			err = prd.CollectLogsInPod(nil, v, nil)
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).Should(ContainSubstring("var(path=options) not exist"))

			v, err = value.NewValue(`cluster: "local"
namespace: "default"
pod: "hello-world"
options: {
  container: 1
}`, nil, "")
			Expect(err).Should(Succeed())
			err = prd.CollectLogsInPod(nil, v, nil)
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).Should(ContainSubstring("invalid log options content"))

			v, err = value.NewValue(`cluster: "local"
namespace: "default"
pod: "hello-world"
options: {
  container: "main"
  previous: true
  sinceSeconds: 100
  tailLines: 50
}`, nil, "")
			Expect(err).Should(Succeed())
			Expect(prd.CollectLogsInPod(nil, v, nil)).Should(Succeed())
			_, err = v.GetString("outputs", "logs")
			Expect(err).Should(Succeed())
		})
	})

	It("Test install provider", func() {
		p := providers.NewProviders()
		Install(p, k8sClient, cfg)
		h, ok := p.GetHandler("query", "listResourcesInApp")
		Expect(h).ShouldNot(BeNil())
		Expect(ok).Should(Equal(true))
		h, ok = p.GetHandler("query", "collectPods")
		Expect(h).ShouldNot(BeNil())
		Expect(ok).Should(Equal(true))
		h, ok = p.GetHandler("query", "searchEvents")
		Expect(ok).Should(Equal(true))
		Expect(h).ShouldNot(BeNil())
		h, ok = p.GetHandler("query", "collectLogsInPod")
		Expect(ok).Should(Equal(true))
		Expect(h).ShouldNot(BeNil())
		h, ok = p.GetHandler("query", "collectServiceEndpoints")
		Expect(ok).Should(Equal(true))
		Expect(h).ShouldNot(BeNil())
	})

	It("Test generator service endpoints", func() {
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
			Status: common.AppStatus{
				AppliedResources: []common.ClusterObjectReference{
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
							Kind:      "Service",
							Namespace: "default",
							Name:      "nodeport",
						},
					},
					{
						Cluster: "",
						ObjectReference: corev1.ObjectReference{
							Kind:      "Service",
							Namespace: "default",
							Name:      "loadbalancer",
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
					{
						Cluster: "",
						ObjectReference: corev1.ObjectReference{
							Kind:      "SeldonDeployment",
							Namespace: "default",
							Name:      "sdep",
						},
					},
				},
			},
		}
		err := k8sClient.Create(context.TODO(), testApp)
		Expect(err).Should(BeNil())

		testServicelist := []map[string]interface{}{
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
					{Port: 80, TargetPort: intstr.FromInt(80), Name: "80port"},
					{Port: 81, TargetPort: intstr.FromInt(81), Name: "81port"},
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
					"helm.toolkit.fluxcd.io/name":      "helmRelease",
					"helm.toolkit.fluxcd.io/namespace": "default",
				},
			},
			{
				"name": "istio-service",
				"ports": []corev1.ServicePort{
					{Port: 80, TargetPort: intstr.FromInt(80), Name: "80port"},
				},
				"type": corev1.ServiceTypeLoadBalancer,
				"labels": map[string]string{
					"istio": "ingressgateway",
				},
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
		for _, s := range testServicelist {
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      s["name"].(string),
					Namespace: "default",
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
		var prefixbeta = networkv1beta1.PathTypePrefix
		testIngress := []client.Object{
			&networkv1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-http",
					Namespace: "default",
				},
				Spec: networkv1beta1.IngressSpec{
					Rules: []networkv1beta1.IngressRule{
						{
							Host: "ingress.domain",
							IngressRuleValue: networkv1beta1.IngressRuleValue{
								HTTP: &networkv1beta1.HTTPIngressRuleValue{
									Paths: []networkv1beta1.HTTPIngressPath{
										{
											Path: "/",
											Backend: networkv1beta1.IngressBackend{
												ServiceName: "clusterip",
												ServicePort: intstr.FromInt(80),
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
			&networkv1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-https",
					Namespace: "default",
				},
				Spec: networkv1beta1.IngressSpec{
					TLS: []networkv1beta1.IngressTLS{
						{
							SecretName: "https-secret",
						},
					},
					Rules: []networkv1beta1.IngressRule{
						{
							Host: "ingress.domain.https",
							IngressRuleValue: networkv1beta1.IngressRuleValue{
								HTTP: &networkv1beta1.HTTPIngressRuleValue{
									Paths: []networkv1beta1.HTTPIngressPath{
										{
											Path: "/",
											Backend: networkv1beta1.IngressBackend{
												ServiceName: "clusterip",
												ServicePort: intstr.FromInt(80),
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
			&networkv1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-paths",
					Namespace: "default",
				},
				Spec: networkv1beta1.IngressSpec{
					TLS: []networkv1beta1.IngressTLS{
						{
							SecretName: "https-secret",
						},
					},
					Rules: []networkv1beta1.IngressRule{
						{
							Host: "ingress.domain.path",
							IngressRuleValue: networkv1beta1.IngressRuleValue{
								HTTP: &networkv1beta1.HTTPIngressRuleValue{
									Paths: []networkv1beta1.HTTPIngressPath{
										{
											Path: "/test",
											Backend: networkv1beta1.IngressBackend{
												ServiceName: "clusterip",
												ServicePort: intstr.FromInt(80),
											},
											PathType: &prefixbeta,
										},
										{
											Path: "/test2",
											Backend: networkv1beta1.IngressBackend{
												ServiceName: "clusterip",
												ServicePort: intstr.FromInt(80),
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
			&networkv1beta1.Ingress{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "networking.k8s.io/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-helm",
					Namespace: "default",
					Labels: map[string]string{
						"helm.toolkit.fluxcd.io/name":      "helmRelease",
						"helm.toolkit.fluxcd.io/namespace": "default",
					},
				},
				Spec: networkv1beta1.IngressSpec{
					Rules: []networkv1beta1.IngressRule{
						{
							Host: "ingress.domain.helm",
							IngressRuleValue: networkv1beta1.IngressRuleValue{
								HTTP: &networkv1beta1.HTTPIngressRuleValue{
									Paths: []networkv1beta1.HTTPIngressPath{
										{
											Path: "/",
											Backend: networkv1beta1.IngressBackend{
												ServiceName: "clusterip",
												ServicePort: intstr.FromInt(80),
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

		err = k8sClient.Create(context.TODO(), &istio.VirtualService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sdep",
				Namespace: "default",
			},
			Spec: istiov1beta1.VirtualService{
				Http: []*istiov1beta1.HTTPRoute{
					{
						Match: []*istiov1beta1.HTTPMatchRequest{
							{
								Uri: &istiov1beta1.StringMatch{
									MatchType: &istiov1beta1.StringMatch_Prefix{Prefix: "/seldon/test"},
								},
							},
						},
					},
				},
			},
		})
		Expect(err).Should(BeNil())
		opt := `app: {
			name: "endpoints-app"
			namespace: "default"
			filter: {
				cluster: "",
				clusterNamespace: "default",
			}
		}`
		v, err := value.NewValue(opt, nil, "")
		Expect(err).Should(BeNil())
		pr := &provider{
			cli: k8sClient,
		}
		err = pr.GeneratorServiceEndpoints(nil, v, nil)
		Expect(err).Should(BeNil())
		var node corev1.NodeList
		err = k8sClient.List(context.TODO(), &node)
		Expect(err).Should(BeNil())
		var gatewayIP string
		if len(node.Items) > 0 {
			for _, address := range node.Items[0].Status.Addresses {
				if address.Type == corev1.NodeInternalIP {
					gatewayIP = address.Address
					break
				}
			}
		}
		urls := []string{
			"http://ingress.domain",
			"https://ingress.domain.https",
			"https://ingress.domain.path/test",
			"https://ingress.domain.path/test2",
			fmt.Sprintf("http://%s:30229", gatewayIP),
			"http://10.10.10.10",
			"http://text.example.com",
			"tcp://10.10.10.10:81",
			"tcp://text.example.com:81",
			// helmRelease
			fmt.Sprintf("http://%s:30002", gatewayIP),
			"http://ingress.domain.helm",
			"tcp://1.1.1.1:80/seldon/test",
		}
		endValue, err := v.Field("list")
		Expect(err).Should(BeNil())
		var endpoints []querytypes.ServiceEndpoint
		err = endValue.Decode(&endpoints)
		Expect(err).Should(BeNil())
		for i, endpoint := range endpoints {
			Expect(endpoint.String()).Should(BeEquivalentTo(urls[i]))
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
