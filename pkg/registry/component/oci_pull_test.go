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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart/loader"
	helmregistry "helm.sh/helm/v3/pkg/registry"
)

// fakeOCIRegistry is a read-only, plain-HTTP registry speaking enough of the
// distribution spec for the Helm registry client to resolve a tag, list tags
// and pull a chart: manifests by tag or digest, blobs by digest, and the tag
// list. Uploads are refused with a bare 404, which is how ECR answers a push
// to a repository that does not exist.
type fakeOCIRegistry struct {
	srv  *httptest.Server
	repo string

	mu        sync.Mutex
	blobs     map[string][]byte
	manifests map[string]ociManifest // keyed by tag and by digest
	tags      []string

	manifestRequests atomic.Int32
	tagListRequests  atomic.Int32
}

type ociManifest struct {
	data      []byte
	digest    string
	mediaType string
}

func newFakeOCIRegistry(t *testing.T, repo string) *fakeOCIRegistry {
	t.Helper()
	reg := &fakeOCIRegistry{repo: repo, blobs: map[string][]byte{}, manifests: map[string]ociManifest{}}
	reg.srv = httptest.NewServer(http.HandlerFunc(reg.serve))
	t.Cleanup(reg.srv.Close)
	t.Cleanup(ResetRateLimitGate)
	t.Cleanup(ResetOCIClientCache)
	return reg
}

func (f *fakeOCIRegistry) host() string { return strings.TrimPrefix(f.srv.URL, "http://") }

func (f *fakeOCIRegistry) repoRef() string { return f.host() + "/" + f.repo }

// registry is the Registry record a caller would store for this server: a
// plain-HTTP OCI endpoint whose prefix is the repository's parent path.
func (f *fakeOCIRegistry) registry() Registry {
	prefix := ""
	if i := strings.LastIndex(f.repo, "/"); i >= 0 {
		prefix = "/" + f.repo[:i]
	}
	return Registry{Name: "fake", Helm: &HelmSource{URL: "http://" + f.host() + prefix}}
}

func sha256Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// publish stores archive under tag as a Helm chart artifact and returns the
// manifest digest the registry will report for it.
func (f *fakeOCIRegistry) publish(t *testing.T, tag string, archive []byte, meta map[string]any) string {
	t.Helper()
	config, err := json.Marshal(meta)
	require.NoError(t, err)
	manifest, err := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.manifest.v1+json",
		"config": map[string]any{
			"mediaType": helmregistry.ConfigMediaType,
			"digest":    sha256Digest(config),
			"size":      len(config),
		},
		"layers": []map[string]any{{
			"mediaType": helmregistry.ChartLayerMediaType,
			"digest":    sha256Digest(archive),
			"size":      len(archive),
		}},
	})
	require.NoError(t, err)

	f.mu.Lock()
	defer f.mu.Unlock()
	f.blobs[sha256Digest(config)] = config
	f.blobs[sha256Digest(archive)] = archive
	entry := ociManifest{data: manifest, digest: sha256Digest(manifest), mediaType: "application/vnd.oci.image.manifest.v1+json"}
	f.manifests[tag] = entry
	f.manifests[entry.digest] = entry
	f.tags = append(f.tags, tag)
	return entry.digest
}

func (f *fakeOCIRegistry) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v2/" {
		w.WriteHeader(http.StatusOK)
		return
	}
	rest, ok := strings.CutPrefix(r.URL.Path, "/v2/"+f.repo+"/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case rest == "tags/list":
		f.tagListRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": f.repo, "tags": f.tags})
	case strings.HasPrefix(rest, "manifests/"):
		f.manifestRequests.Add(1)
		entry, found := f.manifests[strings.TrimPrefix(rest, "manifests/")]
		if !found {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", entry.mediaType)
		w.Header().Set(headerContentDigest, entry.digest)
		w.Header().Set("Content-Length", strconv.Itoa(len(entry.data)))
		if r.Method != http.MethodHead {
			_, _ = w.Write(entry.data)
		}
	case strings.HasPrefix(rest, "blobs/uploads"):
		http.NotFound(w, r)
	case strings.HasPrefix(rest, "blobs/"):
		data, found := f.blobs[strings.TrimPrefix(rest, "blobs/")]
		if !found {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		if r.Method != http.MethodHead {
			_, _ = w.Write(data)
		}
	default:
		http.NotFound(w, r)
	}
}

// chartArchive builds a minimal packaged chart: the gzipped tarball
// `helm package` would write, holding Chart.yaml and values.yaml under the
// chart name.
func chartArchive(t *testing.T, name, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	files := map[string]string{
		name + "/Chart.yaml":  fmt.Sprintf("apiVersion: v2\nname: %s\nversion: %s\n", name, version),
		name + "/values.yaml": "replicas: 1\n",
	}
	for path, content := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: path, Mode: 0o644, Size: int64(len(content))}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func chartMeta(name, version string) map[string]any {
	return map[string]any{"apiVersion": "v2", "name": name, "version": version}
}

// chartVersionOf reads the version out of the pulled Chart.yaml, which is how
// the tests tell one published artifact from another.
func chartVersionOf(t *testing.T, files []*loader.BufferedFile) string {
	t.Helper()
	for _, f := range files {
		// loader.LoadArchiveFiles strips the chart directory off every path.
		if f.Name == "Chart.yaml" {
			for _, line := range strings.Split(string(f.Data), "\n") {
				if v, ok := strings.CutPrefix(line, "version: "); ok {
					return v
				}
			}
		}
	}
	t.Fatal("the pulled files hold no Chart.yaml")
	return ""
}

func TestPullOCIChartOverPlainHTTP(t *testing.T) {
	reg := newFakeOCIRegistry(t, "charts/widget")
	archive := chartArchive(t, "widget", "1.0.0")
	reg.publish(t, "1.0.0", archive, chartMeta("widget", "1.0.0"))

	got, err := PullOCIChartWithPlainHTTP(context.Background(), reg.repoRef()+":1.0.0", reg.host(), "", "")
	require.NoError(t, err)
	assert.Equal(t, archive, got, "the chart layer is returned byte for byte")

	_, err = PullOCIChartWithPlainHTTP(context.Background(), reg.repoRef()+":9.9.9", reg.host(), "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to pull addon chart "+reg.repoRef()+":9.9.9")
	assert.False(t, IsOCIThrottledError(err), "a missing tag is not throttling and must not gate the host")
}

func TestPullOCIChartOverTLSReportsTheTransportError(t *testing.T) {
	t.Cleanup(ResetRateLimitGate)
	t.Cleanup(ResetOCIClientCache)
	host := closedRegistryHost(t)

	_, err := PullOCIChart(context.Background(), host+"/charts/widget:1.0.0", host, "", "")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrRateLimit)
	assert.NoError(t, sourceRateLimit.blocked(host), "a connection failure must not be mistaken for a rate limit")
}

func TestPullOCIChartFilesResolvesTheHighestTag(t *testing.T) {
	reg := newFakeOCIRegistry(t, "charts/widget")
	reg.publish(t, "0.9.0", chartArchive(t, "widget", "0.9.0"), chartMeta("widget", "0.9.0"))
	reg.publish(t, "1.0.0", chartArchive(t, "widget", "1.0.0"), chartMeta("widget", "1.0.0"))

	files, err := PullOCIChartFiles(context.Background(), reg.registry(), "widget", "")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", chartVersionOf(t, files))
	assert.EqualValues(t, 1, reg.tagListRequests.Load(), "an empty version lists the tags once")

	files, err = PullOCIChartFiles(context.Background(), reg.registry(), "widget", "0.9.0")
	require.NoError(t, err)
	assert.Equal(t, "0.9.0", chartVersionOf(t, files))
	assert.EqualValues(t, 1, reg.tagListRequests.Load(), "a pinned version does not list the tags")
}

func TestPullOCIChartFilesReadsAtThePinnedDigest(t *testing.T) {
	reg := newFakeOCIRegistry(t, "charts/widget")
	oldDigest := reg.publish(t, "1.0.0", chartArchive(t, "widget", "0.9.0"), chartMeta("widget", "0.9.0"))
	// The tag is re-pushed: it now points at a different artifact.
	reg.publish(t, "1.0.0", chartArchive(t, "widget", "1.0.0"), chartMeta("widget", "1.0.0"))

	// A registry pinned to the revision that was checked pulls that manifest,
	// not whatever the tag has moved to since.
	pinned := reg.registry().AtRevision("1.0.0@" + oldDigest)
	files, err := PullOCIChartFiles(context.Background(), pinned, "widget", "")
	require.NoError(t, err)
	assert.Equal(t, "0.9.0", chartVersionOf(t, files), "the pinned digest wins over the moved tag")
	assert.EqualValues(t, 0, reg.tagListRequests.Load(), "a pin needs no tag resolution")

	// Asking for the version the pin names still honours the pin.
	files, err = PullOCIChartFiles(context.Background(), pinned, "widget", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "0.9.0", chartVersionOf(t, files))

	// Asking for another version than the pin names ignores the pin: it
	// describes a different artifact.
	reg.publish(t, "2.0.0", chartArchive(t, "widget", "2.0.0"), chartMeta("widget", "2.0.0"))
	files, err = PullOCIChartFiles(context.Background(), pinned, "widget", "2.0.0")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", chartVersionOf(t, files))

	// A pin the registry no longer holds is an error, not a silent fallback.
	gone := reg.registry().AtRevision("1.0.0@sha256:" + strings.Repeat("0", 64))
	_, err = PullOCIChartFiles(context.Background(), gone, "widget", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to pull addon chart")
}

func TestPullOCIChartFilesReportsResolutionAndPullFailures(t *testing.T) {
	reg := newFakeOCIRegistry(t, "charts/widget")
	reg.publish(t, "latest", chartArchive(t, "widget", "1.0.0"), chartMeta("widget", "1.0.0"))

	_, err := PullOCIChartFiles(context.Background(), reg.registry(), "widget", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no semver tags found", "only non-semver tags means nothing to resolve")

	_, err = PullOCIChartFiles(context.Background(), reg.registry(), "widget", "9.9.9")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to pull addon chart "+reg.repoRef()+":9.9.9")
}

func TestPullOCIChartFilesRejectsABrokenArchive(t *testing.T) {
	reg := newFakeOCIRegistry(t, "charts/widget")
	digest := reg.publish(t, "2.0.0", []byte("not a gzipped tarball"), chartMeta("widget", "2.0.0"))

	_, err := PullOCIChartFiles(context.Background(), reg.registry(), "widget", "2.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load chart archive "+reg.repoRef()+":2.0.0")

	_, err = PullOCIChartFiles(context.Background(), reg.registry().AtRevision("2.0.0@"+digest), "widget", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load chart archive "+reg.repoRef()+"@"+digest+" (tag 2.0.0)")
}

func TestPushOCIChartReportsAMissingRepository(t *testing.T) {
	reg := newFakeOCIRegistry(t, "charts/widget")
	archive := chartArchive(t, "widget", "1.0.0")

	err := PushOCIChart(context.Background(), reg.registry(), "widget", "1.0.0", archive)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to push chart "+reg.repoRef()+":1.0.0")
	assert.True(t, IsOCIRepositoryNotFound(err), "a bare 404 on the blob upload is how ECR reports a missing repository: %v", err)

	err = PushOCIChart(context.Background(), Registry{Name: "museum", Helm: &HelmSource{URL: "https://charts.example.com"}}, "widget", "1.0.0", archive)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `registry "museum" is not an OCI registry`)
}

func TestOCIChartTagExistsAgainstARegistry(t *testing.T) {
	reg := newFakeOCIRegistry(t, "charts/widget")
	reg.publish(t, "1.0.0", chartArchive(t, "widget", "1.0.0"), chartMeta("widget", "1.0.0"))

	exists, err := OCIChartTagExists(context.Background(), reg.registry(), "widget", "1.0.0")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = OCIChartTagExists(context.Background(), reg.registry(), "widget", "1.1.0")
	require.NoError(t, err)
	assert.False(t, exists)

	_, err = OCIChartTagExists(context.Background(), Registry{Name: "git", Git: &GitAddonSource{URL: "https://github.com/o/r"}}, "widget", "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `registry "git" is not an OCI registry`)
}

func TestDefaultChartTagListerPicksTheTransportFromTheFlag(t *testing.T) {
	reg := newFakeOCIRegistry(t, "charts/widget")
	reg.publish(t, "1.0.0", chartArchive(t, "widget", "1.0.0"), chartMeta("widget", "1.0.0"))

	tags, err := chartTagLister(context.Background(), reg.repoRef(), reg.host(), "", "", true)
	require.NoError(t, err)
	assert.Equal(t, []string{"1.0.0"}, tags)

	// The same server over TLS is not a TLS server, so the TLS branch fails.
	_, err = chartTagLister(context.Background(), reg.repoRef(), reg.host(), "", "", false)
	assert.Error(t, err)
}

func TestOCIErrorClassifiersIgnoreNil(t *testing.T) {
	assert.False(t, IsOCIRepositoryNotFound(nil))
	assert.False(t, IsOCITagImmutable(nil))
}

func TestNewOCIClientWithCredentialsKeepsAPrivateCredentialsFile(t *testing.T) {
	ResetOCIClientCache()
	t.Cleanup(ResetOCIClientCache)

	_, err := NewOCIClientWithPlainHTTP("creds.example.com", "AWS", "token", false)
	require.NoError(t, err)

	key := ociClientCacheKey("creds.example.com", "AWS", "token", false)
	ociClientCache.Lock()
	entry, ok := ociClientCache.clients[key]
	ociClientCache.Unlock()
	require.True(t, ok)
	require.NotEmpty(t, entry.credFile, "a client built with a credential authenticates from its own file")
	assert.FileExists(t, entry.credFile)

	ResetOCIClientCache()
	assert.NoFileExists(t, entry.credFile, "evicting the client removes its credentials file")
}
