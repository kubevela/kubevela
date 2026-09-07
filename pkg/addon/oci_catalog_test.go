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
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/registry"
)

// unreachableRegistryURL is the registry URL these tests name. Nothing dials
// it: stubUnreachableTransport below replaces the transport, so the URL only
// has to be well-formed.
const unreachableRegistryURL = "registry.example.invalid/addon"

// errRegistryUnreachable is the transport-level failure the stubs return. It
// is deliberately not a NAME_UNKNOWN answer, so isOCIRepositoryAbsentError
// reads it as "could not be read" rather than "does not exist" -- the
// distinction every branch under test turns on.
var errRegistryUnreachable = errors.New("dial tcp registry.example.invalid:443: connect: connection refused")

// stubUnreachableTransport makes every portable-catalog transport call fail
// with errRegistryUnreachable for the duration of the test, and restores the
// real functions afterward.
//
// Previously these tests pointed the real transport at a closed loopback port
// and relied on the kernel refusing the connection instantly. That is not a
// property of the code under test: a sandbox that blocks or proxies loopback
// traffic turns the same call into a timeout or a different error, making the
// tests slow or flaky for reasons unrelated to the branches they cover.
func stubUnreachableTransport(t *testing.T) {
	t.Helper()
	originalTags, originalPull := portableCatalogTagsFn, portableCatalogPullFn
	t.Cleanup(func() {
		portableCatalogTagsFn, portableCatalogPullFn = originalTags, originalPull
	})
	portableCatalogTagsFn = func(string, string, string, string, bool) ([]string, error) {
		return nil, errRegistryUnreachable
	}
	portableCatalogPullFn = func(string, string, string, string, bool) ([]byte, error) {
		return nil, errRegistryUnreachable
	}
}

// TestListPortableOCICatalogWrappers covers listPortableOCICatalog and
// listPortableOCICatalogWithPlainHTTP, which only select a transport before
// delegating to listPortableOCICatalogWithTransport.
func TestListPortableOCICatalogWrappers(t *testing.T) {
	stubUnreachableTransport(t)

	t.Run("https", func(t *testing.T) {
		_, err := listPortableOCICatalog(context.Background(), unreachableRegistryURL, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "portable OCI addon catalog is unavailable")
		assert.NotErrorIs(t, err, ErrOCICatalogAbsent, "a read failure must never read as a confirmed absence")
	})

	t.Run("plain http", func(t *testing.T) {
		_, err := listPortableOCICatalogWithPlainHTTP(context.Background(), unreachableRegistryURL, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "portable OCI addon catalog is unavailable")
		assert.NotErrorIs(t, err, ErrOCICatalogAbsent, "a read failure must never read as a confirmed absence")
	})
}

// TestConfirmPortableCatalogAbsent covers the wrapper that re-probes the
// catalog repository before a rewrite. A read failure is not a confirmed
// absence, so it must be refused; only the registry stating that the
// repository does not exist confirms it.
func TestConfirmPortableCatalogAbsent(t *testing.T) {
	t.Run("a read failure is refused", func(t *testing.T) {
		stubUnreachableTransport(t)

		for _, plainHTTP := range []bool{true, false} {
			err := confirmPortableCatalogAbsent(&HelmSource{URL: unreachableRegistryURL}, plainHTTP)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cannot confirm whether")
		}
	})

	t.Run("a missing repository confirms the absence", func(t *testing.T) {
		originalTags := portableCatalogTagsFn
		t.Cleanup(func() { portableCatalogTagsFn = originalTags })
		portableCatalogTagsFn = func(string, string, string, string, bool) ([]string, error) {
			return nil, errors.New("fluxcd: not found: " + ociErrCodeNameUnknown)
		}

		assert.NoError(t, confirmPortableCatalogAbsent(&HelmSource{URL: unreachableRegistryURL}, false))
	})
}

// TestUpdateOCIAddonCatalog pins the early error branch: when the existing
// catalog cannot be read (neither the portable catalog nor the registry
// catalog enumeration is reachable), the update must refuse rather than
// silently rebuild the catalog from an empty list. A nil *registry.Client is
// safe here because the function never reaches client.Push on this path.
func TestUpdateOCIAddonCatalog(t *testing.T) {
	stubUnreachableTransport(t)
	originalCatalogClient := ociCatalogHTTPClient
	t.Cleanup(func() { ociCatalogHTTPClient = originalCatalogClient })
	// The registry-catalog fallback must fail too, so that neither source can
	// read the existing catalog.
	ociCatalogHTTPClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errRegistryUnreachable
	})}

	addonMeta := &chart.Metadata{Name: "fluxcd", Description: "Flux"}

	for _, plainHTTP := range []bool{true, false} {
		err := updateOCIAddonCatalog(nil, &HelmSource{URL: unreachableRegistryURL}, addonMeta, plainHTTP)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refusing to rewrite the OCI addon catalog: cannot read the existing catalog")
	}
}

// roundTripperFunc adapts a function to http.RoundTripper so a test can
// supply an HTTP client that never leaves the process.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

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
		updateOCIAddonCatalogOnceFn = func(*registry.Client, *HelmSource, *chart.Metadata, bool) (bool, error) {
			calls++
			if calls < 3 {
				return true, assert.AnError
			}
			return false, nil
		}
		err := updateOCIAddonCatalog(nil, &HelmSource{URL: unreachableRegistryURL}, &chart.Metadata{Name: "fluxcd"}, false)
		require.NoError(t, err)
		assert.Equal(t, 3, calls, "must stop retrying as soon as an attempt succeeds")
	})

	t.Run("gives up after maxCatalogPublishAttempts and reports the last conflict", func(t *testing.T) {
		var calls int
		updateOCIAddonCatalogOnceFn = func(*registry.Client, *HelmSource, *chart.Metadata, bool) (bool, error) {
			calls++
			return true, assert.AnError
		}
		err := updateOCIAddonCatalog(nil, &HelmSource{URL: unreachableRegistryURL}, &chart.Metadata{Name: "fluxcd"}, false)
		require.Error(t, err)
		assert.Equal(t, maxCatalogPublishAttempts, calls)
		assert.Contains(t, err.Error(), "a concurrent publisher kept winning the race")
	})

	t.Run("a non-conflict error returns immediately without retrying", func(t *testing.T) {
		var calls int
		updateOCIAddonCatalogOnceFn = func(*registry.Client, *HelmSource, *chart.Metadata, bool) (bool, error) {
			calls++
			return false, assert.AnError
		}
		err := updateOCIAddonCatalog(nil, &HelmSource{URL: unreachableRegistryURL}, &chart.Metadata{Name: "fluxcd"}, false)
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

		conflict, err := publishCatalogEntry(nil, &HelmSource{URL: unreachableRegistryURL}, addonMeta, nil, false)
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
			_, _ = publishCatalogEntry(nil, &HelmSource{URL: unreachableRegistryURL}, addonMeta, nil, false)
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

		conflict, err := publishCatalogEntry(nil, &HelmSource{URL: unreachableRegistryURL}, addonMeta, nil, false)
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

		conflict, err := publishCatalogEntry(nil, &HelmSource{URL: unreachableRegistryURL}, addonMeta, nil, false)
		require.Error(t, err)
		assert.True(t, conflict, "a failed re-check must be retried, not treated as unchanged")
		assert.Contains(t, err.Error(), "cannot confirm the portable OCI addon catalog tag is still unchanged")
		assert.Equal(t, 2, calls, "must not push once the re-check fails")
	})
}

// TestPublishCatalogEntryBootstrapsFirstCatalog pins the case
// TestPublishCatalogEntryRefusesOnTagListError's fix regressed: on the very
// first push to a registry, the catalog repository does not exist yet, so
// both catalog-tag listings answer NAME_UNKNOWN. That is not an
// unconfirmable failure -- it is the registry stating there is nothing
// there -- so it must let the attempt proceed and bootstrap the catalog,
// not be treated the same as a genuine listing failure and retried forever.
func TestPublishCatalogEntryBootstrapsFirstCatalog(t *testing.T) {
	originalCatalogTags := catalogRepoTagsFn
	originalAddonTags := addonVersionsTagsFn
	defer func() {
		catalogRepoTagsFn = originalCatalogTags
		addonVersionsTagsFn = originalAddonTags
	}()
	addonVersionsTagsFn = func(_, _, _, _ string, _ bool) ([]string, error) {
		return nil, nil
	}

	nameUnknown := errors.New(`unexpected status code 404: name unknown: repository name not known to registry`)
	var calls int
	catalogRepoTagsFn = func(_, _, _, _ string, _ bool) ([]string, error) {
		calls++
		return nil, nameUnknown
	}

	addonMeta := &chart.Metadata{Name: "fluxcd", Description: "Flux"}

	// client is nil, so a real push would panic; reaching client.Push proves
	// the confirmed-absent repository let this attempt through to bootstrap
	// the catalog instead of reporting a conflict.
	assert.Panics(t, func() {
		_, _ = publishCatalogEntry(nil, &HelmSource{URL: unreachableRegistryURL}, addonMeta, nil, false)
	}, "a confirmed-absent catalog repository must bootstrap, not retry forever")
	assert.Equal(t, 2, calls, "both the initial read and the pre-push re-check must see the same confirmed-absent answer")
}

// TestValidateOCIAddonName pins the reserved-name rejection. The catalog is
// published to a fixed repository name so discovery stays portable across
// registries, which means an addon of that name shares its repository and tag
// namespace: pushing it would overwrite catalog tags with addon archives and
// leave both unreadable.
func TestValidateOCIAddonName(t *testing.T) {
	assert.NoError(t, validateOCIAddonName("fluxcd"))
	assert.NoError(t, validateOCIAddonName("kubevela-addon-catalog-extra"))

	for _, name := range []string{ociCatalogChartName, " " + ociCatalogChartName + " ", "KubeVela-Addon-Catalog"} {
		err := validateOCIAddonName(name)
		require.Error(t, err, "name %q must be refused", name)
		assert.Contains(t, err.Error(), "is reserved for the portable OCI addon catalog")
	}
}

// TestPublishCatalogEntryRefusesReservedAddonName is the same rule at the
// publisher, so no caller can reach the point of writing an addon's versions
// into the repository that holds the catalog itself.
func TestPublishCatalogEntryRefusesReservedAddonName(t *testing.T) {
	originalAddonTags := addonVersionsTagsFn
	t.Cleanup(func() { addonVersionsTagsFn = originalAddonTags })
	addonVersionsTagsFn = func(_, _, _, _ string, _ bool) ([]string, error) {
		t.Fatal("must refuse the reserved name before listing any tags")
		return nil, nil
	}

	conflict, err := publishCatalogEntry(nil, &HelmSource{URL: unreachableRegistryURL},
		&chart.Metadata{Name: ociCatalogChartName}, nil, false)
	require.Error(t, err)
	assert.False(t, conflict, "a reserved name is not a race worth retrying")
	assert.Contains(t, err.Error(), "is reserved for the portable OCI addon catalog")
}

// TestUpdateOCIAddonCatalogOnceRequiresConfirmedAbsence covers the gap between
// "the portable catalog said nothing" and "nothing is published there".
//
// When the portable catalog cannot be read, listUIData falls back to
// enumerating the registry. A successful enumeration says nothing about what
// the portable catalog holds, and an empty one says even less: a registry that
// answers /v2/_catalog with no repositories (a proxy, a namespace-scoped
// token, a cache) would otherwise authorize replacing a populated catalog with
// just the addon being pushed.
func TestUpdateOCIAddonCatalogOnceRequiresConfirmedAbsence(t *testing.T) {
	originalCatalogClient := ociCatalogHTTPClient
	originalPortableTags, originalPortablePull := portableCatalogTagsFn, portableCatalogPullFn
	originalAddonTags, originalCatalogTags := addonVersionsTagsFn, catalogRepoTagsFn
	t.Cleanup(func() {
		ociCatalogHTTPClient = originalCatalogClient
		portableCatalogTagsFn, portableCatalogPullFn = originalPortableTags, originalPortablePull
		addonVersionsTagsFn, catalogRepoTagsFn = originalAddonTags, originalCatalogTags
	})

	// The registry enumerates cleanly, and reports nothing.
	ociCatalogHTTPClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(`{"repositories":[]}`)),
		}, nil
	})}
	// The portable catalog itself is unreadable: its tag listing fails with
	// something that is not a missing-repository answer.
	portableCatalogTagsFn = func(string, string, string, string, bool) ([]string, error) {
		return nil, errRegistryUnreachable
	}
	portableCatalogPullFn = func(string, string, string, string, bool) ([]byte, error) {
		return nil, errRegistryUnreachable
	}
	addonVersionsTagsFn = func(_, _, _, _ string, _ bool) ([]string, error) {
		return []string{"1.0.0"}, nil
	}
	catalogRepoTagsFn = func(_, _, _, _ string, _ bool) ([]string, error) {
		t.Fatal("must refuse before computing the next catalog version")
		return nil, nil
	}

	conflict, err := updateOCIAddonCatalogOnce(nil, &HelmSource{URL: unreachableRegistryURL},
		&chart.Metadata{Name: "fluxcd"}, false)
	require.Error(t, err)
	assert.False(t, conflict)
	assert.Contains(t, err.Error(), "refusing to rewrite the OCI addon catalog")
}
