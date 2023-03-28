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

package applicationconfiguration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	controllerscheme "sigs.k8s.io/controller-runtime/pkg/scheme"

	core "github.com/oam-dev/kubevela/apis/core.oam.dev"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	// +kubebuilder:scaffold:imports
)

var reconciler *OAMApplicationReconciler
var componentHandler *ComponentHandler
var controllerDone context.CancelFunc
var testEnv *envtest.Environment
var cfg *rest.Config
var k8sClient client.Client
var scheme = runtime.NewScheme()
var crd crdv1.CustomResourceDefinition

// OAM runtime is deprecated and we won't run test here.
func TestReconcilerSuit(t *testing.T) {
	t.SkipNow()

	RegisterFailHandler(Fail)

	RunSpecs(t, "OAM Core Resource Controller Unit test Suite")
}

var _ = BeforeSuite(func(done Done) {
	By("Bootstrapping test environment")
	var yamlPath string
	if _, set := os.LookupEnv("COMPATIBILITY_TEST"); set {
		yamlPath = "../../../../../test/compatibility-test/testdata"
	} else {
		yamlPath = filepath.Join("../../../../..", "charts", "vela-core", "crds")
	}
	compCRD := "../../../../../charts/oam-runtime/crds/core.oam.dev_components.yaml"
	acCRD := "../../../../../charts/oam-runtime/crds/core.oam.dev_applicationconfigurations.yaml"
	logf.Log.Info("start applicationconfiguration suit test", "yaml_path", yamlPath)
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths: []string{
			yamlPath, // this has all the required CRDs,
			compCRD,
			acCRD,
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
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		logf.Log.Error(err, "failed to create a client")
		Fail("setup failed")
	}
	Expect(k8sClient).ShouldNot(BeNil())
	By("Finished setting up test environment")

	By("Creating Reconciler for appconfig")
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme, MetricsBindAddress: "0"})
	Expect(err).Should(BeNil())
	var ctx context.Context
	ctx, controllerDone = context.WithCancel(context.Background())
	go mgr.Start(ctx)

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
				Name:         "v1",
				Served:       true,
				Storage:      true,
				Subresources: &crdv1.CustomResourceSubresources{Status: &crdv1.CustomResourceSubresourceStatus{}},
				Schema: &crdv1.CustomResourceValidation{
					OpenAPIV3Schema: &crdv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]crdv1.JSONSchemaProps{
							"spec": {
								Type:                   "object",
								XPreserveUnknownFields: pointer.Bool(true),
								Properties: map[string]crdv1.JSONSchemaProps{
									"key": {Type: "string"},
								}},
							"status": {
								Type:                   "object",
								XPreserveUnknownFields: pointer.Bool(true),
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

	reconciler = NewReconciler(mgr, dm)
	componentHandler = &ComponentHandler{Client: k8sClient, RevisionLimit: 100}

	By("Creating workload definition and trait definition")
	wd := v1alpha2.WorkloadDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo.example.com",
			Namespace: "vela-system",
		},
		Spec: v1alpha2.WorkloadDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: "foo.example.com",
			},
		},
	}
	td := v1alpha2.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo.example.com",
			Namespace: "vela-system",
		},
		Spec: v1alpha2.TraitDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: "foo.example.com",
			},
		},
	}

	rollout := v1alpha2.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rollout-revision",
			Namespace: "vela-system",
		},
		Spec: v1alpha2.TraitDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: "foo.example.com",
			},
			RevisionEnabled: true,
		},
	}
	definitionNs := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vela-system"}}
	Expect(k8sClient.Create(context.Background(), definitionNs.DeepCopy())).Should(BeNil())

	// For some reason, WorkloadDefinition is created as a Cluster scope object
	Expect(k8sClient.Create(ctx, &wd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	// For some reason, TraitDefinition is created as a Cluster scope object
	Expect(k8sClient.Create(ctx, &td)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	// rollout trait is used for revisionEnable case test
	Expect(k8sClient.Create(ctx, &rollout)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

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
	controllerDone()
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
