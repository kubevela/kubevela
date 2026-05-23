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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("release_secrets", func() {

	Describe("InvalidateRelease", func() {
		It("should clear all cache entries for a release", func() {
			p := NewProviderWithConfig(nil)

			cacheKey := "default/test-rel"
			p.releaseMu.Lock()
			p.releaseFingerprints[cacheKey] = "fp1"
			p.releaseManifests[cacheKey] = "manifest"
			p.releaseVersions[cacheKey] = 1
			p.releaseMu.Unlock()

			Expect(p.releaseFingerprints[cacheKey]).To(Equal("fp1"))

			p.InvalidateRelease("test-rel", "default")

			_, ok := p.releaseFingerprints[cacheKey]
			Expect(ok).To(BeFalse())
			_, ok = p.releaseManifests[cacheKey]
			Expect(ok).To(BeFalse())
			_, ok = p.releaseVersions[cacheKey]
			Expect(ok).To(BeFalse())
		})
	})

	Describe("validateReleaseHealth", func() {
		It("should not panic without a real cluster", func() {
			p := NewProviderWithConfig(nil)
			// This runs in background normally, but we call it directly
			// It will fail to get action config, but should not panic
			Expect(func() {
				p.validateReleaseHealth("nonexistent-release", "default")
			}).ToNot(Panic())
		})
	})

	Describe("cluster-dependent functions (no cluster)", func() {
		var p *Provider

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
		})

		It("cleanOrphanedReleaseSecrets should not panic", func() {
			Expect(func() {
				_ = p.cleanOrphanedReleaseSecrets(nil, "test-release", "default", nil)
			}).ToNot(Panic())
		})

		It("deleteReleaseSecretsDirect should not panic", func() {
			Expect(func() {
				_ = p.deleteReleaseSecretsDirect("default", "test-release", nil)
			}).ToNot(Panic())
		})

		It("listReleaseSecretNames should return empty or nil for nonexistent release", func() {
			names := p.listReleaseSecretNames("default", "nonexistent-release-xyz")
			// May return nil (no cluster) or empty slice (cluster but no secrets)
			Expect(len(names)).To(Equal(0))
		})

		It("labelReleaseSecrets should not panic with nil context", func() {
			Expect(func() {
				p.labelReleaseSecrets("default", "test-release", nil)
			}).ToNot(Panic())
		})

		It("labelReleaseSecrets should not panic without cluster", func() {
			velaCtx := &ContextParams{
				AppName:      "app",
				AppNamespace: "ns",
				Name:         "comp",
			}
			Expect(func() {
				p.labelReleaseSecrets("default", "test-release", velaCtx)
			}).ToNot(Panic())
		})
	})

})
