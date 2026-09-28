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
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"oras.land/oras-go/pkg/registry/remote/auth"
)

// The manifest digest is OCI's ETag. A registry answers HEAD
// /v2/<repo>/manifests/<tag> with Docker-Content-Digest, the sha256 of the
// manifest bytes, and serves the same value as the response ETag -- so
// If-None-Match against the digest we already hold gets 304 Not Modified and
// no body. It is a stronger key than git's commit SHA in one way and weaker in
// another: it is content-addressed, so it changes exactly when the pushed
// artifact changes and never otherwise, but a tag is mutable, so the digest
// behind a pinned version can move when someone re-pushes the same tag. That is
// precisely what this check catches, and it catches it for one small request
// instead of pulling the chart.
//
// This is the same for every registry that speaks the distribution spec: ECR,
// Docker Hub, GHCR, GAR, Harbor, ACR. Pulls are what those registries meter --
// ECR throttles on its API rate, Docker Hub counts manifest requests against
// the pull limit -- so replacing a pull per reconcile with a conditional HEAD
// is the difference that matters there too.
const (
	// ociManifestAccept lists every manifest media type a chart may be stored
	// as, so the registry does not answer 404 for a manifest it holds under a
	// media type we forgot to ask for.
	ociManifestAccept = "application/vnd.oci.image.manifest.v1+json," +
		"application/vnd.oci.image.index.v1+json," +
		"application/vnd.docker.distribution.manifest.v2+json," +
		"application/vnd.docker.distribution.manifest.list.v2+json"

	// headerContentDigest carries the manifest digest. Every distribution-spec
	// registry sets it on a manifest response.
	headerContentDigest = "Docker-Content-Digest"
)

// ociRevisionTimeout bounds the conditional HEAD. It is meant to be a cheap
// probe on the reconcile path, so it fails fast rather than holding a worker.
const ociRevisionTimeout = 15 * time.Second

// OCIManifestDigest returns the digest that tag currently resolves to in the
// repository, without pulling the artifact.
//
// lastKnown is the digest the caller already holds, or empty. When it is still
// current the registry answers 304 and lastKnown is returned unchanged, which
// costs no bandwidth and, on registries that meter by pull, nothing at all.
func OCIManifestDigest(ctx context.Context, repoRef, host, username, password, tag, lastKnown string, plainHTTP bool) (string, error) {
	if err := sourceRateLimit.blocked(host); err != nil {
		return "", err
	}
	return AwaitOCICall(ctx, func() (string, error) {
		return ociManifestDigest(ctx, repoRef, host, username, password, tag, lastKnown, plainHTTP)
	})
}

func ociManifestDigest(ctx context.Context, repoRef, host, username, password, tag, lastKnown string, plainHTTP bool) (string, error) {
	scheme := "https"
	if plainHTTP {
		scheme = "http"
	}
	// repoRef is "<host>/<path>"; the manifest URL wants the path alone.
	repoPath := strings.TrimPrefix(repoRef, host+"/")
	// The tag comes from a caller's version parameter. Go sends EscapedPath()
	// with dot segments intact, so an unescaped one would point this HEAD
	// somewhere else on the registry and key the cache on whatever answered.
	manifestURL := fmt.Sprintf("%s://%s/v2/%s/manifests/%s", scheme, host, repoPath, url.PathEscape(tag))

	reqCtx, cancel := context.WithTimeout(ctx, ociRevisionTimeout)
	defer cancel()
	// The scope hint spares a second round trip: without it the client learns
	// which token it needs only from the challenge this request comes back with.
	reqCtx = auth.WithScopes(reqCtx, auth.ScopeRepository(repoPath, auth.ActionPull))

	req, err := http.NewRequestWithContext(reqCtx, http.MethodHead, manifestURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", ociManifestAccept)
	if lastKnown != "" {
		req.Header.Set("If-None-Match", quoteETag(lastKnown))
	}

	client := &auth.Client{
		// A cache of this client's own, not auth.DefaultCache: that one is
		// keyed by host, scheme and scopes alone, so two registries on the
		// same host with different credentials would share a bearer token.
		Cache: auth.NewCache(),
		Credential: func(context.Context, string) (auth.Credential, error) {
			if username == "" && password == "" {
				return auth.EmptyCredential, nil
			}
			// A registry given only a token treats it as a bearer secret; ECR
			// and Docker Hub both also accept it as the password half of a
			// basic credential, which is how the pull path already sends it.
			return auth.Credential{Username: username, Password: password}, nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read manifest of %s:%s", repoRef, tag)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusNotModified:
		return lastKnown, nil
	case http.StatusOK:
		if digest := resp.Header.Get(headerContentDigest); digest != "" {
			return digest, nil
		}
		// A registry that omits the digest header still sends the ETag, which
		// for a manifest is the digest.
		if etag := unquoteETag(resp.Header.Get("ETag")); etag != "" {
			return etag, nil
		}
		return "", errors.Errorf("registry did not report a digest for %s:%s", repoRef, tag)
	case http.StatusTooManyRequests:
		until := retryAfter(resp)
		sourceRateLimit.trip(host, until)
		return "", &RateLimitedError{Source: host, Until: until}
	default:
		return "", errors.Errorf("failed to read manifest of %s:%s: %s", repoRef, tag, resp.Status)
	}
}

// retryAfter reads the registry's Retry-After, falling back to a conservative
// hold when it does not send one.
//
// The header is either a whole number of seconds or an HTTP date. Seconds are
// parsed as an integer rather than by appending "s" and calling
// ParseDuration: that reads a "2m" as "2ms", a two-millisecond hold, instead
// of letting the date parser have it.
func retryAfter(resp *http.Response) time.Time {
	if v := strings.TrimSpace(resp.Header.Get("Retry-After")); v != "" {
		if seconds, err := strconv.Atoi(v); err == nil && seconds > 0 {
			return time.Now().Add(time.Duration(seconds) * time.Second)
		}
		if at, err := http.ParseTime(v); err == nil {
			return at
		}
	}
	return time.Now().Add(defaultRateLimitHold)
}

func quoteETag(v string) string {
	if strings.HasPrefix(v, `"`) {
		return v
	}
	return `"` + v + `"`
}

func unquoteETag(v string) string {
	return strings.Trim(strings.TrimPrefix(v, "W/"), `"`)
}

// ociThrottleMarkers are how the registries answer "you are asking too often".
// The distribution spec says 429 TOOMANYREQUESTS, Docker Hub says it in prose
// when the pull limit is hit, and ECR raises its own throttling exception --
// and all of it arrives here as an opaque error from the Helm registry client,
// which does not keep the status code.
// A bare "429" is deliberately absent. Pull failures carry the resolved
// digest, and those three digits appear somewhere in a random 64-hex digest
// about 1.5% of the time -- and always in a host port like :5429 or a tag like
// v1429. Matching that rewrites a genuine failure (a missing layer, a bad
// media type, an auth refusal) as a rate limit, discards its real cause, and
// gates the whole registry host, blocking every unrelated addon and module on
// it. So the digits are only trusted with a status word next to them.
var ociThrottleMarkers = []string{
	"toomanyrequests",
	"too many requests",
	"http status 429",
	"status code 429",
	"statuscode: 429",
	"status: 429",
	"throttl",         // ECR ThrottlingException, Harbor "throttled"
	"rate exceeded",   // ECR
	"pull rate limit", // Docker Hub
	"requested access to the resource is denied due to rate",
}

// IsOCIThrottledError reports whether the registry refused because the caller
// is asking too often, rather than because of what it asked for.
func IsOCIThrottledError(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := rateLimitHold(err); ok {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range ociThrottleMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// holdOCIThrottle records a throttling refusal against the registry host so the
// next call is answered without spending another request on it. Any other error
// is returned untouched.
//
// The hold is the conservative default rather than a reset time: unlike
// GitHub, these registries rarely say when the limit clears, and the error has
// usually lost the Retry-After header by the time it reaches here.
func holdOCIThrottle(host string, err error) error {
	if !IsOCIThrottledError(err) {
		return err
	}
	if until, ok := rateLimitHold(err); ok {
		sourceRateLimit.trip(host, until)
		return &RateLimitedError{Source: host, Until: until}
	}
	until := time.Now().Add(defaultRateLimitHold)
	sourceRateLimit.trip(host, until)
	return &RateLimitedError{Source: host, Until: until}
}
