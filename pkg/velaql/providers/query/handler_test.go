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
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
)

type AppResourcesList struct {
	List []AppResources `json:"list,omitempty"`
	App  interface{}    `json:"app"`
	Err  string         `json:"err,omitempty"`
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
				components: ["web"]
			}`
			v, err := value.NewValue(opt, nil, "")
			Expect(err).Should(BeNil())
			Expect(prd.ListResourcesInApp(nil, v, nil)).Should(BeNil())

			type AppResourcesList struct {
				List []AppResources `json:"list"`
				App  interface{}    `json:"app"`
			}
			appResList := new(AppResourcesList)
			Expect(v.UnmarshalTo(appResList)).Should(BeNil())

			Expect(len(appResList.List)).Should(Equal(1))
			Expect(len(appResList.List[0].Components)).Should(Equal(1))
			Expect(len(appResList.List[0].Components[0].Resources)).Should(Equal(2))

			Expect(appResList.List[0].Components[0].Resources[0].Object.GroupVersionKind()).Should(Equal(oldApp.Status.AppliedResources[0].GroupVersionKind()))
			Expect(appResList.List[0].Components[0].Resources[1].Object.GroupVersionKind()).Should(Equal(oldApp.Status.AppliedResources[1].GroupVersionKind()))
		})

		It("Test list legacy resources created by application", func() {
			appName := "test-legacy"
			appNs := "test-legacy"
			ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: appNs}}
			Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
			for i := 1; i <= 5; i++ {
				rt := new(v1beta1.ResourceTracker)
				rt.SetName(fmt.Sprintf("%s-v%d-%s", appName, i, appNs))
				rt.SetLabels(map[string]string{
					oam.LabelAppName:      appName,
					oam.LabelAppNamespace: appNs,
				})
				Expect(k8sClient.Create(ctx, rt)).Should(BeNil())
				oldRT := new(v1beta1.ResourceTracker)
				Eventually(func() error {
					if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rt), oldRT); err != nil {
						return err
					}
					oldRT.Status.TrackedResources = []corev1.ObjectReference{{
						APIVersion: "v1",
						Kind:       "Service",
						Namespace:  appNs,
						Name:       fmt.Sprintf("web-v%d", i),
					}, {
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Namespace:  appNs,
						Name:       fmt.Sprintf("web-v%d", i),
					}}
					if err := k8sClient.Status().Update(ctx, oldRT); err != nil {
						return err
					}
					return nil
				}, 3*time.Second, 300*time.Microsecond).Should(BeNil())

				appDeploy := baseDeploy.DeepCopy()
				appDeploy.SetName(fmt.Sprintf("web-v%d", i))
				appDeploy.SetNamespace(appNs)
				appDeploy.SetLabels(map[string]string{
					oam.LabelAppComponent: "web",
					oam.LabelAppRevision:  fmt.Sprintf("%s-v%d", appName, i),
				})
				Expect(k8sClient.Create(ctx, appDeploy)).Should(BeNil())

				appService := baseService.DeepCopy()
				appService.SetName(fmt.Sprintf("web-v%d", i))
				appService.SetNamespace(appNs)
				appService.SetLabels(map[string]string{
					oam.LabelAppComponent: "web",
					oam.LabelAppRevision:  fmt.Sprintf("%s-v%d", appName, i),
				})
				Expect(k8sClient.Create(ctx, appService)).Should(BeNil())
			}

			prd := provider{cli: k8sClient}
			opt := `app: {
				name: "test-legacy"
				namespace: "test-legacy"
				components: ["web"]
				enableHistoryQuery: true
			}`
			v, err := value.NewValue(opt, nil, "")
			Expect(err).Should(BeNil())
			Expect(prd.ListResourcesInApp(nil, v, nil)).Should(BeNil())

			type AppResourcesList struct {
				List []AppResources `json:"list"`
				App  interface{}    `json:"app"`
			}
			appResList := new(AppResourcesList)
			Expect(v.UnmarshalTo(appResList)).Should(BeNil())

			Expect(len(appResList.List)).Should(Equal(5))
			for _, app := range appResList.List {
				Expect(len(app.Components)).Should(Equal(1))
				Expect(app.Components[0].Resources[0].Object.GroupVersionKind()).Should(Equal((&corev1.ObjectReference{
					APIVersion: "v1",
					Kind:       "Service",
				}).GroupVersionKind()))
				Expect(app.Components[0].Resources[1].Object.GroupVersionKind()).Should(Equal((&corev1.ObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
				}).GroupVersionKind()))
			}
		})

		It("Test list legacy resources meet complex scene", func() {
			appName := "test-legacy-complex"
			appNs := "test-legacy-complex"
			ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: appNs}}
			Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
			for i := 1; i <= 2; i++ {
				rt := new(v1beta1.ResourceTracker)
				rt.SetName(fmt.Sprintf("%s-v%d-%s", appName, i, appNs))
				rt.SetLabels(map[string]string{
					oam.LabelAppName:      appName,
					oam.LabelAppNamespace: appNs,
				})
				Expect(k8sClient.Create(ctx, rt)).Should(BeNil())
				oldRT := new(v1beta1.ResourceTracker)
				Eventually(func() error {
					if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rt), oldRT); err != nil {
						return err
					}
					oldRT.Status.TrackedResources = []corev1.ObjectReference{{
						APIVersion: "v1",
						Kind:       "Service",
						Namespace:  appNs,
						Name:       fmt.Sprintf("web-v%d", i),
					}, {
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Namespace:  appNs,
						Name:       fmt.Sprintf("web-v%d", i),
					}}
					if err := k8sClient.Status().Update(ctx, oldRT); err != nil {
						return err
					}
					return nil
				}, 3*time.Second, 300*time.Microsecond).Should(BeNil())

				appDeploy := baseDeploy.DeepCopy()
				appDeploy.SetName(fmt.Sprintf("web-v%d", i))
				appDeploy.SetNamespace(appNs)
				appDeploy.SetLabels(map[string]string{
					oam.LabelAppComponent: "web",
					oam.LabelAppRevision:  fmt.Sprintf("%s-v%d", appName, i),
				})
				Expect(k8sClient.Create(ctx, appDeploy)).Should(BeNil())

				appService := baseService.DeepCopy()
				appService.SetName(fmt.Sprintf("web-v%d", i))
				appService.SetNamespace(appNs)
				appService.SetLabels(map[string]string{
					oam.LabelAppComponent: "web",
					oam.LabelAppRevision:  fmt.Sprintf("%s-v%d", appName, i),
				})
				Expect(k8sClient.Create(ctx, appService)).Should(BeNil())
			}

			By("create resourceTracker without trackedResource")
			emptyRT := new(v1beta1.ResourceTracker)
			emptyRT.SetName(fmt.Sprintf("%s-%s", appName, appNs))
			emptyRT.SetLabels(map[string]string{
				oam.LabelAppName:      appName,
				oam.LabelAppNamespace: appNs,
			})
			Expect(k8sClient.Create(ctx, emptyRT)).Should(BeNil())

			prd := provider{cli: k8sClient}
			opt := `app: {
				name: "test-legacy-complex"
				namespace: "test-legacy-complex"
				components: []
				enableHistoryQuery: true
			}`
			v, err := value.NewValue(opt, nil, "")
			Expect(err).Should(BeNil())
			Expect(prd.ListResourcesInApp(nil, v, nil)).Should(BeNil())

			appResList := new(AppResourcesList)
			Expect(v.UnmarshalTo(appResList)).Should(BeNil())
			Expect(len(appResList.List)).Should(Equal(2))

			By("create resourceTracker tracked an un-exist resource")
			rt := new(v1beta1.ResourceTracker)
			rt.SetName(fmt.Sprintf("%s-v%d-%s", appName, 3, appNs))
			rt.SetLabels(map[string]string{
				oam.LabelAppName:      appName,
				oam.LabelAppNamespace: appNs,
			})
			Expect(k8sClient.Create(ctx, rt)).Should(BeNil())

			oldRT := new(v1beta1.ResourceTracker)
			Eventually(func() error {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rt), oldRT); err != nil {
					return err
				}
				oldRT.Status.TrackedResources = []corev1.ObjectReference{{
					APIVersion: "v1",
					Kind:       "Service",
					Namespace:  appNs,
					Name:       fmt.Sprintf("web-v%d", 3),
				}, {
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Namespace:  appNs,
					Name:       fmt.Sprintf("web-v%d", 3),
				}}
				if err := k8sClient.Status().Update(ctx, oldRT); err != nil {
					return err
				}
				return nil
			}, 3*time.Second, 300*time.Microsecond).Should(BeNil())

			appService := baseService.DeepCopy()
			appService.SetName(fmt.Sprintf("web-v%d", 4))
			appService.SetNamespace(appNs)
			appService.SetLabels(map[string]string{
				oam.LabelAppComponent: "web",
				oam.LabelAppRevision:  fmt.Sprintf("%s-v%d", appName, 4),
			})
			Expect(k8sClient.Create(ctx, appService)).Should(BeNil())

			newV, err := value.NewValue(opt, nil, "")
			Expect(err).Should(BeNil())
			Expect(prd.ListResourcesInApp(nil, newV, nil)).Should(BeNil())
			appResList = new(AppResourcesList)
			Expect(v.UnmarshalTo(appResList)).Should(BeNil())
			Expect(len(appResList.List)).Should(Equal(2))

			By("create resourceTracker tracked with wrong name")
			wrongNameRT := new(v1beta1.ResourceTracker)
			wrongNameRT.SetName("test-1")
			wrongNameRT.SetLabels(map[string]string{
				oam.LabelAppName:      appName,
				oam.LabelAppNamespace: appNs,
			})
			Expect(k8sClient.Create(ctx, wrongNameRT)).Should(BeNil())

			oldRT = new(v1beta1.ResourceTracker)
			Eventually(func() error {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(wrongNameRT), oldRT); err != nil {
					return err
				}
				oldRT.Status.TrackedResources = []corev1.ObjectReference{{
					APIVersion: "v1",
					Kind:       "Service",
					Namespace:  appNs,
					Name:       fmt.Sprintf("web-v%d", 4),
				}, {
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Namespace:  appNs,
					Name:       fmt.Sprintf("web-v%d", 4),
				}}
				if err := k8sClient.Status().Update(ctx, oldRT); err != nil {
					return err
				}
				return nil
			}, 3*time.Second, 300*time.Microsecond).Should(BeNil())

			newV, err = value.NewValue(opt, nil, "")
			Expect(err).Should(BeNil())
			Expect(prd.ListResourcesInApp(nil, newV, nil)).Should(BeNil())
			appResList = new(AppResourcesList)
			Expect(newV.UnmarshalTo(appResList)).Should(BeNil())
			Expect(len(appResList.Err)).ShouldNot(Equal(0))
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

	It("Test install provider", func() {
		p := providers.NewProviders()
		Install(p, k8sClient)
		h, ok := p.GetHandler("query", "listResourcesInApp")
		Expect(h).ShouldNot(BeNil())
		Expect(ok).Should(Equal(true))
		h, ok = p.GetHandler("query", "collectPods")
		Expect(h).ShouldNot(BeNil())
		Expect(ok).Should(Equal(true))
		h, ok = p.GetHandler("query", "searchEvents")
		Expect(h).ShouldNot(BeNil())
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
