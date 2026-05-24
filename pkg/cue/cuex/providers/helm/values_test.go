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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kubevela/pkg/util/singleton"
)

var _ = Describe("values", func() {

	Describe("mergeValues", func() {
		var (
			p   *Provider
			ctx context.Context
		)

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
			ctx = context.Background()
			// Empty fake client so ConfigMap/Secret Gets return NotFound. Tests
			// that need specific resources override the singleton themselves.
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).Build())
		})

		It("should return base values when no valuesFrom", func() {
			baseValues := map[string]interface{}{
				"key1": "value1",
				"nested": map[string]interface{}{
					"key2": "value2",
				},
			}
			result, err := p.mergeValues(ctx, baseValues, nil, "default", "default")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).To(Equal(baseValues))
		})

		It("should return empty map for nil base values", func() {
			result, err := p.mergeValues(ctx, nil, nil, "default", "default")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result).To(BeEmpty())
		})

		It("should skip optional source when ConfigMap is missing", func() {
			base := map[string]interface{}{"key": "value"}
			valuesFrom := []ValuesFromParams{
				{Kind: "ConfigMap", Name: "missing", Optional: true},
			}
			result, err := p.mergeValues(ctx, base, valuesFrom, "default", "default")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result["key"]).To(Equal("value"))
		})

		It("should propagate missing-source errors when not optional", func() {
			base := map[string]interface{}{"key": "value"}
			valuesFrom := []ValuesFromParams{
				{Kind: "ConfigMap", Name: "missing", Optional: false},
			}
			_, err := p.mergeValues(ctx, base, valuesFrom, "default", "default")
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to load values"))
		})
	})

	Describe("loadValuesFromSource dispatcher", func() {
		var p *Provider

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
		})

		It("returns an error for unsupported kinds (including the reserved OCIRepository)", func() {
			for _, kind := range []string{"OCIRepository", "Unknown", "configmap", ""} {
				_, err := p.loadValuesFromSource(context.Background(),
					ValuesFromParams{Kind: kind, Name: "test"},
					"default", "default")
				Expect(err).Should(HaveOccurred(), "kind=%q must fail", kind)
				Expect(err.Error()).To(ContainSubstring("unsupported values source kind"),
					"kind=%q must surface as unsupported", kind)
			}
		})
	})

	Describe("cross-namespace valuesFrom rejection", func() {
		var p *Provider

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).Build())
		})

		It("rejects a ConfigMap reference to a namespace other than the Application's or release's", func() {
			// Distinct app vs release namespaces so the guard must reject a
			// third unrelated namespace (kube-system here) on both axes.
			_, err := p.loadConfigMapValues(context.Background(),
				ValuesFromParams{Kind: "ConfigMap", Name: "secrets-bearer", Namespace: "kube-system"},
				"tenant-a", "release-a")
			Expect(err).Should(HaveOccurred())
			Expect(errors.Is(err, errCrossNamespaceValuesFrom)).To(BeTrue(),
				"cross-ns error must be detectable via errors.Is")
			Expect(err.Error()).To(ContainSubstring("kube-system"))
			Expect(err.Error()).To(ContainSubstring("tenant-a"))
			Expect(err.Error()).To(ContainSubstring("release-a"))
		})

		It("rejects a Secret reference to a namespace other than the Application's or release's", func() {
			_, err := p.loadSecretValues(context.Background(),
				ValuesFromParams{Kind: "Secret", Name: "any", Namespace: "other-tenant"},
				"tenant-a", "release-a")
			Expect(err).Should(HaveOccurred())
			Expect(errors.Is(err, errCrossNamespaceValuesFrom)).To(BeTrue())
		})

		It("allows an explicit Namespace equal to the Application's namespace", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "same", Namespace: "tenant-a"},
				Data:       map[string]string{"values.yaml": "k: v"},
			}
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build())
			values, err := p.loadConfigMapValues(context.Background(),
				ValuesFromParams{Kind: "ConfigMap", Name: "same", Namespace: "tenant-a"},
				"tenant-a", "release-a")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values["k"]).To(Equal("v"))
		})

		It("allows an explicit Namespace equal to the release's namespace", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "same", Namespace: "release-a"},
				Data:       map[string]string{"values.yaml": "k: v"},
			}
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build())
			values, err := p.loadConfigMapValues(context.Background(),
				ValuesFromParams{Kind: "ConfigMap", Name: "same", Namespace: "release-a"},
				"tenant-a", "release-a")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values["k"]).To(Equal("v"))
		})
	})

	Describe("loadConfigMapValues", func() {
		const releaseNS = "prod"

		var p *Provider

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
		})

		buildClient := func(objs ...client.Object) {
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, o := range objs {
				builder = builder.WithObjects(o)
			}
			singleton.KubeClient.Set(builder.Build())
		}

		It("loads from the default values.yaml key when Key is empty", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "replicaCount: 3\nimage: nginx"},
			}
			buildClient(cm)

			values, err := p.loadConfigMapValues(context.Background(),
				ValuesFromParams{Kind: "ConfigMap", Name: "cfg"}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values["replicaCount"]).To(BeEquivalentTo(3))
			Expect(values["image"]).To(Equal("nginx"))
		})

		It("loads from an explicit Key", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: releaseNS},
				Data:       map[string]string{"prod.yaml": "replicaCount: 5"},
			}
			buildClient(cm)

			values, err := p.loadConfigMapValues(context.Background(),
				ValuesFromParams{Kind: "ConfigMap", Name: "cfg", Key: "prod.yaml"}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values["replicaCount"]).To(BeEquivalentTo(5))
		})

		It("accepts an explicit Namespace that equals the Application's namespace", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "replicaCount: 7"},
			}
			buildClient(cm)

			values, err := p.loadConfigMapValues(context.Background(),
				ValuesFromParams{Kind: "ConfigMap", Name: "cfg", Namespace: releaseNS}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values["replicaCount"]).To(BeEquivalentTo(7))
		})

		It("returns a missing-source error when the ConfigMap does not exist", func() {
			buildClient()
			_, err := p.loadConfigMapValues(context.Background(),
				ValuesFromParams{Kind: "ConfigMap", Name: "absent"}, releaseNS, releaseNS)
			Expect(err).Should(HaveOccurred())
			Expect(isValueSourceMissing(err)).To(BeTrue(),
				"missing ConfigMap should surface as valueSourceMissingError")
		})

		It("returns a missing-source error when the key is absent", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: releaseNS},
				Data:       map[string]string{"other.yaml": "foo: bar"},
			}
			buildClient(cm)

			_, err := p.loadConfigMapValues(context.Background(),
				ValuesFromParams{Kind: "ConfigMap", Name: "cfg"}, releaseNS, releaseNS)
			Expect(err).Should(HaveOccurred())
			Expect(isValueSourceMissing(err)).To(BeTrue())
		})

		It("surfaces YAML parse errors and never classifies them as missing", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "replicas: [unterminated"},
			}
			buildClient(cm)

			_, err := p.loadConfigMapValues(context.Background(),
				ValuesFromParams{Kind: "ConfigMap", Name: "cfg"}, releaseNS, releaseNS)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid YAML"))
			Expect(isValueSourceMissing(err)).To(BeFalse(),
				"parse errors must NOT be swallowed by optional")
		})
	})

	Describe("loadSecretValues", func() {
		const releaseNS = "prod"

		var p *Provider

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
		})

		buildClient := func(objs ...client.Object) {
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, o := range objs {
				builder = builder.WithObjects(o)
			}
			singleton.KubeClient.Set(builder.Build())
		}

		It("loads YAML from Secret.Data (already base64-decoded by the API)", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: releaseNS},
				Data: map[string][]byte{
					"values.yaml": []byte("password: s3cret\nuser: admin"),
				},
			}
			buildClient(secret)

			values, err := p.loadSecretValues(context.Background(),
				ValuesFromParams{Kind: "Secret", Name: "creds"}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values["user"]).To(Equal("admin"))
			Expect(values["password"]).To(Equal("s3cret"))
		})

		It("returns a missing-source error when the Secret does not exist", func() {
			buildClient()
			_, err := p.loadSecretValues(context.Background(),
				ValuesFromParams{Kind: "Secret", Name: "absent"}, releaseNS, releaseNS)
			Expect(err).Should(HaveOccurred())
			Expect(isValueSourceMissing(err)).To(BeTrue())
		})

		It("surfaces YAML parse errors and does not leak raw secret bytes", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: releaseNS},
				Data:       map[string][]byte{"values.yaml": []byte("super-secret: [unterminated")},
			}
			buildClient(secret)

			_, err := p.loadSecretValues(context.Background(),
				ValuesFromParams{Kind: "Secret", Name: "creds"}, releaseNS, releaseNS)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).ToNot(ContainSubstring("super-secret"),
				"Secret contents must never appear in error messages")
			Expect(isValueSourceMissing(err)).To(BeFalse())
		})
	})

	Describe("mergeValues priority", func() {
		const releaseNS = "prod"

		var (
			p   *Provider
			ctx context.Context
		)

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
			ctx = context.Background()
		})

		It("gives inline values the highest priority over valuesFrom", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "replicaCount: 3\nimage: from-configmap"},
			}
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build())

			base := map[string]interface{}{"image": "from-inline"}
			result, err := p.mergeValues(ctx, base,
				[]ValuesFromParams{{Kind: "ConfigMap", Name: "cm"}}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result["image"]).To(Equal("from-inline"), "inline must win over ConfigMap")
			Expect(result["replicaCount"]).To(BeEquivalentTo(3), "CM-only keys must remain")
		})

		It("makes later valuesFrom entries override earlier ones", func() {
			cmA := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "tier: free\ncolour: blue"},
			}
			cmB := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "tier: paid"},
			}
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(cmA, cmB).Build())

			result, err := p.mergeValues(ctx, nil,
				[]ValuesFromParams{
					{Kind: "ConfigMap", Name: "a"},
					{Kind: "ConfigMap", Name: "b"},
				}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result["tier"]).To(Equal("paid"), "later source must win on conflict")
			Expect(result["colour"]).To(Equal("blue"), "earlier source keeps non-overridden keys")
		})
	})

	Describe("mergeValues edge cases", func() {
		const releaseNS = "prod"

		var (
			p   *Provider
			ctx context.Context
		)

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
			ctx = context.Background()
		})

		buildClient := func(objs ...client.Object) {
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, o := range objs {
				builder = builder.WithObjects(o)
			}
			singleton.KubeClient.Set(builder.Build())
		}

		It("treats empty valuesFrom slice equivalently to nil", func() {
			buildClient()
			base := map[string]interface{}{"key": "value"}

			fromNil, errNil := p.mergeValues(ctx, base, nil, releaseNS, releaseNS)
			fromEmpty, errEmpty := p.mergeValues(ctx, base, []ValuesFromParams{}, releaseNS, releaseNS)

			Expect(errNil).ShouldNot(HaveOccurred())
			Expect(errEmpty).ShouldNot(HaveOccurred())
			Expect(fromEmpty).To(Equal(fromNil))
		})

		It("skips a missing optional source and continues with a following required source", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "real", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "replicaCount: 7"},
			}
			buildClient(cm)

			result, err := p.mergeValues(ctx, nil, []ValuesFromParams{
				{Kind: "ConfigMap", Name: "missing", Optional: true},
				{Kind: "ConfigMap", Name: "real"},
			}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result["replicaCount"]).To(BeEquivalentTo(7))
		})

		It("preserves orthogonal nested keys while resolving conflicts at depth", func() {
			cmA := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: releaseNS},
				Data: map[string]string{"values.yaml": `resources:
  limits:
    cpu: 100m
    memory: 256Mi
  requests:
    cpu: 50m`},
			}
			cmB := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: releaseNS},
				Data: map[string]string{"values.yaml": `resources:
  limits:
    memory: 512Mi`},
			}
			buildClient(cmA, cmB)

			result, err := p.mergeValues(ctx, nil, []ValuesFromParams{
				{Kind: "ConfigMap", Name: "a"},
				{Kind: "ConfigMap", Name: "b"},
			}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())

			resources := result["resources"].(map[string]interface{})
			limits := resources["limits"].(map[string]interface{})
			requests := resources["requests"].(map[string]interface{})
			Expect(limits["memory"]).To(Equal("512Mi"), "later source wins on conflict deep in the tree")
			Expect(limits["cpu"]).To(Equal("100m"), "orthogonal sibling in the same sub-object preserved")
			Expect(requests["cpu"]).To(Equal("50m"), "untouched sub-object preserved in full")
		})

		It("replaces array values instead of merging them (helm CoalesceTables semantics)", func() {
			cmA := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "extraArgs:\n  - --level=debug\n  - --timeout=30"},
			}
			cmB := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "extraArgs:\n  - --level=info"},
			}
			buildClient(cmA, cmB)

			result, err := p.mergeValues(ctx, nil, []ValuesFromParams{
				{Kind: "ConfigMap", Name: "a"},
				{Kind: "ConfigMap", Name: "b"},
			}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())

			args := result["extraArgs"].([]interface{})
			Expect(args).To(HaveLen(1), "later array wholly replaces earlier array")
			Expect(args[0]).To(Equal("--level=info"))
		})

		It("surfaces parse errors even when Optional is true", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "broken", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "replicas: [unterminated"},
			}
			buildClient(cm)

			_, err := p.mergeValues(ctx, nil, []ValuesFromParams{
				{Kind: "ConfigMap", Name: "broken", Optional: true},
			}, releaseNS, releaseNS)
			Expect(err).Should(HaveOccurred(),
				"Optional must not mask parse errors — this is the critical contract")
			Expect(err.Error()).To(ContainSubstring("invalid YAML"))
			Expect(isValueSourceMissing(err)).To(BeFalse())
		})

		It("mixes a Secret and ConfigMap in the same valuesFrom list", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "replicaCount: 2\nimage: cm-image"},
			}
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: releaseNS},
				Data:       map[string][]byte{"values.yaml": []byte("image: secret-image")},
			}
			buildClient(cm, secret)

			result, err := p.mergeValues(ctx, nil, []ValuesFromParams{
				{Kind: "ConfigMap", Name: "cm"},
				{Kind: "Secret", Name: "creds"},
			}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result["image"]).To(Equal("secret-image"), "later Secret wins over earlier ConfigMap")
			Expect(result["replicaCount"]).To(BeEquivalentTo(2), "orthogonal CM key preserved")
		})
	})

	Describe("deepCloneValues", func() {
		It("returns nil for nil input", func() {
			Expect(deepCloneValues(nil)).To(BeNil())
		})

		It("preserves Go int values (does not coerce to float64)", func() {
			in := map[string]interface{}{"replicas": 2, "name": "podinfo"}
			out := deepCloneValues(in)
			Expect(out["replicas"]).To(BeAssignableToTypeOf(int(0)))
			Expect(out["replicas"]).To(Equal(2))
		})

		It("preserves Go int64 and float64 values", func() {
			in := map[string]interface{}{"big": int64(1 << 40), "ratio": float64(1.5)}
			out := deepCloneValues(in)
			Expect(out["big"]).To(BeAssignableToTypeOf(int64(0)))
			Expect(out["big"]).To(Equal(int64(1 << 40)))
			Expect(out["ratio"]).To(BeAssignableToTypeOf(float64(0)))
			Expect(out["ratio"]).To(Equal(1.5))
		})

		It("isolates nested maps so the caller's tree is not mutated through CoalesceTables", func() {
			in := map[string]interface{}{
				"resources": map[string]interface{}{
					"limits":   map[string]interface{}{"cpu": "100m"},
					"requests": map[string]interface{}{"cpu": "50m"},
				},
			}
			out := deepCloneValues(in)
			// Mutate the clone; the original must be untouched.
			out["resources"].(map[string]interface{})["limits"].(map[string]interface{})["cpu"] = "999m"
			Expect(in["resources"].(map[string]interface{})["limits"].(map[string]interface{})["cpu"]).To(Equal("100m"))
		})

		It("isolates slices", func() {
			in := map[string]interface{}{"args": []interface{}{"--debug", "--timeout=30s"}}
			out := deepCloneValues(in)
			out["args"].([]interface{})[0] = "MUTATED"
			Expect(in["args"].([]interface{})[0]).To(Equal("--debug"))
		})
	})

})
