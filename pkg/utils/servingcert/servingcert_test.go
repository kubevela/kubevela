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

package servingcert

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testNamespace = "vela-system"
	testSecret    = "serving-tls"
	testService   = "my-service"
)

var testServiceRef = Service{Namespace: testNamespace, Name: testService}

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	return s
}

func newClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(objs...).Build()
}

func secretFrom(data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: testSecret, Namespace: testNamespace}, Data: data}
}

// jobSecret is like the Secrets the old kube-webhook-certgen Jobs made: no CA
// key, and only the two required names.
func jobSecret(t *testing.T, notAfter time.Time) *corev1.Secret {
	t.Helper()
	data, err := Generate(testServiceRef.RequiredHosts(), time.Now().Add(-time.Hour), notAfter)
	require.NoError(t, err)
	delete(data, CAKeyKey)
	return secretFrom(data)
}

func getSecret(t *testing.T, c client.Client) *corev1.Secret {
	t.Helper()
	secret := &corev1.Secret{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: testSecret}, secret))
	return secret
}

func bundleCerts(t *testing.T, bundle []byte) []*x509.Certificate {
	t.Helper()
	var certs []*x509.Certificate
	for {
		var block *pem.Block
		block, bundle = pem.Decode(bundle)
		if block == nil {
			return certs
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)
		certs = append(certs, cert)
	}
}

func TestGenerateAndCheck(t *testing.T) {
	hosts := testServiceRef.Hosts()
	data, err := Generate(hosts, time.Now().Add(-time.Hour), time.Now().Add(365*24*time.Hour))
	require.NoError(t, err)
	require.NotEmpty(t, data[CAKeyKey])
	require.NoError(t, Check(data, hosts, time.Now()))
	require.NoError(t, Check(data, testServiceRef.RequiredHosts(), time.Now().Add(300*24*time.Hour)))

	require.Error(t, Check(data, hosts, time.Now().Add(400*24*time.Hour)), "expired at validUntil")
	require.Error(t, Check(data, []string{"other-service"}, time.Now()), "host not covered")

	other, err := Generate(hosts, time.Now().Add(-time.Hour), time.Now().Add(365*24*time.Hour))
	require.NoError(t, err)
	require.Error(t, Check(map[string][]byte{CAKey: other[CAKey], CertKey: data[CertKey], KeyKey: data[KeyKey]}, hosts, time.Now()),
		"cert not signed by the CA")
	require.Error(t, Check(map[string][]byte{CAKey: data[CAKey], CertKey: data[CertKey], KeyKey: other[KeyKey]}, hosts, time.Now()),
		"key does not match the cert")
	require.Error(t, Check(map[string][]byte{CAKey: []byte("garbage")}, hosts, time.Now()))
	require.Error(t, Check(nil, hosts, time.Now()))
}

func TestEnsureSecret(t *testing.T) {
	ctx := context.Background()
	inAYear := time.Now().Add(365 * 24 * time.Hour)

	t.Run("creates the Secret, with the CA key, when missing", func(t *testing.T) {
		c := newClient(t)

		res, err := EnsureSecret(ctx, c, testNamespace, testSecret, testServiceRef)
		require.NoError(t, err)

		secret := getSecret(t, c)
		require.Equal(t, res.CABundle, secret.Data[CAKey])
		require.NotEmpty(t, secret.Data[CAKeyKey])
		require.NoError(t, Check(secret.Data, testServiceRef.Hosts(), time.Now()))
		require.Equal(t, secret.Data[CAKey], res.CABundle)
	})

	t.Run("reuses a valid Secret made by the old Job", func(t *testing.T) {
		existing := jobSecret(t, inAYear)
		c := newClient(t, existing.DeepCopy())

		res, err := EnsureSecret(ctx, c, testNamespace, testSecret, testServiceRef)
		require.NoError(t, err)

		require.Equal(t, existing.Data, getSecret(t, c).Data)
		require.Equal(t, existing.Data[CAKey], res.CABundle)
		require.Equal(t, existing.Data[CertKey], res.Cert)
	})

	t.Run("renews an expiring serving cert with the same CA when it has the CA key", func(t *testing.T) {
		now := time.Now()
		data, err := Generate(testServiceRef.Hosts(), now.Add(-time.Hour), now.Add(validity))
		require.NoError(t, err)
		// Swap in a serving cert that expires in a week, signed by the same CA.
		short, err := issue(data, testServiceRef.Hosts(), now.Add(-time.Hour), now.Add(7*24*time.Hour))
		require.NoError(t, err)
		existing := secretFrom(short)
		c := newClient(t, existing.DeepCopy())

		res, err := EnsureSecret(ctx, c, testNamespace, testSecret, testServiceRef)
		require.NoError(t, err)

		secret := getSecret(t, c)
		require.Equal(t, existing.Data[CAKey], secret.Data[CAKey], "the CA must not change")
		require.Equal(t, existing.Data[CAKey], res.CABundle, "so the caBundle does not change either")
		require.NotEqual(t, existing.Data[CertKey], secret.Data[CertKey])
		require.NoError(t, Check(secret.Data, testServiceRef.Hosts(), time.Now().Add(renewBefore)))
	})

	t.Run("replaces the CA of an expiring Job Secret and trusts both CAs for a while", func(t *testing.T) {
		existing := jobSecret(t, time.Now().Add(7*24*time.Hour))
		c := newClient(t, existing.DeepCopy())

		res, err := EnsureSecret(ctx, c, testNamespace, testSecret, testServiceRef)
		require.NoError(t, err)

		secret := getSecret(t, c)
		require.NotEqual(t, existing.Data[CAKey], secret.Data[CAKey])
		require.NoError(t, Check(secret.Data, testServiceRef.Hosts(), time.Now().Add(renewBefore)))
		require.Equal(t, existing.Data[CAKey], secret.Data[PreviousCAKey])
		bundle := bundleCerts(t, res.CABundle)
		require.Len(t, bundle, 2, "new and previous CA")
		require.True(t, bytes.Contains(res.CABundle, secret.Data[CAKey]))
		require.True(t, bytes.Contains(res.CABundle, existing.Data[CAKey]))

		// A second pass within the overlap keeps both CAs and changes nothing.
		again, err := EnsureSecret(ctx, c, testNamespace, testSecret, testServiceRef)
		require.NoError(t, err)
		require.Equal(t, res.CABundle, again.CABundle)
		require.Equal(t, secret.Data, getSecret(t, c).Data)
	})

	t.Run("drops the previous CA once the overlap has passed", func(t *testing.T) {
		data, err := Generate(testServiceRef.Hosts(), time.Now().Add(-time.Hour), inAYear)
		require.NoError(t, err)
		old, err := Generate(testServiceRef.Hosts(), time.Now().Add(-time.Hour), inAYear)
		require.NoError(t, err)
		data[PreviousCAKey] = old[CAKey]
		existing := secretFrom(data)
		existing.Annotations = map[string]string{PreviousCAUntilAnnotation: time.Now().Add(-time.Minute).Format(time.RFC3339)}
		c := newClient(t, existing)

		res, err := EnsureSecret(ctx, c, testNamespace, testSecret, testServiceRef)
		require.NoError(t, err)

		require.Equal(t, data[CAKey], res.CABundle)
		require.Len(t, bundleCerts(t, res.CABundle), 1)
	})

	t.Run("regenerates a cert that is not valid yet", func(t *testing.T) {
		data, err := Generate(testServiceRef.Hosts(), time.Now().Add(24*time.Hour), inAYear)
		require.NoError(t, err)
		c := newClient(t, secretFrom(data))

		res, err := EnsureSecret(ctx, c, testNamespace, testSecret, testServiceRef)
		require.NoError(t, err)

		require.NotEqual(t, data[CertKey], res.Cert)
		require.NoError(t, Check(getSecret(t, c).Data, testServiceRef.Hosts(), time.Now()))
	})

	t.Run("regenerates a cert for another Service", func(t *testing.T) {
		data, err := Generate([]string{"other-service"}, time.Now().Add(-time.Hour), inAYear)
		require.NoError(t, err)
		c := newClient(t, secretFrom(data))

		_, err = EnsureSecret(ctx, c, testNamespace, testSecret, testServiceRef)
		require.NoError(t, err)

		require.NoError(t, Check(getSecret(t, c).Data, testServiceRef.Hosts(), time.Now()))
	})

	t.Run("regenerates garbage data and keeps unrelated keys", func(t *testing.T) {
		c := newClient(t, secretFrom(map[string][]byte{CAKey: []byte("garbage"), "extra": []byte("keep")}))

		res, err := EnsureSecret(ctx, c, testNamespace, testSecret, testServiceRef)
		require.NoError(t, err)

		secret := getSecret(t, c)
		require.NoError(t, Check(secret.Data, testServiceRef.Hosts(), time.Now()))
		require.Equal(t, []byte("keep"), secret.Data["extra"])
		require.Equal(t, secret.Data[CAKey], res.CABundle, "an unparsable old CA is not trusted")
	})

	t.Run("uses the Secret another replica created first", func(t *testing.T) {
		winner, err := Generate(testServiceRef.Hosts(), time.Now().Add(-time.Hour), inAYear)
		require.NoError(t, err)
		raced := false
		base := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
		c := &hookClient{Client: base, create: func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*corev1.Secret); ok && !raced {
				raced = true
				require.NoError(t, base.Create(ctx, secretFrom(winner)))
				return apierrors.NewAlreadyExists(corev1.Resource("secrets"), obj.GetName())
			}
			return base.Create(ctx, obj, opts...)
		}}

		res, err := EnsureSecret(ctx, c, testNamespace, testSecret, testServiceRef)
		require.NoError(t, err)

		require.Equal(t, winner[CAKey], res.CABundle)
		require.Equal(t, winner, getSecret(t, c).Data)
	})
}

func TestGenerateRejectsNoHosts(t *testing.T) {
	_, err := Generate(nil, time.Now(), time.Now().Add(time.Hour))
	require.Error(t, err)
}

func TestCovers(t *testing.T) {
	a, err := Generate([]string{testService}, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	require.NoError(t, err)
	b, err := Generate([]string{testService}, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	require.NoError(t, err)
	both := append(append([]byte{}, a[CAKey]...), b[CAKey]...)

	require.True(t, Covers(a[CAKey], a[CAKey]))
	require.True(t, Covers(both, a[CAKey]), "a bundle covers each of its CAs")
	require.True(t, Covers(both, both))
	require.False(t, Covers(a[CAKey], both), "b is not trusted yet")
	require.False(t, Covers(a[CAKey], b[CAKey]))
	require.True(t, Covers(a[CAKey], nil))
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

func TestWriteFileIfChangedTightensPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tls.key")
	require.NoError(t, os.WriteFile(path, []byte("key"), 0o644))
	// WriteFile applies the umask, so set the wide mode explicitly.
	require.NoError(t, os.Chmod(path, 0o644))

	changed, err := WriteFileIfChanged(path, []byte("key"))
	require.NoError(t, err)
	require.False(t, changed)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "an unchanged file is tightened too")

	require.NoError(t, os.Chmod(path, 0o644))
	changed, err = WriteFileIfChanged(path, []byte("new key"))
	require.NoError(t, err)
	require.True(t, changed)
	info, err = os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
