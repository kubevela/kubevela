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

package common

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	workflowfeatures "github.com/kubevela/workflow/pkg/features"
	"github.com/kubevela/workflow/pkg/utils/httpguard"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
)

func TestCommonSpecs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "common utils")
}

var _ = Describe("guardedRedirect", func() {
	hop := func(url string) *http.Request {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		Expect(err).NotTo(HaveOccurred())
		return req
	}
	chain := func(n int) []*http.Request {
		via := make([]*http.Request, n)
		for i := range via {
			via[i] = hop("https://example.com/")
		}
		return via
	}

	It("stops at the redirect limit before it checks the target", func() {
		called := false
		check := guardedRedirect(outboundHTTPPolicy(), func(*http.Request, []*http.Request) error {
			called = true
			return nil
		})

		err := check(hop("https://example.com/"), chain(defaultRedirectLimit))

		Expect(err).To(MatchError("stopped after 10 redirects"))
		Expect(called).To(BeFalse())
	})

	It("rejects a hop to a blocked host without calling next", func() {
		called := false
		check := guardedRedirect(outboundHTTPPolicy(), func(*http.Request, []*http.Request) error {
			called = true
			return nil
		})

		err := check(hop("http://169.254.169.254/latest/meta-data/"), chain(1))

		Expect(err).To(MatchError(ContainSubstring("blocked SSRF target")))
		Expect(called).To(BeFalse())
	})

	It("passes an allowed hop to next and returns its error", func() {
		refused := errors.New("refused by next")
		check := guardedRedirect(outboundHTTPPolicy(), func(*http.Request, []*http.Request) error {
			return refused
		})

		Expect(check(hop("https://example.com/next"), chain(1))).To(MatchError(refused))
	})

	It("allows a hop when next is not set", func() {
		Expect(guardedRedirect(outboundHTTPPolicy(), nil)(hop("https://example.com/next"), chain(1))).To(Succeed())
	})
})

var _ = Describe("outbound HTTP guard", func() {
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

		Expect(outboundHTTPPolicy().BlockPrivate).To(BeTrue())
	})

	It("rejects a blocked host before it sends the request", func() {
		_, err := HTTPGetResponse(context.Background(), "http://169.254.169.254/latest/meta-data/", nil)
		Expect(err).To(MatchError(ContainSubstring("blocked SSRF target")))
	})

	It("keeps the dial-time guard on the transport when TLS verification is skipped", func() {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		DeferCleanup(server.Close)
		// localhost is not an IP literal, so the pre-request host check lets it
		// through. Only the guarded transport sees the resolved loopback address.
		url := "https://localhost:" + strconv.Itoa(server.Listener.Addr().(*net.TCPAddr).Port)
		opts := &HTTPOption{InsecureSkipTLS: true}

		resp, err := HTTPGetResponse(context.Background(), url, opts)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Body.Close()).To(Succeed())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		httpguard.SetEnhancer(func(p httpguard.Policy) httpguard.Policy {
			p.BlockLoopback = true
			return p
		})
		DeferCleanup(httpguard.SetEnhancer, httpguard.PolicyEnhancer(func(p httpguard.Policy) httpguard.Policy { return p }))

		_, err = HTTPGetResponse(context.Background(), url, opts)
		Expect(err).To(MatchError(ContainSubstring("blocked SSRF target")))
	})
})
