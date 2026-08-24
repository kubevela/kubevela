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

// This suite lives in e2e/addon-component so it runs as part of
// `make e2e-api-test`'s `ginkgo -v -skipPackage capability,setup,application -r
// e2e` (the "Run API e2e tests" step of .github/actions/e2e-test/action.yaml),
// at the point in the CI pipeline where `make e2e-setup-core` has already
// started the e2e/addon/mock server and pointed the real "KubeVela" addon
// registry at it. That server's embedded testdata includes the "example"
// addon this suite installs; e2e/addon/addon_test.go's `vela addon ls` output
// in that same step lists "example ... KubeVela ..." successfully, confirming
// the registry is live by the time suites in this step run.
//
// BeforeEach only sanity-checks that "KubeVela" resolves to a reachable OSS
// endpoint before proceeding, failing fast with a clear message if not,
// rather than trying to start or manage the mock server itself.
//
//   - It installs the "example" addon: it is renderable (namespace +
//     resources) and, being absent from the imperative pre-enable in the e2e
//     setup, avoids a child-Application name collision on addon-<name>.
//   - "skipVersionValidation: true" is set on the component properties so the
//     addon's SystemRequirements check does not fail when the controller's
//     reported version cannot satisfy it; this mirrors the imperative
//     "vela addon enable --skip-version-validating" escape hatch.

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Addon as component e2e", func() {
	args := common.Args{Schema: common.Scheme}
	k8sClient, err := args.GetClient()
	Expect(err).Should(BeNil())

	ctx := context.Background()

	const (
		systemNamespace = "vela-system"
		// wrapping Application that declares the addon as a component.
		wrappingAppName = "comp-example"
		// addonRegistry is the real, chart-default registry that
		// e2e/addon/mock's main() points at itself when it starts (see
		// e2e/addon/mock/utils.ApplyMockServerConfig).
		addonRegistry = "KubeVela"
		// the addon's own name and the child Application RenderApp produces
		// (RenderApp forces the name to addon-<name> in vela-system).
		addonName    = "example"
		childAppName = "addon-example"
		// an auxiliary the example addon renders: the helm-example ComponentDefinition.
		helmCompDefName = "helm-example"

		waitTimeout = 300 * time.Second
		pollPeriod  = 5 * time.Second
	)

	// buildWrappingApp constructs the wrapping Application with a single
	// type: addon component. Properties are carried as a RawExtension so the
	// registry and the skipVersionValidation escape hatch are threaded through to
	// the addon renderer.
	buildWrappingApp := func() *v1beta1.Application {
		return &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      wrappingAppName,
				Namespace: systemNamespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []oamcommon.ApplicationComponent{
					{
						Name: addonName,
						Type: "addon",
						// properties.example is the "example" addon's own required
						// parameter, threaded through the addon component's
						// pass-through properties field.
						Properties: &runtime.RawExtension{Raw: []byte(`{"registry":"` + addonRegistry + `","skipVersionValidation":true,"properties":{"example":"e2e"}}`)},
					},
				},
			},
		}
	}

	BeforeEach(func() {
		By("Confirming the KubeVela addon registry resolves to a reachable mock server")
		Eventually(func() error {
			reg, err := pkgaddon.NewRegistryDataStore(k8sClient).GetRegistry(ctx, addonRegistry)
			if err != nil {
				return err
			}
			if reg.OSS == nil {
				return fmt.Errorf("registry %q is not OSS-backed (got %+v); expected the e2e/addon/mock server", addonRegistry, reg)
			}
			// An explicit timeout matters here: Eventually cannot preempt a
			// function that is already running, so a hung dial on the default
			// client would block past the 30s deadline and stall the suite
			// instead of failing it.
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, reg.OSS.Endpoint, nil)
			if err != nil {
				return err
			}
			resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
			if err != nil {
				return fmt.Errorf("mock addon server at %q is not reachable: %w (has `make e2e-setup-core` run yet?)", reg.OSS.Endpoint, err)
			}
			defer resp.Body.Close()
			// A completed connection is not a working server: without this the
			// probe passes against anything that answers, including a 404 from an
			// unrelated service on the port.
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("mock addon server at %q answered %s; expected 200", reg.OSS.Endpoint, resp.Status)
			}
			return nil
		}, 30*time.Second, time.Second).Should(Succeed())
	})

	AfterEach(func() {
		By("Deleting the wrapping application")
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: wrappingAppName, Namespace: systemNamespace},
		}
		Expect(k8sClient.Delete(ctx, app)).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))

		By("Waiting for the child addon Application to be garbage-collected")
		Eventually(func() error {
			childApp := new(v1beta1.Application)
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: systemNamespace, Name: childAppName}, childApp)
			if err == nil {
				return fmt.Errorf("child application %q still exists", childAppName)
			}
			if !apierrors.IsNotFound(err) {
				return err
			}
			return nil
		}, waitTimeout, pollPeriod).Should(BeNil())
	})

	It("installs an addon declared as a component, tracks it, and heals its auxiliaries", func() {
		By("Applying the wrapping application with a single type: addon component")
		app := buildWrappingApp()
		Eventually(func() error {
			return k8sClient.Create(ctx, app)
		}, 15*time.Second, time.Second).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

		By("Waiting for the wrapping application to reach the running phase")
		Eventually(func() error {
			wrapping := new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: systemNamespace, Name: wrappingAppName}, wrapping); err != nil {
				return err
			}
			if wrapping.Status.Phase != oamcommon.ApplicationRunning {
				return fmt.Errorf("wrapping application phase is %q, want %q", wrapping.Status.Phase, oamcommon.ApplicationRunning)
			}
			return nil
		}, waitTimeout, pollPeriod).Should(BeNil())

		By("Waiting for the child addon Application to exist and reach the running phase")
		Eventually(func() error {
			childApp := new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: systemNamespace, Name: childAppName}, childApp); err != nil {
				return err
			}
			if childApp.Status.Phase != oamcommon.ApplicationRunning {
				return fmt.Errorf("child application phase is %q, want %q", childApp.Status.Phase, oamcommon.ApplicationRunning)
			}
			return nil
		}, waitTimeout, pollPeriod).Should(BeNil())

		By("Asserting the helm ComponentDefinition auxiliary exists in vela-system")
		Eventually(func() error {
			cd := new(v1beta1.ComponentDefinition)
			return k8sClient.Get(ctx, types.NamespacedName{Namespace: systemNamespace, Name: helmCompDefName}, cd)
		}, waitTimeout, pollPeriod).Should(BeNil())

		By("Asserting the wrapping application's ResourceTracker records ONLY the child addon Application")
		Eventually(func() error {
			rt := new(v1beta1.ResourceTracker)
			if err := k8sClient.Get(ctx, generateResourceTrackerKey(systemNamespace, wrappingAppName, 1), rt); err != nil {
				return err
			}
			var recordsChild, recordsHelm bool
			for _, mr := range rt.Spec.ManagedResources {
				if mr.Kind == v1beta1.ApplicationKind && mr.Name == childAppName {
					recordsChild = true
				}
				if mr.Kind == v1beta1.ComponentDefinitionKind && mr.Name == helmCompDefName {
					recordsHelm = true
				}
			}
			if !recordsChild {
				return fmt.Errorf("outer resourceTracker %q does not record child Application %q", rt.Name, childAppName)
			}
			if recordsHelm {
				return fmt.Errorf("outer resourceTracker %q must NOT record auxiliary %q (it belongs to the inner app now)", rt.Name, helmCompDefName)
			}
			return nil
		}, waitTimeout, pollPeriod).Should(BeNil())

		By("Asserting the child addon Application's ResourceTracker records the helm auxiliary")
		Eventually(func() error {
			rt := new(v1beta1.ResourceTracker)
			if err := k8sClient.Get(ctx, generateResourceTrackerKey(systemNamespace, childAppName, 1), rt); err != nil {
				return err
			}
			for _, mr := range rt.Spec.ManagedResources {
				if mr.Kind == v1beta1.ComponentDefinitionKind && mr.Name == helmCompDefName {
					return nil
				}
			}
			return fmt.Errorf("inner resourceTracker %q does not record auxiliary %q", rt.Name, helmCompDefName)
		}, waitTimeout, pollPeriod).Should(BeNil())

		By("Deleting the helm ComponentDefinition auxiliary and asserting StateKeep heals it back")
		cd := &v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: helmCompDefName, Namespace: systemNamespace},
		}
		Expect(k8sClient.Delete(ctx, cd)).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		Eventually(func() error {
			healed := new(v1beta1.ComponentDefinition)
			return k8sClient.Get(ctx, types.NamespacedName{Namespace: systemNamespace, Name: helmCompDefName}, healed)
		}, waitTimeout, pollPeriod).Should(BeNil())
	})
})

// generateResourceTrackerKey builds the deterministic ResourceTracker name
// KubeVela generates for a given Application revision: "<app>-v<rev>-<ns>".
// Duplicated locally from test/e2e-test/app_resourcetracker_test.go, which is
// a different package this suite no longer depends on.
func generateResourceTrackerKey(namespace, appName string, revision int) types.NamespacedName {
	return types.NamespacedName{Name: fmt.Sprintf("%s-v%d-%s", appName, revision, namespace)}
}
