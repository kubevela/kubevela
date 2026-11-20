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

package controllers_test

import (
	"context"
	"encoding/json"
	"time"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	veltypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// The unmarked fixture at testdata/module/placement-flat has no placement
// field and no annotations, so every object in it is hub.
const (
	flatModuleName     = "placement-flat"
	flatModuleVersion  = "1.0.0"
	flatFixtureRelPath = "test/e2e-module-test/testdata/module/placement-flat"
	flatInstallAppName = "module-" + flatModuleName + "-install"
	flatOwnedAppName   = "module-" + flatModuleName
	flatCMConfig       = "placement-flat-config"

	// noMatchLabel is a cluster label no cluster in the suite carries.
	noMatchLabel = "e2e-placement=none"
)

// moduleApplication builds a type: module Application. place is the
// placement property; nil leaves it off.
func moduleApplication(name, namespace, module, registry, version string, place map[string]any) *v1beta1.Application {
	props := map[string]any{"module": module, "registry": registry, "version": version}
	if place != nil {
		props["placement"] = place
	}
	raw, err := json.Marshal(props)
	Expect(err).NotTo(HaveOccurred())
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{
				Name:       module,
				Type:       "module",
				Properties: &runtime.RawExtension{Raw: raw},
			}},
		},
	}
}

func getOwnedApp(ctx context.Context, name string) v1beta1.Application {
	var app v1beta1.Application
	Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: name, Namespace: veltypes.DefaultKubeVelaNS}, &app)).To(Succeed())
	return app
}

func topologyPolicyNames(app v1beta1.Application) []string {
	var names []string
	for _, policy := range app.Spec.Policies {
		if policy.Type == v1alpha1.TopologyPolicyType {
			names = append(names, policy.Name)
		}
	}
	return names
}

func stepPhase(app v1beta1.Application, name string) string {
	if app.Status.Workflow == nil {
		return ""
	}
	for _, step := range app.Status.Workflow.Steps {
		if step.Name == name {
			return string(step.Phase)
		}
	}
	return ""
}

var _ = Describe("Module placement inputs", Serial, func() {
	var repoRoot string

	BeforeEach(func() {
		repoRoot = modulePublishRepoRoot()
	})

	// The flags are checked before the registry is resolved, so these need no
	// registry and apply nothing.
	DescribeTable("vela module deploy rejects bad placement flags before anything is applied",
		func(args []string, want string) {
			out, err := runVelaCommand(repoRoot, append([]string{"module", "deploy", placementModuleName}, args...)...)
			Expect(err).To(HaveOccurred(), "output:\n%s", out)
			Expect(out).To(ContainSubstring(want))
			Expect(out).NotTo(ContainSubstring("Applied Application"))
		},
		Entry("both selection flags", []string{"--clusters", pkgmulticluster.Local, "--cluster-selector", "tier=runtime"},
			"--clusters and --cluster-selector cannot be used together"),
		Entry("allow-empty alone", []string{"--allow-empty"},
			"--allow-empty needs --clusters or --cluster-selector"),
		Entry("a blank cluster name", []string{"--clusters", pkgmulticluster.Local + ","},
			"placement clusters must not contain an empty name"),
	)

	// The module provider checks placement before its validation-only return,
	// so admission rejects these without fetching the module.
	DescribeTable("admission rejects a type: module Application with bad placement",
		func(place map[string]any, want string) {
			app := moduleApplication("placement-reject", "default", placementModuleName, "unused", placementModuleVersion, place)
			err := k8sClient.Create(context.Background(), app)
			if err == nil {
				_ = k8sClient.Delete(context.Background(), app)
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(want))
		},
		Entry("clusters and a selector", map[string]any{"clusters": []string{"spoke"}, "clusterLabelSelector": map[string]string{"tier": "runtime"}},
			"placement must set clusters or clusterLabelSelector, not both"),
		Entry("an empty placement", map[string]any{},
			"placement must set clusters or clusterLabelSelector"),
		Entry("an empty cluster list", map[string]any{"clusters": []string{}},
			"placement clusters must name at least one cluster"),
	)

	Context("with the fixtures published", Ordered, func() {
		var (
			ctx           = context.Background()
			testNamespace string
			registryName  string
		)

		BeforeAll(func() {
			repoRoot = modulePublishRepoRoot()
			testNamespace = randomNamespaceName("placement-inputs")
			registryName = randomNamespaceName("placement-inputs-reg")

			By("Removing leftover owned Applications from an earlier run")
			deletePlacementApp(ctx, placementOwnedAppName, veltypes.DefaultKubeVelaNS)
			deletePlacementApp(ctx, flatOwnedAppName, veltypes.DefaultKubeVelaNS)

			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			DeferCleanup(func() {
				for _, key := range []k8stypes.NamespacedName{
					{Name: placementInstallAppName, Namespace: testNamespace},
					{Name: flatInstallAppName, Namespace: testNamespace},
					{Name: placementOwnedAppName, Namespace: veltypes.DefaultKubeVelaNS},
					{Name: flatOwnedAppName, Namespace: veltypes.DefaultKubeVelaNS},
				} {
					_ = k8sClient.Delete(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace}})
				}
				_ = k8sClient.Delete(ctx, ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
			})

			By("Starting the in-cluster OCI registry")
			Expect(createOCIRegistry(ctx, k8sClient, testNamespace)).To(Succeed())
			waitForOCIRegistryDeploymentAvailable(ctx, testNamespace)
			registryURL := moduleE2ERegistryURL(ctx, testNamespace)
			registryBase, err := registryHTTPBase(registryURL)
			Expect(err).NotTo(HaveOccurred(), "registry URL: %s", registryURL)
			waitForOCIRegistryReachable(registryBase + "/v2/")

			By("Publishing both fixtures and registering the registry")
			runVelaCommandSucceed(repoRoot, "module", "publish", placementFixtureRelPath, registryURL)
			runVelaCommandSucceed(repoRoot, "module", "publish", flatFixtureRelPath, registryURL)
			runVelaCommandSucceed(repoRoot, "module", "registry", "add", registryName, registryURL, "--type", "oci")
			DeferCleanup(func() {
				_, _ = runVelaCommand(repoRoot, "module", "registry", "delete", registryName)
			})
		})

		deploy := func(args ...string) (string, error) {
			full := append([]string{"module", "deploy", placementModuleName, "--registry", registryName,
				"--version", placementModuleVersion, "-n", testNamespace}, args...)
			runCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
			defer cancel()
			return runVelaCommandContext(runCtx, repoRoot, full...)
		}
		runtimeCMs := []string{placementCMDefaults, placementCMKeepRuntime, placementCMV1Line}

		It("prints the placement property on deploy --dry-run", func() {
			out := runVelaCommandSucceed(repoRoot, "module", "deploy", placementModuleName, "--registry", registryName,
				"--version", placementModuleVersion, "--clusters", "spoke-a,spoke-b", "--dry-run")
			Expect(out).To(ContainSubstring("placement:"))
			Expect(out).To(ContainSubstring("- spoke-a"))
			Expect(out).To(ContainSubstring("- spoke-b"))

			out = runVelaCommandSucceed(repoRoot, "module", "deploy", placementModuleName, "--registry", registryName,
				"--version", placementModuleVersion, "--cluster-selector", "tier=runtime", "--allow-empty", "--dry-run")
			Expect(out).To(ContainSubstring("clusterLabelSelector:"))
			Expect(out).To(ContainSubstring("tier: runtime"))
			Expect(out).To(ContainSubstring("allowEmpty: true"))

			out = runVelaCommandSucceed(repoRoot, "module", "deploy", placementModuleName, "--registry", registryName,
				"--version", placementModuleVersion, "--dry-run")
			Expect(out).NotTo(ContainSubstring("placement:"), "no flags leaves the property off")
		})

		It("reports the failed runtime step when the selector matches no cluster", func() {
			out, err := deploy("--cluster-selector", noMatchLabel, "--timeout", "90s")
			Expect(err).To(HaveOccurred(), "output:\n%s", out)
			Expect(out).To(ContainSubstring("deploy-module-runtime failed"))
			Expect(out).To(ContainSubstring("failed to find any cluster"))
			Expect(out).NotTo(ContainSubstring("is installed"))

			owned := getOwnedApp(ctx, placementOwnedAppName)
			Expect(stepPhase(owned, "deploy-module-hub")).To(Equal("succeeded"))
			Expect(stepPhase(owned, "deploy-module-runtime")).To(Equal("failed"))
			expectPlacementConfigMap(ctx, testNamespace, placementCMAdmission, oam.PlacementHub)
			expectPlacementConfigMap(ctx, testNamespace, placementCMV1Controller, oam.PlacementHub)
			expectPlacementConfigMap(ctx, testNamespace, placementCMV2Migration, "")
			for _, name := range runtimeCMs {
				expectPlacementConfigMapAbsent(ctx, testNamespace, name)
			}
		})

		It("installs nothing on runtime clusters with --allow-empty and reports the tiers as not placed", func() {
			out, err := deploy("--cluster-selector", noMatchLabel, "--allow-empty")
			Expect(err).NotTo(HaveOccurred(), "output:\n%s", out)
			Expect(out).To(ContainSubstring("Not placed"))
			Expect(out).To(ContainSubstring("is installed"))

			owned := getOwnedApp(ctx, placementOwnedAppName)
			Expect(stepPhase(owned, "deploy-module-runtime")).To(Equal("succeeded"))
			for _, name := range runtimeCMs {
				expectPlacementConfigMapAbsent(ctx, testNamespace, name)
			}
		})

		It("puts runtime objects on the named cluster with --clusters", func() {
			out, err := deploy("--clusters", pkgmulticluster.Local)
			Expect(err).NotTo(HaveOccurred(), "output:\n%s", out)
			Expect(out).To(ContainSubstring("is installed"))

			owned := getOwnedApp(ctx, placementOwnedAppName)
			Expect(placementPolicy(owned, "module-runtime").Clusters).To(Equal([]string{pkgmulticluster.Local}))
			Eventually(func(g Gomega) {
				for _, name := range runtimeCMs {
					g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: name, Namespace: testNamespace}, &corev1.ConfigMap{})).To(Succeed(), name)
				}
			}, time.Minute, 2*time.Second).Should(Succeed())
		})

		It("installs an unmarked module flat, and splits it once placement is set", func() {
			By("Installing with no placement")
			app := moduleApplication(flatInstallAppName, testNamespace, flatModuleName, registryName, flatModuleVersion, nil)
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			var owned v1beta1.Application
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: flatOwnedAppName, Namespace: veltypes.DefaultKubeVelaNS}, &owned)).To(Succeed())
				g.Expect(owned.Status.Phase).To(Equal(common.ApplicationRunning))
			}, 5*time.Minute, 2*time.Second).Should(Succeed())
			Expect(topologyPolicyNames(owned)).To(BeEmpty(), "an unmarked module with no placement has no topology policies")
			Expect(owned.Spec.Workflow).To(BeNil(), "and no workflow")
			expectPlacementConfigMap(ctx, testNamespace, flatCMConfig, "")

			By("Setting placement.clusters on the same install")
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: flatInstallAppName, Namespace: testNamespace}, app)).To(Succeed())
			app.Spec.Components[0].Properties = moduleApplication("", "", flatModuleName, registryName, flatModuleVersion,
				map[string]any{"clusters": []string{pkgmulticluster.Local}}).Spec.Components[0].Properties
			Expect(k8sClient.Update(ctx, app)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: flatOwnedAppName, Namespace: veltypes.DefaultKubeVelaNS}, &owned)).To(Succeed())
				g.Expect(topologyPolicyNames(owned)).To(ConsistOf("module-hub", "module-runtime"))
				g.Expect(owned.Status.Phase).To(Equal(common.ApplicationRunning))
			}, 5*time.Minute, 2*time.Second).Should(Succeed())
			Expect(placementPolicy(owned, "module-runtime-override").Selector).To(BeEmpty(), "nothing in the module is runtime")
			expectPlacementConfigMap(ctx, testNamespace, flatCMConfig, "")
		})
	})
})
