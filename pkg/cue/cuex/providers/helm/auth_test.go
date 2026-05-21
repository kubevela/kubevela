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
	"encoding/json"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kubevela/pkg/util/singleton"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("auth.go scaffolding", func() {
	It("defines the source-type constants", func() {
		Expect(sourceTypeOCI).To(Equal("oci"))
		Expect(sourceTypeURL).To(Equal("url"))
		Expect(sourceTypeRepo).To(Equal("repo"))
	})

	It("constructs secretRef and authResolveOptions zero-values without panic", func() {
		ref := secretRef{Name: "n", Namespace: "ns"}
		Expect(ref.Name).To(Equal("n"))
		opts := authResolveOptions{AppNamespace: "a", ReleaseNamespace: "r", RegistryHost: "h", SourceScheme: "https", SourceType: sourceTypeOCI}
		Expect(opts.SourceType).To(Equal("oci"))
	})
})

var _ = Describe("extractRegistryHost", func() {
	DescribeTable("derives the registry host from chart source / repoURL",
		func(source, repoURL, want string) {
			Expect(extractRegistryHost(source, repoURL)).To(Equal(want))
		},
		Entry("OCI with path", "oci://ghcr.io/stefanprodan/charts/podinfo", "", "ghcr.io"),
		Entry("OCI with port", "oci://registry.example.com:5000/charts/x", "", "registry.example.com:5000"),
		Entry("URL https", "https://example.com/charts/x-1.0.0.tgz", "", "example.com"),
		Entry("URL http with port", "http://10.0.0.1:8080/x.tgz", "", "10.0.0.1:8080"),
		Entry("Repo source uses repoURL", "podinfo", "https://stefanprodan.github.io/podinfo", "stefanprodan.github.io"),
		Entry("Repo with port", "podinfo", "https://repo.example.com:8443/charts", "repo.example.com:8443"),
		Entry("Empty inputs", "", "", ""),
	)
})

var _ = Describe("resolveAuthSecretNamespace", func() {
	It("defaults empty namespace to release namespace", func() {
		ns, err := resolveAuthSecretNamespace(secretRef{Name: "s"}, authResolveOptions{AppNamespace: "app-ns", ReleaseNamespace: "rel-ns"})
		Expect(err).NotTo(HaveOccurred())
		Expect(ns).To(Equal("rel-ns"))
	})

	It("accepts an explicit release namespace", func() {
		ns, err := resolveAuthSecretNamespace(secretRef{Name: "s", Namespace: "rel-ns"}, authResolveOptions{AppNamespace: "app-ns", ReleaseNamespace: "rel-ns"})
		Expect(err).NotTo(HaveOccurred())
		Expect(ns).To(Equal("rel-ns"))
	})

	It("accepts an explicit Application namespace", func() {
		ns, err := resolveAuthSecretNamespace(secretRef{Name: "s", Namespace: "app-ns"}, authResolveOptions{AppNamespace: "app-ns", ReleaseNamespace: "rel-ns"})
		Expect(err).NotTo(HaveOccurred())
		Expect(ns).To(Equal("app-ns"))
	})

	It("rejects a foreign namespace with the RFC-grounded message", func() {
		_, err := resolveAuthSecretNamespace(secretRef{Name: "s", Namespace: "other"}, authResolveOptions{AppNamespace: "app-ns", ReleaseNamespace: "rel-ns"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`"other/s"`))
		Expect(err.Error()).To(ContainSubstring("MUST equal the release namespace"))
		Expect(err.Error()).To(ContainSubstring("rel-ns"))
		Expect(err.Error()).To(ContainSubstring("app-ns"))
	})
})

var _ = Describe("validateBasicCredentials", func() {
	It("accepts a plain user/pass", func() {
		Expect(validateBasicCredentials("alice", "wonderland", "ns", "s")).To(Succeed())
	})

	It("rejects a username containing ':'", func() {
		err := validateBasicCredentials("al:ice", "p", "ns", "s")
		Expect(err).To(MatchError(ContainSubstring(`username MUST NOT contain ':' (RFC 7617 §2)`)))
		Expect(err).To(MatchError(ContainSubstring(`"ns/s"`)))
	})

	It("rejects a username containing a control character", func() {
		err := validateBasicCredentials("a\x01b", "p", "ns", "s")
		Expect(err).To(MatchError(ContainSubstring(`credentials MUST NOT contain control characters (RFC 7617 §2)`)))
	})

	It("rejects a password containing a control character", func() {
		err := validateBasicCredentials("u", "p\x7f", "ns", "s")
		Expect(err).To(MatchError(ContainSubstring(`credentials MUST NOT contain control characters (RFC 7617 §2)`)))
	})
})

var _ = Describe("validateBearerToken", func() {
	DescribeTable("accepts tokens in the b64token charset",
		func(token string) {
			Expect(validateBearerToken(token, "ns", "s")).To(Succeed())
		},
		Entry("plain", "abcDEF123"),
		Entry("with special chars", "a-b._c~d+e/f=="),
		Entry("JWT shape", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.abc"),
	)

	DescribeTable("rejects tokens with chars outside the b64token charset",
		func(token string) {
			err := validateBearerToken(token, "ns", "s")
			Expect(err).To(MatchError(ContainSubstring(`b64token charset (RFC 6750 §2.1)`)))
		},
		Entry("space", "abc def"),
		Entry("null", "abc\x00def"),
		Entry("non-ASCII", "abcö"),
	)
})

var _ = Describe("validateBearerTransport", func() {
	It("permits Bearer over HTTPS without insecureSkipTLS", func() {
		Expect(validateBearerTransport("https", false, "https://r.example.com/x", "ns", "s")).To(Succeed())
	})

	It("rejects Bearer over plain HTTP", func() {
		err := validateBearerTransport("http", false, "http://r.example.com/x", "ns", "s")
		Expect(err).To(MatchError(ContainSubstring(`bearer tokens MUST be sent only over HTTPS or OCI (RFC 6750 §2)`)))
		Expect(err).To(MatchError(ContainSubstring(`"ns/s"`)))
	})

	It("rejects Bearer with insecureSkipTLS=true", func() {
		err := validateBearerTransport("https", true, "https://r.example.com/x", "ns", "s")
		Expect(err).To(MatchError(ContainSubstring(`bearer tokens MUST NOT be sent with TLS verification disabled (RFC 6750 §2)`)))
	})
})

var _ = Describe("dispatchBasicAuthSecret", func() {
	It("populates HTTPOption.Username/Password from a basic-auth Secret", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "ns"},
			Type:       corev1.SecretTypeBasicAuth,
			Data: map[string][]byte{
				corev1.BasicAuthUsernameKey: []byte("alice"),
				corev1.BasicAuthPasswordKey: []byte("wonderland"),
			},
		}
		opts, err := dispatchBasicAuthSecret(s, authResolveOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts).To(Equal(&common.HTTPOption{Username: "alice", Password: "wonderland"}))
	})

	It("rejects a basic-auth Secret missing the username key", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "ns"},
			Type:       corev1.SecretTypeBasicAuth,
			Data:       map[string][]byte{corev1.BasicAuthPasswordKey: []byte("p")},
		}
		_, err := dispatchBasicAuthSecret(s, authResolveOptions{})
		Expect(err).To(MatchError(ContainSubstring(`MUST contain key "username"`)))
	})

	It("rejects a basic-auth Secret missing the password key", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "ns"},
			Type:       corev1.SecretTypeBasicAuth,
			Data:       map[string][]byte{corev1.BasicAuthUsernameKey: []byte("u")},
		}
		_, err := dispatchBasicAuthSecret(s, authResolveOptions{})
		Expect(err).To(MatchError(ContainSubstring(`MUST contain key "password"`)))
	})

	It("rejects a basic-auth Secret whose username contains ':'", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "ns"},
			Type:       corev1.SecretTypeBasicAuth,
			Data: map[string][]byte{
				corev1.BasicAuthUsernameKey: []byte("a:b"),
				corev1.BasicAuthPasswordKey: []byte("p"),
			},
		}
		_, err := dispatchBasicAuthSecret(s, authResolveOptions{})
		Expect(err).To(MatchError(ContainSubstring(`RFC 7617 §2`)))
	})
})

var _ = Describe("dispatchDockerConfigJSONSecret", func() {
	validCfg := []byte(`{"auths":{"ghcr.io":{"username":"alice","password":"wonderland","auth":"YWxpY2U6d29uZGVybGFuZA=="}}}`)

	It("populates HTTPOption from the matching host entry", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "dh", Namespace: "ns"},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data:       map[string][]byte{corev1.DockerConfigJsonKey: validCfg},
		}
		opts, raw, err := dispatchDockerConfigJSONSecret(s, authResolveOptions{RegistryHost: "ghcr.io", SourceType: sourceTypeURL})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts).To(Equal(&common.HTTPOption{Username: "alice", Password: "wonderland"}))
		Expect(raw).To(BeNil(), "raw bytes returned only when SourceType=oci")
	})

	It("returns raw bytes for OCI sources alongside the parsed creds", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "dh", Namespace: "ns"},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data:       map[string][]byte{corev1.DockerConfigJsonKey: validCfg},
		}
		opts, raw, err := dispatchDockerConfigJSONSecret(s, authResolveOptions{RegistryHost: "ghcr.io", SourceType: sourceTypeOCI})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.Username).To(Equal("alice"))
		Expect(raw).To(Equal(validCfg))
	})

	It("decodes the `auth` field when username/password are absent (docker login style)", func() {
		// `docker login` writes only the base64 "username:password" auth field.
		authOnly := []byte(`{"auths":{"ghcr.io":{"auth":"YWxpY2U6d29uZGVybGFuZA=="}}}`)
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "dh", Namespace: "ns"},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data:       map[string][]byte{corev1.DockerConfigJsonKey: authOnly},
		}
		opts, _, err := dispatchDockerConfigJSONSecret(s, authResolveOptions{RegistryHost: "ghcr.io"})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.Username).To(Equal("alice"))
		Expect(opts.Password).To(Equal("wonderland"))
	})

	It("rejects an `auth` field that is not valid base64", func() {
		bad := []byte(`{"auths":{"ghcr.io":{"auth":"not!base64!"}}}`)
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "dh", Namespace: "ns"},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data:       map[string][]byte{corev1.DockerConfigJsonKey: bad},
		}
		_, _, err := dispatchDockerConfigJSONSecret(s, authResolveOptions{RegistryHost: "ghcr.io"})
		Expect(err).To(MatchError(ContainSubstring(`not valid base64`)))
	})

	It("rejects an `auth` field that decodes without a colon", func() {
		// base64("nocolonhere")
		bad := []byte(`{"auths":{"ghcr.io":{"auth":"bm9jb2xvbmhlcmU="}}}`)
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "dh", Namespace: "ns"},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data:       map[string][]byte{corev1.DockerConfigJsonKey: bad},
		}
		_, _, err := dispatchDockerConfigJSONSecret(s, authResolveOptions{RegistryHost: "ghcr.io"})
		Expect(err).To(MatchError(ContainSubstring(`MUST decode to "username:password"`)))
	})

	It("rejects when the .dockerconfigjson key is absent", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "dh", Namespace: "ns"},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data:       map[string][]byte{"wrong-key": validCfg},
		}
		_, _, err := dispatchDockerConfigJSONSecret(s, authResolveOptions{RegistryHost: "ghcr.io"})
		Expect(err).To(MatchError(ContainSubstring(`MUST contain key ".dockerconfigjson"`)))
	})

	It("rejects malformed JSON", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "dh", Namespace: "ns"},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data:       map[string][]byte{corev1.DockerConfigJsonKey: []byte("{not json")},
		}
		_, _, err := dispatchDockerConfigJSONSecret(s, authResolveOptions{RegistryHost: "ghcr.io"})
		Expect(err).To(MatchError(ContainSubstring(`MUST contain a valid Docker configuration`)))
	})

	It("rejects when no entry matches the registry host", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "dh", Namespace: "ns"},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data:       map[string][]byte{corev1.DockerConfigJsonKey: validCfg},
		}
		_, _, err := dispatchDockerConfigJSONSecret(s, authResolveOptions{RegistryHost: "other.example.com"})
		Expect(err).To(MatchError(ContainSubstring(`no entry matching registry host "other.example.com"`)))
	})
})

var _ = Describe("dispatchTLSSecret", func() {
	crt := []byte("-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----")
	key := []byte("-----BEGIN PRIVATE KEY-----\nMIIE...\n-----END PRIVATE KEY-----")
	ca := []byte("-----BEGIN CERTIFICATE-----\nCAcert\n-----END CERTIFICATE-----")

	It("populates CertFile/KeyFile (PEM strings) without ca.crt", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "ns"},
			Type:       corev1.SecretTypeTLS,
			Data:       map[string][]byte{corev1.TLSCertKey: crt, corev1.TLSPrivateKeyKey: key},
		}
		opts, err := dispatchTLSSecret(s, authResolveOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.CertFile).To(Equal(string(crt)))
		Expect(opts.KeyFile).To(Equal(string(key)))
		Expect(opts.CaFile).To(BeEmpty())
	})

	It("populates CaFile when ca.crt is set", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "ns"},
			Type:       corev1.SecretTypeTLS,
			Data:       map[string][]byte{corev1.TLSCertKey: crt, corev1.TLSPrivateKeyKey: key, "ca.crt": ca},
		}
		opts, err := dispatchTLSSecret(s, authResolveOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.CaFile).To(Equal(string(ca)))
	})

	It("rejects when tls.crt is absent", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "ns"},
			Type:       corev1.SecretTypeTLS,
			Data:       map[string][]byte{corev1.TLSPrivateKeyKey: key},
		}
		_, err := dispatchTLSSecret(s, authResolveOptions{})
		Expect(err).To(MatchError(ContainSubstring(`MUST contain key "tls.crt"`)))
	})

	It("rejects when tls.key is absent", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "ns"},
			Type:       corev1.SecretTypeTLS,
			Data:       map[string][]byte{corev1.TLSCertKey: crt},
		}
		_, err := dispatchTLSSecret(s, authResolveOptions{})
		Expect(err).To(MatchError(ContainSubstring(`MUST contain key "tls.key"`)))
	})
})

var _ = Describe("dispatchOpaqueSecret", func() {
	It("detects username+password keys", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "o", Namespace: "ns"},
			Data:       map[string][]byte{"username": []byte("alice"), "password": []byte("wonderland")},
		}
		opts, err := dispatchOpaqueSecret(s, authResolveOptions{SourceScheme: "https"})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts).To(Equal(&common.HTTPOption{Username: "alice", Password: "wonderland"}))
	})

	It("detects token key over HTTPS without insecureSkipTLS", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "o", Namespace: "ns"},
			Data:       map[string][]byte{"token": []byte("abcDEF123")},
		}
		opts, err := dispatchOpaqueSecret(s, authResolveOptions{SourceScheme: "https"})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts).To(Equal(&common.HTTPOption{BearerToken: "abcDEF123"}))
	})

	It("rejects when both username/password and token are set", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "o", Namespace: "ns"},
			Data:       map[string][]byte{"username": []byte("u"), "password": []byte("p"), "token": []byte("t")},
		}
		_, err := dispatchOpaqueSecret(s, authResolveOptions{SourceScheme: "https"})
		Expect(err).To(MatchError(ContainSubstring(`at most one credential method MUST be configured per Secret (RFC 6750 §2)`)))
	})

	It("supports TLS-only Opaque Secrets", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "o", Namespace: "ns"},
			Data: map[string][]byte{
				"certFile": []byte("CRT"),
				"keyFile":  []byte("KEY"),
				"caFile":   []byte("CA"),
			},
		}
		opts, err := dispatchOpaqueSecret(s, authResolveOptions{SourceScheme: "https"})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts).To(Equal(&common.HTTPOption{CertFile: "CRT", KeyFile: "KEY", CaFile: "CA"}))
	})

	It("honors insecureSkipTLS Opaque key", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "o", Namespace: "ns"},
			Data: map[string][]byte{
				"username":        []byte("u"),
				"password":        []byte("p"),
				"insecureSkipTLS": []byte("true"),
			},
		}
		opts, err := dispatchOpaqueSecret(s, authResolveOptions{SourceScheme: "https"})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.InsecureSkipTLS).To(BeTrue())
	})

	It("honors insecurePlainHTTP Opaque key for OCI plain-HTTP", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "o", Namespace: "ns"},
			Data: map[string][]byte{
				"username":          []byte("u"),
				"password":          []byte("p"),
				"insecurePlainHTTP": []byte("true"),
			},
		}
		opts, err := dispatchOpaqueSecret(s, authResolveOptions{SourceScheme: "oci"})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.PlainHTTP).To(BeTrue())
		Expect(opts.Username).To(Equal("u"))
		Expect(opts.Password).To(Equal("p"))
	})

	It("leaves PlainHTTP unset when insecurePlainHTTP is absent", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "o", Namespace: "ns"},
			Data: map[string][]byte{
				"username": []byte("u"),
				"password": []byte("p"),
			},
		}
		opts, err := dispatchOpaqueSecret(s, authResolveOptions{SourceScheme: "oci"})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.PlainHTTP).To(BeFalse())
	})
})

var _ = Describe("resolveAuthOptions", func() {
	var scheme *runtime.Scheme
	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
	})

	It("returns (nil, nil, nil) when ref is nil", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		opts, raw, err := resolveAuthOptions(context.Background(), c, nil, authResolveOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts).To(BeNil())
		Expect(raw).To(BeNil())
	})

	It("returns the not-found error verbatim when the Secret is missing", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		_, _, err := resolveAuthOptions(context.Background(), c, &secretRef{Name: "missing"}, authResolveOptions{AppNamespace: "app-ns", ReleaseNamespace: "rel-ns"})
		Expect(err).To(MatchError(ContainSubstring(`not found: it MUST exist in the release namespace "rel-ns" or the Application namespace "app-ns"`)))
	})

	It("rejects unsupported Secret types", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "rel-ns"},
			Type:       corev1.SecretTypeServiceAccountToken,
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(s).Build()
		_, _, err := resolveAuthOptions(context.Background(), c, &secretRef{Name: "x"}, authResolveOptions{ReleaseNamespace: "rel-ns", AppNamespace: "app-ns"})
		Expect(err).To(MatchError(ContainSubstring(`has unsupported type`)))
		Expect(err).To(MatchError(ContainSubstring(`MUST be one of kubernetes.io/basic-auth, kubernetes.io/dockerconfigjson, kubernetes.io/tls, or Opaque`)))
	})

	It("dispatches kubernetes.io/basic-auth", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "rel-ns"},
			Type:       corev1.SecretTypeBasicAuth,
			Data: map[string][]byte{
				corev1.BasicAuthUsernameKey: []byte("alice"),
				corev1.BasicAuthPasswordKey: []byte("wonderland"),
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(s).Build()
		opts, _, err := resolveAuthOptions(context.Background(), c, &secretRef{Name: "b"}, authResolveOptions{ReleaseNamespace: "rel-ns", AppNamespace: "app-ns", SourceScheme: "https"})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.Username).To(Equal("alice"))
		Expect(opts.Password).To(Equal("wonderland"))
	})

	It("rejects a foreign namespace before reading the Secret", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		_, _, err := resolveAuthOptions(context.Background(), c, &secretRef{Name: "x", Namespace: "other"}, authResolveOptions{AppNamespace: "app-ns", ReleaseNamespace: "rel-ns"})
		Expect(err).To(MatchError(ContainSubstring(`namespace MUST equal the release namespace "rel-ns" or the Application namespace "app-ns"`)))
	})

	It("rejects user-supplied bearer on OCI sources", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "rel-ns"},
			Data:       map[string][]byte{"token": []byte("abc")},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(s).Build()
		_, _, err := resolveAuthOptions(context.Background(), c, &secretRef{Name: "t"}, authResolveOptions{ReleaseNamespace: "rel-ns", AppNamespace: "app-ns", SourceScheme: "oci", SourceType: sourceTypeOCI})
		Expect(err).To(MatchError(ContainSubstring(`user-supplied bearer tokens MUST NOT be used with OCI sources`)))
	})

	It("rejects bearer over plain http", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "rel-ns"},
			Data:       map[string][]byte{"token": []byte("abc")},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(s).Build()
		_, _, err := resolveAuthOptions(context.Background(), c, &secretRef{Name: "t"}, authResolveOptions{ReleaseNamespace: "rel-ns", AppNamespace: "app-ns", SourceScheme: "http", SourceType: sourceTypeURL})
		Expect(err).To(MatchError(ContainSubstring(`bearer tokens MUST be sent only over HTTPS or OCI (RFC 6750 §2)`)))
	})
})

var _ = Describe("writeOCIRegistryConfigFile", func() {
	It("synthesizes a one-entry config from username/password", func() {
		path, cleanup, err := writeOCIRegistryConfigFile(&common.HTTPOption{Username: "alice", Password: "wonderland"}, nil, "ghcr.io")
		Expect(err).NotTo(HaveOccurred())
		defer cleanup()
		b, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		var parsed dockerConfigJSON
		Expect(json.Unmarshal(b, &parsed)).To(Succeed())
		Expect(parsed.Auths).To(HaveKey("ghcr.io"))
		Expect(parsed.Auths["ghcr.io"].Username).To(Equal("alice"))
	})

	// Docker Hub stores credentials under the canonical
	// "https://index.docker.io/v1/" key, not under the "registry-1.docker.io"
	// pull host. The Helm/ORAS auth resolver follows that convention, so the
	// synthesized config MUST register the entry under BOTH keys when the host
	// is one of the Docker Hub OCI endpoints; otherwise the lookup falls
	// through to anonymous and the registry returns insufficient_scope.
	It("registers Docker Hub creds under both registry-1.docker.io and index.docker.io/v1/", func() {
		path, cleanup, err := writeOCIRegistryConfigFile(
			&common.HTTPOption{Username: "dh-user", Password: "dckr_pat_fake"}, nil, "registry-1.docker.io")
		Expect(err).NotTo(HaveOccurred())
		defer cleanup()
		b, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		var parsed dockerConfigJSON
		Expect(json.Unmarshal(b, &parsed)).To(Succeed())
		Expect(parsed.Auths).To(HaveKey("registry-1.docker.io"))
		Expect(parsed.Auths).To(HaveKey("https://index.docker.io/v1/"))
		Expect(parsed.Auths["registry-1.docker.io"].Username).To(Equal("dh-user"))
		Expect(parsed.Auths["https://index.docker.io/v1/"].Username).To(Equal("dh-user"))
		Expect(parsed.Auths["registry-1.docker.io"].Auth).
			To(Equal(parsed.Auths["https://index.docker.io/v1/"].Auth))
	})

	It("registers Docker Hub creds under both keys when host is index.docker.io", func() {
		path, cleanup, err := writeOCIRegistryConfigFile(
			&common.HTTPOption{Username: "dh-user", Password: "dckr_pat_fake"}, nil, "index.docker.io")
		Expect(err).NotTo(HaveOccurred())
		defer cleanup()
		b, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		var parsed dockerConfigJSON
		Expect(json.Unmarshal(b, &parsed)).To(Succeed())
		Expect(parsed.Auths).To(HaveKey("index.docker.io"))
		Expect(parsed.Auths).To(HaveKey("https://index.docker.io/v1/"))
	})

	It("registers Docker Hub creds under both keys when host is the short docker.io alias", func() {
		// oras-go's resolveHostname normalises "docker.io" to the v1 index too,
		// so charts written as oci://docker.io/library/... must also get the
		// canonical key, not just the explicit pull hosts.
		path, cleanup, err := writeOCIRegistryConfigFile(
			&common.HTTPOption{Username: "dh-user", Password: "dckr_pat_fake"}, nil, "docker.io")
		Expect(err).NotTo(HaveOccurred())
		defer cleanup()
		b, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		var parsed dockerConfigJSON
		Expect(json.Unmarshal(b, &parsed)).To(Succeed())
		Expect(parsed.Auths).To(HaveKey("docker.io"))
		Expect(parsed.Auths).To(HaveKey("https://index.docker.io/v1/"))
	})

	It("does NOT add the Docker Hub alias for non-Docker-Hub hosts", func() {
		path, cleanup, err := writeOCIRegistryConfigFile(
			&common.HTTPOption{Username: "u", Password: "p"}, nil, "ghcr.io")
		Expect(err).NotTo(HaveOccurred())
		defer cleanup()
		b, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		var parsed dockerConfigJSON
		Expect(json.Unmarshal(b, &parsed)).To(Succeed())
		Expect(parsed.Auths).To(HaveKey("ghcr.io"))
		Expect(parsed.Auths).NotTo(HaveKey("https://index.docker.io/v1/"))
		Expect(parsed.Auths).To(HaveLen(1))
	})

	It("writes raw .dockerconfigjson bytes verbatim", func() {
		raw := []byte(`{"auths":{"ghcr.io":{"username":"x","password":"y"},"other.io":{"username":"a","password":"b"}}}`)
		path, cleanup, err := writeOCIRegistryConfigFile(nil, raw, "ghcr.io")
		Expect(err).NotTo(HaveOccurred())
		defer cleanup()
		b, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(b).To(Equal(raw))
	})

	It("cleanup removes the file", func() {
		path, cleanup, err := writeOCIRegistryConfigFile(&common.HTTPOption{Username: "u", Password: "p"}, nil, "h")
		Expect(err).NotTo(HaveOccurred())
		cleanup()
		_, err = os.Stat(path)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

var _ = Describe("computeAuthCacheTag", func() {
	var (
		scheme         *runtime.Scheme
		origKubeClient client.Client
	)
	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		origKubeClient = singleton.KubeClient.Get()
	})
	AfterEach(func() {
		singleton.KubeClient.Set(origKubeClient)
	})

	It("returns empty tag when no auth.secretRef is declared", func() {
		singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).Build())
		tag, err := computeAuthCacheTag(context.Background(),
			&ChartSourceParams{Source: "https://example.com/x.tgz"},
			"app-ns", "rel-ns")
		Expect(err).NotTo(HaveOccurred())
		Expect(tag).To(Equal(""))
	})

	It("returns a stable hex tag for a given Secret content", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "rel-ns"},
			Type:       corev1.SecretTypeBasicAuth,
			Data:       map[string][]byte{"username": []byte("u"), "password": []byte("p")},
		}
		singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(s).Build())
		params := &ChartSourceParams{
			Source: "oci://r.example.com/charts/c",
			Auth:   &AuthParams{SecretRef: &SecretRefParams{Name: "creds"}},
		}
		tag1, err := computeAuthCacheTag(context.Background(), params, "app-ns", "rel-ns")
		Expect(err).NotTo(HaveOccurred())
		Expect(tag1).To(HaveLen(16))
		tag2, err := computeAuthCacheTag(context.Background(), params, "app-ns", "rel-ns")
		Expect(err).NotTo(HaveOccurred())
		Expect(tag2).To(Equal(tag1))
	})

	It("produces a different tag when the Secret data changes", func() {
		mkSecret := func(pass string) *corev1.Secret {
			return &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "rel-ns"},
				Type:       corev1.SecretTypeBasicAuth,
				Data:       map[string][]byte{"username": []byte("u"), "password": []byte(pass)},
			}
		}
		params := &ChartSourceParams{
			Source: "oci://r.example.com/charts/c",
			Auth:   &AuthParams{SecretRef: &SecretRefParams{Name: "creds"}},
		}
		singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(mkSecret("p1")).Build())
		tag1, err := computeAuthCacheTag(context.Background(), params, "app-ns", "rel-ns")
		Expect(err).NotTo(HaveOccurred())
		singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(mkSecret("p2")).Build())
		tag2, err := computeAuthCacheTag(context.Background(), params, "app-ns", "rel-ns")
		Expect(err).NotTo(HaveOccurred())
		Expect(tag2).NotTo(Equal(tag1))
	})

	It("produces a different tag when the Secret Type changes", func() {
		mkSecret := func(t corev1.SecretType) *corev1.Secret {
			return &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "rel-ns"},
				Type:       t,
				Data:       map[string][]byte{"username": []byte("u"), "password": []byte("p")},
			}
		}
		params := &ChartSourceParams{
			Source: "oci://r.example.com/charts/c",
			Auth:   &AuthParams{SecretRef: &SecretRefParams{Name: "creds"}},
		}
		singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(mkSecret(corev1.SecretTypeBasicAuth)).Build())
		tag1, _ := computeAuthCacheTag(context.Background(), params, "app-ns", "rel-ns")
		singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(mkSecret(corev1.SecretTypeOpaque)).Build())
		tag2, _ := computeAuthCacheTag(context.Background(), params, "app-ns", "rel-ns")
		Expect(tag2).NotTo(Equal(tag1))
	})

	It("returns a clear not-found error when the Secret is missing", func() {
		singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).Build())
		params := &ChartSourceParams{
			Source: "oci://r.example.com/charts/c",
			Auth:   &AuthParams{SecretRef: &SecretRefParams{Name: "missing"}},
		}
		_, err := computeAuthCacheTag(context.Background(), params, "app-ns", "rel-ns")
		Expect(err).To(MatchError(ContainSubstring(`not found: it MUST exist in the release namespace`)))
	})

	It("rejects cross-namespace secret references", func() {
		singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).Build())
		params := &ChartSourceParams{
			Source: "oci://r.example.com/charts/c",
			Auth: &AuthParams{SecretRef: &SecretRefParams{
				Name: "creds", Namespace: "kube-system",
			}},
		}
		_, err := computeAuthCacheTag(context.Background(), params, "app-ns", "rel-ns")
		Expect(err).To(MatchError(ContainSubstring(`MUST equal the release namespace`)))
	})
})

var _ = Describe("normalizeDockerHubAliases", func() {
	canonical := "https://index.docker.io/v1/"

	parse := func(b []byte) map[string]interface{} {
		var m map[string]interface{}
		Expect(json.Unmarshal(b, &m)).To(Succeed())
		return m
	}

	It("copies an existing registry-1.docker.io entry under the canonical v1 key", func() {
		in := []byte(`{"auths":{"registry-1.docker.io":{"username":"u","password":"p","auth":"dXA="}}}`)
		out := normalizeDockerHubAliases(in, "registry-1.docker.io")
		auths := parse(out)["auths"].(map[string]interface{})
		Expect(auths).To(HaveKey(canonical))
		Expect(auths).To(HaveKey("registry-1.docker.io"))
	})

	It("copies an existing index.docker.io entry under the canonical v1 key", func() {
		in := []byte(`{"auths":{"index.docker.io":{"username":"u","password":"p","auth":"dXA="}}}`)
		out := normalizeDockerHubAliases(in, "registry-1.docker.io")
		auths := parse(out)["auths"].(map[string]interface{})
		Expect(auths).To(HaveKey(canonical))
	})

	It("copies an existing docker.io entry under the canonical v1 key", func() {
		in := []byte(`{"auths":{"docker.io":{"auth":"dXA="}}}`)
		out := normalizeDockerHubAliases(in, "registry-1.docker.io")
		auths := parse(out)["auths"].(map[string]interface{})
		Expect(auths).To(HaveKey(canonical))
	})

	It("is a no-op when the canonical v1 key is already present", func() {
		in := []byte(`{"auths":{"https://index.docker.io/v1/":{"auth":"dXA="}}}`)
		out := normalizeDockerHubAliases(in, "registry-1.docker.io")
		Expect(string(out)).To(Equal(string(in)))
	})

	It("is a no-op when the host is not Docker Hub", func() {
		in := []byte(`{"auths":{"ghcr.io":{"auth":"dXA="}}}`)
		out := normalizeDockerHubAliases(in, "ghcr.io")
		auths := parse(out)["auths"].(map[string]interface{})
		Expect(auths).NotTo(HaveKey(canonical))
		Expect(auths).To(HaveKey("ghcr.io"))
	})

	It("returns the original bytes on malformed JSON", func() {
		in := []byte(`{not-json`)
		out := normalizeDockerHubAliases(in, "registry-1.docker.io")
		Expect(string(out)).To(Equal(string(in)))
	})

	It("returns the original bytes when the auths field is absent", func() {
		in := []byte(`{"other":"value"}`)
		out := normalizeDockerHubAliases(in, "registry-1.docker.io")
		Expect(string(out)).To(Equal(string(in)))
	})
})

var _ = Describe("resolveHTTPOptions", func() {
	var (
		scheme         *runtime.Scheme
		origKubeClient client.Client
	)
	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		// Capture the package-global KubeClient so the per-spec
		// singleton.KubeClient.Set() calls below cannot leak fake
		// clients into later tests in this package.
		origKubeClient = singleton.KubeClient.Get()
	})
	AfterEach(func() {
		singleton.KubeClient.Set(origKubeClient)
	})

	It("returns (nil, nil, nil) when params.Auth is nil", func() {
		singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).Build())
		params := &ChartSourceParams{Source: "https://example.com/x.tgz"}
		opts, raw, err := resolveHTTPOptions(context.Background(), params, "app-ns", "rel-ns", sourceTypeURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(opts).To(BeNil())
		Expect(raw).To(BeNil())
	})

	It("threads source scheme into the resolver (URL https)", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "rel-ns"},
			Data:       map[string][]byte{"token": []byte("abc.def.ghi")},
		}
		singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(s).Build())
		params := &ChartSourceParams{
			Source: "https://r.example.com/x.tgz",
			Auth:   &AuthParams{SecretRef: &SecretRefParams{Name: "t"}},
		}
		opts, _, err := resolveHTTPOptions(context.Background(), params, "app-ns", "rel-ns", sourceTypeURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.BearerToken).To(Equal("abc.def.ghi"))
	})

	It("rejects bearer over http (provider-side scheme detection)", func() {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "rel-ns"},
			Data:       map[string][]byte{"token": []byte("abc")},
		}
		singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(s).Build())
		params := &ChartSourceParams{
			Source: "http://r.example.com/x.tgz",
			Auth:   &AuthParams{SecretRef: &SecretRefParams{Name: "t"}},
		}
		_, _, err := resolveHTTPOptions(context.Background(), params, "app-ns", "rel-ns", sourceTypeURL)
		Expect(err).To(MatchError(ContainSubstring(`RFC 6750 §2`)))
	})
})
