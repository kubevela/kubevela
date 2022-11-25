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

package query

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	"github.com/kubevela/workflow/pkg/cue/model/value"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
)

var _ = Describe("Test query endpoints", func() {

	BeforeEach(func() {
	})

	Context("Test Generate Endpoints", func() {
		It("Test endpoints with additional rules", func() {
			err := k8sClient.Create(context.TODO(), &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vela-system",
				},
			})
			Expect(err).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
			sts := common.AppStatus{
				AppliedResources: []common.ClusterObjectReference{
					{
						Cluster: "",
						ObjectReference: corev1.ObjectReference{
							APIVersion: "machinelearning.seldon.io/v1",
							Kind:       "SeldonDeployment",
							Namespace:  "default",
							Name:       "sdep2",
						},
					},
				},
			}
			testApp := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "endpoints-app-2",
					Namespace: "default",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "endpoints-test-2",
							Type: "webservice",
						},
					},
				},
				Status: sts,
			}
			Expect(k8sClient.Create(context.TODO(), testApp)).Should(BeNil())
			var gtapp v1beta1.Application
			Expect(k8sClient.Get(context.TODO(), client.ObjectKey{Name: "endpoints-app-2", Namespace: "default"}, &gtapp)).Should(BeNil())
			gtapp.Status = sts
			Expect(k8sClient.Status().Update(ctx, &gtapp)).Should(BeNil())
			var mr []v1beta1.ManagedResource
			for _, ar := range sts.AppliedResources {
				smr := v1beta1.ManagedResource{
					ClusterObjectReference: ar,
				}
				smr.Component = "endpoints-test-2"
				mr = append(mr, smr)
			}
			rt := &v1beta1.ResourceTracker{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "endpoints-app-2",
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

			By("Prepare configmap for relationship")

			err = k8sClient.Create(context.TODO(), &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rule-for-seldon-test",
					Namespace: types.DefaultKubeVelaNS,
					Labels: map[string]string{
						oam.LabelResourceRules:      "true",
						oam.LabelResourceRuleFormat: oam.ResourceTopologyFormatJSON,
					},
				},
				Data: map[string]string{
					"rules": `[
    {
        "parentResourceType": {
            "group": "machinelearning.seldon.io",
            "kind": "SeldonDeployment"
        },
        "childrenResourceType": [
            {
                "apiVersion": "v1",
                "kind": "Service"
            }
        ]
    }
]`,
				},
			})
			Expect(err).Should(BeNil())
			testServiceList := []map[string]interface{}{
				{
					"name": "clusterip-2",
					"ports": []corev1.ServicePort{
						{Port: 80, TargetPort: intstr.FromInt(80), Name: "80port"},
						{Port: 81, TargetPort: intstr.FromInt(81), Name: "81port"},
					},
					"type": corev1.ServiceTypeClusterIP,
				},
				{
					"name": "load-balancer",
					"ports": []corev1.ServicePort{
						{Port: 8080, TargetPort: intstr.FromInt(8080), Name: "8080port", NodePort: 30020},
					},
					"type": corev1.ServiceTypeLoadBalancer,
					"status": corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP: "2.2.2.2",
								},
							},
						},
					},
				},
				{
					"name": "seldon-ambassador-2",
					"ports": []corev1.ServicePort{
						{Port: 80, TargetPort: intstr.FromInt(80), Name: "80port"},
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

			abgvk := schema.GroupVersionKind{
				Group:   "machinelearning.seldon.io",
				Version: "v1",
				Kind:    "SeldonDeployment",
			}
			obj := &unstructured.Unstructured{}
			obj.SetName("sdep2")
			obj.SetNamespace("default")
			obj.SetAnnotations(map[string]string{
				annoAmbassadorServiceName:      "seldon-ambassador-2",
				annoAmbassadorServiceNamespace: "default",
			})
			obj.SetGroupVersionKind(abgvk)
			err = k8sClient.Create(context.TODO(), obj)
			Expect(err).Should(BeNil())
			abobj := &unstructured.Unstructured{}
			abobj.SetGroupVersionKind(abgvk)
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "sdep2", Namespace: "default"}, abobj)).Should(BeNil())

			for _, s := range testServiceList {
				ns := "default"
				if s["namespace"] != nil {
					ns = s["namespace"].(string)
				}
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      s["name"].(string),
						Namespace: ns,
						OwnerReferences: []metav1.OwnerReference{
							{APIVersion: "machinelearning.seldon.io/v1", Kind: "SeldonDeployment", Name: "sdep2", UID: abobj.GetUID()},
						},
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

			opt := `app: {
			name: "endpoints-app-2"
			namespace: "default"
			filter: {
				cluster: "",
				clusterNamespace: "default",
			}
			withTree: true
		}`
			v, err := value.NewValue(opt, nil, "")
			Expect(err).Should(BeNil())
			pr := &provider{
				cli: k8sClient,
			}
			logCtx := monitorContext.NewTraceContext(ctx, "")
			err = pr.CollectServiceEndpoints(logCtx, nil, v, nil)
			Expect(err).Should(BeNil())

			urls := []string{
				"http://1.1.1.1/seldon/default/sdep2",
				"http://clusterip-2.default",
				"clusterip-2.default:81",
				"http://2.2.2.2:8080",
				"http://1.1.1.1",
			}
			endValue, err := v.Field("list")
			Expect(err).Should(BeNil())
			var endpoints []querytypes.ServiceEndpoint
			err = endValue.Decode(&endpoints)
			Expect(err).Should(BeNil())
			var edps []string
			for _, e := range endpoints {
				edps = append(edps, e.String())
			}
			Expect(edps).Should(BeEquivalentTo(urls))
		})

		It("Test select gateway IP", func() {
			masterNode := corev1.Node{
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
							Address: "node1-internal-ip",
						},
					},
				},
			}
			workerNode1 := corev1.Node{
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
							Address: "node2-internal-ip",
						},
						{
							Type:    corev1.NodeExternalIP,
							Address: "node2-external-ip",
						},
					},
				},
			}
			workerNode2 := corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-3",
					Labels: map[string]string{
						"node-role.kubernetes.io/worker": "true",
					},
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: "node3-internal-ip",
						},
					},
				},
			}
			gatewayNode := corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-4",
					Labels: map[string]string{
						"node-role.kubernetes.io/gateway": "true",
					},
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: "node4-internal-ip",
						},
						{
							Type:    corev1.NodeExternalIP,
							Address: "node4-external-ip",
						},
					},
				},
			}
			testCase := []struct {
				note   string
				nodes  []corev1.Node
				wantIP string
			}{
				{
					note:   "only master node",
					nodes:  []corev1.Node{masterNode},
					wantIP: "node1-internal-ip",
				},
				{
					note:   "with worker node, select external ip first",
					nodes:  []corev1.Node{masterNode, workerNode1},
					wantIP: "node2-external-ip",
				},
				{
					note:   "with worker node, select worker's internal ip",
					nodes:  []corev1.Node{masterNode, workerNode2},
					wantIP: "node3-internal-ip",
				},
				{
					note:   "with gateway node, gateway node first",
					nodes:  []corev1.Node{masterNode, workerNode1, workerNode1, gatewayNode},
					wantIP: "node4-external-ip",
				},
			}
			for _, tc := range testCase {
				By(tc.note)
				ip := selectGatewayIP(tc.nodes)
				Expect(ip).Should(Equal(tc.wantIP))
			}
		})
	})
})

var _ = Describe("Test get ingress endpoint", func() {
	It("Test get ingress endpoint with different apiVersion", func() {
		ingress1 := v1.Ingress{}
		Expect(yaml.Unmarshal([]byte(ingressYaml1), &ingress1)).Should(BeNil())

		err := k8sClient.Create(ctx, &ingress1)
		Expect(err).Should(BeNil())
		gvk := schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"}
		Eventually(func() error {
			eps := getServiceEndpoints(ctx, k8sClient, gvk, ingress1.Name, ingress1.Namespace, "", "", nil)
			if len(eps) != 1 {
				return fmt.Errorf("result length missmatch")
			}
			return nil
		}, 2*time.Second, 500*time.Millisecond).Should(BeNil())
	})
})

var ingressYaml1 = `
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: ingress-1
  namespace: default
spec:
  rules:
  - http:
      paths:
      - path: /testpath
        pathType: Prefix
        backend:
          service:
            name: test
            port:
              number: 80
`
