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

// This suite is hermetic: it serves the "example" addon straight off disk from
// the existing e2e/addon/mock/testdata fixture (the same fixture the
// e2e/addon mock server uses) via an in-process httptest.Server (see
// addon_mock_registry_test.go), and registers that server as an OSS-type addon
// registry directly through the RegistryDataStore, in a BeforeEach/AfterEach
// scoped to this Describe block. It does not depend on any externally started
// process (e2e/addon/mock is a `package main` and cannot be imported, and
// running it as a subprocess would reintroduce a process-lifetime dependency),
// so there is no cross-step process-lifetime or ordering concern in CI, and it
// does not reach any external registry.
//
//   - The registry is added under its own name (not "KubeVela") so this test
//     never touches the chart-default "KubeVela" registry that other specs in
//     this suite may rely on.
//   - It installs the "example" addon (served by the mock registry): it is
//     renderable (namespace + resources) and, being absent from the
//     imperative pre-enable in the e2e setup, avoids a child-Application name
//     collision on addon-<name>.
//   - "skipVersionValidate: true" is set on the component properties so the
//     addon's SystemRequirements check does not fail when the controller's
//     reported version cannot satisfy it; this mirrors the imperative
//     "vela addon enable --skip-version-validating" escape hatch.

package controllers_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Addon as component e2e", func() {
	ctx := context.Background()
	var mockServer *httptest.Server

	const (
		systemNamespace = "vela-system"
		// wrapping Application that declares the addon as a component.
		wrappingAppName = "comp-example"
		// addonRegistry is registered against an in-process mock OSS server in
		// BeforeEach below (see addon_mock_registry_test.go), not the
		// chart-default "KubeVela" registry, so this test never depends on or
		// mutates that shared registry.
		addonRegistry = "e2e-mock-oss"
		// addonMockTestdataDir is the real e2e/addon/mock testdata fixture tree,
		// served directly off disk by our own in-process mock OSS server; the
		// addon lives at addonMockTestdataDir/addonName. Reusing this directory
		// (rather than a copy) means there is only one "example" fixture to keep
		// in sync.
		addonMockTestdataDir = "../../e2e/addon/mock/testdata"
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
	// registry and the skipVersionValidate escape hatch are threaded through to
	// the addon renderer.
	buildWrappingApp := func() *v1beta1.Application {
		return &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      wrappingAppName,
				Namespace: systemNamespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: addonName,
						Type: "addon",
						// properties.example is the "example" addon's own required
						// parameter, threaded through the addon component's
						// pass-through properties field.
						Properties: &runtime.RawExtension{Raw: []byte(`{"registry":"` + addonRegistry + `","skipVersionValidate":true,"properties":{"example":"e2e"}}`)},
					},
				},
			},
		}
	}

	BeforeEach(func() {
		By("Starting the in-process mock OSS addon server")
		server, err := newMockOSSAddonServer(addonMockTestdataDir)
		Expect(err).NotTo(HaveOccurred())
		mockServer = server

		By("Registering the mock server as an OSS addon registry")
		registryDS := pkgaddon.NewRegistryDataStore(k8sClient)
		Expect(registryDS.AddRegistry(ctx, pkgaddon.Registry{
			Name: addonRegistry,
			OSS: &pkgaddon.OSSAddonSource{
				Endpoint: mockServer.URL,
			},
		})).To(Succeed())
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

		By("Removing the mock OSS addon registry and stopping its server")
		registryDS := pkgaddon.NewRegistryDataStore(k8sClient)
		Expect(registryDS.DeleteRegistry(ctx, addonRegistry)).To(Succeed())
		mockServer.Close()
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
			if wrapping.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("wrapping application phase is %q, want %q", wrapping.Status.Phase, common.ApplicationRunning)
			}
			return nil
		}, waitTimeout, pollPeriod).Should(BeNil())

		By("Waiting for the child addon Application to exist and reach the running phase")
		Eventually(func() error {
			childApp := new(v1beta1.Application)
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: systemNamespace, Name: childAppName}, childApp); err != nil {
				return err
			}
			if childApp.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("child application phase is %q, want %q", childApp.Status.Phase, common.ApplicationRunning)
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
