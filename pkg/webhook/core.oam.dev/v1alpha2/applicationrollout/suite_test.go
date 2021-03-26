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

package applicationrollout

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	core "github.com/oam-dev/kubevela/apis/core.oam.dev"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
)

var scheme = runtime.NewScheme()
var crd crdv1.CustomResourceDefinition
var reqResource metav1.GroupVersionResource
var decoder *admission.Decoder

func TestApplicationConfigurationWebHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ApplicationConfiguration Web handler")
}

var _ = BeforeSuite(func(done Done) {
	By("Bootstrapping test environment")
	ctrl.SetLogger(zap.New(func(o *zap.Options) {
		o.Development = true
		o.DestWritter = os.Stdout
	}))
	By("Setup scheme")
	err := core.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = clientgoscheme.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	// the crd we will refer to
	crd = crdv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo.example.com",
		},
		Spec: crdv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: crdv1.CustomResourceDefinitionNames{
				Kind:     "Foo",
				ListKind: "FooList",
				Plural:   "foo",
				Singular: "foo",
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
								"status": {
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
	By("Prepare for the admission resource")
	reqResource = metav1.GroupVersionResource{Group: v1alpha2.Group, Version: v1alpha2.Version,
		Resource: "applicationconfigurations"}
	By("Prepare for the admission decoder")
	decoder, err = admission.NewDecoder(scheme)
	Expect(err).Should(BeNil())
	By("Finished test bootstrap")
	close(done)
})
