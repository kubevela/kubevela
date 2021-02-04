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

package routes

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	certmanager "github.com/wonderflow/cert-manager-api/pkg/apis/certmanager/v1"
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

	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/applicationconfiguration"

	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"

	oamCore "github.com/oam-dev/kubevela/apis/core.oam.dev"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"

	standardv1alpha1 "github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/standard.oam.dev/v1alpha1/podspecworkload"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var controllerDone chan struct{}
var routeNS corev1.Namespace

var RouteNSName = "route-test"

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	routeNS = corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: RouteNSName,
		},
	}
	By("Bootstrapping test environment")
	useExistCluster := false
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("../../../../..", "charts/vela-core/crds"), // this has all the required CRDs,
			filepath.Join("..", "testdata/crds"),
		},
		UseExistingCluster: &useExistCluster,
	}
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	Expect(standardv1alpha1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(oamCore.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(certmanager.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme
	By("Create the k8s client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	By("Starting the route trait controller in the background")
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Port:   9443,
	})
	Expect(err).ToNot(HaveOccurred())
	r := Reconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("RouteTrait"),
		Scheme: mgr.GetScheme(),
	}
	Expect(r.SetupWithManager(mgr)).ToNot(HaveOccurred())
	Expect(applicationconfiguration.Setup(mgr, controller.Args{}, logging.NewLogrLogger(ctrl.Log.WithName("AppConfig")))).ToNot(HaveOccurred())
	Expect(podspecworkload.Setup(mgr)).ToNot(HaveOccurred())

	controllerDone = make(chan struct{}, 1)
	// +kubebuilder:scaffold:builder
	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(controllerDone)).ToNot(HaveOccurred())
	}()

	By("Create the routeTrait namespace")
	Expect(k8sClient.Create(context.Background(), &routeNS)).ToNot(HaveOccurred())
	routeDef := &v1alpha2.TraitDefinition{}
	routeDef.Name = "route"
	routeDef.Namespace = RouteNSName
	routeDef.Spec.Reference.Name = "routes.standard.oam.dev"
	routeDef.Spec.WorkloadRefPath = "spec.workloadRef"
	Expect(k8sClient.Create(context.Background(), routeDef)).ToNot(HaveOccurred())

	webservice := &v1alpha2.WorkloadDefinition{}
	webservice.Name = "webservice"
	webservice.Namespace = RouteNSName
	webservice.Spec.Reference.Name = "deployments.apps"
	webservice.Spec.ChildResourceKinds = []v1alpha2.ChildResourceKind{{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
	}, {
		APIVersion: "v1",
		Kind:       "Service",
	}}
	Expect(k8sClient.Create(context.Background(), webservice)).ToNot(HaveOccurred())

	deployment := &v1alpha2.WorkloadDefinition{}
	deployment.Name = "deployment"
	deployment.Namespace = RouteNSName
	deployment.Labels = map[string]string{"workload.oam.dev/podspecable": "true"}
	deployment.Spec.Reference.Name = "deployments.apps"
	Expect(k8sClient.Create(context.Background(), deployment)).ToNot(HaveOccurred())

	deploy := &v1alpha2.WorkloadDefinition{}
	deploy.Name = "deploy"
	deploy.Namespace = RouteNSName
	deploy.Spec.PodSpecPath = "spec.template.spec"
	deploy.Spec.Reference.Name = "deployments.apps"
	Expect(k8sClient.Create(context.Background(), deploy)).ToNot(HaveOccurred())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("Stop the routeTrait controller")
	close(controllerDone)
	By("Delete the route-test namespace")
	Expect(k8sClient.Delete(context.Background(), &routeNS,
		client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
	By("Tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
