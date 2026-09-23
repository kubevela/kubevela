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

package config

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	configoam "github.com/oam-dev/kubevela/apis/config.oam.dev"
	"github.com/oam-dev/kubevela/pkg/controller/config.oam.dev/v1alpha1/configtemplate"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var controllerDone context.CancelFunc
var r Reconciler
var ctReconciler configtemplate.Reconciler

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}

var _ = BeforeSuite(func() {
	By("Bootstrapping test environment")
	useExistCluster := false
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths: []string{
			filepath.Join("../../../../..", "charts/vela-core/crds"),
		},
		UseExistingCluster: &useExistCluster,
	}
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	Expect(configoam.AddToScheme(scheme.Scheme)).Should(BeNil())

	By("Create the k8s client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	By("Ensure the vela-system namespace exists for legacy-style template lookups")
	err = k8sClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "vela-system"},
	})
	Expect(client.IgnoreAlreadyExists(err)).Should(BeNil())

	By("Starting the controllers in the background")
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
			Port: 48083,
		}),
	})
	Expect(err).ToNot(HaveOccurred())

	ctReconciler = configtemplate.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	Expect(ctReconciler.SetupWithManager(mgr)).ToNot(HaveOccurred())

	r = Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	Expect(r.SetupWithManager(mgr)).ToNot(HaveOccurred())

	var ctx context.Context
	ctx, controllerDone = context.WithCancel(context.Background())
	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(ctx)).ToNot(HaveOccurred())
	}()
})

var _ = AfterSuite(func() {
	By("Stop the controllers")
	controllerDone()

	By("Tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
