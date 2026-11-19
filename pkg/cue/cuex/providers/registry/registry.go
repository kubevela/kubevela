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

// Package registry provides a CueX provider that reads a file out of a
// configured addon registry.
//
// It exists so a SourceDefinition can pull a file from git without carrying a
// URL and a credential of its own: the registry is named, looked up in the
// cluster's registry ConfigMap, and read with whatever auth it was registered
// with. That keeps repository credentials a platform concern, which is the same
// separation SourceDefinitions rest on everywhere else.
package registry

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	stdpath "path"
	"strings"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"

	di "github.com/oam-dev/kubevela/pkg/registry"
	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
)

// FileReader reads one file out of a named registry.
//
// The interface lives here and the implementation does not, because reaching
// pkg/addon from this package closes a cycle: cuex -> providers/registry ->
// addon -> config -> cue/script -> cuex. The implementation is registered at
// startup by cmd/core/app/bootstrap.go, which can import both sides.
//
// It also makes the provider testable without a cluster: a test registers its
// own reader.
type FileReader interface {
	// ReadFile reads one file. An empty ref means the registry's own default.
	ReadFile(ctx context.Context, registryName, path, ref string) (string, error)
}

// ReadFileVars are the inputs to a registry file read.
type ReadFileVars struct {
	// Registry names a registry configured in this cluster.
	Registry string `json:"registry"`
	// Path is the file's path within that registry, relative to the registry's
	// own root - the same relative path `vela registry` reads addons from.
	Path string `json:"path"`
	// Ref reads at a specific branch, tag or commit. Empty means whatever the
	// registry's own URL pinned, which is the platform's default.
	Ref string `json:"ref,omitempty"`
}

// ReadFileResult is the file's contents, and whether there was a file at all.
type ReadFileResult struct {
	Content string `json:"content"`
	// Found reports whether the file exists. A missing file is an ordinary
	// answer here rather than an error, matching the http provider's habit of
	// handing back statusCode instead of failing on a 404: a source that reads
	// an optional per-cluster override wants to fall back, not to fail.
	//
	// Content is always empty when Found is false. Nothing is invented.
	Found bool `json:"found"`
}

// ReadFileParams is the params for a registry file read.
type ReadFileParams providers.Params[ReadFileVars]

// ReadFileReturns is the returns for a registry file read.
type ReadFileReturns providers.Returns[ReadFileResult]

// ReadFile fetches one file from a named registry.
//
// Absence and failure are told apart. A file the registry does not have comes
// back as found: false with empty content and no error, so a template can
// branch on it. Everything else - an unknown registry, a rejected credential, a
// network that is down, no reader wired up at all - is an error, because
// resolving those to an empty string would cache the emptiness and it would
// afterwards look like data.
//
// The distinction rests on the reader wrapping errors.ErrFileNotFound; a reader
// that does not is treated as failing, which is the safe direction to be wrong
// in.
func ReadFile(ctx context.Context, params *ReadFileParams) (*ReadFileReturns, error) {
	in := params.Params
	if in.Registry == "" {
		return nil, fmt.Errorf("registry is required")
	}
	if in.Path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if err := validateRegistryPath(in.Path); err != nil {
		return nil, err
	}

	reader, ok := di.Get[FileReader]()
	if !ok {
		// The binary did not wire one up. Saying so plainly beats a nil
		// dereference, and points at the one place that fixes it.
		return nil, fmt.Errorf("no registry file reader is registered; " +
			"cmd/core/app/bootstrap.go should register one")
	}

	content, err := reader.ReadFile(ctx, in.Registry, in.Path, in.Ref)
	switch {
	case errors.Is(err, velaerrors.ErrFileNotFound):
		return &ReadFileReturns{Returns: ReadFileResult{Found: false}}, nil
	case err != nil:
		at := ""
		if in.Ref != "" {
			at = fmt.Sprintf(" at %q", in.Ref)
		}
		return nil, fmt.Errorf("registry %q: reading %q%s: %w", in.Registry, in.Path, at, err)
	}

	return &ReadFileReturns{Returns: ReadFileResult{Content: content, Found: true}}, nil
}

// validateRegistryPath keeps a read inside the registry it names.
//
// A path is relative to the registry's configured root, so an absolute one or a
// path that climbs out of it is a read of somewhere else. What "somewhere else"
// means depends on the backend, and it is not this provider's business to know
// which backends happen to be safe.
func validateRegistryPath(path string) error {
	if strings.HasPrefix(path, "/") {
		return fmt.Errorf("path %q must be relative to the registry root", path)
	}
	clean := stdpath.Clean(path)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("path %q escapes the registry root", path)
	}
	return nil
}

// ProviderName is the name a template references this provider by.
const ProviderName = "registry"

//go:embed registry.cue
var template string

// Package is the provider package, registered with the compilers that should
// offer it.
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"read-file": cuexruntime.GenericProviderFn[ReadFileParams, ReadFileReturns](ReadFile),
}))
