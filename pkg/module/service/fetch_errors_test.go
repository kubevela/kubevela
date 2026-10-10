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

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart/loader"

	"github.com/oam-dev/kubevela/pkg/module"
	"github.com/oam-dev/kubevela/pkg/registry/component"
)

// stubReader is a component.AsyncReader whose listing and file reads a test
// scripts, so readerFS can be driven past what MemoryReader produces.
type stubReader struct {
	meta    map[string]component.SourceMeta
	listErr error
	files   map[string]string
	readErr error
	// relative rewrites the path RelativePath returns, to stand for a reader
	// whose items are not under the module root.
	relative func(item component.Item) string
}

func (r stubReader) ListAddonMeta() (map[string]component.SourceMeta, error) {
	return r.meta, r.listErr
}
func (r stubReader) ReadFile(path string) (string, error) {
	if r.readErr != nil {
		return "", r.readErr
	}
	return r.files[path], nil
}
func (r stubReader) RelativePath(item component.Item) string {
	if r.relative != nil {
		return r.relative(item)
	}
	return item.GetPath()
}

func TestReaderFSReportsListingFailures(t *testing.T) {
	_, err := readerFS(stubReader{meta: map[string]component.SourceMeta{}}, "s3")
	assert.ErrorIs(t, err, module.ErrModuleNotFound, "a package the registry does not carry is the module-not-found sentinel")
	assert.Contains(t, err.Error(), `module "s3" not found in registry`)

	boom := errors.New("listing refused")
	_, err = readerFS(stubReader{listErr: boom}, "s3")
	assert.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "list modules")
}

func TestReaderFSKeepsOnlyFilesUnderTheModuleRoot(t *testing.T) {
	meta := map[string]component.SourceMeta{"s3": {Name: "s3", Items: []component.Item{
		component.NewOSSItem(component.DirType, "s3/v1", "v1"),
		component.NewOSSItem(component.FileType, "s3/_module.cue", "_module.cue"),
		component.NewOSSItem(component.FileType, "other/_module.cue", "_module.cue"),
		component.NewOSSItem(component.FileType, "s3/", ""),
	}}}
	files := map[string]string{"s3/_module.cue": `name: "s3"`}

	fsys, err := readerFS(stubReader{meta: meta, files: files}, "s3")
	require.NoError(t, err)
	m, ok := fsys.(MapFS)
	require.True(t, ok)
	assert.Equal(t, MapFS{"_module.cue": []byte(`name: "s3"`)}, m,
		"directories, files of other modules and the bare root are skipped")
}

func TestReaderFSReportsReadFailuresAndEmptyModules(t *testing.T) {
	meta := map[string]component.SourceMeta{"s3": {Name: "s3", Items: []component.Item{
		component.NewOSSItem(component.FileType, "s3/_module.cue", "_module.cue"),
	}}}

	boom := errors.New("blob gone")
	_, err := readerFS(stubReader{meta: meta, readErr: boom}, "s3")
	assert.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), `module "s3": read s3/_module.cue`)

	onlyOthers := map[string]component.SourceMeta{"s3": {Name: "s3", Items: []component.Item{
		component.NewOSSItem(component.FileType, "other/_module.cue", "_module.cue"),
	}}}
	_, err = readerFS(stubReader{meta: onlyOthers}, "s3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `module "s3" is empty`)
}

func TestSourceFSNeedsAnOCISource(t *testing.T) {
	s := NewService(fakeStore{})
	_, err := s.sourceFS(context.Background(), &component.Registry{Name: "gh", Git: &component.GitAddonSource{URL: "https://github.com/o/r"}}, "s3", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `registry "gh" has no supported module source`)
}

func TestPullModuleChartWrapsTheTransportError(t *testing.T) {
	_, err := pullModuleChart(context.Background(), &component.Registry{Name: "gh", Git: &component.GitAddonSource{URL: "https://github.com/o/r"}}, "s3", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `module "s3": pull OCI chart`)
	assert.Contains(t, err.Error(), `registry "gh" is not an OCI registry`)
}

func TestOCIChartFSNamesTheRegistryOnAnEmptyChart(t *testing.T) {
	s := NewService(fakeStore{})
	// A chart with no files at all: MemoryReader files every name it is given
	// under the module root, so an empty chart is the one way the adapter can
	// come back with nothing.
	s.pullChart = func(context.Context, *component.Registry, string, string) ([]*loader.BufferedFile, error) {
		return nil, nil
	}
	reg := ociRegistry("ecr")
	_, err := s.ociChartFS(context.Background(), &reg, "s3", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `registry "ecr": module "s3" is empty`)
}

func TestFetchModuleReportsAModuleThatDoesNotParse(t *testing.T) {
	t.Cleanup(ResetModuleCache)
	reg := ociRegistry("ecr")
	s := newServiceWithFakes(fakeStore{regs: []component.Registry{reg}}, map[string]string{"s3/README.md": "not a module"})

	_, err := s.FetchModule(context.Background(), "ecr", "s3", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `registry "ecr", module "s3": parse module`)
}

// TestFetchModuleAsksTheRegistryForItsRevision leaves the revision seam unset,
// so the real PackageRevision runs. A plain-HTTP spelling is an OCI chart
// source but not an oci:// one, so it answers ErrRevisionUnsupported locally
// and the fetch reads without touching the network.
func TestFetchModuleAsksTheRegistryForItsRevision(t *testing.T) {
	t.Cleanup(ResetModuleCache)
	reg := component.Registry{Name: "local", Helm: &component.HelmSource{URL: "http://127.0.0.1:30500/modules"}}
	pulls := 0
	s := NewService(fakeStore{regs: []component.Registry{reg}})
	s.pullChart = func(context.Context, *component.Registry, string, string) ([]*loader.BufferedFile, error) {
		pulls++
		return []*loader.BufferedFile{
			{Name: "s3/_module.cue", Data: []byte("module: \"s3\"\nversion: \"1.0.0\"\n")},
			{Name: "s3/v1/_version.cue", Data: []byte("apiVersion: \"v1\"\n")},
			{Name: "s3/v1/definitions/bucket.yaml", Data: []byte("apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: bucket\n")},
		}, nil
	}

	for i := 0; i < 2; i++ {
		mod, err := s.FetchModule(context.Background(), "local", "s3", "1.0.0")
		require.NoError(t, err)
		assert.Equal(t, "s3", mod.Name)
	}
	assert.Equal(t, 2, pulls, "without a revision to compare, every fetch reads")
}

func TestModuleCacheKeyTellsSourcesAndCredentialsApart(t *testing.T) {
	ecr := component.Registry{Name: "ecr", Helm: &component.HelmSource{URL: "oci://reg.example.com/modules", Username: "AWS", Token: "t1"}}
	rotated := component.Registry{Name: "ecr", Helm: &component.HelmSource{URL: "oci://reg.example.com/modules", Username: "AWS", Token: "t2"}}
	git := component.Registry{Name: "gh", Git: &component.GitAddonSource{URL: "https://github.com/o/r"}}

	k1 := moduleCacheKey(&ecr, "s3", "1.0.0")
	assert.Equal(t, k1, moduleCacheKey(&ecr, "s3", "1.0.0"), "the key is stable")
	assert.NotEqual(t, k1, moduleCacheKey(&rotated, "s3", "1.0.0"), "a rotated token must not see what the old one cached")
	assert.NotEqual(t, k1, moduleCacheKey(&ecr, "s3", "1.1.0"))
	assert.NotEqual(t, k1, moduleCacheKey(&ecr, "gcs", "1.0.0"))
	assert.NotContains(t, k1, "t1", "the secret itself never reaches the key")
	assert.Contains(t, moduleCacheKey(&git, "s3", ""), "|unknown|", "a source the fetcher cannot read is keyed as such")
}
