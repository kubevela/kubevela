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

package helm

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = Describe("manifest", func() {

	Describe("orderResources", func() {
		It("should order CRDs first, then Namespaces, then others", func() {
			crd := map[string]interface{}{
				"apiVersion": "apiextensions.k8s.io/v1",
				"kind":       "CustomResourceDefinition",
				"metadata":   map[string]interface{}{"name": "test-crd"},
			}
			namespace := map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata":   map[string]interface{}{"name": "test-namespace"},
			}
			deployment := map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata":   map[string]interface{}{"name": "test-deployment"},
			}
			service := map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata":   map[string]interface{}{"name": "test-service"},
			}

			input := []map[string]interface{}{deployment, service, crd, namespace}
			result := orderResources(input)

			Expect(result).To(HaveLen(4))
			Expect(result[0]["kind"]).To(Equal("CustomResourceDefinition"))
			Expect(result[1]["kind"]).To(Equal("Namespace"))
			Expect(result[2]["kind"]).To(Equal("Deployment"))
			Expect(result[3]["kind"]).To(Equal("Service"))
		})
	})

	Describe("isTestResource", func() {
		DescribeTable("should identify test hook resources",
			func(annotations map[string]interface{}, expected bool) {
				resource := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Pod",
						"metadata": map[string]interface{}{
							"name":        "test-pod",
							"annotations": annotations,
						},
					},
				}
				Expect(isTestResource(resource)).To(Equal(expected))
			},
			Entry("test-success hook", map[string]interface{}{"helm.sh/hook": "test-success"}, true),
			Entry("pre-install hook", map[string]interface{}{"helm.sh/hook": "pre-install"}, false),
		)

		It("should return false for resources without annotations", func() {
			resource := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Service",
					"metadata":   map[string]interface{}{"name": "my-service"},
				},
			}
			Expect(isTestResource(resource)).To(BeFalse())
		})
	})

	Describe("parseManifestResources", func() {
		var p *Provider

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
		})

		It("should skip test hooks by default", func() {
			manifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deploy
---
apiVersion: v1
kind: Service
metadata:
  name: test-svc
---
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  annotations:
    helm.sh/hook: test-success
`
			resources, err := p.parseManifestResources(manifest, nil, "")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(resources).To(HaveLen(2))

			kinds := make([]string, len(resources))
			for i, r := range resources {
				kinds[i], _, _ = unstructured.NestedString(r, "kind")
			}
			Expect(kinds).To(ContainElement("Deployment"))
			Expect(kinds).To(ContainElement("Service"))
			Expect(kinds).ToNot(ContainElement("Pod"))
		})

		It("should include test hooks when skipTests=false", func() {
			manifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deploy
---
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  annotations:
    helm.sh/hook: test-success
`
			skipFalse := false
			resources, err := p.parseManifestResources(manifest, &RenderOptionsParams{SkipTests: &skipFalse}, "")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(resources).To(HaveLen(2))
		})

		It("should order CRDs before Namespaces before others", func() {
			manifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-deploy
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: my-crd
---
apiVersion: v1
kind: Namespace
metadata:
  name: my-ns
`
			resources, err := p.parseManifestResources(manifest, nil, "")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(resources).To(HaveLen(3))

			kind0, _, _ := unstructured.NestedString(resources[0], "kind")
			kind1, _, _ := unstructured.NestedString(resources[1], "kind")
			kind2, _, _ := unstructured.NestedString(resources[2], "kind")
			Expect(kind0).To(Equal("CustomResourceDefinition"))
			Expect(kind1).To(Equal("Namespace"))
			Expect(kind2).To(Equal("Deployment"))
		})

		It("defaults metadata.namespace to releaseNamespace for namespaced resources", func() {
			p := NewProvider()
			manifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
spec: {replicas: 1}
---
apiVersion: v1
kind: Service
metadata:
  name: app
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.example.com
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: app-reader
`
			resources, err := p.parseManifestResources(manifest, nil, "stress-s2")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(resources).To(HaveLen(4))

			byKind := map[string]map[string]interface{}{}
			for _, r := range resources {
				k, _, _ := unstructured.NestedString(r, "kind")
				byKind[k] = r
			}

			depNs, _, _ := unstructured.NestedString(byKind["Deployment"], "metadata", "namespace")
			Expect(depNs).To(Equal("stress-s2"))
			svcNs, _, _ := unstructured.NestedString(byKind["Service"], "metadata", "namespace")
			Expect(svcNs).To(Equal("stress-s2"))

			// Cluster-scoped kinds must NOT be patched with a namespace.
			_, crdHasNs, _ := unstructured.NestedString(byKind["CustomResourceDefinition"], "metadata", "namespace")
			Expect(crdHasNs).To(BeFalse())
			_, crHasNs, _ := unstructured.NestedString(byKind["ClusterRole"], "metadata", "namespace")
			Expect(crHasNs).To(BeFalse())
		})

		It("leaves explicit metadata.namespace untouched", func() {
			p := NewProvider()
			manifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
  namespace: explicit-ns
spec: {replicas: 1}
`
			resources, err := p.parseManifestResources(manifest, nil, "release-ns")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(resources).To(HaveLen(1))
			ns, _, _ := unstructured.NestedString(resources[0], "metadata", "namespace")
			Expect(ns).To(Equal("explicit-ns"))
		})
	})

	Describe("cleanResource", func() {
		It("should remove nil values", func() {
			input := map[string]interface{}{
				"key1": "value1",
				"key2": nil,
				"key3": "value3",
			}
			result := cleanResource(input)
			Expect(result["key1"]).To(Equal("value1"))
			Expect(result["key3"]).To(Equal("value3"))
			Expect(result).ToNot(HaveKey("key2"))
		})

		It("should recursively clean nested maps", func() {
			input := map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":      "test",
					"namespace": nil,
				},
			}
			result := cleanResource(input)
			metadata := result["metadata"].(map[string]interface{})
			Expect(metadata["name"]).To(Equal("test"))
			Expect(metadata).ToNot(HaveKey("namespace"))
		})

		It("should clean arrays with nil items", func() {
			input := map[string]interface{}{
				"items": []interface{}{"a", nil, "c"},
			}
			result := cleanResource(input)
			Expect(result["items"]).To(Equal([]interface{}{"a", "c"}))
		})

		It("should clean nested maps inside arrays", func() {
			input := map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "app",
						"image": nil,
					},
				},
			}
			result := cleanResource(input)
			containers := result["containers"].([]interface{})
			container := containers[0].(map[string]interface{})
			Expect(container["name"]).To(Equal("app"))
			Expect(container).ToNot(HaveKey("image"))
		})

		It("should remove empty nested maps", func() {
			input := map[string]interface{}{
				"status": map[string]interface{}{},
				"spec":   map[string]interface{}{"replicas": 1},
			}
			result := cleanResource(input)
			Expect(result).ToNot(HaveKey("status"))
			Expect(result["spec"].(map[string]interface{})["replicas"]).To(Equal(1))
		})

		It("should remove empty arrays", func() {
			input := map[string]interface{}{
				"items":  []interface{}{nil},
				"labels": map[string]interface{}{"app": "test"},
			}
			result := cleanResource(input)
			// After removing nil, the array is empty and should be dropped
			Expect(result).ToNot(HaveKey("items"))
			Expect(result["labels"].(map[string]interface{})["app"]).To(Equal("test"))
		})
	})

})
