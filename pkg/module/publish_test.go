/*
Copyright 2021 The KubeVela Authors.

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

package module

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart/loader"
)

// archiveFS turns a packaged chart archive back into the fs.FS a fetch would
// see. loader.LoadArchiveFiles already strips each tar entry's leading
// <chartName>/ path segment (helm.sh/helm/v3@v3.14.4/pkg/chart/loader/archive.go),
// so the returned names are already module-root relative; chartName is kept
// as a parameter to document that relationship at call sites.
func archiveFS(t *testing.T, archive []byte, chartName string) fs.FS {
	t.Helper()
	files, err := loader.LoadArchiveFiles(bytes.NewReader(archive))
	require.NoError(t, err)
	out := fstestMapFS{}
	for _, f := range files {
		out[f.Name] = f.Data
	}
	require.NotEmpty(t, out, "archive for chart %q held no files", chartName)
	return out
}

func TestPackageModuleRoundTrip(t *testing.T) {
	dir := minimalModuleDir(t)
	source, err := ParseModuleDir(dir)
	require.NoError(t, err)

	art, err := PackageModule(dir, "")
	require.NoError(t, err)
	require.Equal(t, source.Version, art.Tag)

	pulled, err := ParseModule(archiveFS(t, art.Archive, source.Name))
	require.NoError(t, err)
	require.Equal(t, source, pulled)
}

func TestPackageModuleArchiveContents(t *testing.T) {
	art, err := PackageModule(minimalModuleDir(t), "")
	require.NoError(t, err)

	files, err := loader.LoadArchiveFiles(bytes.NewReader(art.Archive))
	require.NoError(t, err)
	names := map[string]bool{}
	for _, f := range files {
		names[f.Name] = true
	}
	for _, want := range []string{
		"Chart.yaml",
		"_module.cue",
		"auxiliary/xrd.yaml",
		"v1/_version.cue",
		"v1/auxiliary/composition.yaml",
		"v1/definitions/widget.yaml",
	} {
		require.True(t, names[want], "archive is missing %s, has %v", want, names)
	}
}

// TestPackageModuleChartNameMatchesModuleName proves the rule the whole
// feature rests on: the module fetch strips a <moduleName>/ prefix from the
// files it pulls back (pkg/module/service/fetch.go), so the chart's top-level
// directory must be the module's own name, never the source directory's
// name. It packages a tree whose directory is named "wrong-dir-name" while
// _module.cue declares module "s3", then reads the archive as a raw gzip tar
// (bypassing loader.LoadArchiveFiles, which strips the top-level segment
// without checking it against anything) and asserts every entry is rooted
// under "s3".
func TestPackageModuleChartNameMatchesModuleName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "wrong-dir-name")
	files := map[string]string{
		"_module.cue":           "module:  \"s3\"\nversion: \"1.0.0\"\n",
		"v1/_version.cue":       "apiVersion: \"v1\"\n",
		"v1/definitions/d.yaml": "apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: d\n",
	}
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	}

	art, err := PackageModule(dir, "")
	require.NoError(t, err)
	require.Equal(t, "s3", art.Module.Name)

	gz, err := gzip.NewReader(bytes.NewReader(art.Archive))
	require.NoError(t, err)
	tr := tar.NewReader(gz)
	seen := 0
	for {
		hd, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		top := strings.SplitN(hd.Name, "/", 2)[0]
		require.Equal(t, "s3", top, "archive entry %q is not rooted under module name %q", hd.Name, "s3")
		seen++
	}
	require.NotZero(t, seen, "archive contained no entries")
}

func TestPackageModuleAnnotations(t *testing.T) {
	dir := writeModuleTree(t, map[string]string{
		"_module.cue":             "module:  \"two\"\nversion: \"2.3.4\"\n",
		"v1/_version.cue":         "apiVersion: \"v1\"\n",
		"v1/definitions/one.yaml": "apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: one\n",
		"v2/_version.cue":         "apiVersion: \"v2\"\nenabled: false\n",
		"v2/definitions/two.yaml": "apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: two\n",
	})

	art, err := PackageModule(dir, "")
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		AnnotationModule:       "two",
		AnnotationLines:        "v1,v2",
		AnnotationEnabledLines: "v1",
	}, art.Annotations)
}

func TestPackageModuleVersionOverride(t *testing.T) {
	dir := minimalModuleDir(t)
	art, err := PackageModule(dir, "1.1.0-rc1")
	require.NoError(t, err)
	require.Equal(t, "1.1.0-rc1", art.Tag)
	require.Equal(t, "1.0.0", art.Module.Version)

	_, err = PackageModule(dir, "latest")
	require.ErrorContains(t, err, "not a valid semver")
}

func TestPackageModuleInvalidTreeIsRejected(t *testing.T) {
	dir := writeModuleTree(t, map[string]string{
		"_module.cue": "module:  \"bad\"\nversion: \"nope\"\n",
	})
	_, err := PackageModule(dir, "")
	require.ErrorContains(t, err, "not a valid semver")
}

func TestPackageModuleLeavesSourceTreeUntouched(t *testing.T) {
	dir := writeModuleTree(t, map[string]string{
		"_module.cue":           "module:  \"keep\"\nversion: \"1.0.0\"\n",
		"v1/_version.cue":       "apiVersion: \"v1\"\n",
		"v1/definitions/d.yaml": "apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: d\n",
		".helmignore":           "v1/\n",
	})
	before := treeSnapshot(t, dir)

	art, err := PackageModule(dir, "")
	require.NoError(t, err)
	require.Equal(t, before, treeSnapshot(t, dir))

	files, err := loader.LoadArchiveFiles(bytes.NewReader(art.Archive))
	require.NoError(t, err)
	for _, f := range files {
		require.NotEqual(t, ".helmignore", f.Name)
	}
	require.FileExists(t, filepath.Join(dir, ".helmignore"))

	pulled, err := ParseModule(archiveFS(t, art.Archive, "keep"))
	require.NoError(t, err)
	require.Contains(t, pulled.Lines, "v1")
}

// TestPackageModuleDropsHelmignoreAndChartYamlAtRootOnly proves the two
// exclusions in copyModuleTree are scoped to the tree root: a root
// .helmignore is dropped and a root Chart.yaml is replaced by the generated
// one, but a nested file that happens to share either name (module content,
// e.g. a chart shipped as auxiliary content) survives untouched. The parser
// never reads v1/auxiliary/*.yaml except composition.yaml, so a nested
// Chart.yaml there is tolerated.
func TestPackageModuleDropsHelmignoreAndChartYamlAtRootOnly(t *testing.T) {
	const authorRootChart = "apiVersion: v2\nname: author-chart\nversion: 9.9.9\n"
	const nestedChart = "apiVersion: v2\nname: nested-aux-chart\nversion: 0.0.1\n"
	dir := writeModuleTree(t, map[string]string{
		"_module.cue":             "module:  \"nested\"\nversion: \"1.0.0\"\n",
		"v1/_version.cue":         "apiVersion: \"v1\"\n",
		"v1/definitions/d.yaml":   "apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: d\n",
		"v1/auxiliary/Chart.yaml": nestedChart,
		".helmignore":             "v1/\n",
		"Chart.yaml":              authorRootChart,
	})

	art, err := PackageModule(dir, "")
	require.NoError(t, err)

	files, err := loader.LoadArchiveFiles(bytes.NewReader(art.Archive))
	require.NoError(t, err)
	contents := map[string][]byte{}
	for _, f := range files {
		contents[f.Name] = f.Data
	}

	require.NotContains(t, contents, ".helmignore", "root .helmignore must be dropped")
	require.Contains(t, contents, "v1/auxiliary/Chart.yaml", "nested Chart.yaml must be kept")
	require.Equal(t, nestedChart, string(contents["v1/auxiliary/Chart.yaml"]))
	require.Contains(t, contents, "Chart.yaml")
	require.NotEqual(t, authorRootChart, string(contents["Chart.yaml"]),
		"root Chart.yaml must be replaced by the generated one, not the author's")
}

// treeSnapshot maps every file path under dir to its contents, so a test can
// assert the directory is byte-identical before and after an operation.
func treeSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(filepath.Clean(p))
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(dir, p)
		if relErr != nil {
			return relErr
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	}))
	return out
}

// fstestMapFS is a tiny fs.FS over an in-memory file map, enough for
// ParseModule (ReadFile plus ReadDir).
type fstestMapFS map[string][]byte

func (m fstestMapFS) Open(name string) (fs.File, error) { return m.mapFS().Open(name) }

func (m fstestMapFS) ReadFile(name string) ([]byte, error) { return m.mapFS().ReadFile(name) }

func (m fstestMapFS) ReadDir(name string) ([]fs.DirEntry, error) { return m.mapFS().ReadDir(name) }

func (m fstestMapFS) mapFS() fstest.MapFS {
	out := fstest.MapFS{}
	for p, data := range m {
		out[p] = &fstest.MapFile{Data: data}
	}
	return out
}
