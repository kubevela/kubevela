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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAwaitOCICall(t *testing.T) {
	t.Run("returns the operation's result", func(t *testing.T) {
		got, err := AwaitOCICall(context.Background(), func() (int, error) { return 42, nil })
		require.NoError(t, err)
		assert.Equal(t, 42, got)

		boom := errors.New("boom")
		_, err = AwaitOCICall(context.Background(), func() (int, error) { return 0, boom })
		assert.ErrorIs(t, err, boom)
	})

	t.Run("does not start when already cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var started atomic.Bool
		got, err := AwaitOCICall(ctx, func() (string, error) { started.Store(true); return "x", nil })
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, "", got)
		time.Sleep(10 * time.Millisecond)
		assert.False(t, started.Load(), "a cancelled caller must not spawn work whose result it will discard")
	})

	t.Run("releases the caller when cancelled mid-call", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		started := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			_, err := AwaitOCICall(ctx, func() (string, error) { close(started); <-release; return "late", nil })
			done <- err
		}()
		<-started // the operation is running; now the caller gives up
		cancel()
		select {
		case err := <-done:
			assert.ErrorIs(t, err, context.Canceled)
		case <-time.After(2 * time.Second):
			t.Fatal("the caller stayed blocked on an operation that ignores its context")
		}
	})
}

func TestOCIRegistryLocation(t *testing.T) {
	for raw, want := range map[string][2]string{
		"oci://reg.example.com/charts":                 {"reg.example.com", "charts"},
		"OCI://reg.example.com/charts/":                {"reg.example.com", "charts"},
		"oci://reg.example.com":                        {"reg.example.com", ""},
		"oci://reg.example.com/":                       {"reg.example.com", ""},
		"https://reg.example.com/a/b/":                 {"reg.example.com", "a/b"},
		"http://127.0.0.1:30500/modules":               {"127.0.0.1:30500", "modules"},
		"reg.example.com/charts":                       {"reg.example.com", "charts"},
		"123456789012.dkr.ecr.us-west-2.amazonaws.com": {"123456789012.dkr.ecr.us-west-2.amazonaws.com", ""},
		"": {"", ""},
	} {
		host, prefix := OCIRegistryLocation(raw)
		assert.Equal(t, want[0], host, "host of %q", raw)
		assert.Equal(t, want[1], prefix, "prefix of %q", raw)
	}
}

func TestOCIRepoRef(t *testing.T) {
	for _, tc := range []struct{ url, name, repoRef, host string }{
		{"oci://reg.example.com/charts", "widget", "reg.example.com/charts/widget", "reg.example.com"},
		{"oci://reg.example.com/charts/", "/widget", "reg.example.com/charts/widget", "reg.example.com"},
		{"oci://reg.example.com", "widget", "reg.example.com/widget", "reg.example.com"},
		{"http://127.0.0.1:30500/modules", "widget-kit", "127.0.0.1:30500/modules/widget-kit", "127.0.0.1:30500"},
		{"reg.example.com", "widget", "reg.example.com/widget", "reg.example.com"},
	} {
		repoRef, host := OCIRepoRef(tc.url, tc.name)
		assert.Equal(t, tc.repoRef, repoRef, "repoRef for %q + %q", tc.url, tc.name)
		assert.Equal(t, tc.host, host, "host for %q", tc.url)
	}
}

func TestIsOCIRepositoryAbsentError(t *testing.T) {
	assert.False(t, IsOCIRepositoryAbsentError(nil))
	assert.True(t, IsOCIRepositoryAbsentError(errors.New(`GET "https://r/v2/a/tags/list": response status code 404: name unknown: repository name not known to registry`)))
	assert.False(t, IsOCIRepositoryAbsentError(errors.New(`GET "https://r/v2/a/tags/list": response status code 404: Not Found`)),
		"a bare 404 is ambiguous and must stay on the conservative branch")
}

func TestPinnedOCIRevision(t *testing.T) {
	for _, tc := range []struct {
		revision, version string
		tag, digest       string
		ok                bool
	}{
		{"1.0.0@sha256:aaa", "", "1.0.0", "sha256:aaa", true},
		{"1.0.0@sha256:aaa", "1.0.0", "1.0.0", "sha256:aaa", true},
		{"1.0.0@sha256:aaa", "1.1.0", "", "", false}, // the pin describes another artifact
		{"", "", "", "", false},
		{"1.0.0", "", "", "", false},         // no digest half
		{"@sha256:aaa", "", "", "", false},   // no tag half
		{"1.0.0@md5:aaa", "", "", "", false}, // not a digest a registry can be asked for
		{"1.0.0@sha256:", "", "1.0.0", "sha256:", true},
	} {
		tag, digest, ok := pinnedOCIRevision(tc.revision, tc.version)
		assert.Equal(t, tc.ok, ok, "pinnedOCIRevision(%q, %q)", tc.revision, tc.version)
		assert.Equal(t, tc.tag, tag)
		assert.Equal(t, tc.digest, digest)
	}
}

func TestResolveOCITagUsesPinnedVersion(t *testing.T) {
	tag, err := resolveOCITag(context.Background(), "reg.example.com/charts/widget", "reg.example.com", "", "", "2.3.4")
	require.NoError(t, err)
	assert.Equal(t, "2.3.4", tag, "a pinned version is used as-is, without asking the registry")
}

func TestOCICallsHonourTheRateLimitGate(t *testing.T) {
	t.Cleanup(ResetRateLimitGate)
	const host = "gated.example.invalid"
	sourceRateLimit.trip(host, time.Now().Add(time.Hour))
	ctx := context.Background()
	repoRef := host + "/charts/widget"

	var limited *RateLimitedError
	_, err := PullOCIChart(ctx, repoRef+":1.0.0", host, "", "")
	assert.ErrorAs(t, err, &limited)
	_, err = PullOCIChartWithPlainHTTP(ctx, repoRef+":1.0.0", host, "", "")
	assert.ErrorAs(t, err, &limited)
	_, err = ListOCITags(ctx, repoRef, host, "", "")
	assert.ErrorAs(t, err, &limited)
	_, err = ListOCITagsWithPlainHTTP(ctx, repoRef, host, "", "")
	assert.ErrorAs(t, err, &limited)
	_, err = OCIManifestDigest(ctx, repoRef, host, "", "", "1.0.0", "", false)
	assert.ErrorAs(t, err, &limited)
}

// tagListServer is a plain-HTTP registry that answers the tag listing for
// charts/widget and records how often it was asked.
func tagListServer(t *testing.T, status int, tags []string) (host string, requests *atomic.Int32) {
	t.Helper()
	requests = &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/charts/widget/tags/list" {
			http.NotFound(w, r)
			return
		}
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusOK {
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "charts/widget", "tags": tags})
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(ResetRateLimitGate)
	t.Cleanup(ResetOCIClientCache)
	return strings.TrimPrefix(srv.URL, "http://"), requests
}

func TestListOCITagsOverPlainHTTP(t *testing.T) {
	host, requests := tagListServer(t, http.StatusOK, []string{"0.9.0", "1.0.0", "latest", "1.0.0-rc.1", "not-a-version"})
	repoRef := host + "/charts/widget"

	tags, err := ListOCITagsWithPlainHTTP(context.Background(), repoRef, host, "", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"1.0.0", "1.0.0-rc.1", "0.9.0"}, tags, "semver tags only, highest first")
	assert.EqualValues(t, 1, requests.Load())

	tag, err := resolveOCITagWithTransport(context.Background(), repoRef, host, "", "", "", true)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", tag, "an empty version resolves to the highest semver tag")
}

func TestResolveOCITagNeedsASemverTag(t *testing.T) {
	host, _ := tagListServer(t, http.StatusOK, []string{"latest", "dev"})
	repoRef := host + "/charts/widget"

	_, err := resolveOCITagWithTransport(context.Background(), repoRef, host, "", "", "", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no semver tags found for OCI repository "+repoRef)
}

func TestListOCITagsHoldsAThrottledRegistry(t *testing.T) {
	host, requests := tagListServer(t, http.StatusTooManyRequests, nil)
	repoRef := host + "/charts/widget"

	_, err := ListOCITagsWithPlainHTTP(context.Background(), repoRef, host, "", "")
	var limited *RateLimitedError
	require.ErrorAs(t, err, &limited, "a 429 from the registry client is classified as throttling")
	assert.Equal(t, host, limited.Source)

	_, err = ListOCITagsWithPlainHTTP(context.Background(), repoRef, host, "", "")
	require.ErrorAs(t, err, &limited)
	assert.EqualValues(t, 1, requests.Load(), "the second call is answered by the gate, not the registry")

	_, err = resolveOCITagWithTransport(context.Background(), repoRef, host, "", "", "", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list tags for OCI repository")
	assert.ErrorIs(t, err, ErrRateLimit)
}
