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

package addon

import (
	"context"

	"github.com/Masterminds/semver/v3"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
)

const (
	// velaSystemRequirement is the vela version requirement annotation key
	velaSystemRequirement = `system.vela`
	// kubernetesSystemRequirement is the kubernetes requirement annotation key
	kubernetesSystemRequirement = `system.kubernetes`
	// addonSystemRequirement is the annotation key to identity an addon from helm chart structure
	addonSystemRequirement = `addon.name`
)

// VersionedRegistry is the interface of support version registry
type VersionedRegistry interface {
	ListAddon() ([]*UIData, error)
	GetAddonUIData(ctx context.Context, addonName, version string) (*UIData, error)
	GetAddonInstallPackage(ctx context.Context, addonName, version string) (*InstallPackage, error)
	GetDetailedAddon(ctx context.Context, addonName, version string) (*WholeAddonPackage, error)
	// GetAddonAvailableVersion returns the addon's versions, newest first.
	GetAddonAvailableVersion(addonName string) ([]*repo.ChartVersion, error)
}

// chartBackend is the transport half of a chart-backed addon registry. Only
// discovery, version enumeration, and archive retrieval differ between an
// indexed HTTP Helm repository and an OCI registry; everything after the
// archive is shared and lives on helmRegistry.
type chartBackend interface {
	// ListUIData enumerates the registry: index entries over HTTP, the portable
	// catalog (or the distribution catalog) over OCI.
	listUIData(ctx context.Context) ([]*UIData, error)
	// Versions enumerates one addon's versions, newest first.
	versions(ctx context.Context, addonName string) ([]*repo.ChartVersion, error)
	// Resolve selects a version and returns its decoded chart files.
	resolve(ctx context.Context, addonName, version string) (*resolvedChart, error)
	// supportsVersionRequirements reports whether the values from Versions carry
	// the annotations SystemRequirements are read from. A backend that answers
	// false must not be asked which version meets a requirement: it would answer
	// "the newest one" for every requirement.
	supportsVersionRequirements() bool
}

// errorClassifier is an optional chartBackend capability. A backend implements
// it when its load failures need translating into the shared error vocabulary,
// which is how installDependency decides whether to try the next registry. The
// two transports deliberately differ here, so this is opt-in rather than part
// of chartBackend.
type errorClassifier interface {
	classify(error) error
}

// resolvedChart is one addon version fetched from a backend, described in terms
// the facade can act on without knowing how it arrived.
type resolvedChart struct {
	// files is the decoded chart archive. Decoding belongs to the backend
	// because the HTTP backend walks several candidate chart URLs and treats a
	// file that will not decode as a reason to try the next one.
	files []*loader.BufferedFile
	// version is the version actually selected, which differs from the version
	// asked for whenever the caller asked for the latest.
	version string
	// availableVersions is every version the backend saw while resolving. It may
	// be empty when a backend resolved a pinned version without enumerating, as
	// the OCI backend does.
	availableVersions []string
	// requirements is applied only when requirementsSet is true, so a backend
	// that reads requirements from transport metadata can say "none declared"
	// without being confused with a backend that has no such metadata at all.
	requirements    *SystemRequirements
	requirementsSet bool
}

// helmRegistry serves addons packaged as Helm charts, over whichever transport
// its backend speaks. Package decoding, UIData projection, and the registry and
// version stamping happen here once rather than once per transport.
type helmRegistry struct {
	name    string
	backend chartBackend
}

// ListAddon lists every addon the registry advertises.
func (r *helmRegistry) ListAddon() ([]*UIData, error) {
	addons, err := r.backend.listUIData(context.Background())
	if err != nil {
		return nil, err
	}
	for _, addon := range addons {
		addon.RegistryName = r.name
	}
	return addons, nil
}

// GetAddonUIData returns the addon's UI-facing metadata.
func (r *helmRegistry) GetAddonUIData(ctx context.Context, addonName, version string) (*UIData, error) {
	wholePackage, err := r.loadAddon(ctx, addonName, version)
	if err != nil {
		return nil, err
	}
	return uiDataFromPackage(wholePackage), nil
}

// uiDataFromPackage projects the UI-facing subset of a loaded addon package.
// A backend that has to build a listing entry from a real chart uses this too,
// so the projection has one definition rather than one per caller.
func uiDataFromPackage(pkg *WholeAddonPackage) *UIData {
	return &UIData{
		Meta:              pkg.Meta,
		APISchema:         pkg.APISchema,
		Parameters:        pkg.Parameters,
		Detail:            pkg.Detail,
		Definitions:       pkg.Definitions,
		AvailableVersions: pkg.AvailableVersions,
		CUEDefinitions:    pkg.CUEDefinitions,
	}
}

// GetAddonInstallPackage returns the addon's install package.
func (r *helmRegistry) GetAddonInstallPackage(ctx context.Context, addonName, version string) (*InstallPackage, error) {
	wholePackage, err := r.loadAddon(ctx, addonName, version)
	if err != nil {
		return nil, err
	}
	return &wholePackage.InstallPackage, nil
}

// GetDetailedAddon returns the whole addon package.
func (r *helmRegistry) GetDetailedAddon(ctx context.Context, addonName, version string) (*WholeAddonPackage, error) {
	return r.loadAddon(ctx, addonName, version)
}

// GetAddonAvailableVersion lists the addon's versions, newest first.
func (r *helmRegistry) GetAddonAvailableVersion(addonName string) ([]*repo.ChartVersion, error) {
	return r.backend.versions(context.Background(), addonName)
}

// supportsVersionRequirements exposes the backend capability to callers that
// pick a version by system requirement.
func (r *helmRegistry) supportsVersionRequirements() bool {
	return r.backend.supportsVersionRequirements()
}

func (r *helmRegistry) loadAddon(ctx context.Context, addonName, version string) (pkg *WholeAddonPackage, err error) {
	if classifier, ok := r.backend.(errorClassifier); ok {
		defer func() {
			if err != nil {
				err = classifier.classify(err)
			}
		}()
	}

	resolved, err := r.backend.resolve(ctx, addonName, version)
	if err != nil {
		return nil, err
	}
	pkg, err = loadAddonPackage(addonName, resolved.files)
	if err != nil {
		return nil, err
	}
	pkg.RegistryName = r.name
	// The archive knows nothing about its sibling versions, so without this the
	// UI would show every addon as having exactly one version.
	pkg.AvailableVersions = resolved.availableVersions
	if resolved.requirementsSet {
		pkg.SystemRequirements = resolved.requirements
	}
	if pkg.Name != "" {
		klog.V(5).Infof("Addon '%s' with version '%s' loaded successfully from registry '%s'", addonName, resolved.version, r.name)
	}
	return pkg, nil
}

// BuildVersionedRegistry builds a versioned addon registry backed by an indexed
// HTTP Helm repository.
func BuildVersionedRegistry(name, repoURL string, opts *common.HTTPOption) VersionedRegistry {
	return &helmRegistry{
		name: name,
		backend: &httpHelmBackend{
			name: name,
			url:  repoURL,
			h:    helm.NewHelperWithCache(),
			opts: opts,
		},
	}
}

// NewVersionedRegistry builds a chart-backed addon registry from a Helm source,
// selecting the transport from the URL scheme.
//
// Only oci:// is dispatched specially. Everything else keeps the indexed HTTP
// path, because the scheme is not a reliable allowlist here: registries are
// stored with cm:// as well as http(s)://, and rejecting an unrecognised scheme
// would break records that work today.
func NewVersionedRegistry(name string, source *HelmSource) (VersionedRegistry, error) {
	if source == nil {
		return nil, errors.Errorf("addon registry %s has no chart repository configured", name)
	}
	if err := source.validateCredential(); err != nil {
		// Wrapped as ErrFetch so isSkippableRegistryError treats it as "this one
		// registry cannot serve addons". Without that, one hand-edited record
		// aborts listAvailableAddons and installDependency for every registry.
		return nil, errors.Wrapf(ErrFetch, "addon registry %s: %v", name, err)
	}
	username, secret := source.credential()
	if IsOCIURL(source.URL) {
		return &helmRegistry{
			name: name,
			backend: &ociHelmBackend{
				name:           name,
				url:            source.URL,
				username:       username,
				token:          secret,
				pullFn:         pullOCIChart,
				tagsFn:         listOCITags,
				catalogFn:      listOCIRepositories,
				catalogIndexFn: listPortableOCICatalog,
			},
		}, nil
	}
	return &helmRegistry{
		name: name,
		backend: &httpHelmBackend{
			name: name,
			url:  source.URL,
			h:    helm.NewHelperWithCache(),
			opts: &common.HTTPOption{
				Username:        username,
				Password:        secret,
				InsecureSkipTLS: source.InsecureSkipTLS,
			},
		},
	}, nil
}

// ToVersionedRegistry converts registry to versioned registry
func ToVersionedRegistry(registry Registry) (VersionedRegistry, error) {
	if !IsVersionRegistry(registry) {
		return nil, errors.Errorf("registry '%s' is not a versioned registry", registry.Name)
	}
	return NewVersionedRegistry(registry.Name, registry.Helm)
}

func loadAddonPackage(addonName string, files []*loader.BufferedFile) (*WholeAddonPackage, error) {
	mr := MemoryReader{Name: addonName, Files: files}
	metas, err := mr.ListAddonMeta()
	if err != nil {
		return nil, err
	}
	meta := metas[addonName]
	addonUIData, err := GetUIDataFromReader(&mr, &meta, UIMetaOptions)
	if err != nil {
		return nil, err
	}
	installPackage, err := GetInstallPackageFromReader(&mr, &meta, addonUIData)
	if err != nil {
		return nil, err
	}
	return &WholeAddonPackage{
		InstallPackage: *installPackage,
		Detail:         addonUIData.Detail,
		APISchema:      addonUIData.APISchema,
	}, nil
}

// chooseVersion will return the target version and all available versions
// This function is not sensitive to v-prefix, which means if specifiedVersion=0.3.0, v0.3.0 can be chosen.
func chooseVersion(specifiedVersion string, versions []*repo.ChartVersion) (*repo.ChartVersion, []string) {
	var addonVersion *repo.ChartVersion
	var availableVersions []string
	for i, v := range versions {
		availableVersions = append(availableVersions, v.Version)
		if addonVersion != nil {
			// already find the latest not-prerelease version, skip the find
			continue
		}
		if len(specifiedVersion) != 0 {
			if utils.IgnoreVPrefix(v.Version) == utils.IgnoreVPrefix(specifiedVersion) {
				addonVersion = versions[i]
			}
		} else {
			vv, err := semver.NewVersion(v.Version)
			if err != nil {
				continue
			}
			if len(vv.Prerelease()) != 0 {
				continue
			}
			addonVersion = v
		}
	}
	return addonVersion, availableVersions
}

// LoadSystemRequirements load the system version requirements from the addon's meta file
func LoadSystemRequirements(anno map[string]string) *SystemRequirements {
	if len(anno) == 0 {
		return nil
	}
	req := &SystemRequirements{}
	if _, ok := anno[velaSystemRequirement]; ok {
		req.VelaVersion = anno[velaSystemRequirement]
	}
	if _, ok := anno[kubernetesSystemRequirement]; ok {
		req.KubernetesVersion = anno[kubernetesSystemRequirement]
	}
	return req
}
