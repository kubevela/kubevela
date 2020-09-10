/*


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

package metrics

import (
	"context"
	"path/filepath"
	"testing"

	monitoringv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	oamCore "github.com/crossplane/oam-kubernetes-runtime/apis/core"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	standardv1alpha1 "github.com/cloud-native-application/rudrx/api/v1alpha1"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var controllerDone chan struct{}
var serviceMonitorNS corev1.Namespace

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	serviceMonitorNS = corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ServiceMonitorNSName,
		},
	}
	By("Bootstrapping test environment")
	useExistCluster := true
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("../../../..", "charts/third_party/prometheus"), // this has all the required CRDs,
			filepath.Join("../../../..", "charts/vela-core/crds"),         // this has all the required CRDs,
		},
		UseExistingCluster: &useExistCluster,
	}
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = standardv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = monitoringv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = oamCore.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme
	By("Create the k8s client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	By("Starting the metrics trait controller in the background")
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme.Scheme,
		MetricsBindAddress: "0",
		Port:               9443,
		LeaderElection:     false,
		LeaderElectionID:   "9f6dad5a.oam.dev",
	})
	Expect(err).ToNot(HaveOccurred())
	r := Reconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("MetricsTrait"),
		Scheme: mgr.GetScheme(),
	}
	Expect(r.SetupWithManager(mgr)).ToNot(HaveOccurred())
	controllerDone = make(chan struct{}, 1)
	// +kubebuilder:scaffold:builder
	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(controllerDone)).ToNot(HaveOccurred())
	}()

	By("Create the serviceMonitor namespace")
	Expect(k8sClient.Create(context.Background(), &serviceMonitorNS)).ToNot(HaveOccurred())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("Stop the metricTrait controller")
	close(controllerDone)
	By("Delete the serviceMonitor namespace")
	Expect(k8sClient.Delete(context.Background(), &serviceMonitorNS,
		client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
	By("Tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())

})
