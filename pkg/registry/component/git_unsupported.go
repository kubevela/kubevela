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
	"sort"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ErrGitSourceUnsupported means a registry backed by Git was named by an
// Application component. The type: addon and type: module components resolve
// packages from an OCI registry or a Helm repository only.
//
// This restricts the component path, not the registry format. A Git registry
// stays a valid record and `vela addon enable` keeps installing from it; only
// the declarative components refuse to resolve one.
var ErrGitSourceUnsupported = NewError("git registries are not supported for addon and module components")

// The remedy each component type offers when it refuses a Git registry. They
// differ because what the two can read differs.
const (
	// AddonGitRemedy is what a type: addon component can resolve instead.
	AddonGitRemedy = "use an OCI registry or a Helm repository"
	// ModuleGitRemedy is what a type: module component can resolve instead.
	// Modules have never read a Helm chart repository.
	ModuleGitRemedy = "use an OCI registry"
)

// GitFamilySource names the Git flavour a registry is backed by -- "git",
// "gitee" or "gitlab" -- or "" for any other kind of entry.
//
// Every place that refuses Git for a component goes through this rather than
// testing the three fields itself, so the admission check, the module resolver
// and the addon renderer cannot drift apart on what counts as Git.
func GitFamilySource(reg Registry) string {
	switch {
	case reg.Git != nil:
		return "git"
	case reg.Gitee != nil:
		return "gitee"
	case reg.Gitlab != nil:
		return "gitlab"
	default:
		return ""
	}
}

// GitSourceUnsupportedDetail is the one sentence that explains the refusal,
// for a caller that needs prose rather than an error -- an admission webhook
// builds a field.Invalid detail, which takes a string and cannot wrap.
//
// kind comes from GitFamilySource; callers have already established it is
// non-empty. remedy is the caller's, because the answer differs: an addon
// component can resolve from an OCI registry or a Helm repository, a module
// component from an OCI registry only. Baking one remedy in here would tell
// module users to reach for a source modules cannot read.
//
// Every refusal goes through this or through GitSourceUnsupportedError below,
// so the wording cannot drift between what admission says and what the
// resolve reports.
func GitSourceUnsupportedDetail(kind, remedy string) string {
	return fmt.Sprintf("is a %s source, and git registries are not supported for addon and module components; %s",
		kind, remedy)
}

// GitSourceUnsupportedError reports that registry name is Git backed, naming
// the flavour so the reader knows which field to look at, and wrapping
// ErrGitSourceUnsupported so callers can classify it with errors.Is.
func GitSourceUnsupportedError(name, kind, remedy string) error {
	return fmt.Errorf("registry %q %s (%w)",
		name, GitSourceUnsupportedDetail(kind, remedy), ErrGitSourceUnsupported)
}

// ListRegistryGitKinds reports which configured addon registries are Git
// backed, as the GitFamilySource value keyed by registry name. A registry on
// any other source is absent, so an empty map means nothing configured is Git.
//
// It reads the registry ConfigMap and no token Secret. That is what makes it
// usable from admission: a registry's source kind is in the ConfigMap, and
// resolving the package it points at is the part admission must not do.
func ListRegistryGitKinds(ctx context.Context, cli client.Client) (map[string]string, error) {
	return ListRegistryGitKindsIn(ctx, cli, registryConfigMapName)
}

// ListRegistryGitKindsIn is ListRegistryGitKinds over a named ConfigMap, so
// the module registry can be inspected the same way. Module registries live
// in their own ConfigMap but share the record format, and therefore the
// question.
func ListRegistryGitKindsIn(ctx context.Context, cli client.Client, cmName string) (map[string]string, error) {
	registries, err := listRegistryRecordsFrom(ctx, cli, cmName)
	if err != nil {
		return nil, err
	}
	return gitKindsOf(registries), nil
}

// ListRegistrySources reports the configured addon registries: every name in
// sorted order, and the Git kinds as ListRegistryGitKinds returns them.
//
// Both come from one read, because the two answers have to agree. Read
// separately, a registry created between the calls appears in names but not in
// gitKinds, and a caller would treat it as non-Git.
//
// Prefer ListRegistryGitKinds when the names are not needed: this one also
// builds and sorts a slice.
func ListRegistrySources(ctx context.Context, cli client.Client) (names []string, gitKinds map[string]string, err error) {
	registries, err := listRegistryRecordsFrom(ctx, cli, registryConfigMapName)
	if err != nil {
		return nil, nil, err
	}
	names = make([]string, 0, len(registries))
	for name := range registries {
		names = append(names, name)
	}
	// Sorted for the same reason ListRegistries sorts: callers treat the order
	// as a priority, and a map iterates randomly.
	sort.Strings(names)
	return names, gitKindsOf(registries), nil
}

// gitKindsOf keeps only the Git-backed entries, keyed by name. The map is
// allocated lazily: the overwhelmingly common cluster configures no Git
// registry at all, and this runs on the admission and render paths.
func gitKindsOf(registries map[string]Registry) map[string]string {
	var gitKinds map[string]string
	for name, reg := range registries {
		kind := GitFamilySource(reg)
		if kind == "" {
			continue
		}
		if gitKinds == nil {
			gitKinds = make(map[string]string, 1)
		}
		gitKinds[name] = kind
	}
	return gitKinds
}
