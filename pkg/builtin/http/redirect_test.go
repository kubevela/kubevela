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

package http

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/builtin/registry"
)

func TestHTTPBuiltinSpecs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "builtin HTTP")
}

var _ = Describe("HTTP builtin redirects", func() {
	It("stops a redirect loop at the net/http limit", func() {
		var hops atomic.Int32
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hops.Add(1)
			http.Redirect(w, r, server.URL+"/loop", http.StatusFound)
		}))
		DeferCleanup(server.Close)

		runner, err := newHTTPCmd(cue.Value{})
		Expect(err).NotTo(HaveOccurred())
		req := cuecontext.New().CompileString(fmt.Sprintf("method: \"GET\"\nurl: %q", server.URL+"/loop"))
		_, err = runner.Run(&registry.Meta{Obj: req.Value()})

		Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("stopped after %d redirects", maxRedirects))))
		Expect(hops.Load()).To(BeNumerically("==", maxRedirects))
	})
})
