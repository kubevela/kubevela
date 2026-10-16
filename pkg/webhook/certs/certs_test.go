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

package certs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/pkg/utils/servingcert"
)

const (
	testNamespace = "vela-system"
	testSecret    = "kubevela-vela-core-admission"
	testConfig    = "kubevela-vela-core-admission"
	testService   = "vela-core-webhook"
	testCRD       = "applications.core.oam.dev"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, apiextensionsv1.AddToScheme(s))
	return s
}

func testClientConfig() admissionregistrationv1.WebhookClientConfig {
	return admissionregistrationv1.WebhookClientConfig{
		CABundle: []byte("\n"),
		Service:  &admissionregistrationv1.ServiceReference{Namespace: testNamespace, Name: testService, Port: ptrTo[int32](443)},
	}
}

func testMutating() *admissionregistrationv1.MutatingWebhookConfiguration {
	webhook := func(name string) admissionregistrationv1.MutatingWebhook {
		return admissionregistrationv1.MutatingWebhook{
			Name: name, ClientConfig: testClientConfig(),
			FailurePolicy: ptrTo(admissionregistrationv1.Ignore),
		}
	}
	return &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: testConfig},
		Webhooks:   []admissionregistrationv1.MutatingWebhook{webhook("mutating.a"), webhook("mutating.b")},
	}
}

func testValidating() *admissionregistrationv1.ValidatingWebhookConfiguration {
	webhook := func(name string) admissionregistrationv1.ValidatingWebhook {
		return admissionregistrationv1.ValidatingWebhook{
			Name: name, ClientConfig: testClientConfig(),
			FailurePolicy: ptrTo(admissionregistrationv1.Ignore),
		}
	}
	return &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: testConfig},
		Webhooks:   []admissionregistrationv1.ValidatingWebhook{webhook("validating.a"), webhook("validating.b")},
	}
}

func testApplicationCRD() *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: testCRD},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group:      "core.oam.dev",
			Conversion: &apiextensionsv1.CustomResourceConversion{Strategy: apiextensionsv1.NoneConverter},
		},
	}
}

func testOptions() Options {
	return Options{
		SecretNamespace:   testNamespace,
		SecretName:        testSecret,
		ConfigurationName: testConfig,
		FailurePolicy:     admissionregistrationv1.Fail,
	}
}

func getConfigs(t *testing.T, c client.Client) (*admissionregistrationv1.MutatingWebhookConfiguration, *admissionregistrationv1.ValidatingWebhookConfiguration) {
	t.Helper()
	m := &admissionregistrationv1.MutatingWebhookConfiguration{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: testConfig}, m))
	v := &admissionregistrationv1.ValidatingWebhookConfiguration{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: testConfig}, v))
	return m, v
}

func TestEnsure(t *testing.T) {
	ctx := context.Background()

	t.Run("creates the Secret and sets caBundle and failurePolicy on every webhook", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(testMutating(), testValidating()).Build()

		res, err := Ensure(ctx, c, testOptions())
		require.NoError(t, err)

		svc := servingcert.Service{Namespace: testNamespace, Name: testService}
		require.NoError(t, servingcert.Check(map[string][]byte{
			servingcert.CAKey: res.CABundle, servingcert.CertKey: res.Cert, servingcert.KeyKey: res.Key,
		}, svc.Hosts(), time.Now()))
		secret := &corev1.Secret{}
		require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: testNamespace, Name: testSecret}, secret))
		require.Equal(t, res.CABundle, secret.Data[servingcert.CAKey])

		m, v := getConfigs(t, c)
		for _, w := range m.Webhooks {
			require.Equal(t, res.CABundle, w.ClientConfig.CABundle, w.Name)
			require.Equal(t, admissionregistrationv1.Fail, *w.FailurePolicy, w.Name)
		}
		for _, w := range v.Webhooks {
			require.Equal(t, res.CABundle, w.ClientConfig.CABundle, w.Name)
			require.Equal(t, admissionregistrationv1.Fail, *w.FailurePolicy, w.Name)
		}
	})

	t.Run("does nothing on a second run", func(t *testing.T) {
		updates := 0
		base := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(testMutating(), testValidating()).Build()
		c := &hookClient{Client: base, update: func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
			updates++
			return base.Update(ctx, obj, opts...)
		}}

		first, err := Ensure(ctx, c, testOptions())
		require.NoError(t, err)
		require.Equal(t, 2, updates)

		second, err := Ensure(ctx, c, testOptions())
		require.NoError(t, err)
		require.Equal(t, first.CABundle, second.CABundle)
		require.Equal(t, first.Cert, second.Cert)
		require.Equal(t, 2, updates, "a second run must not update anything")
	})

	t.Run("leaves failurePolicy alone when none is given", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(testMutating(), testValidating()).Build()
		opts := testOptions()
		opts.FailurePolicy = ""

		_, err := Ensure(ctx, c, opts)
		require.NoError(t, err)

		m, v := getConfigs(t, c)
		for _, w := range m.Webhooks {
			require.Equal(t, admissionregistrationv1.Ignore, *w.FailurePolicy, w.Name)
		}
		for _, w := range v.Webhooks {
			require.Equal(t, admissionregistrationv1.Ignore, *w.FailurePolicy, w.Name)
		}
	})

	t.Run("works with only one configuration", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(testValidating()).Build()

		res, err := Ensure(ctx, c, testOptions())
		require.NoError(t, err)

		v := &admissionregistrationv1.ValidatingWebhookConfiguration{}
		require.NoError(t, c.Get(ctx, client.ObjectKey{Name: testConfig}, v))
		require.Equal(t, res.CABundle, v.Webhooks[0].ClientConfig.CABundle)
	})

	t.Run("fails when neither configuration exists", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(testScheme(t)).Build()

		_, err := Ensure(ctx, c, testOptions())
		require.Error(t, err)
	})

	t.Run("points the CRD conversion webhook at the webhook Service", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(testScheme(t)).
			WithObjects(testMutating(), testValidating(), testApplicationCRD()).Build()
		opts := testOptions()
		opts.ConversionCRDs = []string{testCRD}

		res, err := Ensure(ctx, c, opts)
		require.NoError(t, err)

		crd := &apiextensionsv1.CustomResourceDefinition{}
		require.NoError(t, c.Get(ctx, client.ObjectKey{Name: testCRD}, crd))
		require.Equal(t, apiextensionsv1.WebhookConverter, crd.Spec.Conversion.Strategy)
		cc := crd.Spec.Conversion.Webhook.ClientConfig
		require.Equal(t, res.CABundle, cc.CABundle)
		require.Equal(t, testService, cc.Service.Name)
		require.Equal(t, testNamespace, cc.Service.Namespace)
		require.Equal(t, "/convert", *cc.Service.Path)
		require.Equal(t, int32(443), *cc.Service.Port)
		require.Equal(t, []string{"v1beta1", "v1alpha2"}, crd.Spec.Conversion.Webhook.ConversionReviewVersions)
	})

	t.Run("does not update the CRD conversion webhook on a second run", func(t *testing.T) {
		crdUpdates := 0
		base := fake.NewClientBuilder().WithScheme(testScheme(t)).
			WithObjects(testMutating(), testValidating(), testApplicationCRD()).Build()
		c := &hookClient{Client: base, update: func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
			if _, ok := obj.(*apiextensionsv1.CustomResourceDefinition); ok {
				crdUpdates++
			}
			return base.Update(ctx, obj, opts...)
		}}
		opts := testOptions()
		opts.ConversionCRDs = []string{testCRD}

		_, err := Ensure(ctx, c, opts)
		require.NoError(t, err)
		require.Equal(t, 1, crdUpdates)

		_, err = Ensure(ctx, c, opts)
		require.NoError(t, err)
		require.Equal(t, 1, crdUpdates, "a second run must not update the CRD")
	})
}

func TestWriteFiles(t *testing.T) {
	result := func() *servingcert.Result {
		data, err := servingcert.Generate([]string{testService}, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
		require.NoError(t, err)
		return &servingcert.Result{Cert: data[servingcert.CertKey], Key: data[servingcert.KeyKey], CABundle: data[servingcert.CAKey]}
	}
	dir := filepath.Join(t.TempDir(), "serving-certs")
	first := result()

	changed, err := WriteFiles(dir, first)
	require.NoError(t, err)
	require.True(t, changed)
	got, err := os.ReadFile(filepath.Join(dir, servingcert.KeyKey))
	require.NoError(t, err)
	require.Equal(t, first.Key, got)

	// The same cert again, as on every resync: nothing is rewritten.
	changed, err = WriteFiles(dir, first)
	require.NoError(t, err)
	require.False(t, changed)

	renewed := result()
	changed, err = WriteFiles(dir, renewed)
	require.NoError(t, err)
	require.True(t, changed)
	got, err = os.ReadFile(filepath.Join(dir, servingcert.CertKey))
	require.NoError(t, err)
	require.Equal(t, renewed.Cert, got)
}

// hookClient wraps a client so a test can intercept Create and Update.
// controller-runtime on this branch has no fake-client interceptor.
type hookClient struct {
	client.Client
	create func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error
	update func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error
}

func (h *hookClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if h.create != nil {
		return h.create(ctx, obj, opts...)
	}
	return h.Client.Create(ctx, obj, opts...)
}

func (h *hookClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if h.update != nil {
		return h.update(ctx, obj, opts...)
	}
	return h.Client.Update(ctx, obj, opts...)
}
