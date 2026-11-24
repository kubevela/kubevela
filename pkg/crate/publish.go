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

package crate

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
)

const (
	// AnnotationCrate is the OCI annotation recording the published crate's
	// name, so a registry listing identifies the crate without pulling it.
	AnnotationCrate = "crates.oam.dev/crate"

	// AnnotationLines is the OCI annotation recording every API line in the
	// published artifact, comma-separated and sorted.
	AnnotationLines = "crates.oam.dev/lines"

	// AnnotationEnabledLines is the OCI annotation recording the subset of
	// lines their author left enabled, which is what the render service
	// installs. It is absent when no line is enabled.
	AnnotationEnabledLines = "crates.oam.dev/enabled-lines"

	// chartTypeLibrary marks the artifact a Helm library chart so nobody can
	// helm install a crate by accident, matching what addon packaging does.
	chartTypeLibrary = "library"
)

// Artifact is a crate packaged for publication: the parsed crate, the tag it
// publishes under, the OCI annotations to stamp, and the Helm chart archive
// carrying the crate tree.
type Artifact struct {
	// Crate is the parsed crate the archive was built from.
	Crate *Crate
	// Tag is the version the artifact publishes under: the crate's own
	// version, or versionOverride when PackageCrate was given one.
	Tag string
	// Annotations are the OCI annotations to stamp on the published artifact.
	Annotations map[string]string
	// Archive is the gzipped tar of the Helm chart carrying the crate tree.
	Archive []byte
}

// PackageCrate parses the crate tree at dir and packages it as a Helm chart
// archive named after the crate and tagged from its version, or from
// versionOverride when that is set. The chart name must be the crate's own
// name: the fetch strips a <crateName>/ prefix from the files it pulls back,
// so an archive built under any other name reads as an empty crate.
//
// dir is read only. The generated Chart.yaml is written into a temporary copy
// of the tree, so publishing leaves the author's source directory untouched
// whether it succeeds or fails.
func PackageCrate(dir, versionOverride string) (*Artifact, error) {
	mod, err := ParseCrateDir(dir)
	if err != nil {
		return nil, err
	}

	tag := mod.Version
	if versionOverride != "" {
		if err := validateCrateVersion(versionOverride, "--version"); err != nil {
			return nil, err
		}
		tag = versionOverride
	}

	workdir, err := os.MkdirTemp("", "vela-crate-publish-")
	if err != nil {
		return nil, fmt.Errorf("package crate %q: create work directory: %w", mod.Name, err)
	}
	defer func() {
		_ = os.RemoveAll(workdir)
	}()

	treeDir := filepath.Join(workdir, "tree")
	if err := copyCrateTree(dir, treeDir); err != nil {
		return nil, fmt.Errorf("package crate %q: %w", mod.Name, err)
	}

	annotations := crateAnnotations(mod)
	meta := &chart.Metadata{
		APIVersion:  chart.APIVersionV2,
		Name:        mod.Name,
		Version:     tag,
		AppVersion:  mod.Version,
		Type:        chartTypeLibrary,
		Description: fmt.Sprintf("KubeVela crate %s", mod.Name),
		Annotations: annotations,
	}
	if err := chartutil.SaveChartfile(filepath.Join(treeDir, chartutil.ChartfileName), meta); err != nil {
		return nil, fmt.Errorf("package crate %q: write %s: %w", mod.Name, chartutil.ChartfileName, err)
	}

	ch, err := loader.LoadDir(treeDir)
	if err != nil {
		return nil, fmt.Errorf("package crate %q: load chart: %w", mod.Name, err)
	}
	outDir := filepath.Join(workdir, "out")
	if err := os.Mkdir(outDir, 0o750); err != nil {
		return nil, fmt.Errorf("package crate %q: create output directory: %w", mod.Name, err)
	}
	archivePath, err := chartutil.Save(ch, outDir)
	if err != nil {
		return nil, fmt.Errorf("package crate %q: package chart: %w", mod.Name, err)
	}
	archive, err := os.ReadFile(filepath.Clean(archivePath))
	if err != nil {
		return nil, fmt.Errorf("package crate %q: read archive: %w", mod.Name, err)
	}

	return &Artifact{Crate: mod, Tag: tag, Annotations: annotations, Archive: archive}, nil
}

// crateAnnotations builds the crate, lines, and enabled-lines annotations,
// with line names sorted so a republished artifact is byte-stable.
func crateAnnotations(mod *Crate) map[string]string {
	lines := make([]string, 0, len(mod.Lines))
	enabled := make([]string, 0, len(mod.Lines))
	for name, line := range mod.Lines {
		lines = append(lines, name)
		if line.Enabled {
			enabled = append(enabled, name)
		}
	}
	sort.Strings(lines)
	sort.Strings(enabled)

	annotations := map[string]string{
		AnnotationCrate: mod.Name,
		AnnotationLines: strings.Join(lines, ","),
	}
	if len(enabled) > 0 {
		annotations[AnnotationEnabledLines] = strings.Join(enabled, ",")
	}
	return annotations
}

// copyCrateTree copies the crate tree at src into dst.
//
// Three entries are deliberately dropped. A root .helmignore would make
// Helm's directory loader skip the author's own files (loader applies
// ignore.Empty() only when the file is absent, and only at the chart root),
// silently shrinking the artifact. A .git directory is repository state, not
// crate content, dropped at any depth. A root Chart.yaml is replaced by the
// generated one; a nested file that happens to share either name (for
// example a chart shipped as auxiliary content) is ordinary crate content
// and is kept, since neither Helm's loader nor the generated Chart.yaml
// cares about it. Symlinks are rejected rather than followed, so packaging
// cannot reach outside the tree.
func copyCrateTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o750)
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o750)
		}
		if rel == ".helmignore" || rel == chartutil.ChartfileName {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("crate tree contains a symlink at %s; publish requires plain files", rel)
		}
		data, err := os.ReadFile(filepath.Clean(p))
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o600)
	})
}
