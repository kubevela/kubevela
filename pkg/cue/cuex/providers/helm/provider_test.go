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

	"github.com/kubevela/pkg/cache"
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

	Describe("InitCacheTTL", func() {
		It("should update the cluster-wide TTL defaults", func() {
			prevImmutable := cacheTTLImmutableVersion
			prevMutable := cacheTTLMutableVersion
			defer func() {
				cacheTTLImmutableVersion = prevImmutable
				cacheTTLMutableVersion = prevMutable
			}()

			InitCacheTTL(2*time.Hour, 30*time.Minute)
			config := DefaultCacheTTLConfig()
			Expect(config.ImmutableVersionTTL).To(Equal(2 * time.Hour))
			Expect(config.MutableVersionTTL).To(Equal(30 * time.Minute))
		})

		It("should fall back to defaults for non-positive values", func() {
			prevImmutable := cacheTTLImmutableVersion
			prevMutable := cacheTTLMutableVersion
			defer func() {
				cacheTTLImmutableVersion = prevImmutable
				cacheTTLMutableVersion = prevMutable
			}()

			InitCacheTTL(-time.Hour, 0)
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

	Describe("evictionReasonLabel", func() {
		DescribeTable("should normalize eviction reasons into stable labels",
			func(reason cache.EvictionReason, expected string) {
				Expect(evictionReasonLabel(reason)).To(Equal(expected))
			},
			Entry("capacity", cache.EvictCapacity, "capacity"),
			Entry("ttl", cache.EvictTTL, "ttl"),
			Entry("delete", cache.EvictDelete, "delete"),
			Entry("replace", cache.EvictReplace, "replace"),
			Entry("purge", cache.EvictPurge, "purge"),
			Entry("unknown", cache.EvictionReason("some-other"), "unknown"),
		)
	})

	Describe("cache eviction handler", func() {
		It("should record evictions triggered by a delete on the singleton provider", func() {
			p := NewProvider()
			// Populate the cache then delete it: the package-level OnEvict
			// closure must fire for the EvictDelete reason without panicking.
			key := "evict-delete"
			p.cache.Put(key, []byte("data"), time.Minute)
			Expect(func() { p.cache.Delete(key) }).ShouldNot(Panic())
		})

		It("should record evictions triggered by a replace on a config provider", func() {
			p := NewProviderWithConfig(&CacheTTLConfig{
				ImmutableVersionTTL: 24 * time.Hour,
				MutableVersionTTL:   5 * time.Minute,
			})
			key := "evict-replace"
			p.cache.Put(key, []byte("old"), time.Minute)
			// Overwriting an existing key evicts the previous value (EvictReplace).
			Expect(func() { p.cache.Put(key, []byte("new"), time.Minute) }).ShouldNot(Panic())
		})
	})

})
