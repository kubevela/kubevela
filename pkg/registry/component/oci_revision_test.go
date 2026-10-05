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
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requestRecorder keeps the last request a handler saw, behind a mutex: the
// handler runs on the server's goroutine and the test reads on its own.
type requestRecorder struct {
	mu   sync.Mutex
	last *http.Request
}

func (rec *requestRecorder) record(r *http.Request) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.last = r.Clone(context.Background())
}

// request returns the last recorded request, failing the test if there was none.
func (rec *requestRecorder) request(t *testing.T) *http.Request {
	t.Helper()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	require.NotNil(t, rec.last, "the registry was never asked")
	return rec.last
}

// manifestServer is a plain-HTTP stand-in for a distribution-spec registry
// that only answers manifest HEADs. It returns the server, the host the
// probe must be pointed at, and the repository reference under that host.
func manifestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, string, string) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	t.Cleanup(ResetRateLimitGate)
	host := strings.TrimPrefix(srv.URL, "http://")
	return srv, host, host + "/charts/widget"
}

func TestOCIManifestDigestReadsDigestHeader(t *testing.T) {
	rec := &requestRecorder{}
	_, host, repoRef := manifestServer(t, func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set(headerContentDigest, "sha256:aaa")
		w.WriteHeader(http.StatusOK)
	})

	digest, err := OCIManifestDigest(context.Background(), repoRef, host, "", "", "1.0.0", "", true)
	require.NoError(t, err)
	assert.Equal(t, "sha256:aaa", digest)

	seen := rec.request(t)
	assert.Equal(t, http.MethodHead, seen.Method, "a revision probe must not pull the manifest body")
	assert.Equal(t, "/v2/charts/widget/manifests/1.0.0", seen.URL.Path)
	assert.Equal(t, ociManifestAccept, seen.Header.Get("Accept"))
	assert.Empty(t, seen.Header.Get("If-None-Match"), "nothing known yet, so nothing to revalidate")
}

func TestOCIManifestDigestRevalidatesWithLastKnown(t *testing.T) {
	rec := &requestRecorder{}
	_, host, repoRef := manifestServer(t, func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.WriteHeader(http.StatusNotModified)
	})

	digest, err := OCIManifestDigest(context.Background(), repoRef, host, "", "", "1.0.0", "sha256:aaa", true)
	require.NoError(t, err)
	assert.Equal(t, "sha256:aaa", digest, "304 means the digest the caller holds is still current")
	assert.Equal(t, `"sha256:aaa"`, rec.request(t).Header.Get("If-None-Match"), "the digest is offered as a quoted ETag")
}

func TestOCIManifestDigestFallsBackToETag(t *testing.T) {
	_, host, repoRef := manifestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `W/"sha256:bbb"`)
		w.WriteHeader(http.StatusOK)
	})

	digest, err := OCIManifestDigest(context.Background(), repoRef, host, "", "", "1.0.0", "", true)
	require.NoError(t, err)
	assert.Equal(t, "sha256:bbb", digest, "a weak, quoted ETag is unwrapped to the digest")
}

func TestOCIManifestDigestRefusesAnswerWithoutDigest(t *testing.T) {
	_, host, repoRef := manifestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	_, err := OCIManifestDigest(context.Background(), repoRef, host, "", "", "1.0.0", "", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not report a digest")
}

func TestOCIManifestDigestReportsOtherStatuses(t *testing.T) {
	_, host, repoRef := manifestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := OCIManifestDigest(context.Background(), repoRef, host, "", "", "1.0.0", "", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.Contains(t, err.Error(), repoRef+":1.0.0")
}

func TestOCIManifestDigestEscapesTheTag(t *testing.T) {
	rec := &requestRecorder{}
	_, host, repoRef := manifestServer(t, func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set(headerContentDigest, "sha256:aaa")
		w.WriteHeader(http.StatusOK)
	})

	_, err := OCIManifestDigest(context.Background(), repoRef, host, "", "", "v1 beta", "", true)
	require.NoError(t, err)
	assert.Equal(t, "/v2/charts/widget/manifests/v1%20beta", rec.request(t).RequestURI,
		"a tag off an Application spec must not be able to redirect the probe")
}

func TestOCIManifestDigestSendsCredentialsOnChallenge(t *testing.T) {
	var requests atomic.Int32
	rec := &requestRecorder{}
	_, host, repoRef := manifestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		rec.record(r)
		w.Header().Set(headerContentDigest, "sha256:ccc")
		w.WriteHeader(http.StatusOK)
	})

	digest, err := OCIManifestDigest(context.Background(), repoRef, host, "AWS", "token", "1.0.0", "", true)
	require.NoError(t, err)
	assert.Equal(t, "sha256:ccc", digest)
	assert.EqualValues(t, 2, requests.Load(), "the challenge costs one extra round trip")
	assert.True(t, strings.HasPrefix(rec.request(t).Header.Get("Authorization"), "Basic "), "the token is sent as the password half of a basic credential")
}

func TestOCIManifestDigestTripsTheGateOn429(t *testing.T) {
	var requests atomic.Int32
	_, host, repoRef := manifestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	before := time.Now()
	_, err := OCIManifestDigest(context.Background(), repoRef, host, "", "", "1.0.0", "", true)
	var limited *RateLimitedError
	require.ErrorAs(t, err, &limited)
	assert.Equal(t, host, limited.Source)
	assert.WithinRange(t, limited.Until, before.Add(119*time.Second), time.Now().Add(121*time.Second))

	// The next probe is answered locally, without spending a request.
	_, err = OCIManifestDigest(context.Background(), repoRef, host, "", "", "1.0.0", "", true)
	require.ErrorAs(t, err, &limited)
	assert.EqualValues(t, 1, requests.Load(), "a registry that said to wait is not asked again")
}

func TestOCIManifestDigestReturnsEarlyWhenCancelled(t *testing.T) {
	var requests atomic.Int32
	_, host, repoRef := manifestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := OCIManifestDigest(ctx, repoRef, host, "", "", "1.0.0", "", true)
	assert.ErrorIs(t, err, context.Canceled)
	assert.EqualValues(t, 0, requests.Load(), "an already cancelled caller must not start a request")
}

func TestRetryAfter(t *testing.T) {
	header := func(v string) *http.Response {
		resp := &http.Response{Header: http.Header{}}
		if v != "" {
			resp.Header.Set("Retry-After", v)
		}
		return resp
	}

	before := time.Now()
	assert.WithinRange(t, retryAfter(header("30")), before.Add(29*time.Second), time.Now().Add(31*time.Second))

	at := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	assert.True(t, retryAfter(header(at.Format(http.TimeFormat))).Equal(at), "an HTTP date is honoured as given")

	for _, v := range []string{"", "0", "-5", "2m", "soon"} {
		got := retryAfter(header(v))
		assert.WithinRange(t, got, before.Add(defaultRateLimitHold-time.Second), time.Now().Add(defaultRateLimitHold+time.Second),
			"Retry-After %q must fall back to the default hold", v)
	}
}

func TestETagQuoting(t *testing.T) {
	assert.Equal(t, `"sha256:aaa"`, quoteETag("sha256:aaa"))
	assert.Equal(t, `"sha256:aaa"`, quoteETag(`"sha256:aaa"`), "an already quoted value is left alone")
	assert.Equal(t, "sha256:aaa", unquoteETag(`"sha256:aaa"`))
	assert.Equal(t, "sha256:aaa", unquoteETag(`W/"sha256:aaa"`))
	assert.Equal(t, "sha256:aaa", unquoteETag("sha256:aaa"))
	assert.Equal(t, "", unquoteETag(""))
}

func TestIsOCIThrottledError(t *testing.T) {
	assert.False(t, IsOCIThrottledError(nil))
	assert.True(t, IsOCIThrottledError(&RateLimitedError{Source: "ghcr.io"}), "an error this package already classified counts")
	for _, msg := range []string{
		"GET \"https://x/v2/a/manifests/1\": response status code 429: Too Many Requests",
		"TOOMANYREQUESTS: You have reached your pull rate limit",
		"ThrottlingException: Rate exceeded",
		"toomanyrequests: requested access to the resource is denied due to rate limiting",
		"pull failed: HTTP status 429",
	} {
		assert.True(t, IsOCIThrottledError(errors.New(msg)), "%q must read as throttling", msg)
	}
	for _, msg := range []string{
		"failed to pull chart sha256:4291f00d: manifest unknown",
		"dial tcp 10.0.0.1:5429: connection refused",
		"tag v1429 not found",
		"unauthorized: authentication required",
	} {
		assert.False(t, IsOCIThrottledError(errors.New(msg)), "%q must not read as throttling", msg)
	}
}

func TestHoldOCIThrottle(t *testing.T) {
	t.Cleanup(ResetRateLimitGate)

	plain := errors.New("manifest unknown")
	assert.Same(t, plain, holdOCIThrottle("reg.example.com", plain), "a non-throttling error is returned untouched")
	assert.NoError(t, sourceRateLimit.blocked("reg.example.com"))

	before := time.Now()
	err := holdOCIThrottle("reg.example.com", errors.New("response status code 429: Too Many Requests"))
	var limited *RateLimitedError
	require.ErrorAs(t, err, &limited)
	assert.Equal(t, "reg.example.com", limited.Source)
	assert.WithinRange(t, limited.Until, before.Add(defaultRateLimitHold-time.Second), time.Now().Add(defaultRateLimitHold+time.Second))
	assert.Error(t, sourceRateLimit.blocked("reg.example.com"), "the host is gated for the default hold")

	// A refusal that already carries a deadline keeps it.
	until := time.Now().Add(30 * time.Minute)
	err = holdOCIThrottle("other.example.com", &RateLimitedError{Source: "other.example.com", Until: until})
	require.ErrorAs(t, err, &limited)
	assert.True(t, limited.Until.Equal(until))
}
