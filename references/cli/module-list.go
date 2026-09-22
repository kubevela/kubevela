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

package cli

import (
	"context"
	"fmt"
	"sort"

	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pkgmodule "github.com/oam-dev/kubevela/pkg/module"
	"github.com/oam-dev/kubevela/pkg/registry/component"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// moduleListRegistryFlag names one registry to list, matching addon list's
// --registry flag.
const moduleListRegistryFlag = "registry"

// moduleEntry is one row of `vela module list`: a module name discovered in a
// registry, plus which registry it came from and that registry's source type.
type moduleEntry struct {
	name         string
	registryName string
	sourceType   string
}

// NewModuleListCommand returns the vela module list command.
func NewModuleListCommand(c common.Args) *cobra.Command {
	var registryFlag string
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List modules",
		Long:    "List modules present in the configured module registries.",
		Example: `  List modules from every configured registry:
	vela module list
  List modules in one registry, useful to reveal modules with duplicated names:
	vela module list --registry <registry-name>
`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			k8sClient, err := c.GetClient()
			if err != nil {
				return err
			}
			table, err := listModules(context.Background(), k8sClient, registryFlag, nil)
			if err != nil {
				return err
			}
			fmt.Println(table.String())
			return nil
		},
	}
	cmd.Flags().StringVarP(&registryFlag, moduleListRegistryFlag, "r", "", "specify the registry name to list")
	return cmd
}

// listMetaFn lists the modules a registry advertises without reading any
// module's own files. Production uses (*component.Registry).ListAddonMeta;
// tests inject a fake to avoid a real registry round trip.
type listMetaFn func(reg component.Registry) (map[string]component.SourceMeta, error)

// listModules enumerates the modules present in the configured registries,
// optionally scoped to one by name. listMeta defaults to
// (*component.Registry).ListAddonMeta; tests pass their own.
//
// Only a git-source registry can be listed: module.ResolveRegistry's own doc
// allows git or OCI, but OCI has no registry-wide catalog listing anywhere in
// this codebase (module.go:376's OCI fetch always pulls one already-named
// module's chart; there is no "list every chart this registry holds"
// primitive, matching the OCI distribution spec's own lack of a portable
// catalog endpoint). An OCI registry is therefore skipped when listing across
// every registry, and named plainly as unsupported when asked for directly.
func listModules(ctx context.Context, cli client.Client, registryFilter string, listMeta listMetaFn) (*uitable.Table, error) {
	if listMeta == nil {
		listMeta = func(reg component.Registry) (map[string]component.SourceMeta, error) {
			return reg.ListAddonMeta()
		}
	}

	store := pkgmodule.NewStore(cli)
	registries, err := store.ListRegistries(ctx)
	if err != nil {
		return nil, err
	}

	var entries []moduleEntry
	seen := map[string]bool{}
	for _, reg := range registries {
		if registryFilter != "" && reg.Name != registryFilter {
			continue
		}
		if reg.Git == nil {
			if registryFilter != "" {
				return nil, fmt.Errorf(
					"module registry %q is a %s source; listing modules is only supported for a git registry",
					reg.Name, pkgmodule.SourceTypeName(reg))
			}
			continue
		}

		meta, err := listMeta(reg)
		if err != nil {
			if registryFilter != "" {
				return nil, fmt.Errorf("registry %q: %w", reg.Name, err)
			}
			continue
		}

		names := make([]string, 0, len(meta))
		for name := range meta {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			key := reg.Name + "/" + name
			if seen[key] {
				continue
			}
			seen[key] = true
			entries = append(entries, moduleEntry{
				name:         name,
				registryName: reg.Name,
				sourceType:   pkgmodule.SourceTypeName(reg),
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].name != entries[j].name {
			return entries[i].name < entries[j].name
		}
		return entries[i].registryName < entries[j].registryName
	})

	table := uitable.New()
	table.AddRow("NAME", "REGISTRY", "SOURCE")
	for _, e := range entries {
		table.AddRow(e.name, e.registryName, e.sourceType)
	}
	return table, nil
}
