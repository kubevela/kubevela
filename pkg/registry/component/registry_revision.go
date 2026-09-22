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
	"context"
	"fmt"
	"strings"
)

// ErrRevisionUnsupported means this registry source cannot name its revision
// cheaply, so a caller has to read the package to know what it holds.
var ErrRevisionUnsupported = NewError("registry source cannot report a revision")

// PackageRevision names what the registry currently holds for one package,
// without reading the package. Two callers of this in a row cost one small
// request between them, or none at all when the source can revalidate
// conditionally.
//
// lastKnown is the revision the caller holds, or empty. When it is still
// current it is returned unchanged.
//
// The returned string is opaque and only ever compared for equality. It is not
// a version: a git registry reports a commit, an OCI registry a tag and the
// digest behind it, and neither is ordered.
func (r *Registry) PackageRevision(ctx context.Context, name, version, lastKnown string) (string, error) {
	// OCISource, not OCIChartSource: the latter deliberately widens to
	// scheme-less and http:// URLs for the module chart path, and a plain Helm
	// repository served over HTTP matches that spelling too. Asking such a
	// server for a manifest digest fails, and failing a revision probe is worse
	// than not having one -- so only an unambiguous oci:// endpoint takes this
	// branch.
	if oci := r.OCISource(); oci != nil {
		return r.ociPackageRevision(ctx, oci, name, version, lastKnown)
	}
	// Nothing else can name a revision. A Helm chart repository has no
	// equivalent of a manifest digest, and its index is the thing a caller
	// would have to fetch anyway; the git, gitee, gitlab and OSS readers report
	// none, so a caller that caches on revisions reads every time for those.
	return "", ErrRevisionUnsupported
}

// ociPackageRevision is the resolved tag and the digest behind it. Both belong
// in the key: the digest alone would miss a move from one tag to another when
// the caller asked for no particular version, and the tag alone would miss a
// re-push of the same tag, which is the ordinary way a module is corrected.
func (r *Registry) ociPackageRevision(ctx context.Context, oci *HelmSource, name, version, lastKnown string) (string, error) {
	repoRef, host := OCIRepoRef(oci.URL, name)
	tag, err := resolveOCITag(ctx, repoRef, host, oci.Username, oci.Token, version)
	if err != nil {
		return "", err
	}
	// Only offer the digest half back to the registry, and only if it was read
	// for this same tag -- a conditional request carrying another tag's digest
	// would be answered 200 anyway, and carrying a composite string would be
	// answered 200 for a digest the registry never issued.
	lastDigest := ""
	if lastTag, digest, found := strings.Cut(lastKnown, "@"); found && lastTag == tag {
		lastDigest = digest
	}
	digest, err := OCIManifestDigest(ctx, repoRef, host, oci.Username, oci.Token, tag, lastDigest, false)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s@%s", tag, digest), nil
}

// ListPackageMeta lists one package's files by listing the registry and picking
// the one entry out.
func (r *Registry) ListPackageMeta(name string) (SourceMeta, error) {
	reader, err := r.BuildReader()
	if err != nil {
		return SourceMeta{}, err
	}
	return ListPackageMeta(reader, name)
}

// ListPackageMeta lists one package from a reader.
func ListPackageMeta(reader AsyncReader, name string) (SourceMeta, error) {
	// Rejected before the listing rather than after: a name that cannot be a
	// package is not going to be a key in the result, and the listing is a
	// request per directory in the registry.
	if !IsPackageName(name) {
		return SourceMeta{}, fmt.Errorf("%q: %w", name, ErrPackageNotExist)
	}
	metas, err := reader.ListAddonMeta()
	if err != nil {
		return SourceMeta{}, err
	}
	meta, ok := metas[name]
	if !ok {
		return SourceMeta{}, fmt.Errorf("%q: %w", name, ErrPackageNotExist)
	}
	return meta, nil
}
