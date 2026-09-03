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

	Describe("kustomizePostRenderer", func() {
		It("should apply a patches entry to matching resources", func() {
			manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: podinfo
  namespace: default
spec:
  replicas: 1
`
			params := &KustomizeParams{
				Patches: []interface{}{
					map[string]interface{}{
						"patch": "- op: replace\n  path: /spec/replicas\n  value: 3\n",
						"target": map[string]interface{}{
							"kind": "Deployment",
							"name": "podinfo",
						},
					},
				},
			}
			renderer := &kustomizePostRenderer{params: params}
			result, err := renderer.Run(bytes.NewBufferString(manifest))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.String()).To(ContainSubstring("replicas: 3"))
		})

		It("should apply a patchesStrategicMerge map to matching resources", func() {
			manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: podinfo
  namespace: default
spec:
  replicas: 1
`
			params := &KustomizeParams{
				PatchesStrategicMerge: []interface{}{
					map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata":   map[string]interface{}{"name": "podinfo"},
						"spec":       map[string]interface{}{"replicas": 5},
					},
				},
			}
			renderer := &kustomizePostRenderer{params: params}
			result, err := renderer.Run(bytes.NewBufferString(manifest))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.String()).To(ContainSubstring("replicas: 5"))
		})

		It("should apply a patchesStrategicMerge string to matching resources", func() {
			manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: podinfo
  namespace: default
spec:
  replicas: 1
`
			patch := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: podinfo
spec:
  replicas: 7
`
			params := &KustomizeParams{
				PatchesStrategicMerge: []interface{}{patch},
			}
			renderer := &kustomizePostRenderer{params: params}
			result, err := renderer.Run(bytes.NewBufferString(manifest))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.String()).To(ContainSubstring("replicas: 7"))
		})

		It("should return unchanged buffer when manifest is empty", func() {
			params := &KustomizeParams{}
			renderer := &kustomizePostRenderer{params: params}
			buf := &bytes.Buffer{}
			result, err := renderer.Run(buf)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).To(Equal(buf))
		})

		It("should apply image tag replacement", func() {
			manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: podinfo
  namespace: default
spec:
  template:
    spec:
      containers:
      - name: podinfo
        image: ghcr.io/stefanprodan/podinfo:6.0.0
`
			params := &KustomizeParams{
				Images: []interface{}{
					map[string]interface{}{
						"name":   "ghcr.io/stefanprodan/podinfo",
						"newTag": "6.1.0",
					},
				},
			}
			renderer := &kustomizePostRenderer{params: params}
			result, err := renderer.Run(bytes.NewBufferString(manifest))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.String()).To(ContainSubstring("6.1.0"))
		})

		It("should apply a patchesJson6902 entry to matching resources", func() {
			manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: podinfo
  namespace: default
spec:
  replicas: 1
`
			params := &KustomizeParams{
				PatchesJson6902: []interface{}{
					map[string]interface{}{
						"target": map[string]interface{}{
							"kind": "Deployment",
							"name": "podinfo",
						},
						"patch": "- op: replace\n  path: /spec/replicas\n  value: 2\n",
					},
				},
			}
			renderer := &kustomizePostRenderer{params: params}
			result, err := renderer.Run(bytes.NewBufferString(manifest))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.String()).To(ContainSubstring("replicas: 2"))
		})

		It("should apply a replicas entry to matching resources", func() {
			manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: podinfo
  namespace: default
spec:
  replicas: 1
`
			params := &KustomizeParams{
				Replicas: []interface{}{
					map[string]interface{}{
						"name":  "podinfo",
						"count": 4,
					},
				},
			}
			renderer := &kustomizePostRenderer{params: params}
			result, err := renderer.Run(bytes.NewBufferString(manifest))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.String()).To(ContainSubstring("replicas: 4"))
		})

		It("should return an error when krusty run fails", func() {
			manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: podinfo
  namespace: default
spec:
  replicas: 1
`
			// Reference a patch file that does not exist in the in-memory fs
			// so krusty fails trying to load it.
			params := &KustomizeParams{
				Patches: []interface{}{
					map[string]interface{}{
						"path": "nonexistent-patch.yaml",
					},
				},
			}
			renderer := &kustomizePostRenderer{params: params}
			_, err := renderer.Run(bytes.NewBufferString(manifest))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kustomize run failed"))
		})
	})

	Describe("compositePostRenderer", func() {
		It("should chain renderers in order", func() {
			// First renderer appends "AAA", second appends "BBB".
			// Verifies ordering: first output becomes second input.
			first := &appendingRenderer{suffix: "AAA"}
			second := &appendingRenderer{suffix: "BBB"}
			composite := &compositePostRenderer{renderers: []helmPostRenderer{first, second}}
			result, err := composite.Run(bytes.NewBufferString("start\n"))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.String()).To(Equal("start\nAAABBB"))
		})

		It("should propagate errors from any renderer", func() {
			first := &appendingRenderer{suffix: "AAA"}
			bad := &errorRenderer{}
			composite := &compositePostRenderer{renderers: []helmPostRenderer{first, bad}}
			_, err := composite.Run(bytes.NewBufferString("start\n"))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("newPostRenderer", func() {
		It("should return a velaLabelPostRenderer when no kustomize params", func() {
			renderer := newPostRenderer(nil, nil, "rel", "ns")
			_, ok := renderer.(*velaLabelPostRenderer)
			Expect(ok).To(BeTrue())
		})

		It("should return a compositePostRenderer when kustomize params are set", func() {
			params := &PostRenderParams{
				Kustomize: &KustomizeParams{},
			}
			renderer := newPostRenderer(params, nil, "rel", "ns")
			composite, ok := renderer.(*compositePostRenderer)
			Expect(ok).To(BeTrue())
			Expect(composite.renderers).To(HaveLen(2))
			_, isKustomize := composite.renderers[0].(*kustomizePostRenderer)
			Expect(isKustomize).To(BeTrue())
			_, isVela := composite.renderers[1].(*velaLabelPostRenderer)
			Expect(isVela).To(BeTrue())
		})

		It("should return velaLabelPostRenderer when PostRenderParams has no kustomize", func() {
			params := &PostRenderParams{Kustomize: nil}
			renderer := newPostRenderer(params, nil, "rel", "ns")
			_, ok := renderer.(*velaLabelPostRenderer)
			Expect(ok).To(BeTrue())
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

// appendingRenderer is a test helmPostRenderer that appends a fixed suffix to the buffer.
type appendingRenderer struct{ suffix string }

func (r *appendingRenderer) Run(buf *bytes.Buffer) (*bytes.Buffer, error) {
	return bytes.NewBufferString(buf.String() + r.suffix), nil
}

// errorRenderer is a test helmPostRenderer that always returns an error.
type errorRenderer struct{}

func (r *errorRenderer) Run(_ *bytes.Buffer) (*bytes.Buffer, error) {
	return nil, fmt.Errorf("intentional test error")
}
