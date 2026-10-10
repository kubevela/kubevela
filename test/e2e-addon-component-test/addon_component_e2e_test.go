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

// This file is the e2e suite for the addon-as-a-component feature: registry
// management, push, install and use, reconciler behaviour, uninstall,
// ownership between the imperative and declarative install paths, version
// movement, and admission refusals.
//
// The registry is a plain-HTTP ChartMuseum brought up in-cluster rather than
// an OCI registry. An addon registry of --type oci must use the oci:// scheme,
// and that scheme always takes the TLS path on the controller's fetch
// (ociURLIsPlainHTTP accepts only a literal "http://" URL), so an OCI registry
// here would need certificates materialised into the cluster before the
// controller could pull -- which tests TLS plumbing, not addons.
//
// Each group that installs an addon is an Ordered container with its own
// renamed copy of the fixtures (see addonScope), so groups run on separate
// Ginkgo workers while the specs inside one group run in order.
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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	veltypes "github.com/oam-dev/kubevela/apis/types"
)

const (
	echoAddonName    = "echo-server"
	echoAddonVersion = "1.0.0"
	echoAddonUpgrade = "1.1.0"
	echoFixturePath  = "test/e2e-addon-component-test/testdata/addon/echo-server"
	echoUpgradePath  = "test/e2e-addon-component-test/testdata/addon/echo-server-1.1.0"
	echoNamespace    = "echo-system"

	configAddonName   = "config-store"
	configFixturePath = "test/e2e-addon-component-test/testdata/addon/config-store"

	addonRegistryName = "addon-e2e-cm"

	// bundleAppName prefixes the outer Application declaring two addons.
	bundleAppName = "platform-bundle"

	waitTimeout = 300 * time.Second
	pollPeriod  = 3 * time.Second
)

var _ = Describe("Addon as a component", func() {
	ctx := context.Background()

	// applyWrappingApp builds and creates an outer Application with a single
	// type: addon component. Properties go through a RawExtension because the
	// addon component's schema is open: KubeVela's own fields sit alongside a
	// nested `properties` block belonging to the addon.
	applyWrappingApp := func(appName, addon, version string, addonProps string) error {
		props := fmt.Sprintf(`{"addon":%q,"registry":%q`, addon, addonRegistryName)
		if version != "" {
			props += fmt.Sprintf(`,"version":%q`, version)
		}
		if addonProps != "" {
			props += `,"properties":` + addonProps
		}
		props += `}`
		return k8sClient.Create(ctx, &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: veltypes.DefaultKubeVelaNS},
			Spec: v1beta1.ApplicationSpec{
				Components: []oamcommon.ApplicationComponent{{
					Name:       addon,
					Type:       "addon",
					Properties: &runtime.RawExtension{Raw: []byte(props)},
				}},
			},
		})
	}

	expectAppGone := func(name string) {
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, k8stypes.NamespacedName{Name: name, Namespace: veltypes.DefaultKubeVelaNS}, &v1beta1.Application{})
			g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue(), "Application %s should be gone", name)
		}, waitTimeout, pollPeriod).Should(Succeed())
	}

	// deleteApp waits for the object to actually disappear. Returning as soon
	// as the delete call is accepted lets the next spec recreate the same name
	// while finalizers are still running, which the apiserver rejects with
	// "object is being deleted ... already exists".
	deleteApp := func(name string) {
		_ = k8sClient.Delete(ctx, &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: veltypes.DefaultKubeVelaNS},
		})
		expectAppGone(name)
	}

	expectAppHealthy := func(name string) {
		Eventually(func(g Gomega) {
			app := &v1beta1.Application{}
			g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: name, Namespace: veltypes.DefaultKubeVelaNS}, app)).Should(Succeed())
			g.Expect(app.Status.Phase).Should(Equal(oamcommon.ApplicationRunning), "Application %s phase", name)
		}, waitTimeout, pollPeriod).Should(Succeed())
	}

	// --- Scenario 1: registry management ---

	// Serial: it adds and deletes entries in the cluster-wide registry
	// ConfigMap that every other group reads.
	Context("registry management (scenario 1)", Ordered, Serial, func() {
		const secondRegistry = "addon-e2e-cm-2"

		AfterEach(func() {
			_, _ = runVelaCommand(root, "addon", "registry", "delete", secondRegistry)
		})

		It("lists the registry it was registered with", func() {
			out := runVelaCommandSucceed(root, "addon", "registry", "list")
			Expect(out).Should(ContainSubstring(addonRegistryName))
			Expect(out).Should(ContainSubstring(registryURL))
		})

		It("adds and deletes a registry", func() {
			runVelaCommandSucceed(root, "addon", "registry", "add", secondRegistry, "--type", "helm", "--endpoint", registryURL)
			out := runVelaCommandSucceed(root, "addon", "registry", "list")
			Expect(out).Should(ContainSubstring(secondRegistry))

			runVelaCommandSucceed(root, "addon", "registry", "delete", secondRegistry)
			out = runVelaCommandSucceed(root, "addon", "registry", "list")
			Expect(out).ShouldNot(ContainSubstring(secondRegistry))
		})

		It("re-adding an existing name succeeds rather than refusing", func() {
			// `add` is an upsert, not a create. Asserted so a change to that
			// behaviour is caught here rather than surprising someone
			// re-running a setup script.
			runVelaCommandSucceed(root, "addon", "registry", "add", secondRegistry, "--type", "helm", "--endpoint", registryURL)
			runVelaCommandSucceed(root, "addon", "registry", "add", secondRegistry, "--type", "helm", "--endpoint", registryURL)

			out := runVelaCommandSucceed(root, "addon", "registry", "list")
			Expect(strings.Count(out, secondRegistry)).Should(Equal(1), "the registry must not be duplicated")
		})

		It("refuses a helm registry whose index cannot be fetched", func() {
			// The endpoint is validated at add time by downloading its index,
			// so a typo fails here rather than as a confusing render failure
			// the first time an Application references it.
			out, err := runVelaCommand(root, "addon", "registry", "add", secondRegistry,
				"--type", "helm", "--endpoint", registryURL+"/no-such-repo")
			Expect(err).Should(HaveOccurred())
			Expect(out).Should(ContainSubstring("index file"))
		})

		It("refuses an oci registry whose endpoint is not oci://", func() {
			out, err := runVelaCommand(root, "addon", "registry", "add", secondRegistry, "--type", "oci", "--endpoint", registryURL)
			Expect(err).Should(HaveOccurred())
			Expect(out).Should(ContainSubstring("oci://"),
				"the scheme assertion exists so a mistyped endpoint fails here rather than as an opaque auth error later")
		})

		// Credential storage for an OCI registry (token moved into a Secret,
		// tokenSecretRef left in the ConfigMap) is deliberately not covered
		// here: `addon registry add --type oci` validates the endpoint by
		// listing the registry's catalog, so it needs a reachable TLS OCI
		// registry, which this plain-HTTP in-cluster setup cannot provide.
		// That contract is exercised manually instead.
		It("records the registry in the vela-addon-registry ConfigMap", func() {
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{
				Name: "vela-addon-registry", Namespace: veltypes.DefaultKubeVelaNS,
			}, &cm)).Should(Succeed())
			Expect(cm.Data["registries"]).Should(ContainSubstring(addonRegistryName))
			Expect(cm.Data["registries"]).Should(ContainSubstring(registryURL))
		})
	})

	// --- Scenario 2: push and discovery ---

	Context("push and discovery (scenario 2)", func() {
		It("lists both pushed addons in the registry", func() {
			out := runVelaCommandSucceed(root, "addon", "list", "--registry", addonRegistryName)
			Expect(out).Should(ContainSubstring(echoAddonName))
			Expect(out).Should(ContainSubstring(configAddonName))
		})

		It("reports both published versions of the same addon", func() {
			out := runVelaCommandSucceed(root, "addon", "status", echoAddonName, "--verbose")
			Expect(out).Should(ContainSubstring(echoAddonVersion))
			Expect(out).Should(ContainSubstring(echoAddonUpgrade))
		})

		It("refuses to push an addon directory that is not one", func() {
			out, err := runVelaCommand(root, "addon", "push",
				"test/e2e-addon-component-test/testdata", addonRegistryName, "--use-http")
			Expect(err).Should(HaveOccurred(), "a directory with no metadata.yaml is not an addon")
			Expect(out).ShouldNot(BeEmpty())
		})
	})

	// --- Scenario 3: install and use ---

	Context("install and use (scenario 3)", Ordered, func() {
		s := installScope

		AfterEach(func() {
			deleteApp(s.wrappingApp)
			expectAppGone(s.echoOwnedApp())
		})

		It("installs a pinned version and the outer Application owns the inner one", func() {
			Expect(applyWrappingApp(s.wrappingApp, s.echo, echoAddonVersion, "")).Should(Succeed())
			expectAppHealthy(s.wrappingApp)
			expectAppHealthy(s.echoOwnedApp())

			owned := &v1beta1.Application{}
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echoOwnedApp(), Namespace: veltypes.DefaultKubeVelaNS}, owned)).Should(Succeed())
			Expect(owned.Labels["app.oam.dev/name"]).Should(Equal(s.wrappingApp),
				"the inner Application must record which outer Application installed it")
			Expect(owned.Labels["addons.oam.dev/version"]).Should(Equal(echoAddonVersion))
		})

		It("renders the addon's own workloads into its namespace", func() {
			Expect(applyWrappingApp(s.wrappingApp, s.echo, echoAddonVersion, "")).Should(Succeed())
			expectAppHealthy(s.echoOwnedApp())

			Eventually(func(g Gomega) {
				deploy := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echo, Namespace: s.echoNamespace}, deploy)).Should(Succeed())
				g.Expect(deploy.Labels["addons.oam.dev/version"]).Should(Equal(echoAddonVersion))

				svc := &corev1.Service{}
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echo, Namespace: s.echoNamespace}, svc)).Should(Succeed())
			}, waitTimeout, pollPeriod).Should(Succeed())
		})

		It("registers the addon's definitions, labelled with the owning Application", func() {
			Expect(applyWrappingApp(s.wrappingApp, s.echo, echoAddonVersion, "")).Should(Succeed())
			expectAppHealthy(s.echoOwnedApp())

			Eventually(func(g Gomega) {
				cd := &v1beta1.ComponentDefinition{}
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echo, Namespace: veltypes.DefaultKubeVelaNS}, cd)).Should(Succeed())
				g.Expect(cd.Labels["app.oam.dev/name"]).Should(Equal(s.echoOwnedApp()),
					"definitions are dispatched as components of the inner Application, so they carry its tracking label")
			}, waitTimeout, pollPeriod).Should(Succeed())
		})

		It("passes the addon's own parameters through the nested properties block", func() {
			Expect(applyWrappingApp(s.wrappingApp, s.echo, echoAddonVersion, `{"replicas":3}`)).Should(Succeed())
			expectAppHealthy(s.echoOwnedApp())

			Eventually(func(g Gomega) {
				deploy := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echo, Namespace: s.echoNamespace}, deploy)).Should(Succeed())
				g.Expect(deploy.Spec.Replicas).ShouldNot(BeNil())
				g.Expect(*deploy.Spec.Replicas).Should(BeEquivalentTo(3))
			}, waitTimeout, pollPeriod).Should(Succeed())
		})

		It("makes the addon's component type usable by an ordinary Application", func() {
			Expect(applyWrappingApp(s.wrappingApp, s.echo, echoAddonVersion, "")).Should(Succeed())
			expectAppHealthy(s.echoOwnedApp())

			consumerNS := randomNamespaceName("addon-consumer")
			Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: consumerNS}})).Should(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: consumerNS}})
			}()

			consumer := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "echo-consumer", Namespace: consumerNS},
				Spec: v1beta1.ApplicationSpec{
					Components: []oamcommon.ApplicationComponent{{
						Name:       "my-echo",
						Type:       s.echo,
						Properties: &runtime.RawExtension{Raw: []byte(`{"replicas":1}`)},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, consumer)).Should(Succeed())

			Eventually(func(g Gomega) {
				deploy := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: "my-echo", Namespace: consumerNS}, deploy)).Should(Succeed())
			}, waitTimeout, pollPeriod).Should(Succeed())
		})

		It("resolves the latest version when none is pinned", func() {
			Expect(applyWrappingApp(s.wrappingApp, s.echo, "", "")).Should(Succeed())
			expectAppHealthy(s.echoOwnedApp())

			Eventually(func(g Gomega) {
				deploy := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echo, Namespace: s.echoNamespace}, deploy)).Should(Succeed())
				g.Expect(deploy.Labels["addons.oam.dev/version"]).Should(Equal(echoAddonUpgrade),
					"an empty version must resolve to the highest published one, not the lowest")
			}, waitTimeout, pollPeriod).Should(Succeed())
		})
	})

	// --- Scenario 4: reconciler behaviour ---

	Context("reconciler (scenario 4)", Ordered, func() {
		s := reconcileScope

		BeforeAll(func() {
			Expect(applyWrappingApp(s.wrappingApp, s.echo, echoAddonVersion, "")).Should(Succeed())
			expectAppHealthy(s.echoOwnedApp())
		})

		AfterAll(func() {
			deleteApp(s.wrappingApp)
			expectAppGone(s.echoOwnedApp())
		})

		It("recreates a deleted definition", func() {
			cd := &v1beta1.ComponentDefinition{}
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echo, Namespace: veltypes.DefaultKubeVelaNS}, cd)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, cd)).Should(Succeed())

			Eventually(func(g Gomega) {
				restored := &v1beta1.ComponentDefinition{}
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echo, Namespace: veltypes.DefaultKubeVelaNS}, restored)).Should(Succeed())
			}, waitTimeout, pollPeriod).Should(Succeed())
		})

		It("reverts an edited workload", func() {
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echo, Namespace: s.echoNamespace}, deploy)).Should(Succeed())
			edited := deploy.DeepCopy()
			replicas := int32(7)
			edited.Spec.Replicas = &replicas
			Expect(k8sClient.Update(ctx, edited)).Should(Succeed())

			Eventually(func(g Gomega) {
				current := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echo, Namespace: s.echoNamespace}, current)).Should(Succeed())
				g.Expect(*current.Spec.Replicas).Should(BeEquivalentTo(1), "drift must be corrected back to the addon's rendered value")
			}, waitTimeout, pollPeriod).Should(Succeed())
		})

		It("restores the inner Application when it is deleted directly", func() {
			owned := &v1beta1.Application{}
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echoOwnedApp(), Namespace: veltypes.DefaultKubeVelaNS}, owned)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, owned)).Should(Succeed())

			Eventually(func(g Gomega) {
				restored := &v1beta1.Application{}
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echoOwnedApp(), Namespace: veltypes.DefaultKubeVelaNS}, restored)).Should(Succeed())
			}, waitTimeout, pollPeriod).Should(Succeed())
		})
	})

	// --- Group A: uninstall ---

	Context("uninstall (group A)", Ordered, func() {
		s := uninstallScope

		It("removes the inner Application, its definitions and its workloads", func() {
			Expect(applyWrappingApp(s.wrappingApp, s.echo, echoAddonVersion, "")).Should(Succeed())
			expectAppHealthy(s.echoOwnedApp())

			deleteApp(s.wrappingApp)
			expectAppGone(s.echoOwnedApp())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echo, Namespace: veltypes.DefaultKubeVelaNS}, &v1beta1.ComponentDefinition{})
				g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue(), "the addon's ComponentDefinition must go with it")
			}, waitTimeout, pollPeriod).Should(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echo, Namespace: s.echoNamespace}, &appsv1.Deployment{})
				g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue(), "the addon's Deployment must go with it")
			}, waitTimeout, pollPeriod).Should(Succeed())
		})

		It("reinstall restores the definition set", func() {
			Expect(applyWrappingApp(s.wrappingApp, s.echo, echoAddonVersion, "")).Should(Succeed())
			expectAppHealthy(s.echoOwnedApp())

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echo, Namespace: veltypes.DefaultKubeVelaNS}, &v1beta1.ComponentDefinition{})).Should(Succeed())
			}, waitTimeout, pollPeriod).Should(Succeed())

			deleteApp(s.wrappingApp)
			expectAppGone(s.echoOwnedApp())
		})

		It("leaves the registry entry alone when the addon is removed", func() {
			out := runVelaCommandSucceed(root, "addon", "registry", "list")
			Expect(out).Should(ContainSubstring(addonRegistryName),
				"uninstalling an addon must not deregister the registry it came from")
		})
	})

	// --- Group B: ownership between the two install paths ---

	Context("ownership (group B)", Ordered, func() {
		s := ownershipScope

		AfterEach(func() {
			deleteApp(s.wrappingApp)
			_, _ = runVelaCommand(root, "addon", "disable", s.echo)
			expectAppGone(s.echoOwnedApp())
		})

		It("enabling the same addon twice is idempotent", func() {
			runVelaCommandSucceed(root, "addon", "enable", s.echo, "--version", echoAddonVersion, "-y")
			runVelaCommandSucceed(root, "addon", "enable", s.echo, "--version", echoAddonVersion, "-y")

			apps := &v1beta1.ApplicationList{}
			Expect(k8sClient.List(ctx, apps, client.InNamespace(veltypes.DefaultKubeVelaNS))).Should(Succeed())
			count := 0
			for i := range apps.Items {
				if apps.Items[i].Name == s.echoOwnedApp() {
					count++
				}
			}
			Expect(count).Should(Equal(1), "a second enable must update in place, not create a duplicate")
		})

		It("a declarative install refuses to take over an imperatively installed addon", func() {
			runVelaCommandSucceed(root, "addon", "enable", s.echo, "--version", echoAddonVersion, "-y")

			Expect(applyWrappingApp(s.wrappingApp, s.echo, echoAddonVersion, "")).Should(Succeed())

			// Admitted, because the conflict is only discoverable at dispatch,
			// then never healthy. The inner Application keeps its original
			// owner rather than being adopted.
			Consistently(func(g Gomega) {
				owned := &v1beta1.Application{}
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echoOwnedApp(), Namespace: veltypes.DefaultKubeVelaNS}, owned)).Should(Succeed())
				g.Expect(owned.Labels["app.oam.dev/name"]).ShouldNot(Equal(s.wrappingApp),
					"the declarative install must not hijack an addon owned by the imperative one")
			}, 30*time.Second, pollPeriod).Should(Succeed())
		})

		// The reverse direction -- `vela addon enable` over an addon already
		// owned by an Application -- is deliberately not asserted here. It does
		// not refuse today: the enable succeeds and strips the owning
		// Application's label, orphaning the addon. See BUG-8 in
		// addon-e2e-testing/COMMON-BUGS.md. Asserting the documented contract
		// would add a permanently failing spec to this suite.

		It("a second Application declaring the same addon does not take ownership", func() {
			Expect(applyWrappingApp(s.wrappingApp, s.echo, echoAddonVersion, "")).Should(Succeed())
			expectAppHealthy(s.echoOwnedApp())

			secondApp := s.wrappingApp + "-2"
			Expect(applyWrappingApp(secondApp, s.echo, echoAddonVersion, "")).Should(Succeed())
			defer deleteApp(secondApp)

			Consistently(func(g Gomega) {
				owned := &v1beta1.Application{}
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echoOwnedApp(), Namespace: veltypes.DefaultKubeVelaNS}, owned)).Should(Succeed())
				g.Expect(owned.Labels["app.oam.dev/name"]).Should(Equal(s.wrappingApp),
					"the first owner keeps the addon; the second must not adopt it")
			}, 30*time.Second, pollPeriod).Should(Succeed())
		})
	})

	// --- Group C: version movement ---

	Context("version (group C)", Ordered, func() {
		s := versionScope

		AfterEach(func() {
			deleteApp(s.wrappingApp)
			expectAppGone(s.echoOwnedApp())
		})

		expectRenderedVersion := func(want string) {
			Eventually(func(g Gomega) {
				deploy := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echo, Namespace: s.echoNamespace}, deploy)).Should(Succeed())
				g.Expect(deploy.Labels["addons.oam.dev/version"]).Should(Equal(want))
			}, waitTimeout, pollPeriod).Should(Succeed())
		}

		It("a pinned version does not move when a newer one exists", func() {
			Expect(applyWrappingApp(s.wrappingApp, s.echo, echoAddonVersion, "")).Should(Succeed())
			expectAppHealthy(s.echoOwnedApp())
			expectRenderedVersion(echoAddonVersion)

			Consistently(func(g Gomega) {
				deploy := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.echo, Namespace: s.echoNamespace}, deploy)).Should(Succeed())
				g.Expect(deploy.Labels["addons.oam.dev/version"]).Should(Equal(echoAddonVersion))
			}, 30*time.Second, pollPeriod).Should(Succeed())
		})

		It("changing the pin moves the installed version in place", func() {
			Expect(applyWrappingApp(s.wrappingApp, s.echo, echoAddonVersion, "")).Should(Succeed())
			expectAppHealthy(s.echoOwnedApp())
			expectRenderedVersion(echoAddonVersion)

			Eventually(func(g Gomega) {
				app := &v1beta1.Application{}
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.wrappingApp, Namespace: veltypes.DefaultKubeVelaNS}, app)).Should(Succeed())
				app.Spec.Components[0].Properties = &runtime.RawExtension{
					Raw: []byte(fmt.Sprintf(`{"addon":%q,"registry":%q,"version":%q}`, s.echo, addonRegistryName, echoAddonUpgrade)),
				}
				g.Expect(k8sClient.Update(ctx, app)).Should(Succeed())
			}, 30*time.Second, time.Second).Should(Succeed())

			expectRenderedVersion(echoAddonUpgrade)
		})

		It("downgrading back to the older pin is allowed", func() {
			Expect(applyWrappingApp(s.wrappingApp, s.echo, echoAddonUpgrade, "")).Should(Succeed())
			expectAppHealthy(s.echoOwnedApp())
			expectRenderedVersion(echoAddonUpgrade)

			Eventually(func(g Gomega) {
				app := &v1beta1.Application{}
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.wrappingApp, Namespace: veltypes.DefaultKubeVelaNS}, app)).Should(Succeed())
				app.Spec.Components[0].Properties = &runtime.RawExtension{
					Raw: []byte(fmt.Sprintf(`{"addon":%q,"registry":%q,"version":%q}`, s.echo, addonRegistryName, echoAddonVersion)),
				}
				g.Expect(k8sClient.Update(ctx, app)).Should(Succeed())
			}, 30*time.Second, time.Second).Should(Succeed())

			expectRenderedVersion(echoAddonVersion)
		})

		It("a version that was never published leaves the Application unhealthy", func() {
			Expect(applyWrappingApp(s.wrappingApp, s.echo, "9.9.9", "")).Should(Succeed())

			Eventually(func(g Gomega) {
				app := &v1beta1.Application{}
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.wrappingApp, Namespace: veltypes.DefaultKubeVelaNS}, app)).Should(Succeed())
				g.Expect(app.Status.Phase).ShouldNot(Equal(oamcommon.ApplicationRunning))
				// status.Workflow is nil until the first step runs, so this has
				// to be polled rather than read once.
				g.Expect(app.Status.Workflow).ShouldNot(BeNil(), "workflow status not reported yet")
				// The failure names the addon, the version and the registry it
				// searched -- a bad version is a registry fact admission
				// deliberately does not fetch, so this is where it surfaces.
				found := false
				for _, step := range app.Status.Workflow.Steps {
					if strings.Contains(step.Message, "9.9.9") {
						found = true
					}
				}
				g.Expect(found).Should(BeTrue(), "the workflow step message must name the version that could not be resolved")
			}, waitTimeout, pollPeriod).Should(Succeed())
		})
	})

	// --- Group D: multiple addons in one Application ---

	Context("multiple addons in one Application (group D)", Ordered, func() {
		s := bundleScope

		AfterEach(func() {
			deleteApp(s.bundleApp)
			expectAppGone(s.echoOwnedApp())
			expectAppGone(s.configOwnedApp())
		})

		applyBundle := func() error {
			mk := func(addon, version string) oamcommon.ApplicationComponent {
				return oamcommon.ApplicationComponent{
					Name: addon,
					Type: "addon",
					Properties: &runtime.RawExtension{
						Raw: []byte(fmt.Sprintf(`{"addon":%q,"registry":%q,"version":%q}`, addon, addonRegistryName, version)),
					},
				}
			}
			return k8sClient.Create(ctx, &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: s.bundleApp, Namespace: veltypes.DefaultKubeVelaNS},
				Spec: v1beta1.ApplicationSpec{
					Components: []oamcommon.ApplicationComponent{
						mk(s.echo, echoAddonVersion),
						mk(s.config, "1.0.0"),
					},
				},
			})
		}

		It("installs every addon it declares", func() {
			Expect(applyBundle()).Should(Succeed())
			expectAppHealthy(s.bundleApp)
			expectAppHealthy(s.echoOwnedApp())
			expectAppHealthy(s.configOwnedApp())
		})

		It("reports one status entry per addon component", func() {
			Expect(applyBundle()).Should(Succeed())
			expectAppHealthy(s.bundleApp)

			Eventually(func(g Gomega) {
				app := &v1beta1.Application{}
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: s.bundleApp, Namespace: veltypes.DefaultKubeVelaNS}, app)).Should(Succeed())
				names := map[string]bool{}
				for _, svc := range app.Status.Services {
					names[svc.Name] = true
				}
				g.Expect(names).Should(HaveKey(s.echo))
				g.Expect(names).Should(HaveKey(s.config))
			}, waitTimeout, pollPeriod).Should(Succeed())
		})

		It("deleting the bundle removes every addon it installed", func() {
			Expect(applyBundle()).Should(Succeed())
			expectAppHealthy(s.echoOwnedApp())
			expectAppHealthy(s.configOwnedApp())

			deleteApp(s.bundleApp)
			expectAppGone(s.echoOwnedApp())
			expectAppGone(s.configOwnedApp())
		})
	})

	// --- Group E: admission refusals ---

	Context("admission refusals (group E)", Ordered, func() {
		s := admissionScope

		It("refuses a component naming a registry that is not configured", func() {
			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "reject-unknown-registry", Namespace: veltypes.DefaultKubeVelaNS},
				Spec: v1beta1.ApplicationSpec{
					Components: []oamcommon.ApplicationComponent{{
						Name:       echoAddonName,
						Type:       "addon",
						Properties: &runtime.RawExtension{Raw: []byte(`{"addon":"echo-server","registry":"does-not-exist"}`)},
					}},
				},
			}
			err := k8sClient.Create(ctx, app)
			Expect(err).Should(HaveOccurred())
			// The message lists the configured names rather than rendering as a
			// bare "not found", so the author does not have to guess.
			Expect(err.Error()).Should(ContainSubstring("not a configured addon registry"))
			Expect(err.Error()).Should(ContainSubstring(addonRegistryName))
		})

		It("refuses properties that cannot be decoded", func() {
			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "reject-malformed", Namespace: veltypes.DefaultKubeVelaNS},
				Spec: v1beta1.ApplicationSpec{
					Components: []oamcommon.ApplicationComponent{{
						Name:       echoAddonName,
						Type:       "addon",
						Properties: &runtime.RawExtension{Raw: []byte(`{"addon":"echo-server","version":1.0}`)},
					}},
				},
			}
			err := k8sClient.Create(ctx, app)
			Expect(err).Should(HaveOccurred(), "version must be a string; a number cannot decode")
			Expect(err.Error()).Should(ContainSubstring("addon component properties"))
		})

		It("admits a valid component", func() {
			// The negative control. Without it, a webhook rejecting everything
			// would pass every other spec in this Context.
			Expect(applyWrappingApp(s.wrappingApp, s.echo, echoAddonVersion, "")).Should(Succeed())
			defer func() {
				deleteApp(s.wrappingApp)
				expectAppGone(s.echoOwnedApp())
			}()
			expectAppHealthy(s.wrappingApp)
		})

		It("admits an unknown addon name and reports it at reconcile", func() {
			// Whether an addon exists is a registry fact; admission
			// deliberately does not fetch, so this surfaces in status rather
			// than being rejected.
			const appName = "unknown-addon"
			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: veltypes.DefaultKubeVelaNS},
				Spec: v1beta1.ApplicationSpec{
					Components: []oamcommon.ApplicationComponent{{
						Name: "no-such-addon",
						Type: "addon",
						Properties: &runtime.RawExtension{
							Raw: []byte(fmt.Sprintf(`{"addon":"no-such-addon","registry":%q}`, addonRegistryName)),
						},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			defer deleteApp(appName)

			Eventually(func(g Gomega) {
				current := &v1beta1.Application{}
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: appName, Namespace: veltypes.DefaultKubeVelaNS}, current)).Should(Succeed())
				g.Expect(current.Status.Phase).ShouldNot(Equal(oamcommon.ApplicationRunning))
			}, waitTimeout, pollPeriod).Should(Succeed())
		})
	})
})
