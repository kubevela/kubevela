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

// This file is the cluster suite for addons that import modules. The
// synchronized suite setup publishes immutable fixtures once for all workers.
// Each scenario owns a namespace, API group and scoped addon/module/definition
// identities, so independent Ordered containers can use separate workers in
// one cluster. Registry mutations, controller restarts and gate changes are
// Serial. Scenarios 08 and 07 share one Ordered container and fixture scope.
//
// The Contexts run in the order written. Two orderings matter: "latest vs
// pinned" (scenario 08) has to run while widget-kit 1.0.0 is the only tag in
// the module registry, so it comes before the upgrade scenario (07) and
// publishes 1.1.0 and 1.2.0 itself. Global-state scenarios restore shared state
// and can run in any order after the parallel phase.
package addonmoduletest

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	veltypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const (
	fixtureWidgetPlatformApp   = "widget-platform"
	fixtureAddonWidgetPlatform = "addon-widget-platform"
	fixtureModuleWidgetKit     = "module-widget-kit"
	fixtureModuleGadgetKit     = "module-gadget-kit"
	fixtureModuleProbeKit      = "module-probe-kit"

	fixtureWidgetsCRD       = "widgets.kit.example.com"
	fixtureWidgetClassesCRD = "widgetclasses.kit.example.com"
	fixtureGadgetsCRD       = "gadgets.kit.example.com"
)

// widgetKitTiers is the tier order RenderApplication emits for widget-kit
// 1.0.0 and 1.1.0 (lines v1 and v2 enabled, v1beta1 disabled).
var fixtureWidgetKitTiers = []string{"widget-kit-aux", "widget-kit-v1-aux", "widget-kit-v1-defs", "widget-kit-v2-aux", "widget-kit-v2-defs"}

// widgetKitDefinitions is every definition widget-kit 1.0.0 installs.
var fixtureWidgetKitDefinitions = []string{"widget-kit-v1-widget", "widget-kit-v2-widget", "widget-kit-v1-labeler", "widget-kit-v1-note", "widget-kit-v2-labeler"}

func resolveAddonModuleRegistries(ctx context.Context) {
	moduleRegistry = resolveRegistryEndpoints(ctx, moduleRegistryURLEnv, moduleRegistryHostURLEnv, moduleRegistryNodePort, "/modules", "/v2/")
	addonRegistry = resolveRegistryEndpoints(ctx, addonRegistryURLEnv, addonRegistryHostURLEnv, addonRegistryNodePort, "", "/index.yaml")
}

func setupAddonModuleFixtures(ctx context.Context) {
	By("bringing up the in-cluster registries: registry:2 for modules, ChartMuseum for addons")
	Expect(applyManifestFile(ctx, testdataPath("registry.yaml"))).Should(Succeed())
	waitForDeploymentAvailable(ctx, "default", "oci-registry")
	waitForDeploymentAvailable(ctx, "default", "addon-module-chartmuseum")
	resolveAddonModuleRegistries(ctx)

	By("registering the module registry and publishing isolated scenario fixtures")
	runVelaSucceed("module", "registry", "add", moduleRegistryName, moduleRegistry.cluster, "--type", "oci")
	// Publish every initial addon version before any render so the index
	// cache cannot hide another worker's newly published scenario identity.
	for _, id := range scenarioIDs {
		scenarioScopes[id].publishInitialFixtures()
	}
	addAddonRegistry(ctx)
}

func cleanupAddonModuleFixtures(ctx context.Context) {
	By("removing only this run's scenario resources")
	for _, id := range scenarioIDs {
		scenarioScopes[id].cleanup(ctx)
	}
	_, _ = runVela("module", "registry", "delete", moduleRegistryName)
	_, _ = runVela("addon", "registry", "delete", addonRegistryName)
	deleteManifestFile(ctx, testdataPath("registry.yaml"))
}

var _ = Describe("Addons that import modules", func() {
	ctx := context.Background()
	crd := func(name string) *apiextensionsv1.CustomResourceDefinition {
		return &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: name}}
	}

	// --- Scenario 01 ---
	Context("publish and inspect artifacts (scenario 01)", Label("addon-module-scenario-01"), func() {
		scope := scenarioScopes["01"]

		It("published widget-kit 1.0.0 as the only tag of its repository", func() {
			Expect(ociTags(moduleRegistry.hostBase, scope.Text("modules/widget-kit"))).Should(ConsistOf("1.0.0"), scope.Text("the latest-vs-pinned scenario needs 1.0.0 to be the highest widget-kit tag; a registry left over from an earlier run breaks that"))
		})

		It("vela module deploy --dry-run fetches the module and prints the wrapper Application", func() {
			skipUnlessHostReachesCluster(moduleRegistry, "vela module deploy --dry-run (a client-side fetch through the stored registry record)")
			out := runVelaSucceed("module", "deploy", scope.Text("widget-kit"), "--registry", moduleRegistryName, "--version", "1.0.0", "-n", systemNS, "--dry-run")
			Expect(out).Should(ContainSubstring(scope.Text("module-widget-kit-deploy")))
			Expect(out).Should(ContainSubstring("type: module"))
			Expect(out).Should(ContainSubstring("registry: " + moduleRegistryName))
			Expect(out).Should(ContainSubstring("version: 1.0.0"))

			out, err := runVela("module", "deploy", scope.Text("widget-kit"), "--registry", moduleRegistryName, "--version", "9.9.9", "-n", systemNS, "--dry-run")
			Expect(err).Should(HaveOccurred(), out)
			Expect(out).Should(ContainSubstring("9.9.9"))
		})
	})

	// --- Scenario 02 ---
	Context("single-module addon install (scenario 02)", Ordered, Label("addon-module-scenario-02"), func() {
		scope := scenarioScopes["02"]
		var widgetPlatformApp string
		var addonWidgetPlatform string
		var moduleWidgetKit string
		var widgetsCRD string
		var widgetClassesCRD string
		var widgetKitTiers []string

		var widgetKitDefinitions []string
		var testNS string
		widgetGVK := widgetGVK
		widgetClassGVK := widgetClassGVK
		addonApplication := scope.addonApplication
		testdataPath := scope.Path
		uninstall := func(app, addon string, modules ...string) {
			scope.uninstall(ctx, app, addon, modules...)
		}
		BeforeAll(func() {
			widgetPlatformApp = scope.Text(fixtureWidgetPlatformApp)
			addonWidgetPlatform = scope.Text(fixtureAddonWidgetPlatform)
			moduleWidgetKit = scope.Text(fixtureModuleWidgetKit)
			widgetsCRD = scope.Text(fixtureWidgetsCRD)
			widgetClassesCRD = scope.Text(fixtureWidgetClassesCRD)
			widgetKitTiers = scope.Texts(fixtureWidgetKitTiers)
			widgetKitDefinitions = scope.Texts(fixtureWidgetKitDefinitions)
			testNS = scope.Namespace
			widgetGVK = scope.GVK("Widget")
			widgetClassGVK = scope.GVK("WidgetClass")
			scope.prepare(ctx)
		})

		BeforeAll(func() {
			By("installing widget-platform 1.0.0 with addon parameters")
			Expect(k8sClient.Create(ctx, addonApplication(widgetPlatformApp, scope.Text("widget-platform"), "1.0.0", map[string]interface{}{
				"greeting": "hello from scenario 02",
				"team":     "team-blue",
			}))).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
			waitAppRunning(ctx, systemNS, addonWidgetPlatform, shortWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
		})

		It("level 1: the user Application reports the seven generated components healthy", func() {
			Eventually(func(g Gomega) {
				app := getAppG(g, ctx, testNS, widgetPlatformApp)
				svc := findService(app, scope.Text("widget-platform"))
				g.Expect(svc).ShouldNot(BeNil(), appStatusText(app))
				g.Expect(svc.Healthy).Should(BeTrue(), svc.Message)
				g.Expect(svc.Message).Should(Equal("Ready:7/7"))
			}, shortWait, pollInterval).Should(Succeed())
		})

		It("level 2: the addon Application has the addon's components plus one generated module component", func() {
			app := mustGetApp(ctx, systemNS, addonWidgetPlatform)

			By("labels naming the addon, its version, the registry that served it and the owning Application")
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAddonName, scope.Text("widget-platform")))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAddonVersion, "1.0.0"))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAddonRegistry, addonRegistryName))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAppName, widgetPlatformApp))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAppNamespace, testNS))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAppComponent, scope.Text("widget-platform")))

			By("components in render order: template, resources, module import, auxiliary groups")
			Expect(componentNames(app)).Should(Equal([]string{
				"platform-info", scope.Text("widget-platform-resources"), "team-config", scope.Text("widget-kit"), "addon-definitions", "addon-secret", "addon-auxiliaries",
			}))
			Expect(findComponent(app, "platform-info").Type).Should(Equal("k8s-objects"))
			Expect(findComponent(app, "platform-info").DependsOn).Should(Equal([]string{scope.Text("widget-platform-resources")}))
			Expect(findComponent(app, "team-config").DependsOn).Should(Equal([]string{scope.Text("widget-platform-resources")}))

			By("the generated module component depending on the resource components and the outputs, not on template components")
			mod := findComponent(app, scope.Text("widget-kit"))
			Expect(mod.Type).Should(Equal("module"))
			Expect(mod.DependsOn).Should(ConsistOf(scope.Text("widget-platform-resources"), "team-config", "addon-auxiliaries"))
			Expect(propertiesOf(mod.Properties)).Should(Equal(map[string]interface{}{
				"module": scope.Text("widget-kit"), "registry": moduleRegistryName, "version": "1.0.0",
			}))

			By("the state-keep policy and the skipped last-applied annotation")
			Expect(app.Spec.Policies).Should(HaveLen(1))
			Expect(app.Spec.Policies[0].Name).Should(Equal("addon-component-state-keep"))
			Expect(app.Spec.Policies[0].Type).Should(Equal("apply-once"))
			Expect(propertiesOf(app.Spec.Policies[0].Properties)).Should(HaveKeyWithValue("enable", false))
			Expect(app.Annotations).Should(HaveKeyWithValue(oam.AnnotationLastAppliedConfig, "skip"))

			By("every service healthy, the module one saying Ready:5/5")
			for _, svc := range app.Status.Services {
				Expect(svc.Healthy).Should(BeTrue(), "%s: %s", svc.Name, svc.Message)
			}
			Expect(findService(app, scope.Text("widget-kit")).Message).Should(Equal("Ready:5/5"))
		})

		It("level 2: the addon's own objects, parameters and definition are installed", func() {
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: scope.Text("widget-platform-system")}, &corev1.Namespace{})).Should(Succeed())

			cm, err := getConfigMap(ctx, systemNS, scope.Text("widget-platform-config"))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(cm.Data).Should(HaveKeyWithValue("source", "resources/platform-config.yaml"))

			notes, err := getConfigMap(ctx, systemNS, scope.Text("widget-platform-notes"))
			Expect(err).ShouldNot(HaveOccurred(), "template.cue outputs become the addon-auxiliaries component")
			Expect(notes.Data).Should(HaveKeyWithValue("greeting", "hello from scenario 02"))
			Expect(notes.Data).Should(HaveKeyWithValue("addonVersion", "1.0.0"))

			info, err := getConfigMap(ctx, scope.Text("widget-platform-system"), scope.Text("widget-platform-info"))
			Expect(err).ShouldNot(HaveOccurred(), "a package main resource file is merged into template.cue")
			Expect(info.Data).Should(HaveKeyWithValue("greeting", "hello from scenario 02"))

			team, err := getConfigMap(ctx, scope.Text("widget-platform-system"), scope.Text("widget-platform-team"))
			Expect(err).ShouldNot(HaveOccurred(), "a resource .cue file without a package header renders as its own component")
			Expect(team.Data).Should(HaveKeyWithValue("team", "team-blue"))

			var secret corev1.Secret
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Namespace: systemNS, Name: scope.Text("addon-secret-widget-platform")}, &secret)).Should(Succeed())
			Expect(string(secret.Data["addonParameterDataKey"])).Should(SatisfyAll(
				ContainSubstring(`"greeting":"hello from scenario 02"`), ContainSubstring(`"team":"team-blue"`)))

			owner, err := getTraitDefinition(ctx, systemNS, scope.Text("platform-owner"))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(owner.Labels).ShouldNot(HaveKey(veltypes.LabelDefinitionModule), "an addon-level definition carries no module labels")
		})

		It("level 3: the module Application has one tier per level and enabled line, in dependency order", func() {
			app := mustGetApp(ctx, systemNS, moduleWidgetKit)
			Expect(app.Labels).Should(HaveKeyWithValue(veltypes.LabelDefinitionModule, scope.Text("widget-kit")))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAppName, addonWidgetPlatform))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAppComponent, scope.Text("widget-kit")))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAddonName, scope.Text("widget-platform")), "the addon Application's labels are merged onto everything it dispatches")
			Expect(app.Annotations).Should(HaveKeyWithValue(veltypes.AnnoDefinitionModuleVersion, "1.0.0"))

			Expect(componentNames(app)).Should(Equal(widgetKitTiers))
			Expect(findComponent(app, scope.Text("widget-kit-aux")).DependsOn).Should(BeEmpty())
			Expect(findComponent(app, scope.Text("widget-kit-v1-aux")).DependsOn).Should(Equal([]string{scope.Text("widget-kit-aux")}))
			Expect(findComponent(app, scope.Text("widget-kit-v1-defs")).DependsOn).Should(Equal([]string{scope.Text("widget-kit-v1-aux")}))
			Expect(findComponent(app, scope.Text("widget-kit-v2-aux")).DependsOn).Should(Equal([]string{scope.Text("widget-kit-aux")}), "lines are siblings: v2 never waits for v1")
			Expect(findComponent(app, scope.Text("widget-kit-v2-defs")).DependsOn).Should(Equal([]string{scope.Text("widget-kit-v2-aux")}))
			for _, c := range app.Spec.Components {
				Expect(c.Type).Should(Equal("k8s-objects"), c.Name)
			}
		})

		It("level 3: the module installed its CRDs, auxiliary objects and stamped definitions, and nothing from the disabled line", func() {
			By("module-level auxiliary objects: two CRDs from one multi-document file, a ConfigMap from CUE, a ClusterRole")
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: widgetsCRD}, crd(widgetsCRD))).Should(Succeed())
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: widgetClassesCRD}, crd(widgetClassesCRD))).Should(Succeed())
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: scope.Text("widget-kit-viewer")}, &rbacv1.ClusterRole{})).Should(Succeed())
			info, err := getConfigMap(ctx, systemNS, scope.Text("widget-kit-module-info"))
			Expect(err).ShouldNot(HaveOccurred(), "an auxiliary object without a namespace lands in the definition namespace")
			Expect(info.Data).Should(HaveKeyWithValue("moduleVersion", "1.0.0"))

			By("line-level auxiliary objects: a WidgetClass and a ConfigMap per enabled line")
			for _, line := range []string{"v1", "v2"} {
				className := map[string]string{"v1": scope.Text("widget-kit-v1-standard"), "v2": scope.Text("widget-kit-v2-premium")}[line]
				class, err := getUnstructured(ctx, widgetClassGVK, "", className)
				Expect(err).ShouldNot(HaveOccurred(), className)
				Expect(class.GetLabels()).Should(HaveKeyWithValue(scope.Text("kit.example.com/line"), line))
				_, err = getConfigMap(ctx, systemNS, scope.Text("widget-kit-")+line+"-line-config")
				Expect(err).ShouldNot(HaveOccurred())
			}

			By("five stamped definitions with module labels")
			Expect(moduleDefinitionNames(ctx, systemNS, scope.Text("widget-kit"))).Should(ConsistOf(widgetKitDefinitions))
			cd, err := getComponentDefinition(ctx, systemNS, scope.Text("widget-kit-v1-widget"))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(cd.Labels).Should(HaveKeyWithValue(veltypes.LabelDefinitionModuleAPIVersion, "v1"))
			Expect(cd.Labels).Should(HaveKeyWithValue(veltypes.LabelDefinitionName, scope.Text("widget")))
			Expect(cd.Annotations).Should(HaveKeyWithValue(veltypes.AnnoDefinitionModuleFullName, scope.Text("widget-kit-v1-widget")))
			// stampIdentity (pkg/module/service/render.go) writes
			// addons.oam.dev/name=<module> into the manifest, but that is not what
			// reaches the cluster on this path. An Application merges its own
			// labels onto every object it dispatches
			// (generateAndFilterCommonLabels in pkg/appfile/appfile.go), so the
			// addons.oam.dev/{name,version,registry} labels the renderer put on
			// addon-widget-platform travel onto module-widget-kit and from there
			// onto the definitions, overriding the module's stamp. A module
			// installed through "vela module deploy" keeps the module name here;
			// one installed through an addon is grouped under that addon.
			Expect(cd.Labels).Should(HaveKeyWithValue(oam.LabelAddonName, scope.Text("widget-platform")))
			Expect(cd.Labels).Should(HaveKeyWithValue(oam.LabelAddonVersion, "1.0.0"))
			Expect(cd.Labels).Should(HaveKeyWithValue(oam.LabelAddonRegistry, addonRegistryName))
			v2, err := getComponentDefinition(ctx, systemNS, scope.Text("widget-kit-v2-widget"))
			Expect(err).ShouldNot(HaveOccurred(), "a YAML definition installs like a CUE one")
			Expect(v2.Labels).Should(HaveKeyWithValue(veltypes.LabelDefinitionModuleAPIVersion, "v2"))

			By("the disabled v1beta1 line leaving nothing behind")
			Expect(isNotFound(ctx, componentDefinitionObj(systemNS, scope.Text("widget-kit-v1beta1-widget")))).Should(BeTrue())
			Expect(isNotFound(ctx, configMapObj(systemNS, scope.Text("widget-kit-v1beta1-preview")))).Should(BeTrue())
		})

		It("a consumer uses Form 3, a module trait and the addon-level trait", func() {
			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-widget.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, "widget-consumer", shortWait)

			blue, err := getUnstructured(ctx, widgetGVK, testNS, "blue-widget")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(nestedString(blue, "spec", "classRef")).Should(Equal(scope.Text("widget-kit-v1-standard")))
			Expect(nestedString(blue, "spec", "apiLine")).Should(Equal("v1"))
			Expect(nestedString(blue, "spec", "color")).Should(Equal("blue"))
			Expect(nestedString(blue, "spec", "replicas")).Should(Equal("2"))
			Expect(blue.GetLabels()).Should(HaveKeyWithValue(scope.Text("kit.example.com/tier"), "gold"))
			Expect(blue.GetLabels()).Should(HaveKeyWithValue(scope.Text("kit.example.com/labeled-by"), scope.Text("widget-kit-v1-labeler")))
			Expect(blue.GetLabels()).Should(HaveKeyWithValue(oam.WorkloadTypeLabel, scope.Text("widget-kit-v1-widget")), scope.Text("the workload type label carries the resolved name, not the widget-kit/v1/widget spelling"))
			Expect(blue.GetAnnotations()).Should(HaveKeyWithValue(scope.Text("kit.example.com/owner"), "team-blue"))

			big, err := getUnstructured(ctx, widgetGVK, testNS, "big-widget")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(nestedString(big, "spec", "classRef")).Should(Equal(scope.Text("widget-kit-v2-premium")))
			Expect(nestedString(big, "spec", "size")).Should(Equal("L"))
			Expect(big.GetLabels()).Should(HaveKeyWithValue(scope.Text("kit.example.com/tier"), "silver"))
			Expect(big.GetLabels()).Should(HaveKeyWithValue(scope.Text("kit.example.com/line"), "v2"))
			Expect(big.GetLabels()).Should(HaveKeyWithValue(scope.Text("kit.example.com/labeled-by"), scope.Text("widget-kit-v2-labeler")))

			card, err := getConfigMap(ctx, testNS, "blue-widget-card")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(card.Data).Should(Equal(map[string]string{"color": "blue", "line": "v1", "module": scope.Text("widget-kit")}))
		})

		It("every level has its own ResourceTracker", func() {
			for _, name := range []string{widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit, "widget-consumer"} {
				var trackers v1beta1.ResourceTrackerList
				namespace := testNS
				if name == addonWidgetPlatform || name == moduleWidgetKit {
					namespace = systemNS
				}
				Expect(k8sClient.List(ctx, &trackers, client.MatchingLabels{oam.LabelAppName: name, oam.LabelAppNamespace: namespace})).Should(Succeed())
				Expect(trackers.Items).ShouldNot(BeEmpty(), "no ResourceTracker for %s", name)
			}
		})

		It("deleting the user Application cascades through every level", func() {
			deleteApp(ctx, testNS, "widget-consumer")
			waitGone(ctx, unstructuredObj(widgetGVK, testNS, "blue-widget"), shortWait)

			uninstall(widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit)
			waitGone(ctx, crd(widgetClassesCRD), shortWait)
			waitGone(ctx, componentDefinitionObj(systemNS, scope.Text("widget-kit-v1-widget")), shortWait)
			waitGone(ctx, traitDefinitionObj(systemNS, scope.Text("platform-owner")), shortWait)
			waitGone(ctx, configMapObj(systemNS, scope.Text("widget-kit-module-info")), shortWait)
			waitGone(ctx, configMapObj(systemNS, scope.Text("widget-platform-notes")), shortWait)
			waitGone(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: scope.Text("widget-platform-system")}}, reconcileWait)
		})
	})

	// --- Scenario 03 ---
	Context("one addon, two modules (scenario 03)", Ordered, Label("addon-module-scenario-03"), func() {
		scope := scenarioScopes["03"]
		var moduleWidgetKit string
		var moduleGadgetKit string
		var gadgetsCRD string
		var testNS string
		widgetGVK := widgetGVK
		gadgetGVK := gadgetGVK
		addonApplication := scope.addonApplication
		testdataPath := scope.Path

		uninstall := func(app, addon string, modules ...string) {
			scope.uninstall(ctx, app, addon, modules...)
		}
		BeforeAll(func() {
			moduleWidgetKit = scope.Text(fixtureModuleWidgetKit)
			moduleGadgetKit = scope.Text(
				fixtureModuleGadgetKit)
			gadgetsCRD = scope.Text(fixtureGadgetsCRD)
			testNS = scope.Namespace
			widgetGVK =
				scope.GVK("Widget")
			gadgetGVK = scope.GVK("Gadget")
			scope.prepare(ctx)
		})

		BeforeAll(func() {
			Expect(k8sClient.Create(ctx, addonApplication(scope.Text("kit-suite"), scope.Text("kit-suite"), "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, scope.Text("kit-suite"), installWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
			waitAppRunning(ctx, systemNS, moduleGadgetKit, shortWait)
			DeferCleanup(func() {
				deleteApp(ctx, testNS, "suite-consumer")
				uninstall(scope.Text("kit-suite"), scope.Text("addon-kit-suite"), moduleWidgetKit, moduleGadgetKit)
			})
		})

		It("fans one addon Application out into two module components and two module Applications", func() {
			app := mustGetApp(ctx, systemNS, scope.Text("addon-kit-suite"))
			Expect(componentNames(app)).Should(Equal([]string{scope.Text("kit-suite-resources"), scope.Text("widget-kit"), scope.Text("gadget-kit")}))
			for _, name := range []string{scope.Text("widget-kit"), scope.Text("gadget-kit")} {
				c := findComponent(app, name)
				Expect(c.Type).Should(Equal("module"))
				Expect(c.DependsOn).Should(Equal([]string{scope.Text("kit-suite-resources")}), scope.Text("kit-suite has no outputs, so there is no addon-auxiliaries dependency"))
				Expect(propertiesOf(c.Properties)).Should(Equal(map[string]interface{}{"module": name, "registry": moduleRegistryName, "version": "1.0.0"}))
			}
			Expect(findService(app, scope.Text("widget-kit")).Message).Should(Equal("Ready:5/5"))
			Expect(findService(app, scope.Text("gadget-kit")).Message).Should(Equal("Ready:3/3"))
			Expect(findService(mustGetApp(ctx, testNS, scope.Text("kit-suite")), scope.Text("kit-suite")).Message).Should(Equal("Ready:3/3"))

			for _, name := range []string{moduleWidgetKit, moduleGadgetKit} {
				mod := mustGetApp(ctx, systemNS, name)
				Expect(mod.Labels).Should(HaveKeyWithValue(oam.LabelAppName, scope.Text("addon-kit-suite")))
				Expect(mod.Labels).Should(HaveKeyWithValue(oam.LabelAppComponent, strings.TrimPrefix(name, "module-")))
			}
		})

		It("installs gadget-kit's three tiers and objects", func() {
			mod := mustGetApp(ctx, systemNS, moduleGadgetKit)
			Expect(componentNames(mod)).Should(Equal([]string{scope.Text("gadget-kit-aux"), scope.Text("gadget-kit-v1-aux"), scope.Text("gadget-kit-v1-defs")}))
			Expect(findComponent(mod, scope.Text("gadget-kit-v1-aux")).DependsOn).Should(Equal([]string{scope.Text("gadget-kit-aux")}))
			Expect(findComponent(mod, scope.Text("gadget-kit-v1-defs")).DependsOn).Should(Equal([]string{scope.Text("gadget-kit-v1-aux")}))
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: gadgetsCRD}, crd(gadgetsCRD))).Should(Succeed())
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: scope.Text("gadget-kit-editor")}, &rbacv1.ClusterRole{})).Should(Succeed())
			for _, cm := range []string{scope.Text("gadget-kit-v1-defaults"), scope.Text("gadget-kit-v1-profile")} {
				_, err := getConfigMap(ctx, systemNS, cm)
				Expect(err).ShouldNot(HaveOccurred(), cm)
			}

			By("three traits sharing the short name labeler")
			var tds v1beta1.TraitDefinitionList
			Expect(k8sClient.List(ctx, &tds, client.MatchingLabels{veltypes.LabelDefinitionName: scope.Text("labeler")})).Should(Succeed())
			var names []string
			for _, td := range tds.Items {
				names = append(names, td.Name)
			}
			Expect(names).Should(ConsistOf(scope.Text("gadget-kit-v1-labeler"), scope.Text("widget-kit-v1-labeler"), scope.Text("widget-kit-v2-labeler")))
		})

		It("rejects a Form 1 trait that both modules ship, at admission", func() {
			results := createEachFromFile(ctx, testdataPath("apps", "consumer-ambiguous-traits.yaml"))
			Expect(results).Should(HaveLen(1))
			// The module list in the message follows the order the definitions
			// were listed in, which is not sorted, so each name is checked on
			// its own.
			for name, typeName := range map[string]string{"ambiguous-form1": scope.Text("labeler")} {
				Expect(results[name]).Should(HaveOccurred(), name)
				Expect(results[name].Error()).Should(SatisfyAll(
					ContainSubstring(`type "`+typeName+`" is ambiguous: definitions from modules [`),
					ContainSubstring(scope.Text("gadget-kit")),
					ContainSubstring(scope.Text("widget-kit")),
					ContainSubstring("] all match"),
				), name)
			}
			for name := range results {
				Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS}})).Should(BeTrue(), name)
			}
		})

		It("resolves Form 3 always and Form 1 when the short name is unique", func() {
			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-suite.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, "suite-consumer", shortWait)

			widget, err := getUnstructured(ctx, widgetGVK, testNS, "suite-widget")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(nestedString(widget, "spec", "color")).Should(Equal("green"))
			Expect(widget.GetLabels()).Should(HaveKeyWithValue(scope.Text("kit.example.com/labeled-by"), scope.Text("widget-kit-v1-labeler")))

			gadget, err := getUnstructured(ctx, gadgetGVK, testNS, "suite-gadget")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(nestedString(gadget, "spec", "mode")).Should(Equal("turbo"))
			Expect(gadget.GetLabels()).Should(HaveKeyWithValue(scope.Text("kit.example.com/labeled-by"), scope.Text("gadget-kit-v1-labeler")))
			// Only a type containing a slash is swapped for the installed name
			// (makeWorkloadWithContext, pkg/appfile/appfile.go). A Form 1 name
			// is already a valid label value and is kept as written.
			Expect(gadget.GetLabels()).Should(HaveKeyWithValue(oam.WorkloadTypeLabel, scope.Text("gadget")), "Form 1 gadget keeps its short name in the label")
		})
	})

	// --- Scenario 04 ---
	Context("type reference forms (scenario 04)", Ordered, Label("addon-module-scenario-04"), func() {
		scope := scenarioScopes["04"]
		var widgetPlatformApp string
		var addonWidgetPlatform string
		var moduleWidgetKit string
		var widgetKitDefinitions []string
		var testNS string
		widgetGVK := widgetGVK

		addonApplication := scope.addonApplication
		testdataPath := scope.Path
		uninstall := func(app, addon string, modules ...string) {
			scope.uninstall(ctx, app, addon, modules...)
		}
		BeforeAll(func() {
			widgetPlatformApp = scope.Text(fixtureWidgetPlatformApp)
			addonWidgetPlatform = scope.Text(fixtureAddonWidgetPlatform)
			moduleWidgetKit = scope.Text(fixtureModuleWidgetKit)
			widgetKitDefinitions = scope.Texts(fixtureWidgetKitDefinitions)
			testNS = scope.Namespace
			widgetGVK = scope.GVK("Widget")
			scope.prepare(ctx)
		},
		)

		BeforeAll(func() {
			Expect(k8sClient.Create(ctx, addonApplication(widgetPlatformApp, scope.Text("widget-platform"), "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
			Expect(moduleDefinitionNames(ctx, systemNS, scope.Text("widget-kit"))).Should(ConsistOf(widgetKitDefinitions))
			DeferCleanup(func() {
				for _, name := range []string{"forms-accepted", "v2-contract", "trait-outputs-form3"} {
					deleteApp(ctx, testNS, name)
				}
				uninstall(widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit)
			})
		})

		It("without addon parameters there is no addon-secret component", func() {
			Expect(componentNames(mustGetApp(ctx, systemNS, addonWidgetPlatform))).ShouldNot(ContainElement("addon-secret"))
			Expect(findService(mustGetApp(ctx, testNS, widgetPlatformApp), scope.Text("widget-platform")).Message).Should(Equal("Ready:6/6"))
		})

		It("accepts Form 3, the installed name and a unique Form 1 trait", func() {
			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-forms-accepted.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, "forms-accepted", shortWait)
			for name, want := range map[string][2]string{
				"form3-widget":   {"v1", scope.Text("widget-kit-v1-widget")},
				"stamped-widget": {"v1", scope.Text("widget-kit-v1-widget")},
			} {
				w, err := getUnstructured(ctx, widgetGVK, testNS, name)
				Expect(err).ShouldNot(HaveOccurred(), name)
				Expect(nestedString(w, "spec", "apiLine")).Should(Equal(want[0]), name)
				Expect(w.GetLabels()).Should(HaveKeyWithValue(oam.WorkloadTypeLabel, want[1]), name)
			}
			note, err := getConfigMap(ctx, testNS, "stamped-widget-note")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(note.Data).Should(HaveKeyWithValue("text", "written by a Form 1 trait"))
			// Trait outputs of a module trait carry the installed name whatever
			// form the trait was written in, so the dispatcher finds it in the
			// revision (unlike the workload label, which keeps a Form 1 name).
			Expect(note.Labels).Should(HaveKeyWithValue(oam.TraitTypeLabel, scope.Text("widget-kit-v1-note")))
		})

		It("refuses ambiguous, disabled-line and malformed references at admission", func() {
			results := createEachFromFile(ctx, testdataPath("apps", "consumer-rejected.yaml"))
			want := map[string]string{
				"reject-form1-ambiguous":           scope.Text(`type "widget" is ambiguous`),
				"reject-disabled-line":             scope.Text("widget-kit-v1beta1-widget"),
				"reject-two-segment-disabled-line": `two-segment references are not supported`,
				"reject-bad-api-version":           `"1" is not a valid API version`,
				"reject-too-many-segments":         "expected either 1 segment (name, Form 1) or 3 segments (module/v<N>/name, Form 3)",
			}
			Expect(results).Should(HaveLen(len(want)))
			for name, fragment := range want {
				Expect(results[name]).Should(HaveOccurred(), "%s must be denied", name)
				Expect(results[name].Error()).Should(ContainSubstring(fragment), name)
				Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS}})).Should(BeTrue(), name)
			}
		})

		It("holds a consumer to the v2 contract: size is required, color does not exist", func() {
			err := applyManifestFile(ctx, testdataPath("apps", "consumer-v2-contract.yaml"))
			if err != nil {
				// With EnableCueValidation the webhook already refuses it.
				Expect(err.Error()).Should(ContainSubstring("size"))
				return
			}
			waitAppStatusContains(ctx, testNS, "v2-contract", "size", shortWait)
			Expect(mustGetApp(ctx, testNS, "v2-contract").Status.Phase).ShouldNot(Equal(common.ApplicationRunning))
			Expect(isNotFound(ctx, unstructuredObj(widgetGVK, testNS, "w"))).Should(BeTrue(), "the render must stop before creating the Widget")
		})

		It("labels the output of a Form 3 trait with the installed name", func() {
			// A trait written as widget-kit/v1/note used to stamp that string
			// into trait.oam.dev/type, which is not a valid label value. Trait
			// outputs of a module trait now carry the installed name.
			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-trait-outputs-form3.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, "trait-outputs-form3", shortWait)
			cm, err := getConfigMap(ctx, testNS, "noted-widget-note")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(cm.Labels).Should(HaveKeyWithValue(oam.TraitTypeLabel, scope.Text("widget-kit-v1-note")))
		})
	})

	// --- Scenario 05 ---
	Context("_imports.cue options and defaults (scenario 05)", Ordered, Serial, Label("addon-module-scenario-05"), func() {
		scope := scenarioScopes["05"]
		var moduleWidgetKit string
		var moduleGadgetKit string
		var widgetKitTiers []string
		var testNS string
		addonApplication := scope.addonApplication
		uninstall := func(
			app,
			addon string, modules ...string) {
			scope.uninstall(ctx, app, addon, modules...)
		}
		BeforeAll(func() {
			moduleWidgetKit = scope.Text(fixtureModuleWidgetKit)
			moduleGadgetKit = scope.Text(fixtureModuleGadgetKit)
			widgetKitTiers = scope.Texts(fixtureWidgetKitTiers)
			testNS = scope.Namespace
			scope.prepare(ctx)
		})

		BeforeAll(func() {
			Expect(k8sClient.Create(ctx, addonApplication(scope.Text("import-options"), scope.Text("import-options"), "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, scope.Text("import-options"), installWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
			DeferCleanup(func() {
				uninstall(scope.Text("import-options"), scope.Text("addon-import-options"), moduleWidgetKit)
				Expect(isNotFound(ctx, configMapObj(systemNS, scope.Text("import-options-marker")))).Should(BeTrue())
			})
		})

		It("generates one component per enabled import, without a registry when none is named, with a -2 suffix on a name collision", func() {
			app := mustGetApp(ctx, systemNS, scope.Text("addon-import-options"))
			Expect(componentNames(app)).Should(Equal([]string{scope.Text("widget-kit"), scope.Text("import-options-resources"), scope.Text("widget-kit-2")}))
			Expect(findComponent(app, scope.Text("widget-kit")).Type).Should(Equal("k8s-objects"), "the template's own component keeps the name")
			mod := findComponent(app, scope.Text("widget-kit-2"))
			Expect(mod.Type).Should(Equal("module"))
			Expect(mod.DependsOn).Should(Equal([]string{scope.Text("import-options-resources")}))
			Expect(propertiesOf(mod.Properties)).Should(Equal(map[string]interface{}{"module": scope.Text("widget-kit"), "version": "1.0.0"}))

			Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: moduleGadgetKit, Namespace: systemNS}})).Should(BeTrue(),
				"enabled: false produces no component at all")
			Expect(mustGetApp(ctx, systemNS, moduleWidgetKit).Labels).Should(HaveKeyWithValue(oam.LabelAppComponent, scope.Text("widget-kit-2")))
		})

		It("does not enforce the versions filter: every enabled line installs", func() {
			Expect(componentNames(mustGetApp(ctx, systemNS, moduleWidgetKit))).Should(Equal(widgetKitTiers))
			_, err := getComponentDefinition(ctx, systemNS, scope.Text("widget-kit-v2-widget"))
			Expect(err).ShouldNot(HaveOccurred(), "versions: [v1] was requested, v2 installs anyway")
		})

		It("keeps an installed module when the default registry stops resolving, and resolves again once one is named catalog", func() {
			runVelaSucceed("module", "registry", "add", "spare-modules", moduleRegistry.cluster, "--type", "oci")
			DeferCleanup(func() {
				_, _ = runVela("module", "registry", "delete", "spare-modules")
				_, _ = runVela("module", "registry", "delete", "catalog")
			})

			By("two registries and none named catalog: the health-check render fails, nothing is removed")
			// A health check that cannot render is only logged and the service keeps
			// the health it last recorded, so the registry error is not surfaced in
			// status. That is a known limitation of module health; what must hold
			// is that a failed render garbage-collects nothing.
			Consistently(func(g Gomega) {
				_, err := getComponentDefinition(ctx, systemNS, scope.Text("widget-kit-v1-widget"))
				g.Expect(err).ShouldNot(HaveOccurred(), "a failed render does not garbage-collect")
				g.Expect(getAppG(g, ctx, systemNS, moduleWidgetKit).Name).Should(Equal(moduleWidgetKit))
			}, 30*time.Second, 5*time.Second).Should(Succeed())

			By("a registry named catalog makes the empty registry resolve again")
			runVelaSucceed("module", "registry", "add", "catalog", moduleRegistry.cluster, "--type", "oci")
			Eventually(func(g Gomega) {
				svc := findService(getAppG(g, ctx, systemNS, scope.Text("addon-import-options")), scope.Text("widget-kit-2"))
				g.Expect(svc).ShouldNot(BeNil())
				g.Expect(svc.Healthy).Should(BeTrue(), svc.Message)
				g.Expect(svc.Message).Should(Equal("Ready:5/5"))
			}, reconcileWait, pollInterval).Should(Succeed())
		})
	})

	// --- Scenario 06 ---
	Context("a type: module component declared in template.cue with a tenant namespace (scenario 06)", Ordered, Label("addon-module-scenario-06"), func() {
		scope := scenarioScopes["06"]
		var moduleWidgetKit string
		var widgetsCRD string
		var widgetKitDefinitions []string
		var testNS string
		widgetGVK := widgetGVK
		widgetClassGVK := widgetClassGVK
		addonApplication := scope.addonApplication
		testdataPath := scope.Path
		uninstall := func(app, addon string, modules ...string) {
			scope.uninstall(ctx, app, addon, modules...)
		}
		BeforeAll(func() {
			moduleWidgetKit = scope.Text(fixtureModuleWidgetKit)
			widgetsCRD =
				scope.Text(fixtureWidgetsCRD)
			widgetKitDefinitions = scope.Texts(fixtureWidgetKitDefinitions)
			testNS =
				scope.Namespace
			widgetGVK = scope.GVK("Widget")
			widgetClassGVK = scope.GVK("WidgetClass")
			scope.prepare(ctx)
		})

		BeforeAll(func() {
			Expect(k8sClient.Create(ctx, addonApplication(scope.Text("tenant-widgets"), scope.Text("tenant-widgets"), "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, scope.Text("tenant-widgets"), installWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
			DeferCleanup(func() {
				deleteApp(ctx, scope.Text("kit-tenant"), "tenant-consumer")
				waitGone(ctx, unstructuredObj(widgetGVK, scope.Text("kit-tenant"), "tenant-widget"), shortWait)
				uninstall(scope.Text("tenant-widgets"), scope.Text("addon-tenant-widgets"), moduleWidgetKit)
				waitGone(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: scope.Text("kit-tenant")}}, reconcileWait)
			})
		})

		It("lets the template's module component win over the matching _imports.cue entry", func() {
			app := mustGetApp(ctx, systemNS, scope.Text("addon-tenant-widgets"))
			Expect(componentNames(app)).Should(ConsistOf("tenant-kit", scope.Text("tenant-widgets-resources")))
			Expect(componentNames(app)).ShouldNot(ContainElement(scope.Text("widget-kit")))
			mod := findComponent(app, "tenant-kit")
			Expect(mod.Type).Should(Equal("module"))
			Expect(mod.DependsOn).Should(Equal([]string{scope.Text("tenant-widgets-resources")}), "a hand-written module component gets no automatic dependsOn")
			Expect(propertiesOf(mod.Properties)).Should(Equal(map[string]interface{}{
				"module": scope.Text("widget-kit"), "namespace": scope.Text("kit-tenant"), "registry": moduleRegistryName, "version": "1.0.0",
			}))
			Expect(mustGetApp(ctx, systemNS, moduleWidgetKit).Labels).Should(HaveKeyWithValue(oam.LabelAppComponent, "tenant-kit"))
		})

		It("installs definitions and namespaced auxiliary objects into the tenant namespace only", func() {
			Expect(moduleDefinitionNames(ctx, scope.Text("kit-tenant"), scope.Text("widget-kit"))).Should(ConsistOf(widgetKitDefinitions))
			Expect(moduleDefinitionNames(ctx, systemNS, scope.Text("widget-kit"))).Should(BeEmpty())
			for _, cm := range []string{scope.Text("widget-kit-module-info"), scope.Text("widget-kit-v1-line-config"), scope.Text("widget-kit-v2-line-config")} {
				_, err := getConfigMap(ctx, scope.Text("kit-tenant"), cm)
				Expect(err).ShouldNot(HaveOccurred(), cm)
				Expect(isNotFound(ctx, configMapObj(systemNS, cm))).Should(BeTrue(), cm)
			}
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: widgetsCRD}, crd(widgetsCRD))).Should(Succeed(), "cluster-scoped objects are unaffected")
			_, err := getUnstructured(ctx, widgetClassGVK, "", scope.Text("widget-kit-v1-standard"))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: scope.Text("widget-kit-viewer")}, &rbacv1.ClusterRole{})).Should(Succeed())
		})

		It("is usable from the tenant namespace and refused from another", func() {
			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-tenant.yaml"))).Should(Succeed())
			waitAppRunning(ctx, scope.Text("kit-tenant"), "tenant-consumer", shortWait)
			w, err := getUnstructured(ctx, widgetGVK, scope.Text("kit-tenant"), "tenant-widget")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(nestedString(w, "spec", "classRef")).Should(Equal(scope.Text("widget-kit-v1-standard")))
			Expect(w.GetLabels()).Should(HaveKeyWithValue(scope.Text("kit.example.com/labeled-by"), scope.Text("widget-kit-v1-labeler")))

			results := createEachFromFile(ctx, testdataPath("apps", "consumer-default-tenant.yaml"))
			Expect(results).Should(HaveLen(1))
			// The friendly "ensure the module is installed" text comes from the
			// definition permission check, which only runs with the alpha
			// ValidateDefinitionPermissions gate. Without it the render refuses
			// the Application with the template loader's error, so only the
			// installed name and "not found" are checked.
			for name, err := range results {
				Expect(err).Should(HaveOccurred(), "%s must be denied from the default namespace", name)
				Expect(err.Error()).Should(SatisfyAll(
					ContainSubstring(scope.Text(`"widget-kit-v1-widget"`)),
					ContainSubstring("not found"),
				), name)
				Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS}})).Should(BeTrue(), name)
			}
		})
	})
})

// --- Scenario 08 (before 07: it needs 1.0.0 to be the highest tag) ---
var _ = Describe("Addons that import modules", Ordered, func() {
	ctx := context.Background()
	BeforeAll(func() {
		scenarioScopes["08"].prepare(ctx)
	})

	Context("latest vs pinned module version (scenario 08)", Label("addon-module-scenario-08"), func() {
		scope := scenarioScopes["08"]
		var moduleWidgetKit string
		var widgetKitTiers []string
		var testNS string
		addonApplication := scope.addonApplication
		publishModuleFixture := scope.publishModuleFixture

		uninstall := func(app, addon string, modules ...string) {
			scope.uninstall(ctx, app, addon, modules...)
		}
		BeforeAll(func() {
			moduleWidgetKit = scope.Text(fixtureModuleWidgetKit)
			widgetKitTiers = scope.Texts(fixtureWidgetKitTiers)
			testNS = scope.Namespace
		})

		// "" while module-widget-kit does not exist, so Eventually and
		// Consistently keep polling instead of aborting on a NotFound.
		moduleVersion := func() string {
			app, err := getApp(ctx, systemNS, moduleWidgetKit)
			if err != nil {
				return ""
			}
			return app.Annotations[veltypes.AnnoDefinitionModuleVersion]
		}

		BeforeAll(func() {
			Expect(ociTags(moduleRegistry.hostBase, scope.Text("modules/widget-kit"))).Should(ConsistOf("1.0.0"), scope.Text("widget-kit 1.0.0 must be the only published tag here"))
			Expect(k8sClient.Create(ctx, addonApplication(scope.Text("widget-latest"), scope.Text("widget-latest"), "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, scope.Text("widget-latest"), installWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
			DeferCleanup(func() {
				annotateApp(ctx, systemNS, scope.Text("addon-widget-latest"), oam.AnnotationWorkflowRestart, "")
				uninstall(scope.Text("widget-latest"), scope.Text("addon-widget-latest"), moduleWidgetKit)
			})
		})

		It("resolves an unpinned import to the highest tag at install", func() {
			app := mustGetApp(ctx, systemNS, scope.Text("addon-widget-latest"))
			Expect(propertiesOf(findComponent(app, scope.Text("widget-kit")).Properties)).Should(Equal(map[string]interface{}{"module": scope.Text("widget-kit"), "registry": moduleRegistryName}),
				"no version property when _imports.cue names none")
			cd, err := getComponentDefinition(ctx, systemNS, "module")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(cd.Annotations).Should(HaveKeyWithValue(veltypes.AnnoDefinitionRedispatchOnWorkflowRun, "true"),
				"the module definition asks for its components to be re-applied on every workflow run, which is what lets a restart deliver a new tag")
			Expect(moduleVersion()).Should(Equal("1.0.0"))
			Expect(componentNames(mustGetApp(ctx, systemNS, moduleWidgetKit))).Should(Equal(widgetKitTiers))
		})

		It("does not move when a newer tag is published: renders after a succeeded workflow are health checks only", func() {
			publishModuleFixture("widget-kit-1.1.0")
			Expect(ociTags(moduleRegistry.hostBase, scope.Text("modules/widget-kit"))).Should(ConsistOf("1.0.0", "1.1.0"))
			Consistently(func(g Gomega) {
				g.Expect(moduleVersion()).Should(Equal("1.0.0"))
				g.Expect(isNotFound(ctx, componentDefinitionObj(systemNS, scope.Text("widget-kit-v1-gauge")))).Should(BeTrue())
				for _, svc := range mustGetApp(ctx, systemNS, scope.Text("addon-widget-latest")).Status.Services {
					g.Expect(svc.Healthy).Should(BeTrue(), "%s: %s", svc.Name, svc.Message)
				}
			}, 80*time.Second, 5*time.Second).Should(Succeed(), "one full resync (reSyncPeriod=1m) must not dispatch the new version")
		})

		It("does not move when the user Application's workflow re-runs: the addon render is unchanged", func() {
			restartWorkflow(ctx, testNS, scope.Text("widget-latest"), "true")
			Eventually(func(g Gomega) string {
				return getAppG(g, ctx, testNS, scope.Text("widget-latest")).Annotations[oam.AnnotationWorkflowRestart]
			}, shortWait, pollInterval).Should(BeEmpty(), "a one-shot restart annotation is removed once used")
			waitAppRunning(ctx, testNS, scope.Text("widget-latest"), shortWait)
			Consistently(moduleVersion, 45*time.Second, 5*time.Second).Should(Equal("1.0.0"))
		})

		It("moves when the Application that renders the module component re-runs its workflow", func() {
			restartWorkflow(ctx, systemNS, scope.Text("addon-widget-latest"), "true")
			Eventually(moduleVersion, reconcileWait, pollInterval).Should(Equal("1.1.0"))
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
			Eventually(func() error {
				_, err := getComponentDefinition(ctx, systemNS, scope.Text("widget-kit-v1-gauge"))
				return err
			}, shortWait, pollInterval).Should(Succeed())
			Expect(mustGetApp(ctx, systemNS, scope.Text("addon-widget-latest")).Annotations).ShouldNot(HaveKey(oam.AnnotationWorkflowRestart))
		})

		It("follows the registry with a recurring restart, including a publish that switches a line off", func() {
			restartWorkflow(ctx, systemNS, scope.Text("addon-widget-latest"), "2m")
			publishModuleFixture("widget-kit-1.2.0")
			Eventually(moduleVersion, 6*time.Minute, 10*time.Second).Should(Equal("1.2.0"))
			Eventually(func(g Gomega) []string {
				return componentNames(getAppG(g, ctx, systemNS, moduleWidgetKit))
			}, shortWait, pollInterval).Should(Equal([]string{scope.Text("widget-kit-aux"), scope.Text("widget-kit-v1-aux"), scope.Text("widget-kit-v1-defs")}))
			waitGone(ctx, componentDefinitionObj(systemNS, scope.Text("widget-kit-v2-widget")), reconcileWait)
			Expect(mustGetApp(ctx, systemNS, scope.Text("addon-widget-latest")).Annotations).Should(HaveKeyWithValue(oam.AnnotationWorkflowRestart, "2m"), "a duration is recurring and stays")
		})
	})

	// --- Scenario 07 ---
	Context("upgrade the addon and so the module, add a definition, remove an API line, roll back (scenario 07)", Label("addon-module-scenario-07"), func() {
		scope := scenarioScopes["08"]
		var widgetPlatformApp string
		var addonWidgetPlatform string
		var moduleWidgetKit string
		var widgetKitTiers []string
		var widgetKitDefinitions []string
		var testNS string

		widgetGVK := widgetGVK
		widgetClassGVK :=

			widgetClassGVK
		addonApplication := scope.addonApplication
		setAddonVersion := scope.setAddonVersion
		expectConsumerOfRemovedDefinition := scope.expectConsumerOfRemovedDefinition
		testdataPath := scope.Path
		uninstall := func(app, addon string, modules ...string) {
			scope.uninstall(ctx, app, addon, modules...)
		}
		BeforeAll(func() {
			widgetPlatformApp = scope.Text(fixtureWidgetPlatformApp)
			addonWidgetPlatform = scope.Text(fixtureAddonWidgetPlatform)
			moduleWidgetKit = scope.Text(fixtureModuleWidgetKit)
			widgetKitTiers = scope.Texts(fixtureWidgetKitTiers)
			widgetKitDefinitions = scope.Texts(fixtureWidgetKitDefinitions)
			testNS = scope.Namespace
			widgetGVK = scope.GVK("Widget")
			widgetClassGVK = scope.GVK("WidgetClass")
		})

		moduleState := func(g Gomega, version string, tiers []string) {
			mod := mustGetApp(ctx, systemNS, moduleWidgetKit)
			g.Expect(mod.Annotations).Should(HaveKeyWithValue(veltypes.AnnoDefinitionModuleVersion, version))
			g.Expect(componentNames(mod)).Should(Equal(tiers))
		}

		BeforeAll(func() {
			Expect(ociTags(moduleRegistry.hostBase, scope.Text("modules/widget-kit"))).Should(ConsistOf("1.0.0", "1.1.0", "1.2.0"))
			Expect(k8sClient.Create(ctx, addonApplication(widgetPlatformApp, scope.Text("widget-platform"), "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-v2.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, "v2-consumer", shortWait)
			DeferCleanup(func() {
				deleteApp(ctx, testNS, "gauge-consumer")
				deleteApp(ctx, testNS, "v2-consumer")
				waitAppGone(ctx, testNS, "gauge-consumer", reconcileWait)
				waitAppGone(ctx, testNS, "v2-consumer", reconcileWait)
				uninstall(widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit)
			})
		})

		It("starts from 1.0.0 with both lines", func() {
			Eventually(func(g Gomega) { moduleState(g, "1.0.0", widgetKitTiers) }, shortWait, pollInterval).Should(Succeed())
			Expect(moduleDefinitionNames(ctx, systemNS, scope.Text("widget-kit"))).Should(ConsistOf(widgetKitDefinitions))
		})

		It("1.1.0 adds a definition and updates auxiliary objects in place", func() {
			setAddonVersion(ctx, widgetPlatformApp, "1.1.0")
			Eventually(func(g Gomega) { moduleState(g, "1.1.0", widgetKitTiers) }, installWait, pollInterval).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
			Expect(mustGetApp(ctx, systemNS, addonWidgetPlatform).Labels).Should(HaveKeyWithValue(oam.LabelAddonVersion, "1.1.0"))
			Expect(moduleDefinitionNames(ctx, systemNS, scope.Text("widget-kit"))).Should(ConsistOf(append([]string{scope.Text("widget-kit-v1-gauge")}, widgetKitDefinitions...)))
			info, err := getConfigMap(ctx, systemNS, scope.Text("widget-kit-module-info"))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(info.Data).Should(HaveKeyWithValue("moduleVersion", "1.1.0"))
			line, err := getConfigMap(ctx, systemNS, scope.Text("widget-kit-v1-line-config"))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(line.Data).Should(HaveKeyWithValue("configRevision", "2"))

			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-gauge.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, "gauge-consumer", shortWait)
			gauge, err := getConfigMap(ctx, testNS, "fuel-gauge")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(gauge.Data).Should(Equal(map[string]string{"line": "v1", "module": scope.Text("widget-kit"), "reading": "80", "since": "1.1.0"}))
		})

		It("1.2.0 switches the v2 line off: its tiers, definitions and auxiliary objects are garbage-collected, a consumer's Widget is not", func() {
			setAddonVersion(ctx, widgetPlatformApp, "1.2.0")
			Eventually(func(g Gomega) {
				moduleState(g, "1.2.0", []string{scope.Text("widget-kit-aux"), scope.Text("widget-kit-v1-aux"), scope.Text("widget-kit-v1-defs")})
			}, installWait, pollInterval).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
			waitGone(ctx, componentDefinitionObj(systemNS, scope.Text("widget-kit-v2-widget")), reconcileWait)
			waitGone(ctx, traitDefinitionObj(systemNS, scope.Text("widget-kit-v2-labeler")), shortWait)
			waitGone(ctx, unstructuredObj(widgetClassGVK, "", scope.Text("widget-kit-v2-premium")), shortWait)
			waitGone(ctx, configMapObj(systemNS, scope.Text("widget-kit-v2-line-config")), shortWait)
			info, err := getConfigMap(ctx, systemNS, scope.Text("widget-kit-module-info"))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(info.Data).Should(HaveKeyWithValue("servedLines", "v1"))
			Expect(info.Data).Should(HaveKeyWithValue("disabledLines", "v1beta1,v2"))

			_, err = getUnstructured(ctx, widgetGVK, testNS, "premium-widget")
			Expect(err).ShouldNot(HaveOccurred(), "nothing deletes a consumer's already-applied objects")

			By("the v2 consumer cannot be updated, and fails to parse on its next reconcile")
			expectConsumerOfRemovedDefinition(ctx, "v2-consumer", scope.Text("widget-kit-v2-widget"), "premium-widget", "after-1.2.0")
		})

		It("rolling back to 1.0.0 brings v2 back and drops the gauge", func() {
			setAddonVersion(ctx, widgetPlatformApp, "1.0.0")
			Eventually(func(g Gomega) { moduleState(g, "1.0.0", widgetKitTiers) }, installWait, pollInterval).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
			Eventually(func() []string { return moduleDefinitionNames(ctx, systemNS, scope.Text("widget-kit")) }, reconcileWait, pollInterval).Should(ConsistOf(widgetKitDefinitions))

			bumpPublishVersion(ctx, testNS, "v2-consumer", "after-rollback")
			waitAppRunning(ctx, testNS, "v2-consumer", reconcileWait)

			// widget-kit-v1-gauge only exists from 1.1.0, so the gauge consumer
			// is now in the position the v2 consumer was in on 1.2.0.
			expectConsumerOfRemovedDefinition(ctx, "gauge-consumer", scope.Text("widget-kit-v1-gauge"), "", "after-rollback")
			_, err := getConfigMap(ctx, testNS, "fuel-gauge")
			Expect(err).ShouldNot(HaveOccurred(), "nothing deletes a consumer's already-applied objects")
		})
	})
})

// --- Scenario 09 ---
var _ = Describe("Addons that import modules", func() {
	ctx := context.Background()
	crd := func(name string) *apiextensionsv1.CustomResourceDefinition {
		return &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: name}}
	}

	Context("re-pushing the same tag: revision cache vs pinned render cache (scenario 09)", Ordered, Serial, Label("addon-module-scenario-09"), func() {
		scope := scenarioScopes["09"]
		var moduleProbeKit string
		var testNS string
		addonApplication := scope.addonApplication
		publishModuleFixture := scope.publishModuleFixture
		pushAddonFixture := scope.pushAddonFixture
		uninstall := func(app, addon string, modules ...string) {
			scope.uninstall(ctx, app, addon, modules...)
		}
		BeforeAll(func() {
			moduleProbeKit = scope.Text(fixtureModuleProbeKit)
			testNS = scope.Namespace
			scope.prepare(ctx)
		})

		type builds struct{ addon, module, line, def string }
		read := func() builds {
			var b builds
			if cm, err := getConfigMap(ctx, systemNS, scope.Text("cache-probe-build")); err == nil {
				b.addon = cm.Data["build"]
			}
			if cm, err := getConfigMap(ctx, systemNS, scope.Text("probe-kit-build")); err == nil {
				b.module = cm.Data["build"]
			}
			if cm, err := getConfigMap(ctx, systemNS, scope.Text("probe-kit-v1-line")); err == nil {
				b.line = cm.Data["build"]
			}
			if cd, err := getComponentDefinition(ctx, systemNS, scope.Text("probe-kit-v1-probe")); err == nil {
				b.def = cd.Annotations[veltypes.AnnoDefinitionDescription]
			}
			return b
		}
		buildA := builds{addon: "a", module: "a", line: "a", def: scope.Text("probe-kit v1 probe (build a).")}

		BeforeAll(func() {
			By("starting from an empty addon render cache")
			restartVelaCore(ctx)
			Expect(k8sClient.Create(ctx, addonApplication(scope.Text("cache-probe"), scope.Text("cache-probe"), "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, scope.Text("cache-probe"), installWait)
			waitAppRunning(ctx, systemNS, moduleProbeKit, shortWait)
			DeferCleanup(func() {
				uninstall(scope.Text("cache-probe"), scope.Text("addon-cache-probe"), moduleProbeKit)
				Expect(isNotFound(ctx, configMapObj(systemNS, scope.Text("cache-probe-build")))).Should(BeTrue())
				Expect(isNotFound(ctx, configMapObj(systemNS, scope.Text("probe-kit-build")))).Should(BeTrue())
			})
		})

		It("installs build a of both", func() {
			Eventually(read, shortWait, pollInterval).Should(Equal(buildA))
			cd, err := getComponentDefinition(ctx, systemNS, "addon")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(cd.Annotations).Should(HaveKeyWithValue(veltypes.AnnoDefinitionRedispatchOnWorkflowRun, "true"),
				"the addon definition asks for its components to be re-applied on every workflow run, which is what lets a restart deliver a re-pushed addon")
		})

		It("changes nothing on the cluster when both tags are re-pushed and the workflows do not run", func() {
			publishModuleFixture("probe-kit-1.0.0-b")
			pushAddonFixture("cache-probe-1.0.0-b")
			Consistently(read, 130*time.Second, 10*time.Second).Should(Equal(buildA), "two resyncs render for health only")
		})

		It("picks up the module's new digest once the addon Application's workflow re-runs, while the pinned addon stays cached", func() {
			restartWorkflow(ctx, systemNS, scope.Text("addon-cache-probe"), "true")
			Eventually(read, reconcileWait, pollInterval).Should(Equal(builds{addon: "a", module: "b", line: "b", def: scope.Text("probe-kit v1 probe (build b).")}))
			waitAppRunning(ctx, systemNS, moduleProbeKit, shortWait)

			restartWorkflow(ctx, testNS, scope.Text("cache-probe"), "true")
			Eventually(func(g Gomega) string {
				return getAppG(g, ctx, testNS, scope.Text("cache-probe")).Annotations[oam.AnnotationWorkflowRestart]
			}, shortWait, pollInterval).Should(BeEmpty())
			waitAppRunning(ctx, testNS, scope.Text("cache-probe"), shortWait)
			Consistently(func() string { return read().addon }, 45*time.Second, 5*time.Second).Should(Equal("a"), "the pinned addon render comes from vela-core's in-memory cache")
		})

		It("delivers the re-pushed addon only after a controller restart and a re-run of the user Application's workflow", func() {
			restartVelaCore(ctx)
			Expect(read().addon).Should(Equal("a"), "a restart alone dispatches nothing")
			restartWorkflow(ctx, testNS, scope.Text("cache-probe"), "true")
			Eventually(func() string { return read().addon }, reconcileWait, pollInterval).Should(Equal("b"))
			waitAppRunning(ctx, testNS, scope.Text("cache-probe"), shortWait)
			waitAppRunning(ctx, systemNS, scope.Text("addon-cache-probe"), shortWait)
		})
	})

	// --- Scenario 10 ---
	Context("removing a module from an addon deletes its CRDs and every custom resource of them (scenario 10)", Ordered, Label("addon-module-scenario-10"), func() {
		scope := scenarioScopes["10"]
		var moduleWidgetKit string
		var moduleGadgetKit string
		var gadgetsCRD string
		var testNS string
		widgetGVK := widgetGVK
		gadgetGVK := gadgetGVK
		addonApplication := scope.addonApplication
		setAddonVersion := scope.setAddonVersion
		expectConsumerOfRemovedDefinition := scope.expectConsumerOfRemovedDefinition
		testdataPath := scope.Path
		uninstall :=
			func(app, addon string, modules ...string) {
				scope.uninstall(ctx, app, addon,
					modules...)
			}
		BeforeAll(func() {
			moduleWidgetKit = scope.Text(fixtureModuleWidgetKit)
			moduleGadgetKit = scope.Text(fixtureModuleGadgetKit)
			gadgetsCRD = scope.Text(fixtureGadgetsCRD)
			testNS = scope.Namespace
			widgetGVK = scope.GVK("Widget")
			gadgetGVK = scope.GVK("Gadget")
			scope.prepare(ctx)
		})

		BeforeAll(func() {
			Expect(k8sClient.Create(ctx, addonApplication(scope.Text("kit-suite"), scope.Text("kit-suite"), "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, scope.Text("kit-suite"), installWait)
			waitAppRunning(ctx, systemNS, moduleGadgetKit, shortWait)
			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-suite-keep-doomed.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, "suite-consumer", shortWait)
			DeferCleanup(func() {
				deleteApp(ctx, testNS, "suite-consumer")
				waitAppGone(ctx, testNS, "suite-consumer", reconcileWait)
				uninstall(scope.Text("kit-suite"), scope.Text("addon-kit-suite"), moduleWidgetKit, moduleGadgetKit)
			})
		})

		It("has one custom resource of each module before the upgrade", func() {
			_, err := getUnstructured(ctx, widgetGVK, testNS, "keep-widget")
			Expect(err).ShouldNot(HaveOccurred())
			_, err = getUnstructured(ctx, gadgetGVK, testNS, "doomed-gadget")
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("2.0.0 drops gadget-kit: module Application, definitions, auxiliary objects, CRD and Gadgets go; widget-kit is untouched", func() {
			setAddonVersion(ctx, scope.Text("kit-suite"), "2.0.0")
			waitAppGone(ctx, systemNS, moduleGadgetKit, installWait)
			waitAppRunning(ctx, testNS, scope.Text("kit-suite"), installWait)
			Expect(componentNames(mustGetApp(ctx, systemNS, scope.Text("addon-kit-suite")))).Should(Equal([]string{scope.Text("kit-suite-resources"), scope.Text("widget-kit")}))
			waitGone(ctx, crd(gadgetsCRD), shortWait)
			Expect(isNotFound(ctx, componentDefinitionObj(systemNS, scope.Text("gadget-kit-v1-gadget")))).Should(BeTrue())
			Expect(isNotFound(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: scope.Text("gadget-kit-editor")}})).Should(BeTrue())
			Expect(isNotFound(ctx, configMapObj(systemNS, scope.Text("gadget-kit-v1-defaults")))).Should(BeTrue())
			_, err := getUnstructured(ctx, widgetGVK, testNS, "keep-widget")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(mustGetApp(ctx, systemNS, moduleWidgetKit).Status.Phase).Should(Equal(common.ApplicationRunning))
		})

		It("leaves the consumer broken: its Gadget is gone with the kind and a new render finds no definition", func() {
			// Deleting the CRD deleted every Gadget, so unlike scenario 07 the
			// consumer's applied object of the removed module is gone too.
			expectConsumerOfRemovedDefinition(ctx, "suite-consumer", scope.Text("gadget-kit-v1-gadget"), "", "after-removal")
			Expect(isNotFound(ctx, crd(gadgetsCRD))).Should(BeTrue(), "the Gadget kind no longer exists")
			_, err := getUnstructured(ctx, widgetGVK, testNS, "keep-widget")
			Expect(err).ShouldNot(HaveOccurred(), "the other module's object is untouched")
		})

		It("downgrading to 1.0.0 brings the module, the CRD and, on the consumer's next run, the Gadget back", func() {
			setAddonVersion(ctx, scope.Text("kit-suite"), "1.0.0")
			waitAppRunning(ctx, systemNS, moduleGadgetKit, installWait)
			bumpPublishVersion(ctx, testNS, "suite-consumer", "after-restore")
			waitAppRunning(ctx, testNS, "suite-consumer", reconcileWait)
			_, err := getUnstructured(ctx, gadgetGVK, testNS, "doomed-gadget")
			Expect(err).ShouldNot(HaveOccurred(), "a new Gadget; the old one was deleted with the CRD")
		})
	})

	// --- Scenario 11 ---
	Context("deleting things by hand at every level: the Application one level up restores them (scenario 11)", Ordered, Label("addon-module-scenario-11"), func() {
		scope := scenarioScopes["11"]
		var widgetPlatformApp string
		var addonWidgetPlatform string
		var moduleWidgetKit string
		var widgetsCRD string
		var testNS string
		widgetGVK := widgetGVK
		widgetClassGVK := widgetClassGVK
		addonApplication := scope.addonApplication
		testdataPath := scope.Path
		uninstall := func(app,
			addon string, modules ...string) {
			scope.uninstall(ctx, app, addon,
				modules...)
		}
		BeforeAll(func() {
			widgetPlatformApp = scope.Text(fixtureWidgetPlatformApp)
			addonWidgetPlatform = scope.Text(fixtureAddonWidgetPlatform)
			moduleWidgetKit = scope.Text(fixtureModuleWidgetKit)
			widgetsCRD = scope.Text(fixtureWidgetsCRD)
			testNS = scope.Namespace
			widgetGVK = scope.GVK("Widget")
			widgetClassGVK = scope.GVK("WidgetClass")
			scope.prepare(ctx)
		})

		uidOf := func(obj client.Object) k8stypes.UID {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).Should(Succeed())
			return obj.GetUID()
		}
		waitBack := func(obj client.Object, timeout time.Duration) {
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
			}, timeout, pollInterval).Should(Succeed(), "%T %s did not come back", obj, client.ObjectKeyFromObject(obj))
		}

		BeforeAll(func() {
			Expect(k8sClient.Create(ctx, addonApplication(widgetPlatformApp, scope.Text("widget-platform"), "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-blue-widget.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, "widget-consumer", shortWait)
			DeferCleanup(func() {
				deleteApp(ctx, testNS, "widget-consumer")
				waitAppGone(ctx, testNS, "widget-consumer", reconcileWait)
				uninstall(widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit)
			})
		})

		It("recreates a deleted module definition with a new UID", func() {
			// module-widget-kit inherits addons.oam.dev/name from the addon
			// Application, which on its own would make the resource keeper treat
			// it as apply-once and skip state keep; the module renderer declares
			// apply-once off to prevent that.
			mod := mustGetApp(ctx, systemNS, moduleWidgetKit)
			Expect(mod.Labels).Should(HaveKey(oam.LabelAddonName))
			Expect(policyNames(mod)).Should(ContainElement("module-state-keep"))

			td := traitDefinitionObj(systemNS, scope.Text("widget-kit-v1-labeler"))
			before := uidOf(td)
			Expect(k8sClient.Delete(ctx, td)).Should(Succeed())
			waitBack(traitDefinitionObj(systemNS, scope.Text("widget-kit-v1-labeler")), reconcileWait)
			Expect(uidOf(traitDefinitionObj(systemNS, scope.Text("widget-kit-v1-labeler")))).ShouldNot(Equal(before))
		})

		It("reverts an edited line auxiliary ConfigMap", func() {
			cm, err := getConfigMap(ctx, systemNS, scope.Text("widget-kit-v1-line-config"))
			Expect(err).ShouldNot(HaveOccurred())
			cm.Data["configRevision"] = "tampered"
			Expect(k8sClient.Update(ctx, cm)).Should(Succeed())
			Eventually(func() string {
				got, err := getConfigMap(ctx, systemNS, scope.Text("widget-kit-v1-line-config"))
				if err != nil {
					return ""
				}
				return got.Data["configRevision"]
			}, reconcileWait, pollInterval).Should(Equal("1"))
		})

		It("recreates a deleted line auxiliary custom resource", func() {
			class := unstructuredObj(widgetClassGVK, "", scope.Text("widget-kit-v1-standard"))
			Expect(k8sClient.Delete(ctx, class)).Should(Succeed())
			waitBack(unstructuredObj(widgetClassGVK, "", scope.Text("widget-kit-v1-standard")), reconcileWait)
		})

		It("recreates a deleted module CRD, and the consumer's Widget comes back on its own next state keep", func() {
			Expect(k8sClient.Delete(ctx, crd(widgetsCRD))).Should(Succeed())
			waitBack(crd(widgetsCRD), reconcileWait)
			waitBack(unstructuredObj(widgetGVK, testNS, "blue-widget"), 5*time.Minute)
		})

		It("recreates a deleted addon-level definition", func() {
			Expect(k8sClient.Delete(ctx, traitDefinitionObj(systemNS, scope.Text("platform-owner")))).Should(Succeed())
			waitBack(traitDefinitionObj(systemNS, scope.Text("platform-owner")), reconcileWait)
		})

		It("recreates a deleted module Application, whose finalizer first removed everything it applied", func() {
			before := uidOf(&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: moduleWidgetKit, Namespace: systemNS}})
			deleteApp(ctx, systemNS, moduleWidgetKit)
			waitAppGone(ctx, systemNS, moduleWidgetKit, reconcileWait)
			Expect(isNotFound(ctx, crd(widgetsCRD))).Should(BeTrue(), "the finalizer removed the CRD")
			Expect(isNotFound(ctx, componentDefinitionObj(systemNS, scope.Text("widget-kit-v1-widget")))).Should(BeTrue())
			waitBack(&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: moduleWidgetKit, Namespace: systemNS}}, reconcileWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, installWait)
			Expect(uidOf(&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: moduleWidgetKit, Namespace: systemNS}})).ShouldNot(Equal(before))
			waitBack(unstructuredObj(widgetGVK, testNS, "blue-widget"), 5*time.Minute)
		})

		It("recreates a deleted addon Application, cascading through the module Application", func() {
			deleteApp(ctx, systemNS, addonWidgetPlatform)
			waitAppGone(ctx, systemNS, addonWidgetPlatform, reconcileWait)
			waitAppGone(ctx, systemNS, moduleWidgetKit, reconcileWait)
			waitBack(&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: addonWidgetPlatform, Namespace: systemNS}}, 5*time.Minute)
			waitAppRunning(ctx, systemNS, addonWidgetPlatform, installWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, installWait)
			waitAppRunning(ctx, testNS, widgetPlatformApp, reconcileWait)
		})
	})

	// --- Scenario 12 ---
	Context("deleting the addon while something still uses it (scenario 12)", Ordered, Label("addon-module-scenario-12"), func() {
		scope := scenarioScopes["12"]
		var widgetPlatformApp string
		var addonWidgetPlatform string
		var moduleWidgetKit string
		var widgetsCRD string
		var testNS string
		widgetGVK := widgetGVK
		addonApplication := scope.addonApplication
		expectConsumerOfRemovedDefinition := scope.expectConsumerOfRemovedDefinition
		testdataPath := scope.Path
		uninstall := func(app, addon string, modules ...string) {
			scope.uninstall(ctx, app, addon, modules...)
		}
		BeforeAll(func() {
			widgetPlatformApp = scope.Text(fixtureWidgetPlatformApp)
			addonWidgetPlatform = scope.Text(fixtureAddonWidgetPlatform)
			moduleWidgetKit = scope.Text(fixtureModuleWidgetKit)
			widgetsCRD = scope.Text(fixtureWidgetsCRD)
			testNS = scope.Namespace
			widgetGVK = scope.GVK("Widget")
			scope.prepare(ctx)
		})

		install := func() {
			Expect(k8sClient.Create(ctx, addonApplication(widgetPlatformApp, scope.Text("widget-platform"), "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
		}

		BeforeAll(func() {
			install()
			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-blue-widget.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, "widget-consumer", shortWait)
			_, err := getConfigMap(ctx, testNS, "blue-widget-card")
			Expect(err).ShouldNot(HaveOccurred())
			DeferCleanup(func() {
				deleteApp(ctx, testNS, "widget-consumer")
				waitAppGone(ctx, testNS, "widget-consumer", reconcileWait)
				if !isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: widgetPlatformApp, Namespace: testNS}}) {
					uninstall(widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit)
				}
				waitGone(ctx, crd(widgetsCRD), reconcileWait)
			})
		})

		It("uninstalls with no in-use guard: the chain and the CRD go, the consumer keeps its built-in objects and turns unhealthy", func() {
			uninstall(widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit)
			_, err := getConfigMap(ctx, testNS, "blue-widget-card")
			Expect(err).ShouldNot(HaveOccurred(), "a built-in kind survives; only the Widget went with its CRD")

			Expect(isNotFound(ctx, crd(widgetsCRD))).Should(BeTrue(), "the Widget kind went with the module")

			// The Widget went with its CRD, so only the refusal and the parse
			// failure are left to check.
			expectConsumerOfRemovedDefinition(ctx, "widget-consumer", scope.Text("widget-kit-v1-widget"), "", "after-uninstall")
			_, err = getConfigMap(ctx, testNS, "blue-widget-card")
			Expect(err).ShouldNot(HaveOccurred(), "the built-in object is still there while the consumer fails to parse")
		})

		It("re-installing heals the consumer", func() {
			install()
			bumpPublishVersion(ctx, testNS, "widget-consumer", "after-reinstall")
			waitAppRunning(ctx, testNS, "widget-consumer", reconcileWait)
			w, err := getUnstructured(ctx, widgetGVK, testNS, "blue-widget")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(w.GetLabels()).Should(HaveKeyWithValue(scope.Text("kit.example.com/tier"), "gold"))
		})

		It("deletes a consumer cleanly after its definitions are gone", func() {
			uninstall(widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit)
			deleteApp(ctx, testNS, "widget-consumer")
			waitAppGone(ctx, testNS, "widget-consumer", reconcileWait)
			waitGone(ctx, configMapObj(testNS, "blue-widget-card"), shortWait)
		})
	})

	// --- Scenario 13 ---
	Context("one owner per addon and per module (scenario 13)", Ordered, Label("addon-module-scenario-13"), func() {
		scope := scenarioScopes["13"]
		var widgetPlatformApp string
		var addonWidgetPlatform string
		var moduleWidgetKit string
		var moduleGadgetKit string
		var widgetsCRD string
		var testNS string
		addonApplication := scope.addonApplication
		uninstall := func(app, addon string, modules ...string) {
			scope.uninstall(ctx,
				app, addon, modules...)
		}
		BeforeAll(func() {
			widgetPlatformApp = scope.Text(fixtureWidgetPlatformApp)
			addonWidgetPlatform = scope.Text(fixtureAddonWidgetPlatform)
			moduleWidgetKit = scope.Text(fixtureModuleWidgetKit)
			moduleGadgetKit = scope.Text(fixtureModuleGadgetKit)
			widgetsCRD = scope.Text(fixtureWidgetsCRD)
			testNS = scope.Namespace
			scope.prepare(ctx)
		})

		It("refuses a second user Application for the same addon until the first is gone", func() {
			Expect(k8sClient.Create(ctx, addonApplication("platform-a", scope.Text("widget-platform"), "1.0.0", nil))).Should(Succeed())
			DeferCleanup(func() {
				deleteApp(ctx, testNS, "platform-a")
				deleteApp(ctx, testNS, "platform-b")
				waitAppGone(ctx, systemNS, addonWidgetPlatform, reconcileWait)
				waitAppGone(ctx, systemNS, moduleWidgetKit, reconcileWait)
				waitGone(ctx, crd(widgetsCRD), shortWait)
			})
			waitAppRunning(ctx, testNS, "platform-a", installWait)

			Expect(k8sClient.Create(ctx, addonApplication("platform-b", scope.Text("widget-platform"), "1.0.0", nil))).Should(Succeed())
			waitAppStatusContains(ctx, testNS, "platform-b", scope.Text("existing object Application vela-system/addon-widget-platform is managed by other application default/platform-a"), shortWait)
			Expect(mustGetApp(ctx, testNS, "platform-b").Status.Phase).ShouldNot(Equal(common.ApplicationRunning))
			Expect(mustGetApp(ctx, systemNS, addonWidgetPlatform).Labels).Should(HaveKeyWithValue(oam.LabelAppName, "platform-a"))

			By("deleting the winner lets the loser converge on its workflow retry")
			deleteApp(ctx, testNS, "platform-a")
			waitAppGone(ctx, systemNS, moduleWidgetKit, reconcileWait)
			waitAppRunning(ctx, testNS, "platform-b", retryWait)
			Expect(mustGetApp(ctx, systemNS, addonWidgetPlatform).Labels).Should(HaveKeyWithValue(oam.LabelAppName, "platform-b"))
		})

		It("refuses a second addon importing the same module until the first owner is gone", func() {
			Expect(k8sClient.Create(ctx, addonApplication(widgetPlatformApp, scope.Text("widget-platform"), "1.0.0", nil))).Should(Succeed())
			DeferCleanup(func() {
				deleteApp(ctx, testNS, widgetPlatformApp)
				uninstall(scope.Text("kit-suite"), scope.Text("addon-kit-suite"), moduleWidgetKit, moduleGadgetKit)
				waitAppGone(ctx, systemNS, addonWidgetPlatform, reconcileWait)
			})
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)

			Expect(k8sClient.Create(ctx, addonApplication(scope.Text("kit-suite"), scope.Text("kit-suite"), "1.0.0", nil))).Should(Succeed())
			waitAppStatusContains(ctx, systemNS, scope.Text("addon-kit-suite"), scope.Text("existing object Application vela-system/module-widget-kit is managed by other application vela-system/addon-widget-platform"), reconcileWait)
			waitAppRunning(ctx, systemNS, moduleGadgetKit, installWait)
			Expect(mustGetApp(ctx, systemNS, moduleWidgetKit).Labels).Should(HaveKeyWithValue(oam.LabelAppName, addonWidgetPlatform))
			Expect(mustGetApp(ctx, systemNS, moduleGadgetKit).Labels).Should(HaveKeyWithValue(oam.LabelAppName, scope.Text("addon-kit-suite")))
			Expect(mustGetApp(ctx, systemNS, scope.Text("addon-kit-suite")).Status.Phase).ShouldNot(Equal(common.ApplicationRunning))
			// The refused widget-kit component never gets a service entry on
			// addon-kit-suite (health is not collected for an object it does not
			// own, and its dispatch fails), so the addon status counts 2 healthy
			// of 2 and reports the phase. The reason is on addon-kit-suite, checked
			// above.
			Eventually(func(g Gomega) {
				svc := findService(getAppG(g, ctx, testNS, scope.Text("kit-suite")), scope.Text("kit-suite"))
				g.Expect(svc).ShouldNot(BeNil())
				g.Expect(svc.Healthy).Should(BeFalse())
				g.Expect(svc.Message).Should(ContainSubstring("workflowFailed"))
			}, reconcileWait, pollInterval).Should(Succeed())
			Expect(mustGetApp(ctx, testNS, scope.Text("kit-suite")).Status.Phase).ShouldNot(Equal(common.ApplicationRunning))

			By("removing the other owner")
			deleteApp(ctx, testNS, widgetPlatformApp)
			waitAppGone(ctx, systemNS, addonWidgetPlatform, reconcileWait)
			// Later Contexts create widget-platform again (#23).
			waitAppGone(ctx, testNS, widgetPlatformApp, reconcileWait)

			// The refused step has failed 10 times by now, so the workflow
			// library terminated addon-kit-suite's workflow (workflowFailed). A
			// terminated workflow does not run again by itself, so freeing the
			// module is not enough: its workflow has to be restarted. Restarting
			// the user Application would not do it, since the rendered
			// addon-kit-suite is unchanged.
			By("restarting addon-kit-suite's workflow")
			restartWorkflow(ctx, systemNS, scope.Text("addon-kit-suite"), "true")
			waitAppRunning(ctx, systemNS, scope.Text("addon-kit-suite"), retryWait)
			Eventually(func(g Gomega) string {
				return getAppG(g, ctx, systemNS, moduleWidgetKit).Labels[oam.LabelAppName]
			}, reconcileWait, pollInterval).Should(Equal(scope.Text("addon-kit-suite")))
			waitAppRunning(ctx, testNS, scope.Text("kit-suite"), reconcileWait)
		})
	})

	// --- Scenario 14 ---
	Context("broken _imports.cue files and resolution failures (scenario 14)", Ordered, Label("addon-module-scenario-14"), func() {
		scope := scenarioScopes["14"]
		var appName string
		var moduleWidgetKit string
		var testNS string
		addonApplication := scope.addonApplication
		setAddonVersion := scope.setAddonVersion
		BeforeAll(func() {
			moduleWidgetKit = scope.Text(fixtureModuleWidgetKit)
			testNS =
				scope.Namespace
			appName = scope.Text("broken-imports")
			scope.prepare(ctx)
		})
		BeforeAll(func() {
			Expect(ociTags(moduleRegistry.hostBase, scope.Text("modules/widget-kit"))).ShouldNot(ContainElement("9.9.9"))
			DeferCleanup(func() {
				for _, name := range []string{appName, "missing-addon-version", "missing-addon", "unknown-addon-registry"} {
					deleteApp(ctx, testNS, name)
				}
				waitAppGone(ctx, systemNS, scope.Text("addon-broken-imports"), reconcileWait)
				Expect(isNotFound(ctx, configMapObj(systemNS, scope.Text("broken-imports-marker")))).Should(BeTrue())
			})
		})

		It("loads the addon but fails the generated module component when the import cannot be fetched", func() {
			cases := []struct{ version, component, fragment string }{
				{"1.0.1", scope.Text("widget-kit"), `module registry "no-such-registry" not found`},
				{"1.0.2", scope.Text("ghost-kit"), scope.Text("ghost-kit")},
				{"1.0.3", scope.Text("widget-kit"), "9.9.9"},
				{"1.0.4", scope.Text("widget-kit"), "~1.0.0"},
			}
			Expect(k8sClient.Create(ctx, addonApplication(appName, scope.Text("broken-imports"), cases[0].version, nil))).Should(Succeed())
			for i, tc := range cases {
				By("broken-imports " + tc.version)
				if i > 0 {
					setAddonVersion(ctx, appName, tc.version)
				}
				Eventually(func(g Gomega) {
					addonApp := getAppG(g, ctx, systemNS, scope.Text("addon-broken-imports"))
					g.Expect(addonApp.Labels).Should(HaveKeyWithValue(oam.LabelAddonVersion, tc.version))
					g.Expect(addonApp.Status.Phase).ShouldNot(Equal(common.ApplicationRunning))
					res := findService(addonApp, scope.Text("broken-imports-resources"))
					g.Expect(res).ShouldNot(BeNil(), appStatusText(addonApp))
					g.Expect(res.Healthy).Should(BeTrue(), "the addon's own resources install")
					// The module component fails while its manifest is generated
					// (the module cannot be fetched), before health is collected or
					// anything is dispatched, so it gets no service entry. The reason
					// is in the workflow step message.
					if mod := findService(addonApp, tc.component); mod != nil {
						g.Expect(mod.Healthy).Should(BeFalse())
					}
					text := appStatusText(addonApp)
					g.Expect(text).Should(SatisfyAll(ContainSubstring(tc.component), ContainSubstring(tc.fragment)))
					marker, err := getConfigMap(ctx, systemNS, scope.Text("broken-imports-marker"))
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(marker.Data).Should(HaveKeyWithValue("variant", tc.version))
				}, reconcileWait, pollInterval).Should(Succeed())
				// The user Application's addon status only counts components with
				// a service entry, so it shows Ready:1/1 workflowFailed (#19, #22).
				Eventually(func(g Gomega) {
					svc := findService(getAppG(g, ctx, testNS, appName), appName)
					g.Expect(svc).ShouldNot(BeNil())
					g.Expect(svc.Healthy).Should(BeFalse())
					g.Expect(svc.Message).Should(ContainSubstring("workflowFailed"))
				}, reconcileWait, pollInterval).Should(Succeed())
				Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: moduleWidgetKit, Namespace: systemNS}})).Should(BeTrue())
				Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: scope.Text("module-ghost-kit"), Namespace: systemNS}})).Should(BeTrue())
			}
			deleteApp(ctx, testNS, appName)
			waitAppGone(ctx, systemNS, scope.Text("addon-broken-imports"), reconcileWait)
			// The child goes first; the user Application is only removed once
			// its finalizer has garbage-collected it. The next spec re-creates
			// the same name, which is refused while it is still terminating.
			waitAppGone(ctx, testNS, appName, reconcileWait)
		})

		It("fails the type: addon component itself when _imports.cue cannot be parsed, installing nothing", func() {
			cases := []struct{ version, fragment string }{
				{"1.0.5", `must declare exactly one sources[] entry (found 2)`},
				{"1.0.6", `import missing required field "module"`},
				{"1.0.7", `reference "parameter" not found`},
			}
			for _, tc := range cases {
				By("broken-imports " + tc.version)
				// Whatever the previous spec or case left behind must be fully
				// gone, or the create is refused with "object is being deleted".
				waitAppGone(ctx, testNS, appName, reconcileWait)
				Expect(k8sClient.Create(ctx, addonApplication(appName, scope.Text("broken-imports"), tc.version, nil))).Should(Succeed())
				waitAppStatusContains(ctx, testNS, appName, tc.fragment, reconcileWait)
				Expect(mustGetApp(ctx, testNS, appName).Status.Phase).ShouldNot(Equal(common.ApplicationRunning))
				Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: scope.Text("addon-broken-imports"), Namespace: systemNS}})).Should(BeTrue())
				Expect(isNotFound(ctx, configMapObj(systemNS, scope.Text("broken-imports-marker")))).Should(BeTrue())
				deleteApp(ctx, testNS, appName)
				waitAppGone(ctx, testNS, appName, shortWait)
			}
		})

		It("reports addon-level mistakes where they belong: unknown registry at admission, unknown version or addon at render", func() {
			err := k8sClient.Create(ctx, &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "unknown-addon-registry", Namespace: testNS},
				Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{
					Name: scope.Text("widget-platform"), Type: "addon",
					Properties: rawExtension(map[string]interface{}{"registry": "no-such-addon-registry", "version": "1.0.0"}),
				}}},
			})
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("is not a configured addon registry"))

			Expect(k8sClient.Create(ctx, addonApplication("missing-addon-version", scope.Text("widget-platform"), "9.9.9", nil))).Should(Succeed())
			waitAppStatusContains(ctx, testNS, "missing-addon-version", scope.Text(`addon "widget-platform" version "9.9.9" not found in registries`), reconcileWait)

			Expect(k8sClient.Create(ctx, addonApplication("missing-addon", scope.Text("ghost-addon"), "1.0.0", nil))).Should(Succeed())
			waitAppStatusContains(ctx, testNS, "missing-addon", scope.Text(`addon "ghost-addon" version "1.0.0" not found in registries`), reconcileWait)
		})
	})

	// --- Scenario 15 ---
	Context("registries disappear, credentials go bad (scenario 15)", Ordered, Serial, Label("addon-module-scenario-15"), func() {
		scope := scenarioScopes["15"]
		var widgetPlatformApp string
		var addonWidgetPlatform string
		var moduleWidgetKit string
		var widgetsCRD string
		var testNS string
		addonApplication := scope.addonApplication

		uninstall := func(app, addon string,

			modules ...string) {
			scope.uninstall(ctx, app, addon, modules...)
		}
		BeforeAll(func() {
			widgetPlatformApp = scope.Text(fixtureWidgetPlatformApp)
			addonWidgetPlatform = scope.Text(fixtureAddonWidgetPlatform)
			moduleWidgetKit = scope.Text(fixtureModuleWidgetKit)
			widgetsCRD = scope.Text(fixtureWidgetsCRD)
			testNS = scope.Namespace
			scope.prepare(ctx)
		})

		health := func(g Gomega, ready string) {
			svc := findService(mustGetApp(ctx, testNS, widgetPlatformApp), scope.Text("widget-platform"))
			g.Expect(svc).ShouldNot(BeNil())
			g.Expect(svc.Healthy).Should(BeTrue(), svc.Message)
			g.Expect(svc.Message).Should(Equal(ready))
		}
		installed := func(g Gomega) {
			g.Expect(getAppG(g, ctx, systemNS, moduleWidgetKit).Name).Should(Equal(moduleWidgetKit))
			_, err := getComponentDefinition(ctx, systemNS, scope.Text("widget-kit-v1-widget"))
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: widgetsCRD}, crd(widgetsCRD))).Should(Succeed())
		}
		// A registry that goes away breaks the health-check render, which is only
		// logged: the service keeps the health it last recorded, so the failure
		// is not surfaced in status. That is a known limitation of addon and
		// module health; what must hold is that nothing is removed meanwhile.
		staysInstalled := func() {
			Consistently(installed, 30*time.Second, 5*time.Second).Should(Succeed())
		}
		restoreModuleRegistry := func() {
			runVelaSucceed("module", "registry", "add", moduleRegistryName, moduleRegistry.cluster, "--type", "oci")
		}
		restoreAddonRegistry := func() { addAddonRegistry(ctx) }

		BeforeAll(func() {
			Expect(k8sClient.Create(ctx, addonApplication(widgetPlatformApp, scope.Text("widget-platform"), "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
			Eventually(func(g Gomega) { health(g, "Ready:6/6") }, shortWait, pollInterval).Should(Succeed())
			DeferCleanup(func() {
				restoreModuleRegistry()
				restoreAddonRegistry()
				deleteApp(ctx, testNS, "new-platform")
				uninstall(widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit)
			})
		})

		It("a deleted module registry removes nothing, and health is right when it is back", func() {
			runVelaSucceed("module", "registry", "delete", moduleRegistryName)
			staysInstalled()

			restoreModuleRegistry()
			Eventually(func(g Gomega) { health(g, "Ready:6/6") }, reconcileWait, pollInterval).Should(Succeed())
		})

		It("refuses a credential for a plain-HTTP module registry instead of storing a bad one", func() {
			// The manual scenario sets a wrong ECR password and watches the
			// controller report 401. A plain-HTTP registry never carries a
			// credential: the CLI refuses the record outright, so the check
			// here is that the refusal leaves the working entry alone.
			out, err := runVela("module", "registry", "add", moduleRegistryName, moduleRegistry.cluster, "--type", "oci", "--username", "AWS", "--password", "not-a-real-token")
			Expect(err).Should(HaveOccurred(), out)
			restoreModuleRegistry()
			Expect(runVelaSucceed("module", "registry", "list")).Should(ContainSubstring(moduleRegistryName))
			Consistently(func(g Gomega) { health(g, "Ready:6/6") }, 30*time.Second, 5*time.Second).Should(Succeed())
		})

		It("a deleted addon registry removes nothing, even across a controller restart, and admission still checks the name", func() {
			runVelaSucceed("addon", "registry", "delete", addonRegistryName)
			Consistently(func(g Gomega) { health(g, "Ready:6/6") }, 90*time.Second, 10*time.Second).Should(Succeed(), "the pinned render is served from vela-core's cache")

			err := k8sClient.Create(ctx, addonApplication("new-platform", scope.Text("kit-suite"), "1.0.0", nil))
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("is not a configured addon registry"))

			By("restarting vela-core, which empties the cache so the health-check render now fails")
			restartVelaCore(ctx)
			staysInstalled()

			By("adding the registry back")
			restoreAddonRegistry()
			Eventually(func(g Gomega) { health(g, "Ready:6/6") }, reconcileWait, pollInterval).Should(Succeed())
		})
	})

	// --- Scenario 16 ---
	Context("vela addon enable installs the same addon without its modules (scenario 16)", Ordered, Label("addon-module-scenario-16"), func() {
		scope := scenarioScopes["16"]
		var widgetPlatformApp string
		var addonWidgetPlatform string
		var moduleWidgetKit string
		var testNS string
		addonApplication := scope.addonApplication
		uninstall :=
			func(app, addon string, modules ...string) {
				scope.uninstall(ctx, app, addon, modules...)
			}
		BeforeAll(func() {
			widgetPlatformApp = scope.Text(fixtureWidgetPlatformApp)
			addonWidgetPlatform = scope.Text(fixtureAddonWidgetPlatform)
			moduleWidgetKit = scope.Text(fixtureModuleWidgetKit)
			testNS = scope.Namespace
			scope.prepare(ctx)
		})

		BeforeAll(func() {
			// The legacy installer downloads the addon in the CLI process,
			// through the stored registry record.
			skipUnlessHostReachesCluster(addonRegistry, "vela addon enable")
		})

		AfterAll(func() {
			_, _ = runVela("addon", "disable", scope.Text("widget-platform"), "-f")
			uninstall(widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit)
		})

		It("the legacy installer ignores _imports.cue", func() {
			runVelaSucceed("addon", "enable", addonRegistryName+scope.Text("/widget-platform"), "--version", "1.0.0", "-y")
			app := mustGetApp(ctx, systemNS, addonWidgetPlatform)
			Expect(componentNames(app)).Should(ConsistOf("platform-info", scope.Text("widget-platform-resources"), "team-config"))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAddonName, scope.Text("widget-platform")))
			Expect(app.Labels).ShouldNot(HaveKey(oam.LabelAppName), "a CLI install has no owning Application")
			Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: moduleWidgetKit, Namespace: systemNS}})).Should(BeTrue())
			Expect(moduleDefinitionNames(ctx, systemNS, scope.Text("widget-kit"))).Should(BeEmpty())
			_, err := getTraitDefinition(ctx, systemNS, scope.Text("platform-owner"))
			Expect(err).ShouldNot(HaveOccurred(), "the CLI applies the addon's own definitions")
			Expect(strings.ToLower(runVelaSucceed("addon", "status", scope.Text("widget-platform")))).Should(ContainSubstring("enabled"))
		})

		It("the component path cannot adopt a CLI-installed addon Application", func() {
			Expect(k8sClient.Create(ctx, addonApplication(widgetPlatformApp, scope.Text("widget-platform"), "1.0.0", nil))).Should(Succeed())
			waitAppStatusContains(ctx, testNS, widgetPlatformApp, "exists but not managed by any application", shortWait)
			Expect(mustGetApp(ctx, testNS, widgetPlatformApp).Status.Phase).ShouldNot(Equal(common.ApplicationRunning))
		})

		It("after vela addon disable, the component path installs the addon with its module", func() {
			runVelaSucceed("addon", "disable", scope.Text("widget-platform"))
			waitAppRunning(ctx, testNS, widgetPlatformApp, retryWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
			app := mustGetApp(ctx, systemNS, addonWidgetPlatform)
			Expect(componentNames(app)).Should(ContainElements(scope.Text("widget-kit"), "addon-definitions", "addon-auxiliaries"))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAppName, widgetPlatformApp))
		})
	})

	// --- Scenario 18 (last: it restarts the controller with gates toggled) ---
	Context("turning the feature gates off under a running install (scenario 18)", Ordered, Serial, Label("addon-module-scenario-18"), func() {
		scope := scenarioScopes["18"]
		var widgetPlatformApp string
		var addonWidgetPlatform string
		var moduleWidgetKit string
		var moduleGadgetKit string
		var testNS string
		addonApplication := scope.addonApplication

		testdataPath := scope.Path
		uninstall := func(app, addon string, modules ...string) {
			scope.uninstall(ctx, app, addon, modules...)
		}
		BeforeAll(func() {
			widgetPlatformApp = scope.Text(fixtureWidgetPlatformApp)
			addonWidgetPlatform = scope.Text(
				fixtureAddonWidgetPlatform)
			moduleWidgetKit = scope.Text(fixtureModuleWidgetKit)
			moduleGadgetKit =
				scope.Text(fixtureModuleGadgetKit)
			testNS = scope.Namespace
			scope.prepare(
				ctx)
		})

		BeforeAll(func() {
			addon, module := featureGateArgs(ctx)
			Expect(addon).Should(Equal("true"), "EnableAddonComponent must be on for this suite")
			Expect(module).Should(Equal("true"), "EnableModuleComponent must be on for this suite")
			DeferCleanup(func() {
				setFeatureGates(ctx, true, true)
				deleteApp(ctx, testNS, "gate-off-probe")
				deleteApp(ctx, testNS, "module-direct")
				waitAppGone(ctx, systemNS, moduleGadgetKit, reconcileWait)
				uninstall(widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit)
			})

			Expect(k8sClient.Create(ctx, addonApplication(widgetPlatformApp, scope.Text("widget-platform"), "1.0.0", nil))).Should(Succeed())
			Expect(applyManifestFile(ctx, testdataPath("apps", "module-direct.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
			waitAppRunning(ctx, testNS, "module-direct", installWait)
		})

		// With a gate off, every render of that component type fails. For an
		// install whose workflow already finished, the only render left is the
		// health check, and a health check that cannot render is only logged: the
		// service keeps the health it last recorded. So the gate message is not
		// surfaced on running installs, a known limitation of addon and module
		// health. What must hold is that turning a gate off removes nothing.
		It("with the module gate off, nothing is garbage-collected", func() {
			setFeatureGates(ctx, true, false)
			Consistently(func(g Gomega) {
				g.Expect(getAppG(g, ctx, systemNS, moduleWidgetKit).Name).Should(Equal(moduleWidgetKit))
				g.Expect(getAppG(g, ctx, systemNS, moduleGadgetKit).Name).Should(Equal(moduleGadgetKit))
			}, 30*time.Second, 5*time.Second).Should(Succeed())
		})

		It("with both gates off, nothing is garbage-collected and admission refuses new type: addon Applications with the gate message", func() {
			setFeatureGates(ctx, false, false)
			Consistently(func(g Gomega) {
				g.Expect(getAppG(g, ctx, systemNS, addonWidgetPlatform).Name).Should(Equal(addonWidgetPlatform))
				g.Expect(getAppG(g, ctx, systemNS, moduleWidgetKit).Name).Should(Equal(moduleWidgetKit))
			}, 30*time.Second, 5*time.Second).Should(Succeed())

			probe := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "gate-off-probe", Namespace: testNS},
				Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{
					Name: scope.Text("widget-platform"), Type: "addon",
					Properties: rawExtension(map[string]interface{}{"registry": "no-such-addon-registry"}),
				}}},
			}
			// The registry check is gated and skipped, but admission still renders
			// the Application in validation-only mode, and the vela/addon
			// provider checks the gate before its validation-only shortcut. So
			// the create is refused with the gate message, not with "not a
			// configured addon registry".
			err := k8sClient.Create(ctx, probe)
			Expect(err).Should(HaveOccurred(), "admission refuses a type: addon Application while the gate is off")
			Expect(err.Error()).Should(SatisfyAll(
				ContainSubstring("addon-as-component is disabled; enable the EnableAddonComponent feature gate"),
				Not(ContainSubstring("not a configured addon registry")),
			))
			Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "gate-off-probe", Namespace: testNS}})).Should(BeTrue())
		})

		It("turning both gates back on heals everything on the next reconcile", func() {
			setFeatureGates(ctx, true, true)
			Eventually(func(g Gomega) {
				svc := findService(getAppG(g, ctx, testNS, widgetPlatformApp), scope.Text("widget-platform"))
				g.Expect(svc).ShouldNot(BeNil())
				g.Expect(svc.Healthy).Should(BeTrue(), svc.Message)
				g.Expect(svc.Message).Should(Equal("Ready:6/6"))
			}, reconcileWait, pollInterval).Should(Succeed())
			Eventually(func(g Gomega) {
				svc := findService(getAppG(g, ctx, testNS, "module-direct"), scope.Text("gadget-kit"))
				g.Expect(svc).ShouldNot(BeNil())
				g.Expect(svc.Healthy).Should(BeTrue(), svc.Message)
				g.Expect(svc.Message).Should(Equal("Ready:3/3"))
			}, reconcileWait, pollInterval).Should(Succeed())
		})
	})
})

// These checks read immutable publications directly from the registries. They
// do not depend on the ordered controller lifecycle or mutate its Applications,
// definitions, CRDs, or registry records, so another Ginkgo worker can run them.
var _ = Describe("Addons that import modules", func() {
	Context("publish and inspect artifacts (scenario 01)", Label("addon-module-scenario-01"), func() {
		scope := scenarioScopes["01"]
		testdataPath := scope.Path

		It("refuses to republish an existing version without --force", func() {
			out, err := runVela("module", "publish", testdataPath("modules", "widget-kit-1.0.0"), moduleRegistry.host)
			Expect(err).Should(HaveOccurred(), out)
			Expect(out).Should(ContainSubstring("is already published"))
			Expect(out).Should(ContainSubstring("--force"))
		})

		It("stores the addons in the ChartMuseum index, modules/_imports.cue included", func() {
			index, err := httpGet(addonRegistry.host+"/index.yaml", "")
			Expect(err).ShouldNot(HaveOccurred(), index)
			for _, addon := range []string{scope.Text("widget-platform"), scope.Text("kit-suite"), scope.Text("widget-latest"), scope.Text("import-options"), scope.Text("tenant-widgets"), scope.Text("cache-probe"), scope.Text("broken-imports")} {
				Expect(index).Should(ContainSubstring(addon+"-"), "index.yaml should list %s", addon)
			}
			Expect(index).Should(ContainSubstring(scope.Text("widget-platform-1.2.0.tgz")))
			Expect(index).Should(ContainSubstring(scope.Text("broken-imports-1.0.7.tgz")), "vela addon push does not parse _imports.cue, so every broken variant is accepted")
		})
	})
})
