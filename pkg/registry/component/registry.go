/*
Copyright 2025 The KubeVela Authors.

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
	"strings"

	"github.com/oam-dev/kubevela/pkg/addon"
)

// Registry is the addon registry model, reused verbatim so that a module
// publishes to and is pulled from the same registries `vela addon registry`
// already manages. Modules deliberately do not carry a registry model of their
// own: a registry is a location, not an addon- or module-specific concept.
type Registry = addon.Registry

// RegistryDataStore reads and writes registries.
type RegistryDataStore = addon.RegistryDataStore

// NewRegistryDataStoreFor builds a registry store over the given ConfigMap and
// token-secret prefix, which is how the module registry keeps its entries and
// credentials separate from the addon registry's.
var NewRegistryDataStoreFor = addon.NewRegistryDataStoreFor

// OCIChartSource returns the source holding the OCI registry a module chart is
// published to and pulled from, or nil when reg is not OCI-backed.
func OCIChartSource(reg Registry) *addon.HelmSource { return ociChartSource(reg) }

// The source readers are shared with the addon registry: a module and an addon
// are fetched from the same kinds of location (git, OCI, OSS, local), so they
// read them the same way rather than each carrying its own transport.
type (
	// AsyncReader reads package files from a source.
	AsyncReader = addon.AsyncReader
	// MemoryReader serves files from memory.
	MemoryReader = addon.MemoryReader
	// Item is one file or directory in a source.
	Item = addon.Item
	// SourceMeta is one package's file listing.
	SourceMeta = addon.SourceMeta
	// HelmSource is a Helm-repo or OCI-backed source.
	HelmSource = addon.HelmSource
	// GitAddonSource is a git-backed source.
	GitAddonSource = addon.GitAddonSource
)

const (
	// FileType means a file.
	FileType = addon.FileType
	// DirType means a directory.
	DirType = addon.DirType
)

// NewAsyncReader builds a reader for a source.
var NewAsyncReader = addon.NewAsyncReader

// ociChartSource returns the source holding the OCI registry a module chart is
// published to and pulled from, or nil when reg is not OCI-backed.
//
// A registry records OCI and plain Helm repositories in the same Helm source
// (an oci:// URL is just another Helm URL), so being OCI-backed is a property
// of the URL rather than of a dedicated field. Three spellings qualify:
//
//   - oci://, the canonical form;
//   - http://, an in-process test registry, which is reached over plain HTTP
//     rather than being silently downgraded from TLS;
//   - scheme-less, how registry hosts are conventionally written, e.g.
//     "123456789012.dkr.ecr.us-west-2.amazonaws.com/modules".
//
// An https:// URL is a plain Helm chart repository and is deliberately not
// treated as OCI-backed.
func ociChartSource(reg Registry) *addon.HelmSource {
	if reg.Helm == nil || reg.Helm.URL == "" {
		return nil
	}
	if addon.IsOCIURL(reg.Helm.URL) || ociURLIsPlainHTTP(reg.Helm.URL) || !strings.Contains(reg.Helm.URL, "://") {
		return reg.Helm
	}
	return nil
}
