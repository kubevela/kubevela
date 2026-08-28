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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/registry"
)

// closedPortRegistryURL points listPortableOCICatalog* at a loopback port
// nothing listens on, so listOCITagsWithTransport fails to dial rather than
// answering "repository does not exist" -- exercising the "unavailable" branch
// deterministically without any real network dependency.
const closedPortRegistryURL = "127.0.0.1:1/addon"

// TestListPortableOCICatalogWrappers covers listPortableOCICatalog and
// listPortableOCICatalogWithPlainHTTP, which only select a transport before
// delegating to listPortableOCICatalogWithTransport.
func TestListPortableOCICatalogWrappers(t *testing.T) {
	t.Run("https", func(t *testing.T) {
		_, err := listPortableOCICatalog(context.Background(), closedPortRegistryURL, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "portable OCI addon catalog is unavailable")
	})

	t.Run("plain http", func(t *testing.T) {
		_, err := listPortableOCICatalogWithPlainHTTP(context.Background(), closedPortRegistryURL, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "portable OCI addon catalog is unavailable")
	})
}

// TestConfirmPortableCatalogAbsent covers the wrapper that re-probes the
// catalog repository before a rewrite. A dial failure is not a confirmed
// absence, so it must be refused.
func TestConfirmPortableCatalogAbsent(t *testing.T) {
	for _, plainHTTP := range []bool{true, false} {
		err := confirmPortableCatalogAbsent(&OCIAddonSource{URL: closedPortRegistryURL}, plainHTTP)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot confirm whether")
	}
}

// TestUpdateOCIAddonCatalog pins the early error branch: when the existing
// catalog cannot be read (neither the portable catalog nor the registry
// catalog enumeration is reachable), the update must refuse rather than
// silently rebuild the catalog from an empty list. A nil *registry.Client is
// safe here because the function never reaches client.Push on this path.
func TestUpdateOCIAddonCatalog(t *testing.T) {
	addonMeta := &chart.Metadata{Name: "fluxcd", Description: "Flux"}

	for _, plainHTTP := range []bool{true, false} {
		err := updateOCIAddonCatalog(nil, &OCIAddonSource{URL: closedPortRegistryURL}, addonMeta, plainHTTP)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refusing to rewrite the OCI addon catalog: cannot read the existing catalog")
	}
}

// TestCatalogTagHead pins the comparison publishCatalogEntry's conflict check
// depends on: no tags reads as absent, and the highest tag identifies a
// change. A listing error is handled by the caller before this is ever
// called (see TestPublishCatalogEntryRefusesOnTagListError) -- conflating it
// with "confirmed absent" here would let an unreadable catalog look
// unchanged and get published over.
func TestCatalogTagHead(t *testing.T) {
	assert.Equal(t, "", catalogTagHead(nil), "no tags: absent")
	assert.Equal(t, "", catalogTagHead([]string{}), "empty list: absent")
	assert.Equal(t, "0.0.2", catalogTagHead([]string{"0.0.2", "0.0.1"}), "highest tag identifies the head")
}

// TestUpdateOCIAddonCatalogRetriesOnConflict pins the retry loop itself:
// every attempt reporting a conflict must be retried up to
// maxCatalogPublishAttempts times, backing off between attempts, and then
// surface a clear error rather than silently giving up after one collision.
func TestUpdateOCIAddonCatalogRetriesOnConflict(t *testing.T) {
	original := updateOCIAddonCatalogOnceFn
	defer func() { updateOCIAddonCatalogOnceFn = original }()

	t.Run("retries until a later attempt succeeds", func(t *testing.T) {
		var calls int
		updateOCIAddonCatalogOnceFn = func(*registry.Client, *OCIAddonSource, *chart.Metadata, bool) (bool, error) {
			calls++
			if calls < 3 {
				return true, assert.AnError
			}
			return false, nil
		}
		err := updateOCIAddonCatalog(nil, &OCIAddonSource{URL: closedPortRegistryURL}, &chart.Metadata{Name: "fluxcd"}, false)
		require.NoError(t, err)
		assert.Equal(t, 3, calls, "must stop retrying as soon as an attempt succeeds")
	})

	t.Run("gives up after maxCatalogPublishAttempts and reports the last conflict", func(t *testing.T) {
		var calls int
		updateOCIAddonCatalogOnceFn = func(*registry.Client, *OCIAddonSource, *chart.Metadata, bool) (bool, error) {
			calls++
			return true, assert.AnError
		}
		err := updateOCIAddonCatalog(nil, &OCIAddonSource{URL: closedPortRegistryURL}, &chart.Metadata{Name: "fluxcd"}, false)
		require.Error(t, err)
		assert.Equal(t, maxCatalogPublishAttempts, calls)
		assert.Contains(t, err.Error(), "a concurrent publisher kept winning the race")
	})

	t.Run("a non-conflict error returns immediately without retrying", func(t *testing.T) {
		var calls int
		updateOCIAddonCatalogOnceFn = func(*registry.Client, *OCIAddonSource, *chart.Metadata, bool) (bool, error) {
			calls++
			return false, assert.AnError
		}
		err := updateOCIAddonCatalog(nil, &OCIAddonSource{URL: closedPortRegistryURL}, &chart.Metadata{Name: "fluxcd"}, false)
		require.Error(t, err)
		assert.Equal(t, 1, calls, "a definitive failure must not be retried")
		assert.Same(t, assert.AnError, err)
	})
}

// TestPublishCatalogEntryDetectsConflict drives the actual conflict-detection
// wiring in publishCatalogEntry (not just the pure catalogTagHead helper, and
// not just the retry loop around it): a catalog tag that changes between the
// version-computation read and the pre-push re-check must report a conflict
// and must not push. Because publishCatalogEntry takes the existing addon
// list directly, this needs no real registry for the existing-catalog read
// that normally precedes it.
func TestPublishCatalogEntryDetectsConflict(t *testing.T) {
	originalCatalogTags := catalogRepoTagsFn
	originalAddonTags := addonVersionsTagsFn
	defer func() {
		catalogRepoTagsFn = originalCatalogTags
		addonVersionsTagsFn = originalAddonTags
	}()
	// Not under test here: publishCatalogEntry also looks up the
	// addon-under-publish's own versions to populate the catalog entry. Stub
	// it out so the conflict-detection scenarios below don't need a real
	// registry to satisfy it.
	addonVersionsTagsFn = func(_, _, _, _ string, _ bool) ([]string, error) {
		return []string{"1.0.0"}, nil
	}

	addonMeta := &chart.Metadata{Name: "fluxcd", Description: "Flux"}

	t.Run("a tag that appears between the two reads is a conflict", func(t *testing.T) {
		var calls int
		catalogRepoTagsFn = func(_, _, _, _ string, _ bool) ([]string, error) {
			calls++
			if calls == 1 {
				return nil, nil // no catalog published yet when this attempt started
			}
			return []string{"0.0.1"}, nil // a concurrent publisher landed one before the push
		}

		conflict, err := publishCatalogEntry(nil, &OCIAddonSource{URL: closedPortRegistryURL}, addonMeta, nil, false)
		require.Error(t, err)
		assert.True(t, conflict, "a tag appearing mid-attempt must be reported as a conflict, not a hard failure")
		assert.Contains(t, err.Error(), "a concurrent publisher updated the portable OCI addon catalog")
		assert.Equal(t, 2, calls, "must not push once a conflict is detected")
	})

	t.Run("an unchanged tag across both reads is not a conflict", func(t *testing.T) {
		catalogRepoTagsFn = func(_, _, _, _ string, _ bool) ([]string, error) {
			return []string{"0.0.3"}, nil
		}

		// client is nil, so a real push would panic; reaching client.Push proves
		// the conflict check let this attempt through instead of retrying.
		assert.Panics(t, func() {
			_, _ = publishCatalogEntry(nil, &OCIAddonSource{URL: closedPortRegistryURL}, addonMeta, nil, false)
		}, "an unchanged tag must proceed to publish rather than report a conflict")
	})
}

// TestPublishCatalogEntryRefusesOnTagListError pins the fix for a failed tag
// listing being silently treated as a confirmed-empty catalog: a listing
// error, on either the initial read or the pre-push re-check, must retry
// rather than let publishCatalogEntry default to version "0.0.1" and publish
// over a catalog it never actually confirmed the state of.
func TestPublishCatalogEntryRefusesOnTagListError(t *testing.T) {
	originalCatalogTags := catalogRepoTagsFn
	originalAddonTags := addonVersionsTagsFn
	defer func() {
		catalogRepoTagsFn = originalCatalogTags
		addonVersionsTagsFn = originalAddonTags
	}()
	addonVersionsTagsFn = func(_, _, _, _ string, _ bool) ([]string, error) {
		return []string{"1.0.0"}, nil
	}

	addonMeta := &chart.Metadata{Name: "fluxcd", Description: "Flux"}

	t.Run("the initial read failing is a conflict, not a silent default version", func(t *testing.T) {
		var calls int
		catalogRepoTagsFn = func(_, _, _, _ string, _ bool) ([]string, error) {
			calls++
			return nil, assert.AnError
		}

		conflict, err := publishCatalogEntry(nil, &OCIAddonSource{URL: closedPortRegistryURL}, addonMeta, nil, false)
		require.Error(t, err)
		assert.True(t, conflict, "a failed listing must be retried, not treated as an empty catalog")
		assert.Contains(t, err.Error(), "cannot confirm the portable OCI addon catalog's current tag")
		assert.Equal(t, 1, calls, "must not proceed to build or push the catalog chart")
	})

	t.Run("the pre-push re-check failing is a conflict too", func(t *testing.T) {
		var calls int
		catalogRepoTagsFn = func(_, _, _, _ string, _ bool) ([]string, error) {
			calls++
			if calls == 1 {
				return []string{"0.0.3"}, nil
			}
			return nil, assert.AnError
		}

		conflict, err := publishCatalogEntry(nil, &OCIAddonSource{URL: closedPortRegistryURL}, addonMeta, nil, false)
		require.Error(t, err)
		assert.True(t, conflict, "a failed re-check must be retried, not treated as unchanged")
		assert.Contains(t, err.Error(), "cannot confirm the portable OCI addon catalog tag is still unchanged")
		assert.Equal(t, 2, calls, "must not push once the re-check fails")
	})
}
