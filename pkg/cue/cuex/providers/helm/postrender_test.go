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
	"bytes"
	"fmt"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
)

var _ = Describe("postrender", func() {

	Describe("velaLabelPostRenderer", func() {
		It("should inject KubeVela labels and annotations on all resources", func() {
			manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deploy
  namespace: test-ns
spec:
  replicas: 1
---
apiVersion: v1
kind: Service
metadata:
  name: test-svc
  namespace: test-ns
`
			velaCtx := &ContextParams{
				AppName:      "my-app",
				AppNamespace: "my-app-ns",
				Name:         "my-component",
				Namespace:    "test-ns",
			}
			renderer := &velaLabelPostRenderer{
				context:          velaCtx,
				releaseName:      "my-release",
				releaseNamespace: "test-ns",
			}
			result, err := renderer.Run(bytes.NewBufferString(manifest))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			decoder := kyaml.NewYAMLOrJSONDecoder(bytes.NewReader(result.Bytes()), 4096)
			var resourceCount int
			for {
				obj := &unstructured.Unstructured{}
				if err := decoder.Decode(obj); err != nil {
					if err == io.EOF {
						break
					}
					Fail(fmt.Sprintf("failed to decode: %v", err))
				}
				if len(obj.Object) == 0 {
					continue
				}
				resourceCount++

				labels := obj.GetLabels()
				Expect(labels["app.oam.dev/name"]).To(Equal("my-app"))
				Expect(labels["app.oam.dev/namespace"]).To(Equal("my-app-ns"))
				Expect(labels["app.oam.dev/component"]).To(Equal("my-component"))

				annotations := obj.GetAnnotations()
				Expect(annotations["app.oam.dev/owner"]).To(Equal("helm-provider"))
				Expect(annotations["meta.helm.sh/release-name"]).To(Equal("my-release"))
				Expect(annotations["meta.helm.sh/release-namespace"]).To(Equal("test-ns"))
			}
			Expect(resourceCount).To(Equal(2))
		})

		It("should return original buffer when context is nil", func() {
			manifest := `apiVersion: v1
kind: Service
metadata:
  name: test-svc
`
			renderer := &velaLabelPostRenderer{context: nil}
			buf := bytes.NewBufferString(manifest)
			result, err := renderer.Run(buf)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).To(Equal(buf))
		})
	})

	Describe("getActionConfig", func() {
		It("should not panic without a real cluster", func() {
			p := NewProviderWithConfig(nil)
			// Without a real cluster, Init will fail — we just verify no panic
			_, err := p.getActionConfig("test-namespace")
			_ = err // error is expected
		})
	})

	Describe("velaOwnerLabels", func() {
		It("should return labels for a valid context", func() {
			velaCtx := &ContextParams{
				AppName:      "my-app",
				AppNamespace: "my-ns",
				Name:         "my-component",
			}
			labels := velaOwnerLabels(velaCtx)
			Expect(labels["app.oam.dev/name"]).To(Equal("my-app"))
			Expect(labels["app.oam.dev/namespace"]).To(Equal("my-ns"))
			Expect(labels["app.oam.dev/component"]).To(Equal("my-component"))
		})

		It("should return nil for nil context", func() {
			Expect(velaOwnerLabels(nil)).To(BeNil())
		})
	})

	Describe("isOwnedByVela", func() {
		It("should return false for nil release", func() {
			Expect(isOwnedByVela(nil, &ContextParams{AppName: "app"})).To(BeFalse())
		})

		It("should return false for nil context", func() {
			rel := &release.Release{Labels: map[string]string{"app.oam.dev/name": "app"}}
			Expect(isOwnedByVela(rel, nil)).To(BeFalse())
		})

		It("should return false for nil labels", func() {
			rel := &release.Release{}
			Expect(isOwnedByVela(rel, &ContextParams{AppName: "app"})).To(BeFalse())
		})

		It("should return false when vela label is missing", func() {
			rel := &release.Release{Labels: map[string]string{"other": "val"}}
			Expect(isOwnedByVela(rel, &ContextParams{AppName: "app"})).To(BeFalse())
		})

		It("should return true when vela label is present", func() {
			rel := &release.Release{Labels: map[string]string{"app.oam.dev/name": "my-app"}}
			Expect(isOwnedByVela(rel, &ContextParams{AppName: "my-app"})).To(BeTrue())
		})
	})

})
