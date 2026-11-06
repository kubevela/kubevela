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
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/registry/component"
)

// These tests drive the inline-module reader through a real AsyncReader
// (LocalReader) and the real ClassifyItemByPattern / GetInstallPackageFromReader
// path. The mapFileReader-based tests in module_inline_test.go hand
// readInlineModulesDir bare "modules/..." paths, but a real reader's
// RelativePath is rooted at the addon name ("<addon>/modules/..."), so only a
// real reader proves the module tree is actually found in production.

// writeAddonTree writes files (path relative to the addon root) under
// <tmp>/<addonName> and returns the addon dir.
func writeAddonTree(t *testing.T, addonName string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), addonName)
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	return dir
}

func installPackageFromDir(t *testing.T, addonName, dir string) *InstallPackage {
	t.Helper()
	reader := component.NewLocalReader(dir, addonName)
	metas, err := reader.ListAddonMeta()
	require.NoError(t, err)
	meta := metas[addonName]
	uiData := &UIData{Meta: Meta{Name: addonName}}
	pkg, err := GetInstallPackageFromReader(reader, &meta, uiData)
	require.NoError(t, err)
	return pkg
}

func TestGetPatternFromItemRoutesModulesFilesCorrectly(t *testing.T) {
	r := mockReader{}
	cases := map[string]string{
		"my-addon/modules/_imports.cue":                 ModulesImportsFileName, // exact-path pattern must keep winning
		"my-addon/modules/aws-s3/_module.cue":           ModulesDirName,
		"my-addon/modules/aws-s3/v1/definitions/a.yaml": ModulesDirName,
		"my-addon/modules-extra/x.cue":                  "", // prefix must be a whole path segment
	}
	for p, want := range cases {
		assert.Equal(t, want, GetPatternFromItem(mockItem{path: p}, r, "my-addon"), p)
	}
}

func TestGetInstallPackageFromReaderFindsInlineModulesViaRealReader(t *testing.T) {
	files := map[string]string{
		"metadata.yaml": "name: inline-addon\nversion: 1.0.0\n",
	}
	maps.Copy(files, s3ModuleFiles("modules/aws-s3", "aws-s3"))
	dir := writeAddonTree(t, "inline-addon", files)

	pkg := installPackageFromDir(t, "inline-addon", dir)

	require.Len(t, pkg.InlineModules, 1, "the inline module must be found through a real reader's addon-prefixed paths")
	assert.Equal(t, "aws-s3", pkg.InlineModules[0].Name)
	assert.Empty(t, pkg.Imports)
}

func TestGetInstallPackageFromReaderMixedInlineAndImportsViaRealReader(t *testing.T) {
	files := map[string]string{
		"metadata.yaml":        "name: mixed-addon\nversion: 1.0.0\n",
		"modules/_imports.cue": `imports: [{module: "aws-efs", enabled: true, sources: [{registry: "oam-modules", version: "1.0.0"}]}]`,
	}
	maps.Copy(files, s3ModuleFiles("modules/aws-s3", "aws-s3"))
	dir := writeAddonTree(t, "mixed-addon", files)

	pkg := installPackageFromDir(t, "mixed-addon", dir)

	require.Len(t, pkg.InlineModules, 1)
	assert.Equal(t, "aws-s3", pkg.InlineModules[0].Name)
	require.Len(t, pkg.Imports, 1)
	assert.Equal(t, "aws-efs", pkg.Imports[0].Module)
}

func TestGetInstallPackageFromReaderNoModulesDirIsUnchanged(t *testing.T) {
	dir := writeAddonTree(t, "plain-addon", map[string]string{
		"metadata.yaml": "name: plain-addon\nversion: 1.0.0\n",
	})
	pkg := installPackageFromDir(t, "plain-addon", dir)
	assert.Empty(t, pkg.InlineModules)
	assert.Empty(t, pkg.Imports)
}

func TestGetInstallPackageFromReaderBrokenInlineModuleFailsAddonLoad(t *testing.T) {
	files := map[string]string{
		"metadata.yaml":                            "name: broken-addon\nversion: 1.0.0\n",
		"modules/aws-s3/_module.cue":               `module: "aws-s3"` + "\nversion: \"0.1.0\"",
		"modules/aws-s3/v1/_version.cue":           `apiVersion: "v1"`,
		"modules/aws-s3/v1/definitions/bucket.cue": `this is { not valid cue`,
	}
	dir := writeAddonTree(t, "broken-addon", files)

	reader := component.NewLocalReader(dir, "broken-addon")
	metas, err := reader.ListAddonMeta()
	require.NoError(t, err)
	meta := metas["broken-addon"]
	_, err = GetInstallPackageFromReader(reader, &meta, &UIData{Meta: Meta{Name: "broken-addon"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inline modules")
}

// An addon whose own name contains (or equals) "modules" must not confuse the
// addon-root prefix with the modules/ directory.
func TestGetInstallPackageFromReaderAddonNamedLikeModulesDir(t *testing.T) {
	for _, addonName := range []string{"modules", "inline-modules"} {
		t.Run(addonName, func(t *testing.T) {
			files := map[string]string{"metadata.yaml": "name: " + addonName + "\nversion: 1.0.0\n"}
			maps.Copy(files, s3ModuleFiles("modules/aws-s3", "aws-s3"))
			pkg := installPackageFromDir(t, addonName, writeAddonTree(t, addonName, files))
			require.Len(t, pkg.InlineModules, 1)
			assert.Equal(t, "aws-s3", pkg.InlineModules[0].Name)
		})
	}
}
