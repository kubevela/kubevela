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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart/loader"
)

// closedRegistryHost returns a host:port nothing listens on any more, so a
// call that reaches the network fails at once with a connection refusal.
func closedRegistryHost(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	host := strings.TrimPrefix(srv.URL, "http://")
	srv.Close()
	return host
}

func TestPackageRevisionRejectsBadName(t *testing.T) {
	reg := Registry{Name: "r", Helm: &HelmSource{URL: "oci://reg.example.com/charts"}}
	for _, name := range []string{"", "..", "a/b"} {
		_, err := reg.PackageRevision(context.Background(), name, "1.0.0", "")
		assert.ErrorIs(t, err, ErrPackageNotExist, "name %q", name)
	}
}

func TestPackageRevisionIsUnsupportedOffOCI(t *testing.T) {
	for name, reg := range map[string]Registry{
		"git":        {Name: "r", Git: &GitAddonSource{URL: "https://github.com/org/repo"}},
		"helm https": {Name: "r", Helm: &HelmSource{URL: "https://charts.example.com"}},
		"helm http":  {Name: "r", Helm: &HelmSource{URL: "http://charts.example.com"}},
		"oss":        {Name: "r", OSS: &OSSAddonSource{Endpoint: "oss.example.com", Bucket: "b"}},
		"empty":      {Name: "r"},
	} {
		_, err := reg.PackageRevision(context.Background(), "widget", "", "")
		assert.ErrorIs(t, err, ErrRevisionUnsupported, "%s registry", name)
	}
}

func TestPackageRevisionProbesAnOCIRegistry(t *testing.T) {
	t.Cleanup(ResetRateLimitGate)
	t.Cleanup(ResetOCIClientCache)
	host := closedRegistryHost(t)
	reg := Registry{Name: "r", Helm: &HelmSource{URL: "oci://" + host + "/charts", Username: "u", Token: "t"}}

	// A pinned version goes straight to the manifest probe; the registry is
	// gone, so the probe is what fails.
	_, err := reg.PackageRevision(context.Background(), "widget", "1.0.0", "1.0.0@sha256:aaa")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read manifest of "+host+"/charts/widget:1.0.0")

	// No version means the highest tag is resolved first, which is also a
	// registry call.
	_, err = reg.PackageRevision(context.Background(), "widget", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list tags for OCI repository "+host+"/charts/widget")
}

type erroringReader struct{ err error }

func (r erroringReader) ListAddonMeta() (map[string]SourceMeta, error) { return nil, r.err }
func (r erroringReader) ReadFile(string) (string, error)               { return "", r.err }
func (r erroringReader) RelativePath(item Item) string                 { return item.GetName() }

func TestListPackageMeta(t *testing.T) {
	reader := &MemoryReader{Name: "widget", Files: []*loader.BufferedFile{
		{Name: "metadata.yaml", Data: []byte("name: widget")},
	}}

	meta, err := ListPackageMeta(reader, "widget")
	require.NoError(t, err)
	assert.Equal(t, "widget", meta.Name)
	require.Len(t, meta.Items, 1)
	assert.Equal(t, "metadata.yaml", meta.Items[0].GetName())

	_, err = ListPackageMeta(reader, "gadget")
	assert.ErrorIs(t, err, ErrPackageNotExist, "the registry answered, and the answer was no such package")

	_, err = ListPackageMeta(reader, "../widget")
	assert.ErrorIs(t, err, ErrPackageNotExist, "a name that cannot be a package is refused before listing")

	boom := errors.New("listing failed")
	_, err = ListPackageMeta(erroringReader{err: boom}, "widget")
	assert.ErrorIs(t, err, boom)
}

func TestRegistryListPackageMetaNeedsAReader(t *testing.T) {
	reg := Registry{Name: "empty"}
	_, err := reg.ListPackageMeta("widget")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enough info to build a reader")
}
