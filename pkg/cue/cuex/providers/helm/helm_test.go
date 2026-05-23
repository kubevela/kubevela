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
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/chart"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/kubevela/pkg/cue/cuex/providers"
	"github.com/kubevela/pkg/util/singleton"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var _ = Describe("Helm Provider", func() {
	Describe("Render function (via provider)", func() {
		It("should exercise dry-run render path with pre-cached chart", func() {
			p := NewProviderWithConfig(nil)

			// Pre-seed a chart in cache so fetchChart succeeds
			testChart := &chart.Chart{
				Metadata: &chart.Metadata{Name: "cached-render", Version: "1.0.0"},
				Templates: []*chart.File{
					{
						Name: "templates/deploy.yaml",
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
			p.cache.Put("repo/my-chart/1.0.0", testChart, 1*time.Hour)

			// Call the provider's internal render logic in dry-run mode
			ctx := WithDryRun(context.Background())
			manifest, notes, err := p.dryRunRender(testChart, "release", "default",
				map[string]interface{}{}, nil, nil)
			Expect(err).ShouldNot(HaveOccurred())
			_ = ctx
			_ = notes

			// Parse the manifest
			resources, err := p.parseManifestResources(manifest, nil, "")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(resources).To(HaveLen(1))

			kind, _, _ := unstructured.NestedString(resources[0], "kind")
			Expect(kind).To(Equal("Deployment"))
		})
	})

	Describe("Render top-level function", func() {
		It("should render via dry-run path with context and release params", func() {
			p := NewProvider()

			testChart := &chart.Chart{
				Metadata: &chart.Metadata{Name: "render-dryrun", Version: "1.0.0"},
				Templates: []*chart.File{
					{
						Name: "templates/deploy.yaml",
						Data: []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}
  namespace: {{ .Release.Namespace }}
spec:
  replicas: 1
`),
					},
					{
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
					},
				},
			}
			p.cache.Put("repo/render-dryrun/1.0.0", testChart, 1*time.Hour)

			ctx := WithDryRun(context.Background())
			result, err := Render(ctx, &providers.Params[RenderParams]{
				Params: RenderParams{
					Chart: ChartSourceParams{
						Source:  "render-dryrun",
						Version: "1.0.0",
					},
					Release: &ReleaseParams{
						Name:      "my-release",
						Namespace: "test-ns",
					},
					Values: map[string]interface{}{"replicas": 2},
					Context: &ContextParams{
						AppName:      "my-app",
						AppNamespace: "my-ns",
						Name:         "my-comp",
						Namespace:    "test-ns",
					},
				},
			})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(len(result.Returns.Resources)).To(BeNumerically(">=", 1))

			// Verify resource structure
			kind, _, _ := unstructured.NestedString(result.Returns.Resources[0], "kind")
			Expect(kind).ToNot(BeEmpty())
		})

		It("should use default release name and namespace when not specified", func() {
			p := NewProvider()

			testChart := &chart.Chart{
				Metadata: &chart.Metadata{Name: "render-defaults", Version: "2.0.0"},
				Templates: []*chart.File{
					{
						Name: "templates/cm.yaml",
						Data: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}-cm
data:
  key: value
`),
					},
				},
			}
			p.cache.Put("repo/render-defaults/2.0.0", testChart, 1*time.Hour)

			ctx := WithDryRun(context.Background())
			result, err := Render(ctx, &providers.Params[RenderParams]{
				Params: RenderParams{
					Chart: ChartSourceParams{
						Source:  "render-defaults",
						Version: "2.0.0",
					},
					// No Release, no Context — use defaults
				},
			})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.Returns.Resources).To(HaveLen(1))
		})

		It("should return error when chart fetch fails", func() {
			ctx := WithDryRun(context.Background())
			_, err := Render(ctx, &providers.Params[RenderParams]{
				Params: RenderParams{
					Chart: ChartSourceParams{
						Source:  "nonexistent-chart-xyz",
						Version: "99.99.99",
					},
				},
			})
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to fetch chart"))
		})
	})

	Describe("Render Application publishVersion lookup defense", func() {
		var scheme *runtime.Scheme

		BeforeEach(func() {
			scheme = runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			Expect(v1beta1.AddToScheme(scheme)).To(Succeed())
		})

		It("propagates a transient API error from the Application Get rather than bypassing the pin", func() {
			injected := errors.New("etcdserver: leader changed")
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, kc client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if _, ok := obj.(*v1beta1.Application); ok {
							return injected
						}
						return kc.Get(ctx, key, obj, opts...)
					},
				}).
				Build()
			singleton.KubeClient.Set(c)

			_, err := Render(context.Background(), &providers.Params[RenderParams]{
				Params: RenderParams{
					Chart:   ChartSourceParams{Source: "render-defense-1", Version: "1.0.0"},
					Release: &ReleaseParams{Name: "rel", Namespace: "tenant-a"},
					Context: &ContextParams{
						AppName:      "my-app",
						AppNamespace: "tenant-a",
						Name:         "my-comp",
						Namespace:    "tenant-a",
					},
				},
			})
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read Application tenant-a/my-app for publishVersion lookup"),
				"defense must surface the wrapped App-lookup error")
			Expect(err.Error()).To(ContainSubstring("etcdserver: leader changed"),
				"defense must preserve the underlying cause")
		})

		It("treats a NotFound on the Application as deletion-in-flight and proceeds past the lookup", func() {
			// No App registered → NotFound from the typed fake client.
			// Render must continue past the lookup; we prove it by leaving
			// the chart un-cached so the next failure is from fetchChart,
			// not the App lookup.
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			singleton.KubeClient.Set(c)

			_, err := Render(context.Background(), &providers.Params[RenderParams]{
				Params: RenderParams{
					Chart:   ChartSourceParams{Source: "render-defense-notfound-xyz", Version: "9.9.9"},
					Release: &ReleaseParams{Name: "rel", Namespace: "tenant-a"},
					Context: &ContextParams{
						AppName:      "deleted-app",
						AppNamespace: "tenant-a",
						Name:         "my-comp",
						Namespace:    "tenant-a",
					},
				},
			})
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).ToNot(ContainSubstring("failed to read Application"),
				"NotFound on App must not be reported as a pin-lookup error")
			Expect(err.Error()).To(ContainSubstring("failed to fetch chart"),
				"Render must proceed past the App lookup to the chart fetch step")
		})

		It("succeeds the App lookup when the Application exists with a publishVersion annotation", func() {
			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "pinned-app",
					Namespace:   "tenant-a",
					Annotations: map[string]string{oam.AnnotationPublishVersion: "v42"},
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(app).Build()
			singleton.KubeClient.Set(c)

			_, err := Render(context.Background(), &providers.Params[RenderParams]{
				Params: RenderParams{
					Chart:   ChartSourceParams{Source: "render-defense-pinned-xyz", Version: "9.9.9"},
					Release: &ReleaseParams{Name: "rel", Namespace: "tenant-a"},
					Context: &ContextParams{
						AppName:      "pinned-app",
						AppNamespace: "tenant-a",
						Name:         "my-comp",
						Namespace:    "tenant-a",
					},
				},
			})
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).ToNot(ContainSubstring("failed to read Application"),
				"a present App with a pin must not surface a lookup error")
			Expect(err.Error()).To(ContainSubstring("failed to fetch chart"),
				"Render must proceed past the App lookup to the chart fetch step")
		})

		It("skips the App lookup entirely in dry-run", func() {
			// Dry-run is the webhook admission path; it must never depend on
			// cluster state. Inject a Get error: if the lookup ran, Render
			// would surface it. Instead, dry-run renders client-side.
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, kc client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if _, ok := obj.(*v1beta1.Application); ok {
							return errors.New("dry-run must not call Get on Application")
						}
						return kc.Get(ctx, key, obj, opts...)
					},
				}).
				Build()
			singleton.KubeClient.Set(c)

			p := NewProvider()
			p.cache.Put("repo/render-defense-dryrun/1.0.0", &chart.Chart{
				Metadata: &chart.Metadata{Name: "render-defense-dryrun", Version: "1.0.0"},
				Templates: []*chart.File{{
					Name: "templates/cm.yaml",
					Data: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ .Release.Name }}-cm\n"),
				}},
			}, 1*time.Hour)

			ctx := WithDryRun(context.Background())
			_, err := Render(ctx, &providers.Params[RenderParams]{
				Params: RenderParams{
					Chart:   ChartSourceParams{Source: "render-defense-dryrun", Version: "1.0.0"},
					Release: &ReleaseParams{Name: "rel", Namespace: "tenant-a"},
					Context: &ContextParams{
						AppName:      "any-app",
						AppNamespace: "tenant-a",
						Name:         "my-comp",
						Namespace:    "tenant-a",
					},
				},
			})
			Expect(err).ShouldNot(HaveOccurred(),
				"dry-run must not exercise the App lookup path")
		})
	})
})
