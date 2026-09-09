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

package module

import (
	"context"
	"fmt"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/registry/component"
)

const (
	// ModuleRegistryConfigMap is the ConfigMap in vela-system holding module
	// registries. It is separate from the addon registry ConfigMap so modules
	// are configured, listed, and removed independently of addons.
	ModuleRegistryConfigMap = "vela-module-registry"

	// ModuleRegistrySecretPrefix prefixes the secret holding a module registry's
	// token. Each registry entry gets its own secret, named
	// ModuleRegistrySecretPrefix + the registry name.
	ModuleRegistrySecretPrefix = "module-registry-"

	// DefaultRegistryName is the registry seeded by the chart at install time.
	// It is the default when a caller does not name a registry and more than one
	// is configured.
	DefaultRegistryName = "catalog"

	// DefaultGitPath is the subpath within a git registry that holds modules.
	// The fetch reads DefaultGitPath/<module name>/ from the repository.
	DefaultGitPath = "module"
)

// NewStore returns the module registry store: the shared addon registry
// implementation pointed at the module ConfigMap and secret prefix.
func NewStore(cli client.Client) component.RegistryDataStore {
	return component.NewRegistryDataStoreFor(cli, ModuleRegistryConfigMap, ModuleRegistrySecretPrefix)
}

// ResolveRegistry returns the module registry to use for an operation. A non-empty
// name selects that registry; an empty name selects a default. The returned
// Registry is guaranteed to be a git or OCI source, with its token loaded from
// its secret.
//
// Modules support no other kind of source. The module ConfigMap shares its
// format with the addon one, though, so an entry configured as helm, OSS,
// gitee, or gitlab can be present -- hand-edited, or written by
// `vela addon registry` if it is pointed at this ConfigMap. Such an entry is
// rejected here, naming the entry and its actual type, rather than handed to a
// consumer that assumes reg.Git or an oci:// reg.Helm is set.
//
// With an empty name the rules apply in order: the sole configured registry wins;
// otherwise a registry named DefaultRegistryName wins; otherwise the choice is
// ambiguous and an error names the alternatives. Both rules are needed. The sole
// registry rule alone would start failing as soon as a user adds a second registry
// alongside the seeded one, and the DefaultRegistryName rule alone would fail for
// an operator who removed the seeded entry.
func ResolveRegistry(ctx context.Context, store component.RegistryDataStore, name string) (component.Registry, error) {
	reg, err := resolveRegistryByName(ctx, store, name)
	if err != nil {
		return component.Registry{}, err
	}
	if reg.Git == nil && reg.OCIChartSource() == nil {
		return component.Registry{}, fmt.Errorf(
			"module registry %q is a %s source; modules support only git and OCI registries",
			reg.Name, SourceTypeName(reg))
	}
	return reg, nil
}

// resolveRegistryByName implements ResolveRegistry's name-selection rules,
// without enforcing that the result is a git or OCI source. ResolveRegistry
// applies that check uniformly to every return path below.
func resolveRegistryByName(ctx context.Context, store component.RegistryDataStore, name string) (component.Registry, error) {
	if name != "" {
		reg, err := store.GetRegistry(ctx, name)
		if err == nil {
			return reg, nil
		}
		if !apierrors.IsNotFound(err) {
			return component.Registry{}, err
		}
		return component.Registry{}, notFoundError(ctx, store, name)
	}

	registries, err := store.ListRegistries(ctx)
	if err != nil {
		return component.Registry{}, err
	}
	switch len(registries) {
	case 0:
		return component.Registry{}, fmt.Errorf(
			"no module registry is configured; add one with \"vela module registry add\"")
	case 1:
		return registries[0], nil
	}
	for _, reg := range registries {
		if reg.Name == DefaultRegistryName {
			return reg, nil
		}
	}
	return component.Registry{}, fmt.Errorf(
		"several module registries are configured and none is named %q; pass --registry with one of: %s",
		DefaultRegistryName, strings.Join(sortedRegistryNames(registries), ", "))
}

// SourceTypeName names the kind of source configured on a registry entry, or
// "unknown" if none is set. It covers every source the shared addon ConfigMap
// format can hold, not just git and OCI, so a caller such as list can name an
// entry ResolveRegistry would reject instead of leaving its type blank.
func SourceTypeName(reg component.Registry) string {
	switch {
	case reg.Git != nil:
		return "git"
	case reg.OCIChartSource() != nil:
		// An OCI registry is a Helm source with an oci:// URL, so this has to be
		// tested before the plain helm case or every OCI entry reads as "helm".
		return "oci"
	case reg.Helm != nil:
		return "helm"
	case reg.OSS != nil:
		return "oss"
	case reg.Gitee != nil:
		return "gitee"
	case reg.Gitlab != nil:
		return "gitlab"
	default:
		return "unknown"
	}
}

// NotFoundError builds the same "not found" error ResolveRegistry returns for
// an unknown name. It is exported for commands such as delete, which look up
// a registry directly with store.GetRegistry rather than through
// ResolveRegistry -- so an entry ResolveRegistry would reject as unsupported
// can still be found and removed -- but should report an unknown name the
// same way.
func NotFoundError(ctx context.Context, store component.RegistryDataStore, name string) error {
	return notFoundError(ctx, store, name)
}

// notFoundError reports an unknown registry name. It lists the configured
// names when there are any, says none are configured when the store is
// genuinely empty, and reports the read failure plainly when the store could
// not be listed at all -- rather than telling the operator to add a registry
// that could not be read, e.g. under an RBAC denial or a corrupted ConfigMap.
func notFoundError(ctx context.Context, store component.RegistryDataStore, name string) error {
	names, err := registryNames(ctx, store)
	if err != nil {
		return fmt.Errorf("module registry %q not found: failed to list the configured registries: %w", name, err)
	}
	if len(names) == 0 {
		return fmt.Errorf(
			"module registry %q not found; no module registry is configured, add one with \"vela module registry add\": %w", name, ErrRegistryNotFound)
	}
	return fmt.Errorf("module registry %q not found; configured registries: %s (%w)", name, strings.Join(names, ", "), ErrRegistryNotFound)
}

// sortedRegistryNames returns the registry names in sorted order.
func sortedRegistryNames(registries []component.Registry) []string {
	names := make([]string, 0, len(registries))
	for _, reg := range registries {
		names = append(names, reg.Name)
	}
	sort.Strings(names)
	return names
}

// registryNames returns the sorted names of the configured module registries,
// or an error if the store could not be read. Distinguishing a read failure
// from a genuinely empty store is what lets notFoundError avoid masquerading
// one as the other.
func registryNames(ctx context.Context, store component.RegistryDataStore) ([]string, error) {
	registries, err := store.ListRegistries(ctx)
	if err != nil {
		return nil, err
	}
	return sortedRegistryNames(registries), nil
}
