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

// Prerequisites for running this suite (it exercises a live cluster, so it is
// run manually, not in CI):
//
//   - A vela-core controller BUILT FROM THE feat/addon-component BRANCH is
//     running against the target cluster with the addon renderer wired in and
//     the ZstdResourceTracker feature gate enabled:
//     --feature-gates=ZstdResourceTracker=true
//   - The real addon registry ConfigMap (e.g. vela-addon-registry) is
//     installed in vela-system so "fluxcd" can be resolved from the registry.
//   - The "addon" ComponentDefinition (vela-templates/definitions/internal/
//     component/addon.cue) is installed in vela-system.
//   - "skipVersionValidate: true" is set on the component properties because
//     when vela-core runs out-of-cluster its reported version cannot satisfy
//     the addon's SystemRequirements check; skipping it mirrors the imperative
//     "vela addon enable --skip-version-validating" escape hatch.

package controllers_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Addon as component e2e", func() {
	ctx := context.Background()

	const (
		systemNamespace = "vela-system"
		// wrapping Application that declares the addon as a component.
		wrappingAppName = "comp-fluxcd"
		// the addon's own name and the child Application RenderApp produces
		// (RenderApp forces the name to addon-<name> in vela-system).
		addonName    = "fluxcd"
		childAppName = "addon-fluxcd"
		// an auxiliary the fluxcd addon renders: the helm ComponentDefinition.
		helmCompDefName = "helm"

		waitTimeout = 300 * time.Second
		pollPeriod  = 5 * time.Second
	)

	// buildWrappingApp constructs the wrapping Application with a single
	// type: addon component. Properties are carried as a RawExtension so the
	// version and the skipVersionValidate escape hatch are threaded through to
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
						Name:       addonName,
						Type:       "addon",
						Properties: &runtime.RawExtension{Raw: []byte(`{"version":"3.0.2","skipVersionValidate":true}`)},
					},
				},
			},
		}
	}

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
