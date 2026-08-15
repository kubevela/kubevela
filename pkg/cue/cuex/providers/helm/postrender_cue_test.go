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
	"context"
	"fmt"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
)

// decodeManifests parses a rendered manifest bundle into objects keyed by
// "<kind>/<name>" so assertions can address a single resource.
func decodeManifests(buf *bytes.Buffer) map[string]*unstructured.Unstructured {
	objs := map[string]*unstructured.Unstructured{}
	decoder := kyaml.NewYAMLOrJSONDecoder(bytes.NewReader(buf.Bytes()), 4096)
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
		objs[fmt.Sprintf("%s/%s", obj.GetKind(), obj.GetName())] = obj
	}
	return objs
}

const twoResourceManifest = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: app
        image: nginx:1.0
        env:
        - name: EXISTING
          value: "keep"
---
apiVersion: v1
kind: Service
metadata:
  name: web
spec:
  ports:
  - port: 80
`

func newCUERenderer(template string) *cuePostRenderer {
	return &cuePostRenderer{
		ctx:    context.Background(),
		params: &CUEParams{Template: template},
		velaCtx: &ContextParams{
			AppName:      "my-app",
			AppNamespace: "my-app-ns",
			Name:         "my-component",
			Namespace:    "test-ns",
		},
	}
}

var _ = Describe("cuePostRenderer", func() {

	It("should patch a scalar field on every resource", func() {
		renderer := newCUERenderer(`
patch: metadata: labels: tier: "backend"
`)
		result, err := renderer.Run(bytes.NewBufferString(twoResourceManifest))
		Expect(err).ShouldNot(HaveOccurred())

		objs := decodeManifests(result)
		Expect(objs).To(HaveLen(2))
		Expect(objs["Deployment/web"].GetLabels()).To(HaveKeyWithValue("tier", "backend"))
		Expect(objs["Service/web"].GetLabels()).To(HaveKeyWithValue("tier", "backend"))
	})

	It("should leave resources the template does not target untouched", func() {
		renderer := newCUERenderer(`
patch: {
	if context.resource.kind == "Deployment" {
		spec: {
			// +patchStrategy=retainKeys
			replicas: 5
		}
	}
}
`)
		result, err := renderer.Run(bytes.NewBufferString(twoResourceManifest))
		Expect(err).ShouldNot(HaveOccurred())

		objs := decodeManifests(result)
		replicas, found, err := unstructured.NestedInt64(objs["Deployment/web"].Object, "spec", "replicas")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(replicas).To(Equal(int64(5)))

		// The Service must be byte-for-byte semantically unchanged: no patch
		// field is produced for it, so it passes through the decode/encode
		// round-trip only.
		Expect(objs["Service/web"].Object).To(HaveKey("spec"))
		Expect(objs["Service/web"].Object).ToNot(HaveKey("status"))
		ports, found, err := unstructured.NestedSlice(objs["Service/web"].Object, "spec", "ports")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(ports).To(HaveLen(1))
	})

	It("should merge into a list by patchKey instead of replacing it", func() {
		renderer := newCUERenderer(`
patch: {
	if context.resource.kind == "Deployment" {
		spec: template: spec: {
			// +patchKey=name
			containers: [{
				name: "app"
				// +patchKey=name
				env: [{name: "ENV", value: "prod"}]
			}]
		}
	}
}
`)
		result, err := renderer.Run(bytes.NewBufferString(twoResourceManifest))
		Expect(err).ShouldNot(HaveOccurred())

		objs := decodeManifests(result)
		containers, found, err := unstructured.NestedSlice(objs["Deployment/web"].Object, "spec", "template", "spec", "containers")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(containers).To(HaveLen(1))

		container := containers[0].(map[string]interface{})
		// The untouched fields of the matched container survive the merge.
		Expect(container["image"]).To(Equal("nginx:1.0"))

		env := container["env"].([]interface{})
		Expect(env).To(HaveLen(2))
		Expect(env[0]).To(Equal(map[string]interface{}{"name": "EXISTING", "value": "keep"}))
		Expect(env[1]).To(Equal(map[string]interface{}{"name": "ENV", "value": "prod"}))
	})

	It("should replace a list when patchStrategy=replace is set", func() {
		renderer := newCUERenderer(`
patch: {
	if context.resource.kind == "Deployment" {
		spec: template: spec: {
			// +patchStrategy=replace
			containers: [{name: "only", image: "busybox"}]
		}
	}
}
`)
		result, err := renderer.Run(bytes.NewBufferString(twoResourceManifest))
		Expect(err).ShouldNot(HaveOccurred())

		objs := decodeManifests(result)
		containers, _, err := unstructured.NestedSlice(objs["Deployment/web"].Object, "spec", "template", "spec", "containers")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(containers).To(HaveLen(1))
		Expect(containers[0].(map[string]interface{})["name"]).To(Equal("only"))
	})

	It("should expose the KubeVela context alongside the resource", func() {
		renderer := newCUERenderer(`
patch: metadata: annotations: {
	"example.com/app":       context.appName
	"example.com/namespace": context.appNamespace
	"example.com/component": context.name
	"example.com/self":      context.resource.metadata.name
}
`)
		result, err := renderer.Run(bytes.NewBufferString(twoResourceManifest))
		Expect(err).ShouldNot(HaveOccurred())

		annotations := decodeManifests(result)["Deployment/web"].GetAnnotations()
		Expect(annotations["example.com/app"]).To(Equal("my-app"))
		Expect(annotations["example.com/namespace"]).To(Equal("my-app-ns"))
		Expect(annotations["example.com/component"]).To(Equal("my-component"))
		Expect(annotations["example.com/self"]).To(Equal("web"))
	})

	It("should tolerate a missing vela context", func() {
		renderer := &cuePostRenderer{
			ctx:    context.Background(),
			params: &CUEParams{Template: `patch: metadata: labels: tier: "backend"`},
		}
		result, err := renderer.Run(bytes.NewBufferString(twoResourceManifest))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(decodeManifests(result)["Deployment/web"].GetLabels()).To(HaveKeyWithValue("tier", "backend"))
	})

	It("should pass manifests through when the template produces no patch field", func() {
		renderer := newCUERenderer(`somethingElse: {foo: "bar"}`)
		result, err := renderer.Run(bytes.NewBufferString(twoResourceManifest))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(decodeManifests(result)).To(HaveLen(2))
	})

	It("should pass manifests through unchanged for nil, empty, or blank templates", func() {
		for _, params := range []*CUEParams{nil, {Template: ""}, {Template: "   \n\t "}} {
			renderer := &cuePostRenderer{ctx: context.Background(), params: params}
			in := bytes.NewBufferString(twoResourceManifest)
			result, err := renderer.Run(in)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).To(BeIdenticalTo(in), "expected the input buffer to be returned untouched")
		}
	})

	It("should pass through an empty manifest bundle", func() {
		renderer := newCUERenderer(`patch: metadata: labels: tier: "backend"`)
		in := &bytes.Buffer{}
		result, err := renderer.Run(in)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(result).To(BeIdenticalTo(in))
	})

	It("should fail on a template that does not parse", func() {
		renderer := newCUERenderer(`patch: {this is not cue`)
		_, err := renderer.Run(bytes.NewBufferString(twoResourceManifest))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("cue post-renderer"))
	})

	It("should fail on a patch that conflicts with the rendered resource", func() {
		renderer := newCUERenderer(`
patch: {
	if context.resource.kind == "Deployment" {
		spec: replicas: "not-an-int"
	}
}
`)
		_, err := renderer.Run(bytes.NewBufferString(twoResourceManifest))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Deployment"))
	})

	It("should reject overriding a concrete value without retainKeys", func() {
		// Unification cannot narrow an already-concrete scalar, so overriding a
		// value the chart rendered requires an explicit +patchStrategy=retainKeys.
		// Failing loudly here is what keeps a silent no-op from shipping.
		renderer := newCUERenderer(`
patch: {
	if context.resource.kind == "Deployment" {
		spec: replicas: 5
	}
}
`)
		_, err := renderer.Run(bytes.NewBufferString(twoResourceManifest))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("conflicting values"))
	})

	It("should fail on a patch that leaves the resource incomplete", func() {
		renderer := newCUERenderer(`patch: metadata: labels: tier: string`)
		_, err := renderer.Run(bytes.NewBufferString(twoResourceManifest))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("incomplete"))
	})

	It("should fail on malformed input manifests", func() {
		renderer := newCUERenderer(`patch: metadata: labels: tier: "backend"`)
		_, err := renderer.Run(bytes.NewBufferString("this: is: not: valid: yaml:\n\t- broken"))
		Expect(err).To(HaveOccurred())
	})

	It("should stop when the context is cancelled", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		renderer := &cuePostRenderer{
			ctx:    ctx,
			params: &CUEParams{Template: `patch: metadata: labels: tier: "backend"`},
		}
		_, err := renderer.Run(bytes.NewBufferString(twoResourceManifest))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("cancelled"))
	})
})

var _ = Describe("compositePostRenderer", func() {

	It("should run renderers in order and feed each output to the next", func() {
		renderer := &compositePostRenderer{
			renderers: []helmPostRenderer{
				newCUERenderer(`patch: metadata: labels: first: "1"`),
				newCUERenderer(`patch: metadata: labels: second: context.resource.metadata.labels.first`),
			},
		}
		result, err := renderer.Run(bytes.NewBufferString(twoResourceManifest))
		Expect(err).ShouldNot(HaveOccurred())

		labels := decodeManifests(result)["Deployment/web"].GetLabels()
		Expect(labels).To(HaveKeyWithValue("first", "1"))
		Expect(labels).To(HaveKeyWithValue("second", "1"))
	})

	It("should surface an error from any renderer in the chain", func() {
		renderer := &compositePostRenderer{
			renderers: []helmPostRenderer{
				newCUERenderer(`patch: {this is not cue`),
				newCUERenderer(`patch: metadata: labels: tier: "backend"`),
			},
		}
		_, err := renderer.Run(bytes.NewBufferString(twoResourceManifest))
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("newPostRenderer", func() {

	It("should return the bare vela renderer when no post-rendering is configured", func() {
		for _, postRender := range []*PostRenderParams{nil, {}} {
			renderer := newPostRenderer(context.Background(), postRender, &ContextParams{AppName: "app"}, "rel", "ns")
			Expect(renderer).To(BeAssignableToTypeOf(&velaLabelPostRenderer{}))
		}
	})

	It("should chain the CUE renderer ahead of the vela renderer", func() {
		renderer := newPostRenderer(context.Background(),
			&PostRenderParams{CUE: &CUEParams{Template: `patch: metadata: labels: tier: "backend"`}},
			&ContextParams{AppName: "my-app", AppNamespace: "my-app-ns", Name: "my-component"},
			"my-release", "test-ns")

		composite, ok := renderer.(*compositePostRenderer)
		Expect(ok).To(BeTrue())
		Expect(composite.renderers).To(HaveLen(2))
		Expect(composite.renderers[0]).To(BeAssignableToTypeOf(&cuePostRenderer{}))
		Expect(composite.renderers[1]).To(BeAssignableToTypeOf(&velaLabelPostRenderer{}))
	})

	It("should apply vela ownership labels after the user patch, so they cannot be overwritten", func() {
		renderer := newPostRenderer(context.Background(),
			&PostRenderParams{CUE: &CUEParams{Template: `patch: metadata: labels: "app.oam.dev/name": "hijacked"`}},
			&ContextParams{AppName: "my-app", AppNamespace: "my-app-ns", Name: "my-component"},
			"my-release", "test-ns")

		result, err := renderer.Run(bytes.NewBufferString(twoResourceManifest))
		Expect(err).ShouldNot(HaveOccurred())

		labels := decodeManifests(result)["Deployment/web"].GetLabels()
		Expect(labels["app.oam.dev/name"]).To(Equal("my-app"))
	})
})
