/*
Copyright 2022 The KubeVela Authors.

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

package crdvalidation_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kubevela/pkg/util/compression"
	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/kubevela/pkg/util/test/bootstrap"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"strconv"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/cmd/core/app/hooks/crdvalidation"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var _ = bootstrap.InitKubeBuilderForTest(bootstrap.WithCRDPath("./testdata"))

func TestCRDValidationHook(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CRD Validation Hook Suite")
}

var _ = Describe("CRD validation hook", func() {
	Context("with old CRD that lacks compression support", func() {
		It("should detect incompatible CRD when zstd compression is enabled", func() {
			featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultFeatureGate, features.ZstdApplicationRevision, true)
			featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultFeatureGate, features.GzipApplicationRevision, false)
			ctx := context.Background()
			Expect(k8s.EnsureNamespace(ctx, singleton.KubeClient.Get(), types.DefaultKubeVelaNS)).Should(Succeed())

			hook := crdvalidation.NewHook()
			Expect(hook.Name()).Should(Equal("CRDValidation"))

			err := hook.Run(ctx)
			Expect(err).ShouldNot(Succeed())
			// The old CRD doesn't preserve the application data at all, so we get a basic corruption error
			Expect(err.Error()).Should(ContainSubstring("the ApplicationRevision CRD is not updated"))
		})

		It("should detect incompatible CRD when gzip compression is enabled", func() {
			featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultFeatureGate, features.ZstdApplicationRevision, false)
			featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultFeatureGate, features.GzipApplicationRevision, true)
			ctx := context.Background()
			Expect(k8s.EnsureNamespace(ctx, singleton.KubeClient.Get(), types.DefaultKubeVelaNS)).Should(Succeed())

			hook := crdvalidation.NewHook()
			err := hook.Run(ctx)
			Expect(err).ShouldNot(Succeed())
			// The old CRD doesn't preserve the application data at all, so we get a basic corruption error
			Expect(err.Error()).Should(ContainSubstring("the ApplicationRevision CRD is not updated"))
		})
	})

	It("should skip validation when compression features are disabled", func() {
		featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultFeatureGate, features.ZstdApplicationRevision, false)
		featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultFeatureGate, features.GzipApplicationRevision, false)
		ctx := context.Background()

		hook := crdvalidation.NewHook()
		err := hook.Run(ctx)
		Expect(err).Should(Succeed())
	})

	Context("with dependency injection", func() {
		It("should use custom client when provided", func() {
			featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultFeatureGate, features.ZstdApplicationRevision, true)
			ctx := context.Background()

			// Create a fake client that simulates a CRD with compression support
			fakeClient := fake.NewClientBuilder().WithScheme(singleton.KubeClient.Get().Scheme()).Build()

			// Pre-create the namespace
			ns := &corev1.Namespace{}
			ns.Name = types.DefaultKubeVelaNS
			Expect(fakeClient.Create(ctx, ns)).Should(Succeed())

			// Use NewHookWithClient to inject the fake client
			hook := crdvalidation.NewHookWithClient(fakeClient)
			Expect(hook.Name()).Should(Equal("CRDValidation"))

			// Since fake client preserves all fields, validation should pass
			err := hook.Run(ctx)
			Expect(err).Should(Succeed())
		})
	})

	Context("error scenarios", func() {
		It("should handle Create errors gracefully", func() {
			featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultFeatureGate, features.ZstdApplicationRevision, true)
			ctx := context.Background()

			// Create a client that fails on Create operations
			mockClient := &mockFailingClient{
				Client:     fake.NewClientBuilder().WithScheme(singleton.KubeClient.Get().Scheme()).Build(),
				failCreate: true,
			}

			hook := crdvalidation.NewHookWithClient(mockClient)
			err := hook.Run(ctx)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring("failed to create test ApplicationRevision"))
		})

		It("should handle Get errors gracefully", func() {
			featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultFeatureGate, features.GzipApplicationRevision, true)
			ctx := context.Background()

			// Create a client that fails on Get operations
			mockClient := &mockFailingClient{
				Client:  fake.NewClientBuilder().WithScheme(singleton.KubeClient.Get().Scheme()).Build(),
				failGet: true,
			}

			// Pre-create the namespace
			ns := &corev1.Namespace{}
			ns.Name = types.DefaultKubeVelaNS
			Expect(mockClient.Client.Create(ctx, ns)).Should(Succeed())

			hook := crdvalidation.NewHookWithClient(mockClient)
			err := hook.Run(ctx)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring("failed to read test ApplicationRevision"))
		})

		It("should handle namespace creation errors", func() {
			featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultFeatureGate, features.ZstdApplicationRevision, true)
			ctx := context.Background()

			// Create a client that fails namespace operations
			mockClient := &mockFailingClient{
				Client:           fake.NewClientBuilder().WithScheme(singleton.KubeClient.Get().Scheme()).Build(),
				failNamespaceOps: true,
			}

			hook := crdvalidation.NewHookWithClient(mockClient)
			err := hook.Run(ctx)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring("runtime namespace"))
			Expect(err.Error()).Should(ContainSubstring("does not exist or is not accessible"))
		})
	})

	Context("cleanup verification", func() {
		It("should clean up test resources after validation", func() {
			featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultFeatureGate, features.ZstdApplicationRevision, true)
			ctx := context.Background()

			fakeClient := fake.NewClientBuilder().WithScheme(singleton.KubeClient.Get().Scheme()).Build()

			// Pre-create the namespace
			ns := &corev1.Namespace{}
			ns.Name = types.DefaultKubeVelaNS
			Expect(fakeClient.Create(ctx, ns)).Should(Succeed())

			hook := crdvalidation.NewHookWithClient(fakeClient)
			_ = hook.Run(ctx)

			// Verify that test ApplicationRevisions are cleaned up
			appRevList := &v1beta1.ApplicationRevisionList{}
			err := fakeClient.List(ctx, appRevList,
				client.InNamespace(types.DefaultKubeVelaNS),
				client.MatchingLabels{oam.LabelPreCheck: types.VelaCoreName})
			Expect(err).Should(Succeed())
			Expect(appRevList.Items).Should(HaveLen(0))
		})

		It("should clean up multiple test resources with same label", func() {
			featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultFeatureGate, features.GzipApplicationRevision, true)
			ctx := context.Background()

			fakeClient := fake.NewClientBuilder().WithScheme(singleton.KubeClient.Get().Scheme()).Build()

			// Pre-create the namespace
			ns := &corev1.Namespace{}
			ns.Name = types.DefaultKubeVelaNS
			Expect(fakeClient.Create(ctx, ns)).Should(Succeed())

			// Pre-create multiple test ApplicationRevisions with the precheck label
			for i := 0; i < 3; i++ {
				appRev := &v1beta1.ApplicationRevision{}
				appRev.Name = "old-test-" + strconv.Itoa(i)
				appRev.Namespace = types.DefaultKubeVelaNS
				appRev.SetLabels(map[string]string{oam.LabelPreCheck: types.VelaCoreName})
				appRev.Spec.Compression.Type = compression.Gzip
				Expect(fakeClient.Create(ctx, appRev)).Should(Succeed())
			}

			// Verify pre-existing resources
			appRevList := &v1beta1.ApplicationRevisionList{}
			err := fakeClient.List(ctx, appRevList,
				client.InNamespace(types.DefaultKubeVelaNS),
				client.MatchingLabels{oam.LabelPreCheck: types.VelaCoreName})
			Expect(err).Should(Succeed())
			Expect(appRevList.Items).Should(HaveLen(3))

			// Run the hook
			hook := crdvalidation.NewHookWithClient(fakeClient)
			_ = hook.Run(ctx)

			// Verify all test resources are cleaned up
			err = fakeClient.List(ctx, appRevList,
				client.InNamespace(types.DefaultKubeVelaNS),
				client.MatchingLabels{oam.LabelPreCheck: types.VelaCoreName})
			Expect(err).Should(Succeed())
			Expect(appRevList.Items).Should(HaveLen(0))
		})
	})
})

// mockFailingClient is a test client that can simulate various failure scenarios
type mockFailingClient struct {
	client.Client
	failCreate       bool
	failGet          bool
	failNamespaceOps bool
}

func (m *mockFailingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if m.failCreate {
		if _, ok := obj.(*v1beta1.ApplicationRevision); ok {
			return errors.New("simulated create failure")
		}
	}
	if m.failNamespaceOps {
		if _, ok := obj.(*corev1.Namespace); ok {
			return errors.New("simulated namespace creation failure")
		}
	}
	return m.Client.Create(ctx, obj, opts...)
}

func (m *mockFailingClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if m.failGet {
		if _, ok := obj.(*v1beta1.ApplicationRevision); ok {
			return errors.New("simulated get failure")
		}
	}
	if m.failNamespaceOps {
		if _, ok := obj.(*corev1.Namespace); ok {
			return apierrors.NewNotFound(corev1.SchemeGroupVersion.WithResource("namespaces").GroupResource(), key.Name)
		}
	}
	return m.Client.Get(ctx, key, obj, opts...)
}
