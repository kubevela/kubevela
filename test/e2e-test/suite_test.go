/*
Copyright 2021 The KubeVela Authors.

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
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	controllerscheme "sigs.k8s.io/controller-runtime/pkg/scheme"

	configoam "github.com/oam-dev/kubevela/apis/config.oam.dev"
	core "github.com/oam-dev/kubevela/apis/core.oam.dev"
	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	// +kubebuilder:scaffold:imports
)

var k8sClient client.Client
var scheme = runtime.NewScheme()
var authSetupStarted bool

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "OAM Core Resource Controller Suite")
}

func bootstrapCoreClient() {
	if k8sClient != nil {
		return
	}
	By("Bootstrapping test environment")
	rand.Seed(time.Now().UnixNano())
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	err := clientgoscheme.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = core.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = crdv1.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = kruise.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = configoam.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	depExample := &unstructured.Unstructured{}
	depExample.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "example.com",
		Version: "v1",
		Kind:    "Foo",
	})
	depSchemeGroupVersion := schema.GroupVersion{Group: "example.com", Version: "v1"}
	depSchemeBuilder := &controllerscheme.Builder{GroupVersion: depSchemeGroupVersion}
	depSchemeBuilder.Register(depExample.DeepCopyObject())
	err = depSchemeBuilder.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	By("Setting up kubernetes client")
	k8sClient, err = client.New(config.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		logf.Log.Error(err, "failed to create k8sClient")
		Fail("setup failed")
	}
	By("Finished setting up test environment")
}

var _ = SynchronizedBeforeSuite(func() {
	bootstrapCoreClient()

	// create workload definition for 'deployments'
	wdDeploy := v1beta1.WorkloadDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deployments.apps",
			Namespace: "vela-system",
		},
		Spec: v1beta1.WorkloadDefinitionSpec{
			Reference: commontypes.DefinitionReference{
				Name: "deployments.apps",
			},
		},
	}
	Expect(k8sClient.Create(context.Background(), &wdDeploy)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	By("Created deployments.apps")

	var token [12]byte
	_, err := cryptorand.Read(token[:])
	Expect(err).NotTo(HaveOccurred())
	runID := hex.EncodeToString(token[:])
	ownedRBAC, err := installCoreSuiteRBAC(context.Background(), k8sClient, runID)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() {
		Expect(ownedRBAC.cleanup(context.Background(), k8sClient)).To(Succeed())
	})
	By("Created example.com cluster role and binding for the test service account")

	if os.Getenv("KUBEVELA_E2E_AUTH") == "1" {
		var ns corev1.Namespace
		err := k8sClient.Get(context.Background(), client.ObjectKey{Name: authTestNamespace}, &ns)
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "auth-test namespace already exists or could not be checked: %v", err)
		authSetupStarted = true
		By("Bringing up auth-test registries")
		Expect(setupAuthRegistries(context.Background(), k8sClient)).To(Succeed())

		By("Pushing test chart to auth-test registries")
		cfg, err := authTestRestConfig()
		Expect(err).NotTo(HaveOccurred())
		Expect(pushTestChartToRegistries(context.Background(), cfg)).To(Succeed())
	} else {
		By("Skipping auth-test registries setup (KUBEVELA_E2E_AUTH not set)")
	}
}, func() {
	bootstrapCoreClient()
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	By("Tearing down the test environment")
	if authSetupStarted {
		By("Tearing down auth-test registries")
		Expect(tearDownAuthRegistries(context.Background(), k8sClient)).To(Succeed())
	}
})

// RequestReconcileNow will trigger an immediate reconciliation on K8s object.
// Some test cases may fail for timeout to wait a scheduled reconciliation.
// This is a workaround to avoid long-time wait before next scheduled
// reconciliation.
func RequestReconcileNow(ctx context.Context, o client.Object) {
	oCopy := o.DeepCopyObject()
	oMeta, ok := oCopy.(metav1.Object)
	Expect(ok).Should(BeTrue())
	oMeta.SetAnnotations(map[string]string{
		"app.oam.dev/requestreconcile": time.Now().String(),
	})
	oMeta.SetResourceVersion("")
	By(fmt.Sprintf("Request reconcile %q now", oMeta.GetName()))
	Expect(k8sClient.Patch(ctx, oCopy.(client.Object), client.Merge)).Should(Succeed())
}

// randomNamespaceName generates an independent DNS-label name for each case.
// Empty prefixes are used as suffixes by the PostDispatch fixtures; keep that
// suffix short enough for their longest object name.
func randomNamespaceName(basic string) string {
	var token [8]byte
	if _, err := cryptorand.Read(token[:]); err != nil {
		panic(fmt.Errorf("generate test name: %w", err))
	}
	suffix := "-" + hex.EncodeToString(token[:])
	if basic == "" {
		return suffix
	}
	worker := fmt.Sprintf("-p%d", GinkgoParallelProcess())
	suffix = worker + suffix
	var readable strings.Builder
	lastWasSeparator := false
	for _, char := range strings.ToLower(basic) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			readable.WriteRune(char)
			lastWasSeparator = false
		} else if readable.Len() > 0 && !lastWasSeparator {
			readable.WriteByte('-')
			lastWasSeparator = true
		}
	}
	prefix := strings.Trim(readable.String(), "-")
	if prefix == "" {
		prefix = "e2e"
	}
	if len(prefix) > 63-len(suffix) {
		prefix = strings.TrimRight(prefix[:63-len(suffix)], "-")
	}
	return prefix + suffix
}
