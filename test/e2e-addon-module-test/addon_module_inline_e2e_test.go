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

// These scenarios repeat their external counterparts against a module bundled
// inline under the addon's modules/<name>/ instead of imported from the module
// registry. The inline fixtures are renamed copies (widget-kit-inline,
// kitinline.example.com, ...), so they never share an owned Application name
// or CRDs with the external ones. The two paths differ in two places, marked
// where they occur: an inline module is dispatched as a type: k8s-objects
// component wrapping the already-rendered module Application, and it reports
// generic health rather than an aggregated Ready:N/M count.

// fixtureWidgetKitInlineTiers and fixtureWidgetKitInlineDefinitions are the
// widget-kit-inline counterparts of fixtureWidgetKitTiers and
// fixtureWidgetKitDefinitions.
var fixtureWidgetKitInlineTiers = []string{"widget-kit-inline-aux", "widget-kit-inline-v1-aux", "widget-kit-inline-v1-defs", "widget-kit-inline-v2-aux", "widget-kit-inline-v2-defs"}
var fixtureWidgetKitInlineDefinitions = []string{"widget-kit-inline-v1-widget", "widget-kit-inline-v2-widget", "widget-kit-inline-v1-labeler", "widget-kit-inline-v1-note", "widget-kit-inline-v2-labeler"}

var _ = Describe("Addons that bundle modules inline", func() {
	ctx := context.Background()
	crd := func(name string) *apiextensionsv1.CustomResourceDefinition {
		return &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: name}}
	}

	// --- Scenario 02i ---
	Context("single-module addon install, inline module (scenario 02i)", Ordered, Label("addon-module-scenario-02i"), func() {
		scope := scenarioScopes["02i"]
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
			widgetPlatformApp = scope.Text("widget-platform-inline")
			addonWidgetPlatform = scope.Text("addon-widget-platform-inline")
			moduleWidgetKit = scope.Text("module-widget-kit-inline")
			widgetsCRD = scope.Text("widgets.kitinline.example.com")
			widgetClassesCRD = scope.Text("widgetclasses.kitinline.example.com")
			widgetKitTiers = scope.Texts(fixtureWidgetKitInlineTiers)
			widgetKitDefinitions = scope.Texts(fixtureWidgetKitInlineDefinitions)
			testNS = scope.Namespace
			widgetGVK = scope.InlineGVK("Widget")
			widgetClassGVK = scope.InlineGVK("WidgetClass")
			scope.prepare(ctx)
		})

		BeforeAll(func() {
			By("installing widget-platform-inline 1.0.0 with addon parameters")
			Expect(k8sClient.Create(ctx, addonApplication(widgetPlatformApp, scope.Text("widget-platform-inline"), "1.0.0", map[string]interface{}{
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
				svc := findService(app, scope.Text("widget-platform-inline"))
				g.Expect(svc).ShouldNot(BeNil(), appStatusText(app))
				g.Expect(svc.Healthy).Should(BeTrue(), svc.Message)
				g.Expect(svc.Message).Should(Equal("Ready:7/7"))
			}, shortWait, pollInterval).Should(Succeed())
		})

		It("level 2: the addon Application has the addon's components plus one generated module component", func() {
			app := mustGetApp(ctx, systemNS, addonWidgetPlatform)

			By("labels naming the addon, its version, the registry that served it and the owning Application")
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAddonName, scope.Text("widget-platform-inline")))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAddonVersion, "1.0.0"))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAddonRegistry, addonRegistryName))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAppName, widgetPlatformApp))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAppNamespace, testNS))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAppComponent, scope.Text("widget-platform-inline")))

			By("components in render order: template, resources, module import, auxiliary groups")
			Expect(componentNames(app)).Should(Equal([]string{
				"platform-info", scope.Text("widget-platform-inline-resources"), "team-config", scope.Text("widget-kit-inline"), "addon-definitions", "addon-secret", "addon-auxiliaries",
			}))
			Expect(findComponent(app, "platform-info").Type).Should(Equal("k8s-objects"))
			Expect(findComponent(app, "platform-info").DependsOn).Should(Equal([]string{scope.Text("widget-platform-inline-resources")}))
			Expect(findComponent(app, "team-config").DependsOn).Should(Equal([]string{scope.Text("widget-platform-inline-resources")}))

			By("the generated module component depending on the resource components and the outputs, not on template components")
			mod := findComponent(app, scope.Text("widget-kit-inline"))
			Expect(mod.DependsOn).Should(ConsistOf(scope.Text("widget-platform-inline-resources"), "team-config", "addon-auxiliaries"))
			// Divergence 1/2: an external module component carries its {module,
			// registry, version} request; an inline one has nothing to request.
			// It wraps the already-rendered module Application, so its
			// properties are {objects: [<that Application>]}.
			Expect(mod.Type).Should(Equal("k8s-objects"))
			objs := propertiesOf(mod.Properties)["objects"].([]interface{})
			Expect(objs).Should(HaveLen(1))
			owned := objs[0].(map[string]interface{})
			Expect(owned["kind"]).Should(Equal("Application"))
			Expect(owned["metadata"].(map[string]interface{})["name"]).Should(Equal(moduleWidgetKit))

			By("the state-keep policy and the skipped last-applied annotation")
			Expect(app.Spec.Policies).Should(HaveLen(1))
			Expect(app.Spec.Policies[0].Name).Should(Equal("addon-component-state-keep"))
			Expect(app.Spec.Policies[0].Type).Should(Equal("apply-once"))
			Expect(propertiesOf(app.Spec.Policies[0].Properties)).Should(HaveKeyWithValue("enable", false))
			Expect(app.Annotations).Should(HaveKeyWithValue(oam.AnnotationLastAppliedConfig, "skip"))

			By("every service healthy")
			for _, svc := range app.Status.Services {
				Expect(svc.Healthy).Should(BeTrue(), "%s: %s", svc.Name, svc.Message)
			}
			// Divergence 2/2: type: module reads the module Application's own
			// service health into a "Ready:N/M" message; k8s-objects has no such
			// health policy, so an inline module reports generic health only.
		})

		It("level 2: the addon's own objects, parameters and definition are installed", func() {
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: scope.Text("widget-platform-inline-system")}, &corev1.Namespace{})).Should(Succeed())

			cm, err := getConfigMap(ctx, systemNS, scope.Text("widget-platform-inline-config"))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(cm.Data).Should(HaveKeyWithValue("source", "resources/platform-config.yaml"))

			notes, err := getConfigMap(ctx, systemNS, scope.Text("widget-platform-inline-notes"))
			Expect(err).ShouldNot(HaveOccurred(), "template.cue outputs become the addon-auxiliaries component")
			Expect(notes.Data).Should(HaveKeyWithValue("greeting", "hello from scenario 02"))
			Expect(notes.Data).Should(HaveKeyWithValue("addonVersion", "1.0.0"))

			info, err := getConfigMap(ctx, scope.Text("widget-platform-inline-system"), scope.Text("widget-platform-inline-info"))
			Expect(err).ShouldNot(HaveOccurred(), "a package main resource file is merged into template.cue")
			Expect(info.Data).Should(HaveKeyWithValue("greeting", "hello from scenario 02"))

			team, err := getConfigMap(ctx, scope.Text("widget-platform-inline-system"), scope.Text("widget-platform-inline-team"))
			Expect(err).ShouldNot(HaveOccurred(), "a resource .cue file without a package header renders as its own component")
			Expect(team.Data).Should(HaveKeyWithValue("team", "team-blue"))

			var secret corev1.Secret
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Namespace: systemNS, Name: scope.Text("addon-secret-widget-platform-inline")}, &secret)).Should(Succeed())
			Expect(string(secret.Data["addonParameterDataKey"])).Should(SatisfyAll(
				ContainSubstring(`"greeting":"hello from scenario 02"`), ContainSubstring(`"team":"team-blue"`)))

			owner, err := getTraitDefinition(ctx, systemNS, scope.Text("platform-owner-inline"))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(owner.Labels).ShouldNot(HaveKey(veltypes.LabelDefinitionModule), "an addon-level definition carries no module labels")
		})

		It("level 3: the module Application has one tier per level and enabled line, in dependency order", func() {
			app := mustGetApp(ctx, systemNS, moduleWidgetKit)
			Expect(app.Labels).Should(HaveKeyWithValue(veltypes.LabelDefinitionModule, scope.Text("widget-kit-inline")))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAppName, addonWidgetPlatform))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAppComponent, scope.Text("widget-kit-inline")))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAddonName, scope.Text("widget-platform-inline")), "the addon Application's labels are merged onto everything it dispatches")
			Expect(app.Annotations).Should(HaveKeyWithValue(veltypes.AnnoDefinitionModuleVersion, "1.0.0"))

			Expect(componentNames(app)).Should(Equal(widgetKitTiers))
			Expect(findComponent(app, scope.Text("widget-kit-inline-aux")).DependsOn).Should(BeEmpty())
			Expect(findComponent(app, scope.Text("widget-kit-inline-v1-aux")).DependsOn).Should(Equal([]string{scope.Text("widget-kit-inline-aux")}))
			Expect(findComponent(app, scope.Text("widget-kit-inline-v1-defs")).DependsOn).Should(Equal([]string{scope.Text("widget-kit-inline-v1-aux")}))
			Expect(findComponent(app, scope.Text("widget-kit-inline-v2-aux")).DependsOn).Should(Equal([]string{scope.Text("widget-kit-inline-aux")}), "lines are siblings: v2 never waits for v1")
			Expect(findComponent(app, scope.Text("widget-kit-inline-v2-defs")).DependsOn).Should(Equal([]string{scope.Text("widget-kit-inline-v2-aux")}))
			for _, c := range app.Spec.Components {
				Expect(c.Type).Should(Equal("k8s-objects"), c.Name)
			}
		})

		It("level 3: the module installed its CRDs, auxiliary objects and stamped definitions, and nothing from the disabled line", func() {
			By("module-level auxiliary objects: two CRDs from one multi-document file, a ConfigMap from CUE, a ClusterRole")
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: widgetsCRD}, crd(widgetsCRD))).Should(Succeed())
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: widgetClassesCRD}, crd(widgetClassesCRD))).Should(Succeed())
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: scope.Text("widget-kit-inline-viewer")}, &rbacv1.ClusterRole{})).Should(Succeed())
			info, err := getConfigMap(ctx, systemNS, scope.Text("widget-kit-inline-module-info"))
			Expect(err).ShouldNot(HaveOccurred(), "an auxiliary object without a namespace lands in the definition namespace")
			Expect(info.Data).Should(HaveKeyWithValue("moduleVersion", "1.0.0"))

			By("line-level auxiliary objects: a WidgetClass and a ConfigMap per enabled line")
			for _, line := range []string{"v1", "v2"} {
				className := map[string]string{"v1": scope.Text("widget-kit-inline-v1-standard"), "v2": scope.Text("widget-kit-inline-v2-premium")}[line]
				class, err := getUnstructured(ctx, widgetClassGVK, "", className)
				Expect(err).ShouldNot(HaveOccurred(), className)
				Expect(class.GetLabels()).Should(HaveKeyWithValue(scope.Text("kitinline.example.com/line"), line))
				_, err = getConfigMap(ctx, systemNS, scope.Text("widget-kit-inline-")+line+"-line-config")
				Expect(err).ShouldNot(HaveOccurred())
			}

			By("five stamped definitions with module labels")
			Expect(moduleDefinitionNames(ctx, systemNS, scope.Text("widget-kit-inline"))).Should(ConsistOf(widgetKitDefinitions))
			cd, err := getComponentDefinition(ctx, systemNS, scope.Text("widget-kit-inline-v1-widget"))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(cd.Labels).Should(HaveKeyWithValue(veltypes.LabelDefinitionModuleAPIVersion, "v1"))
			Expect(cd.Labels).Should(HaveKeyWithValue(veltypes.LabelDefinitionName, scope.Text("widget")))
			Expect(cd.Annotations).Should(HaveKeyWithValue(veltypes.AnnoDefinitionModuleFullName, scope.Text("widget-kit-inline-v1-widget")))
			// stampIdentity (pkg/module/service/render.go) writes
			// addons.oam.dev/name=<module> into the manifest, but that is not what
			// reaches the cluster on this path. An Application merges its own
			// labels onto every object it dispatches
			// (generateAndFilterCommonLabels in pkg/appfile/appfile.go), so the
			// addons.oam.dev/{name,version,registry} labels the renderer put on
			// addon-widget-platform-inline travel onto module-widget-kit-inline and from there
			// onto the definitions, overriding the module's stamp. A module
			// installed through "vela module deploy" keeps the module name here;
			// one installed through an addon is grouped under that addon.
			Expect(cd.Labels).Should(HaveKeyWithValue(oam.LabelAddonName, scope.Text("widget-platform-inline")))
			Expect(cd.Labels).Should(HaveKeyWithValue(oam.LabelAddonVersion, "1.0.0"))
			Expect(cd.Labels).Should(HaveKeyWithValue(oam.LabelAddonRegistry, addonRegistryName))
			v2, err := getComponentDefinition(ctx, systemNS, scope.Text("widget-kit-inline-v2-widget"))
			Expect(err).ShouldNot(HaveOccurred(), "a YAML definition installs like a CUE one")
			Expect(v2.Labels).Should(HaveKeyWithValue(veltypes.LabelDefinitionModuleAPIVersion, "v2"))

			By("the disabled v1beta1 line leaving nothing behind")
			Expect(isNotFound(ctx, componentDefinitionObj(systemNS, scope.Text("widget-kit-inline-v1beta1-widget")))).Should(BeTrue())
			Expect(isNotFound(ctx, configMapObj(systemNS, scope.Text("widget-kit-inline-v1beta1-preview")))).Should(BeTrue())
		})

		It("a consumer uses Form 3, a module trait and the addon-level trait", func() {
			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-widget-inline.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, "widget-consumer-inline", shortWait)

			blue, err := getUnstructured(ctx, widgetGVK, testNS, "blue-widget")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(nestedString(blue, "spec", "classRef")).Should(Equal(scope.Text("widget-kit-inline-v1-standard")))
			Expect(nestedString(blue, "spec", "apiLine")).Should(Equal("v1"))
			Expect(nestedString(blue, "spec", "color")).Should(Equal("blue"))
			Expect(nestedString(blue, "spec", "replicas")).Should(Equal("2"))
			Expect(blue.GetLabels()).Should(HaveKeyWithValue(scope.Text("kitinline.example.com/tier"), "gold"))
			Expect(blue.GetLabels()).Should(HaveKeyWithValue(scope.Text("kitinline.example.com/labeled-by"), scope.Text("widget-kit-inline-v1-labeler")))
			Expect(blue.GetLabels()).Should(HaveKeyWithValue(oam.WorkloadTypeLabel, scope.Text("widget-kit-inline-v1-widget")), scope.Text("the workload type label carries the resolved name, not the widget-kit-inline/v1/widget spelling"))
			Expect(blue.GetAnnotations()).Should(HaveKeyWithValue(scope.Text("kitinline.example.com/owner"), "team-blue"))

			big, err := getUnstructured(ctx, widgetGVK, testNS, "big-widget")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(nestedString(big, "spec", "classRef")).Should(Equal(scope.Text("widget-kit-inline-v2-premium")))
			Expect(nestedString(big, "spec", "size")).Should(Equal("L"))
			Expect(big.GetLabels()).Should(HaveKeyWithValue(scope.Text("kitinline.example.com/tier"), "silver"))
			Expect(big.GetLabels()).Should(HaveKeyWithValue(scope.Text("kitinline.example.com/line"), "v2"))
			Expect(big.GetLabels()).Should(HaveKeyWithValue(scope.Text("kitinline.example.com/labeled-by"), scope.Text("widget-kit-inline-v2-labeler")))

			card, err := getConfigMap(ctx, testNS, "blue-widget-card")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(card.Data).Should(Equal(map[string]string{"color": "blue", "line": "v1", "module": scope.Text("widget-kit-inline")}))
		})

		It("every level has its own ResourceTracker", func() {
			for _, name := range []string{widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit, "widget-consumer-inline"} {
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
			deleteApp(ctx, testNS, "widget-consumer-inline")
			waitGone(ctx, unstructuredObj(widgetGVK, testNS, "blue-widget"), shortWait)

			uninstall(widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit)
			waitGone(ctx, crd(widgetClassesCRD), shortWait)
			waitGone(ctx, componentDefinitionObj(systemNS, scope.Text("widget-kit-inline-v1-widget")), shortWait)
			waitGone(ctx, traitDefinitionObj(systemNS, scope.Text("platform-owner-inline")), shortWait)
			waitGone(ctx, configMapObj(systemNS, scope.Text("widget-kit-inline-module-info")), shortWait)
			waitGone(ctx, configMapObj(systemNS, scope.Text("widget-platform-inline-notes")), shortWait)
			waitGone(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: scope.Text("widget-platform-inline-system")}}, reconcileWait)
		})
	})

	// --- Scenario 03i ---
	Context("one addon, two inline modules (scenario 03i)", Ordered, Label("addon-module-scenario-03i"), func() {
		scope := scenarioScopes["03i"]
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
			moduleWidgetKit = scope.Text("module-widget-kit-inline")
			moduleGadgetKit = scope.Text(
				"module-gadget-kit-inline")
			gadgetsCRD = scope.Text("gadgets.kitinline.example.com")
			testNS = scope.Namespace
			widgetGVK =
				scope.InlineGVK("Widget")
			gadgetGVK = scope.InlineGVK("Gadget")
			scope.prepare(ctx)
		})

		BeforeAll(func() {
			Expect(k8sClient.Create(ctx, addonApplication(scope.Text("kit-suite-inline"), scope.Text("kit-suite-inline"), "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, scope.Text("kit-suite-inline"), installWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
			waitAppRunning(ctx, systemNS, moduleGadgetKit, shortWait)
			DeferCleanup(func() {
				deleteApp(ctx, testNS, "suite-consumer-inline")
				uninstall(scope.Text("kit-suite-inline"), scope.Text("addon-kit-suite-inline"), moduleWidgetKit, moduleGadgetKit)
			})
		})

		It("fans one addon Application out into two module components and two module Applications", func() {
			app := mustGetApp(ctx, systemNS, scope.Text("addon-kit-suite-inline"))
			names := componentNames(app)
			Expect(names).Should(HaveLen(3))
			Expect(names[0]).Should(Equal(scope.Text("kit-suite-inline-resources")))
			// Inline modules follow modules/ directory enumeration, which carries
			// no author-declared order, so the two module components' relative
			// order is not asserted.
			Expect(names[1:]).Should(ConsistOf(scope.Text("widget-kit-inline"), scope.Text("gadget-kit-inline")))
			for _, name := range []string{scope.Text("widget-kit-inline"), scope.Text("gadget-kit-inline")} {
				c := findComponent(app, name)
				Expect(c.Type).Should(Equal("k8s-objects"))
				Expect(c.DependsOn).Should(Equal([]string{scope.Text("kit-suite-inline-resources")}), scope.Text("kit-suite-inline has no outputs, so there is no addon-auxiliaries dependency"))
			}
			Expect(findService(mustGetApp(ctx, testNS, scope.Text("kit-suite-inline")), scope.Text("kit-suite-inline")).Message).Should(Equal("Ready:3/3"))

			for _, name := range []string{moduleWidgetKit, moduleGadgetKit} {
				mod := mustGetApp(ctx, systemNS, name)
				Expect(mod.Labels).Should(HaveKeyWithValue(oam.LabelAppName, scope.Text("addon-kit-suite-inline")))
				Expect(mod.Labels).Should(HaveKeyWithValue(oam.LabelAppComponent, strings.TrimPrefix(name, "module-")))
			}
		})

		It("installs gadget-kit-inline's three tiers and objects", func() {
			mod := mustGetApp(ctx, systemNS, moduleGadgetKit)
			Expect(componentNames(mod)).Should(Equal([]string{scope.Text("gadget-kit-inline-aux"), scope.Text("gadget-kit-inline-v1-aux"), scope.Text("gadget-kit-inline-v1-defs")}))
			Expect(findComponent(mod, scope.Text("gadget-kit-inline-v1-aux")).DependsOn).Should(Equal([]string{scope.Text("gadget-kit-inline-aux")}))
			Expect(findComponent(mod, scope.Text("gadget-kit-inline-v1-defs")).DependsOn).Should(Equal([]string{scope.Text("gadget-kit-inline-v1-aux")}))
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: gadgetsCRD}, crd(gadgetsCRD))).Should(Succeed())
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: scope.Text("gadget-kit-inline-editor")}, &rbacv1.ClusterRole{})).Should(Succeed())
			for _, cm := range []string{scope.Text("gadget-kit-inline-v1-defaults"), scope.Text("gadget-kit-inline-v1-profile")} {
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
			Expect(names).Should(ConsistOf(scope.Text("gadget-kit-inline-v1-labeler"), scope.Text("widget-kit-inline-v1-labeler"), scope.Text("widget-kit-inline-v2-labeler")))
		})

		It("rejects a Form 1 trait that both modules ship, at admission", func() {
			results := createEachFromFile(ctx, testdataPath("apps", "consumer-ambiguous-traits-inline.yaml"))
			Expect(results).Should(HaveLen(1))
			// The module list in the message follows the order the definitions
			// were listed in, which is not sorted, so each name is checked on
			// its own.
			for name, typeName := range map[string]string{"ambiguous-form1-inline": scope.Text("labeler")} {
				Expect(results[name]).Should(HaveOccurred(), name)
				Expect(results[name].Error()).Should(SatisfyAll(
					ContainSubstring(`type "`+typeName+`" is ambiguous: definitions from modules [`),
					ContainSubstring(scope.Text("gadget-kit-inline")),
					ContainSubstring(scope.Text("widget-kit-inline")),
					ContainSubstring("] all match"),
				), name)
			}
			for name := range results {
				Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS}})).Should(BeTrue(), name)
			}
		})

		It("resolves Form 3 always and Form 1 when the short name is unique", func() {
			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-suite-inline.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, "suite-consumer-inline", shortWait)

			widget, err := getUnstructured(ctx, widgetGVK, testNS, "suite-widget")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(nestedString(widget, "spec", "color")).Should(Equal("green"))
			Expect(widget.GetLabels()).Should(HaveKeyWithValue(scope.Text("kitinline.example.com/labeled-by"), scope.Text("widget-kit-inline-v1-labeler")))

			gadget, err := getUnstructured(ctx, gadgetGVK, testNS, "suite-gadget")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(nestedString(gadget, "spec", "mode")).Should(Equal("turbo"))
			Expect(gadget.GetLabels()).Should(HaveKeyWithValue(scope.Text("kitinline.example.com/labeled-by"), scope.Text("gadget-kit-inline-v1-labeler")))
			// Only a type containing a slash is swapped for the installed name
			// (makeWorkloadWithContext, pkg/appfile/appfile.go). A Form 1 name
			// is already a valid label value and is kept as written.
			Expect(gadget.GetLabels()).Should(HaveKeyWithValue(oam.WorkloadTypeLabel, scope.Text("gadget")), "Form 1 gadget keeps its short name in the label")
		})
	})

	// --- Scenario 04i ---
	Context("type reference forms, inline module (scenario 04i)", Ordered, Label("addon-module-scenario-04i"), func() {
		scope := scenarioScopes["04i"]
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
			widgetPlatformApp = scope.Text("widget-platform-inline")
			addonWidgetPlatform = scope.Text("addon-widget-platform-inline")
			moduleWidgetKit = scope.Text("module-widget-kit-inline")
			widgetKitDefinitions = scope.Texts(fixtureWidgetKitInlineDefinitions)
			testNS = scope.Namespace
			widgetGVK = scope.InlineGVK("Widget")
			scope.prepare(ctx)
		},
		)

		BeforeAll(func() {
			Expect(k8sClient.Create(ctx, addonApplication(widgetPlatformApp, scope.Text("widget-platform-inline"), "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
			Expect(moduleDefinitionNames(ctx, systemNS, scope.Text("widget-kit-inline"))).Should(ConsistOf(widgetKitDefinitions))
			DeferCleanup(func() {
				for _, name := range []string{"forms-accepted-inline", "v2-contract-inline", "trait-outputs-form3-inline"} {
					deleteApp(ctx, testNS, name)
				}
				uninstall(widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit)
			})
		})

		It("without addon parameters there is no addon-secret component", func() {
			Expect(componentNames(mustGetApp(ctx, systemNS, addonWidgetPlatform))).ShouldNot(ContainElement("addon-secret"))
			Expect(findService(mustGetApp(ctx, testNS, widgetPlatformApp), scope.Text("widget-platform-inline")).Message).Should(Equal("Ready:6/6"))
		})

		It("accepts Form 3, the installed name and a unique Form 1 trait", func() {
			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-forms-accepted-inline.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, "forms-accepted-inline", shortWait)
			for name, want := range map[string][2]string{
				"form3-widget":   {"v1", scope.Text("widget-kit-inline-v1-widget")},
				"stamped-widget": {"v1", scope.Text("widget-kit-inline-v1-widget")},
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
			Expect(note.Labels).Should(HaveKeyWithValue(oam.TraitTypeLabel, scope.Text("widget-kit-inline-v1-note")))
		})

		It("refuses ambiguous, disabled-line and malformed references at admission", func() {
			results := createEachFromFile(ctx, testdataPath("apps", "consumer-rejected-inline.yaml"))
			want := map[string]string{
				"reject-form1-ambiguous-inline":           scope.Text(`type "widget" is ambiguous`),
				"reject-disabled-line-inline":             scope.Text("widget-kit-inline-v1beta1-widget"),
				"reject-two-segment-disabled-line-inline": `two-segment references are not supported`,
				"reject-bad-api-version-inline":           `"1" is not a valid API version`,
				"reject-too-many-segments-inline":         "expected either 1 segment (name, Form 1) or 3 segments (module/v<N>/name, Form 3)",
			}
			Expect(results).Should(HaveLen(len(want)))
			for name, fragment := range want {
				Expect(results[name]).Should(HaveOccurred(), "%s must be denied", name)
				Expect(results[name].Error()).Should(ContainSubstring(fragment), name)
				Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS}})).Should(BeTrue(), name)
			}
		})

		It("holds a consumer to the v2 contract: size is required, color does not exist", func() {
			err := applyManifestFile(ctx, testdataPath("apps", "consumer-v2-contract-inline.yaml"))
			if err != nil {
				// With EnableCueValidation the webhook already refuses it.
				Expect(err.Error()).Should(ContainSubstring("size"))
				return
			}
			waitAppStatusContains(ctx, testNS, "v2-contract-inline", "size", shortWait)
			Expect(mustGetApp(ctx, testNS, "v2-contract-inline").Status.Phase).ShouldNot(Equal(common.ApplicationRunning))
			Expect(isNotFound(ctx, unstructuredObj(widgetGVK, testNS, "w"))).Should(BeTrue(), "the render must stop before creating the Widget")
		})

		It("labels the output of a Form 3 trait with the installed name", func() {
			// A trait written as widget-kit-inline/v1/note used to stamp that string
			// into trait.oam.dev/type, which is not a valid label value. Trait
			// outputs of a module trait now carry the installed name.
			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-trait-outputs-form3-inline.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, "trait-outputs-form3-inline", shortWait)
			cm, err := getConfigMap(ctx, testNS, "noted-widget-note")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(cm.Labels).Should(HaveKeyWithValue(oam.TraitTypeLabel, scope.Text("widget-kit-inline-v1-note")))
		})
	})

	// --- Scenario 07i ---
	Context("upgrade the addon and so its inline module, add a definition, remove an API line, roll back (scenario 07i)", Ordered, Label("addon-module-scenario-07i"), func() {
		scope := scenarioScopes["07i"]
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
		expectConsumerOfRemovedDefinition := scope.expectInlineConsumerOfRemovedDefinition
		testdataPath := scope.Path
		uninstall := func(app, addon string, modules ...string) {
			scope.uninstall(ctx, app, addon, modules...)
		}
		BeforeAll(func() {
			widgetPlatformApp = scope.Text("widget-platform-inline")
			addonWidgetPlatform = scope.Text("addon-widget-platform-inline")
			moduleWidgetKit = scope.Text("module-widget-kit-inline")
			widgetKitTiers = scope.Texts(fixtureWidgetKitInlineTiers)
			widgetKitDefinitions = scope.Texts(fixtureWidgetKitInlineDefinitions)
			testNS = scope.Namespace
			widgetGVK = scope.InlineGVK("Widget")
			widgetClassGVK = scope.InlineGVK("WidgetClass")
			scope.prepare(ctx)
		})

		moduleState := func(g Gomega, version string, tiers []string) {
			mod := mustGetApp(ctx, systemNS, moduleWidgetKit)
			g.Expect(mod.Annotations).Should(HaveKeyWithValue(veltypes.AnnoDefinitionModuleVersion, version))
			g.Expect(componentNames(mod)).Should(Equal(tiers))
		}

		BeforeAll(func() {
			Expect(k8sClient.Create(ctx, addonApplication(widgetPlatformApp, scope.Text("widget-platform-inline"), "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, installWait)
			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-v2-inline.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, "v2-consumer-inline", shortWait)
			DeferCleanup(func() {
				deleteApp(ctx, testNS, "gauge-consumer-inline")
				deleteApp(ctx, testNS, "v2-consumer-inline")
				waitAppGone(ctx, testNS, "gauge-consumer-inline", reconcileWait)
				waitAppGone(ctx, testNS, "v2-consumer-inline", reconcileWait)
				uninstall(widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit)
			})
		})

		It("starts from 1.0.0 with both lines", func() {
			Eventually(func(g Gomega) { moduleState(g, "1.0.0", widgetKitTiers) }, shortWait, pollInterval).Should(Succeed())
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
			Expect(moduleDefinitionNames(ctx, systemNS, scope.Text("widget-kit-inline"))).Should(ConsistOf(widgetKitDefinitions))
		})

		It("1.1.0 adds a definition and updates auxiliary objects in place", func() {
			setAddonVersion(ctx, widgetPlatformApp, "1.1.0")
			Eventually(func(g Gomega) { moduleState(g, "1.1.0", widgetKitTiers) }, installWait, pollInterval).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, installWait)
			Expect(mustGetApp(ctx, systemNS, addonWidgetPlatform).Labels).Should(HaveKeyWithValue(oam.LabelAddonVersion, "1.1.0"))
			Eventually(func() []string { return moduleDefinitionNames(ctx, systemNS, scope.Text("widget-kit-inline")) }, reconcileWait, pollInterval).Should(ConsistOf(append([]string{scope.Text("widget-kit-inline-v1-gauge")}, widgetKitDefinitions...)))
			info, err := getConfigMap(ctx, systemNS, scope.Text("widget-kit-inline-module-info"))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(info.Data).Should(HaveKeyWithValue("moduleVersion", "1.1.0"))
			line, err := getConfigMap(ctx, systemNS, scope.Text("widget-kit-inline-v1-line-config"))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(line.Data).Should(HaveKeyWithValue("configRevision", "2"))

			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-gauge-inline.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, "gauge-consumer-inline", shortWait)
			gauge, err := getConfigMap(ctx, testNS, "fuel-gauge-inline")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(gauge.Data).Should(Equal(map[string]string{"line": "v1", "module": scope.Text("widget-kit-inline"), "reading": "80", "since": "1.1.0"}))
		})

		It("1.2.0 switches the v2 line off: its tiers, definitions and auxiliary objects are garbage-collected, a consumer's Widget is not", func() {
			setAddonVersion(ctx, widgetPlatformApp, "1.2.0")
			Eventually(func(g Gomega) {
				moduleState(g, "1.2.0", []string{scope.Text("widget-kit-inline-aux"), scope.Text("widget-kit-inline-v1-aux"), scope.Text("widget-kit-inline-v1-defs")})
			}, installWait, pollInterval).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, installWait)
			waitGone(ctx, componentDefinitionObj(systemNS, scope.Text("widget-kit-inline-v2-widget")), reconcileWait)
			waitGone(ctx, traitDefinitionObj(systemNS, scope.Text("widget-kit-inline-v2-labeler")), shortWait)
			waitGone(ctx, unstructuredObj(widgetClassGVK, "", scope.Text("widget-kit-inline-v2-premium")), shortWait)
			waitGone(ctx, configMapObj(systemNS, scope.Text("widget-kit-inline-v2-line-config")), shortWait)
			info, err := getConfigMap(ctx, systemNS, scope.Text("widget-kit-inline-module-info"))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(info.Data).Should(HaveKeyWithValue("servedLines", "v1"))
			Expect(info.Data).Should(HaveKeyWithValue("disabledLines", "v1beta1,v2"))

			_, err = getUnstructured(ctx, widgetGVK, testNS, "premium-widget")
			Expect(err).ShouldNot(HaveOccurred(), "nothing deletes a consumer's already-applied objects")

			By("the v2 consumer cannot be updated, and fails to parse on its next reconcile")
			expectConsumerOfRemovedDefinition(ctx, "v2-consumer-inline", scope.Text("widget-kit-inline-v2-widget"), "premium-widget", "after-1.2.0")
		})

		It("rolling back to 1.0.0 brings v2 back and drops the gauge", func() {
			setAddonVersion(ctx, widgetPlatformApp, "1.0.0")
			Eventually(func(g Gomega) { moduleState(g, "1.0.0", widgetKitTiers) }, installWait, pollInterval).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
			Eventually(func() []string { return moduleDefinitionNames(ctx, systemNS, scope.Text("widget-kit-inline")) }, reconcileWait, pollInterval).Should(ConsistOf(widgetKitDefinitions))

			bumpPublishVersion(ctx, testNS, "v2-consumer-inline", "after-rollback")
			waitAppRunning(ctx, testNS, "v2-consumer-inline", reconcileWait)

			// widget-kit-inline-v1-gauge only exists from 1.1.0, so the gauge consumer
			// is now in the position the v2 consumer was in on 1.2.0.
			expectConsumerOfRemovedDefinition(ctx, "gauge-consumer-inline", scope.Text("widget-kit-inline-v1-gauge"), "", "after-rollback")
			_, err := getConfigMap(ctx, testNS, "fuel-gauge-inline")
			Expect(err).ShouldNot(HaveOccurred(), "nothing deletes a consumer's already-applied objects")
		})
	})

	// --- Scenario 10i ---
	Context("removing an inline module from an addon deletes its CRDs and every custom resource of them (scenario 10i)", Ordered, Label("addon-module-scenario-10i"), func() {
		scope := scenarioScopes["10i"]
		var moduleWidgetKit string
		var moduleGadgetKit string
		var gadgetsCRD string
		var testNS string
		widgetGVK := widgetGVK
		gadgetGVK := gadgetGVK
		addonApplication := scope.addonApplication
		setAddonVersion := scope.setAddonVersion
		expectConsumerOfRemovedDefinition := scope.expectInlineConsumerOfRemovedDefinition
		testdataPath := scope.Path
		uninstall :=
			func(app, addon string, modules ...string) {
				scope.uninstall(ctx, app, addon,
					modules...)
			}
		BeforeAll(func() {
			moduleWidgetKit = scope.Text("module-widget-kit-inline")
			moduleGadgetKit = scope.Text("module-gadget-kit-inline")
			gadgetsCRD = scope.Text("gadgets.kitinline.example.com")
			testNS = scope.Namespace
			widgetGVK = scope.InlineGVK("Widget")
			gadgetGVK = scope.InlineGVK("Gadget")
			scope.prepare(ctx)
		})

		BeforeAll(func() {
			Expect(k8sClient.Create(ctx, addonApplication(scope.Text("kit-suite-inline"), scope.Text("kit-suite-inline"), "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, scope.Text("kit-suite-inline"), installWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
			waitAppRunning(ctx, systemNS, moduleGadgetKit, shortWait)
			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-suite-keep-doomed-inline.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, "suite-consumer-inline", shortWait)
			DeferCleanup(func() {
				deleteApp(ctx, testNS, "suite-consumer-inline")
				waitAppGone(ctx, testNS, "suite-consumer-inline", reconcileWait)
				uninstall(scope.Text("kit-suite-inline"), scope.Text("addon-kit-suite-inline"), moduleWidgetKit, moduleGadgetKit)
			})
		})

		It("has one custom resource of each module before the upgrade", func() {
			_, err := getUnstructured(ctx, widgetGVK, testNS, "keep-widget")
			Expect(err).ShouldNot(HaveOccurred())
			_, err = getUnstructured(ctx, gadgetGVK, testNS, "doomed-gadget")
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("2.0.0 drops gadget-kit-inline: module Application, definitions, auxiliary objects, CRD and Gadgets go; widget-kit-inline is untouched", func() {
			setAddonVersion(ctx, scope.Text("kit-suite-inline"), "2.0.0")
			waitAppGone(ctx, systemNS, moduleGadgetKit, installWait)
			waitAppRunning(ctx, testNS, scope.Text("kit-suite-inline"), installWait)
			Expect(componentNames(mustGetApp(ctx, systemNS, scope.Text("addon-kit-suite-inline")))).Should(Equal([]string{scope.Text("kit-suite-inline-resources"), scope.Text("widget-kit-inline")}))
			waitGone(ctx, crd(gadgetsCRD), shortWait)
			Expect(isNotFound(ctx, componentDefinitionObj(systemNS, scope.Text("gadget-kit-inline-v1-gadget")))).Should(BeTrue())
			Expect(isNotFound(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: scope.Text("gadget-kit-inline-editor")}})).Should(BeTrue())
			Expect(isNotFound(ctx, configMapObj(systemNS, scope.Text("gadget-kit-inline-v1-defaults")))).Should(BeTrue())
			_, err := getUnstructured(ctx, widgetGVK, testNS, "keep-widget")
			Expect(err).ShouldNot(HaveOccurred())
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
		})

		It("leaves the consumer broken: its Gadget is gone with the kind and a new render finds no definition", func() {
			// Deleting the CRD deleted every Gadget, so unlike scenario 07 the
			// consumer's applied object of the removed module is gone too.
			expectConsumerOfRemovedDefinition(ctx, "suite-consumer-inline", scope.Text("gadget-kit-inline-v1-gadget"), "", "after-removal")
			Expect(isNotFound(ctx, crd(gadgetsCRD))).Should(BeTrue(), "the Gadget kind no longer exists")
			_, err := getUnstructured(ctx, widgetGVK, testNS, "keep-widget")
			Expect(err).ShouldNot(HaveOccurred(), "the other module's object is untouched")
		})

		It("downgrading to 1.0.0 brings the module, the CRD and, on the consumer's next run, the Gadget back", func() {
			setAddonVersion(ctx, scope.Text("kit-suite-inline"), "1.0.0")
			waitAppRunning(ctx, systemNS, moduleGadgetKit, installWait)
			bumpPublishVersion(ctx, testNS, "suite-consumer-inline", "after-restore")
			waitAppRunning(ctx, testNS, "suite-consumer-inline", reconcileWait)
			_, err := getUnstructured(ctx, gadgetGVK, testNS, "doomed-gadget")
			Expect(err).ShouldNot(HaveOccurred(), "a new Gadget; the old one was deleted with the CRD")
		})
	})

	// --- Scenario 11i ---
	Context("deleting things by hand at every level, inline module: the Application one level up restores them (scenario 11i)", Ordered, Label("addon-module-scenario-11i"), func() {
		scope := scenarioScopes["11i"]
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
			widgetPlatformApp = scope.Text("widget-platform-inline")
			addonWidgetPlatform = scope.Text("addon-widget-platform-inline")
			moduleWidgetKit = scope.Text("module-widget-kit-inline")
			widgetsCRD = scope.Text("widgets.kitinline.example.com")
			testNS = scope.Namespace
			widgetGVK = scope.InlineGVK("Widget")
			widgetClassGVK = scope.InlineGVK("WidgetClass")
			scope.prepare(ctx)
		})

		uidOf := func(obj client.Object) k8stypes.UID {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).Should(Succeed())
			return obj.GetUID()
		}
		// Restoring is state keep, which runs on every reconcile of the owning
		// Application. The chain is listed owner first: a request for an object
		// that is gone fails that poll, and its owner has been requested by then.
		// The consumer is left out: while its definitions are gone, admission
		// refuses its updates, a reconcile request included.
		waitBack := func(obj client.Object, timeout time.Duration) {
			chain := []client.Object{
				appRef(testNS, widgetPlatformApp), appRef(systemNS, addonWidgetPlatform), appRef(systemNS, moduleWidgetKit),
			}
			eventuallyReconciled(ctx, chain, timeout, func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).Should(Succeed())
			}, "%T %s did not come back", obj, client.ObjectKeyFromObject(obj))
		}

		BeforeAll(func() {
			Expect(k8sClient.Create(ctx, addonApplication(widgetPlatformApp, scope.Text("widget-platform-inline"), "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-blue-widget-inline.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, "widget-consumer-inline", shortWait)
			DeferCleanup(func() {
				deleteApp(ctx, testNS, "widget-consumer-inline")
				waitAppGone(ctx, testNS, "widget-consumer-inline", reconcileWait)
				uninstall(widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit)
			})
		})

		It("recreates a deleted module definition with a new UID", func() {
			// module-widget-kit-inline inherits addons.oam.dev/name from the addon
			// Application, which on its own would make the resource keeper treat
			// it as apply-once and skip state keep; the module renderer declares
			// apply-once off to prevent that.
			mod := mustGetApp(ctx, systemNS, moduleWidgetKit)
			Expect(mod.Labels).Should(HaveKey(oam.LabelAddonName))
			Expect(policyNames(mod)).Should(ContainElement("module-state-keep"))

			td := traitDefinitionObj(systemNS, scope.Text("widget-kit-inline-v1-labeler"))
			before := uidOf(td)
			Expect(k8sClient.Delete(ctx, td)).Should(Succeed())
			waitBack(traitDefinitionObj(systemNS, scope.Text("widget-kit-inline-v1-labeler")), reconcileWait)
			Expect(uidOf(traitDefinitionObj(systemNS, scope.Text("widget-kit-inline-v1-labeler")))).ShouldNot(Equal(before))
		})

		It("reverts an edited line auxiliary ConfigMap", func() {
			cm, err := getConfigMap(ctx, systemNS, scope.Text("widget-kit-inline-v1-line-config"))
			Expect(err).ShouldNot(HaveOccurred())
			cm.Data["configRevision"] = "tampered"
			Expect(k8sClient.Update(ctx, cm)).Should(Succeed())
			eventuallyReconciled(ctx, []client.Object{appRef(systemNS, moduleWidgetKit)}, reconcileWait, func(g Gomega) {
				got, err := getConfigMap(ctx, systemNS, scope.Text("widget-kit-inline-v1-line-config"))
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(got.Data["configRevision"]).Should(Equal("1"))
			})
		})

		It("recreates a deleted line auxiliary custom resource", func() {
			class := unstructuredObj(widgetClassGVK, "", scope.Text("widget-kit-inline-v1-standard"))
			Expect(k8sClient.Delete(ctx, class)).Should(Succeed())
			waitBack(unstructuredObj(widgetClassGVK, "", scope.Text("widget-kit-inline-v1-standard")), reconcileWait)
		})

		It("recreates a deleted module CRD, and the consumer's Widget comes back on its own next state keep", func() {
			Expect(k8sClient.Delete(ctx, crd(widgetsCRD))).Should(Succeed())
			waitBack(crd(widgetsCRD), reconcileWait)
			waitBack(unstructuredObj(widgetGVK, testNS, "blue-widget"), 5*time.Minute)
		})

		It("recreates a deleted addon-level definition", func() {
			Expect(k8sClient.Delete(ctx, traitDefinitionObj(systemNS, scope.Text("platform-owner-inline")))).Should(Succeed())
			waitBack(traitDefinitionObj(systemNS, scope.Text("platform-owner-inline")), reconcileWait)
		})

		It("recreates a deleted module Application, whose finalizer first removed everything it applied", func() {
			before := uidOf(&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: moduleWidgetKit, Namespace: systemNS}})
			deleteApp(ctx, systemNS, moduleWidgetKit)
			waitAppGone(ctx, systemNS, moduleWidgetKit, reconcileWait)
			Expect(isNotFound(ctx, crd(widgetsCRD))).Should(BeTrue(), "the finalizer removed the CRD")
			Expect(isNotFound(ctx, componentDefinitionObj(systemNS, scope.Text("widget-kit-inline-v1-widget")))).Should(BeTrue())
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

	// --- Scenario 12i ---
	Context("deleting the addon while something still uses its inline module (scenario 12i)", Ordered, Label("addon-module-scenario-12i"), func() {
		scope := scenarioScopes["12i"]
		var widgetPlatformApp string
		var addonWidgetPlatform string
		var moduleWidgetKit string
		var widgetsCRD string
		var testNS string
		widgetGVK := widgetGVK
		addonApplication := scope.addonApplication
		expectConsumerOfRemovedDefinition := scope.expectInlineConsumerOfRemovedDefinition
		testdataPath := scope.Path
		uninstall := func(app, addon string, modules ...string) {
			scope.uninstall(ctx, app, addon, modules...)
		}
		BeforeAll(func() {
			widgetPlatformApp = scope.Text("widget-platform-inline")
			addonWidgetPlatform = scope.Text("addon-widget-platform-inline")
			moduleWidgetKit = scope.Text("module-widget-kit-inline")
			widgetsCRD = scope.Text("widgets.kitinline.example.com")
			testNS = scope.Namespace
			widgetGVK = scope.InlineGVK("Widget")
			scope.prepare(ctx)
		})

		install := func() {
			Expect(k8sClient.Create(ctx, addonApplication(widgetPlatformApp, scope.Text("widget-platform-inline"), "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
		}

		BeforeAll(func() {
			install()
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-blue-widget-inline.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, "widget-consumer-inline", shortWait)
			_, err := getConfigMap(ctx, testNS, "blue-widget-card")
			Expect(err).ShouldNot(HaveOccurred())
			DeferCleanup(func() {
				deleteApp(ctx, testNS, "widget-consumer-inline")
				waitAppGone(ctx, testNS, "widget-consumer-inline", reconcileWait)
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
			expectConsumerOfRemovedDefinition(ctx, "widget-consumer-inline", scope.Text("widget-kit-inline-v1-widget"), "", "after-uninstall")
			_, err = getConfigMap(ctx, testNS, "blue-widget-card")
			Expect(err).ShouldNot(HaveOccurred(), "the built-in object is still there while the consumer fails to parse")
		})

		It("re-installing heals the consumer", func() {
			install()
			bumpPublishVersion(ctx, testNS, "widget-consumer-inline", "after-reinstall")
			waitAppRunning(ctx, testNS, "widget-consumer-inline", reconcileWait)
			w, err := getUnstructured(ctx, widgetGVK, testNS, "blue-widget")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(w.GetLabels()).Should(HaveKeyWithValue(scope.Text("kitinline.example.com/tier"), "gold"))
		})

		It("deletes a consumer cleanly after its definitions are gone", func() {
			uninstall(widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit)
			deleteApp(ctx, testNS, "widget-consumer-inline")
			waitAppGone(ctx, testNS, "widget-consumer-inline", reconcileWait)
			waitGone(ctx, configMapObj(testNS, "blue-widget-card"), shortWait)
		})
	})

	// --- Scenario 13i ---
	Context("one owner per inline module (scenario 13i)", Ordered, Label("addon-module-scenario-13i"), func() {
		scope := scenarioScopes["13i"]
		var testNS string
		addonApplication := scope.addonApplication
		BeforeAll(func() {
			testNS = scope.Namespace
			scope.prepare(ctx)
		})

		It("refuses a second addon importing the same inline module until the first owner is gone", func() {
			// widget-platform-inline and kit-suite-inline both bundle
			// widget-kit-inline, so this is the external scenario 13 conflict
			// dispatched through a type: k8s-objects component instead of
			// type: module. MustBeControlledByApp (pkg/utils/apply/apply.go) is
			// generic across component types, so the failure mode is the same.
			platformApp, suiteApp := scope.Text("widget-platform-inline"), scope.Text("kit-suite-inline")
			addonPlatform, addonSuite := scope.Text("addon-widget-platform-inline"), scope.Text("addon-kit-suite-inline")
			moduleWidget, moduleGadget := scope.Text("module-widget-kit-inline"), scope.Text("module-gadget-kit-inline")
			Expect(k8sClient.Create(ctx, addonApplication(platformApp, scope.Text("widget-platform-inline"), "1.0.0", nil))).Should(Succeed())
			DeferCleanup(func() {
				deleteApp(ctx, testNS, platformApp)
				scope.uninstall(ctx, suiteApp, addonSuite, moduleWidget, moduleGadget)
				waitAppGone(ctx, systemNS, addonPlatform, reconcileWait)
			})
			waitAppRunning(ctx, testNS, platformApp, installWait)

			Expect(k8sClient.Create(ctx, addonApplication(suiteApp, scope.Text("kit-suite-inline"), "1.0.0", nil))).Should(Succeed())
			waitAppStatusContains(ctx, systemNS, addonSuite,
				scope.Text("existing object Application vela-system/module-widget-kit-inline is managed by other application vela-system/addon-widget-platform-inline"), reconcileWait)
			waitAppRunning(ctx, systemNS, moduleGadget, installWait)
			Expect(mustGetApp(ctx, systemNS, moduleWidget).Labels).Should(HaveKeyWithValue(oam.LabelAppName, addonPlatform))
			Expect(mustGetApp(ctx, systemNS, moduleGadget).Labels).Should(HaveKeyWithValue(oam.LabelAppName, addonSuite))
			Expect(mustGetApp(ctx, systemNS, addonSuite).Status.Phase).ShouldNot(Equal(common.ApplicationRunning))
			Eventually(func(g Gomega) {
				svc := findService(getAppG(g, ctx, testNS, suiteApp), scope.Text("kit-suite-inline"))
				g.Expect(svc).ShouldNot(BeNil())
				g.Expect(svc.Healthy).Should(BeFalse())
				g.Expect(svc.Message).Should(ContainSubstring("workflowFailed"))
			}, reconcileWait, pollInterval).Should(Succeed())
			Expect(mustGetApp(ctx, testNS, suiteApp).Status.Phase).ShouldNot(Equal(common.ApplicationRunning))

			By("removing the other owner")
			deleteApp(ctx, testNS, platformApp)
			waitAppGone(ctx, systemNS, addonPlatform, reconcileWait)
			waitAppGone(ctx, testNS, platformApp, reconcileWait)

			By("restarting the second addon's workflow")
			restartWorkflow(ctx, systemNS, addonSuite, "true")
			waitAppRunning(ctx, systemNS, addonSuite, retryWait)
			Eventually(func(g Gomega) string {
				return getAppG(g, ctx, systemNS, moduleWidget).Labels[oam.LabelAppName]
			}, reconcileWait, pollInterval).Should(Equal(addonSuite))
			waitAppRunning(ctx, testNS, suiteApp, reconcileWait)
		})
	})

	// --- Scenario 14i ---
	Context("broken inline modules (scenario 14i)", Ordered, Label("addon-module-scenario-14i"), func() {
		scope := scenarioScopes["14i"]
		var appName string
		var testNS string
		addonApplication := scope.addonApplication
		BeforeAll(func() {
			testNS = scope.Namespace
			appName = scope.Text("broken-imports-inline")
			scope.prepare(ctx)
			DeferCleanup(func() {
				deleteApp(ctx, testNS, appName)
				waitAppGone(ctx, systemNS, scope.Text("addon-broken-imports-inline"), reconcileWait)
				Expect(isNotFound(ctx, configMapObj(systemNS, scope.Text("broken-imports-inline-marker")))).Should(BeTrue())
			})
		})

		It("fails the type: addon component itself when an inline module is malformed or collides, installing nothing", func() {
			// Unlike modules/_imports.cue (resolved lazily by a type: module
			// component's CueX provider, after the addon's own resources are
			// already dispatched), inline modules are parsed eagerly while the
			// addon package itself is read. Any parse failure or name collision
			// surfaces from the resolveAndRender call that builds the addon's
			// whole component list, so every case here installs nothing.
			cases := []struct{ version, fragment string }{
				{"1.0.1", `missing _module.cue`},
				{"1.0.2", `module declares no API lines`},
				{"1.0.3", `is declared more than once`},
			}
			for _, tc := range cases {
				By("broken-imports-inline " + tc.version)
				waitAppGone(ctx, testNS, appName, reconcileWait)
				Expect(k8sClient.Create(ctx, addonApplication(appName, scope.Text("broken-imports-inline"), tc.version, nil))).Should(Succeed())
				waitAppStatusContains(ctx, testNS, appName, tc.fragment, reconcileWait)
				if tc.version == "1.0.3" {
					// Names both colliding sources, not just the generic fragment.
					text := appStatusText(mustGetApp(ctx, testNS, appName))
					Expect(text).Should(SatisfyAll(
						ContainSubstring(scope.Text(`"widget-kit"`)), ContainSubstring("modules/_imports.cue"), ContainSubstring(scope.Text("modules/widget-kit"))))
				}
				Expect(mustGetApp(ctx, testNS, appName).Status.Phase).ShouldNot(Equal(common.ApplicationRunning))
				Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: scope.Text("addon-broken-imports-inline"), Namespace: systemNS}})).Should(BeTrue())
				Expect(isNotFound(ctx, configMapObj(systemNS, scope.Text("broken-imports-inline-marker")))).Should(BeTrue())
				deleteApp(ctx, testNS, appName)
				waitAppGone(ctx, testNS, appName, shortWait)
			}
		})
	})

	// --- Scenario 16i ---
	Context("vela addon enable installs the same addon without its inline module (scenario 16i)", Ordered, Label("addon-module-scenario-16i"), func() {
		scope := scenarioScopes["16i"]
		BeforeAll(func() {
			scope.prepare(ctx)
			// The legacy installer downloads the addon in the CLI process,
			// through the stored registry record.
			skipUnlessHostReachesCluster(addonRegistry, "vela addon enable")
		})

		AfterAll(func() {
			_, _ = runVela("addon", "disable", scope.Text("widget-platform-inline"), "-f")
			waitAppGone(ctx, systemNS, scope.Text("addon-widget-platform-inline"), reconcileWait)
		})

		It("the legacy installer ignores inline modules the same way", func() {
			// readInlineModulesDir (pkg/addon/addon.go) still parses modules/<name>/
			// into InstallPackage.InlineModules regardless of entry point, but
			// RenderInlineModuleComponents is only called from resolveAndRender
			// (pkg/addon/service/renderer.go), never from this legacy CLI path, so
			// the parsed module is never rendered into a component here.
			runVelaSucceed("addon", "enable", addonRegistryName+scope.Text("/widget-platform-inline"), "--version", "1.0.0", "-y")
			app := mustGetApp(ctx, systemNS, scope.Text("addon-widget-platform-inline"))
			Expect(componentNames(app)).ShouldNot(ContainElement(scope.Text("widget-kit-inline")))
			Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: scope.Text("module-widget-kit-inline"), Namespace: systemNS}})).Should(BeTrue())
			Expect(moduleDefinitionNames(ctx, systemNS, scope.Text("widget-kit-inline"))).Should(BeEmpty())
			runVelaSucceed("addon", "disable", scope.Text("widget-platform-inline"))
		})
	})

	// --- Scenario 19 ---
	Context("inline and imported modules in one addon (scenario 19)", Ordered, Label("addon-module-scenario-19"), func() {
		scope := scenarioScopes["19"]
		var appName, addonApp, moduleInline, moduleImported string
		var testNS string
		BeforeAll(func() {
			appName = scope.Text("inline-mixed")
			addonApp = scope.Text("addon-inline-mixed")
			moduleInline = scope.Text("module-inline-kit")
			moduleImported = scope.Text(fixtureModuleWidgetKit)
			testNS = scope.Namespace
			scope.prepare(ctx)
		})

		It("renders an inline k8s-objects component and an imported type: module component side by side", func() {
			Expect(k8sClient.Create(ctx, scope.addonApplication(appName, scope.Text("inline-mixed"), "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, appName, installWait)
			waitAppRunning(ctx, systemNS, addonApp, shortWait)
			waitAppRunning(ctx, systemNS, moduleInline, shortWait)
			waitAppRunning(ctx, systemNS, moduleImported, shortWait)

			app := mustGetApp(ctx, systemNS, addonApp)
			Expect(findComponent(app, scope.Text("widget-kit")).Type).Should(Equal("module"))
			Expect(findComponent(app, scope.Text("inline-kit")).Type).Should(Equal("k8s-objects"))
			for _, name := range []string{scope.Text("widget-kit"), scope.Text("inline-kit")} {
				Expect(findComponent(app, name).DependsOn).Should(ContainElement(scope.Text("inline-mixed-resources")), name)
			}
		})

		It("deleting the user Application garbage-collects both nested module Applications", func() {
			scope.uninstall(ctx, appName, addonApp, moduleInline, moduleImported)
		})
	})
})
