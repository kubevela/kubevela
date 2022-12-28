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

package addon

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/helm/pkg/tlsutil"
)

var _ = Describe("Addon push command", func() {
	var (
		testTarballPath    = "testdata/charts/sample-1.0.1.tgz"
		testServerCertPath = "testdata/tls/server.crt"
		testServerKeyPath  = "testdata/tls/server.key"
		testServerCAPath   = "testdata/tls/server_ca.crt"
		testClientCAPath   = "testdata/tls/client_ca.crt"
		testClientCertPath = "testdata/tls/client.crt"
		testClientKeyPath  = "testdata/tls/client.key"
	)
	var (
		statusCode int
		body       string
		tmp        string
	)
	var p *PushCmd
	var ts *httptest.Server
	var err error
	var ds RegistryDataStore

	setArgsAndRun := func(args []string) error {
		p = &PushCmd{}
		p.Client = k8sClient
		p.Out = os.Stdout
		p.ChartName = args[0]
		p.RepoName = args[1]
		p.SetFieldsFromEnv()
		return p.Push(context.TODO())
	}

	AfterEach(func() {
		ts.Close()
		_ = os.RemoveAll(tmp)
		err = ds.DeleteRegistry(context.TODO(), "helm-push-test")
		Expect(err).To(Succeed())
	})

	Context("plain old HTTP Server", func() {
		BeforeEach(func() {
			statusCode = 201
			body = "{\"success\": true}"
			ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(statusCode)
				w.Write([]byte(body))
			}))

			// Create new Helm home w/ test repo
			tmp, err = os.MkdirTemp("", "helm-push-test")
			Expect(err).To(Succeed())

			// Add our helm repo to addon registry
			ds = NewRegistryDataStore(k8sClient)
			err = ds.AddRegistry(context.TODO(), Registry{
				Name: "helm-push-test",
				Helm: &HelmSource{
					URL: ts.URL,
				},
			})
			Expect(err).To(Succeed())

			_ = os.Setenv("HELM_REPO_USERNAME", "myuser")
			_ = os.Setenv("HELM_REPO_PASSWORD", "mypass")
			_ = os.Setenv("HELM_REPO_CONTEXT_PATH", "/x/y/z")
		})

		It("Not enough args", func() {
			err = setArgsAndRun([]string{"", ""})
			Expect(err).ShouldNot(Succeed(), "expecting error with missing args, instead got nil")
		})

		It("Bad chart path", func() {
			args := []string{"/this/this/not/a/chart", "helm-push-test"}
			err = setArgsAndRun(args)
			Expect(err).ShouldNot(Succeed(), "expecting error with bad chart path, instead got nil")
		})

		It("Bad repo name", func() {
			args := []string{testTarballPath, "this-is-not-a-valid-repo"}
			err = setArgsAndRun(args)
			Expect(err).ShouldNot(Succeed(), "expecting error with bad repo name, instead got nil")
		})

		It("Valid tar, repo name", func() {
			args := []string{testTarballPath, "helm-push-test"}
			err = setArgsAndRun(args)
			Expect(err).Should(Succeed())
		})

		It("Valid tar, repo URL", func() {
			args := []string{testTarballPath, ts.URL}
			err = setArgsAndRun(args)
			Expect(err).Should(Succeed())
		})

		It("Trigger 409, already exists", func() {
			statusCode = 409
			body = "{\"error\": \"package already exists\"}"
			args := []string{testTarballPath, "helm-push-test"}
			err = setArgsAndRun(args)
			Expect(err).ShouldNot(Succeed(), "expecting error with 409, instead got nil")
		})

		It("Unable to parse JSON response body", func() {
			statusCode = 500
			body = "duiasnhioasd"
			args := []string{testTarballPath, "helm-push-test"}
			err = setArgsAndRun(args)
			Expect(err).ShouldNot(Succeed(), "expecting error with bad response body, instead got nil")
		})
	})

	Context("TLS Enabled Server", func() {
		BeforeEach(func() {
			statusCode = 201
			body = "{\"success\": true}"
			ts = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(statusCode)
				_, _ = w.Write([]byte(body))
			}))
			serverCert, err := tls.LoadX509KeyPair(testServerCertPath, testServerKeyPath)
			Expect(err).To(Succeed(), "failed to load certificate and key")

			clientCaCertPool, err := tlsutil.CertPoolFromFile(testClientCAPath)
			Expect(err).To(Succeed(), "load server CA file failed")

			ts.TLS = &tls.Config{
				ClientCAs:    clientCaCertPool,
				ClientAuth:   tls.RequireAndVerifyClientCert,
				Certificates: []tls.Certificate{serverCert},
				Rand:         rand.Reader,
			}
			ts.StartTLS()

			// Create new Helm home w/ test repo
			tmp, err = os.MkdirTemp("", "helm-push-test")
			Expect(err).To(Succeed())

			// Add our helm repo to addon registry
			ds = NewRegistryDataStore(k8sClient)
			err = ds.AddRegistry(context.TODO(), Registry{
				Name: "helm-push-test",
				Helm: &HelmSource{
					URL: ts.URL,
				},
			})
			Expect(err).To(Succeed())

			_ = os.Setenv("HELM_REPO_USERNAME", "myuser")
			_ = os.Setenv("HELM_REPO_PASSWORD", "mypass")
			_ = os.Setenv("HELM_REPO_CONTEXT_PATH", "/x/y/z")
		})

		It("no cert provided", func() {
			_ = os.Unsetenv("HELM_REPO_CA_FILE")
			_ = os.Unsetenv("HELM_REPO_CERT_FILE")
			_ = os.Unsetenv("HELM_REPO_KEY_FILE")
			args := []string{testTarballPath, "helm-push-test"}
			err = setArgsAndRun(args)
			Expect(err).ShouldNot(Succeed(), "expected non nil error but got nil when run cmd without certificate option")
		})

		It("with cert", func() {
			_ = os.Setenv("HELM_REPO_CA_FILE", testServerCAPath)
			_ = os.Setenv("HELM_REPO_CERT_FILE", testClientCertPath)
			_ = os.Setenv("HELM_REPO_KEY_FILE", testClientKeyPath)
			args := []string{testTarballPath, "helm-push-test"}
			err = setArgsAndRun(args)
			Expect(err).Should(Succeed())
		})
	})
})
