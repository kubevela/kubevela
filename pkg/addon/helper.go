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
	"crypto/sha256"
	"encoding/hex"
	goerrors "errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/registry/component"
	"github.com/oam-dev/kubevela/pkg/utils"
	addonutil "github.com/oam-dev/kubevela/pkg/utils/addon"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

const (
	// disabled indicates the addon is disabled
	disabled = "disabled"
	// enabled indicates the addon is enabled
	enabled = "enabled"
	// enabling indicates the addon is enabling
	enabling = "enabling"
	// disabling indicates the addon related app is deleting
	disabling = "disabling"
	// suspend indicates the addon related app is suspended
	suspend = "suspend"
)

// EnableAddon will enable addon with dependency check, source is where addon from.
func EnableAddon(ctx context.Context, name string, version string, cli client.Client, discoveryClient *discovery.DiscoveryClient, apply apply.Applicator, config *rest.Config, r Registry, args map[string]interface{}, cache *Cache, registries []Registry, opts ...InstallOption) (string, error) {
	h := NewAddonInstaller(ctx, cli, discoveryClient, apply, config, &r, args, cache, registries, opts...)
	pkg, err := h.loadInstallPackage(name, version)
	if err != nil {
		return "", err
	}
	if err := validateAddonPackage(pkg); err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("failed to enable addon: %s", name))
	}
	return h.enableAddon(ctx, pkg)
}

// DisableAddon will disable addon from cluster.
func DisableAddon(ctx context.Context, cli client.Client, name string, config *rest.Config, force bool) error {
	app, err := FetchAddonRelatedApp(ctx, cli, name)
	// if app not exist, report error
	if err != nil {
		return err
	}

	if !force {
		var usingAddonApp []v1beta1.Application
		usingAddonApp, err = checkAddonHasBeenUsed(ctx, cli, name, *app, config)
		if err != nil {
			return err
		}
		if len(usingAddonApp) != 0 {
			return errors.New(appsDependsOnAddonErrInfo(usingAddonApp))
		}
	}

	return cli.Delete(ctx, app)
}

// EnableAddonByLocalDir enable an addon from local dir
// The allowGoDefOverride parameter allows Go definitions to override CUE definitions when conflicts are detected
func EnableAddonByLocalDir(ctx context.Context, name string, dir string, cli client.Client, dc *discovery.DiscoveryClient, applicator apply.Applicator, config *rest.Config, args map[string]interface{}, allowGoDefOverride bool, opts ...InstallOption) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	r := component.NewLocalReader(absDir, name)
	metas, err := r.ListAddonMeta()
	if err != nil {
		return "", err
	}
	meta := metas[r.Name()]
	UIData, err := GetUIDataFromReader(r, &meta, UIMetaOptions)
	if err != nil {
		return "", err
	}
	pkg, err := GetInstallPackageFromReader(r, &meta, UIData)
	if err != nil {
		return "", err
	}

	// Compile Go definitions if godef/ folder exists
	if HasGoDefFolder(absDir) {
		compiledDefs, err := CompileGoDefinitionsFromAddon(ctx, absDir)
		if err != nil {
			return "", errors.Wrap(err, "failed to compile Go definitions")
		}

		// Check for conflicts between CUE and Go definitions
		conflicts := DetectDefinitionConflicts(pkg.CUEDefinitions, compiledDefs)
		if len(conflicts) > 0 {
			if !allowGoDefOverride {
				return "", fmt.Errorf("definition name conflicts detected between definitions/ and godef/: %v. "+
					"Use --override-definitions flag to allow Go definitions to override CUE definitions", conflicts)
			}
			// Remove conflicting CUE definitions when override is allowed
			pkg.CUEDefinitions = removeConflictingDefinitions(pkg.CUEDefinitions, conflicts)
		}

		// Merge compiled Go definitions with existing CUE definitions
		pkg.CUEDefinitions = append(pkg.CUEDefinitions, compiledDefs...)
	}

	if err := validateAddonPackage(pkg); err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("failed to enable addon by local dir: %s", dir))
	}
	h := NewAddonInstaller(ctx, cli, dc, applicator, config, &Registry{Name: LocalAddonRegistryName}, args, nil, nil, opts...)
	needEnableAddonNames, err := h.checkDependency(pkg)
	if err != nil {
		return "", err
	}
	if len(needEnableAddonNames) > 0 {
		return "", fmt.Errorf("you must first enable dependencies: %v", needEnableAddonNames)
	}
	return h.enableAddon(ctx, pkg)
}

// removeConflictingDefinitions removes definitions from the list that match the conflicting names
func removeConflictingDefinitions(definitions []ElementFile, conflicts []string) []ElementFile {
	conflictMap := make(map[string]bool)
	for _, name := range conflicts {
		conflictMap[name] = true
	}

	var result []ElementFile
	for _, def := range definitions {
		// The same extraction DetectDefinitionConflicts used to produce these
		// names. A second, near-identical implementation lived here and skipped
		// the type-prefix step, so "definitions/trait-fluxcd.cue" was flagged as
		// a conflict on "fluxcd" and then looked up as "trait-fluxcd": every
		// type-prefixed definition was detected and never removed.
		name := extractDefinitionName(def)
		if !conflictMap[name] {
			result = append(result, def)
		}
	}
	return result
}

// GetAddonStatus is general func for cli and apiServer get addon status
func GetAddonStatus(ctx context.Context, cli client.Client, name string) (Status, error) {
	var addonStatus Status
	joinedClusters, err := multicluster.NewClusterClient(cli).List(ctx)
	if err != nil {
		return addonStatus, errors.Wrap(err, "failed to list registered clusters")
	}
	var joinedClusterMap = make(map[string]bool)
	for _, joinedCluster := range joinedClusters.Items {
		joinedClusterMap[joinedCluster.Name] = true
	}

	app, err := FetchAddonRelatedApp(ctx, cli, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			addonStatus.AddonPhase = disabled
			return addonStatus, nil
		}
		return addonStatus, err
	}
	labels := app.GetLabels()
	addonStatus.AppStatus = &app.Status
	addonStatus.InstalledVersion = labels[oam.LabelAddonVersion]
	addonStatus.InstalledRegistry = labels[oam.LabelAddonRegistry]

	var clusters = make(map[string]map[string]interface{})
	for _, r := range app.Status.AppliedResources {
		if r.Cluster == "" {
			r.Cluster = multicluster.ClusterLocalName
		}
		// TODO(wonderflow): we should collect all the necessary information as observability, currently we only collect cluster name
		// If cluster is not registered in KubeVela then skip it.
		if joinedClusterMap[r.Cluster] {
			clusters[r.Cluster] = make(map[string]interface{})
		}
	}

	addonStatus.Clusters = clusters

	if app.Status.Workflow != nil && app.Status.Workflow.Suspend {
		addonStatus.AddonPhase = suspend
		return addonStatus, nil
	}

	// Get addon parameters
	var sec v1.Secret
	err = cli.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: addonutil.Addon2SecName(name)}, &sec)
	if err != nil {
		// Not found error can be ignored. Others can't.
		if !apierrors.IsNotFound(err) {
			return addonStatus, err
		}
	} else {
		// Although normally `else` is not preferred, we must use `else` here.
		args, err := FetchArgsFromSecret(&sec)
		if err != nil {
			return addonStatus, err
		}
		addonStatus.Parameters = args
	}

	switch app.Status.Phase {
	case commontypes.ApplicationRunning:
		addonStatus.AddonPhase = enabled
		return addonStatus, nil
	case commontypes.ApplicationDeleting:
		addonStatus.AddonPhase = disabling
		return addonStatus, nil
	default:
		addonStatus.AddonPhase = enabling
		return addonStatus, nil
	}
}

// FindAddonPackagesDetailFromRegistry find addons' WholeInstallPackage from registries, empty registryName indicates matching all
func FindAddonPackagesDetailFromRegistry(ctx context.Context, k8sClient client.Client, addonNames []string, registryNames []string) ([]*WholeAddonPackage, error) {
	var addons []*WholeAddonPackage
	var registries []Registry

	if len(addonNames) == 0 {
		return nil, fmt.Errorf("no addon name specified")
	}
	registryDataStore := NewRegistryDataStore(k8sClient)

	// Find matched registries
	if len(registryNames) == 0 {
		// Empty registryNames will match all registries
		regs, err := registryDataStore.ListRegistries(ctx)
		if err != nil {
			return nil, err
		}
		registries = regs
	} else {
		// Only match specified registries
		for _, registryName := range registryNames {
			r, err := registryDataStore.GetRegistry(ctx, registryName)
			if err != nil {
				continue
			}
			registries = append(registries, r)
		}
	}

	if len(registries) == 0 {
		return nil, ErrRegistryNotExist
	}

	// Found addons, for deduplication purposes
	// Registries are searched in order, so the first one holding an addon wins.
	// Appending a second copy from a later registry would leave callers that take
	// addons[0] resolving against one registry while the duplicate advertises
	// another.
	//
	// That makes the result only as stable as the registry order, which is why
	// RegistryDataStore.ListRegistries sorts rather than iterating its decoded
	// map -- otherwise addons[0] would switch registries between calls.
	foundAddons := make(map[string]bool)
	// Why a registry did not yield the addon, so that an empty result can say
	// more than "addon not exist".
	var lookupErrs []error
	merge := func(addon *WholeAddonPackage) {
		if foundAddons[addon.Name] {
			return
		}
		foundAddons[addon.Name] = true
		addons = append(addons, addon)
	}

	// Find matched addons in registries
	for _, r := range registries {
		switch {
		case IsVersionRegistry(r):
			vr, err := ToVersionedRegistry(r)
			if err != nil {
				klog.Warningf("cannot read addon registry %q: %v", r.Name, err)
				continue
			}
			for _, addonName := range addonNames {
				wholePackage, err := readVersionedAddonPackage(ctx, r, vr, addonName)
				if err != nil {
					// Log rather than silently swallow: a chart pull failure
					// (missing version, auth, media type) otherwise surfaces to
					// the caller only as the misleading "addon not exist".
					klog.Warningf("failed to load addon %q from registry %q: %v", addonName, r.Name, err)
					lookupErrs = append(lookupErrs, fmt.Errorf("addon %q in registry %q: %w", addonName, r.Name, err))
					continue
				}
				merge(wholePackage)
			}
		default:
			// Every failure below has to be recorded. A git or OSS registry that
			// could not be listed, or an addon whose files could not be read,
			// otherwise reaches the caller only as the misleading "addon not
			// exist" -- the same trap the versioned branch above already avoids.
			// A rate-limited or unauthorized private repository looks exactly
			// like an absent addon from here.
			for _, addonName := range addonNames {
				wholePackage, err := readAddonPackage(ctx, r, addonName)
				switch {
				case goerrors.Is(err, component.ErrPackageNotExist):
					// This registry does not carry it; a later one may.
					continue
				case err != nil:
					klog.Warningf("failed to read addon %q from registry %q: %v", addonName, r.Name, err)
					lookupErrs = append(lookupErrs, fmt.Errorf("addon %q in registry %q: %w", addonName, r.Name, err))
					continue
				}
				merge(wholePackage)
			}
		}
	}

	if len(addons) == 0 {
		if len(lookupErrs) > 0 {
			// Wrapped, not replaced: callers test for ErrNotExist with errors.Is.
			return nil, fmt.Errorf("%w: %w", ErrNotExist, goerrors.Join(lookupErrs...))
		}
		return nil, ErrNotExist
	}

	return addons, nil
}

// ValidateSystemRequirements checks an addon's SystemRequirements (vela and
// kubernetes versions) against the running environment. nil require passes.
func ValidateSystemRequirements(ctx context.Context, require *SystemRequirements, k8sClient client.Client, dc *discovery.DiscoveryClient) error {
	if require == nil {
		return nil
	}
	return checkAddonVersionMeetRequired(ctx, require, k8sClient, dc)
}

// GetAddonInstallPackageFromRegistry resolves a specific addon version's
// InstallPackage from the named registry. An empty version resolves the latest.
// It is the shared version-pinning helper used by both the render-only addon
// service and the Application validating webhook.
func GetAddonInstallPackageFromRegistry(ctx context.Context, cli client.Client, registryName, addonName, version string) (*InstallPackage, error) {
	ds := NewRegistryDataStore(cli)
	reg, err := ds.GetRegistry(ctx, registryName)
	if err != nil {
		return nil, fmt.Errorf("get registry %q: %w", registryName, err)
	}

	// This is the path a pinned version takes, which is the common one for a
	// type: addon component: the render resolves the pin directly rather than
	// going through the latest package. Without the cache here, pinning a
	// version -- the safest thing an author can do -- would be the one case
	// that pulls the addon from the registry on every reconcile.
	return installPackageCache.Load(
		installPackageCacheKey(reg, addonName, version),
		func(lastKnown string) (string, error) {
			// The version is known here, so the probe is a single conditional
			// request: no tag listing to discover what "latest" means.
			return reg.PackageRevision(ctx, addonName, version, lastKnown)
		},
		func(revision string) (*InstallPackage, error) {
			return readInstallPackage(ctx, reg.AtRevision(revision), registryName, addonName, version)
		},
	)
}

// installPackageCache holds addon install packages by the registry revision
// they were read at. It is separate from addonCache because this path yields an
// InstallPackage rather than a WholeAddonPackage, and because its key carries
// the pinned version.
var installPackageCache = component.NewRevisionCache[*InstallPackage](component.DefaultRevisionCacheSize)

// ResetInstallPackageCache empties the install package cache, for tests.
func ResetInstallPackageCache() { installPackageCache.Reset() }

// installPackageCacheKey is addonCacheKey plus the pinned version, since two
// versions of one addon are different packages out of the same source.
func installPackageCacheKey(r component.Registry, addonName, version string) string {
	return addonCacheKey(r, addonName) + "|v=" + version
}

// readInstallPackage is the uncached read, kept whole so the cache wraps one
// function rather than interleaving with it.
func readInstallPackage(ctx context.Context, reg component.Registry, registryName, addonName, version string) (*InstallPackage, error) {
	if IsVersionRegistry(reg) {
		vr, err := ToVersionedRegistry(reg)
		if err != nil {
			return nil, err
		}
		return vr.GetAddonInstallPackage(ctx, addonName, version)
	}

	meta, err := reg.ListPackageMeta(addonName)
	if err != nil {
		if goerrors.Is(err, component.ErrPackageNotExist) {
			return nil, fmt.Errorf("addon %q not found in registry %q", addonName, registryName)
		}
		return nil, err
	}
	uiData, err := GetUIData(&reg, &meta, UIMetaOptions)
	if err != nil {
		return nil, err
	}
	pkg, err := GetInstallPackage(&reg, &meta, uiData)
	if err != nil {
		return nil, err
	}
	if err := checkVersionPinSupported(registryName, addonName, version, pkg.Version); err != nil {
		return nil, err
	}
	return pkg, nil
}

// Status contain addon phase and related app status
type Status struct {
	AddonPhase string
	AppStatus  *commontypes.AppStatus
	// the status of multiple clusters
	Clusters         map[string]map[string]interface{} `json:"clusters,omitempty"`
	InstalledVersion string
	Parameters       map[string]interface{}
	// Where the addon is from. Can be empty if not installed.
	InstalledRegistry string
}

// checkVersionPinSupported rejects a version pin that a non-versioned registry
// cannot honor. git/OSS-backed registries serve a single revision of each addon,
// so there is nothing to resolve a pin against; returning the current content
// would silently install something other than what was asked for.
//
// An empty requested version means "whatever the registry serves" and always
// passes. A requested version equal to the available one also passes, so a pin
// that happens to match is not an error.
func checkVersionPinSupported(registryName, addonName, requested, available string) error {
	// A "v" prefix is cosmetic: chooseVersion (versioned_registry.go) already
	// ignores it when resolving versions, so comparing the raw strings here
	// would reject a pin that matches in every way that matters.
	if requested == "" || utils.IgnoreVPrefix(requested) == utils.IgnoreVPrefix(available) {
		return nil
	}
	if available == "" {
		return fmt.Errorf("registry %q does not support version pinning: addon %q reports no version, requested %q",
			registryName, addonName, requested)
	}
	return fmt.Errorf("registry %q does not support version pinning: addon %q is available at version %q, requested %q",
		registryName, addonName, available, requested)
}

// addonCache holds addon packages by the registry revision they were read at.
// Reading one addon from a git registry costs an API request per directory in
// the whole registry plus one per file of the addon, and the render path runs
// on every reconcile of every Application with a type: addon component. Without
// this, a few such Applications retrying on failure spend a 5000-request hour
// in minutes, and every render then fails for the rest of it.
var addonCache = component.NewRevisionCache[*WholeAddonPackage](component.DefaultRevisionCacheSize)

// ResetAddonCache empties the addon package cache. It exists for tests, which
// would otherwise carry a package from one case into the next.
func ResetAddonCache() { addonCache.Reset() }

// addonCacheKey identifies what was read: that addon, out of that source, with
// those credentials. The credentials are a digest so that rotating a token
// invalidates what the old one could see without the secret itself reaching a
// map key or a log line.
func addonCacheKey(r component.Registry, addonName string) string {
	var source, secret string
	switch {
	case r.Git != nil:
		source, secret = "git|"+r.Git.URL+"|"+r.Git.Path, r.Git.Token
	case r.Gitee != nil:
		source, secret = "gitee|"+r.Gitee.URL+"|"+r.Gitee.Path, r.Gitee.Token
	case r.Gitlab != nil:
		source, secret = "gitlab|"+r.Gitlab.URL+"|"+r.Gitlab.Repo+"|"+r.Gitlab.Path, r.Gitlab.Token
	case r.OSS != nil:
		source = "oss|" + r.OSS.Endpoint + "|" + r.OSS.Bucket + "|" + r.OSS.Path
	case r.Helm != nil:
		source, secret = "helm|"+r.Helm.URL, r.Helm.Username+"|"+r.Helm.Token
	default:
		source = "unknown"
	}
	sum := sha256.Sum256([]byte(secret))
	return strings.Join([]string{r.Name, source, addonName, hex.EncodeToString(sum[:8])}, "|")
}

// readAddonPackage returns one addon's whole package, reading the registry only
// when its revision has moved since the package was last read. An addon the
// registry does not carry reports component.ErrPackageNotExist.
func readAddonPackage(ctx context.Context, r component.Registry, addonName string) (*WholeAddonPackage, error) {
	return addonCache.Load(
		addonCacheKey(r, addonName),
		func(lastKnown string) (string, error) {
			return r.PackageRevision(ctx, addonName, "", lastKnown)
		},
		func(revision string) (*WholeAddonPackage, error) {
			// Every read below builds its own reader from this registry value,
			// so pinning the value pins all of them to one revision.
			at := r.AtRevision(revision)
			sourceMeta, err := at.ListPackageMeta(addonName)
			if err != nil {
				return nil, err
			}
			uiData, err := GetUIData(&at, &sourceMeta, UIMetaOptions)
			if err != nil {
				return nil, fmt.Errorf("read metadata: %w", err)
			}
			installPackage, err := GetInstallPackage(&at, &sourceMeta, uiData)
			if err != nil {
				return nil, fmt.Errorf("read package: %w", err)
			}
			return &WholeAddonPackage{
				InstallPackage:    *installPackage,
				APISchema:         uiData.APISchema,
				Detail:            uiData.Detail,
				AvailableVersions: uiData.AvailableVersions,
				RegistryName:      uiData.RegistryName,
			}, nil
		},
	)
}

// readVersionedAddonPackage returns one addon's whole package from a Helm or
// OCI registry, reading it only when the registry's revision has moved.
//
// This branch is the one an OCI registry takes -- IsVersionRegistry is true as
// soon as a registry has a Helm source, and an oci:// endpoint is modelled as
// one -- so without this an OCI addon would pull its chart on every reconcile
// while only git registries got the benefit. The revision is the manifest
// digest behind the resolved tag, which a conditional HEAD confirms without
// transferring the chart.
func readVersionedAddonPackage(ctx context.Context, r component.Registry, vr VersionedRegistry, addonName string) (*WholeAddonPackage, error) {
	return addonCache.Load(
		addonCacheKey(r, addonName),
		func(lastKnown string) (string, error) {
			return r.PackageRevision(ctx, addonName, "", lastKnown)
		},
		func(string) (*WholeAddonPackage, error) {
			// Not pinned: the versioned registry builds its own OCI client from
			// reg.Helm, so the digest cannot be threaded in without changing
			// that backend. The window is a re-push of the same tag between the
			// probe and the pull; see the note on AtRevision.
			return vr.GetDetailedAddon(ctx, addonName, "")
		},
	)
}
