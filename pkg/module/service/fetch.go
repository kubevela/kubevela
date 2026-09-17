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

// Package service fetches a module from a registry and parses it into a
// module.Module, server-side, for the type: module render path. It reuses the
// shared transport (git reader, OCI Helm-chart client) and the Registry
// model; it does not reuse the addon parsing/packaging layer.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	goerrors "errors"
	"fmt"
	"io/fs"
	"strings"

	"helm.sh/helm/v3/pkg/chart/loader"

	"github.com/oam-dev/kubevela/pkg/module"
	"github.com/oam-dev/kubevela/pkg/registry/component"
)

// Service fetches modules. It resolves registries through module.ResolveRegistry
// over a component.RegistryDataStore — reusing that story's default
// policy, source rejection, token loading, and not-found reporting. Its
// reader/puller seams are wired to the real addon transport by NewService and
// overridden by tests.
type Service struct {
	store     component.RegistryDataStore
	newReader func(reg *component.Registry) (component.AsyncReader, error)
	pullChart ociChartPuller
	// revision names what the registry holds without reading it, so a
	// reconcile that finds it unchanged reads nothing. A seam like the two
	// above: a fake reader that cannot report a revision reports
	// ErrRevisionUnsupported and every fetch reads, which is what the
	// uncached path always did.
	revision packageRevisioner
}

// packageRevisioner names the current revision of one package in a registry.
type packageRevisioner func(ctx context.Context, reg *component.Registry, name, version, lastKnown string) (string, error)

// NewService wires the real addon transport. In production the store is
// module.NewStore(cli) (the vela-module-registry ConfigMap).
func NewService(store component.RegistryDataStore) *Service {
	s := &Service{
		store:     store,
		newReader: buildModuleReader,
		pullChart: pullModuleChart,
	}
	s.revision = s.packageRevision
	return s
}

// packageRevision asks the registry what it holds now. The git half goes
// through the reader seam, so whatever reads the files is also what reports
// their revision; a reader that cannot answer stops the caching rather than
// reaching past the seam to the network.
func (s *Service) packageRevision(ctx context.Context, reg *component.Registry, name, version, lastKnown string) (string, error) {
	if reg.OCIChartSource() != nil {
		return reg.PackageRevision(ctx, name, version, lastKnown)
	}
	reader, err := s.newReader(reg)
	if err != nil {
		return "", err
	}
	revisions, ok := reader.(component.RevisionReader)
	if !ok {
		return "", component.ErrRevisionUnsupported
	}
	return revisions.Revision(ctx, lastKnown)
}

// buildModuleReader builds the reader for a module registry, pointed at its
// modules root (the source Path); ListAddonMeta then keys each module by name.
// It reuses the addon transport (reg.BuildReader), returning the component.AsyncReader
// readerFS consumes.
func buildModuleReader(reg *component.Registry) (component.AsyncReader, error) {
	return reg.BuildReader()
}

// FetchModule resolves the registry, fetches the module's files into an fs.FS,
// and parses them. Resolution is module.ResolveRegistry: an empty name selects
// the sole registry or the "catalog" default, non-git/OCI sources are rejected,
// and unknown names report the configured registries (wrapping ErrRegistryNotFound).
//
// version selects the module package version (the OCI/ECR tag vela module
// publish writes from _module.cue's version field). A git source has no tag
// concept and always pulls from the repository's default branch.
func (s *Service) FetchModule(ctx context.Context, registry, moduleName, version string) (*module.Module, error) {
	reg, err := module.ResolveRegistry(ctx, s.store, registry)
	if err != nil {
		return nil, err
	}
	// The render path runs on every reconcile of every Application that names
	// this module, so what it costs the registry is what matters here. Reading
	// the module is skipped entirely whenever the registry reports the same
	// revision it was read at.
	return moduleCache.Load(
		moduleCacheKey(&reg, moduleName, version),
		func(lastKnown string) (string, error) {
			if s.revision == nil {
				return "", component.ErrRevisionUnsupported
			}
			return s.revision(ctx, &reg, moduleName, version, lastKnown)
		},
		func(revision string) (*module.Module, error) {
			// Read at the revision that was checked, so the files parsed here
			// are the ones that revision names.
			at := reg.AtRevision(revision)
			fsys, err := s.sourceFS(ctx, &at, moduleName, version)
			if err != nil {
				return nil, err
			}
			mod, err := module.ParseModule(fsys)
			if err != nil {
				return nil, fmt.Errorf("registry %q, module %q: %w", reg.Name, moduleName, err)
			}
			return mod, nil
		},
	)
}

// moduleCache holds parsed modules by the registry revision they were read at.
// It is package scoped because production builds a Service per call
// (rendererImpl.fetch), so a cache on the Service would never see a second hit.
var moduleCache = component.NewRevisionCache[*module.Module](component.DefaultRevisionCacheSize)

// ResetModuleCache empties the module cache. It exists for tests, which would
// otherwise carry a module from one case into the next.
func ResetModuleCache() { moduleCache.Reset() }

// moduleCacheKey identifies what was read, which is more than the module name:
// it is that module, at that version, out of that source, read with those
// credentials. The credentials are in the key as a digest so that rotating a
// token invalidates what the old one could see, without the secret itself
// reaching a map key, a log line, or a metric label.
func moduleCacheKey(reg *component.Registry, moduleName, version string) string {
	// Same order sourceFS dispatches on, so a registry carrying both sources
	// is not keyed as one and read as the other.
	var source, secret string
	switch {
	case reg.OCIChartSource() != nil:
		oci := reg.OCIChartSource()
		source = "oci|" + oci.URL
		secret = oci.Username + "|" + oci.Token
	case reg.Git != nil:
		source = "git|" + reg.Git.URL + "|" + reg.Git.Path
		secret = reg.Git.Token
	default:
		source = "unknown"
	}
	sum := sha256.Sum256([]byte(secret))
	return strings.Join([]string{reg.Name, source, moduleName, version, hex.EncodeToString(sum[:8])}, "|")
}

// sourceFS dispatches on the registry source and returns the module tree as an
// fs.FS. module.ResolveRegistry already guarantees reg is a git or OCI source
// (it rejects helm/OSS/gitee/gitlab), so only those two branches exist. Both
// converge on readerFS: git supplies the live reader, OCI a MemoryReader over the
// pulled chart. readerFS errors are wrapped with the registry name so a failing
// Application status is actionable. version applies only to the OCI branch; git
// has no tag concept and always reads its configured path off the default branch.
func (s *Service) sourceFS(ctx context.Context, reg *component.Registry, moduleName, version string) (fs.FS, error) {
	switch {
	case reg.OCIChartSource() != nil:
		return s.ociChartFS(ctx, reg, moduleName, version)
	case reg.Git != nil:
		reader, err := s.newReader(reg)
		if err != nil {
			return nil, fmt.Errorf("registry %q: build reader: %w", reg.Name, err)
		}
		fsys, err := readerFS(reader, moduleName)
		if err != nil {
			return nil, fmt.Errorf("registry %q: %w", reg.Name, err)
		}
		return fsys, nil
	default:
		return nil, fmt.Errorf("registry %q has no supported module source", reg.Name)
	}
}

// readerFS is the single source->tree adapter. It reads the module's files from
// any component.AsyncReader and assembles a mapFS keyed module-root-relative. It uses
// RelativePath (not the raw item path) because that is the reader-agnostic path
// both the live git reader and MemoryReader accept for ReadFile: the git reader
// strips its configured base, and MemoryReader returns "<module>/<rel>". Both
// forms start with "<module>/", which readerFS then strips.
func readerFS(r component.AsyncReader, moduleName string) (fs.FS, error) {
	// Scoped when the source can: listing the registry to keep one entry costs
	// an API request per directory of every other module in it.
	meta, err := component.ListPackageMeta(r, moduleName)
	if err != nil {
		if goerrors.Is(err, component.ErrPackageNotExist) {
			return nil, fmt.Errorf("module %q not found in registry: %w", moduleName, module.ErrModuleNotFound)
		}
		return nil, fmt.Errorf("list modules: %w", err)
	}
	prefix := moduleName + "/"
	files := mapFS{}
	for _, item := range meta.Items {
		if item.GetType() != component.FileType {
			continue
		}
		readPath := r.RelativePath(item)
		rel := strings.TrimPrefix(readPath, prefix)
		if rel == "" || rel == readPath {
			continue // not under the module root
		}
		content, err := r.ReadFile(readPath)
		if err != nil {
			return nil, fmt.Errorf("module %q: read %s: %w", moduleName, readPath, err)
		}
		files[rel] = []byte(content)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("module %q is empty", moduleName)
	}
	return files, nil
}

type ociChartPuller func(ctx context.Context, reg *component.Registry, moduleName, version string) ([]*loader.BufferedFile, error)

// pullModuleChart pulls the module's Helm-chart OCI artifact (same semantics as
// vela addon push) and returns its buffered files, paths prefixed by the chart
// (module) name. It reuses addon's pull verbatim; ociChartFS wraps the files in a
// MemoryReader and runs readerFS. version "" resolves the highest semver tag; a
// non-existent tag surfaces as a pull error naming the module and tag.
func pullModuleChart(ctx context.Context, reg *component.Registry, moduleName, version string) ([]*loader.BufferedFile, error) {
	buffered, err := component.PullOCIChartFiles(ctx, *reg, moduleName, version)
	if err != nil {
		return nil, fmt.Errorf("module %q: pull OCI chart: %w", moduleName, err)
	}
	return buffered, nil
}

// ociChartFS pulls the module's Helm chart and reuses readerFS by wrapping the
// buffered files in component.MemoryReader (itself a component.AsyncReader). No new
// adapter — the OCI blob just becomes a reader.
func (s *Service) ociChartFS(ctx context.Context, reg *component.Registry, moduleName, version string) (fs.FS, error) {
	bufs, err := s.pullChart(ctx, reg, moduleName, version)
	if err != nil {
		return nil, fmt.Errorf("registry %q: %w", reg.Name, err)
	}
	fsys, err := readerFS(&component.MemoryReader{Name: moduleName, Files: bufs}, moduleName)
	if err != nil {
		return nil, fmt.Errorf("registry %q: %w", reg.Name, err)
	}
	return fsys, nil
}
