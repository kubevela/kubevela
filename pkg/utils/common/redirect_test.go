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
	"errors"
	"net/http"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
