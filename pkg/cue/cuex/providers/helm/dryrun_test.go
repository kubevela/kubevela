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
	"helm.sh/helm/v3/pkg/chart"
)

var _ = Describe("dryrun", func() {

	Describe("dryRunRender", func() {
		It("should render a chart client-side", func() {
			p := NewProviderWithConfig(nil)
			ch := &chart.Chart{
				Metadata: &chart.Metadata{
					Name:    "test-chart",
					Version: "1.0.0",
				},
				Templates: []*chart.File{
					{
						Name: "templates/deployment.yaml",
						Data: []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}
  namespace: {{ .Release.Namespace }}
spec:
  replicas: 1
`),
					},
				},
			}

			manifest, _, err := p.dryRunRender(ch, "test-release", "test-ns",
				map[string]interface{}{"key": "value"}, nil, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(manifest).To(ContainSubstring("kind: Deployment"))
			Expect(manifest).To(ContainSubstring("name: test-release"))
		})
	})

	Describe("dryRunRender with options", func() {
		var (
			p  *Provider
			ch *chart.Chart
		)

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
			ch = &chart.Chart{
				Metadata: &chart.Metadata{
					Name:    "test-chart",
					Version: "1.0.0",
				},
				Templates: []*chart.File{
					{
						Name: "templates/configmap.yaml",
						Data: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}-config
  namespace: {{ .Release.Namespace }}
data:
  key: value
`),
					},
				},
			}
		})

		It("should render with velaCtx labels injected", func() {
			velaCtx := &ContextParams{
				AppName:      "my-app",
				AppNamespace: "my-ns",
				Name:         "my-comp",
				Namespace:    "test-ns",
			}
			manifest, _, err := p.dryRunRender(ch, "my-rel", "test-ns",
				map[string]interface{}{}, nil, velaCtx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(manifest).To(ContainSubstring("kind: ConfigMap"))
			Expect(manifest).To(ContainSubstring("app.oam.dev/name"))
		})

		It("should apply skipHooks option", func() {
			skipHooks := true
			manifest, _, err := p.dryRunRender(ch, "my-rel", "test-ns",
				map[string]interface{}{}, &RenderOptionsParams{SkipHooks: &skipHooks}, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(manifest).To(ContainSubstring("kind: ConfigMap"))
		})

		It("should render multiple templates", func() {
			ch.Templates = append(ch.Templates, &chart.File{
				Name: "templates/service.yaml",
				Data: []byte(`apiVersion: v1
kind: Service
metadata:
  name: {{ .Release.Name }}-svc
  namespace: {{ .Release.Namespace }}
spec:
  ports:
    - port: 80
`),
			})
			manifest, _, err := p.dryRunRender(ch, "my-rel", "test-ns",
				map[string]interface{}{}, nil, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(manifest).To(ContainSubstring("kind: ConfigMap"))
			Expect(manifest).To(ContainSubstring("kind: Service"))
		})

		It("should fail on invalid chart template", func() {
			badChart := &chart.Chart{
				Metadata: &chart.Metadata{Name: "bad", Version: "1.0.0"},
				Templates: []*chart.File{
					{
						Name: "templates/bad.yaml",
						Data: []byte(`{{ .Values.undefined.nested.deep }}`),
					},
				},
			}
			_, _, err := p.dryRunRender(badChart, "rel", "ns",
				map[string]interface{}{}, nil, nil)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dry-run render failed"))
		})
	})

	Describe("getKubeVersion", func() {
		It("should not panic and return a result or nil", func() {
			p := NewProviderWithConfig(nil)
			// May return a KubeVersion (if cluster available) or nil (if not)
			// Either way, it should not panic
			Expect(func() {
				_ = p.getKubeVersion()
			}).ToNot(Panic())
		})
	})

})
