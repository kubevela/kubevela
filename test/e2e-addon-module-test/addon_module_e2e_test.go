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

// This file is the ordered cluster suite for addons that import modules. One
// Describe shares the two in-cluster registries and the published fixtures;
// each Context is one scenario of testing/addon-module-cr-based-single-cluster
// and leaves the cluster the way it found it (no Applications of ours, no
// kit.example.com CRDs).
//
// The Contexts run in the order written. Two orderings matter: "latest vs
// pinned" (scenario 08) has to run while widget-kit 1.0.0 is the only tag in
// the module registry, so it comes before the upgrade scenario (07) and
// publishes 1.1.0 and 1.2.0 itself; and "feature gates off" (18) restarts the
// controller with gates toggled, so it comes last.
package addonmoduletest

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	veltypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const (
	widgetPlatformApp   = "widget-platform"
	addonWidgetPlatform = "addon-widget-platform"
	moduleWidgetKit     = "module-widget-kit"
	moduleGadgetKit     = "module-gadget-kit"
	moduleProbeKit      = "module-probe-kit"
	moduleInlineKit     = "module-inline-kit"

	widgetsCRD       = "widgets.kit.example.com"
	widgetClassesCRD = "widgetclasses.kit.example.com"
	gadgetsCRD       = "gadgets.kit.example.com"
)

// widgetKitTiers is the tier order RenderApplication emits for widget-kit
// 1.0.0 and 1.1.0 (lines v1 and v2 enabled, v1beta1 disabled).
var widgetKitTiers = []string{"widget-kit-aux", "widget-kit-v1-aux", "widget-kit-v1-defs", "widget-kit-v2-aux", "widget-kit-v2-defs"}

// widgetKitDefinitions is every definition widget-kit 1.0.0 installs.
var widgetKitDefinitions = []string{"widget-kit-v1-widget", "widget-kit-v2-widget", "widget-kit-v1-labeler", "widget-kit-v1-note", "widget-kit-v2-labeler"}

// widgetKitInlineTiers/widgetKitInlineDefinitions are the same shape as
// widgetKitTiers/widgetKitDefinitions, for widget-kit-inline -- an exact rename
// of widget-kit-1.0.0 bundled under widget-platform-inline-1.0.0/modules/
// instead of published to a registry. Renamed (not just relocated) so this
// module never contends for the same owned Application name or the same CRDs
// as widget-kit when both run in the same suite.
var widgetKitInlineTiers = []string{"widget-kit-inline-aux", "widget-kit-inline-v1-aux", "widget-kit-inline-v1-defs", "widget-kit-inline-v2-aux", "widget-kit-inline-v2-defs"}
var widgetKitInlineDefinitions = []string{"widget-kit-inline-v1-widget", "widget-kit-inline-v2-widget", "widget-kit-inline-v1-labeler", "widget-kit-inline-v1-note", "widget-kit-inline-v2-labeler"}

// widgetVariant describes one way a module reaches an addon, so a scenario
// that ought to behave identically either way can run once per variant
// (scenario 02) instead of being duplicated by hand. addonAppName/
// moduleAppName are methods, not fields, because they are always exactly
// "addon-"/"module-" plus the slug -- the real render convention, not a
// test-only naming choice -- so there is nothing to keep in sync by hand.
type widgetVariant struct {
	label          string // "external" or "inline"
	addonFixture   string // testdata/addons/<dir>, pushed in BeforeAll
	addonSlug      string // the published addon's own name (metadata.yaml)
	moduleSlug     string // the module's own name (_module.cue)
	crdGroup       string // the CRD group widget-kit(-inline)'s auxiliary/ declares
	moduleCompType string // "module" (external) or "k8s-objects" (inline)
	appName        string // the user Application this scenario creates
	consumerApp    string // the consumer Application name
	consumerFile   string // testdata/apps/<file>
	ownerTraitName string // the addon-level trait definition name (platform-owner or its inline-renamed twin)
	suffix         string // "" (external) or "-inline" -- appended to shared fixture-file/app names scenario 04 reuses
	tiers          []string
	definitions    []string

	// selfHealConsumerApp/selfHealConsumerFile are scenario 11's own consumer
	// fixture: one Form 3 Widget plus a module trait, no addon-level trait --
	// distinct from consumerApp/consumerFile, which scenario 02 uses.
	selfHealConsumerApp  string
	selfHealConsumerFile string

	// v2ConsumerApp/v2ConsumerFile and gaugeConsumerApp/gaugeConsumerFile are
	// scenario 07's own consumer fixtures: a v2-line Widget (gone once the
	// addon upgrades to 1.2.0) and a v1 Gauge (only present from 1.1.0 on).
	v2ConsumerApp     string
	v2ConsumerFile    string
	gaugeConsumerApp  string
	gaugeConsumerFile string
}

func (v widgetVariant) addonAppName() string  { return "addon-" + v.addonSlug }
func (v widgetVariant) moduleAppName() string { return "module-" + v.moduleSlug }

var externalWidgetVariant = widgetVariant{
	label:          "external",
	addonFixture:   "widget-platform-1.0.0",
	addonSlug:      "widget-platform",
	moduleSlug:     "widget-kit",
	crdGroup:       "kit.example.com",
	moduleCompType: "module",
	appName:        widgetPlatformApp,
	consumerApp:    "widget-consumer",
	consumerFile:   "consumer-widget.yaml",
	tiers:          widgetKitTiers,
	definitions:    widgetKitDefinitions,
	ownerTraitName: "platform-owner",
	suffix:         "",

	selfHealConsumerApp:  "widget-consumer",
	selfHealConsumerFile: "consumer-blue-widget.yaml",

	v2ConsumerApp:     "v2-consumer",
	v2ConsumerFile:    "consumer-v2.yaml",
	gaugeConsumerApp:  "gauge-consumer",
	gaugeConsumerFile: "consumer-gauge.yaml",
}

// suiteVariant is scenario 03's own variant struct -- two modules in one
// addon, not one, and no separate tenant namespace, so it does not reuse
// widgetVariant even though several fields overlap in spirit.
type suiteVariant struct {
	label             string
	addonFixture      string
	addonSlug         string // also the user Application name, same as the external fixture's convention
	widgetModule      string
	gadgetModule      string
	crdGroup          string
	moduleCompType    string
	appName           string
	consumerApp       string
	consumerFile      string
	ambiguousFile     string
	ambiguousForm2App string
	ambiguousForm1App string

	// keepDoomedFile is scenario 10's own consumer fixture: one component per
	// module, so dropping the gadget module's import/inline bundle takes only
	// the Gadget component's definition and CRD with it.
	keepDoomedFile string
}

var externalSuiteVariant = suiteVariant{
	label:             "external",
	addonFixture:      "kit-suite-1.0.0",
	addonSlug:         "kit-suite",
	widgetModule:      "widget-kit",
	gadgetModule:      "gadget-kit",
	crdGroup:          "kit.example.com",
	moduleCompType:    "module",
	appName:           "kit-suite",
	consumerApp:       "suite-consumer",
	consumerFile:      "consumer-suite.yaml",
	ambiguousFile:     "consumer-ambiguous-traits.yaml",
	ambiguousForm2App: "ambiguous-form2",
	ambiguousForm1App: "ambiguous-form1",
	keepDoomedFile:    "consumer-suite-keep-doomed.yaml",
}

var inlineSuiteVariant = suiteVariant{
	label:             "inline",
	addonFixture:      "kit-suite-inline-1.0.0",
	addonSlug:         "kit-suite-inline",
	widgetModule:      "widget-kit-inline",
	gadgetModule:      "gadget-kit-inline",
	crdGroup:          "kitinline.example.com",
	moduleCompType:    "k8s-objects",
	appName:           "kit-suite-inline",
	consumerApp:       "suite-consumer-inline",
	consumerFile:      "consumer-suite-inline.yaml",
	ambiguousFile:     "consumer-ambiguous-traits-inline.yaml",
	ambiguousForm2App: "ambiguous-form2-inline",
	ambiguousForm1App: "ambiguous-form1-inline",
	keepDoomedFile:    "consumer-suite-keep-doomed-inline.yaml",
}

var inlineWidgetVariant = widgetVariant{
	label:          "inline",
	addonFixture:   "widget-platform-inline-1.0.0",
	addonSlug:      "widget-platform-inline",
	moduleSlug:     "widget-kit-inline",
	crdGroup:       "kitinline.example.com",
	moduleCompType: "k8s-objects",
	appName:        "widget-platform-inline",
	consumerApp:    "widget-consumer-inline",
	consumerFile:   "consumer-widget-inline.yaml",
	tiers:          widgetKitInlineTiers,
	definitions:    widgetKitInlineDefinitions,
	ownerTraitName: "platform-owner-inline",
	suffix:         "-inline",

	selfHealConsumerApp:  "widget-consumer-inline",
	selfHealConsumerFile: "consumer-blue-widget-inline.yaml",

	v2ConsumerApp:     "v2-consumer-inline",
	v2ConsumerFile:    "consumer-v2-inline.yaml",
	gaugeConsumerApp:  "gauge-consumer-inline",
	gaugeConsumerFile: "consumer-gauge-inline.yaml",
}

var _ = Describe("Addons that import modules", Ordered, func() {
	var ctx context.Context

	crd := func(name string) *apiextensionsv1.CustomResourceDefinition {
		return &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: name}}
	}

	// uninstall deletes a user Application and waits for the whole chain below
	// it to be garbage-collected: the addon Application, the module
	// Applications named, and the widget CRD when widget-kit was among them.
	uninstall := func(appName, addonAppName string, moduleApps ...string) {
		deleteApp(ctx, testNS, appName)
		waitAppGone(ctx, testNS, appName, shortWait)
		waitAppGone(ctx, systemNS, addonAppName, reconcileWait)
		for _, m := range moduleApps {
			waitAppGone(ctx, systemNS, m, reconcileWait)
			if m == moduleWidgetKit {
				waitGone(ctx, crd(widgetsCRD), shortWait)
			}
			if m == moduleGadgetKit {
				waitGone(ctx, crd(gadgetsCRD), shortWait)
			}
		}
	}

	BeforeAll(func() {
		ctx = context.Background()

		By("bringing up the in-cluster registries: registry:2 for modules, ChartMuseum for addons")
		Expect(applyManifestFile(ctx, testdataPath("registry.yaml"))).Should(Succeed())
		waitForDeploymentAvailable(ctx, "default", "oci-registry")
		waitForDeploymentAvailable(ctx, "default", "addon-module-chartmuseum")
		moduleRegistry = resolveRegistryEndpoints(ctx, moduleRegistryURLEnv, moduleRegistryHostURLEnv, moduleRegistryNodePort, "/modules", "/v2/")
		addonRegistry = resolveRegistryEndpoints(ctx, addonRegistryURLEnv, addonRegistryHostURLEnv, addonRegistryNodePort, "", "/index.yaml")

		By("registering the module registry and publishing the modules the early scenarios need")
		// The record carries the cluster URL; "vela module registry add" only
		// writes the ConfigMap and Secret, it does not dial the registry.
		// widget-kit 1.1.0 and 1.2.0 are deliberately NOT published here: the
		// "latest vs pinned" scenario needs 1.0.0 to be the highest tag when it
		// installs, and publishes the two newer versions itself.
		runVelaSucceed("module", "registry", "add", moduleRegistryName, moduleRegistry.cluster, "--type", "oci")
		publishModuleFixture("widget-kit-1.0.0")
		publishModuleFixture("gadget-kit-1.0.0")
		publishModuleFixture("probe-kit-1.0.0-a")

		By("pushing every addon version to ChartMuseum, then registering it as a Helm addon registry")
		// Every version is pushed before the first render so the controller's
		// index cache (3 minutes for a small repository) never hides one.
		for _, dir := range []string{
			"widget-platform-1.0.0", "widget-platform-1.1.0", "widget-platform-1.2.0",
			"widget-platform-inline-1.0.0", "widget-platform-inline-1.1.0", "widget-platform-inline-1.2.0",
			"kit-suite-1.0.0", "kit-suite-2.0.0", "kit-suite-inline-1.0.0", "kit-suite-inline-2.0.0",
			"widget-latest-1.0.0", "import-options-1.0.0", "tenant-widgets-1.0.0",
			"cache-probe-1.0.0-a",
			"broken-imports-1.0.1", "broken-imports-1.0.2", "broken-imports-1.0.3", "broken-imports-1.0.4",
			"broken-imports-1.0.5", "broken-imports-1.0.6", "broken-imports-1.0.7",
			"broken-imports-inline-1.0.1", "broken-imports-inline-1.0.2", "broken-imports-inline-1.0.3",
			"inline-mixed-1.0.0",
		} {
			pushAddonFixture(dir)
		}
		addAddonRegistry(ctx)
	})

	AfterAll(func() {
		By("removing whatever a failed scenario may have left behind")
		for _, name := range []string{
			widgetPlatformApp, "widget-platform-inline", "kit-suite", "kit-suite-inline", "import-options", "tenant-widgets", "widget-latest", "cache-probe",
			"platform-a", "platform-b", "broken-imports", "missing-addon-version", "missing-addon", "new-platform",
			"module-direct", "gate-off-probe", "widget-consumer", "widget-consumer-inline", "suite-consumer", "suite-consumer-inline", "forms-accepted", "v2-contract",
			"trait-outputs-form3", "default-consumer", "default-consumer-form2", "gauge-consumer", "v2-consumer",
			"gauge-consumer-inline", "v2-consumer-inline", "broken-imports-inline",
		} {
			deleteApp(ctx, testNS, name)
		}
		for _, name := range []string{addonWidgetPlatform, "addon-widget-platform-inline", "addon-kit-suite", "addon-kit-suite-inline", "addon-import-options", "addon-tenant-widgets",
			"addon-widget-latest", "addon-cache-probe", "addon-broken-imports", "addon-broken-imports-inline",
			"addon-inline-mixed"} {
			waitAppGone(ctx, systemNS, name, reconcileWait)
		}
		for _, name := range []string{moduleWidgetKit, "module-widget-kit-inline", moduleGadgetKit, "module-gadget-kit-inline", moduleProbeKit, moduleInlineKit} {
			waitAppGone(ctx, systemNS, name, reconcileWait)
		}
		_, _ = runVela("module", "registry", "delete", moduleRegistryName)
		_, _ = runVela("addon", "registry", "delete", addonRegistryName)
		deleteManifestFile(ctx, testdataPath("registry.yaml"))
	})

	// --- Scenario 01 ---
	Context("publish and inspect artifacts (scenario 01)", func() {
		It("published widget-kit 1.0.0 as the only tag of its repository", func() {
			Expect(ociTags(moduleRegistry.hostBase, "modules/widget-kit")).Should(ConsistOf("1.0.0"),
				"the latest-vs-pinned scenario needs 1.0.0 to be the highest widget-kit tag; a registry left over from an earlier run breaks that")
		})

		It("refuses to republish an existing version without --force", func() {
			out, err := runVela("module", "publish", testdataPath("modules", "widget-kit-1.0.0"), moduleRegistry.host)
			Expect(err).Should(HaveOccurred(), out)
			Expect(out).Should(ContainSubstring("is already published"))
			Expect(out).Should(ContainSubstring("--force"))
		})

		It("stores the addons in the ChartMuseum index, modules/_imports.cue included", func() {
			index, err := httpGet(addonRegistry.host+"/index.yaml", "")
			Expect(err).ShouldNot(HaveOccurred(), index)
			for _, addon := range []string{"widget-platform", "kit-suite", "widget-latest", "import-options", "tenant-widgets", "cache-probe", "broken-imports"} {
				Expect(index).Should(ContainSubstring(addon+"-"), "index.yaml should list %s", addon)
			}
			Expect(index).Should(ContainSubstring("widget-platform-1.2.0.tgz"))
			Expect(index).Should(ContainSubstring("broken-imports-1.0.7.tgz"), "vela addon push does not parse _imports.cue, so every broken variant is accepted")
		})

		It("vela module deploy --dry-run fetches the module and prints the wrapper Application", func() {
			skipUnlessHostReachesCluster(moduleRegistry, "vela module deploy --dry-run (a client-side fetch through the stored registry record)")
			out := runVelaSucceed("module", "deploy", "widget-kit", "--registry", moduleRegistryName, "--version", "1.0.0", "-n", systemNS, "--dry-run")
			Expect(out).Should(ContainSubstring("module-widget-kit-deploy"))
			Expect(out).Should(ContainSubstring("type: module"))
			Expect(out).Should(ContainSubstring("registry: " + moduleRegistryName))
			Expect(out).Should(ContainSubstring("version: 1.0.0"))

			out, err := runVela("module", "deploy", "widget-kit", "--registry", moduleRegistryName, "--version", "9.9.9", "-n", systemNS, "--dry-run")
			Expect(err).Should(HaveOccurred(), out)
			Expect(out).Should(ContainSubstring("9.9.9"))
		})
	})

	// --- Scenario 02 ---
	// Runs once per module source: external (modules/_imports.cue, a
	// type: module component) and inline (modules/<name>/, a type: k8s-objects
	// component wrapping the owned Application). Everything here should hold
	// regardless of source except the two spots marked below, which is the
	// point of running both: a divergence anywhere else is a real bug, not an
	// expected difference.
	for _, v := range []widgetVariant{externalWidgetVariant, inlineWidgetVariant} {
		v := v
		Context(fmt.Sprintf("single-module addon install (scenario 02, %s)", v.label), func() {
			BeforeAll(func() {
				By("installing " + v.addonSlug + " 1.0.0 with addon parameters")
				Expect(k8sClient.Create(ctx, addonApplication(v.appName, v.addonSlug, "1.0.0", map[string]interface{}{
					"greeting": "hello from scenario 02",
					"team":     "team-blue",
				}))).Should(Succeed())
				waitAppRunning(ctx, testNS, v.appName, installWait)
				waitAppRunning(ctx, systemNS, v.addonAppName(), shortWait)
				waitAppRunning(ctx, systemNS, v.moduleAppName(), shortWait)
			})

			It("level 1: the user Application reports the seven generated components healthy", func() {
				Eventually(func(g Gomega) {
					app := getAppG(g, ctx, testNS, v.appName)
					svc := findService(app, v.addonSlug)
					g.Expect(svc).ShouldNot(BeNil(), appStatusText(app))
					g.Expect(svc.Healthy).Should(BeTrue(), svc.Message)
					g.Expect(svc.Message).Should(Equal("Ready:7/7"))
				}, shortWait, pollInterval).Should(Succeed())
			})

			It("level 2: the addon Application has the addon's components plus one generated module component", func() {
				app := mustGetApp(ctx, systemNS, v.addonAppName())

				By("labels naming the addon, its version, the registry that served it and the owning Application")
				Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAddonName, v.addonSlug))
				Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAddonVersion, "1.0.0"))
				Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAddonRegistry, addonRegistryName))
				Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAppName, v.appName))
				Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAppNamespace, testNS))
				Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAppComponent, v.addonSlug))

				By("components in render order: template, resources, module import, auxiliary groups")
				Expect(componentNames(app)).Should(Equal([]string{
					"platform-info", v.addonSlug + "-resources", "team-config", v.moduleSlug,
					"addon-definitions", "addon-secret", "addon-auxiliaries",
				}))
				Expect(findComponent(app, "platform-info").Type).Should(Equal("k8s-objects"))
				Expect(findComponent(app, "platform-info").DependsOn).Should(Equal([]string{v.addonSlug + "-resources"}))
				Expect(findComponent(app, "team-config").DependsOn).Should(Equal([]string{v.addonSlug + "-resources"}))

				By("the generated module component depending on the resource components and the outputs, not on template components")
				mod := findComponent(app, v.moduleSlug)
				Expect(mod.Type).Should(Equal(v.moduleCompType))
				Expect(mod.DependsOn).Should(ConsistOf(v.addonSlug+"-resources", "team-config", "addon-auxiliaries"))
				// Divergent spot 1/2: an external module component carries its
				// {module, registry, version} request; an inline one has nothing
				// to request -- it wraps the already-rendered owned Application
				// directly, so its properties are {objects: [<that Application>]}.
				if v.label == "external" {
					Expect(propertiesOf(mod.Properties)).Should(Equal(map[string]interface{}{
						"module": v.moduleSlug, "registry": moduleRegistryName, "version": "1.0.0",
					}))
				} else {
					objs := propertiesOf(mod.Properties)["objects"].([]interface{})
					Expect(objs).Should(HaveLen(1))
					owned := objs[0].(map[string]interface{})
					Expect(owned["kind"]).Should(Equal("Application"))
					Expect(owned["metadata"].(map[string]interface{})["name"]).Should(Equal(v.moduleAppName()))
				}

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
				// Divergent spot 2/2: type: module has its own healthPolicy/
				// customStatus reading the owned Application's own service
				// health into a "Ready:N/M" message (module.cue). k8s-objects
				// (vela-templates/definitions/internal/component/k8s-objects.cue)
				// has no such policy -- it is a plain passthrough -- so an
				// inline module reports generic health only, not an aggregated
				// count. This is a known, currently-accepted gap between the
				// two paths, not a test bug: see the design doc's health note.
				if v.label == "external" {
					Expect(findService(app, v.moduleSlug).Message).Should(Equal("Ready:5/5"))
				}
			})

			It("level 2: the addon's own objects, parameters and definition are installed", func() {
				Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: v.addonSlug + "-system"}, &corev1.Namespace{})).Should(Succeed())

				cm, err := getConfigMap(ctx, systemNS, v.addonSlug+"-config")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(cm.Data).Should(HaveKeyWithValue("source", "resources/platform-config.yaml"))

				notes, err := getConfigMap(ctx, systemNS, v.addonSlug+"-notes")
				Expect(err).ShouldNot(HaveOccurred(), "template.cue outputs become the addon-auxiliaries component")
				Expect(notes.Data).Should(HaveKeyWithValue("greeting", "hello from scenario 02"))
				Expect(notes.Data).Should(HaveKeyWithValue("addonVersion", "1.0.0"))

				info, err := getConfigMap(ctx, v.addonSlug+"-system", v.addonSlug+"-info")
				Expect(err).ShouldNot(HaveOccurred(), "a package main resource file is merged into template.cue")
				Expect(info.Data).Should(HaveKeyWithValue("greeting", "hello from scenario 02"))

				team, err := getConfigMap(ctx, v.addonSlug+"-system", v.addonSlug+"-team")
				Expect(err).ShouldNot(HaveOccurred(), "a resource .cue file without a package header renders as its own component")
				Expect(team.Data).Should(HaveKeyWithValue("team", "team-blue"))

				var secret corev1.Secret
				Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Namespace: systemNS, Name: "addon-secret-" + v.addonSlug}, &secret)).Should(Succeed())
				Expect(string(secret.Data["addonParameterDataKey"])).Should(SatisfyAll(
					ContainSubstring(`"greeting":"hello from scenario 02"`), ContainSubstring(`"team":"team-blue"`)))

				owner, err := getTraitDefinition(ctx, systemNS, v.ownerTraitName)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(owner.Labels).ShouldNot(HaveKey(veltypes.LabelDefinitionModule), "an addon-level definition carries no module labels")
			})

			It("level 3: the module Application has one tier per level and enabled line, in dependency order", func() {
				app := mustGetApp(ctx, systemNS, v.moduleAppName())
				Expect(app.Labels).Should(HaveKeyWithValue(veltypes.LabelDefinitionModule, v.moduleSlug))
				Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAppName, v.addonAppName()))
				Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAppComponent, v.moduleSlug))
				Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAddonName, v.addonSlug), "the addon Application's labels are merged onto everything it dispatches")
				Expect(app.Annotations).Should(HaveKeyWithValue(veltypes.AnnoDefinitionModuleVersion, "1.0.0"))

				Expect(componentNames(app)).Should(Equal(v.tiers))
				Expect(findComponent(app, v.moduleSlug+"-aux").DependsOn).Should(BeEmpty())
				Expect(findComponent(app, v.moduleSlug+"-v1-aux").DependsOn).Should(Equal([]string{v.moduleSlug + "-aux"}))
				Expect(findComponent(app, v.moduleSlug+"-v1-defs").DependsOn).Should(Equal([]string{v.moduleSlug + "-v1-aux"}))
				Expect(findComponent(app, v.moduleSlug+"-v2-aux").DependsOn).Should(Equal([]string{v.moduleSlug + "-aux"}), "lines are siblings: v2 never waits for v1")
				Expect(findComponent(app, v.moduleSlug+"-v2-defs").DependsOn).Should(Equal([]string{v.moduleSlug + "-v2-aux"}))
				for _, c := range app.Spec.Components {
					Expect(c.Type).Should(Equal("k8s-objects"), c.Name)
				}
			})

			It("level 3: the module installed its CRDs, auxiliary objects and stamped definitions, and nothing from the disabled line", func() {
				By("module-level auxiliary objects: two CRDs from one multi-document file, a ConfigMap from CUE, a ClusterRole")
				Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "widgets." + v.crdGroup}, crd("widgets."+v.crdGroup))).Should(Succeed())
				Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "widgetclasses." + v.crdGroup}, crd("widgetclasses."+v.crdGroup))).Should(Succeed())
				Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: v.moduleSlug + "-viewer"}, &rbacv1.ClusterRole{})).Should(Succeed())
				info, err := getConfigMap(ctx, systemNS, v.moduleSlug+"-module-info")
				Expect(err).ShouldNot(HaveOccurred(), "an auxiliary object without a namespace lands in the definition namespace")
				Expect(info.Data).Should(HaveKeyWithValue("moduleVersion", "1.0.0"))

				By("line-level auxiliary objects: a WidgetClass and a ConfigMap per enabled line")
				widgetClassGVK := schema.GroupVersionKind{Group: v.crdGroup, Version: "v1alpha1", Kind: "WidgetClass"}
				for _, line := range []string{"v1", "v2"} {
					className := map[string]string{"v1": v.moduleSlug + "-v1-standard", "v2": v.moduleSlug + "-v2-premium"}[line]
					class, err := getUnstructured(ctx, widgetClassGVK, "", className)
					Expect(err).ShouldNot(HaveOccurred(), className)
					Expect(class.GetLabels()).Should(HaveKeyWithValue(v.crdGroup+"/line", line))
					_, err = getConfigMap(ctx, systemNS, v.moduleSlug+"-"+line+"-line-config")
					Expect(err).ShouldNot(HaveOccurred())
				}

				By("five stamped definitions with module labels")
				Expect(moduleDefinitionNames(ctx, systemNS, v.moduleSlug)).Should(ConsistOf(v.definitions))
				cd, err := getComponentDefinition(ctx, systemNS, v.moduleSlug+"-v1-widget")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(cd.Labels).Should(HaveKeyWithValue(veltypes.LabelDefinitionModuleAPIVersion, "v1"))
				Expect(cd.Labels).Should(HaveKeyWithValue(veltypes.LabelDefinitionName, "widget"))
				Expect(cd.Annotations).Should(HaveKeyWithValue(veltypes.AnnoDefinitionModuleFullName, v.moduleSlug+"-v1-widget"))
				// stampIdentity (pkg/module/service/render.go) writes
				// addons.oam.dev/name=<module> into the manifest, but that is not what
				// reaches the cluster on this path. An Application merges its own
				// labels onto every object it dispatches
				// (generateAndFilterCommonLabels in pkg/appfile/appfile.go), so the
				// addons.oam.dev/{name,version,registry} labels the renderer put on
				// the addon Application travel onto the module Application and from
				// there onto the definitions, overriding the module's stamp. A module
				// installed through "vela module deploy" keeps the module name here;
				// one installed through an addon (inline or external alike) is
				// grouped under that addon.
				Expect(cd.Labels).Should(HaveKeyWithValue(oam.LabelAddonName, v.addonSlug))
				Expect(cd.Labels).Should(HaveKeyWithValue(oam.LabelAddonVersion, "1.0.0"))
				Expect(cd.Labels).Should(HaveKeyWithValue(oam.LabelAddonRegistry, addonRegistryName))
				v2, err := getComponentDefinition(ctx, systemNS, v.moduleSlug+"-v2-widget")
				Expect(err).ShouldNot(HaveOccurred(), "a YAML definition installs like a CUE one")
				Expect(v2.Labels).Should(HaveKeyWithValue(veltypes.LabelDefinitionModuleAPIVersion, "v2"))

				By("the disabled v1beta1 line leaving nothing behind")
				Expect(isNotFound(ctx, componentDefinitionObj(systemNS, v.moduleSlug+"-v1beta1-widget"))).Should(BeTrue())
				Expect(isNotFound(ctx, configMapObj(systemNS, v.moduleSlug+"-v1beta1-preview"))).Should(BeTrue())
			})

			It("a consumer uses Form 3, Form 2, a module trait and the addon-level trait", func() {
				Expect(applyManifestFile(ctx, testdataPath("apps", v.consumerFile))).Should(Succeed())
				waitAppRunning(ctx, testNS, v.consumerApp, shortWait)

				widgetGVK := schema.GroupVersionKind{Group: v.crdGroup, Version: "v1alpha1", Kind: "Widget"}
				blue, err := getUnstructured(ctx, widgetGVK, testNS, "blue-widget")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(nestedString(blue, "spec", "classRef")).Should(Equal(v.moduleSlug + "-v1-standard"))
				Expect(nestedString(blue, "spec", "apiLine")).Should(Equal("v1"))
				Expect(nestedString(blue, "spec", "color")).Should(Equal("blue"))
				Expect(nestedString(blue, "spec", "replicas")).Should(Equal("2"))
				Expect(blue.GetLabels()).Should(HaveKeyWithValue(v.crdGroup+"/tier", "gold"))
				Expect(blue.GetLabels()).Should(HaveKeyWithValue(v.crdGroup+"/labeled-by", v.moduleSlug+"-v1-labeler"))
				Expect(blue.GetLabels()).Should(HaveKeyWithValue(oam.WorkloadTypeLabel, v.moduleSlug+"-v1-widget"),
					"the workload type label carries the resolved name, not the "+v.moduleSlug+"/v1/widget spelling")
				Expect(blue.GetAnnotations()).Should(HaveKeyWithValue(v.crdGroup+"/owner", "team-blue"))

				big, err := getUnstructured(ctx, widgetGVK, testNS, "big-widget")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(nestedString(big, "spec", "classRef")).Should(Equal(v.moduleSlug + "-v2-premium"))
				Expect(nestedString(big, "spec", "size")).Should(Equal("L"))
				Expect(big.GetLabels()).Should(HaveKeyWithValue(v.crdGroup+"/tier", "silver"))
				Expect(big.GetLabels()).Should(HaveKeyWithValue(v.crdGroup+"/line", "v2"))
				Expect(big.GetLabels()).Should(HaveKeyWithValue(v.crdGroup+"/labeled-by", v.moduleSlug+"-v2-labeler"))

				card, err := getConfigMap(ctx, testNS, "blue-widget-card")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(card.Data).Should(Equal(map[string]string{"color": "blue", "line": "v1", "module": v.moduleSlug}))
			})

			It("every level has its own ResourceTracker", func() {
				for _, name := range []string{v.appName, v.addonAppName(), v.moduleAppName(), v.consumerApp} {
					var trackers v1beta1.ResourceTrackerList
					Expect(k8sClient.List(ctx, &trackers, client.MatchingLabels{oam.LabelAppName: name})).Should(Succeed())
					Expect(trackers.Items).ShouldNot(BeEmpty(), "no ResourceTracker for %s", name)
				}
			})

			It("deleting the user Application cascades through every level", func() {
				widgetGVK := schema.GroupVersionKind{Group: v.crdGroup, Version: "v1alpha1", Kind: "Widget"}
				deleteApp(ctx, testNS, v.consumerApp)
				waitGone(ctx, unstructuredObj(widgetGVK, testNS, "blue-widget"), shortWait)

				uninstall(v.appName, v.addonAppName(), v.moduleAppName())
				waitGone(ctx, crd("widgetclasses."+v.crdGroup), shortWait)
				waitGone(ctx, componentDefinitionObj(systemNS, v.moduleSlug+"-v1-widget"), shortWait)
				waitGone(ctx, traitDefinitionObj(systemNS, v.ownerTraitName), shortWait)
				waitGone(ctx, configMapObj(systemNS, v.moduleSlug+"-module-info"), shortWait)
				waitGone(ctx, configMapObj(systemNS, v.addonSlug+"-notes"), shortWait)
				waitGone(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: v.addonSlug + "-system"}}, reconcileWait)
			})
		})
	}

	// --- Scenario 03 ---
	// Runs once per module source, same reasoning as scenario 02: everything
	// here -- fan-out into two module components, cross-module trait
	// ambiguity, Form 3/Form 1 resolution -- is resolution-layer behavior
	// that does not know or care whether a module arrived inline or
	// externally, so a divergence would be a real bug.
	for _, v := range []suiteVariant{externalSuiteVariant, inlineSuiteVariant} {
		v := v
		Context(fmt.Sprintf("one addon, two modules (scenario 03, %s)", v.label), func() {
			BeforeAll(func() {
				Expect(k8sClient.Create(ctx, addonApplication(v.appName, v.addonSlug, "1.0.0", nil))).Should(Succeed())
				waitAppRunning(ctx, testNS, v.appName, installWait)
				waitAppRunning(ctx, systemNS, "module-"+v.widgetModule, shortWait)
				waitAppRunning(ctx, systemNS, "module-"+v.gadgetModule, shortWait)
				DeferCleanup(func() {
					deleteApp(ctx, testNS, v.consumerApp)
					uninstall(v.appName, "addon-"+v.addonSlug, "module-"+v.widgetModule, "module-"+v.gadgetModule)
				})
			})

			It("fans one addon Application out into two module components and two module Applications", func() {
				app := mustGetApp(ctx, systemNS, "addon-"+v.addonSlug)
				Expect(componentNames(app)).Should(Equal([]string{v.addonSlug + "-resources", v.widgetModule, v.gadgetModule}))
				for _, name := range []string{v.widgetModule, v.gadgetModule} {
					c := findComponent(app, name)
					Expect(c.Type).Should(Equal(v.moduleCompType))
					Expect(c.DependsOn).Should(Equal([]string{v.addonSlug + "-resources"}), v.addonSlug+" has no outputs, so there is no addon-auxiliaries dependency")
					if v.label == "external" {
						Expect(propertiesOf(c.Properties)).Should(Equal(map[string]interface{}{"module": name, "registry": moduleRegistryName, "version": "1.0.0"}))
					}
				}
				if v.label == "external" {
					Expect(findService(app, v.widgetModule).Message).Should(Equal("Ready:5/5"))
					Expect(findService(app, v.gadgetModule).Message).Should(Equal("Ready:3/3"))
				}
				Expect(findService(mustGetApp(ctx, testNS, v.appName), v.addonSlug).Message).Should(Equal("Ready:3/3"))

				for _, name := range []string{v.widgetModule, v.gadgetModule} {
					mod := mustGetApp(ctx, systemNS, "module-"+name)
					Expect(mod.Labels).Should(HaveKeyWithValue(oam.LabelAppName, "addon-"+v.addonSlug))
					Expect(mod.Labels).Should(HaveKeyWithValue(oam.LabelAppComponent, name))
				}
			})

			It("installs the gadget module's three tiers and objects", func() {
				mod := mustGetApp(ctx, systemNS, "module-"+v.gadgetModule)
				Expect(componentNames(mod)).Should(Equal([]string{v.gadgetModule + "-aux", v.gadgetModule + "-v1-aux", v.gadgetModule + "-v1-defs"}))
				Expect(findComponent(mod, v.gadgetModule+"-v1-aux").DependsOn).Should(Equal([]string{v.gadgetModule + "-aux"}))
				Expect(findComponent(mod, v.gadgetModule+"-v1-defs").DependsOn).Should(Equal([]string{v.gadgetModule + "-v1-aux"}))
				Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "gadgets." + v.crdGroup}, crd("gadgets."+v.crdGroup))).Should(Succeed())
				Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: v.gadgetModule + "-editor"}, &rbacv1.ClusterRole{})).Should(Succeed())
				for _, cm := range []string{v.gadgetModule + "-v1-defaults", v.gadgetModule + "-v1-profile"} {
					_, err := getConfigMap(ctx, systemNS, cm)
					Expect(err).ShouldNot(HaveOccurred(), cm)
				}

				By("three traits sharing the short name labeler")
				var tds v1beta1.TraitDefinitionList
				Expect(k8sClient.List(ctx, &tds, client.MatchingLabels{veltypes.LabelDefinitionName: "labeler"})).Should(Succeed())
				var names []string
				for _, td := range tds.Items {
					names = append(names, td.Name)
				}
				Expect(names).Should(ConsistOf(v.gadgetModule+"-v1-labeler", v.widgetModule+"-v1-labeler", v.widgetModule+"-v2-labeler"))
			})

			It("rejects a Form 1 or Form 2 trait that both modules ship, at admission", func() {
				results := createEachFromFile(ctx, testdataPath("apps", v.ambiguousFile))
				Expect(results).Should(HaveLen(2))
				// The module list in the message follows the order the definitions
				// were listed in, which is not sorted, so each name is checked on
				// its own.
				for name, typeName := range map[string]string{v.ambiguousForm2App: "v1/labeler", v.ambiguousForm1App: "labeler"} {
					Expect(results[name]).Should(HaveOccurred(), name)
					Expect(results[name].Error()).Should(SatisfyAll(
						ContainSubstring(`type "`+typeName+`" is ambiguous: definitions from modules [`),
						ContainSubstring(v.gadgetModule),
						ContainSubstring(v.widgetModule),
						ContainSubstring("] all match"),
					), name)
				}
				for name := range results {
					Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS}})).Should(BeTrue(), name)
				}
			})

			It("resolves Form 3 always and Form 1 when the short name is unique", func() {
				widgetGVK := schema.GroupVersionKind{Group: v.crdGroup, Version: "v1alpha1", Kind: "Widget"}
				gadgetGVK := schema.GroupVersionKind{Group: v.crdGroup, Version: "v1alpha1", Kind: "Gadget"}
				Expect(applyManifestFile(ctx, testdataPath("apps", v.consumerFile))).Should(Succeed())
				waitAppRunning(ctx, testNS, v.consumerApp, shortWait)

				widget, err := getUnstructured(ctx, widgetGVK, testNS, "suite-widget")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(nestedString(widget, "spec", "color")).Should(Equal("green"))
				Expect(widget.GetLabels()).Should(HaveKeyWithValue(v.crdGroup+"/labeled-by", v.widgetModule+"-v1-labeler"))

				gadget, err := getUnstructured(ctx, gadgetGVK, testNS, "suite-gadget")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(nestedString(gadget, "spec", "mode")).Should(Equal("turbo"))
				Expect(gadget.GetLabels()).Should(HaveKeyWithValue(v.crdGroup+"/labeled-by", v.gadgetModule+"-v1-labeler"))
				// Only a type containing a slash is swapped for the installed name
				// (makeWorkloadWithContext, pkg/appfile/appfile.go). A Form 1 name
				// is already a valid label value and is kept as written.
				Expect(gadget.GetLabels()).Should(HaveKeyWithValue(oam.WorkloadTypeLabel, "gadget"), "Form 1 gadget keeps its short name in the label")
			})
		})
	}

	// --- Scenario 04 ---
	// Resolution-layer behavior again (Form 1/2/3 acceptance and rejection,
	// trait output labeling, the v2 contract check): none of it depends on
	// whether the module backing these definitions arrived inline or
	// externally, so this reuses widget-platform/widget-platform-inline's own
	// installed definitions and runs once per variant.
	for _, v := range []widgetVariant{externalWidgetVariant, inlineWidgetVariant} {
		v := v
		Context(fmt.Sprintf("type reference forms (scenario 04, %s)", v.label), func() {
			BeforeAll(func() {
				Expect(k8sClient.Create(ctx, addonApplication(v.appName, v.addonSlug, "1.0.0", nil))).Should(Succeed())
				waitAppRunning(ctx, testNS, v.appName, installWait)
				Expect(moduleDefinitionNames(ctx, systemNS, v.moduleSlug)).Should(ConsistOf(v.definitions))
				DeferCleanup(func() {
					for _, name := range []string{"forms-accepted" + v.suffix, "v2-contract" + v.suffix, "trait-outputs-form3" + v.suffix} {
						deleteApp(ctx, testNS, name)
					}
					uninstall(v.appName, v.addonAppName(), v.moduleAppName())
				})
			})

			It("without addon parameters there is no addon-secret component", func() {
				Expect(componentNames(mustGetApp(ctx, systemNS, v.addonAppName()))).ShouldNot(ContainElement("addon-secret"))
				Expect(findService(mustGetApp(ctx, testNS, v.appName), v.addonSlug).Message).Should(Equal("Ready:6/6"))
			})

			It("accepts Form 3, Form 2, the installed name and a unique Form 1 trait", func() {
				widgetGVK := schema.GroupVersionKind{Group: v.crdGroup, Version: "v1alpha1", Kind: "Widget"}
				Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-forms-accepted"+v.suffix+".yaml"))).Should(Succeed())
				waitAppRunning(ctx, testNS, "forms-accepted"+v.suffix, shortWait)
				for name, want := range map[string][2]string{
					"form3-widget":   {"v1", v.moduleSlug + "-v1-widget"},
					"form2-widget":   {"v2", v.moduleSlug + "-v2-widget"},
					"stamped-widget": {"v1", v.moduleSlug + "-v1-widget"},
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
				Expect(note.Labels).Should(HaveKeyWithValue(oam.TraitTypeLabel, v.moduleSlug+"-v1-note"))
			})

			It("refuses ambiguous, disabled-line and malformed references at admission", func() {
				results := createEachFromFile(ctx, testdataPath("apps", "consumer-rejected"+v.suffix+".yaml"))
				want := map[string]string{
					"reject-form1-ambiguous" + v.suffix:     `type "widget" is ambiguous`,
					"reject-disabled-line" + v.suffix:       v.moduleSlug + "-v1beta1-widget",
					"reject-form2-disabled-line" + v.suffix: `no definition found for type "v1beta1/widget"`,
					"reject-bad-api-version" + v.suffix:     `"1" is not a valid API version`,
					"reject-too-many-segments" + v.suffix:   "expected 1 segment (name), 2 segments (v<N>/name), or 3 segments (module/v<N>/name)",
				}
				Expect(results).Should(HaveLen(len(want)))
				for name, fragment := range want {
					Expect(results[name]).Should(HaveOccurred(), "%s must be denied", name)
					Expect(results[name].Error()).Should(ContainSubstring(fragment), name)
					Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS}})).Should(BeTrue(), name)
				}
			})

			It("holds a consumer to the v2 contract: size is required, color does not exist", func() {
				widgetGVK := schema.GroupVersionKind{Group: v.crdGroup, Version: "v1alpha1", Kind: "Widget"}
				err := applyManifestFile(ctx, testdataPath("apps", "consumer-v2-contract"+v.suffix+".yaml"))
				if err != nil {
					// With EnableCueValidation the webhook already refuses it.
					Expect(err.Error()).Should(ContainSubstring("size"))
					return
				}
				waitAppStatusContains(ctx, testNS, "v2-contract"+v.suffix, "size", shortWait)
				Expect(mustGetApp(ctx, testNS, "v2-contract"+v.suffix).Status.Phase).ShouldNot(Equal(common.ApplicationRunning))
				Expect(isNotFound(ctx, unstructuredObj(widgetGVK, testNS, "w"))).Should(BeTrue(), "the render must stop before creating the Widget")
			})

			It("labels the output of a Form 3 trait with the installed name", func() {
				// A trait written as <module>/v1/note used to stamp that string
				// into trait.oam.dev/type, which is not a valid label value. Trait
				// outputs of a module trait now carry the installed name.
				Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-trait-outputs-form3"+v.suffix+".yaml"))).Should(Succeed())
				waitAppRunning(ctx, testNS, "trait-outputs-form3"+v.suffix, shortWait)
				cm, err := getConfigMap(ctx, testNS, "noted-widget-note")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(cm.Labels).Should(HaveKeyWithValue(oam.TraitTypeLabel, v.moduleSlug+"-v1-note"))
			})
		})
	}

	// --- Scenario 05 ---
	Context("_imports.cue options and defaults (scenario 05)", func() {
		BeforeAll(func() {
			Expect(k8sClient.Create(ctx, addonApplication("import-options", "import-options", "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, "import-options", installWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
			DeferCleanup(func() {
				uninstall("import-options", "addon-import-options", moduleWidgetKit)
				Expect(isNotFound(ctx, configMapObj(systemNS, "import-options-marker"))).Should(BeTrue())
			})
		})

		It("generates one component per enabled import, without a registry when none is named, with a -2 suffix on a name collision", func() {
			app := mustGetApp(ctx, systemNS, "addon-import-options")
			Expect(componentNames(app)).Should(Equal([]string{"widget-kit", "import-options-resources", "widget-kit-2"}))
			Expect(findComponent(app, "widget-kit").Type).Should(Equal("k8s-objects"), "the template's own component keeps the name")
			mod := findComponent(app, "widget-kit-2")
			Expect(mod.Type).Should(Equal("module"))
			Expect(mod.DependsOn).Should(Equal([]string{"import-options-resources"}))
			Expect(propertiesOf(mod.Properties)).Should(Equal(map[string]interface{}{"module": "widget-kit", "version": "1.0.0"}))

			Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: moduleGadgetKit, Namespace: systemNS}})).Should(BeTrue(),
				"enabled: false produces no component at all")
			Expect(mustGetApp(ctx, systemNS, moduleWidgetKit).Labels).Should(HaveKeyWithValue(oam.LabelAppComponent, "widget-kit-2"))
		})

		It("does not enforce the versions filter: every enabled line installs", func() {
			Expect(componentNames(mustGetApp(ctx, systemNS, moduleWidgetKit))).Should(Equal(widgetKitTiers))
			_, err := getComponentDefinition(ctx, systemNS, "widget-kit-v2-widget")
			Expect(err).ShouldNot(HaveOccurred(), "versions: [v1] was requested, v2 installs anyway")
		})

		It("applies the default module registry rule: sole registry, else one named catalog, else an error", func() {
			runVelaSucceed("module", "registry", "add", "spare-modules", moduleRegistry.cluster, "--type", "oci")
			DeferCleanup(func() {
				_, _ = runVela("module", "registry", "delete", "spare-modules")
				_, _ = runVela("module", "registry", "delete", "catalog")
			})

			By("two registries and none named catalog: the next health-check render fails, nothing is removed")
			Eventually(func(g Gomega) {
				svc := findService(getAppG(g, ctx, systemNS, "addon-import-options"), "widget-kit-2")
				g.Expect(svc).ShouldNot(BeNil())
				g.Expect(svc.Healthy).Should(BeFalse())
				g.Expect(svc.Message).Should(ContainSubstring(`several module registries are configured and none is named "catalog"`))
			}, reconcileWait, pollInterval).Should(Succeed())
			waitAppStatusContains(ctx, testNS, "import-options", "widget-kit-2 unhealthy", reconcileWait)
			_, err := getComponentDefinition(ctx, systemNS, "widget-kit-v1-widget")
			Expect(err).ShouldNot(HaveOccurred(), "a failed render does not garbage-collect")

			By("a registry named catalog makes the empty registry resolve again")
			runVelaSucceed("module", "registry", "add", "catalog", moduleRegistry.cluster, "--type", "oci")
			Eventually(func(g Gomega) {
				svc := findService(getAppG(g, ctx, systemNS, "addon-import-options"), "widget-kit-2")
				g.Expect(svc).ShouldNot(BeNil())
				g.Expect(svc.Healthy).Should(BeTrue(), svc.Message)
				g.Expect(svc.Message).Should(Equal("Ready:5/5"))
			}, reconcileWait, pollInterval).Should(Succeed())
		})
	})

	// --- Scenario 06 ---
	Context("a type: module component declared in template.cue with a tenant namespace (scenario 06)", func() {
		BeforeAll(func() {
			Expect(k8sClient.Create(ctx, addonApplication("tenant-widgets", "tenant-widgets", "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, "tenant-widgets", installWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
			DeferCleanup(func() {
				deleteApp(ctx, "kit-tenant", "tenant-consumer")
				waitGone(ctx, unstructuredObj(widgetGVK, "kit-tenant", "tenant-widget"), shortWait)
				uninstall("tenant-widgets", "addon-tenant-widgets", moduleWidgetKit)
				waitGone(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kit-tenant"}}, reconcileWait)
			})
		})

		It("lets the template's module component win over the matching _imports.cue entry", func() {
			app := mustGetApp(ctx, systemNS, "addon-tenant-widgets")
			Expect(componentNames(app)).Should(ConsistOf("tenant-kit", "tenant-widgets-resources"))
			Expect(componentNames(app)).ShouldNot(ContainElement("widget-kit"))
			mod := findComponent(app, "tenant-kit")
			Expect(mod.Type).Should(Equal("module"))
			Expect(mod.DependsOn).Should(Equal([]string{"tenant-widgets-resources"}), "a hand-written module component gets no automatic dependsOn")
			Expect(propertiesOf(mod.Properties)).Should(Equal(map[string]interface{}{
				"module": "widget-kit", "namespace": "kit-tenant", "registry": moduleRegistryName, "version": "1.0.0",
			}))
			Expect(mustGetApp(ctx, systemNS, moduleWidgetKit).Labels).Should(HaveKeyWithValue(oam.LabelAppComponent, "tenant-kit"))
		})

		It("installs definitions and namespaced auxiliary objects into the tenant namespace only", func() {
			Expect(moduleDefinitionNames(ctx, "kit-tenant", "widget-kit")).Should(ConsistOf(widgetKitDefinitions))
			Expect(moduleDefinitionNames(ctx, systemNS, "widget-kit")).Should(BeEmpty())
			for _, cm := range []string{"widget-kit-module-info", "widget-kit-v1-line-config", "widget-kit-v2-line-config"} {
				_, err := getConfigMap(ctx, "kit-tenant", cm)
				Expect(err).ShouldNot(HaveOccurred(), cm)
				Expect(isNotFound(ctx, configMapObj(systemNS, cm))).Should(BeTrue(), cm)
			}
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: widgetsCRD}, crd(widgetsCRD))).Should(Succeed(), "cluster-scoped objects are unaffected")
			_, err := getUnstructured(ctx, widgetClassGVK, "", "widget-kit-v1-standard")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "widget-kit-viewer"}, &rbacv1.ClusterRole{})).Should(Succeed())
		})

		It("is usable from the tenant namespace and refused from another", func() {
			Expect(applyManifestFile(ctx, testdataPath("apps", "consumer-tenant.yaml"))).Should(Succeed())
			waitAppRunning(ctx, "kit-tenant", "tenant-consumer", shortWait)
			w, err := getUnstructured(ctx, widgetGVK, "kit-tenant", "tenant-widget")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(nestedString(w, "spec", "classRef")).Should(Equal("widget-kit-v1-standard"))
			Expect(w.GetLabels()).Should(HaveKeyWithValue("kit.example.com/labeled-by", "widget-kit-v1-labeler"))

			results := createEachFromFile(ctx, testdataPath("apps", "consumer-default-tenant.yaml"))
			Expect(results).Should(HaveLen(2))
			// The friendly "ensure the module is installed" text comes from the
			// definition permission check, which only runs with the alpha
			// ValidateDefinitionPermissions gate. Without it the render refuses
			// the Application with the template loader's error, so only the
			// installed name and "not found" are checked.
			for name, err := range results {
				Expect(err).Should(HaveOccurred(), "%s must be denied from the default namespace", name)
				Expect(err.Error()).Should(SatisfyAll(
					ContainSubstring(`"widget-kit-v1-widget"`),
					ContainSubstring("not found"),
				), name)
				Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS}})).Should(BeTrue(), name)
			}
		})
	})

	// --- Scenario 08 (before 07: it needs 1.0.0 to be the highest tag) ---
	Context("latest vs pinned module version (scenario 08)", func() {
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
			Expect(ociTags(moduleRegistry.hostBase, "modules/widget-kit")).Should(ConsistOf("1.0.0"), "widget-kit 1.0.0 must be the only published tag here")
			Expect(k8sClient.Create(ctx, addonApplication("widget-latest", "widget-latest", "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, "widget-latest", installWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
			DeferCleanup(func() {
				annotateApp(ctx, systemNS, "addon-widget-latest", oam.AnnotationWorkflowRestart, "")
				uninstall("widget-latest", "addon-widget-latest", moduleWidgetKit)
			})
		})

		It("resolves an unpinned import to the highest tag at install", func() {
			app := mustGetApp(ctx, systemNS, "addon-widget-latest")
			Expect(propertiesOf(findComponent(app, "widget-kit").Properties)).Should(Equal(map[string]interface{}{"module": "widget-kit", "registry": moduleRegistryName}),
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
			Expect(ociTags(moduleRegistry.hostBase, "modules/widget-kit")).Should(ConsistOf("1.0.0", "1.1.0"))
			Consistently(func(g Gomega) {
				g.Expect(moduleVersion()).Should(Equal("1.0.0"))
				g.Expect(isNotFound(ctx, componentDefinitionObj(systemNS, "widget-kit-v1-gauge"))).Should(BeTrue())
				for _, svc := range mustGetApp(ctx, systemNS, "addon-widget-latest").Status.Services {
					g.Expect(svc.Healthy).Should(BeTrue(), "%s: %s", svc.Name, svc.Message)
				}
			}, 80*time.Second, 5*time.Second).Should(Succeed(), "one full resync (reSyncPeriod=1m) must not dispatch the new version")
		})

		It("does not move when the user Application's workflow re-runs: the addon render is unchanged", func() {
			restartWorkflow(ctx, testNS, "widget-latest", "true")
			Eventually(func(g Gomega) string {
				return getAppG(g, ctx, testNS, "widget-latest").Annotations[oam.AnnotationWorkflowRestart]
			}, shortWait, pollInterval).Should(BeEmpty(), "a one-shot restart annotation is removed once used")
			waitAppRunning(ctx, testNS, "widget-latest", shortWait)
			Consistently(moduleVersion, 45*time.Second, 5*time.Second).Should(Equal("1.0.0"))
		})

		It("moves when the Application that renders the module component re-runs its workflow", func() {
			restartWorkflow(ctx, systemNS, "addon-widget-latest", "true")
			Eventually(moduleVersion, reconcileWait, pollInterval).Should(Equal("1.1.0"))
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
			Eventually(func() error {
				_, err := getComponentDefinition(ctx, systemNS, "widget-kit-v1-gauge")
				return err
			}, shortWait, pollInterval).Should(Succeed())
			Expect(mustGetApp(ctx, systemNS, "addon-widget-latest").Annotations).ShouldNot(HaveKey(oam.AnnotationWorkflowRestart))
		})

		It("follows the registry with a recurring restart, including a publish that switches a line off", func() {
			restartWorkflow(ctx, systemNS, "addon-widget-latest", "2m")
			publishModuleFixture("widget-kit-1.2.0")
			Eventually(moduleVersion, 6*time.Minute, 10*time.Second).Should(Equal("1.2.0"))
			Eventually(func(g Gomega) []string {
				return componentNames(getAppG(g, ctx, systemNS, moduleWidgetKit))
			}, shortWait, pollInterval).Should(Equal([]string{"widget-kit-aux", "widget-kit-v1-aux", "widget-kit-v1-defs"}))
			waitGone(ctx, componentDefinitionObj(systemNS, "widget-kit-v2-widget"), reconcileWait)
			Expect(mustGetApp(ctx, systemNS, "addon-widget-latest").Annotations).Should(HaveKeyWithValue(oam.AnnotationWorkflowRestart, "2m"), "a duration is recurring and stays")
		})
	})

	// --- Scenario 07 ---
	for _, v := range []widgetVariant{externalWidgetVariant, inlineWidgetVariant} {
		v := v
		Context(fmt.Sprintf("upgrade the addon and so the module, add a definition, remove an API line, roll back, %s module (scenario 07)", v.label), func() {
			widgetGVK := schema.GroupVersionKind{Group: v.crdGroup, Version: "v1alpha1", Kind: "Widget"}
			widgetClassGVK := schema.GroupVersionKind{Group: v.crdGroup, Version: "v1alpha1", Kind: "WidgetClass"}
			v1OnlyTiers := append([]string{}, v.tiers[:3]...)
			gaugeDef := v.moduleSlug + "-v1-gauge"
			moduleInfo := v.moduleSlug + "-module-info"
			v1LineConfig := v.moduleSlug + "-v1-line-config"
			v2WidgetDef := v.moduleSlug + "-v2-widget"
			v2LabelerDef := v.moduleSlug + "-v2-labeler"
			v2PremiumCR := v.moduleSlug + "-v2-premium"
			v2LineConfig := v.moduleSlug + "-v2-line-config"
			gaugeConfigMap := "fuel-gauge" + v.suffix

			moduleState := func(g Gomega, version string, tiers []string) {
				mod := mustGetApp(ctx, systemNS, v.moduleAppName())
				g.Expect(mod.Annotations).Should(HaveKeyWithValue(veltypes.AnnoDefinitionModuleVersion, version))
				g.Expect(componentNames(mod)).Should(Equal(tiers))
			}

			BeforeAll(func() {
				if v.label == "external" {
					Expect(ociTags(moduleRegistry.hostBase, "modules/widget-kit")).Should(ConsistOf("1.0.0", "1.1.0", "1.2.0"))
				}
				Expect(k8sClient.Create(ctx, addonApplication(v.appName, v.addonSlug, "1.0.0", nil))).Should(Succeed())
				waitAppRunning(ctx, testNS, v.appName, installWait)
				Expect(applyManifestFile(ctx, testdataPath("apps", v.v2ConsumerFile))).Should(Succeed())
				waitAppRunning(ctx, testNS, v.v2ConsumerApp, shortWait)
				DeferCleanup(func() {
					deleteApp(ctx, testNS, v.gaugeConsumerApp)
					deleteApp(ctx, testNS, v.v2ConsumerApp)
					waitAppGone(ctx, testNS, v.gaugeConsumerApp, reconcileWait)
					waitAppGone(ctx, testNS, v.v2ConsumerApp, reconcileWait)
					uninstall(v.appName, v.addonAppName(), v.moduleAppName())
				})
			})

			It("starts from 1.0.0 with both lines", func() {
				Eventually(func(g Gomega) { moduleState(g, "1.0.0", v.tiers) }, shortWait, pollInterval).Should(Succeed())
				Expect(moduleDefinitionNames(ctx, systemNS, v.moduleSlug)).Should(ConsistOf(v.definitions))
			})

			It("1.1.0 adds a definition and updates auxiliary objects in place", func() {
				setAddonVersion(ctx, v.appName, "1.1.0")
				Eventually(func(g Gomega) { moduleState(g, "1.1.0", v.tiers) }, installWait, pollInterval).Should(Succeed())
				waitAppRunning(ctx, testNS, v.appName, installWait)
				Expect(mustGetApp(ctx, systemNS, v.addonAppName()).Labels).Should(HaveKeyWithValue(oam.LabelAddonVersion, "1.1.0"))
				Expect(moduleDefinitionNames(ctx, systemNS, v.moduleSlug)).Should(ConsistOf(append([]string{gaugeDef}, v.definitions...)))
				info, err := getConfigMap(ctx, systemNS, moduleInfo)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(info.Data).Should(HaveKeyWithValue("moduleVersion", "1.1.0"))
				line, err := getConfigMap(ctx, systemNS, v1LineConfig)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(line.Data).Should(HaveKeyWithValue("configRevision", "2"))

				Expect(applyManifestFile(ctx, testdataPath("apps", v.gaugeConsumerFile))).Should(Succeed())
				waitAppRunning(ctx, testNS, v.gaugeConsumerApp, shortWait)
				gauge, err := getConfigMap(ctx, testNS, gaugeConfigMap)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(gauge.Data).Should(Equal(map[string]string{"line": "v1", "module": v.moduleSlug, "reading": "80", "since": "1.1.0"}))
			})

			It("1.2.0 switches the v2 line off: its tiers, definitions and auxiliary objects are garbage-collected, a consumer's Widget is not", func() {
				setAddonVersion(ctx, v.appName, "1.2.0")
				Eventually(func(g Gomega) {
					moduleState(g, "1.2.0", v1OnlyTiers)
				}, installWait, pollInterval).Should(Succeed())
				waitAppRunning(ctx, testNS, v.appName, installWait)
				waitGone(ctx, componentDefinitionObj(systemNS, v2WidgetDef), reconcileWait)
				waitGone(ctx, traitDefinitionObj(systemNS, v2LabelerDef), shortWait)
				waitGone(ctx, unstructuredObj(widgetClassGVK, "", v2PremiumCR), shortWait)
				waitGone(ctx, configMapObj(systemNS, v2LineConfig), shortWait)
				info, err := getConfigMap(ctx, systemNS, moduleInfo)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(info.Data).Should(HaveKeyWithValue("servedLines", "v1"))
				Expect(info.Data).Should(HaveKeyWithValue("disabledLines", "v1beta1,v2"))

				_, err = getUnstructured(ctx, widgetGVK, testNS, "premium-widget")
				Expect(err).ShouldNot(HaveOccurred(), "nothing deletes a consumer's already-applied objects")

				By("the v2 consumer cannot be updated, and fails to parse on its next reconcile")
				expectConsumerOfRemovedDefinition(ctx, v.v2ConsumerApp, v2WidgetDef, "premium-widget", "after-1.2.0")
			})

			It("rolling back to 1.0.0 brings v2 back and drops the gauge", func() {
				setAddonVersion(ctx, v.appName, "1.0.0")
				Eventually(func(g Gomega) { moduleState(g, "1.0.0", v.tiers) }, installWait, pollInterval).Should(Succeed())
				waitAppRunning(ctx, testNS, v.appName, installWait)
				Eventually(func() []string { return moduleDefinitionNames(ctx, systemNS, v.moduleSlug) }, reconcileWait, pollInterval).Should(ConsistOf(v.definitions))

				bumpPublishVersion(ctx, testNS, v.v2ConsumerApp, "after-rollback")
				waitAppRunning(ctx, testNS, v.v2ConsumerApp, reconcileWait)

				// The gauge definition only exists from 1.1.0, so the gauge
				// consumer is now in the position the v2 consumer was in on 1.2.0.
				expectConsumerOfRemovedDefinition(ctx, v.gaugeConsumerApp, gaugeDef, "", "after-rollback")
				_, err := getConfigMap(ctx, testNS, gaugeConfigMap)
				Expect(err).ShouldNot(HaveOccurred(), "nothing deletes a consumer's already-applied objects")
			})
		})
	}

	// --- Scenario 09 ---
	Context("re-pushing the same tag: revision cache vs pinned render cache (scenario 09)", func() {
		type builds struct{ addon, module, line, def string }
		read := func() builds {
			var b builds
			if cm, err := getConfigMap(ctx, systemNS, "cache-probe-build"); err == nil {
				b.addon = cm.Data["build"]
			}
			if cm, err := getConfigMap(ctx, systemNS, "probe-kit-build"); err == nil {
				b.module = cm.Data["build"]
			}
			if cm, err := getConfigMap(ctx, systemNS, "probe-kit-v1-line"); err == nil {
				b.line = cm.Data["build"]
			}
			if cd, err := getComponentDefinition(ctx, systemNS, "probe-kit-v1-probe"); err == nil {
				b.def = cd.Annotations[veltypes.AnnoDefinitionDescription]
			}
			return b
		}
		buildA := builds{addon: "a", module: "a", line: "a", def: "probe-kit v1 probe (build a)."}

		BeforeAll(func() {
			By("starting from an empty addon render cache")
			restartVelaCore(ctx)
			Expect(k8sClient.Create(ctx, addonApplication("cache-probe", "cache-probe", "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, "cache-probe", installWait)
			waitAppRunning(ctx, systemNS, moduleProbeKit, shortWait)
			DeferCleanup(func() {
				uninstall("cache-probe", "addon-cache-probe", moduleProbeKit)
				Expect(isNotFound(ctx, configMapObj(systemNS, "cache-probe-build"))).Should(BeTrue())
				Expect(isNotFound(ctx, configMapObj(systemNS, "probe-kit-build"))).Should(BeTrue())
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
			restartWorkflow(ctx, systemNS, "addon-cache-probe", "true")
			Eventually(read, reconcileWait, pollInterval).Should(Equal(builds{addon: "a", module: "b", line: "b", def: "probe-kit v1 probe (build b)."}))
			waitAppRunning(ctx, systemNS, moduleProbeKit, shortWait)

			restartWorkflow(ctx, testNS, "cache-probe", "true")
			Eventually(func(g Gomega) string {
				return getAppG(g, ctx, testNS, "cache-probe").Annotations[oam.AnnotationWorkflowRestart]
			}, shortWait, pollInterval).Should(BeEmpty())
			waitAppRunning(ctx, testNS, "cache-probe", shortWait)
			Consistently(func() string { return read().addon }, 45*time.Second, 5*time.Second).Should(Equal("a"), "the pinned addon render comes from vela-core's in-memory cache")
		})

		It("delivers the re-pushed addon only after a controller restart and a re-run of the user Application's workflow", func() {
			restartVelaCore(ctx)
			Expect(read().addon).Should(Equal("a"), "a restart alone dispatches nothing")
			restartWorkflow(ctx, testNS, "cache-probe", "true")
			Eventually(func() string { return read().addon }, reconcileWait, pollInterval).Should(Equal("b"))
			waitAppRunning(ctx, testNS, "cache-probe", shortWait)
			waitAppRunning(ctx, systemNS, "addon-cache-probe", shortWait)
		})
	})

	// --- Scenario 10 ---
	for _, v := range []suiteVariant{externalSuiteVariant, inlineSuiteVariant} {
		v := v
		Context(fmt.Sprintf("removing a module from an addon deletes its CRDs and every custom resource of them, %s modules (scenario 10)", v.label), func() {
			widgetGVK := schema.GroupVersionKind{Group: v.crdGroup, Version: "v1alpha1", Kind: "Widget"}
			gadgetGVK := schema.GroupVersionKind{Group: v.crdGroup, Version: "v1alpha1", Kind: "Gadget"}
			gadgetsCRD := "gadgets." + v.crdGroup
			addonAppName := "addon-" + v.addonSlug
			moduleWidgetApp := "module-" + v.widgetModule
			moduleGadgetApp := "module-" + v.gadgetModule
			gadgetDef := v.gadgetModule + "-v1-gadget"
			gadgetEditorRole := v.gadgetModule + "-editor"
			gadgetDefaults := v.gadgetModule + "-v1-defaults"

			BeforeAll(func() {
				Expect(k8sClient.Create(ctx, addonApplication(v.appName, v.addonSlug, "1.0.0", nil))).Should(Succeed())
				waitAppRunning(ctx, testNS, v.appName, installWait)
				waitAppRunning(ctx, systemNS, moduleGadgetApp, shortWait)
				Expect(applyManifestFile(ctx, testdataPath("apps", v.keepDoomedFile))).Should(Succeed())
				waitAppRunning(ctx, testNS, v.consumerApp, shortWait)
				DeferCleanup(func() {
					deleteApp(ctx, testNS, v.consumerApp)
					waitAppGone(ctx, testNS, v.consumerApp, reconcileWait)
					uninstall(v.appName, addonAppName, moduleWidgetApp, moduleGadgetApp)
				})
			})

			It("has one custom resource of each module before the upgrade", func() {
				_, err := getUnstructured(ctx, widgetGVK, testNS, "keep-widget")
				Expect(err).ShouldNot(HaveOccurred())
				_, err = getUnstructured(ctx, gadgetGVK, testNS, "doomed-gadget")
				Expect(err).ShouldNot(HaveOccurred())
			})

			It("2.0.0 drops the gadget module: module Application, definitions, auxiliary objects, CRD and Gadgets go; the widget module is untouched", func() {
				setAddonVersion(ctx, v.appName, "2.0.0")
				waitAppGone(ctx, systemNS, moduleGadgetApp, installWait)
				waitAppRunning(ctx, testNS, v.appName, installWait)
				Expect(componentNames(mustGetApp(ctx, systemNS, addonAppName))).Should(Equal([]string{v.addonSlug + "-resources", v.widgetModule}))
				waitGone(ctx, crd(gadgetsCRD), shortWait)
				Expect(isNotFound(ctx, componentDefinitionObj(systemNS, gadgetDef))).Should(BeTrue())
				Expect(isNotFound(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: gadgetEditorRole}})).Should(BeTrue())
				Expect(isNotFound(ctx, configMapObj(systemNS, gadgetDefaults))).Should(BeTrue())
				_, err := getUnstructured(ctx, widgetGVK, testNS, "keep-widget")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(mustGetApp(ctx, systemNS, moduleWidgetApp).Status.Phase).Should(Equal(common.ApplicationRunning))
			})

			It("leaves the consumer broken: its Gadget is gone with the kind and a new render finds no definition", func() {
				// Deleting the CRD deleted every Gadget, so unlike scenario 07 the
				// consumer's applied object of the removed module is gone too.
				expectConsumerOfRemovedDefinition(ctx, v.consumerApp, gadgetDef, "", "after-removal")
				Expect(isNotFound(ctx, crd(gadgetsCRD))).Should(BeTrue(), "the Gadget kind no longer exists")
				_, err := getUnstructured(ctx, widgetGVK, testNS, "keep-widget")
				Expect(err).ShouldNot(HaveOccurred(), "the other module's object is untouched")
			})

			It("downgrading to 1.0.0 brings the module, the CRD and, on the consumer's next run, the Gadget back", func() {
				setAddonVersion(ctx, v.appName, "1.0.0")
				waitAppRunning(ctx, systemNS, moduleGadgetApp, installWait)
				bumpPublishVersion(ctx, testNS, v.consumerApp, "after-restore")
				waitAppRunning(ctx, testNS, v.consumerApp, reconcileWait)
				_, err := getUnstructured(ctx, gadgetGVK, testNS, "doomed-gadget")
				Expect(err).ShouldNot(HaveOccurred(), "a new Gadget; the old one was deleted with the CRD")
			})
		})
	}

	// --- Scenario 11 ---
	for _, v := range []widgetVariant{externalWidgetVariant, inlineWidgetVariant} {
		v := v
		Context(fmt.Sprintf("deleting things by hand at every level: the Application one level up restores them, %s module (scenario 11)", v.label), func() {
			widgetGVK := schema.GroupVersionKind{Group: v.crdGroup, Version: "v1alpha1", Kind: "Widget"}
			widgetClassGVK := schema.GroupVersionKind{Group: v.crdGroup, Version: "v1alpha1", Kind: "WidgetClass"}
			widgetsCRD := "widgets." + v.crdGroup
			labelerDef := v.moduleSlug + "-v1-labeler"
			lineConfig := v.moduleSlug + "-v1-line-config"
			standardCR := v.moduleSlug + "-v1-standard"
			widgetDef := v.moduleSlug + "-v1-widget"

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
				Expect(k8sClient.Create(ctx, addonApplication(v.appName, v.addonSlug, "1.0.0", nil))).Should(Succeed())
				waitAppRunning(ctx, testNS, v.appName, installWait)
				Expect(applyManifestFile(ctx, testdataPath("apps", v.selfHealConsumerFile))).Should(Succeed())
				waitAppRunning(ctx, testNS, v.selfHealConsumerApp, shortWait)
				DeferCleanup(func() {
					deleteApp(ctx, testNS, v.selfHealConsumerApp)
					waitAppGone(ctx, testNS, v.selfHealConsumerApp, reconcileWait)
					uninstall(v.appName, v.addonAppName(), v.moduleAppName())
				})
			})

			It("recreates a deleted module definition with a new UID", func() {
				// module-widget-kit(-inline) inherits addons.oam.dev/name from the
				// addon Application, which on its own would make the resource
				// keeper treat it as apply-once and skip state keep; the module
				// renderer declares apply-once off to prevent that.
				mod := mustGetApp(ctx, systemNS, v.moduleAppName())
				Expect(mod.Labels).Should(HaveKey(oam.LabelAddonName))
				Expect(policyNames(mod)).Should(ContainElement("module-state-keep"))

				td := traitDefinitionObj(systemNS, labelerDef)
				before := uidOf(td)
				Expect(k8sClient.Delete(ctx, td)).Should(Succeed())
				waitBack(traitDefinitionObj(systemNS, labelerDef), reconcileWait)
				Expect(uidOf(traitDefinitionObj(systemNS, labelerDef))).ShouldNot(Equal(before))
			})

			It("reverts an edited line auxiliary ConfigMap", func() {
				cm, err := getConfigMap(ctx, systemNS, lineConfig)
				Expect(err).ShouldNot(HaveOccurred())
				cm.Data["configRevision"] = "tampered"
				Expect(k8sClient.Update(ctx, cm)).Should(Succeed())
				Eventually(func() string {
					got, err := getConfigMap(ctx, systemNS, lineConfig)
					if err != nil {
						return ""
					}
					return got.Data["configRevision"]
				}, reconcileWait, pollInterval).Should(Equal("1"))
			})

			It("recreates a deleted line auxiliary custom resource", func() {
				class := unstructuredObj(widgetClassGVK, "", standardCR)
				Expect(k8sClient.Delete(ctx, class)).Should(Succeed())
				waitBack(unstructuredObj(widgetClassGVK, "", standardCR), reconcileWait)
			})

			It("recreates a deleted module CRD, and the consumer's Widget comes back on its own next state keep", func() {
				Expect(k8sClient.Delete(ctx, crd(widgetsCRD))).Should(Succeed())
				waitBack(crd(widgetsCRD), reconcileWait)
				waitBack(unstructuredObj(widgetGVK, testNS, "blue-widget"), 5*time.Minute)
			})

			It("recreates a deleted addon-level definition", func() {
				Expect(k8sClient.Delete(ctx, traitDefinitionObj(systemNS, v.ownerTraitName))).Should(Succeed())
				waitBack(traitDefinitionObj(systemNS, v.ownerTraitName), reconcileWait)
			})

			It("recreates a deleted module Application, whose finalizer first removed everything it applied", func() {
				before := uidOf(&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: v.moduleAppName(), Namespace: systemNS}})
				deleteApp(ctx, systemNS, v.moduleAppName())
				waitAppGone(ctx, systemNS, v.moduleAppName(), reconcileWait)
				Expect(isNotFound(ctx, crd(widgetsCRD))).Should(BeTrue(), "the finalizer removed the CRD")
				Expect(isNotFound(ctx, componentDefinitionObj(systemNS, widgetDef))).Should(BeTrue())
				waitBack(&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: v.moduleAppName(), Namespace: systemNS}}, reconcileWait)
				waitAppRunning(ctx, systemNS, v.moduleAppName(), installWait)
				Expect(uidOf(&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: v.moduleAppName(), Namespace: systemNS}})).ShouldNot(Equal(before))
				waitBack(unstructuredObj(widgetGVK, testNS, "blue-widget"), 5*time.Minute)
			})

			It("recreates a deleted addon Application, cascading through the module Application", func() {
				deleteApp(ctx, systemNS, v.addonAppName())
				waitAppGone(ctx, systemNS, v.addonAppName(), reconcileWait)
				waitAppGone(ctx, systemNS, v.moduleAppName(), reconcileWait)
				waitBack(&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: v.addonAppName(), Namespace: systemNS}}, 5*time.Minute)
				waitAppRunning(ctx, systemNS, v.addonAppName(), installWait)
				waitAppRunning(ctx, systemNS, v.moduleAppName(), installWait)
				waitAppRunning(ctx, testNS, v.appName, reconcileWait)
			})
		})
	}

	// --- Scenario 12 ---
	for _, v := range []widgetVariant{externalWidgetVariant, inlineWidgetVariant} {
		v := v
		Context(fmt.Sprintf("deleting the addon while something still uses it, %s module (scenario 12)", v.label), func() {
			widgetGVK := schema.GroupVersionKind{Group: v.crdGroup, Version: "v1alpha1", Kind: "Widget"}
			widgetsCRD := "widgets." + v.crdGroup
			widgetDef := v.moduleSlug + "-v1-widget"
			tierLabel := v.crdGroup + "/tier"

			install := func() {
				Expect(k8sClient.Create(ctx, addonApplication(v.appName, v.addonSlug, "1.0.0", nil))).Should(Succeed())
				waitAppRunning(ctx, testNS, v.appName, installWait)
			}

			BeforeAll(func() {
				install()
				Expect(applyManifestFile(ctx, testdataPath("apps", v.selfHealConsumerFile))).Should(Succeed())
				waitAppRunning(ctx, testNS, v.selfHealConsumerApp, shortWait)
				_, err := getConfigMap(ctx, testNS, "blue-widget-card")
				Expect(err).ShouldNot(HaveOccurred())
				DeferCleanup(func() {
					deleteApp(ctx, testNS, v.selfHealConsumerApp)
					waitAppGone(ctx, testNS, v.selfHealConsumerApp, reconcileWait)
					if !isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: v.appName, Namespace: testNS}}) {
						uninstall(v.appName, v.addonAppName(), v.moduleAppName())
					}
					waitGone(ctx, crd(widgetsCRD), reconcileWait)
				})
			})

			It("uninstalls with no in-use guard: the chain and the CRD go, the consumer keeps its built-in objects and turns unhealthy", func() {
				uninstall(v.appName, v.addonAppName(), v.moduleAppName())
				_, err := getConfigMap(ctx, testNS, "blue-widget-card")
				Expect(err).ShouldNot(HaveOccurred(), "a built-in kind survives; only the Widget went with its CRD")

				Expect(isNotFound(ctx, crd(widgetsCRD))).Should(BeTrue(), "the Widget kind went with the module")

				// The Widget went with its CRD, so only the refusal and the parse
				// failure are left to check.
				expectConsumerOfRemovedDefinition(ctx, v.selfHealConsumerApp, widgetDef, "", "after-uninstall")
				_, err = getConfigMap(ctx, testNS, "blue-widget-card")
				Expect(err).ShouldNot(HaveOccurred(), "the built-in object is still there while the consumer fails to parse")
			})

			It("re-installing heals the consumer", func() {
				install()
				bumpPublishVersion(ctx, testNS, v.selfHealConsumerApp, "after-reinstall")
				waitAppRunning(ctx, testNS, v.selfHealConsumerApp, reconcileWait)
				w, err := getUnstructured(ctx, widgetGVK, testNS, "blue-widget")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(w.GetLabels()).Should(HaveKeyWithValue(tierLabel, "gold"))
			})

			It("deletes a consumer cleanly after its definitions are gone", func() {
				uninstall(v.appName, v.addonAppName(), v.moduleAppName())
				deleteApp(ctx, testNS, v.selfHealConsumerApp)
				waitAppGone(ctx, testNS, v.selfHealConsumerApp, reconcileWait)
				waitGone(ctx, configMapObj(testNS, "blue-widget-card"), shortWait)
			})
		})
	}

	// --- Scenario 13 ---
	Context("one owner per addon and per module (scenario 13)", func() {
		It("refuses a second user Application for the same addon until the first is gone", func() {
			Expect(k8sClient.Create(ctx, addonApplication("platform-a", "widget-platform", "1.0.0", nil))).Should(Succeed())
			DeferCleanup(func() {
				deleteApp(ctx, testNS, "platform-a")
				deleteApp(ctx, testNS, "platform-b")
				waitAppGone(ctx, systemNS, addonWidgetPlatform, reconcileWait)
				waitAppGone(ctx, systemNS, moduleWidgetKit, reconcileWait)
				waitGone(ctx, crd(widgetsCRD), shortWait)
			})
			waitAppRunning(ctx, testNS, "platform-a", installWait)

			Expect(k8sClient.Create(ctx, addonApplication("platform-b", "widget-platform", "1.0.0", nil))).Should(Succeed())
			waitAppStatusContains(ctx, testNS, "platform-b",
				"existing object Application vela-system/addon-widget-platform is managed by other application default/platform-a", shortWait)
			Expect(mustGetApp(ctx, testNS, "platform-b").Status.Phase).ShouldNot(Equal(common.ApplicationRunning))
			Expect(mustGetApp(ctx, systemNS, addonWidgetPlatform).Labels).Should(HaveKeyWithValue(oam.LabelAppName, "platform-a"))

			By("deleting the winner lets the loser converge on its workflow retry")
			deleteApp(ctx, testNS, "platform-a")
			waitAppGone(ctx, systemNS, moduleWidgetKit, reconcileWait)
			waitAppRunning(ctx, testNS, "platform-b", retryWait)
			Expect(mustGetApp(ctx, systemNS, addonWidgetPlatform).Labels).Should(HaveKeyWithValue(oam.LabelAppName, "platform-b"))
		})

		It("refuses a second addon importing the same module until the first owner is gone", func() {
			Expect(k8sClient.Create(ctx, addonApplication(widgetPlatformApp, "widget-platform", "1.0.0", nil))).Should(Succeed())
			DeferCleanup(func() {
				deleteApp(ctx, testNS, widgetPlatformApp)
				uninstall("kit-suite", "addon-kit-suite", moduleWidgetKit, moduleGadgetKit)
				waitAppGone(ctx, systemNS, addonWidgetPlatform, reconcileWait)
			})
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)

			Expect(k8sClient.Create(ctx, addonApplication("kit-suite", "kit-suite", "1.0.0", nil))).Should(Succeed())
			waitAppStatusContains(ctx, systemNS, "addon-kit-suite",
				"existing object Application vela-system/module-widget-kit is managed by other application vela-system/addon-widget-platform", reconcileWait)
			waitAppRunning(ctx, systemNS, moduleGadgetKit, installWait)
			Expect(mustGetApp(ctx, systemNS, moduleWidgetKit).Labels).Should(HaveKeyWithValue(oam.LabelAppName, addonWidgetPlatform))
			Expect(mustGetApp(ctx, systemNS, moduleGadgetKit).Labels).Should(HaveKeyWithValue(oam.LabelAppName, "addon-kit-suite"))
			Expect(mustGetApp(ctx, systemNS, "addon-kit-suite").Status.Phase).ShouldNot(Equal(common.ApplicationRunning))
			// The refused widget-kit component never gets a service entry on
			// addon-kit-suite (health is not collected for an object it does not
			// own, and its dispatch fails), so the addon status counts 2 healthy
			// of 2 and reports the phase. The reason is on addon-kit-suite, checked
			// above.
			Eventually(func(g Gomega) {
				svc := findService(getAppG(g, ctx, testNS, "kit-suite"), "kit-suite")
				g.Expect(svc).ShouldNot(BeNil())
				g.Expect(svc.Healthy).Should(BeFalse())
				g.Expect(svc.Message).Should(ContainSubstring("workflowFailed"))
			}, reconcileWait, pollInterval).Should(Succeed())
			Expect(mustGetApp(ctx, testNS, "kit-suite").Status.Phase).ShouldNot(Equal(common.ApplicationRunning))

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
			restartWorkflow(ctx, systemNS, "addon-kit-suite", "true")
			waitAppRunning(ctx, systemNS, "addon-kit-suite", retryWait)
			Eventually(func(g Gomega) string {
				return getAppG(g, ctx, systemNS, moduleWidgetKit).Labels[oam.LabelAppName]
			}, reconcileWait, pollInterval).Should(Equal("addon-kit-suite"))
			waitAppRunning(ctx, testNS, "kit-suite", reconcileWait)
		})

		It("refuses a second addon importing the same inline module until the first owner is gone", func() {
			// Reuses widget-platform-inline-1.0.0 and kit-suite-inline-1.0.0
			// (scenarios 02 and 03): both already bundle widget-kit-inline, so
			// this is the same cross-addon ownership conflict as the external
			// case above, just dispatched through a type: k8s-objects component
			// instead of type: module -- MustBeControlledByApp (pkg/utils/apply/
			// apply.go) is generic across component types, so the failure mode
			// should be identical.
			Expect(k8sClient.Create(ctx, addonApplication("widget-platform-inline", "widget-platform-inline", "1.0.0", nil))).Should(Succeed())
			DeferCleanup(func() {
				deleteApp(ctx, testNS, "widget-platform-inline")
				uninstall("kit-suite-inline", "addon-kit-suite-inline", "module-widget-kit-inline", "module-gadget-kit-inline")
				waitAppGone(ctx, systemNS, "addon-widget-platform-inline", reconcileWait)
			})
			waitAppRunning(ctx, testNS, "widget-platform-inline", installWait)

			Expect(k8sClient.Create(ctx, addonApplication("kit-suite-inline", "kit-suite-inline", "1.0.0", nil))).Should(Succeed())
			waitAppStatusContains(ctx, systemNS, "addon-kit-suite-inline",
				"existing object Application vela-system/module-widget-kit-inline is managed by other application vela-system/addon-widget-platform-inline", reconcileWait)
			waitAppRunning(ctx, systemNS, "module-gadget-kit-inline", installWait)
			Expect(mustGetApp(ctx, systemNS, "module-widget-kit-inline").Labels).Should(HaveKeyWithValue(oam.LabelAppName, "addon-widget-platform-inline"))
			Expect(mustGetApp(ctx, systemNS, "module-gadget-kit-inline").Labels).Should(HaveKeyWithValue(oam.LabelAppName, "addon-kit-suite-inline"))
			Expect(mustGetApp(ctx, systemNS, "addon-kit-suite-inline").Status.Phase).ShouldNot(Equal(common.ApplicationRunning))
			Eventually(func(g Gomega) {
				svc := findService(getAppG(g, ctx, testNS, "kit-suite-inline"), "kit-suite-inline")
				g.Expect(svc).ShouldNot(BeNil())
				g.Expect(svc.Healthy).Should(BeFalse())
				g.Expect(svc.Message).Should(ContainSubstring("workflowFailed"))
			}, reconcileWait, pollInterval).Should(Succeed())
			Expect(mustGetApp(ctx, testNS, "kit-suite-inline").Status.Phase).ShouldNot(Equal(common.ApplicationRunning))

			By("removing the other owner")
			deleteApp(ctx, testNS, "widget-platform-inline")
			waitAppGone(ctx, systemNS, "addon-widget-platform-inline", reconcileWait)
			waitAppGone(ctx, testNS, "widget-platform-inline", reconcileWait)

			By("restarting addon-kit-suite-inline's workflow")
			restartWorkflow(ctx, systemNS, "addon-kit-suite-inline", "true")
			waitAppRunning(ctx, systemNS, "addon-kit-suite-inline", retryWait)
			Eventually(func(g Gomega) string {
				return getAppG(g, ctx, systemNS, "module-widget-kit-inline").Labels[oam.LabelAppName]
			}, reconcileWait, pollInterval).Should(Equal("addon-kit-suite-inline"))
			waitAppRunning(ctx, testNS, "kit-suite-inline", reconcileWait)
		})
	})

	// --- Scenario 14 ---
	Context("broken _imports.cue files and resolution failures (scenario 14)", func() {
		const appName = "broken-imports"

		const inlineAppName = "broken-imports-inline"

		BeforeAll(func() {
			Expect(ociTags(moduleRegistry.hostBase, "modules/widget-kit")).ShouldNot(ContainElement("9.9.9"))
			DeferCleanup(func() {
				for _, name := range []string{appName, inlineAppName, "missing-addon-version", "missing-addon", "unknown-addon-registry"} {
					deleteApp(ctx, testNS, name)
				}
				waitAppGone(ctx, systemNS, "addon-broken-imports", reconcileWait)
				waitAppGone(ctx, systemNS, "addon-broken-imports-inline", reconcileWait)
				Expect(isNotFound(ctx, configMapObj(systemNS, "broken-imports-marker"))).Should(BeTrue())
				Expect(isNotFound(ctx, configMapObj(systemNS, "broken-imports-inline-marker"))).Should(BeTrue())
			})
		})

		It("loads the addon but fails the generated module component when the import cannot be fetched", func() {
			cases := []struct{ version, component, fragment string }{
				{"1.0.1", "widget-kit", `module registry "no-such-registry" not found`},
				{"1.0.2", "ghost-kit", "ghost-kit"},
				{"1.0.3", "widget-kit", "9.9.9"},
				{"1.0.4", "widget-kit", "~1.0.0"},
			}
			Expect(k8sClient.Create(ctx, addonApplication(appName, "broken-imports", cases[0].version, nil))).Should(Succeed())
			for i, tc := range cases {
				By("broken-imports " + tc.version)
				if i > 0 {
					setAddonVersion(ctx, appName, tc.version)
				}
				Eventually(func(g Gomega) {
					addonApp := getAppG(g, ctx, systemNS, "addon-broken-imports")
					g.Expect(addonApp.Labels).Should(HaveKeyWithValue(oam.LabelAddonVersion, tc.version))
					g.Expect(addonApp.Status.Phase).ShouldNot(Equal(common.ApplicationRunning))
					res := findService(addonApp, "broken-imports-resources")
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
					marker, err := getConfigMap(ctx, systemNS, "broken-imports-marker")
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
				Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "module-ghost-kit", Namespace: systemNS}})).Should(BeTrue())
			}
			deleteApp(ctx, testNS, appName)
			waitAppGone(ctx, systemNS, "addon-broken-imports", reconcileWait)
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
				Expect(k8sClient.Create(ctx, addonApplication(appName, "broken-imports", tc.version, nil))).Should(Succeed())
				waitAppStatusContains(ctx, testNS, appName, tc.fragment, reconcileWait)
				Expect(mustGetApp(ctx, testNS, appName).Status.Phase).ShouldNot(Equal(common.ApplicationRunning))
				Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "addon-broken-imports", Namespace: systemNS}})).Should(BeTrue())
				Expect(isNotFound(ctx, configMapObj(systemNS, "broken-imports-marker"))).Should(BeTrue())
				deleteApp(ctx, testNS, appName)
				waitAppGone(ctx, testNS, appName, shortWait)
			}
		})

		It("fails the type: addon component itself when an inline module is malformed or collides, installing nothing", func() {
			// Unlike modules/_imports.cue (resolved lazily by a type: module
			// component's CueX provider, after the addon's own resources are
			// already dispatched), inline modules are parsed eagerly while the
			// addon package itself is read, and any resulting error -- parse
			// failure or name collision -- surfaces from the same
			// resolveAndRender call that builds the addon's whole component
			// list. So every inline failure here lands in the same
			// "installs nothing" bucket as the _imports.cue parse failures
			// above, none in the "module component fails, addon resources
			// still install" bucket of the previous spec.
			cases := []struct{ version, fragment string }{
				{"1.0.1", `missing _module.cue`},
				{"1.0.2", `module declares no API lines`},
				{"1.0.3", `is declared more than once`},
			}
			for _, tc := range cases {
				By("broken-imports-inline " + tc.version)
				waitAppGone(ctx, testNS, inlineAppName, reconcileWait)
				Expect(k8sClient.Create(ctx, addonApplication(inlineAppName, "broken-imports-inline", tc.version, nil))).Should(Succeed())
				waitAppStatusContains(ctx, testNS, inlineAppName, tc.fragment, reconcileWait)
				if tc.version == "1.0.3" {
					// Names both colliding sources, not just the generic fragment.
					text := appStatusText(mustGetApp(ctx, testNS, inlineAppName))
					Expect(text).Should(SatisfyAll(
						ContainSubstring(`"widget-kit"`), ContainSubstring("modules/_imports.cue"), ContainSubstring("modules/widget-kit")))
				}
				Expect(mustGetApp(ctx, testNS, inlineAppName).Status.Phase).ShouldNot(Equal(common.ApplicationRunning))
				Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "addon-broken-imports-inline", Namespace: systemNS}})).Should(BeTrue())
				Expect(isNotFound(ctx, configMapObj(systemNS, "broken-imports-inline-marker"))).Should(BeTrue())
				deleteApp(ctx, testNS, inlineAppName)
				waitAppGone(ctx, testNS, inlineAppName, shortWait)
			}
		})

		It("reports addon-level mistakes where they belong: unknown registry at admission, unknown version or addon at render", func() {
			err := k8sClient.Create(ctx, &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "unknown-addon-registry", Namespace: testNS},
				Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{
					Name: "widget-platform", Type: "addon",
					Properties: rawExtension(map[string]interface{}{"registry": "no-such-addon-registry", "version": "1.0.0"}),
				}}},
			})
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("is not a configured addon registry"))

			Expect(k8sClient.Create(ctx, addonApplication("missing-addon-version", "widget-platform", "9.9.9", nil))).Should(Succeed())
			waitAppStatusContains(ctx, testNS, "missing-addon-version", `addon "widget-platform" version "9.9.9" not found in registries`, reconcileWait)

			Expect(k8sClient.Create(ctx, addonApplication("missing-addon", "ghost-addon", "1.0.0", nil))).Should(Succeed())
			waitAppStatusContains(ctx, testNS, "missing-addon", `addon "ghost-addon" version "1.0.0" not found in registries`, reconcileWait)
		})
	})

	// --- Scenario 15 ---
	Context("registries disappear, credentials go bad (scenario 15)", func() {
		health := func(g Gomega, ready string) {
			svc := findService(mustGetApp(ctx, testNS, widgetPlatformApp), "widget-platform")
			g.Expect(svc).ShouldNot(BeNil())
			g.Expect(svc.Healthy).Should(BeTrue(), svc.Message)
			g.Expect(svc.Message).Should(Equal(ready))
		}
		installed := func() {
			Expect(mustGetApp(ctx, systemNS, moduleWidgetKit).Name).Should(Equal(moduleWidgetKit))
			_, err := getComponentDefinition(ctx, systemNS, "widget-kit-v1-widget")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: widgetsCRD}, crd(widgetsCRD))).Should(Succeed())
		}
		restoreModuleRegistry := func() {
			runVelaSucceed("module", "registry", "add", moduleRegistryName, moduleRegistry.cluster, "--type", "oci")
		}
		restoreAddonRegistry := func() { addAddonRegistry(ctx) }

		BeforeAll(func() {
			Expect(k8sClient.Create(ctx, addonApplication(widgetPlatformApp, "widget-platform", "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
			Eventually(func(g Gomega) { health(g, "Ready:6/6") }, shortWait, pollInterval).Should(Succeed())
			DeferCleanup(func() {
				restoreModuleRegistry()
				restoreAddonRegistry()
				deleteApp(ctx, testNS, "new-platform")
				uninstall(widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit)
			})
		})

		It("a deleted module registry breaks the health-check render, removes nothing, and heals when it is back", func() {
			runVelaSucceed("module", "registry", "delete", moduleRegistryName)
			Eventually(func(g Gomega) {
				svc := findService(getAppG(g, ctx, systemNS, addonWidgetPlatform), "widget-kit")
				g.Expect(svc).ShouldNot(BeNil())
				g.Expect(svc.Healthy).Should(BeFalse())
				g.Expect(svc.Message).Should(ContainSubstring(`module registry "` + moduleRegistryName + `" not found`))
			}, reconcileWait, pollInterval).Should(Succeed())
			waitAppStatusContains(ctx, testNS, widgetPlatformApp, "widget-kit unhealthy", reconcileWait)
			installed()

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

		It("a deleted addon registry is invisible to a pinned addon until the controller restarts, and admission still checks the name", func() {
			runVelaSucceed("addon", "registry", "delete", addonRegistryName)
			Consistently(func(g Gomega) { health(g, "Ready:6/6") }, 90*time.Second, 10*time.Second).Should(Succeed(), "the pinned render is served from vela-core's cache")

			err := k8sClient.Create(ctx, addonApplication("new-platform", "kit-suite", "1.0.0", nil))
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("is not a configured addon registry"))

			By("restarting vela-core, which empties the cache")
			restartVelaCore(ctx)
			Eventually(func(g Gomega) {
				svc := findService(getAppG(g, ctx, testNS, widgetPlatformApp), "widget-platform")
				g.Expect(svc).ShouldNot(BeNil())
				g.Expect(svc.Healthy).Should(BeFalse())
				g.Expect(svc.Message).Should(SatisfyAll(ContainSubstring(`addon "widget-platform" version "1.0.0"`), ContainSubstring(addonRegistryName)))
			}, reconcileWait, pollInterval).Should(Succeed())
			installed()

			By("adding the registry back")
			restoreAddonRegistry()
			Eventually(func(g Gomega) { health(g, "Ready:6/6") }, reconcileWait, pollInterval).Should(Succeed())
		})
	})

	// --- Scenario 16 ---
	Context("vela addon enable installs the same addon without its modules (scenario 16)", func() {
		BeforeAll(func() {
			// The legacy installer downloads the addon in the CLI process,
			// through the stored registry record.
			skipUnlessHostReachesCluster(addonRegistry, "vela addon enable")
		})

		AfterAll(func() {
			_, _ = runVela("addon", "disable", "widget-platform", "-f")
			_, _ = runVela("addon", "disable", "widget-platform-inline", "-f")
			uninstall(widgetPlatformApp, addonWidgetPlatform, moduleWidgetKit)
		})

		It("the legacy installer ignores _imports.cue", func() {
			runVelaSucceed("addon", "enable", addonRegistryName+"/widget-platform", "--version", "1.0.0", "-y")
			app := mustGetApp(ctx, systemNS, addonWidgetPlatform)
			Expect(componentNames(app)).Should(ConsistOf("platform-info", "widget-platform-resources", "team-config"))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAddonName, "widget-platform"))
			Expect(app.Labels).ShouldNot(HaveKey(oam.LabelAppName), "a CLI install has no owning Application")
			Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: moduleWidgetKit, Namespace: systemNS}})).Should(BeTrue())
			Expect(moduleDefinitionNames(ctx, systemNS, "widget-kit")).Should(BeEmpty())
			_, err := getTraitDefinition(ctx, systemNS, "platform-owner")
			Expect(err).ShouldNot(HaveOccurred(), "the CLI applies the addon's own definitions")
			Expect(strings.ToLower(runVelaSucceed("addon", "status", "widget-platform"))).Should(ContainSubstring("enabled"))
		})

		It("the legacy installer ignores inline modules the same way", func() {
			// readInlineModulesDir (pkg/addon/addon.go) still parses modules/<name>/
			// into InstallPackage.InlineModules regardless of entry point, but
			// RenderInlineModuleComponents is only ever called from
			// resolveAndRender (pkg/addon/service/renderer.go) -- never from this
			// legacy CLI path -- so the parsed module is never rendered into a
			// component here either.
			runVelaSucceed("addon", "enable", addonRegistryName+"/widget-platform-inline", "--version", "1.0.0", "-y")
			app := mustGetApp(ctx, systemNS, "addon-widget-platform-inline")
			Expect(componentNames(app)).ShouldNot(ContainElement("widget-kit-inline"))
			Expect(isNotFound(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "module-widget-kit-inline", Namespace: systemNS}})).Should(BeTrue())
			Expect(moduleDefinitionNames(ctx, systemNS, "widget-kit-inline")).Should(BeEmpty())
			runVelaSucceed("addon", "disable", "widget-platform-inline")
		})

		It("the component path cannot adopt a CLI-installed addon Application", func() {
			Expect(k8sClient.Create(ctx, addonApplication(widgetPlatformApp, "widget-platform", "1.0.0", nil))).Should(Succeed())
			waitAppStatusContains(ctx, testNS, widgetPlatformApp, "exists but not managed by any application", shortWait)
			Expect(mustGetApp(ctx, testNS, widgetPlatformApp).Status.Phase).ShouldNot(Equal(common.ApplicationRunning))
		})

		It("after vela addon disable, the component path installs the addon with its module", func() {
			runVelaSucceed("addon", "disable", "widget-platform")
			waitAppRunning(ctx, testNS, widgetPlatformApp, retryWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)
			app := mustGetApp(ctx, systemNS, addonWidgetPlatform)
			Expect(componentNames(app)).Should(ContainElements("widget-kit", "addon-definitions", "addon-auxiliaries"))
			Expect(app.Labels).Should(HaveKeyWithValue(oam.LabelAppName, widgetPlatformApp))
		})
	})

	// --- Scenario 17 ---
	// An addon that bundles a module under modules/<name>/ renders it eagerly,
	// in the controller, into a type: k8s-objects component holding the
	// module-<name> Application; no registry is involved, so every spec here
	// works without publishing anything.
	Context("inline and imported modules in one addon (scenario 17)", func() {
		const appName = "inline-mixed"

		It("renders an inline k8s-objects component and an imported type: module component side by side", func() {
			Expect(k8sClient.Create(ctx, addonApplication(appName, "inline-mixed", "1.0.0", nil))).Should(Succeed())
			waitAppRunning(ctx, testNS, appName, installWait)
			waitAppRunning(ctx, systemNS, "addon-inline-mixed", shortWait)
			waitAppRunning(ctx, systemNS, moduleInlineKit, shortWait)
			waitAppRunning(ctx, systemNS, moduleWidgetKit, shortWait)

			app := mustGetApp(ctx, systemNS, "addon-inline-mixed")
			Expect(findComponent(app, "widget-kit").Type).Should(Equal("module"))
			Expect(findComponent(app, "inline-kit").Type).Should(Equal("k8s-objects"))
			for _, name := range []string{"widget-kit", "inline-kit"} {
				Expect(findComponent(app, name).DependsOn).Should(ContainElement("inline-mixed-resources"), name)
			}
		})

		It("deleting the user Application garbage-collects both nested module Applications", func() {
			uninstall(appName, "addon-inline-mixed", moduleInlineKit, moduleWidgetKit)
		})
	})

	// --- Scenario 18 (last: it restarts the controller with gates toggled) ---
	Context("turning the feature gates off under a running install (scenario 18)", func() {
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
				uninstall(inlineWidgetVariant.appName, inlineWidgetVariant.addonAppName(), inlineWidgetVariant.moduleAppName())
			})

			Expect(k8sClient.Create(ctx, addonApplication(widgetPlatformApp, "widget-platform", "1.0.0", nil))).Should(Succeed())
			Expect(k8sClient.Create(ctx, addonApplication(inlineWidgetVariant.appName, inlineWidgetVariant.addonSlug, "1.0.0", nil))).Should(Succeed())
			Expect(applyManifestFile(ctx, testdataPath("apps", "module-direct.yaml"))).Should(Succeed())
			waitAppRunning(ctx, testNS, widgetPlatformApp, installWait)
			waitAppRunning(ctx, testNS, inlineWidgetVariant.appName, installWait)
			waitAppRunning(ctx, testNS, "module-direct", installWait)
		})

		It("with the module gate off, every type: module render fails and nothing is garbage-collected", func() {
			setFeatureGates(ctx, true, false)
			Eventually(func(g Gomega) {
				svc := findService(getAppG(g, ctx, systemNS, addonWidgetPlatform), "widget-kit")
				g.Expect(svc).ShouldNot(BeNil())
				g.Expect(svc.Healthy).Should(BeFalse())
				g.Expect(svc.Message).Should(ContainSubstring("module-as-component is disabled; enable the EnableModuleComponent feature gate"))
			}, reconcileWait, pollInterval).Should(Succeed())
			waitAppStatusContains(ctx, testNS, widgetPlatformApp, "widget-kit unhealthy", reconcileWait)
			waitAppStatusContains(ctx, testNS, "module-direct", "module-as-component is disabled", reconcileWait)
			Expect(mustGetApp(ctx, systemNS, moduleWidgetKit).Name).Should(Equal(moduleWidgetKit))
			Expect(mustGetApp(ctx, systemNS, moduleGadgetKit).Name).Should(Equal(moduleGadgetKit))

			// widget-platform-inline bundles its module inline: there is no
			// standalone type: module component with its own CueX provider to
			// fail in place while the addon's other resources stay up. Instead
			// RenderInlineModuleComponents's gate check runs inside the same
			// resolveAndRender call that builds addon-widget-platform-inline's
			// whole component list, so the *entire* addon-level render fails on
			// its next reconcile -- surfacing as the top-level "widget-platform-
			// inline" component itself going unhealthy with the gate message,
			// not as a "widget-kit-inline" sub-service. Nothing already
			// installed is touched (the render step simply fails to produce a
			// new manifest this round), so the owned Application is still there.
			waitAppStatusContains(ctx, testNS, inlineWidgetVariant.appName, "module-as-component is disabled; enable the EnableModuleComponent feature gate", reconcileWait)
			Expect(mustGetApp(ctx, systemNS, inlineWidgetVariant.moduleAppName()).Name).Should(Equal(inlineWidgetVariant.moduleAppName()))
		})

		It("with both gates off, type: addon renders fail too and admission refuses new type: addon Applications with the gate message", func() {
			setFeatureGates(ctx, false, false)
			waitAppStatusContains(ctx, testNS, widgetPlatformApp, "addon-as-component is disabled; enable the EnableAddonComponent feature gate", reconcileWait)
			// Same "type: addon" CueX provider either way, so widget-platform-inline
			// fails its own render with the identical gate message.
			waitAppStatusContains(ctx, testNS, inlineWidgetVariant.appName, "addon-as-component is disabled; enable the EnableAddonComponent feature gate", reconcileWait)

			probe := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "gate-off-probe", Namespace: testNS},
				Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{
					Name: "widget-platform", Type: "addon",
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
				svc := findService(getAppG(g, ctx, testNS, widgetPlatformApp), "widget-platform")
				g.Expect(svc).ShouldNot(BeNil())
				g.Expect(svc.Healthy).Should(BeTrue(), svc.Message)
				g.Expect(svc.Message).Should(Equal("Ready:6/6"))
			}, reconcileWait, pollInterval).Should(Succeed())
			Eventually(func(g Gomega) {
				svc := findService(getAppG(g, ctx, testNS, "module-direct"), "gadget-kit")
				g.Expect(svc).ShouldNot(BeNil())
				g.Expect(svc.Healthy).Should(BeTrue(), svc.Message)
				g.Expect(svc.Message).Should(Equal("Ready:3/3"))
			}, reconcileWait, pollInterval).Should(Succeed())
			Eventually(func(g Gomega) {
				svc := findService(getAppG(g, ctx, testNS, inlineWidgetVariant.appName), inlineWidgetVariant.addonSlug)
				g.Expect(svc).ShouldNot(BeNil())
				g.Expect(svc.Healthy).Should(BeTrue(), svc.Message)
				g.Expect(svc.Message).Should(Equal("Ready:6/6"))
			}, reconcileWait, pollInterval).Should(Succeed())
		})
	})
})
