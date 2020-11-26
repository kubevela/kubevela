package applicationconfiguration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	controllerscheme "sigs.k8s.io/controller-runtime/pkg/scheme"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/discoverymapper"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
	// +kubebuilder:scaffold:imports
)

var reconciler *OAMApplicationReconciler
var mgrclose chan struct{}
var testEnv *envtest.Environment
var cfg *rest.Config
var k8sClient resource.ClientApplicator
var scheme = runtime.NewScheme()
var crd crdv1.CustomResourceDefinition

func TestReconcilder(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"OAM Core Resource Controller Unit test Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	ctx := context.Background()
	By("Bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("../../../..", "charts/oam-kubernetes-runtime/crds"), // this has all the required CRDs,
		},
	}
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	Expect(clientgoscheme.AddToScheme(scheme)).Should(BeNil())
	Expect(core.AddToScheme(scheme)).Should(BeNil())
	Expect(crdv1.AddToScheme(scheme)).Should(BeNil())
	depExample := &unstructured.Unstructured{}
	depExample.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "example.com",
		Version: "v1",
		Kind:    "Foo",
	})
	depSchemeBuilder := &controllerscheme.Builder{GroupVersion: schema.GroupVersion{Group: "example.com", Version: "v1"}}
	depSchemeBuilder.Register(depExample.DeepCopyObject())
	Expect(depSchemeBuilder.AddToScheme(scheme)).Should(BeNil())

	By("Setting up kubernetes client")
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		logf.Log.Error(err, "failed to create a client")
		Fail("setup failed")
	}
	k8sClient = resource.ClientApplicator{
		Client:     c,
		Applicator: resource.NewAPIUpdatingApplicator(c),
	}
	if err != nil {
		logf.Log.Error(err, "failed to create k8sClient")
		Fail("setup failed")
	}
	By("Finished setting up test environment")

	By("Creating Reconciler for appconfig")
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme, MetricsBindAddress: "0"})
	Expect(err).Should(BeNil())
	mgrclose = make(chan struct{})
	go mgr.Start(mgrclose)

	// Create a crd for appconfig dependency test
	crd = crdv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "foo.example.com",
			Labels: map[string]string{"crd": "dependency"},
		},
		Spec: crdv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: crdv1.CustomResourceDefinitionNames{
				Kind:     "Foo",
				ListKind: "FooList",
				Plural:   "foo",
				Singular: "foo",
			},
			Versions: []crdv1.CustomResourceDefinitionVersion{{
				Name:    "v1",
				Served:  true,
				Storage: true,
				Schema: &crdv1.CustomResourceValidation{
					OpenAPIV3Schema: &crdv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]crdv1.JSONSchemaProps{
							"spec": {
								Type: "object",
								Properties: map[string]crdv1.JSONSchemaProps{
									"key": {Type: "string"},
								}},
							"status": {
								Type: "object",
								Properties: map[string]crdv1.JSONSchemaProps{
									"key":      {Type: "string"},
									"app-hash": {Type: "string"},
								}}}}}},
			},
			Scope: crdv1.NamespaceScoped,
		},
	}
	Expect(k8sClient.Create(context.Background(), &crd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	By("Created a crd for appconfig dependency test")

	dm, err := discoverymapper.New(cfg)
	Expect(err).Should(BeNil())

	var mapping *meta.RESTMapping
	Eventually(func() error {
		mapping, err = dm.RESTMapping(schema.GroupKind{
			Group: "example.com",
			Kind:  "Foo",
		}, "v1")
		return err
	}, time.Second*30, time.Millisecond*500).Should(BeNil())
	Expect(mapping.Resource.Resource).Should(Equal("foo"))

	reconciler = NewReconciler(mgr, dm, WithLogger(logging.NewLogrLogger(ctrl.Log.WithName("suit-test-appconfig"))))

	By("Creating workload definition and trait definition")
	wd := v1alpha2.WorkloadDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo.example.com",
		},
		Spec: v1alpha2.WorkloadDefinitionSpec{
			Reference: v1alpha2.DefinitionReference{
				Name: "foo.example.com",
			},
		},
	}
	td := v1alpha2.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo.example.com",
		},
		Spec: v1alpha2.TraitDefinitionSpec{
			Reference: v1alpha2.DefinitionReference{
				Name: "foo.example.com",
			},
		},
	}
	// For some reason, WorkloadDefinition is created as a Cluster scope object
	Expect(k8sClient.Create(ctx, &wd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	// For some reason, TraitDefinition is created as a Cluster scope object
	Expect(k8sClient.Create(ctx, &td)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	close(done)
}, 300)

var _ = AfterSuite(func() {

	crd = crdv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "foo.example.com",
			Labels: map[string]string{"crd": "dependency"},
		},
	}
	Expect(k8sClient.Delete(context.Background(), &crd)).Should(BeNil())
	By("Deleted the custom resource definition")

	By("Tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
	close(mgrclose)
})

func reconcileRetry(r reconcile.Reconciler, req reconcile.Request) {
	Eventually(func() error {
		_, err := r.Reconcile(req)
		return err
	}, 3*time.Second, time.Second).Should(BeNil())
}
