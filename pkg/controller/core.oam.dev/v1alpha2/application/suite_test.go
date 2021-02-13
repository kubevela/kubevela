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

package application

import (
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/applicationconfiguration"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.
var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var testScheme = runtime.NewScheme()
var reconciler *Reconciler
var stop = make(chan struct{})
var ctlManager ctrl.Manager

func TestAPIs(t *testing.T) {

	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

type NoOpReconciler struct {
	Log logr.Logger
}

func (r *NoOpReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	r.Log.Info("received a request", "object name", req.Name)
	return ctrl.Result{}, nil
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	rand.Seed(time.Now().UnixNano())
	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		UseExistingCluster: pointer.BoolPtr(false),
		CRDDirectoryPaths:  []string{filepath.Join("../../../../..", "charts", "vela-core", "crds")},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = v1alpha2.SchemeBuilder.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())

	err = v1alpha1.SchemeBuilder.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())

	err = scheme.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())
	dm, err := discoverymapper.New(cfg)
	Expect(err).To(BeNil())
	reconciler = &Reconciler{
		Client: k8sClient,
		Log:    ctrl.Log.WithName("Application-Test"),
		Scheme: testScheme,
		dm:     dm,
	}
	// setup the controller manager since we need the component handler to run in the background
	ctlManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                  testScheme,
		MetricsBindAddress:      ":8080",
		LeaderElection:          false,
		LeaderElectionNamespace: "default",
		LeaderElectionID:        "test",
	})
	Expect(err).NotTo(HaveOccurred())
	// start to run the no op reconciler that creates component revision
	err = ctrl.NewControllerManagedBy(ctlManager).
		Named("component").
		For(&v1alpha2.Component{}).
		Watches(&source.Kind{Type: &v1alpha2.Component{}}, &applicationconfiguration.ComponentHandler{
			Client:                ctlManager.GetClient(),
			Logger:                logging.NewLogrLogger(ctrl.Log.WithName("component-handler")),
			RevisionLimit:         100,
			CustomRevisionHookURL: "",
		}).Complete(&NoOpReconciler{
		Log: ctrl.Log.WithName("NoOp-Reconciler"),
	})
	Expect(err).NotTo(HaveOccurred())
	// start the controller in the background so that new componentRevisions are created
	go func() {
		err = ctlManager.Start(stop)
		Expect(err).NotTo(HaveOccurred())
	}()
	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
	close(stop)
})
