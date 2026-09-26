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

// This file is the e2e suite for the module-as-a-component feature: registry
// management, publish, install and use, reconciler behaviour, uninstall,
// namespace isolation, git-registry refusals, and the module error paths. It
// reuses the CLI/registry plumbing from module_publish_test.go
// (runVelaCommand, moduleE2ERegistryURL, etc).
package controllers_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	veltypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"

	pkgmodule "github.com/oam-dev/kubevela/pkg/module"
	regcomponent "github.com/oam-dev/kubevela/pkg/registry/component"
)

const (
	demoStoreModuleName      = "demo-store"
	demoStoreModuleVersion   = "1.0.0"
	demoStoreUpgradeVersion  = "1.1.0"
	demoStoreRegistryName    = "demo-store-oci"
	demoStoreFixtureRelPath  = "test/e2e-module-test/testdata/module/demo-store"
	demoStoreUpgradeRelPath  = "test/e2e-module-test/testdata/module/demo-store-1.1.0"
	demoStoreDeployAppName   = "module-" + demoStoreModuleName + "-deploy"
	demoStoreOwnedAppName    = "module-" + demoStoreModuleName
	demoStoreGitRegistryName = "demo-store-gitreg"
)

var _ = Describe("Module as a component", Ordered, func() {
	var (
		ctx          context.Context
		repoRoot     string
		registryURL  string
		registryBase string
		store        regcomponent.RegistryDataStore
		// moduleInstallNamespace is where the deploy Application currently
		// lives. It starts in vela-system and moves to a tenant namespace
		// once the "namespace (group C)" Context claims the install, so
		// AfterAll knows where to actually find it rather than deleting a
		// vela-system Application that has been gone since group C started.
		moduleInstallNamespace string
	)

	BeforeAll(func() {
		ctx = context.Background()
		repoRoot = modulePublishRepoRoot()
		store = pkgmodule.NewStore(k8sClient)
		moduleInstallNamespace = veltypes.DefaultKubeVelaNS

		By("bringing up the in-cluster OCI registry")
		Expect(applyManifestFile(ctx, k8sClient, "testdata/module/registry.yaml")).Should(Succeed())
		waitForOCIRegistryDeploymentAvailable(ctx)
		registryURL = moduleE2ERegistryURL(ctx)
		var err error
		registryBase, err = registryHTTPBase(registryURL)
		Expect(err).ShouldNot(HaveOccurred())
		waitForOCIRegistryReachable(registryBase + "/v2/")

		By("registering the registry once, so scenario 1's own It only exercises add/delete on a second entry")
		runVelaCommandSucceed(repoRoot, "module", "registry", "add", demoStoreRegistryName, registryURL, "--type", "oci")

		By("publishing demo-store 1.0.0 and 1.1.0")
		runVelaCommandSucceed(repoRoot, "module", "publish", demoStoreFixtureRelPath, "--registry", demoStoreRegistryName)
		runVelaCommandSucceed(repoRoot, "module", "publish", demoStoreUpgradeRelPath, "--registry", demoStoreRegistryName)
	})

	AfterAll(func() {
		_, _ = runVelaCommand(repoRoot, "module", "registry", "delete", demoStoreRegistryName)
		_ = k8sClient.Delete(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: demoStoreDeployAppName, Namespace: moduleInstallNamespace}})
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, k8stypes.NamespacedName{Name: demoStoreOwnedAppName, Namespace: veltypes.DefaultKubeVelaNS}, &v1beta1.Application{})
			g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue(), "the owned Application must be gone before the next run of this suite reuses this cluster")
		}, 90*time.Second, 3*time.Second).Should(Succeed())
		_ = k8sClient.Delete(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "oci-registry", Namespace: "default"}})
		_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "oci-registry", Namespace: "default"}})
	})

	// --- Scenario 1: registry management ---
	Context("registry management (scenario 1)", func() {
		const secondRegistry = "demo-store-oci-2"

		AfterEach(func() {
			_, _ = runVelaCommand(repoRoot, "module", "registry", "delete", secondRegistry)
		})

		It("resolves the sole configured registry by default", func() {
			out := runVelaCommandSucceed(repoRoot, "module", "deploy", demoStoreModuleName, "--dry-run")
			Expect(out).Should(ContainSubstring("registry: " + demoStoreRegistryName))
		})

		It("refuses ambiguous resolution once a second registry exists, naming both", func() {
			runVelaCommandSucceed(repoRoot, "module", "registry", "add", secondRegistry, registryURL, "--type", "oci")
			out, err := runVelaCommand(repoRoot, "module", "publish", demoStoreFixtureRelPath, "--dry-run")
			Expect(err).Should(HaveOccurred())
			Expect(out).Should(ContainSubstring(demoStoreRegistryName))
			Expect(out).Should(ContainSubstring(secondRegistry))
		})

		It("adds and deletes a registry", func() {
			runVelaCommandSucceed(repoRoot, "module", "registry", "add", secondRegistry, registryURL, "--type", "oci")
			out := runVelaCommandSucceed(repoRoot, "module", "registry", "list")
			Expect(out).Should(ContainSubstring(secondRegistry))

			runVelaCommandSucceed(repoRoot, "module", "registry", "delete", secondRegistry)
			out = runVelaCommandSucceed(repoRoot, "module", "registry", "list")
			Expect(out).ShouldNot(ContainSubstring(secondRegistry))
		})

		// The in-cluster registry.yaml registry (used everywhere else in this
		// file) is anonymous, so it cannot exercise credential storage. A real
		// authenticated push/pull needs a TLS-terminated oci:// endpoint
		// (ociURLIsPlainHTTP in pkg/registry/component/oci_chart.go treats
		// anything but a literal "http://" URL as TLS, and credentials are
		// refused outright for "http://"), which is exactly the ECR-shaped
		// setup this environment cannot reach. So this checks only what does
		// not need a reachable registry: `registry add` never dials out, it
		// just writes the ConfigMap and Secret.
		It("stores a real credential in a Secret, not the ConfigMap", func() {
			const credRegistryName = "demo-store-cred"
			runVelaCommandSucceed(repoRoot, "module", "registry", "add", credRegistryName,
				"oci://registry.invalid/modules", "--username", "testuser", "--password", "testpass")
			defer func() { _, _ = runVelaCommand(repoRoot, "module", "registry", "delete", credRegistryName) }()

			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: pkgmodule.ModuleRegistryConfigMap, Namespace: veltypes.DefaultKubeVelaNS}, &cm)).Should(Succeed())
			Expect(cm.Data["registries"]).Should(ContainSubstring(`"tokenSecretRef":"module-registry-` + credRegistryName + `"`))
			Expect(cm.Data["registries"]).ShouldNot(ContainSubstring("testpass"), "the password must not land in the ConfigMap")

			var secret corev1.Secret
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "module-registry-" + credRegistryName, Namespace: veltypes.DefaultKubeVelaNS}, &secret)).Should(Succeed())
		})
	})

	// --- Scenario 2: publish ---
	Context("publish (scenario 2)", func() {
		It("dry run prints the target and every annotation and pushes nothing", func() {
			out := runVelaCommandSucceed(repoRoot, "module", "publish", demoStoreFixtureRelPath,
				"--registry", demoStoreRegistryName, "--version", "9.9.9-dryrun", "--dry-run")
			Expect(out).Should(ContainSubstring("modules.oam.dev/module: demo-store"))
			Expect(out).Should(ContainSubstring("modules.oam.dev/lines: v1,v1alpha1,v2"))
			Expect(out).Should(ContainSubstring("modules.oam.dev/enabled-lines: v1,v2"))

			tagsResp, tagsBody, err := fetchRegistryJSON(registryBase+"/v2/modules/demo-store/tags/list", "")
			Expect(err).ShouldNot(HaveOccurred(), tagsBody)
			Expect(tagsResp["tags"]).ShouldNot(ContainElement("9.9.9-dryrun"), "dry run must not push")
		})

		It("refuses a republish of an already-published version, and --force overrides it", func() {
			out, err := runVelaCommand(repoRoot, "module", "publish", demoStoreFixtureRelPath, "--registry", demoStoreRegistryName)
			Expect(err).Should(HaveOccurred())
			Expect(out).Should(ContainSubstring("bump version"))

			out = runVelaCommandSucceed(repoRoot, "module", "publish", demoStoreFixtureRelPath, "--registry", demoStoreRegistryName, "--force")
			Expect(out).Should(ContainSubstring("demo-store:1.0.0"))

			tagsResp, tagsBody, err := fetchRegistryJSON(registryBase+"/v2/modules/demo-store/tags/list", "")
			Expect(err).ShouldNot(HaveOccurred(), tagsBody)
			Expect(tagsResp["tags"]).Should(ContainElement("1.0.0"), "the real push must land in the registry")
		})

		It("--version retags the artifact and warns, without changing the module's own version", func() {
			out := runVelaCommandSucceed(repoRoot, "module", "publish", demoStoreFixtureRelPath,
				"--registry", demoStoreRegistryName, "--version", "1.0.0-retag", "--dry-run")
			Expect(out).Should(ContainSubstring("still declares version 1.0.0"))
			Expect(out).Should(ContainSubstring("demo-store:1.0.0-retag"))
		})
	})

	// --- Scenario 3: install and use ---
	Context("install and use (scenario 3)", func() {
		BeforeAll(func() {
			By("deploying demo-store 1.0.0 into vela-system")
			// Both 1.0.0 and 1.1.0 are already published (top BeforeAll, for
			// scenario 4d's upgrade), so this must pin the version: an
			// unpinned deploy resolves the highest published tag and would
			// install 1.1.0, not the 1.0.0 this whole context asserts about.
			runVelaCommandSucceed(repoRoot, "module", "deploy", demoStoreModuleName, "--registry", demoStoreRegistryName, "--version", demoStoreModuleVersion)
		})

		It("installs five tiers, all healthy, and the owned Application carries the module label and version", func() {
			Eventually(func(g Gomega) {
				var owned v1beta1.Application
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: demoStoreOwnedAppName, Namespace: veltypes.DefaultKubeVelaNS}, &owned)).Should(Succeed())
				g.Expect(owned.Labels[veltypes.LabelDefinitionModule]).Should(Equal(demoStoreModuleName))
				g.Expect(owned.Annotations[veltypes.AnnoDefinitionModuleVersion]).Should(Equal(demoStoreModuleVersion))
				g.Expect(owned.Status.Services).Should(HaveLen(5))
				for _, svc := range owned.Status.Services {
					g.Expect(svc.Healthy).Should(BeTrue(), "tier %q: %s", svc.Name, svc.Message)
				}
			}, 60*time.Second, 2*time.Second).Should(Succeed())
		})

		It("installs every definition under its derived name, and nothing from the disabled v1alpha1 line", func() {
			var cdList v1beta1.ComponentDefinitionList
			Expect(k8sClient.List(ctx, &cdList, client.InNamespace(veltypes.DefaultKubeVelaNS),
				client.MatchingLabels{veltypes.LabelDefinitionModule: demoStoreModuleName})).Should(Succeed())
			var names []string
			for _, cd := range cdList.Items {
				names = append(names, cd.Name)
			}
			Expect(names).Should(ConsistOf("demo-store-v1-bucket", "demo-store-v1-raw-config", "demo-store-v2-bucket"))

			var noPreview v1beta1.ComponentDefinition
			err := k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-v1alpha1-preview", Namespace: veltypes.DefaultKubeVelaNS}, &noPreview)
			Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
		})

		It("installs auxiliary objects from both scopes and all three formats", func() {
			ns := veltypes.DefaultKubeVelaNS
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-shared", Namespace: ns}, &corev1.ConfigMap{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-v1-settings", Namespace: ns}, &corev1.ConfigMap{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-v2-settings", Namespace: ns}, &corev1.ConfigMap{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-runner", Namespace: ns}, &corev1.ServiceAccount{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-shared-token", Namespace: ns}, &corev1.Secret{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-v1-reader", Namespace: ns}, &rbacv1.Role{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-v1-reader", Namespace: ns}, &rbacv1.RoleBinding{})).Should(Succeed())
		})

		It("renders Form 3 with a trait from another line", func() {
			Expect(applyManifestFile(ctx, k8sClient, "testdata/module/consumer-v2-bucket.yaml")).Should(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "photos", Namespace: "default"}})
			})
			Eventually(func(g Gomega) {
				var cm corev1.ConfigMap
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "photos", Namespace: "default"}, &cm)).Should(Succeed())
				g.Expect(cm.Data["renderedBy"]).Should(Equal("demo-store-v2-bucket"))
				g.Expect(cm.Labels["team"]).Should(Equal("platform"))
			}, 60*time.Second, 2*time.Second).Should(Succeed())
		})

		It("resolves Form 2 by label", func() {
			Expect(applyManifestFile(ctx, k8sClient, "testdata/module/consumer-v1-bucket.yaml")).Should(Succeed())
			DeferCleanup(func() {
				// Delete the Application first: deleting only the rendered
				// ConfigMap leaves the consumer running, so reconciliation
				// just recreates it and leaks state into later specs.
				_ = k8sClient.Delete(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "demo-store-consumer-v1", Namespace: "default"}})
				_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "documents", Namespace: "default"}})
			})
			Eventually(func(g Gomega) {
				var cm corev1.ConfigMap
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "documents", Namespace: "default"}, &cm)).Should(Succeed())
				g.Expect(cm.Data["renderedBy"]).Should(Equal("demo-store-v1-bucket"))
			}, 60*time.Second, 2*time.Second).Should(Succeed())
		})

		It("refuses Form 1 as ambiguous across two lines", func() {
			err := applyManifestFile(ctx, k8sClient, "testdata/module/consumer-ambiguous.yaml")
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("is ambiguous"))
		})

		It("stops the render when a required parameter has no value", func() {
			err := applyManifestFile(ctx, k8sClient, "testdata/module/consumer-missing-region.yaml")
			if err == nil {
				DeferCleanup(func() {
					_ = k8sClient.Delete(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "demo-store-missing-region", Namespace: "default"}})
				})
			}
			Eventually(func(g Gomega) string {
				var app v1beta1.Application
				if k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-missing-region", Namespace: "default"}, &app) != nil {
					return ""
				}
				return workflowMessage(&app)
			}, 60*time.Second, 2*time.Second).Should(ContainSubstring("region: incomplete value string"))

			var cm corev1.ConfigMap
			err = k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "broken", Namespace: "default"}, &cm)
			Expect(k8serrors.IsNotFound(err)).Should(BeTrue(), "the render must stop before creating the ConfigMap")
		})
	})

	// --- Scenario 4: reconciler behaviour ---
	Context("reconciler (scenario 4)", func() {
		It("reverts an edited auxiliary object", func() {
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-v1-settings", Namespace: veltypes.DefaultKubeVelaNS}, &cm)).Should(Succeed())
			cm.Data["line"] = "tampered"
			Expect(k8sClient.Update(ctx, &cm)).Should(Succeed())

			Eventually(func(g Gomega) string {
				var got corev1.ConfigMap
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-v1-settings", Namespace: veltypes.DefaultKubeVelaNS}, &got)).Should(Succeed())
				return got.Data["line"]
			}, 3*time.Minute, 5*time.Second).Should(Equal("v1"))
		})

		It("recreates a deleted definition", func() {
			Expect(k8sClient.Delete(ctx, &v1beta1.ComponentDefinition{ObjectMeta: metav1.ObjectMeta{Name: "demo-store-v1-bucket", Namespace: veltypes.DefaultKubeVelaNS}})).Should(Succeed())

			Eventually(func(g Gomega) error {
				return k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-v1-bucket", Namespace: veltypes.DefaultKubeVelaNS}, &v1beta1.ComponentDefinition{})
			}, 3*time.Minute, 5*time.Second).Should(Succeed())
		})

		It("applies a pinned upgrade in full", func() {
			runVelaCommandSucceed(repoRoot, "module", "deploy", demoStoreModuleName, "--registry", demoStoreRegistryName, "--version", demoStoreUpgradeVersion)

			Eventually(func(g Gomega) {
				var owned v1beta1.Application
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: demoStoreOwnedAppName, Namespace: veltypes.DefaultKubeVelaNS}, &owned)).Should(Succeed())
				g.Expect(owned.Annotations[veltypes.AnnoDefinitionModuleVersion]).Should(Equal(demoStoreUpgradeVersion))
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			ns := veltypes.DefaultKubeVelaNS
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-v1-archive", Namespace: ns}, &v1beta1.ComponentDefinition{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-v1alpha1-preview", Namespace: ns}, &v1beta1.ComponentDefinition{})).Should(Succeed())
			err := k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-v1-raw-config", Namespace: ns}, &v1beta1.ComponentDefinition{})
			Expect(k8serrors.IsNotFound(err)).Should(BeTrue(), "the definition dropped by the upgrade must be garbage collected")
		})

		It("a republished version with no spec change is not picked up", func() {
			ns := veltypes.DefaultKubeVelaNS

			By("switching to an unpinned install, so the Application keeps resolving whatever it first saw")
			Expect(k8sClient.Delete(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: demoStoreDeployAppName, Namespace: ns}})).Should(Succeed())
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, k8stypes.NamespacedName{Name: demoStoreOwnedAppName, Namespace: ns}, &v1beta1.Application{})
				g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
			}, 90*time.Second, 3*time.Second).Should(Succeed())
			runVelaCommandSucceed(repoRoot, "module", "deploy", demoStoreModuleName, "--registry", demoStoreRegistryName)
			Eventually(func(g Gomega) {
				var owned v1beta1.Application
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: demoStoreOwnedAppName, Namespace: ns}, &owned)).Should(Succeed())
				g.Expect(owned.Annotations[veltypes.AnnoDefinitionModuleVersion]).Should(Equal(demoStoreUpgradeVersion))
			}, 90*time.Second, 3*time.Second).Should(Succeed())

			By("publishing a newer tag while the Application spec stays untouched")
			runVelaCommandSucceed(repoRoot, "module", "publish", demoStoreUpgradeRelPath, "--registry", demoStoreRegistryName, "--version", "1.1.1")

			By("confirming a full resync (reSyncPeriod=1m in e2e.mk) does not move the installed version")
			Consistently(func(g Gomega) {
				var owned v1beta1.Application
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: demoStoreOwnedAppName, Namespace: ns}, &owned)).Should(Succeed())
				g.Expect(owned.Annotations[veltypes.AnnoDefinitionModuleVersion]).Should(Equal(demoStoreUpgradeVersion),
					"an unpinned install must keep serving whatever it first resolved, until the -rr- gate lands")
			}, 80*time.Second, 5*time.Second).Should(Succeed())

			By("pinning back to 1.1.0, handing group A a known version to uninstall")
			runVelaCommandSucceed(repoRoot, "module", "deploy", demoStoreModuleName, "--registry", demoStoreRegistryName, "--version", demoStoreUpgradeVersion)
		})
	})

	// --- Group A: uninstall ---
	Context("uninstall (group A)", func() {
		BeforeAll(func() {
			By("applying a consumer of the v2 bucket capability, so its behaviour once the capability is removed can be observed")
			Expect(applyManifestFile(ctx, k8sClient, "testdata/module/consumer-v2-bucket.yaml")).Should(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "demo-store-consumer-v2", Namespace: "default"}})
				_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "photos", Namespace: "default"}})
			})
			Eventually(func(g Gomega) {
				var cm corev1.ConfigMap
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "photos", Namespace: "default"}, &cm)).Should(Succeed())
			}, 60*time.Second, 2*time.Second).Should(Succeed())
		})

		It("removes the owned Application, every definition and every auxiliary object, and both ResourceTrackers", func() {
			Expect(k8sClient.Delete(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: demoStoreDeployAppName, Namespace: veltypes.DefaultKubeVelaNS}})).Should(Succeed())

			ns := veltypes.DefaultKubeVelaNS
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, k8stypes.NamespacedName{Name: demoStoreOwnedAppName, Namespace: ns}, &v1beta1.Application{})
				g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())

				var cdList v1beta1.ComponentDefinitionList
				g.Expect(k8sClient.List(ctx, &cdList, client.InNamespace(ns), client.MatchingLabels{veltypes.LabelDefinitionModule: demoStoreModuleName})).Should(Succeed())
				g.Expect(cdList.Items).Should(BeEmpty())

				err = k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-shared", Namespace: ns}, &corev1.ConfigMap{})
				g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
				err = k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-shared-token", Namespace: ns}, &corev1.Secret{})
				g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
				err = k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-runner", Namespace: ns}, &corev1.ServiceAccount{})
				g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())

				var trackers v1beta1.ResourceTrackerList
				g.Expect(k8sClient.List(ctx, &trackers, client.MatchingLabels{oam.LabelAppName: demoStoreOwnedAppName})).Should(Succeed())
				g.Expect(trackers.Items).Should(BeEmpty())
			}, 90*time.Second, 3*time.Second).Should(Succeed())
		})

		It("a consumer of the removed capability keeps its resources", func() {
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "photos", Namespace: "default"}, &cm)).Should(Succeed())

			Eventually(func(g Gomega) {
				var consumer v1beta1.Application
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-consumer-v2", Namespace: "default"}, &consumer)).Should(Succeed())
				var readyFalse bool
				for _, c := range consumer.Status.Conditions {
					if string(c.Type) == "Ready" && string(c.Status) == "False" {
						readyFalse = true
					}
				}
				g.Expect(readyFalse).Should(BeTrue(), "Ready must go False while the capability it uses is gone: %+v", consumer.Status.Conditions)
			}, 90*time.Second, 3*time.Second).Should(Succeed())
		})

		It("reinstall restores the definition set and the consumer recovers on its own", func() {
			ns := veltypes.DefaultKubeVelaNS
			runVelaCommandSucceed(repoRoot, "module", "deploy", demoStoreModuleName, "--registry", demoStoreRegistryName, "--version", demoStoreUpgradeVersion)
			Eventually(func(g Gomega) {
				var cdList v1beta1.ComponentDefinitionList
				g.Expect(k8sClient.List(ctx, &cdList, client.InNamespace(ns), client.MatchingLabels{veltypes.LabelDefinitionModule: demoStoreModuleName})).Should(Succeed())
				g.Expect(cdList.Items).ShouldNot(BeEmpty())
			}, 90*time.Second, 3*time.Second).Should(Succeed())

			Eventually(func(g Gomega) common.ApplicationPhase {
				var consumer v1beta1.Application
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "demo-store-consumer-v2", Namespace: "default"}, &consumer)).Should(Succeed())
				return consumer.Status.Phase
			}, 2*time.Minute, 3*time.Second).Should(Equal(common.ApplicationRunning), "the untouched consumer must recover once the module is back")
		})
	})

	// --- Group C: namespace ---
	Context("namespace (group C)", func() {
		var tenant string

		BeforeAll(func() {
			// Group A leaves demo-store installed in vela-system. The owned
			// Application is a cluster-wide singleton, so that install must
			// be gone before a tenant-namespaced one can claim it.
			Expect(k8sClient.Delete(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: demoStoreDeployAppName, Namespace: veltypes.DefaultKubeVelaNS}})).Should(Succeed())
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, k8stypes.NamespacedName{Name: demoStoreOwnedAppName, Namespace: veltypes.DefaultKubeVelaNS}, &v1beta1.Application{})
				g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
			}, 90*time.Second, 3*time.Second).Should(Succeed())

			tenant = randomNamespaceName("module-tenant")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: tenant}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
			})
		})

		It("installs definitions and auxiliary objects into the named namespace, owned Application stays in vela-system", func() {
			runVelaCommandSucceed(repoRoot, "module", "deploy", demoStoreModuleName, "--registry", demoStoreRegistryName, "-n", tenant)

			Eventually(func(g Gomega) {
				var cdList v1beta1.ComponentDefinitionList
				g.Expect(k8sClient.List(ctx, &cdList, client.InNamespace(tenant), client.MatchingLabels{veltypes.LabelDefinitionModule: demoStoreModuleName})).Should(Succeed())
				g.Expect(cdList.Items).ShouldNot(BeEmpty())

				var owned v1beta1.Application
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: demoStoreOwnedAppName, Namespace: veltypes.DefaultKubeVelaNS}, &owned)).Should(Succeed())
			}, 90*time.Second, 3*time.Second).Should(Succeed())

			var deployApp v1beta1.Application
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: demoStoreDeployAppName, Namespace: tenant}, &deployApp)).Should(Succeed())
			moduleInstallNamespace = tenant
		})

		It("is usable from the tenant namespace it was installed into", func() {
			consumer := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "demo-store-tenant-consumer", Namespace: tenant},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name:       "tenant-bucket",
						Type:       "demo-store/v2/bucket",
						Properties: rawExtension(`{"bucketName":"tenant","region":"us-west-2","class":"standard"}`),
					}},
				},
			}
			Expect(k8sClient.Create(ctx, consumer)).Should(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, consumer)
				_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "tenant-bucket", Namespace: tenant}})
			})
			Eventually(func(g Gomega) {
				var cm corev1.ConfigMap
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "tenant-bucket", Namespace: tenant}, &cm)).Should(Succeed())
				g.Expect(cm.Data["renderedBy"]).Should(Equal("demo-store-v2-bucket"))
			}, 60*time.Second, 2*time.Second).Should(Succeed())
		})

		It("does not resolve from another namespace", func() {
			err := applyManifestFile(ctx, k8sClient, "testdata/module/consumer-v2-bucket.yaml")
			// The Form 3 reference the fixture uses names a definition this
			// module only installed into the tenant namespace, so from
			// "default" it must not resolve.
			if err == nil {
				_ = k8sClient.Delete(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "demo-store-consumer-v2", Namespace: "default"}})
			}
			Expect(err).Should(HaveOccurred())
			// The message currently says "WorkloadDefinition ... not found"
			// (a known wording bug, tracked as a defect to fix, not a
			// contract). Assert on the definition name, not that wording.
			Expect(err.Error()).Should(ContainSubstring("demo-store-v2-bucket"))
		})

		It("refuses a second install into another namespace and leaves the first untouched", func() {
			out, err := runVelaCommand(repoRoot, "module", "deploy", demoStoreModuleName, "--registry", demoStoreRegistryName, "-n", "default")
			Expect(err).Should(HaveOccurred(), "output:\n%s", out)
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: demoStoreDeployAppName, Namespace: "default"}})
			})

			Eventually(func(g Gomega) string {
				var app v1beta1.Application
				if k8sClient.Get(ctx, k8stypes.NamespacedName{Name: demoStoreDeployAppName, Namespace: "default"}, &app) != nil {
					return ""
				}
				return workflowMessage(&app)
			}, 60*time.Second, 2*time.Second).Should(ContainSubstring("managed by other application"))

			// The tenant install must be untouched.
			var owned v1beta1.Application
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: demoStoreOwnedAppName, Namespace: veltypes.DefaultKubeVelaNS}, &owned)).Should(Succeed())
			Expect(owned.Labels["app.oam.dev/namespace"]).Should(Equal(tenant))
		})
	})

	// --- Group B: git refusals ---
	Context("git refusals (group B)", func() {
		const gitURL = "https://github.com/kubevela/catalog"

		It("the CLI rejects every unsupported URL shape", func() {
			// https://, no .git -- the scheme is checked first, so this is
			// the "modules cannot read a registry over https" message, not the
			// git one.
			out, err := runVelaCommand(repoRoot, "module", "registry", "add", "b1refused", gitURL)
			Expect(err).Should(HaveOccurred())
			Expect(out).Should(ContainSubstring("modules cannot read a registry over https"))

			// https://....git -- the scheme is still checked first, so a .git
			// suffix does not change the answer: same https message as above,
			// not the git-specific one.
			out, err = runVelaCommand(repoRoot, "module", "registry", "add", "b2refused", gitURL+".git")
			Expect(err).Should(HaveOccurred())
			Expect(out).Should(ContainSubstring("modules cannot read a registry over https"))

			// The git message is reached by a schemeless .git form instead.
			out, err = runVelaCommand(repoRoot, "module", "registry", "add", "b2git", "git@github.com:kubevela/catalog.git")
			Expect(err).Should(HaveOccurred())
			Expect(out).Should(ContainSubstring("git registries are not supported"))

			// Schemeless, no .git either -- refused rather than guessed.
			out, err = runVelaCommand(repoRoot, "module", "registry", "add", "b3refused", "github.com/kubevela/catalog")
			Expect(err).Should(HaveOccurred())
			Expect(out).Should(ContainSubstring("cannot infer the registry type"))

			// --type git explicit.
			out, err = runVelaCommand(repoRoot, "module", "registry", "add", "b4refused", gitURL, "--type", "git")
			Expect(err).Should(HaveOccurred())
			Expect(out).Should(ContainSubstring("unsupported registry type"))

			out = runVelaCommandSucceed(repoRoot, "module", "registry", "list")
			Expect(out).ShouldNot(ContainSubstring("b1refused"), "none of the rejected adds should have been stored")
		})

		It("a git entry already in the ConfigMap is listed with type git", func() {
			Expect(store.AddRegistry(ctx, regcomponent.Registry{
				Name: demoStoreGitRegistryName,
				Git:  &regcomponent.GitAddonSource{URL: gitURL},
			})).Should(Succeed())
			DeferCleanup(func() {
				_ = store.DeleteRegistry(ctx, demoStoreGitRegistryName)
			})
			out := runVelaCommandSucceed(repoRoot, "module", "registry", "list")
			Expect(out).Should(ContainSubstring(demoStoreGitRegistryName))
			Expect(out).Should(ContainSubstring("git"))
		})

		It("registry get, publish --dry-run and deploy all refuse a git-backed entry before any network call", func() {
			Expect(store.AddRegistry(ctx, regcomponent.Registry{
				Name: demoStoreGitRegistryName,
				Git:  &regcomponent.GitAddonSource{URL: gitURL},
			})).Should(Succeed())
			DeferCleanup(func() {
				_ = store.DeleteRegistry(ctx, demoStoreGitRegistryName)
			})

			out, err := runVelaCommand(repoRoot, "module", "registry", "get", demoStoreGitRegistryName)
			Expect(err).Should(HaveOccurred())
			Expect(out).Should(ContainSubstring("is a git source"))

			out, err = runVelaCommand(repoRoot, "module", "publish", demoStoreFixtureRelPath, "--registry", demoStoreGitRegistryName, "--dry-run")
			Expect(err).Should(HaveOccurred())
			Expect(out).Should(ContainSubstring("is a git source"))

			out, err = runVelaCommand(repoRoot, "module", "deploy", demoStoreModuleName, "--registry", demoStoreGitRegistryName)
			Expect(err).Should(HaveOccurred())
			Expect(out).Should(ContainSubstring("is a git source"))
			var deployApp v1beta1.Application
			getErr := k8sClient.Get(ctx, k8stypes.NamespacedName{Name: demoStoreDeployAppName, Namespace: veltypes.DefaultKubeVelaNS}, &deployApp)
			Expect(k8serrors.IsNotFound(getErr)).Should(BeTrue(), "no deploy Application should have been created")
		})

		It("the webhook refuses a type: module component naming a git registry", func() {
			Expect(store.AddRegistry(ctx, regcomponent.Registry{
				Name: demoStoreGitRegistryName,
				Git:  &regcomponent.GitAddonSource{URL: gitURL},
			})).Should(Succeed())
			DeferCleanup(func() {
				_ = store.DeleteRegistry(ctx, demoStoreGitRegistryName)
			})

			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "demo-store-gitreg-consumer", Namespace: "default"},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name:       "gitmodule",
						Type:       "module",
						Properties: rawExtension(fmt.Sprintf(`{"module":"%s","registry":"%s"}`, demoStoreModuleName, demoStoreGitRegistryName)),
					}},
				},
			}
			err := k8sClient.Create(ctx, app)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("use an OCI registry"))
		})

		It("the webhook refuses a type: addon component naming a git registry, with the addon remedy", func() {
			// The addon webhook reads a different ConfigMap (vela-addon-registry)
			// than the module one, so this needs its own store and its own entry.
			addonStore := regcomponent.NewRegistryDataStore(k8sClient)
			Expect(addonStore.AddRegistry(ctx, regcomponent.Registry{
				Name: demoStoreGitRegistryName,
				Git:  &regcomponent.GitAddonSource{URL: gitURL},
			})).Should(Succeed())
			DeferCleanup(func() {
				_ = addonStore.DeleteRegistry(ctx, demoStoreGitRegistryName)
			})

			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "demo-store-gitreg-addon-consumer", Namespace: "default"},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name:       "gitaddon",
						Type:       "addon",
						Properties: rawExtension(fmt.Sprintf(`{"addon":"whatever","registry":"%s"}`, demoStoreGitRegistryName)),
					}},
				},
			}
			err := k8sClient.Create(ctx, app)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("use an OCI registry or a Helm repository"))
		})

		It("a component naming no registry is admitted and refused at reconcile, naming the ambiguity", func() {
			Expect(store.AddRegistry(ctx, regcomponent.Registry{
				Name: demoStoreGitRegistryName,
				Git:  &regcomponent.GitAddonSource{URL: gitURL},
			})).Should(Succeed())
			DeferCleanup(func() {
				_ = store.DeleteRegistry(ctx, demoStoreGitRegistryName)
			})

			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "demo-store-no-registry-consumer", Namespace: "default"},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name:       "unresolved",
						Type:       "module",
						Properties: rawExtension(fmt.Sprintf(`{"module":"%s"}`, demoStoreModuleName)),
					}},
				},
			}
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, app)
			})

			Eventually(func(g Gomega) string {
				got := &v1beta1.Application{}
				if k8sClient.Get(ctx, k8stypes.NamespacedName{Name: app.Name, Namespace: app.Namespace}, got) != nil {
					return ""
				}
				return workflowMessage(got)
			}, 60*time.Second, 2*time.Second).Should(ContainSubstring("none is named"))
		})

		It("with a git registry as the sole configured one, an unnamed registry is refused at reconcile as git", func() {
			// resolveRegistryByName's sole-registry rule picks this one, so
			// resolution reaches the git gate instead of the ambiguity error
			// above.
			runVelaCommandSucceed(repoRoot, "module", "registry", "delete", demoStoreRegistryName)
			DeferCleanup(func() {
				runVelaCommandSucceed(repoRoot, "module", "registry", "add", demoStoreRegistryName, registryURL, "--type", "oci")
			})
			Expect(store.AddRegistry(ctx, regcomponent.Registry{
				Name: demoStoreGitRegistryName,
				Git:  &regcomponent.GitAddonSource{URL: gitURL},
			})).Should(Succeed())
			DeferCleanup(func() {
				_ = store.DeleteRegistry(ctx, demoStoreGitRegistryName)
			})

			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "demo-store-sole-git-consumer", Namespace: "default"},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name:       "unresolved-sole",
						Type:       "module",
						Properties: rawExtension(fmt.Sprintf(`{"module":"%s"}`, demoStoreModuleName)),
					}},
				},
			}
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, app)
			})

			Eventually(func(g Gomega) string {
				got := &v1beta1.Application{}
				if k8sClient.Get(ctx, k8stypes.NamespacedName{Name: app.Name, Namespace: app.Namespace}, got) != nil {
					return ""
				}
				return workflowMessage(got)
			}, 60*time.Second, 2*time.Second).Should(ContainSubstring("is a git source"))
		})
	})

	// --- Group D: error paths ---
	Context("error paths (group D)", func() {
		It("deploy of a module never published names it plainly", func() {
			out, err := runVelaCommand(repoRoot, "module", "deploy", "nosuchmodule", "--registry", demoStoreRegistryName)
			Expect(err).Should(HaveOccurred())
			Expect(out).Should(ContainSubstring(`module "nosuchmodule" is not published to this registry`))

			var deployApp v1beta1.Application
			getErr := k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "module-nosuchmodule-deploy", Namespace: veltypes.DefaultKubeVelaNS}, &deployApp)
			Expect(k8serrors.IsNotFound(getErr)).Should(BeTrue())
		})

		It("malformed component properties are refused at admission", func() {
			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "demo-store-malformed-consumer", Namespace: "default"},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name:       "malformed",
						Type:       "module",
						Properties: rawExtension(`{"module":12345}`),
					}},
				},
			}
			err := k8sClient.Create(ctx, app)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("cannot be decoded as module component properties"))
		})

		It("an unknown registry name is admitted and refused at reconcile", func() {
			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "demo-store-unknown-registry-consumer", Namespace: "default"},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name:       "unknownreg",
						Type:       "module",
						Properties: rawExtension(fmt.Sprintf(`{"module":"%s","registry":"doesnotexist"}`, demoStoreModuleName)),
					}},
				},
			}
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, app)
			})

			Eventually(func(g Gomega) string {
				got := &v1beta1.Application{}
				if k8sClient.Get(ctx, k8stypes.NamespacedName{Name: app.Name, Namespace: app.Namespace}, got) != nil {
					return ""
				}
				return workflowMessage(got)
			}, 60*time.Second, 2*time.Second).Should(ContainSubstring(`module registry "doesnotexist" not found`))
		})
	})
})

// rawExtension wraps a JSON literal as component properties, for the
// Applications built directly against k8sClient in the webhook and reconcile
// cases below (there is no fixture file for these; they exist to trigger one
// specific refusal).
func rawExtension(json string) *runtime.RawExtension {
	return &runtime.RawExtension{Raw: []byte(json)}
}

// workflowMessage returns app's workflow-level message plus every step's own
// message, joined. A render failure surfaces on the step (StepStatus.Message
// in the kubevela/workflow API), not necessarily on the workflow's own
// aggregate message, so checking only the latter can see an empty string
// even once the step has already failed.
func workflowMessage(app *v1beta1.Application) string {
	if app.Status.Workflow == nil {
		return ""
	}
	parts := []string{app.Status.Workflow.Message}
	for _, step := range app.Status.Workflow.Steps {
		parts = append(parts, step.Message)
		for _, sub := range step.SubStepsStatus {
			parts = append(parts, sub.Message)
		}
	}
	return strings.Join(parts, "\n")
}
