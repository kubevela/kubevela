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

package addonmoduletest

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func policyProperties(app *v1beta1.Application, name string) map[string]interface{} {
	for _, p := range app.Spec.Policies {
		if p.Name == name {
			return propertiesOf(p.Properties)
		}
	}
	Fail("policy " + name + " not found on " + app.Name)
	return nil
}

func workflowSteps(app *v1beta1.Application) map[string][]interface{} {
	Expect(app.Spec.Workflow).ShouldNot(BeNil(), "Application %s has no workflow", app.Name)
	steps := map[string][]interface{}{}
	for _, step := range app.Spec.Workflow.Steps {
		policies, _ := propertiesOf(step.Properties)["policies"].([]interface{})
		steps[step.Name] = policies
	}
	return steps
}

// withPlacement sets placement on the type: addon component, next to its
// registry and version, the same place a type: module component takes it.
func withPlacement(app *v1beta1.Application, place map[string]interface{}) *v1beta1.Application {
	props := propertiesOf(app.Spec.Components[0].Properties)
	props["placement"] = place
	raw, err := json.Marshal(props)
	Expect(err).ShouldNot(HaveOccurred())
	app.Spec.Components[0].Properties = &k8sruntime.RawExtension{Raw: raw}
	return app
}

var _ = Describe("Addons that place an imported module", func() {
	ctx := context.Background()

	// --- Scenario 20 ---
	// The catalog's multi-cluster shape: a clusters parameter feeds one
	// topology policy, there is no workflow, and the import points at that
	// policy. CI runs a single cluster, so the addon asks for the local one;
	// the assertions are about the wiring.
	Context("an addon with a topology policy and placement.fromPolicy (scenario 20)", Ordered, Label("addon-module-scenario-20"), func() {
		scope := scenarioScopes["20"]
		var addonApp, moduleApp, moduleComponent string

		BeforeAll(func() {
			addonApp = scope.Text("addon-placement-import")
			moduleApp = scope.Text(fixtureModuleWidgetKit)
			moduleComponent = scope.Text("widget-kit")
			scope.prepare(ctx)

			Expect(k8sClient.Create(ctx, scope.addonApplication(scope.Text("placement-import"), scope.Text("placement-import"), "1.0.0",
				map[string]interface{}{"clusters": []interface{}{"local"}}))).Should(Succeed())
			waitAppRunning(ctx, scope.Namespace, scope.Text("placement-import"), installWait)
			waitAppRunning(ctx, systemNS, addonApp, shortWait)
			waitAppRunning(ctx, systemNS, moduleApp, shortWait)
			DeferCleanup(func() {
				scope.uninstall(ctx, scope.Text("placement-import"), addonApp, moduleApp)
			})
		})

		It("forwards the named topology policy's clusters as the module's placement", func() {
			mod := findComponent(mustGetApp(ctx, systemNS, addonApp), moduleComponent)
			Expect(mod.Type).Should(Equal("module"))
			Expect(propertiesOf(mod.Properties)["placement"]).Should(Equal(map[string]interface{}{"clusters": []interface{}{"local"}}))
		})

		It("keeps the module component on the hub with its own deploy step", func() {
			app := mustGetApp(ctx, systemNS, addonApp)
			Expect(workflowSteps(app)).Should(Equal(map[string][]interface{}{
				"deploy-topo-runtime":  {"addon-components", "topo-runtime"},
				"deploy-addon-modules": {"addon-modules-hub", "addon-modules"},
			}), "the addon's topology deploys everything but the module, as the default workflow would")
			Expect(policyProperties(app, "addon-modules-hub")).Should(Equal(map[string]interface{}{"clusters": []interface{}{"local"}}))
			Expect(policyProperties(app, "addon-modules")).Should(Equal(map[string]interface{}{"selector": []interface{}{moduleComponent}}))
			Expect(policyProperties(app, "addon-components")["selector"]).ShouldNot(ContainElement(moduleComponent))
			Expect(findComponent(app, moduleComponent).DependsOn).Should(BeEmpty(),
				"a deploy step waits only for components it deploys, so the step order replaces dependsOn")
		})

		It("renders the module's runtime policy from the forwarded placement", func() {
			owned := mustGetApp(ctx, systemNS, moduleApp)
			Expect(owned.Labels).Should(HaveKeyWithValue(oam.LabelAppCluster, "local"), "the module Application was deployed through a topology policy")
			Expect(policyProperties(owned, "module-runtime")).Should(Equal(map[string]interface{}{"clusters": []interface{}{"local"}}))
			Expect(policyProperties(owned, "module-hub")).Should(Equal(map[string]interface{}{"clusters": []interface{}{"local"}}))
		})
	})

	// --- Scenario 21 ---
	// placement on the type: addon component, in the same form as a type:
	// module component's. An addon with no topology of its own takes it and
	// its imports follow it; an addon that places itself refuses it.
	Context("placement on a type: addon component (scenario 21)", Ordered, Label("addon-module-scenario-21"), func() {
		scope := scenarioScopes["21"]
		var addonApp, moduleApp, moduleComponent string

		BeforeAll(func() {
			addonApp = scope.Text("addon-placement-plain")
			moduleApp = scope.Text(fixtureModuleWidgetKit)
			moduleComponent = scope.Text("widget-kit")
			scope.prepare(ctx)

			Expect(k8sClient.Create(ctx, withPlacement(scope.addonApplication(scope.Text("placement-plain"), scope.Text("placement-plain"), "1.0.0", nil),
				map[string]interface{}{"clusters": []interface{}{"local"}}))).Should(Succeed())
			waitAppRunning(ctx, scope.Namespace, scope.Text("placement-plain"), installWait)
			waitAppRunning(ctx, systemNS, addonApp, shortWait)
			waitAppRunning(ctx, systemNS, moduleApp, shortWait)
			DeferCleanup(func() {
				scope.uninstall(ctx, scope.Text("placement-plain"), addonApp, moduleApp)
			})
		})

		It("places the addon with a topology built from the placement", func() {
			app := mustGetApp(ctx, systemNS, addonApp)
			Expect(policyProperties(app, "addon-placement")).Should(Equal(map[string]interface{}{"clusters": []interface{}{"local"}}))
			Expect(workflowSteps(app)).Should(HaveKeyWithValue("deploy-addon-modules", []interface{}{"addon-modules-hub", "addon-modules"}),
				"the module component stays on the hub")
		})

		It("gives the import with no placement of its own the addon's placement", func() {
			mod := findComponent(mustGetApp(ctx, systemNS, addonApp), moduleComponent)
			Expect(propertiesOf(mod.Properties)["placement"]).Should(Equal(map[string]interface{}{"clusters": []interface{}{"local"}}))
			Expect(policyProperties(mustGetApp(ctx, systemNS, moduleApp), "module-runtime")).Should(Equal(map[string]interface{}{"clusters": []interface{}{"local"}}))
		})

		It("refuses placement on an addon that defines its own topology", func() {
			name := scope.Text("placement-import")
			Expect(k8sClient.Create(ctx, withPlacement(scope.addonApplication(name, name, "1.0.0", nil),
				map[string]interface{}{"clusters": []interface{}{"local"}}))).Should(Succeed())
			DeferCleanup(func() { deleteApp(ctx, scope.Namespace, name) })
			Eventually(func(g Gomega) {
				app := &v1beta1.Application{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: scope.Namespace, Name: name}, app)).Should(Succeed())
				status, err := json.Marshal(app.Status)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(string(status)).Should(ContainSubstring(`places itself with its topology policy \"topo-runtime\", so placement cannot be set`))
			}, shortWait, pollInterval).Should(Succeed())
			Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: scope.Text("addon-placement-import"), Namespace: systemNS}})).Should(BeTrue(),
				"nothing is installed when the request is refused")
		})
	})
})
