/*
Copyright 2025 The KubeVela Authors.

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

package controllers_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// The PostDispatch specs share one namespace and one set of definitions. Each
// scenario is a component of an Application whose outcome it shares: the
// healthy scenarios in one app, the degraded ones in another, since a single
// unhealthy component keeps a whole app out of Running. A scenario whose state
// would leak into others, a failing PostDispatch render or a timed readiness
// flip, gets an app of its own.
var _ = Describe("PostDispatch Trait tests", Ordered, ContinueOnFailure, func() {
	ctx := context.Background()
	var namespace string
	var defs postDispatchDefs

	var healthyApp, degradedApp, noHealthApp *v1beta1.Application

	BeforeAll(func() {
		defs = createPostDispatchDefs(ctx)

		namespace = randomNamespaceName("postdispatch-test")
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, ns) })

		// Created together so their rollouts overlap; the flip app is created by
		// its own spec, since its readiness is on a timer from pod start.
		healthyApp = postDispatchApp(namespace, "app-postdispatch-healthy",
			webserviceWithStatusTraits(defs, "test-deployment", 3, "nginx:1.21"),
			webserviceWithStatusTraits(defs, "test-deployment-b", 3, "nginx:1.21"),
			common.ApplicationComponent{
				Name:       "test-component",
				Type:       defs.workerComp,
				Properties: rawJSON(`{"name":"test-worker","image":"nginx:1.21"}`),
				Traits: []common.ApplicationTrait{
					{Type: defs.statusTrait},
					{Type: "scaler", Properties: rawJSON(`{"replicas":3}`)},
				},
			},
			common.ApplicationComponent{
				Name:       "slow-component",
				Type:       defs.slowComp,
				Properties: rawJSON(`{"name":"slow-worker","image":"nginx:1.21"}`),
				Traits:     []common.ApplicationTrait{{Type: defs.markerTrait}},
			},
		)
		degradedApp = postDispatchApp(namespace, "app-postdispatch-degraded",
			webserviceWithStatusTraits(defs, "bad-component", 1, "nginx:1.21abc"),
			webserviceWithStatusTraits(defs, "two-replica-component", 2, "nginx:1.21"),
			webserviceWithStatusTraits(defs, "good-component", 3, "nginx:1.21"),
		)
		noHealthApp = postDispatchApp(namespace, "app-postdispatch-no-health",
			common.ApplicationComponent{
				Name:       "no-health-component",
				Type:       defs.noHealthComp,
				Properties: rawJSON(`{"name":"no-health-worker","image":"nginx:1.21"}`),
				Traits:     []common.ApplicationTrait{{Type: defs.statusTrait}},
			},
		)
		for _, app := range []*v1beta1.Application{healthyApp, degradedApp, noHealthApp} {
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		}
	})

	getApp := func(g Gomega, app *v1beta1.Application) *v1beta1.Application {
		current := &v1beta1.Application{}
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(app), current)).Should(Succeed())
		return current
	}

	expectReadyDeployment := func(g Gomega, name string, replicas int32) {
		deploy := &appsv1.Deployment{}
		g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, deploy)).Should(Succeed())
		g.Expect(deploy.Status.Replicas).Should(Equal(replicas))
		g.Expect(deploy.Status.ReadyReplicas).Should(Equal(replicas))
	}

	configMapData := func(g Gomega, name string) map[string]string {
		cm := &corev1.ConfigMap{}
		g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, cm)).Should(Succeed())
		return cm.Data
	}

	Context("Healthy components", func() {
		It("Should show a PostDispatch trait as pending before its component is healthy", func() {
			Eventually(func(g Gomega) {
				app := getApp(g, healthyApp)
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunningWorkflow))
				trait := findTraitStatus(g, app, "slow-component", defs.markerTrait)
				g.Expect(trait.Healthy).Should(BeFalse())
				g.Expect(trait.Pending).Should(BeTrue())
				g.Expect(trait.Message).Should(ContainSubstring("Waiting for component to be healthy"))
			}, 20*time.Second, 500*time.Millisecond).Should(Succeed())
		})

		It("Should mark the application, every component and every PostDispatch trait healthy", func() {
			EventuallyReconciled(ctx, healthyApp, func(g Gomega) {
				app := getApp(g, healthyApp)
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
				wantTraits := map[string]int{"test-deployment": 3, "test-deployment-b": 3, "test-component": 2, "slow-component": 1}
				g.Expect(app.Status.Services).Should(HaveLen(len(wantTraits)))
				for _, svc := range app.Status.Services {
					g.Expect(svc.Healthy).Should(BeTrue(), "component %s", svc.Name)
					g.Expect(svc.Traits).Should(HaveLen(wantTraits[svc.Name]), "traits on %s", svc.Name)
					for _, trait := range svc.Traits {
						g.Expect(trait.Healthy).Should(BeTrue(), "trait %s on %s", trait.Type, svc.Name)
						g.Expect(trait.Pending).Should(BeFalse(), "trait %s on %s", trait.Type, svc.Name)
					}
				}
			}).WithTimeout(3 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("Should render PostDispatch outputs from the healthy component's status", func() {
			By("A trait-managed Deployment sized from the component's replicas")
			Eventually(func(g Gomega) {
				expectReadyDeployment(g, "test-deployment", 3)
				expectReadyDeployment(g, "trait-deployment-test-deployment", 3)
				g.Expect(configMapData(g, "test-deployment-status")).Should(And(
					HaveKeyWithValue("componentName", "test-deployment"),
					HaveKeyWithValue("replicas", "3"),
					HaveKeyWithValue("readyReplicas", "3"),
				))
			}, 30*time.Second, time.Second).Should(Succeed())

			By("A second component with the same PostDispatch traits gets its own outputs")
			Eventually(func(g Gomega) {
				expectReadyDeployment(g, "trait-deployment-test-deployment-b", 3)
				g.Expect(configMapData(g, "test-deployment-b-status")).Should(HaveKeyWithValue("componentName", "test-deployment-b"))
			}, 30*time.Second, time.Second).Should(Succeed())

			By("A status ConfigMap re-rendered once every replica of a custom component is ready")
			EventuallyReconciled(ctx, healthyApp, func(g Gomega) {
				expectReadyDeployment(g, "test-worker", 3)
				g.Expect(configMapData(g, "test-component-status")).Should(And(
					HaveKeyWithValue("componentName", "test-component"),
					HaveKeyWithValue("replicas", "3"),
					HaveKeyWithValue("readyReplicas", "3"),
				))
			}).WithTimeout(time.Minute).Should(Succeed())

			By("A marker dispatched once the slow component passed its readiness probe")
			Eventually(func(g Gomega) {
				g.Expect(configMapData(g, "slow-component-marker")).Should(HaveKeyWithValue("status", "deployed"))
			}, 30*time.Second, time.Second).Should(Succeed())
		})
	})

	Context("Degraded components", func() {
		It("Should keep PostDispatch traits pending on an unhealthy component, and report each trait's own health on the others", func() {
			EventuallyReconciled(ctx, degradedApp, func(g Gomega) {
				app := getApp(g, degradedApp)
				g.Expect(app.Status.Phase).ShouldNot(Equal(common.ApplicationRunning))
				g.Expect(app.Status.Services).Should(HaveLen(3))

				bad := findServiceStatus(g, app, "bad-component")
				g.Expect(bad.Healthy).Should(BeFalse())
				for _, traitType := range []string{defs.deploymentTrait, defs.cmTrait} {
					trait := findTraitStatus(g, app, "bad-component", traitType)
					g.Expect(trait.Healthy).Should(BeFalse())
					g.Expect(trait.Pending).Should(BeTrue())
					g.Expect(trait.Message).Should(ContainSubstring("Waiting for component to be healthy"))
				}
				scaler := findTraitStatus(g, app, "bad-component", "scaler")
				g.Expect(scaler.Healthy).Should(BeTrue())
				g.Expect(scaler.Pending).Should(BeFalse())

				two := findServiceStatus(g, app, "two-replica-component")
				g.Expect(two.Healthy).Should(BeFalse())
				g.Expect(findTraitStatus(g, app, "two-replica-component", defs.deploymentTrait).Healthy).Should(BeTrue())
				twoCM := findTraitStatus(g, app, "two-replica-component", defs.cmTrait)
				g.Expect(twoCM.Healthy).Should(BeFalse())
				g.Expect(twoCM.Pending).Should(BeFalse())

				good := findServiceStatus(g, app, "good-component")
				g.Expect(good.Healthy).Should(BeTrue())
				g.Expect(good.Traits).Should(HaveLen(3))
				for _, trait := range good.Traits {
					g.Expect(trait.Healthy).Should(BeTrue(), "trait %s", trait.Type)
					g.Expect(trait.Pending).Should(BeFalse(), "trait %s", trait.Type)
				}
			}).WithTimeout(3 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("Should surface unhealthy status when a PostDispatch trait's workload stops being ready", func() {
			app := postDispatchApp(namespace, "app-postdispatch-flip", common.ApplicationComponent{
				Name:       "flip-component",
				Type:       "webservice",
				Properties: rawJSON(`{"image":"nginx:1.21","port":80,"cpu":"100m","memory":"128Mi"}`),
				Traits: []common.ApplicationTrait{
					{Type: "scaler", Properties: rawJSON(`{"replicas":1}`)},
					{Type: defs.flipTrait, Properties: rawJSON(`{"name":"trait-deployment-flip","image":"nginx:1.21"}`)},
				},
			})
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			By("The trait is healthy while its pod is ready")
			EventuallyReconciled(ctx, app, func(g Gomega) {
				current := getApp(g, app)
				g.Expect(current.Status.Phase).Should(Equal(common.ApplicationRunning))
				g.Expect(findServiceStatus(g, current, "flip-component").Healthy).Should(BeTrue())
				g.Expect(findTraitStatus(g, current, "flip-component", defs.flipTrait).Healthy).Should(BeTrue())
			}).WithTimeout(time.Minute).WithPolling(time.Second).Should(Succeed())

			By("The trait and its component turn unhealthy once the pod stops being ready")
			EventuallyReconciled(ctx, app, func(g Gomega) {
				current := getApp(g, app)
				g.Expect(findServiceStatus(g, current, "flip-component").Healthy).Should(BeFalse())
				g.Expect(findTraitStatus(g, current, "flip-component", defs.flipTrait).Healthy).Should(BeFalse())
			}).WithTimeout(time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		})
	})

	Context("Component without a health policy", func() {
		It("Should render a PostDispatch trait from the component's status once the workload reports it", func() {
			EventuallyReconciled(ctx, noHealthApp, func(g Gomega) {
				app := getApp(g, noHealthApp)
				g.Expect(app.Status.Workflow).ShouldNot(BeNil())
				g.Expect(string(app.Status.Workflow.Phase)).Should(Equal("succeeded"))
				g.Expect(findTraitStatus(g, app, "no-health-component", defs.statusTrait).Healthy).Should(BeTrue())
				g.Expect(configMapData(g, "no-health-component-status")).Should(And(
					HaveKeyWithValue("replicas", "1"),
					HaveKeyWithValue("readyReplicas", "1"),
				))
			}).WithTimeout(time.Minute).WithPolling(time.Second).Should(Succeed())
		})
	})
})

// postDispatchDefs names the definitions the PostDispatch specs share. Names
// carry a random suffix since the definitions live in vela-system.
type postDispatchDefs struct {
	deploymentTrait string
	cmTrait         string
	flipTrait       string
	statusTrait     string
	markerTrait     string
	workerComp      string
	slowComp        string
	noHealthComp    string
}

func createPostDispatchDefs(ctx context.Context) postDispatchDefs {
	suffix := randomNamespaceName("")
	defs := postDispatchDefs{
		deploymentTrait: "test-deployment-trait" + suffix,
		cmTrait:         "test-cm-trait" + suffix,
		flipTrait:       "test-flip-trait" + suffix,
		statusTrait:     "test-status-trait" + suffix,
		markerTrait:     "test-marker-trait" + suffix,
		workerComp:      "test-worker" + suffix,
		slowComp:        "test-slow-worker" + suffix,
		noHealthComp:    "test-no-health" + suffix,
	}

	objects := []client.Object{
		postDispatchTrait(defs.deploymentTrait, traitDeploymentTemplate(""), deploymentHealthPolicy),
		postDispatchTrait(defs.cmTrait, statusConfigMapTemplate, `cm: context.outputs.statusConfigMap
isHealth: cm.data.readyReplicas != "2"
`),
		postDispatchTrait(defs.flipTrait, traitDeploymentTemplate(flipContainer), deploymentHealthPolicy),
		postDispatchTrait(defs.statusTrait, statusConfigMapTemplate, ""),
		postDispatchTrait(defs.markerTrait, `
outputs: marker: {
	apiVersion: "v1"
	kind: "ConfigMap"
	metadata: {
		name: context.name + "-marker"
		namespace: context.namespace
	}
	data: status: "deployed"
}
`, ""),
		deploymentComponent(defs.workerComp, "", `isHealth: context.output.status.readyReplicas > 0`),
		deploymentComponent(defs.slowComp, `
		readinessProbe: {
			httpGet: {
				path: "/"
				port: 80
			}
			initialDelaySeconds: 10
			periodSeconds: 2
		}`, `isHealth: context.output.status.readyReplicas > 0`),
		deploymentComponent(defs.noHealthComp, "", ""),
	}
	for _, obj := range objects {
		Expect(k8sClient.Create(ctx, obj)).Should(Succeed())
	}
	DeferCleanup(func() {
		for _, obj := range objects {
			_ = k8sClient.Delete(ctx, obj)
		}
	})
	return defs
}

func postDispatchTrait(name, template, healthPolicy string) *v1beta1.TraitDefinition {
	def := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vela-system"},
		Spec: v1beta1.TraitDefinitionSpec{
			Stage:     v1beta1.PostDispatch,
			Schematic: &common.Schematic{CUE: &common.CUE{Template: template}},
		},
	}
	if healthPolicy != "" {
		def.Spec.Status = &common.Status{HealthPolicy: healthPolicy}
	}
	return def
}

func deploymentComponent(name, containerExtra, healthPolicy string) *v1beta1.ComponentDefinition {
	def := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vela-system"},
		Spec: v1beta1.ComponentDefinitionSpec{
			Workload: common.WorkloadTypeDescriptor{
				Definition: common.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"},
			},
			Schematic: &common.Schematic{CUE: &common.CUE{Template: fmt.Sprintf(`
output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: {
		name: parameter.name
		labels: app: parameter.name
	}
	spec: {
		replicas: parameter.replicas
		selector: matchLabels: app: parameter.name
		template: {
			metadata: labels: app: parameter.name
			spec: containers: [{
				name: parameter.name
				image: parameter.image%s
			}]
		}
	}
}

parameter: {
	name: string
	image: string
	replicas: *1 | int
}
`, containerExtra)}},
		},
	}
	if healthPolicy != "" {
		def.Spec.Status = &common.Status{HealthPolicy: healthPolicy}
	}
	return def
}

// traitDeploymentTemplate renders a Deployment sized from the component's
// reported replicas, so it can only be dispatched once that status exists.
func traitDeploymentTemplate(containerExtra string) string {
	return fmt.Sprintf(`
outputs: statusPod: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: name: parameter.name
	spec: {
		replicas: context.output.status.replicas
		selector: matchLabels: app: parameter.name
		template: {
			metadata: labels: app: parameter.name
			spec: containers: [{
				name: parameter.name
				image: parameter.image%s
			}]
		}
	}
}

parameter: {
	name: string
	image: string
}
`, containerExtra)
}

// flipContainer is ready for its first 30 seconds and never again, a
// permanent state any reconcile observes.
const flipContainer = `
				command: ["sh", "-c", "touch /tmp/ready && sleep 30 && rm /tmp/ready && sleep 3600"]
				readinessProbe: {
					exec: command: ["cat", "/tmp/ready"]
					periodSeconds: 1
				}`

const statusConfigMapTemplate = `
outputs: statusConfigMap: {
	apiVersion: "v1"
	kind: "ConfigMap"
	metadata: {
		name: context.name + "-status"
		namespace: context.namespace
	}
	data: {
		replicas: "\(context.output.status.replicas)"
		readyReplicas: "\(context.output.status.readyReplicas)"
		componentName: context.name
	}
}
`

const deploymentHealthPolicy = `pod: context.outputs.statusPod
ready: {
	updatedReplicas:    *0 | int
	readyReplicas:      *0 | int
	replicas:           *0 | int
	observedGeneration: *0 | int
} & {
	if pod.status.updatedReplicas != _|_ {
		updatedReplicas: pod.status.updatedReplicas
	}
	if pod.status.readyReplicas != _|_ {
		readyReplicas: pod.status.readyReplicas
	}
	if pod.status.replicas != _|_ {
		replicas: pod.status.replicas
	}
	if pod.status.observedGeneration != _|_ {
		observedGeneration: pod.status.observedGeneration
	}
}
_isHealth: (pod.spec.replicas == ready.readyReplicas) && (pod.spec.replicas == ready.updatedReplicas) && (pod.spec.replicas == ready.replicas) && (ready.observedGeneration == pod.metadata.generation || ready.observedGeneration > pod.metadata.generation)
isHealth: *_isHealth | bool
if pod.metadata.annotations != _|_ {
	if pod.metadata.annotations["app.oam.dev/disable-health-check"] != _|_ {
		isHealth: true
	}
}
`

// webserviceWithStatusTraits is a webservice carrying both status-reading
// PostDispatch traits; its trait Deployment is named after the component.
func webserviceWithStatusTraits(defs postDispatchDefs, name string, replicas int, image string) common.ApplicationComponent {
	return common.ApplicationComponent{
		Name:       name,
		Type:       "webservice",
		Properties: rawJSON(fmt.Sprintf(`{"image":%q,"port":80,"cpu":"100m","memory":"128Mi"}`, image)),
		Traits: []common.ApplicationTrait{
			{Type: "scaler", Properties: rawJSON(fmt.Sprintf(`{"replicas":%d}`, replicas))},
			{Type: defs.deploymentTrait, Properties: rawJSON(fmt.Sprintf(`{"name":"trait-deployment-%s","image":"nginx:1.21"}`, name))},
			{Type: defs.cmTrait},
		},
	}
}

func postDispatchApp(namespace, name string, components ...common.ApplicationComponent) *v1beta1.Application {
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       v1beta1.ApplicationSpec{Components: components},
	}
}

func findServiceStatus(g Gomega, app *v1beta1.Application, component string) common.ApplicationComponentStatus {
	for _, svc := range app.Status.Services {
		if svc.Name == component {
			return svc
		}
	}
	g.Expect(app.Status.Services).Should(ContainElement(HaveField("Name", component)))
	return common.ApplicationComponentStatus{}
}

func findTraitStatus(g Gomega, app *v1beta1.Application, component, traitType string) common.ApplicationTraitStatus {
	svc := findServiceStatus(g, app, component)
	for _, trait := range svc.Traits {
		if trait.Type == traitType {
			return trait
		}
	}
	g.Expect(svc.Traits).Should(ContainElement(HaveField("Type", traitType)), "on component %s", component)
	return common.ApplicationTraitStatus{}
}

func rawJSON(s string) *runtime.RawExtension {
	return &runtime.RawExtension{Raw: []byte(s)}
}
