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
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kubevela/workflow/pkg/utils/httpguard"
)

// An OCI pull goes through the same outbound guard as every other chart fetch.
var _ = Describe("An OCI chart pull", func() {
	It("is refused for a destination the outbound policy denies", func() {
		registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer registry.Close()
		httpguard.SetEnhancer(func(p httpguard.Policy) httpguard.Policy {
			p.BlockLoopback = true
			return p
		})
		DeferCleanup(httpguard.SetEnhancer, httpguard.PolicyEnhancer(func(p httpguard.Policy) httpguard.Policy { return p }))

		host := strings.TrimPrefix(registry.URL, "http://")
		_, err := NewProviderWithConfig(nil).fetchOCIChart(context.Background(),
			&ChartSourceParams{Source: "oci://" + host + "/charts/c", Version: "1.0.0"}, "team-a", "team-a")
		Expect(err).To(MatchError(ContainSubstring("blocked SSRF")))
	})

	It("stops when the render does, against a registry that never answers", func() {
		silent, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).ShouldNot(HaveOccurred())
		defer silent.Close()
		go func() {
			for {
				conn, err := silent.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err = NewProviderWithConfig(nil).fetchOCIChart(ctx,
			&ChartSourceParams{Source: "oci://" + silent.Addr().String() + "/charts/c", Version: "1.0.0"}, "team-a", "team-a")
		Expect(err).To(MatchError(context.DeadlineExceeded))
		Expect(time.Since(start)).To(BeNumerically("<", 5*time.Second))
	})
})
