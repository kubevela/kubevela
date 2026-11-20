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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart/loader"

	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
)

// gitBuilderCall records what NewAsyncReader handed the registered Git-family
// builder.
type gitBuilderCall struct {
	baseURL, repo, subPath, token, ref string
	rdType                             ReaderType
}

// withFakeGitReaderBuilder registers a builder that records its arguments and
// returns reader, restoring whatever was registered before when the test ends.
// pkg/addon registers the real one from an init; this package has none.
func withFakeGitReaderBuilder(t *testing.T, reader AsyncReader, err error) *[]gitBuilderCall {
	t.Helper()
	previous := gitReaderBuilder
	t.Cleanup(func() { gitReaderBuilder = previous })
	calls := &[]gitBuilderCall{}
	RegisterGitReaderBuilder(func(baseURL, repo, subPath, token string, rdType ReaderType, ref string) (AsyncReader, error) {
		*calls = append(*calls, gitBuilderCall{baseURL, repo, subPath, token, ref, rdType})
		return reader, err
	})
	return calls
}

func TestValidateCredentialNeedsUsernameAndTokenTogether(t *testing.T) {
	assert.NoError(t, (&HelmSource{URL: "oci://reg.example.com/addons"}).ValidateCredential(), "anonymous access is both fields empty")
	assert.NoError(t, (&HelmSource{URL: "oci://reg.example.com/addons", Username: "AWS", Token: "t"}).ValidateCredential())
	assert.NoError(t, (&HelmSource{URL: "oci://reg.example.com/addons", Username: "AWS", TokenSecretRef: "addon-registry-ecr"}).ValidateCredential(),
		"a token already moved into a Secret still counts as configured")

	for name, src := range map[string]*HelmSource{
		"username only": {URL: "oci://reg.example.com/addons", Username: "AWS"},
		"token only":    {URL: "oci://reg.example.com/addons", Token: "t"},
	} {
		err := src.ValidateCredential()
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "needs username and token together")
	}
}

func TestReaderOptions(t *testing.T) {
	assert.Equal(t, readerConfig{}, newReaderConfig(nil))
	assert.Equal(t, readerConfig{ref: "v1.2.3"}, newReaderConfig([]ReaderOption{WithRef("v1.2.3")}))
	assert.Equal(t, readerConfig{ref: "second"}, newReaderConfig([]ReaderOption{WithRef("first"), WithRef("second")}), "the last option wins")
}

func TestNewAsyncReaderDelegatesTheGitFamily(t *testing.T) {
	t.Run("without a registered builder", func(t *testing.T) {
		previous := gitReaderBuilder
		t.Cleanup(func() { gitReaderBuilder = previous })
		RegisterGitReaderBuilder(nil)
		for _, rdType := range []ReaderType{GitType, GiteeType, GitlabType} {
			_, err := NewAsyncReader("https://github.com/o/r", "", "", "", "", rdType)
			require.Error(t, err, "%s", rdType)
			assert.Contains(t, err.Error(), `no reader is registered for addon registry type "`+string(rdType)+`"`)
		}
	})

	t.Run("with a registered builder", func(t *testing.T) {
		want := &MemoryReader{Name: "widget"}
		calls := withFakeGitReaderBuilder(t, want, nil)
		got, err := NewAsyncReader("https://gitlab.com", "", "org/repo", "addons", "tok", GitlabType, WithRef("main"))
		require.NoError(t, err)
		assert.Same(t, want, got)
		require.Len(t, *calls, 1)
		assert.Equal(t, gitBuilderCall{baseURL: "https://gitlab.com", repo: "org/repo", subPath: "addons", token: "tok", ref: "main", rdType: GitlabType}, (*calls)[0])
	})
}

func TestNewAsyncReaderBuildsAnOSSReader(t *testing.T) {
	got, err := NewAsyncReader("https://oss-cn-hangzhou.aliyuncs.com", "my-bucket", "", "addons", "", OSSType)
	require.NoError(t, err)
	oss, ok := got.(*ossReader)
	require.True(t, ok)
	assert.Equal(t, "https://my-bucket.oss-cn-hangzhou.aliyuncs.com", oss.bucketEndPoint, "the bucket becomes a subdomain of the endpoint")
	assert.Equal(t, "addons", oss.path)

	got, err = NewAsyncReader("http://oss.example.com", "my-bucket", "", "", "", OSSType)
	require.NoError(t, err)
	assert.Equal(t, "http://my-bucket.oss.example.com", got.(*ossReader).bucketEndPoint, "an explicit scheme is kept")

	// A scheme-less endpoint parses as a path, not a host, so the bucket URL
	// loses the endpoint. This is long-standing behaviour, pinned here so a
	// change to it is a deliberate one.
	got, err = NewAsyncReader("oss.example.com", "my-bucket", "", "", "", OSSType)
	require.NoError(t, err)
	assert.Equal(t, "https://my-bucket.", got.(*ossReader).bucketEndPoint)

	got, err = NewAsyncReader("https://my-bucket.oss.example.com", "", "", "", "", OSSType)
	require.NoError(t, err)
	assert.Equal(t, "https://my-bucket.oss.example.com", got.(*ossReader).bucketEndPoint, "without a bucket the endpoint is used as given")

	_, err = NewAsyncReader("://not-a-url", "b", "", "", "", OSSType)
	assert.Error(t, err)

	_, err = NewAsyncReader("https://charts.example.com", "", "", "", "", ReaderType("helm"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid addon registry type 'helm'")
}

func TestBuildReaderPicksTheConfiguredSource(t *testing.T) {
	calls := withFakeGitReaderBuilder(t, &MemoryReader{Name: "widget"}, nil)

	for name, tc := range map[string]struct {
		reg  Registry
		want gitBuilderCall
	}{
		"git":    {Registry{Git: &GitAddonSource{URL: "https://github.com/o/r", Path: "addons", Token: "t1"}}, gitBuilderCall{baseURL: "https://github.com/o/r", subPath: "addons", token: "t1", rdType: GitType}},
		"gitee":  {Registry{Gitee: &GiteeAddonSource{URL: "https://gitee.com/o/r", Path: "p", Token: "t2"}}, gitBuilderCall{baseURL: "https://gitee.com/o/r", subPath: "p", token: "t2", rdType: GiteeType}},
		"gitlab": {Registry{Gitlab: &GitlabAddonSource{URL: "https://gitlab.com", Repo: "o/r", Path: "p", Token: "t3"}}, gitBuilderCall{baseURL: "https://gitlab.com", repo: "o/r", subPath: "p", token: "t3", rdType: GitlabType}},
	} {
		t.Run(name, func(t *testing.T) {
			*calls = nil
			_, err := tc.reg.BuildReader(WithRef("pinned"))
			require.NoError(t, err)
			require.Len(t, *calls, 1)
			tc.want.ref = "pinned"
			assert.Equal(t, tc.want, (*calls)[0])
		})
	}

	t.Run("oss", func(t *testing.T) {
		reader, err := (&Registry{OSS: &OSSAddonSource{Endpoint: "https://oss.example.com", Bucket: "b", Path: "addons"}}).BuildReader()
		require.NoError(t, err)
		assert.Equal(t, "https://b.oss.example.com", reader.(*ossReader).bucketEndPoint)
	})

	t.Run("nothing configured", func(t *testing.T) {
		_, err := (&Registry{Name: "empty"}).BuildReader()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "enough info to build a reader")
	})
}

func TestRegistryListsThroughItsReader(t *testing.T) {
	reader := &MemoryReader{Name: "widget", Files: []*loader.BufferedFile{{Name: "metadata.yaml", Data: []byte("name: widget")}}}
	withFakeGitReaderBuilder(t, reader, nil)
	reg := Registry{Name: "gh", Git: &GitAddonSource{URL: "https://github.com/o/r"}}

	metas, err := reg.ListAddonMeta()
	require.NoError(t, err)
	assert.Contains(t, metas, "widget")

	meta, err := reg.ListPackageMeta("widget")
	require.NoError(t, err)
	assert.Equal(t, "widget", meta.Name)

	_, err = reg.ListPackageMeta("gadget")
	assert.ErrorIs(t, err, ErrPackageNotExist)

	withFakeGitReaderBuilder(t, nil, assert.AnError)
	_, err = reg.ListAddonMeta()
	assert.ErrorIs(t, err, assert.AnError, "a reader that cannot be built fails the listing")
}

func TestLocalReaderName(t *testing.T) {
	reader := NewLocalReader(t.TempDir(), "local-addons")
	assert.Equal(t, "local-addons", reader.Name())
}

func TestOSSItem(t *testing.T) {
	item := NewOSSItem("file", "addons/widget/metadata.yaml", "metadata.yaml")
	assert.Equal(t, "file", item.GetType())
	assert.Equal(t, "addons/widget/metadata.yaml", item.GetPath())
	assert.Equal(t, "metadata.yaml", item.GetName())
}

func TestOSSReaderReportsAMissingFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/addons/widget/metadata.yaml":
			_, _ = w.Write([]byte("name: widget"))
		case "/addons/widget/broken.yaml":
			http.Error(w, "throttled", http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	reader, err := NewAsyncReader(srv.URL, "", "", "addons", "", OSSType)
	require.NoError(t, err)

	content, err := reader.ReadFile("widget/metadata.yaml")
	require.NoError(t, err)
	assert.Equal(t, "name: widget", content)

	_, err = reader.ReadFile("widget/missing.yaml")
	assert.ErrorIs(t, err, velaerrors.ErrFileNotFound, "a 404 is the file-not-found sentinel callers already test for")

	_, err = reader.ReadFile("widget/broken.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status 503")
	assert.NotErrorIs(t, err, velaerrors.ErrFileNotFound)
}
