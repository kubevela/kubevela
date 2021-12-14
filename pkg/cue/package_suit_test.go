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

package cue

import (
	"context"
	"errors"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"cuelang.org/go/cue/build"
	"github.com/google/go-cmp/cmp"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	certificatesv1beta1 "k8s.io/api/certificates/v1beta1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1beta1 "k8s.io/api/discovery/v1beta1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Package discovery resources for definition from K8s APIServer", func() {
	PIt("check that all built-in k8s resource are registered", func() {
		var localSchemeBuilder = runtime.SchemeBuilder{
			admissionregistrationv1.AddToScheme,
			appsv1.AddToScheme,
			batchv1.AddToScheme,
			certificatesv1beta1.AddToScheme,
			coordinationv1.AddToScheme,
			corev1.AddToScheme,
			discoveryv1beta1.AddToScheme,
			networkingv1.AddToScheme,
			policyv1beta1.AddToScheme,
			rbacv1.AddToScheme,
		}

		var localScheme = runtime.NewScheme()
		localSchemeBuilder.AddToScheme(localScheme)
		types := localScheme.AllKnownTypes()
		for typ := range types {
			if strings.HasSuffix(typ.Kind, "List") {
				continue
			}
			if strings.HasSuffix(typ.Kind, "Options") {
				continue
			}
			switch typ.Kind {
			case "WatchEvent":
				continue
			case "APIGroup", "APIVersions":
				continue
			case "RangeAllocation", "ComponentStatus", "Status":
				continue
			case "SerializedReference", "EndpointSlice":
				continue
			case "PodStatusResult", "EphemeralContainers":
				continue
			}

			Expect(pd.Exist(metav1.GroupVersionKind{
				Group:   typ.Group,
				Version: typ.Version,
				Kind:    typ.Kind,
			})).Should(BeTrue(), typ.String())
		}
	})

	PIt("discovery built-in k8s resource with kube prefix", func() {

		By("test ingress in kube package")
		bi := build.NewContext().NewInstance("", nil)
		err := bi.AddFile("-", `
import (
	kube	"kube/networking.k8s.io/v1beta1"
)
output: kube.#Ingress
output: {
	apiVersion: "networking.k8s.io/v1beta1"
	kind:       "Ingress"
	metadata: name: "myapp"
	spec: {
		rules: [{
			host: parameter.domain
			http: {
				paths: [
					for k, v in parameter.http {
						path: k
						backend: {
							serviceName: "myname"
							servicePort: v
						}
					},
				]
			}
		}]
	}
}
parameter: {
	domain: "abc.com"
	http: {
		"/": 80
	}
}`)
		Expect(err).ToNot(HaveOccurred())
		inst, err := pd.ImportPackagesAndBuildInstance(bi)
		Expect(err).Should(BeNil())
		base, err := model.NewBase(inst.Lookup("output"))
		Expect(err).Should(BeNil())
		data, err := base.Unstructured()
		Expect(err).Should(BeNil())

		Expect(cmp.Diff(data, &unstructured.Unstructured{Object: map[string]interface{}{
			"kind":       "Ingress",
			"apiVersion": "networking.k8s.io/v1beta1",
			"metadata":   map[string]interface{}{"name": "myapp"},
			"spec": map[string]interface{}{
				"rules": []interface{}{
					map[string]interface{}{
						"host": "abc.com",
						"http": map[string]interface{}{
							"paths": []interface{}{
								map[string]interface{}{
									"path": "/",
									"backend": map[string]interface{}{
										"serviceName": "myname",
										"servicePort": int64(80),
									}}}}}}}},
		})).Should(BeEquivalentTo(""))
		By("test Invalid Import path")
		bi = build.NewContext().NewInstance("", nil)
		bi.AddFile("-", `
import (
	kube	"kube/networking.k8s.io/v1"
)
output: kube.#Deployment
output: {
	metadata: {
		"name": parameter.name
	}
	spec: template: spec: {
		containers: [{
			name:"invalid-path",
			image: parameter.image
		}]
	}
}

parameter: {
	name:  "myapp"
	image: "nginx"
}`)
		inst, err = pd.ImportPackagesAndBuildInstance(bi)
		Expect(err).Should(BeNil())
		_, err = model.NewBase(inst.Lookup("output"))
		Expect(err).ShouldNot(BeNil())
		Expect(err.Error()).Should(Equal("_|_ // undefined field \"#Deployment\""))

		By("test Deployment in kube package")
		bi = build.NewContext().NewInstance("", nil)
		bi.AddFile("-", `
import (
	kube	"kube/apps/v1"
)
output: kube.#Deployment
output: {
	metadata: {
		"name": parameter.name
	}
	spec: template: spec: {
		containers: [{
			name:"test",
			image: parameter.image
		}]
	}
}
parameter: {
	name:  "myapp"
	image: "nginx"
}`)
		inst, err = pd.ImportPackagesAndBuildInstance(bi)
		Expect(err).Should(BeNil())
		base, err = model.NewBase(inst.Lookup("output"))
		Expect(err).Should(BeNil())
		data, err = base.Unstructured()
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(data, &unstructured.Unstructured{Object: map[string]interface{}{
			"kind":       "Deployment",
			"apiVersion": "apps/v1",
			"metadata":   map[string]interface{}{"name": "myapp"},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{},
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "test",
								"image": "nginx"}}}}}},
		})).Should(BeEquivalentTo(""))

		By("test Secret in kube package")
		bi = build.NewContext().NewInstance("", nil)
		bi.AddFile("-", `
import (
	kube "kube/v1"
)
output: kube.#Secret
output: {
	metadata: {
		"name": parameter.name
	}
	type:"kubevela"
}
parameter: {
	name:  "myapp"
}`)
		inst, err = pd.ImportPackagesAndBuildInstance(bi)
		Expect(err).Should(BeNil())
		base, err = model.NewBase(inst.Lookup("output"))
		Expect(err).Should(BeNil())
		data, err = base.Unstructured()
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(data, &unstructured.Unstructured{Object: map[string]interface{}{
			"kind":       "Secret",
			"apiVersion": "v1",
			"metadata":   map[string]interface{}{"name": "myapp"},
			"type":       "kubevela"}})).Should(BeEquivalentTo(""))

		By("test Service in kube package")
		bi = build.NewContext().NewInstance("", nil)
		bi.AddFile("-", `
import (
	kube "kube/v1"
)
output: kube.#Service
output: {
	metadata: {
		"name": parameter.name
	}
	spec: type: "ClusterIP",
}
parameter: {
	name:  "myapp"
}`)
		inst, err = pd.ImportPackagesAndBuildInstance(bi)
		Expect(err).Should(BeNil())
		base, err = model.NewBase(inst.Lookup("output"))
		Expect(err).Should(BeNil())
		data, err = base.Unstructured()
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(data, &unstructured.Unstructured{Object: map[string]interface{}{
			"kind":       "Service",
			"apiVersion": "v1",
			"metadata":   map[string]interface{}{"name": "myapp"},
			"spec": map[string]interface{}{
				"type": "ClusterIP"}},
		})).Should(BeEquivalentTo(""))
		Expect(pd.Exist(metav1.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Service",
		})).Should(Equal(true))

		By("Check newly added CRD refreshed and could be used in CUE package")
		crd1 := crdv1.CustomResourceDefinition{
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
									XPreserveUnknownFields: pointer.BoolPtr(true),
									Properties: map[string]crdv1.JSONSchemaProps{
										"key": {Type: "string"},
									}},
								"status": {
									Type:                   "object",
									XPreserveUnknownFields: pointer.BoolPtr(true),
									Properties: map[string]crdv1.JSONSchemaProps{
										"key":      {Type: "string"},
										"app-hash": {Type: "string"},
									}}}}}},
				},
				Scope: crdv1.NamespaceScoped,
			},
		}
		Expect(k8sClient.Create(context.Background(), &crd1)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		mapper, err := discoverymapper.New(cfg)
		Expect(err).ShouldNot(HaveOccurred())
		Eventually(func() error {
			_, err := mapper.RESTMapping(schema.GroupKind{Group: "example.com", Kind: "Foo"}, "v1")
			return err
		}, time.Second*2, time.Millisecond*300).Should(BeNil())

		Expect(pd.Exist(metav1.GroupVersionKind{
			Group:   "example.com",
			Version: "v1",
			Kind:    "Foo",
		})).Should(Equal(false))

		By("test new added CRD in kube package")
		Eventually(func() error {
			if err := pd.RefreshKubePackagesFromCluster(); err != nil {
				return err
			}
			if !pd.Exist(metav1.GroupVersionKind{
				Group:   "example.com",
				Version: "v1",
				Kind:    "Foo",
			}) {
				return errors.New("crd(example.com/v1.Foo) not register to openAPI")
			}
			return nil
		}, time.Second*30, time.Millisecond*300).Should(BeNil())

		bi = build.NewContext().NewInstance("", nil)
		err = bi.AddFile("-", `
import (
	kv1 "kube/example.com/v1"
)
output: kv1.#Foo
output: {
	spec: key: "test1"
    status: key: "test2"
}
`)
		Expect(err).Should(BeNil())
		inst, err = pd.ImportPackagesAndBuildInstance(bi)
		Expect(err).Should(BeNil())
		base, err = model.NewBase(inst.Lookup("output"))
		Expect(err).Should(BeNil())
		data, err = base.Unstructured()
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(data, &unstructured.Unstructured{Object: map[string]interface{}{
			"kind":       "Foo",
			"apiVersion": "example.com/v1",
			"spec": map[string]interface{}{
				"key": "test1"},
			"status": map[string]interface{}{
				"key": "test2"}},
		})).Should(BeEquivalentTo(""))

	})

	PIt("discovery built-in k8s resource with third-party path", func() {

		By("test ingress in kube package")
		bi := build.NewContext().NewInstance("", nil)
		err := bi.AddFile("-", `
import (
	network "k8s.io/networking/v1beta1"
)
output: network.#Ingress
output: {
	metadata: name: "myapp"
	spec: {
		rules: [{
			host: parameter.domain
			http: {
				paths: [
					for k, v in parameter.http {
						path: k
						backend: {
							serviceName: "myname"
							servicePort: v
						}
					},
				]
			}
		}]
	}
}
parameter: {
	domain: "abc.com"
	http: {
		"/": 80
	}
}`)
		Expect(err).ToNot(HaveOccurred())
		inst, err := pd.ImportPackagesAndBuildInstance(bi)
		Expect(err).Should(BeNil())
		base, err := model.NewBase(inst.Lookup("output"))
		Expect(err).Should(BeNil())
		data, err := base.Unstructured()
		Expect(err).Should(BeNil())

		Expect(cmp.Diff(data, &unstructured.Unstructured{Object: map[string]interface{}{
			"kind":       "Ingress",
			"apiVersion": "networking.k8s.io/v1beta1",
			"metadata":   map[string]interface{}{"name": "myapp"},
			"spec": map[string]interface{}{
				"rules": []interface{}{
					map[string]interface{}{
						"host": "abc.com",
						"http": map[string]interface{}{
							"paths": []interface{}{
								map[string]interface{}{
									"path": "/",
									"backend": map[string]interface{}{
										"serviceName": "myname",
										"servicePort": int64(80),
									}}}}}}}},
		})).Should(BeEquivalentTo(""))
		By("test Invalid Import path")
		bi = build.NewContext().NewInstance("", nil)
		bi.AddFile("-", `
import (
	"k8s.io/networking/v1"
)
output: v1.#Deployment
output: {
	metadata: {
		"name": parameter.name
	}
	spec: template: spec: {
		containers: [{
			name:"invalid-path",
			image: parameter.image
		}]
	}
}

parameter: {
	name:  "myapp"
	image: "nginx"
}`)
		inst, err = pd.ImportPackagesAndBuildInstance(bi)
		Expect(err).Should(BeNil())
		_, err = model.NewBase(inst.Lookup("output"))
		Expect(err).ShouldNot(BeNil())
		Expect(err.Error()).Should(Equal("_|_ // undefined field \"#Deployment\""))

		By("test Deployment in kube package")
		bi = build.NewContext().NewInstance("", nil)
		bi.AddFile("-", `
import (
	apps "k8s.io/apps/v1"
)
output: apps.#Deployment
output: {
	metadata: {
		"name": parameter.name
	}
	spec: template: spec: {
		containers: [{
			name:"test",
			image: parameter.image
		}]
	}
}
parameter: {
	name:  "myapp"
	image: "nginx"
}`)
		inst, err = pd.ImportPackagesAndBuildInstance(bi)
		Expect(err).Should(BeNil())
		base, err = model.NewBase(inst.Lookup("output"))
		Expect(err).Should(BeNil())
		data, err = base.Unstructured()
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(data, &unstructured.Unstructured{Object: map[string]interface{}{
			"kind":       "Deployment",
			"apiVersion": "apps/v1",
			"metadata":   map[string]interface{}{"name": "myapp"},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{},
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "test",
								"image": "nginx"}}}}}},
		})).Should(BeEquivalentTo(""))

		By("test Secret in kube package")
		bi = build.NewContext().NewInstance("", nil)
		bi.AddFile("-", `
import (
	"k8s.io/core/v1"
)
output: v1.#Secret
output: {
	metadata: {
		"name": parameter.name
	}
	type:"kubevela"
}
parameter: {
	name:  "myapp"
}`)
		inst, err = pd.ImportPackagesAndBuildInstance(bi)
		Expect(err).Should(BeNil())
		base, err = model.NewBase(inst.Lookup("output"))
		Expect(err).Should(BeNil())
		data, err = base.Unstructured()
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(data, &unstructured.Unstructured{Object: map[string]interface{}{
			"kind":       "Secret",
			"apiVersion": "v1",
			"metadata":   map[string]interface{}{"name": "myapp"},
			"type":       "kubevela"}})).Should(BeEquivalentTo(""))

		By("test Service in kube package")
		bi = build.NewContext().NewInstance("", nil)
		bi.AddFile("-", `
import (
	"k8s.io/core/v1"
)
output: v1.#Service
output: {
	metadata: {
		"name": parameter.name
	}
	spec: type: "ClusterIP",
}
parameter: {
	name:  "myapp"
}`)
		inst, err = pd.ImportPackagesAndBuildInstance(bi)
		Expect(err).Should(BeNil())
		base, err = model.NewBase(inst.Lookup("output"))
		Expect(err).Should(BeNil())
		data, err = base.Unstructured()
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(data, &unstructured.Unstructured{Object: map[string]interface{}{
			"kind":       "Service",
			"apiVersion": "v1",
			"metadata":   map[string]interface{}{"name": "myapp"},
			"spec": map[string]interface{}{
				"type": "ClusterIP"}},
		})).Should(BeEquivalentTo(""))
		Expect(pd.Exist(metav1.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Service",
		})).Should(Equal(true))

		By("Check newly added CRD refreshed and could be used in CUE package")
		crd1 := crdv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: "bar.example.com",
			},
			Spec: crdv1.CustomResourceDefinitionSpec{
				Group: "example.com",
				Names: crdv1.CustomResourceDefinitionNames{
					Kind:     "Bar",
					ListKind: "BarList",
					Plural:   "bar",
					Singular: "bar",
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
									XPreserveUnknownFields: pointer.BoolPtr(true),
									Properties: map[string]crdv1.JSONSchemaProps{
										"key": {Type: "string"},
									}},
								"status": {
									Type:                   "object",
									XPreserveUnknownFields: pointer.BoolPtr(true),
									Properties: map[string]crdv1.JSONSchemaProps{
										"key":      {Type: "string"},
										"app-hash": {Type: "string"},
									}}}}}},
				},
				Scope: crdv1.NamespaceScoped,
			},
		}
		Expect(k8sClient.Create(context.Background(), &crd1)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		mapper, err := discoverymapper.New(cfg)
		Expect(err).ShouldNot(HaveOccurred())
		Eventually(func() error {
			_, err := mapper.RESTMapping(schema.GroupKind{Group: "example.com", Kind: "Bar"}, "v1")
			return err
		}, time.Second*2, time.Millisecond*300).Should(BeNil())

		Expect(pd.Exist(metav1.GroupVersionKind{
			Group:   "example.com",
			Version: "v1",
			Kind:    "Bar",
		})).Should(Equal(false))

		By("test new added CRD in kube package")
		Eventually(func() error {
			if err := pd.RefreshKubePackagesFromCluster(); err != nil {
				return err
			}
			if !pd.Exist(metav1.GroupVersionKind{
				Group:   "example.com",
				Version: "v1",
				Kind:    "Bar",
			}) {
				return errors.New("crd(example.com/v1.Bar) not register to openAPI")
			}
			return nil
		}, time.Second*30, time.Millisecond*300).Should(BeNil())

		bi = build.NewContext().NewInstance("", nil)
		err = bi.AddFile("-", `
import (
	ev1 "example.com/v1"
)
output: ev1.#Bar
output: {
	spec: key: "test1"
    status: key: "test2"
}
`)
		Expect(err).Should(BeNil())
		inst, err = pd.ImportPackagesAndBuildInstance(bi)
		Expect(err).Should(BeNil())
		base, err = model.NewBase(inst.Lookup("output"))
		Expect(err).Should(BeNil())
		data, err = base.Unstructured()
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(data, &unstructured.Unstructured{Object: map[string]interface{}{
			"kind":       "Bar",
			"apiVersion": "example.com/v1",
			"spec": map[string]interface{}{
				"key": "test1"},
			"status": map[string]interface{}{
				"key": "test2"}},
		})).Should(BeEquivalentTo(""))
	})
})
