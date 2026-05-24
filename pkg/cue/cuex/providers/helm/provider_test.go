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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("provider", func() {

	Describe("dry-run context", func() {
		It("should default to false", func() {
			ctx := context.Background()
			Expect(isDryRun(ctx)).To(BeFalse())
		})

		It("should be true after WithDryRun", func() {
			ctx := context.Background()
			dryCtx := WithDryRun(ctx)
			Expect(isDryRun(dryCtx)).To(BeTrue())
		})

		It("should not affect the original context", func() {
			ctx := context.Background()
			_ = WithDryRun(ctx)
			Expect(isDryRun(ctx)).To(BeFalse())
		})
	})

	Describe("DefaultCacheTTLConfig", func() {
		It("should return correct defaults", func() {
			config := DefaultCacheTTLConfig()
			Expect(config.ImmutableVersionTTL).To(Equal(24 * time.Hour))
			Expect(config.MutableVersionTTL).To(Equal(5 * time.Minute))
		})
	})

	Describe("NewProviderWithConfig", func() {
		It("should use defaults when config is nil", func() {
			p := NewProviderWithConfig(nil)
			Expect(p.cacheTTL.ImmutableVersionTTL).To(Equal(24 * time.Hour))
			Expect(p.cacheTTL.MutableVersionTTL).To(Equal(5 * time.Minute))
			Expect(p.releaseFingerprints).ToNot(BeNil())
			Expect(p.releaseManifests).ToNot(BeNil())
			Expect(p.releaseVersions).ToNot(BeNil())
		})

		It("should use custom config when provided", func() {
			p := NewProviderWithConfig(&CacheTTLConfig{
				ImmutableVersionTTL: 1 * time.Hour,
				MutableVersionTTL:   1 * time.Minute,
			})
			Expect(p.cacheTTL.ImmutableVersionTTL).To(Equal(1 * time.Hour))
			Expect(p.cacheTTL.MutableVersionTTL).To(Equal(1 * time.Minute))
		})
	})

	Describe("Template and Package exports", func() {
		It("should have a non-empty embedded CUE template", func() {
			Expect(Template).ToNot(BeEmpty())
		})

		It("should have a non-nil provider package", func() {
			Expect(Package).ToNot(BeNil())
		})
	})

})
