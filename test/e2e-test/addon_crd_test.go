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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

const (
	addonFinalizer  = "addon.oam.dev/cleanup"
	addonPauseLabel = "controller.core.oam.dev/pause"
	mockRegistry    = "KubeVela"
)

// newAddon builds an Addon CR pointing at the mock registry. skipVersionCheck
// is set so installs do not depend on a vela-core Deployment being present.
func newAddon(name, version, registry string) *v1beta1.Addon {
	return &v1beta1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1beta1.AddonSpec{
			Version:          version,
			Registry:         registry,
			SkipVersionCheck: true,
		},
	}
}

// exampleParams supplies the required parameter.example input for the mock
// "example" addon, whose resources/parameter.cue declares `example: string`
// with no default (render fails without it).
func exampleParams() *runtime.RawExtension {
	return &runtime.RawExtension{Raw: []byte(`{"example":"e2e"}`)}
}

// getAddon fetches the current Addon, returning nil on NotFound.
func getAddon(ctx context.Context, name string) *v1beta1.Addon {
	ad := &v1beta1.Addon{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: name}, ad); err != nil {
		return nil
	}
	return ad
}

// addonPhase returns the addon's phase, or "" when the addon is absent. It is
// nil-safe for use inside Eventually/Consistently closures (getAddon returns
// nil on NotFound, and dereferencing that would panic the polling goroutine).
func addonPhase(ctx context.Context, name string) v1beta1.AddonPhase {
	if ad := getAddon(ctx, name); ad != nil {
		return ad.Status.Phase
	}
	return ""
}

// condition returns the named condition's status/reason as "Status/Reason", or
// "" if absent.
func addonCondition(ad *v1beta1.Addon, condType string) string {
	if ad == nil {
		return ""
	}
	for _, c := range ad.Status.Conditions {
		if string(c.Type) == condType {
			return fmt.Sprintf("%s/%s", c.Status, c.Reason)
		}
	}
	return ""
}

var _ = Describe("Addon CRD lifecycle e2e", func() {
	ctx := context.Background()

	Context("CEL admission validation", func() {
		dryRun := func(ad *v1beta1.Addon) error {
			return k8sClient.Create(ctx, ad, client.DryRunAll)
		}

		It("rejects semver constraints in spec.version", func() {
			for _, v := range []string{">=1.2.0", "1.x", "latest"} {
				err := dryRun(newAddon("cel-ver", v, mockRegistry))
				Expect(err).To(HaveOccurred(), "version %q must be rejected", v)
				Expect(err.Error()).To(ContainSubstring("semver constraints are not supported"))
			}
		})

		It("accepts exact semver tags in spec.version", func() {
			for _, v := range []string{"v1.2.0", "1.0.0", "v1.2.0-rc.1"} {
				Expect(dryRun(newAddon("cel-ver", v, mockRegistry))).
					To(Succeed(), "version %q must be accepted", v)
			}
		})

		It("rejects Force and Orphan deletion policies", func() {
			for _, p := range []v1beta1.AddonDeletionPolicy{
				v1beta1.AddonDeletionPolicyForce, v1beta1.AddonDeletionPolicyOrphan,
			} {
				ad := newAddon("cel-del", "v1.0.0", mockRegistry)
				ad.Spec.DeletionPolicy = p
				err := dryRun(ad)
				Expect(err).To(HaveOccurred(), "policy %q must be rejected", p)
				Expect(err.Error()).To(ContainSubstring("only Protect is accepted"))
			}
		})

		It("accepts Protect and empty deletion policy", func() {
			adProtect := newAddon("cel-del", "v1.0.0", mockRegistry)
			adProtect.Spec.DeletionPolicy = v1beta1.AddonDeletionPolicyProtect
			Expect(dryRun(adProtect)).To(Succeed())
			Expect(dryRun(newAddon("cel-del", "v1.0.0", mockRegistry))).To(Succeed())
		})
	})

	Context("reconcile, install, and status", func() {
		var created []string

		AfterEach(func() {
			for _, name := range created {
				_ = k8sClient.Delete(ctx, &v1beta1.Addon{ObjectMeta: metav1.ObjectMeta{Name: name}})
				Eventually(func() *v1beta1.Addon { return getAddon(ctx, name) },
					180*time.Second, 3*time.Second).Should(BeNil(), "addon %q must be cleaned up", name)
			}
			created = nil
		})

		track := func(ad *v1beta1.Addon) *v1beta1.Addon { created = append(created, ad.Name); return ad }

		// The addon name (CR metadata.name) IS the registry lookup key, so
		// install-expecting specs must use a name the mock registry serves:
		// "example" (healthy) or "broken-render" (render failure).
		It("installs a healthy addon and reaches running", func() {
			ad := track(newAddon("example", "1.0.0", mockRegistry))
			ad.Spec.Parameters = exampleParams()
			Expect(k8sClient.Create(ctx, ad)).To(Succeed())

			Eventually(func() v1beta1.AddonPhase {
				return addonPhase(ctx, "example")
			}, 180*time.Second, 3*time.Second).Should(Equal(v1beta1.AddonPhaseRunning))

			got := getAddon(ctx, "example")
			Expect(got).NotTo(BeNil())
			Expect(addonCondition(got, v1beta1.AddonConditionReady)).To(Equal("True/Installed"))
			Expect(addonCondition(got, v1beta1.AddonConditionSourceResolved)).To(Equal("True/SourceFetched"))
			Expect(got.Status.InstalledVersion).To(Equal("1.0.0"))
			Expect(got.Finalizers).To(ContainElement(addonFinalizer))

			By("the owned Application addon-example exists")
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Namespace: "vela-system", Name: "addon-example"},
					&v1beta1.Application{})
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})

		It("holds in installing with SourceResolved=False when the registry is unreachable", func() {
			Expect(k8sClient.Create(ctx, track(newAddon("e2e-badreg", "1.0.0", "NoSuchRegistry")))).To(Succeed())

			Eventually(func() string {
				return addonCondition(getAddon(ctx, "e2e-badreg"), v1beta1.AddonConditionSourceResolved)
			}, 60*time.Second, 3*time.Second).Should(Equal("False/RegistryUnreachable"))

			got := getAddon(ctx, "e2e-badreg")
			Expect(got.Status.Phase).NotTo(Equal(v1beta1.AddonPhaseRunning))
			readyMsg := ""
			for _, c := range got.Status.Conditions {
				if string(c.Type) == v1beta1.AddonConditionSourceResolved {
					readyMsg = c.Message
				}
			}
			Expect(readyMsg).To(ContainSubstring("NoSuchRegistry"))
		})

		It("fails with InstallFailed and SourceResolved=True on a render error", func() {
			// broken-render exists in the registry (fetch succeeds) but its CUE
			// fails to render → non-fetch error → InstallFailed, SourceResolved=True.
			Expect(k8sClient.Create(ctx, track(newAddon("broken-render", "1.0.0", mockRegistry)))).To(Succeed())
			Eventually(func() v1beta1.AddonPhase {
				return addonPhase(ctx, "broken-render")
			}, 90*time.Second, 3*time.Second).Should(Equal(v1beta1.AddonPhaseFailed))

			got := getAddon(ctx, "broken-render")
			Expect(got).NotTo(BeNil())
			Expect(addonCondition(got, v1beta1.AddonConditionSourceResolved)).To(Equal("True/SourceFetched"))
			Expect(addonCondition(got, v1beta1.AddonConditionReady)).To(Equal("False/InstallFailed"))
		})

		It("skips reconciliation when paused", func() {
			ad := track(newAddon("e2e-paused", "1.0.0", mockRegistry))
			ad.Labels = map[string]string{addonPauseLabel: "true"}
			Expect(k8sClient.Create(ctx, ad)).To(Succeed())

			By("status makes no progress while paused (longer than one resync would take)")
			Consistently(func() v1beta1.AddonPhase {
				return addonPhase(ctx, "e2e-paused") // "" whether absent or status-empty
			}, 45*time.Second, 5*time.Second).Should(Equal(v1beta1.AddonPhase("")),
				"a paused addon must not advance to installing/running")
		})
	})

	Context("deletion (Protect policy)", func() {
		// Both specs install the registry's "example" addon (the name is the
		// lookup key) and fully delete it, so they are safe to run serially.
		const addonName = "example"
		const ownedApp = "addon-example"

		install := func() {
			ad := newAddon(addonName, "1.0.0", mockRegistry)
			ad.Spec.Parameters = exampleParams()
			Expect(k8sClient.Create(ctx, ad)).To(Succeed())
			Eventually(func() v1beta1.AddonPhase {
				return addonPhase(ctx, addonName)
			}, 180*time.Second, 3*time.Second).Should(Equal(v1beta1.AddonPhaseRunning))
		}

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "uses-helm-example", Namespace: "default"}})
			_ = k8sClient.Delete(ctx, &v1beta1.Addon{ObjectMeta: metav1.ObjectMeta{Name: addonName}})
			Eventually(func() *v1beta1.Addon { return getAddon(ctx, addonName) },
				180*time.Second, 5*time.Second).Should(BeNil(), "addon must be cleaned up between specs")
		})

		It("deletes the owned Application and then the CR when there are no dependents", func() {
			install()
			Expect(k8sClient.Delete(ctx, &v1beta1.Addon{ObjectMeta: metav1.ObjectMeta{Name: addonName}})).To(Succeed())

			By("owned Application is removed")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "vela-system", Name: ownedApp},
					&v1beta1.Application{})
				return err != nil
			}, 120*time.Second, 3*time.Second).Should(BeTrue())

			By("CR is removed (finalizer released)")
			Eventually(func() *v1beta1.Addon { return getAddon(ctx, addonName) },
				120*time.Second, 3*time.Second).Should(BeNil())
		})

		It("blocks deletion while a dependent Application exists, then completes once unblocked", func() {
			install()

			By("create an Application that uses the addon's helm-example component")
			dep := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "uses-helm-example", Namespace: "default"},
			}
			dep.Spec.Components = []common.ApplicationComponent{{
				Name: "c1", Type: "helm-example",
				Properties: &runtime.RawExtension{Raw: []byte(`{"repoType":"git","url":"https://example.com/repo","chart":"x"}`)},
			}}
			Expect(k8sClient.Create(ctx, dep)).To(Succeed())

			By("deleting the addon is blocked")
			Expect(k8sClient.Delete(ctx, &v1beta1.Addon{ObjectMeta: metav1.ObjectMeta{Name: addonName}})).To(Succeed())
			var got *v1beta1.Addon
			Eventually(func() v1beta1.AddonPhase {
				got = getAddon(ctx, addonName)
				if got == nil {
					return ""
				}
				return got.Status.Phase
			}, 60*time.Second, 3*time.Second).Should(Equal(v1beta1.AddonPhaseDeleting))
			Expect(addonCondition(got, v1beta1.AddonConditionReady)).To(Equal("False/DeletionBlocked"))
			Expect(got.Finalizers).To(ContainElement(addonFinalizer))

			By("remove the dependent and nudge the CR (F1: no Application watch)")
			Expect(k8sClient.Delete(ctx, dep)).To(Succeed())
			Eventually(func() error { // nudge until the CR is gone
				cur := getAddon(ctx, addonName)
				if cur == nil {
					return nil
				}
				if cur.Annotations == nil {
					cur.Annotations = map[string]string{}
				}
				cur.Annotations["e2e.nudge"] = fmt.Sprintf("%d", time.Now().UnixNano())
				_ = k8sClient.Update(ctx, cur)
				return fmt.Errorf("still present")
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})
	})
})
