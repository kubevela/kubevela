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

package controllers_test

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	controllerscheme "sigs.k8s.io/controller-runtime/pkg/scheme"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
	// +kubebuilder:scaffold:imports
)

var k8sClient client.Client
var scheme = runtime.NewScheme()
var manualscalertrait v1alpha2.TraitDefinition
var extendedmanualscalertrait v1alpha2.TraitDefinition
var roleName = "oam-example-com"
var roleBindingName = "oam-role-binding"
var crd crdv1.CustomResourceDefinition

// A DefinitionExtension is an Object type for xxxDefinitin.spec.extension
type DefinitionExtension struct {
	Alias string `json:"alias,omitempty"`
}

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"OAM Core Resource Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	By("Bootstrapping test environment")
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	err := clientgoscheme.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = core.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = crdv1.AddToScheme(scheme)
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

	By("Applying CRD of WorkloadDefinition and TraitDefinition")
	var workloadDefinitionCRD crdv1.CustomResourceDefinition
	Expect(readYaml("../../charts/oam-kubernetes-runtime/crds/core.oam.dev_workloaddefinitions.yaml", &workloadDefinitionCRD)).Should(BeNil())
	Expect(k8sClient.Create(context.Background(), &workloadDefinitionCRD)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	var traitDefinitionCRD crdv1.CustomResourceDefinition
	Expect(readYaml("../../charts/oam-kubernetes-runtime/crds/core.oam.dev_traitdefinitions.yaml", &traitDefinitionCRD)).Should(BeNil())
	Expect(k8sClient.Create(context.Background(), &traitDefinitionCRD)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	// Create manual scaler trait definition
	manualscalertrait = v1alpha2.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "manualscalertraits.core.oam.dev",
			Labels: map[string]string{"trait": "manualscalertrait"},
		},
		Spec: v1alpha2.TraitDefinitionSpec{
			WorkloadRefPath: "spec.workloadRef",
			Reference: v1alpha2.DefinitionReference{
				Name: "manualscalertraits.core.oam.dev",
			},
		},
	}
	// For some reason, traitDefinition is created as a Cluster scope object
	Expect(k8sClient.Create(context.Background(), &manualscalertrait)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	By("Created manual scalar trait definition")

	// Create manual scaler trait definition with spec.extension field
	definitionExtension := DefinitionExtension{
		Alias: "ManualScaler",
	}
	in := new(runtime.RawExtension)
	in.Raw, _ = json.Marshal(definitionExtension)

	extendedmanualscalertrait = v1alpha2.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "manualscalertraits-extended.core.oam.dev",
			Labels: map[string]string{"trait": "manualscalertrait"},
		},
		Spec: v1alpha2.TraitDefinitionSpec{
			WorkloadRefPath: "spec.workloadRef",
			Reference: v1alpha2.DefinitionReference{
				Name: "manualscalertraits-extended.core.oam.dev",
			},
			Extension: in,
		},
	}
	Expect(k8sClient.Create(context.Background(), &extendedmanualscalertrait)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	By("Created extended manualscalertraits.core.oam.dev")

	// For some reason, workloadDefinition is created as a Cluster scope object
	label := map[string]string{"workload": "containerized-workload"}
	// create a workload definition
	wd := v1alpha2.WorkloadDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "containerizedworkloads.core.oam.dev",
			Labels: label,
		},
		Spec: v1alpha2.WorkloadDefinitionSpec{
			Reference: v1alpha2.DefinitionReference{
				Name: "containerizedworkloads.core.oam.dev",
			},
			ChildResourceKinds: []v1alpha2.ChildResourceKind{
				{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       util.KindService,
				},
				{
					APIVersion: appsv1.SchemeGroupVersion.String(),
					Kind:       util.KindDeployment,
				},
			},
		},
	}
	Expect(k8sClient.Create(context.Background(), &wd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	By("Created containerizedworkload.core.oam.dev")

	exampleClusterRole := rbac.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: roleName,
			Labels: map[string]string{
				"oam":                                  "clusterrole",
				"rbac.oam.dev/aggregate-to-controller": "true",
			},
		},
		Rules: []rbac.PolicyRule{{
			APIGroups: []string{"example.com"},
			Resources: []string{rbac.ResourceAll},
			Verbs:     []string{rbac.VerbAll},
		}},
	}
	Expect(k8sClient.Create(context.Background(), &exampleClusterRole)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	By("Created example.com cluster role for the test service account")

	adminRoleBinding := rbac.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   roleBindingName,
			Labels: map[string]string{"oam": "clusterrole"},
		},
		Subjects: []rbac.Subject{
			{
				Kind: "User",
				Name: "system:serviceaccount:oam-system:oam-kubernetes-runtime-e2e",
			},
		},
		RoleRef: rbac.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
	}
	Expect(k8sClient.Create(context.Background(), &adminRoleBinding)).Should(BeNil())
	By("Created cluster role binding for the test service account")

	crd = crdv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "bars.example.com",
			Labels: map[string]string{"crd": "revision-test"},
		},
		Spec: crdv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: crdv1.CustomResourceDefinitionNames{
				Kind:     "Bar",
				ListKind: "BarList",
				Plural:   "bars",
				Singular: "bar",
			},
			Versions: []crdv1.CustomResourceDefinitionVersion{
				{
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
									},
								},
							},
						},
					},
				},
			},
			Scope: crdv1.NamespaceScoped,
		},
	}
	Expect(k8sClient.Create(context.Background(), &crd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	By("Created a crd for revision mechanism test")

	By("Create workload definition for revision mechanism test")
	var nwd v1alpha2.WorkloadDefinition
	Expect(readYaml("testdata/revision/workload-def.yaml", &nwd)).Should(BeNil())
	Expect(k8sClient.Create(context.Background(), &nwd)).Should(Succeed())

	close(done)
}, 300)

var _ = AfterSuite(func() {
	By("Tearing down the test environment")
	adminRoleBinding := rbac.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   roleBindingName,
			Labels: map[string]string{"oam": "clusterrole"},
		},
	}
	Expect(k8sClient.Delete(context.Background(), &adminRoleBinding)).Should(BeNil())
	By("Deleted the cluster role binding")

	crd = crdv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "bars.example.com",
			Labels: map[string]string{"crd": "revision-test"},
		},
	}
	Expect(k8sClient.Delete(context.Background(), &crd)).Should(BeNil())
	By("Deleted the custom resource definition")

	crd = crdv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "workloaddefinitions.core.oam.dev",
		},
	}
	Expect(k8sClient.Delete(context.Background(), &crd)).Should(BeNil())
	By("Deleted the workloaddefinitions CRD")

	crd = crdv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "traitdefinitions.core.oam.dev",
		},
	}
	Expect(k8sClient.Delete(context.Background(), &crd)).Should(BeNil())
	By("Deleted the workloaddefinitions CRD")
})
