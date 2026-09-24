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

package addon

import "github.com/oam-dev/kubevela/pkg/registry/component"

// The registry model, its data store and the source readers now live in
// pkg/registry/component, shared with pkg/module. These aliases keep the addon
// package's existing spelling working; new code should reach for the component
// package directly.
type (
	// Registry is the addon registry model.
	Registry = component.Registry
	// RegistryDataStore reads and writes registries.
	RegistryDataStore = component.RegistryDataStore
	// GitAddonSource is a git-backed source.
	GitAddonSource = component.GitAddonSource
	// GiteeAddonSource is a Gitee-backed source.
	GiteeAddonSource = component.GiteeAddonSource
	// GitlabAddonSource is a GitLab-backed source.
	GitlabAddonSource = component.GitlabAddonSource
	// HelmSource is a Helm-repo or OCI-backed source.
	HelmSource = component.HelmSource
	// OSSAddonSource is an OSS-backed source.
	OSSAddonSource = component.OSSAddonSource
	// TokenSource names the source holding a registry token.
	TokenSource = component.TokenSource
	// SafeCopier copies a source without its credentials.
	SafeCopier = component.SafeCopier
	// AsyncReader reads package files from a source.
	AsyncReader = component.AsyncReader
	// ReaderType marks which transport a reader speaks.
	ReaderType = component.ReaderType
	// Item is one file or directory in a source.
	Item = component.Item
	// SourceMeta is one package's file listing.
	SourceMeta = component.SourceMeta
	// OSSItem is an item in an OSS bucket.
	OSSItem = component.OSSItem
	// MemoryReader serves files from memory.
	MemoryReader = component.MemoryReader
	// ListBucketResult is an OSS bucket listing response.
	ListBucketResult = component.ListBucketResult
	// File is one entry in an OSS bucket listing.
	File = component.File
)

// Constants and functions re-exported from pkg/registry/component.
const (
	// DirType means a directory.
	DirType = component.DirType
	// FileType means a file.
	FileType = component.FileType
	// BlobType means a blob.
	BlobType = component.BlobType
	// TreeType means a tree.
	TreeType = component.TreeType
	// EOFError is the error returned by an xml parse hitting EOF.
	EOFError = component.EOFError
	// MetadataFileName is the package metadata.yaml file name.
	MetadataFileName = component.MetadataFileName
)

// Reader types, kept in the addon package's original spelling.
const (
	gitType    = component.GitType
	ossType    = component.OSSType
	giteeType  = component.GiteeType
	gitlabType = component.GitlabType
)

var (
	// NewRegistryDataStore builds the addon registry store.
	NewRegistryDataStore = component.NewRegistryDataStore
	// NewRegistryDataStoreFor builds a registry store over the given ConfigMap.
	NewRegistryDataStoreFor = component.NewRegistryDataStoreFor
	// NewAsyncReader builds a reader for a source.
	NewAsyncReader = component.NewAsyncReader
	// IsOCIURL reports whether a URL names an OCI registry.
	IsOCIURL = component.IsOCIURL
	// OCIChartRef returns the full reference a chart publishes to.
	OCIChartRef = component.OCIChartRef
	// PushOCIChart pushes a chart archive.
	PushOCIChart = component.PushOCIChart
	// OCIChartTagExists reports whether a tag is already published.
	OCIChartTagExists = component.OCIChartTagExists
	// PullOCIChartFiles pulls a chart artifact's files.
	PullOCIChartFiles = component.PullOCIChartFiles
	// IsOCIRepositoryNotFound classifies a missing-repository error.
	IsOCIRepositoryNotFound = component.IsOCIRepositoryNotFound
	// IsOCITagImmutable classifies an immutable-tag error.
	IsOCITagImmutable = component.IsOCITagImmutable
)
