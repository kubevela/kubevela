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

// Package service fetches a crate from a registry and parses it into a
// crate.Crate, server-side, for the type: crate render path. It reuses the
// shared OCI Helm-chart transport and the Registry model; it does not reuse
// the addon parsing/packaging layer.
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

	"github.com/oam-dev/kubevela/pkg/crate"
	"github.com/oam-dev/kubevela/pkg/registry/component"
)

// Service fetches crates. It resolves registries through crate.ResolveRegistry
// over a component.RegistryDataStore — reusing that story's default policy,
// source rejection, token loading, and not-found reporting.
type Service struct {
	store component.RegistryDataStore
	// pullChart is wired to the real OCI transport by NewService and
	// overridden by tests.
	pullChart ociChartPuller
	// revision names what the registry holds without reading it, so a
	// reconcile that finds it unchanged reads nothing. Nil means ask the
	// registry; tests set a fake, and one that cannot report a revision
	// returns ErrRevisionUnsupported so every fetch reads.
	revision packageRevisioner
}

// packageRevisioner names the current revision of one package in a registry.
type packageRevisioner func(ctx context.Context, reg *component.Registry, name, version, lastKnown string) (string, error)

// NewService wires the real addon transport. In production the store is
// crate.NewStore(cli) (the vela-crate-registry ConfigMap).
func NewService(store component.RegistryDataStore) *Service {
	return &Service{store: store, pullChart: pullCrateChart}
}

// FetchCrate resolves the registry, fetches the crate's files into an fs.FS,
// and parses them. Resolution is crate.ResolveRegistry: an empty name selects
// the sole registry or the "catalog" default, non-OCI sources are rejected,
// and unknown names report the configured registries (wrapping ErrRegistryNotFound).
//
// version selects the crate package version (the OCI/ECR tag vela crate
// publish writes from _crate.cue's version field).
func (s *Service) FetchCrate(ctx context.Context, registry, crateName, version string) (*crate.Crate, error) {
	reg, err := crate.ResolveRegistry(ctx, s.store, registry)
	if err != nil {
		return nil, err
	}
	// The render path runs on every reconcile of every Application that names
	// this crate, so what it costs the registry is what matters here. Reading
	// the crate is skipped entirely whenever the registry reports the same
	// revision it was read at.
	return crateCache.Load(
		crateCacheKey(&reg, crateName, version),
		func(lastKnown string) (string, error) {
			if s.revision != nil {
				return s.revision(ctx, &reg, crateName, version, lastKnown)
			}
			// An oci:// registry answers with its manifest digest. The two
			// spellings OCIChartSource also accepts, http:// and a bare host,
			// are not oci:// to OCISource, so PackageRevision reports
			// ErrRevisionUnsupported for those and the caching turns off.
			return reg.PackageRevision(ctx, crateName, version, lastKnown)
		},
		func(revision string) (*crate.Crate, error) {
			// Read at the revision that was checked, so the files parsed here
			// are the ones that revision names.
			at := reg.AtRevision(revision)
			fsys, err := s.sourceFS(ctx, &at, crateName, version)
			if err != nil {
				return nil, err
			}
			mod, err := crate.ParseCrate(fsys)
			if err != nil {
				return nil, fmt.Errorf("registry %q, crate %q: %w", reg.Name, crateName, err)
			}
			return mod, nil
		},
	)
}

// crateCache holds parsed crates by the registry revision they were read at.
// It is package scoped because production builds a Service per call
// (rendererImpl.fetch), so a cache on the Service would never see a second hit.
var crateCache = component.NewRevisionCache[*crate.Crate](component.DefaultRevisionCacheSize)

// ResetCrateCache empties the crate cache. It exists for tests, which would
// otherwise carry a crate from one case into the next.
func ResetCrateCache() { crateCache.Reset() }

// crateCacheKey identifies what was read, which is more than the crate name:
// it is that crate, at that version, out of that source, read with those
// credentials. The credentials are in the key as a digest so that rotating a
// token invalidates what the old one could see, without the secret itself
// reaching a map key, a log line, or a metric label.
func crateCacheKey(reg *component.Registry, crateName, version string) string {
	var source, secret string
	if oci := reg.OCIChartSource(); oci != nil {
		source = "oci|" + oci.URL
		secret = oci.Username + "|" + oci.Token
	} else {
		source = "unknown"
	}
	sum := sha256.Sum256([]byte(secret))
	return strings.Join([]string{reg.Name, source, crateName, version, hex.EncodeToString(sum[:8])}, "|")
}

// sourceFS returns the crate tree as an fs.FS. crate.ResolveRegistry already
// guarantees reg is an OCI source (it rejects git, gitee, gitlab, helm and
// OSS), so there is one branch; the default stays as a guard against a resolver
// that admitted something it should not have.
func (s *Service) sourceFS(ctx context.Context, reg *component.Registry, crateName, version string) (fs.FS, error) {
	if reg.OCIChartSource() != nil {
		return s.ociChartFS(ctx, reg, crateName, version)
	}
	return nil, fmt.Errorf("registry %q has no supported crate source", reg.Name)
}

// readerFS is the source->tree adapter. It reads the crate's files from a
// component.AsyncReader and assembles a MapFS keyed crate-root-relative. It
// uses RelativePath (not the raw item path) because that is the reader-agnostic
// path MemoryReader accepts for ReadFile: it returns "<crate>/<rel>", which
// starts with "<crate>/", and readerFS then strips that prefix.
func readerFS(r component.AsyncReader, crateName string) (fs.FS, error) {
	// Scoped when the source can: listing the registry to keep one entry costs
	// an API request per directory of every other crate in it.
	meta, err := component.ListPackageMeta(r, crateName)
	if err != nil {
		if goerrors.Is(err, component.ErrPackageNotExist) {
			return nil, fmt.Errorf("crate %q not found in registry: %w", crateName, crate.ErrCrateNotFound)
		}
		return nil, fmt.Errorf("list crates: %w", err)
	}
	prefix := crateName + "/"
	files := MapFS{}
	for _, item := range meta.Items {
		if item.GetType() != component.FileType {
			continue
		}
		readPath := r.RelativePath(item)
		rel := strings.TrimPrefix(readPath, prefix)
		if rel == "" || rel == readPath {
			continue // not under the crate root
		}
		content, err := r.ReadFile(readPath)
		if err != nil {
			return nil, fmt.Errorf("crate %q: read %s: %w", crateName, readPath, err)
		}
		files[rel] = []byte(content)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("crate %q is empty", crateName)
	}
	return files, nil
}

type ociChartPuller func(ctx context.Context, reg *component.Registry, crateName, version string) ([]*loader.BufferedFile, error)

// pullCrateChart pulls the crate's Helm-chart OCI artifact (same semantics as
// vela addon push) and returns its buffered files, paths prefixed by the chart
// (crate) name. It reuses addon's pull verbatim; ociChartFS wraps the files in a
// MemoryReader and runs readerFS. version "" resolves the highest semver tag; a
// non-existent tag surfaces as a pull error naming the crate and tag.
func pullCrateChart(ctx context.Context, reg *component.Registry, crateName, version string) ([]*loader.BufferedFile, error) {
	buffered, err := component.PullOCIChartFiles(ctx, *reg, crateName, version)
	if err != nil {
		return nil, fmt.Errorf("crate %q: pull OCI chart: %w", crateName, err)
	}
	return buffered, nil
}

// ociChartFS pulls the crate's Helm chart and reuses readerFS by wrapping the
// buffered files in component.MemoryReader (itself a component.AsyncReader). No new
// adapter — the OCI blob just becomes a reader.
func (s *Service) ociChartFS(ctx context.Context, reg *component.Registry, crateName, version string) (fs.FS, error) {
	bufs, err := s.pullChart(ctx, reg, crateName, version)
	if err != nil {
		// Each crate is its own repository in an OCI registry, so a crate
		// that was never published is a missing repository, which the tag
		// listing reports before any pull. Say that rather than pass on the
		// registry's 404.
		if component.IsOCIRepositoryNotFound(err) {
			return nil, fmt.Errorf("registry %q: crate %q is not published to this registry: %w",
				reg.Name, crateName, crate.ErrCrateNotFound)
		}
		return nil, fmt.Errorf("registry %q: %w", reg.Name, err)
	}
	fsys, err := readerFS(&component.MemoryReader{Name: crateName, Files: bufs}, crateName)
	if err != nil {
		return nil, fmt.Errorf("registry %q: %w", reg.Name, err)
	}
	return fsys, nil
}
