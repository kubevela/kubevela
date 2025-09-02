/*
Copyright 2022 The KubeVela Authors.

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

package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	certificatesv1 "k8s.io/api/certificates/v1"
	certificatesv1beta1 "k8s.io/api/certificates/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestNewKubeConfigGenerateOptions(t *testing.T) {
	testCases := map[string]struct {
		opts     []KubeConfigGenerateOption
		validate func(*testing.T, *KubeConfigGenerateOptions)
	}{
		"default options": {
			opts: []KubeConfigGenerateOption{},
			validate: func(t *testing.T, opts *KubeConfigGenerateOptions) {
				r := require.New(t)
				r.NotNil(opts.X509)
				r.Nil(opts.ServiceAccount)
				r.Equal(user.Anonymous, opts.X509.User)
				r.Contains(opts.X509.Groups, KubeVelaClientGroup)
			},
		},
		"with user and group options": {
			opts: []KubeConfigGenerateOption{
				KubeConfigWithUserGenerateOption("test-user"),
				KubeConfigWithGroupGenerateOption("test-group"),
			},
			validate: func(t *testing.T, opts *KubeConfigGenerateOptions) {
				r := require.New(t)
				r.Equal("test-user", opts.X509.User)
				r.Contains(opts.X509.Groups, "test-group")
			},
		},
		"with service account option": {
			opts: []KubeConfigGenerateOption{KubeConfigWithServiceAccountGenerateOption(types.NamespacedName{Name: "sa", Namespace: "ns"})},
			validate: func(t *testing.T, opts *KubeConfigGenerateOptions) {
				r := require.New(t)
				r.Nil(opts.X509)
				r.NotNil(opts.ServiceAccount)
				r.Equal("sa", opts.ServiceAccount.ServiceAccountName)
				r.Equal("ns", opts.ServiceAccount.ServiceAccountNamespace)
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			opts := newKubeConfigGenerateOptions(tc.opts...)
			tc.validate(t, opts)
		})
	}
}

func TestGenKubeConfig(t *testing.T) {
	baseCfg := &clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"test-cluster": {Server: "https://localhost:6443"},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"test-context": {Cluster: "test-cluster", AuthInfo: "test-user"},
		},
		CurrentContext: "test-context",
	}
	authInfo := &clientcmdapi.AuthInfo{
		ClientKeyData:         []byte("test-key"),
		ClientCertificateData: []byte("test-cert"),
	}

	testCases := map[string]struct {
		baseCfg   *clientcmdapi.Config
		authInfo  *clientcmdapi.AuthInfo
		caData    []byte
		expectErr bool
	}{
		"valid config": {
			baseCfg:  baseCfg,
			authInfo: authInfo,
			caData:   []byte("ca-data"),
		},
		"no clusters in config": {
			baseCfg:   &clientcmdapi.Config{},
			authInfo:  authInfo,
			expectErr: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			cfg, err := genKubeConfig(tc.baseCfg, tc.authInfo, tc.caData)
			if tc.expectErr {
				r.Error(err)
				r.Contains(err.Error(), "no clusters")
				return
			}
			r.NoError(err)
			r.NotNil(cfg)
			r.Len(cfg.Clusters, 1)
			r.Equal(tc.caData, cfg.Clusters["test-cluster"].CertificateAuthorityData)
		})
	}
}

func TestMakeCertAndKey(t *testing.T) {
	r := require.New(t)
	buf := &bytes.Buffer{}
	csr, key, err := makeCertAndKey(buf, &KubeConfigGenerateX509Options{User: "bob", Groups: []string{"g"}, PrivateKeyBits: 2048})
	r.NoError(err)
	r.NotEmpty(csr)
	r.NotEmpty(key)
	r.Contains(buf.String(), "Private key generated.")
}

func TestMakeCSRName(t *testing.T) {
	r := require.New(t)
	name := makeCSRName("test-user")
	r.Equal("kubevela-csr-test-user", name)
}

func TestGenerateX509KubeConfig(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	minimalCfg := func() *clientcmdapi.Config {
		return &clientcmdapi.Config{
			Clusters:       map[string]*clientcmdapi.Cluster{"c": {Server: "https://example"}},
			Contexts:       map[string]*clientcmdapi.Context{"ctx": {Cluster: "c", AuthInfo: "ai"}},
			CurrentContext: "ctx",
		}
	}

	t.Run("v1", func(t *testing.T) {
		cli := fake.NewSimpleClientset()
		var stored *certificatesv1.CertificateSigningRequest
		cli.Fake.PrependReactor("create", "certificatesigningrequests", func(action ktesting.Action) (bool, runtime.Object, error) {
			create := action.(ktesting.CreateAction)
			obj := create.GetObject().(*certificatesv1.CertificateSigningRequest)
			stored = obj.DeepCopy()
			return true, stored, nil
		})
		cli.Fake.PrependReactor("update", "certificatesigningrequests/approval", func(action ktesting.Action) (bool, runtime.Object, error) {
			return true, stored, nil
		})
		getCount := 0
		cli.Fake.PrependReactor("get", "certificatesigningrequests", func(action ktesting.Action) (bool, runtime.Object, error) {
			getCount++
			obj := stored.DeepCopy()
			if getCount >= 2 {
				obj.Status.Certificate = []byte("CERT")
			}
			return true, obj, nil
		})
		cli.Fake.PrependReactor("delete", "certificatesigningrequests", func(action ktesting.Action) (bool, runtime.Object, error) {
			return true, nil, nil
		})

		buf := &bytes.Buffer{}
		cfg := minimalCfg()
		res, err := generateX509KubeConfigV1(ctx, cli, cfg, buf, &KubeConfigGenerateX509Options{User: "alice", Groups: []string{"g"}, ExpireTime: time.Hour, PrivateKeyBits: 2048})
		r.NoError(err)
		ai := res.AuthInfos[cfg.Contexts[cfg.CurrentContext].AuthInfo]
		r.NotEmpty(ai.ClientCertificateData)
		r.NotEmpty(ai.ClientKeyData)
	})

	t.Run("v1beta1", func(t *testing.T) {
		cli := fake.NewSimpleClientset()
		var stored *certificatesv1beta1.CertificateSigningRequest
		cli.Fake.PrependReactor("create", "certificatesigningrequests", func(action ktesting.Action) (bool, runtime.Object, error) {
			create := action.(ktesting.CreateAction)
			obj := create.GetObject().(*certificatesv1beta1.CertificateSigningRequest)
			stored = obj.DeepCopy()
			return true, stored, nil
		})
		cli.Fake.PrependReactor("update", "certificatesigningrequests", func(action ktesting.Action) (bool, runtime.Object, error) {
			update := action.(ktesting.UpdateAction)
			obj := update.GetObject().(*certificatesv1beta1.CertificateSigningRequest)
			stored = obj.DeepCopy()
			return true, stored, nil
		})
		getCount := 0
		cli.Fake.PrependReactor("get", "certificatesigningrequests", func(action ktesting.Action) (bool, runtime.Object, error) {
			getCount++
			obj := stored.DeepCopy()
			if getCount >= 2 {
				obj.Status.Certificate = []byte("CERT-BETA")
			}
			return true, obj, nil
		})
		cli.Fake.PrependReactor("delete", "certificatesigningrequests", func(action ktesting.Action) (bool, runtime.Object, error) {
			return true, nil, nil
		})

		buf := &bytes.Buffer{}
		cfg := minimalCfg()
		res, err := generateX509KubeConfigV1Beta(ctx, cli, cfg, buf, &KubeConfigGenerateX509Options{User: "alice-beta", Groups: []string{"g"}, ExpireTime: time.Hour, PrivateKeyBits: 2048})
		r.NoError(err)
		ai := res.AuthInfos[cfg.Contexts[cfg.CurrentContext].AuthInfo]
		r.Equal([]byte("CERT-BETA"), ai.ClientCertificateData)
		r.NotEmpty(ai.ClientKeyData)
	})
}

func TestGenerateServiceAccountKubeConfig(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	minimalCfg := func() *clientcmdapi.Config {
		return &clientcmdapi.Config{
			Clusters:       map[string]*clientcmdapi.Cluster{"c": {Server: "https://example"}},
			Contexts:       map[string]*clientcmdapi.Context{"ctx": {Cluster: "c", AuthInfo: "ai"}},
			CurrentContext: "ctx",
		}
	}

	t.Run("with secret", func(t *testing.T) {
		sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "sa", Namespace: "ns"}, Secrets: []corev1.ObjectReference{{Name: "s"}}}
		sec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "ns"}, Data: map[string][]byte{"token": []byte("tok"), "ca.crt": []byte("CA")}}
		cli := fake.NewSimpleClientset(sa, sec)

		buf := &bytes.Buffer{}
		cfg := minimalCfg()
		got, err := generateServiceAccountKubeConfig(ctx, cli, cfg, buf, &KubeConfigGenerateServiceAccountOptions{ServiceAccountName: "sa", ServiceAccountNamespace: "ns", ExpireTime: time.Hour})
		r.NoError(err)
		ai := got.AuthInfos[cfg.Contexts[cfg.CurrentContext].AuthInfo]
		r.Equal("tok", ai.Token)
		cl := got.Clusters[cfg.Contexts[cfg.CurrentContext].Cluster]
		r.Equal("CA", string(cl.CertificateAuthorityData))
	})

	t.Run("with token request", func(t *testing.T) {
		sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "sa", Namespace: "ns"}}
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "kube-root-ca.crt", Namespace: "ns"}, Data: map[string]string{"ca.crt": "CA"}}
		cli := fake.NewSimpleClientset(sa, cm)

		cli.Fake.PrependReactor("create", "serviceaccounts/token", func(action ktesting.Action) (bool, runtime.Object, error) {
			obj := &authenticationv1.TokenRequest{
				Status: authenticationv1.TokenRequestStatus{Token: "rtok"},
			}
			return true, obj, nil
		})

		buf := &bytes.Buffer{}
		cfg := minimalCfg()
		got, err := generateServiceAccountKubeConfig(ctx, cli, cfg, buf, &KubeConfigGenerateServiceAccountOptions{ServiceAccountName: "sa", ServiceAccountNamespace: "ns", ExpireTime: time.Hour})
		r.NoError(err)
		ai := got.AuthInfos[cfg.Contexts[cfg.CurrentContext].AuthInfo]
		r.Equal("rtok", ai.Token)
		cl := got.Clusters[cfg.Contexts[cfg.CurrentContext].Cluster]
		r.Equal("CA", string(cl.CertificateAuthorityData))
	})
}

func TestReadIdentityFromKubeConfig(t *testing.T) {
	r := require.New(t)
	dir := t.TempDir()

	t.Run("from certificate", func(t *testing.T) {
		priv, err := rsa.GenerateKey(rand.Reader, 2048)
		r.NoError(err)
		tmpl := &x509.Certificate{SerialNumber: new(big.Int).SetInt64(1), Subject: pkix.Name{CommonName: "alice", Organization: []string{"g1", "g2"}}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
		certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
		r.NoError(err)
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

		kcfg := &clientcmdapi.Config{
			Clusters:       map[string]*clientcmdapi.Cluster{"c": {Server: "https://example"}},
			Contexts:       map[string]*clientcmdapi.Context{"ctx": {Cluster: "c", AuthInfo: "ai"}},
			CurrentContext: "ctx",
			AuthInfos:      map[string]*clientcmdapi.AuthInfo{"ai": {ClientCertificateData: certPEM}},
		}
		path := filepath.Join(dir, "kubeconfig-cert")
		err = clientcmd.WriteToFile(*kcfg, path)
		r.NoError(err)

		id, err := ReadIdentityFromKubeConfig(path)
		r.NoError(err)
		r.Equal("alice", id.User)
		r.Equal([]string{"g1", "g2"}, id.Groups)
	})

	t.Run("no auth returns error", func(t *testing.T) {
		kcfg := &clientcmdapi.Config{
			Clusters:       map[string]*clientcmdapi.Cluster{"c": {Server: "https://example"}},
			Contexts:       map[string]*clientcmdapi.Context{"ctx": {Cluster: "c", AuthInfo: "ai"}},
			CurrentContext: "ctx",
			AuthInfos:      map[string]*clientcmdapi.AuthInfo{"ai": {}},
		}
		path := filepath.Join(dir, "kubeconfig-empty")
		err := clientcmd.WriteToFile(*kcfg, path)
		r.NoError(err)

		_, err = ReadIdentityFromKubeConfig(path)
		r.Error(err)
		r.Contains(err.Error(), "cannot find client certificate or serviceaccount token in kubeconfig")
	})
}
