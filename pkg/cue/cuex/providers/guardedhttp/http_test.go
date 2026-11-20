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

package guardedhttp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex"
	kvhttp "github.com/kubevela/pkg/cue/cuex/providers/http"
	workflowfeatures "github.com/kubevela/workflow/pkg/features"
	"github.com/kubevela/workflow/pkg/utils/httpguard"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
)

func TestGuardedHTTPSpecs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "guarded vela/http")
}

var _ = Describe("guarded vela/http", func() {
	get := func(url string) error {
		src := fmt.Sprintf("import \"vela/http\"\nr: http.#Do & {$params: {method: \"GET\", url: %q}}\n", url)
		_, err := cuex.NewCompilerWithInternalPackages(Package).CompileString(context.Background(), src)
		return err
	}

	It("keeps the upstream vela/http template", func() {
		Expect(Package.GetName()).To(Equal(kvhttp.Package.GetName()))
		Expect(Package.GetTemplates()).To(Equal(kvhttp.Package.GetTemplates()))
	})

	It("gives up on an endpoint that does not answer within the maximum timeout", func() {
		previous := httpguard.MaxRequestTimeout
		httpguard.MaxRequestTimeout = 200 * time.Millisecond
		DeferCleanup(func() { httpguard.MaxRequestTimeout = previous })
		release := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}))
		DeferCleanup(server.Close)
		DeferCleanup(func() { close(release) })

		start := time.Now()
		Expect(get(server.URL)).To(MatchError(ContainSubstring("Timeout")))
		Expect(time.Since(start)).To(BeNumerically("<", 5*time.Second))
	})

	It("refuses a response body over the limit", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("x", int(httpguard.MaxResponseBytes)+1)))
		}))
		DeferCleanup(server.Close)

		Expect(get(server.URL)).To(MatchError(ContainSubstring("exceeds")))
	})

	It("blocks the cloud metadata address", func() {
		Expect(get("http://169.254.169.254/latest/meta-data/")).To(MatchError(ContainSubstring("blocked SSRF target")))
	})

	It("blocks a redirect into the cloud metadata address", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
		}))
		DeferCleanup(server.Close)

		Expect(get(server.URL)).To(MatchError(ContainSubstring("blocked SSRF target")))
	})

	It("stops a redirect loop at the net/http limit", func() {
		var hops atomic.Int32
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hops.Add(1)
			http.Redirect(w, r, server.URL+"/loop", http.StatusFound)
		}))
		DeferCleanup(server.Close)

		Expect(get(server.URL)).To(MatchError(ContainSubstring("stopped after 10 redirects")))
		Expect(hops.Load()).To(BeNumerically("==", maxRedirects))
	})

	It("blocks private addresses when BlockPrivateHTTPAddresses is on", func() {
		previous := utilfeature.DefaultMutableFeatureGate.Enabled(workflowfeatures.BlockPrivateHTTPAddresses)
		DeferCleanup(func() {
			Expect(utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
				string(workflowfeatures.BlockPrivateHTTPAddresses): previous,
			})).To(Succeed())
		})
		Expect(utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
			string(workflowfeatures.BlockPrivateHTTPAddresses): true,
		})).To(Succeed())

		Expect(get("http://10.0.0.1/")).To(MatchError(ContainSubstring("blocked SSRF target")))
	})

	It("still returns the response from an allowed address", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}))
		DeferCleanup(server.Close)

		src := fmt.Sprintf("import \"vela/http\"\nr: http.#Do & {$params: {method: \"GET\", url: %q}}\n", server.URL)
		v, err := cuex.NewCompilerWithInternalPackages(Package).CompileString(context.Background(), src)
		Expect(err).NotTo(HaveOccurred())
		body, err := v.LookupPath(cue.ParsePath("r.$returns.body")).String()
		Expect(err).NotTo(HaveOccurred())
		Expect(body).To(Equal("ok"))
	})

	It("sends the request headers the template sets", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(r.Header.Get("X-Token") + "," + r.Header.Get("Content-Type")))
		}))
		DeferCleanup(server.Close)

		src := fmt.Sprintf(`
import "vela/http"

r: http.#Do & {$params: {
	method: "POST"
	url:    %q
	request: header: {"X-Token": "abc", "Content-Type": "application/json"}
}}
`, server.URL)
		v, err := cuex.NewCompilerWithInternalPackages(Package).CompileString(context.Background(), src)
		Expect(err).NotTo(HaveOccurred())
		body, err := v.LookupPath(cue.ParsePath("r.$returns.body")).String()
		Expect(err).NotTo(HaveOccurred())
		Expect(body).To(Equal("abc,application/json"))
	})

	It("sends every value of header names that differ only in case", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			values := r.Header.Values("X-Tag")
			sort.Strings(values)
			_, _ = w.Write([]byte(strings.Join(values, ",")))
		}))
		DeferCleanup(server.Close)

		src := fmt.Sprintf(`
import "vela/http"

r: http.#Do & {$params: {
	method: "GET"
	url:    %q
	request: header: {"X-Tag": "a", "x-tag": "b"}
}}
`, server.URL)
		v, err := cuex.NewCompilerWithInternalPackages(Package).CompileString(context.Background(), src)
		Expect(err).NotTo(HaveOccurred())
		body, err := v.LookupPath(cue.ParsePath("r.$returns.body")).String()
		Expect(err).NotTo(HaveOccurred())
		Expect(body).To(Equal("a,b"))
	})
})
