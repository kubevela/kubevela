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
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sync/singleflight"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/registry"
)

// ociScheme is the canonical OCI URL scheme prefix. IsOCIURL classifies the
// scheme case-insensitively, so the stripping in ociRegistryLocation has to
// match, or a registry stored as "OCI://..." would classify as OCI but build a
// malformed host such as "OCI:".
const ociScheme = "oci://"

// OCIRegistryLocation returns the registry host and repository prefix. Any of
// the schemes a registry URL is written with is stripped first: without that,
// an "http://" URL splits at the scheme's own slash and yields the host
// "http:".
// AwaitOCICall runs a blocking helm registry operation and returns as soon as
// ctx is done, so a cancelled caller is released even though the helm registry
// client itself takes no context. The operation keeps running in the
// background; its HTTP client timeout is what eventually reclaims it.
func AwaitOCICall[T any](ctx context.Context, op func() (T, error)) (T, error) {
	if err := ctx.Err(); err != nil {
		// A caller that arrives already cancelled must not start a fresh
		// goroutine that can run for up to ociCallTimeout: under frequent
		// reconcile cancellation that accumulates orphaned goroutines and
		// connections for no benefit, since the result would be discarded
		// immediately below anyway.
		var zero T
		return zero, err
	}
	type outcome struct {
		value T
		err   error
	}
	done := make(chan outcome, 1)
	go func() {
		value, err := op()
		done <- outcome{value: value, err: err}
	}()
	select {
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	case res := <-done:
		return res.value, res.err
	}
}

// ociCallTimeout bounds a single registry call.
const ociCallTimeout = 5 * time.Minute

func OCIRegistryLocation(rawURL string) (host, prefix string) {
	base := rawURL
	for _, scheme := range []string{ociScheme, "https://", "http://"} {
		if len(base) >= len(scheme) && strings.EqualFold(base[:len(scheme)], scheme) {
			base = base[len(scheme):]
			break
		}
	}
	base = strings.Trim(base, "/")
	host = base
	if i := strings.Index(base, "/"); i >= 0 {
		host = base[:i]
		prefix = strings.Trim(base[i+1:], "/")
	}
	return host, prefix
}

// OCIRepoRef builds the OCI repository reference (no tag) and host from a
// registry URL and addon name. The URL may carry an "oci://" scheme and/or a
// trailing slash. The host is the registry authority (everything before the
// first path separator), used for login.
func OCIRepoRef(url, name string) (repoRef, host string) {
	host, prefix := OCIRegistryLocation(url)
	repoRef = host
	if prefix != "" {
		repoRef += "/" + prefix
	}
	repoRef += "/" + strings.TrimPrefix(name, "/")
	return repoRef, host
}

// ociClientCache reuses a logged-in registry client across calls.
//
// Every OCI operation funnels through newOCIClientWithPlainHTTP, so without
// this a single listing costs one TLS handshake and one login per addon: the
// catalog is enumerated, then each entry is resolved and its versions listed.
// Against a real registry that fails once the catalog holds more than a couple
// of addons, with connection resets and TLS handshake timeouts, and it gets
// worse as more addons are published.
//
// The key includes the credentials, so a rotated password (an ECR login token
// lasts 12 hours) yields a new client rather than reusing a stale one.
var ociClientCache = struct {
	sync.Mutex
	clients map[string]*registry.Client
}{clients: map[string]*registry.Client{}}

// ociClientCacheLimit bounds the cache. Entries are keyed by credentials, so the
// live set is small; the cap only stops unbounded growth as tokens rotate.
const ociClientCacheLimit = 16

// ociClientCreation coordinates concurrent creation of the same cache entry so
// two goroutines resolving the same registry at once do not both dial and log
// in. Keying it by cache key, rather than using ociClientCache's own lock for
// this, keeps unrelated keys from blocking on each other's network I/O.
var ociClientCreation singleflight.Group

func ociClientCacheKey(host, username, password string, plainHTTP bool) string {
	sum := sha256.Sum256([]byte(username + "\x00" + password))
	return fmt.Sprintf("%s|%t|%x", host, plainHTTP, sum[:8])
}

// ResetOCIClientCache empties the logged-in client cache. It exists for tests in
// packages that drive the OCI transport from outside this one: the cache is
// process-wide, so a test that builds clients would otherwise leak them into
// whichever test ran next.
func ResetOCIClientCache() {
	ociClientCache.Lock()
	defer ociClientCache.Unlock()
	ociClientCache.clients = map[string]*registry.Client{}
}

func cachedOCIClient(key string) (*registry.Client, bool) {
	ociClientCache.Lock()
	defer ociClientCache.Unlock()
	client, ok := ociClientCache.clients[key]
	return client, ok
}

func storeOCIClient(key string, client *registry.Client) {
	ociClientCache.Lock()
	defer ociClientCache.Unlock()
	if len(ociClientCache.clients) >= ociClientCacheLimit {
		// Cheap eviction: the entries are interchangeable, and a dropped one is
		// only re-logged-in on next use.
		for k := range ociClientCache.clients {
			delete(ociClientCache.clients, k)
			break
		}
	}
	ociClientCache.clients[key] = client
}

func NewOCIClientWithPlainHTTP(host, username, password string, plainHTTP bool) (*registry.Client, error) {
	key := ociClientCacheKey(host, username, password, plainHTTP)

	if client, ok := cachedOCIClient(key); ok {
		return client, nil
	}

	// The dial and login below run outside ociClientCache's lock: only the map
	// reads and writes hold it, so a slow or unreachable registry blocks
	// nothing but callers asking for this same key.
	result, err, _ := ociClientCreation.Do(key, func() (interface{}, error) {
		if client, ok := cachedOCIClient(key); ok {
			return client, nil
		}

		// The client's own HTTP timeout is what actually terminates a request
		// abandoned by AwaitOCICall; without it a hung registry would keep the
		// orphaned goroutine, its connection, and its buffered result alive for
		// as long as the process runs.
		opts := []registry.ClientOption{registry.ClientOptHTTPClient(&http.Client{Timeout: ociCallTimeout})}
		if plainHTTP {
			opts = append(opts, registry.ClientOptPlainHTTP())
		}
		client, err := registry.NewClient(opts...)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create OCI registry client")
		}
		if username != "" || password != "" {
			if err := client.Login(host, registry.LoginOptBasicAuth(username, password)); err != nil {
				return nil, errors.Wrapf(err, "failed to login to OCI registry %s", host)
			}
		}

		storeOCIClient(key, client)
		return client, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*registry.Client), nil
}

// PullOCIChart is the production puller: it logs in (when credentials are set)
// and pulls the chart layer from the OCI registry via the Helm registry client.
func PullOCIChart(ctx context.Context, ref, host, username, password string) ([]byte, error) {
	return AwaitOCICall(ctx, func() ([]byte, error) {
		return PullOCIChartWithTransport(ref, host, username, password, false)
	})
}

func PullOCIChartWithPlainHTTP(ctx context.Context, ref, host, username, password string) ([]byte, error) {
	return AwaitOCICall(ctx, func() ([]byte, error) {
		return PullOCIChartWithTransport(ref, host, username, password, true)
	})
}

func PullOCIChartWithTransport(ref, host, username, password string, plainHTTP bool) ([]byte, error) {
	client, err := NewOCIClientWithPlainHTTP(host, username, password, plainHTTP)
	if err != nil {
		return nil, err
	}
	result, err := client.Pull(ref, registry.PullOptWithChart(true))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to pull addon chart %s", ref)
	}
	if result == nil || result.Chart == nil || len(result.Chart.Data) == 0 {
		return nil, errors.Errorf("addon chart %s has no chart layer", ref)
	}
	return result.Chart.Data, nil
}

// ListOCITags lists the repository's semver tags (highest first) via the Helm
// registry client, which filters non-semver tags and sorts descending.
func ListOCITags(ctx context.Context, repoRef, host, username, password string) ([]string, error) {
	return AwaitOCICall(ctx, func() ([]string, error) {
		return ListOCITagsWithTransport(repoRef, host, username, password, false)
	})
}

func ListOCITagsWithPlainHTTP(ctx context.Context, repoRef, host, username, password string) ([]string, error) {
	return AwaitOCICall(ctx, func() ([]string, error) {
		return ListOCITagsWithTransport(repoRef, host, username, password, true)
	})
}

func ListOCITagsWithTransport(repoRef, host, username, password string, plainHTTP bool) ([]string, error) {
	client, err := NewOCIClientWithPlainHTTP(host, username, password, plainHTTP)
	if err != nil {
		return nil, err
	}
	return client.Tags(repoRef)
}

// ociErrCodeNameUnknown is how the OCI distribution spec reports a repository
// that does not exist. oras-go renders the code by lowercasing it and turning
// underscores into spaces (NAME_UNKNOWN -> "name unknown").
const ociErrCodeNameUnknown = "name unknown"

// IsOCIRepositoryAbsentError reports whether err is a registry answer confirming
// "this repository does not exist", as opposed to "this repository could not be
// read". Only the first lets the caller publish a catalog, because there is
// nothing to preserve; misreading the second rebuilds the catalog from an empty
// list and drops every addon already published.
//
// The test is the NAME_UNKNOWN error code, not the 404 status. A bare 404 is
// ambiguous -- a proxy, a gateway, or a registry that does not serve the
// tag-list route answers the same way for a repository that does exist -- so it
// stays on the conservative branch.
//
// The code has to be read out of the message: oras-go v1.2.5 builds these errors
// with fmt.Errorf and keeps its error types in the unexported
// pkg/registry/remote/internal/errutil. A miss is safe in the same direction --
// callers refuse to rewrite the catalog.
func IsOCIRepositoryAbsentError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), ociErrCodeNameUnknown)
}

// IsDockerHubHost reports whether host is a known Docker Hub registry alias.
// Mirrors the alias list in pkg/cue/cuex/providers/helm/auth.go's
// normalizeDockerHubAliases; kept here rather than imported because that
// package is a different, heavier dependency (CUE #Helm chart-fetch auth)
// that the registry code does not otherwise need.
//
// It lives with the generic OCI primitives rather than in pkg/addon because
// which hosts are Docker Hub is a property of the registry, not of addons:
// the module publish and fetch paths reach the same registries.
func IsDockerHubHost(host string) bool {
	h := strings.ToLower(host)
	if i := strings.IndexByte(h, ':'); i >= 0 {
		h = h[:i]
	}
	switch h {
	case "docker.io", "index.docker.io", "registry-1.docker.io":
		return true
	}
	return false
}

// resolveOCITag returns the tag to pull. A pinned version is used as-is; an
// empty version resolves to the highest semver tag published in the repository.
func resolveOCITag(ctx context.Context, repoRef, host, username, password, version string) (string, error) {
	if version != "" {
		return version, nil
	}
	tags, err := ListOCITags(ctx, repoRef, host, username, password)
	if err != nil {
		return "", errors.Wrapf(err, "failed to list tags for OCI repository %s", repoRef)
	}
	if len(tags) == 0 {
		return "", errors.Errorf("no semver tags found for OCI repository %s; push a versioned tag or pin an explicit version", repoRef)
	}
	// helm's Tags returns semver-filtered, highest-first.
	return tags[0], nil
}

// PullOCIChartFiles pulls the Helm-chart artifact for name[:version] (an empty
// version resolves the highest semver tag) and returns its files, paths
// prefixed by the chart name.
func PullOCIChartFiles(ctx context.Context, reg Registry, name, version string) ([]*loader.BufferedFile, error) {
	oci := reg.OCIChartSource()
	if oci == nil {
		return nil, errors.Errorf("registry %q is not an OCI registry", reg.Name)
	}
	repoRef, host := OCIRepoRef(oci.URL, name)
	tag, err := resolveOCITag(ctx, repoRef, host, oci.Username, oci.Token, version)
	if err != nil {
		return nil, err
	}
	ref := fmt.Sprintf("%s:%s", repoRef, tag)
	archive, err := PullOCIChart(ctx, ref, host, oci.Username, oci.Token)
	if err != nil {
		return nil, err
	}
	files, err := loader.LoadArchiveFiles(bytes.NewReader(archive))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load chart archive %s", ref)
	}
	return files, nil
}
