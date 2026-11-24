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

package addon

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

var _ = Describe("an OCI registry failure another registry cannot make up for", func() {
	// answer is the error oras-go v2 returns for a refused request, wrapped
	// the way the pull path wraps it.
	answer := func(status int, code, message string) error {
		resp := &errcode.ErrorResponse{
			Method:     http.MethodGet,
			URL:        &url.URL{Scheme: "https", Host: "registry.example", Path: "/v2/addons/demo/manifests/1.0.0"},
			StatusCode: status,
		}
		if code != "" {
			resp.Errors = errcode.Errors{{Code: code, Message: message}}
		}
		return fmt.Errorf("pull addon: %w", resp)
	}

	DescribeTable("isOCIFatalError",
		func(err error, fatal bool) {
			Expect(isOCIFatalError(err)).To(Equal(fatal))
			// The same error after something turned it into text.
			Expect(isOCIFatalError(errors.New(err.Error()))).To(Equal(fatal))
		},
		Entry("401 with the spec's code", answer(http.StatusUnauthorized, errcode.ErrorCodeUnauthorized, "authentication required"), true),
		Entry("401 with no body", answer(http.StatusUnauthorized, "", ""), true),
		Entry("403 with the spec's code", answer(http.StatusForbidden, errcode.ErrorCodeDenied, "requested access to the resource is denied"), true),
		Entry("403 with no body", answer(http.StatusForbidden, "", ""), true),
		Entry("a missing tag", answer(http.StatusNotFound, errcode.ErrorCodeManifestUnknown, "manifest unknown"), false),
		Entry("a registry that is down", answer(http.StatusServiceUnavailable, "", ""), false),
	)

	It("still reads failures that never were a registry response", func() {
		Expect(isOCIFatalError(errors.New("failed to login to OCI registry: no credentials"))).To(BeTrue())
		Expect(isOCIFatalError(errors.New("artifact has no chart layer"))).To(BeTrue())
		Expect(isOCIFatalError(errors.New("dial tcp: connection refused"))).To(BeFalse())
	})
})
