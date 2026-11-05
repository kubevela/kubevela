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
	"testing"

	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex"
	kvhttp "github.com/kubevela/pkg/cue/cuex/providers/http"
	workflowfeatures "github.com/kubevela/workflow/pkg/features"
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
		hops := 0
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hops++
			http.Redirect(w, r, server.URL+"/loop", http.StatusFound)
		}))
		DeferCleanup(server.Close)

		Expect(get(server.URL)).To(MatchError(ContainSubstring("stopped after 10 redirects")))
		Expect(hops).To(Equal(maxRedirects))
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
})
