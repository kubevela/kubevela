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

package component

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

func TestOCIErrors(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OCI registry errors")
}

// registryAnswer is the error oras-go v2 returns for a request the registry
// refused, wrapped the way a caller further up would wrap it.
func registryAnswer(status int, code, message string) error {
	resp := &errcode.ErrorResponse{
		Method:     http.MethodGet,
		URL:        &url.URL{Scheme: "https", Host: "registry.example", Path: "/v2/addons/demo/tags/list"},
		StatusCode: status,
	}
	if code != "" {
		resp.Errors = errcode.Errors{{Code: code, Message: message}}
	}
	return fmt.Errorf("list tags: %w", resp)
}

// flattened is the same error after something turned it into text.
func flattened(err error) error { return errors.New(err.Error()) }

var _ = Describe("registry errors from oras-go v2", func() {
	It("finds the registry's answer through wrapping, and not in plain text", func() {
		err := registryAnswer(http.StatusNotFound, errcode.ErrorCodeNameUnknown, "repository name not known to registry")
		resp, ok := OCIErrorResponse(err)
		Expect(ok).To(BeTrue())
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))

		_, ok = OCIErrorResponse(flattened(err))
		Expect(ok).To(BeFalse())
		_, ok = OCIErrorResponse(nil)
		Expect(ok).To(BeFalse())
	})

	DescribeTable("a repository that does not exist",
		func(err error, absent bool) {
			Expect(IsOCIRepositoryAbsentError(err)).To(Equal(absent))
			// The message fallback must agree with the typed answer.
			Expect(IsOCIRepositoryAbsentError(flattened(err))).To(Equal(absent))
		},
		Entry("NAME_UNKNOWN", registryAnswer(http.StatusNotFound, errcode.ErrorCodeNameUnknown, "repository name not known to registry"), true),
		Entry("NAME_UNKNOWN without a message", registryAnswer(http.StatusNotFound, errcode.ErrorCodeNameUnknown, ""), true),
		// A bare 404 is ambiguous: a proxy answers the same for a repository that exists.
		Entry("a bare 404", registryAnswer(http.StatusNotFound, "", ""), false),
		Entry("a missing tag", registryAnswer(http.StatusNotFound, errcode.ErrorCodeManifestUnknown, "manifest unknown"), false),
		Entry("a refused caller", registryAnswer(http.StatusUnauthorized, errcode.ErrorCodeUnauthorized, "authentication required"), false),
	)

	DescribeTable("a registry that is throttling",
		func(err error, throttled bool) {
			Expect(IsOCIThrottledError(err)).To(Equal(throttled))
			Expect(IsOCIThrottledError(flattened(err))).To(Equal(throttled))
		},
		Entry("429 with the spec's code", registryAnswer(http.StatusTooManyRequests, "TOOMANYREQUESTS", "pull rate limit"), true),
		Entry("429 with no body", registryAnswer(http.StatusTooManyRequests, "", ""), true),
		Entry("a missing tag", registryAnswer(http.StatusNotFound, errcode.ErrorCodeManifestUnknown, "manifest unknown"), false),
		Entry("a refused caller", registryAnswer(http.StatusUnauthorized, errcode.ErrorCodeUnauthorized, "authentication required"), false),
	)
})
