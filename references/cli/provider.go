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

package cli

import (
	"context"
	"fmt"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam"
	"strings"

	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

const (

	// ProviderTerraformProviderNamespace is the namespace of addon terraform provider
	ProviderTerraformProviderNamespace = "default"
	// ProviderTerraformProviderNameArgument is the argument name of addon terraform provider
	ProviderTerraformProviderNameArgument = "providerName"
)

// NewProviderCommand create `addon` command
func NewProviderCommand(c common.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "addon",
		Short: "Manage addons for extension.",
		Long:  "Manage addons for extension.",
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeExtension,
		},
	}
	cmd.AddCommand(
		NewProviderListCommand(c),
	)
	return cmd
}

// NewProviderListCommand create addon list command
func NewProviderListCommand(c common.Args) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List addons",
		Long:    "List addons in KubeVela",
		RunE: func(cmd *cobra.Command, args []string) error {
			k8sClient, err := c.GetClient()
			if err != nil {
				return err
			}
			err = listProviders(context.Background(), k8sClient, "")
			if err != nil {
				return err
			}
			return nil
		},
	}
}

func listProviders(ctx context.Context, clt client.Client, registry string) error {
	var addons []*pkgaddon.UIData
	var err error
	registryDS := pkgaddon.NewRegistryDataStore(clt)
	registries, err := registryDS.ListRegistries(ctx)
	if err != nil {
		return err
	}
	onlineAddon := map[string]bool{}
	for _, r := range registries {
		if registry != "" && r.Name != registry {
			continue
		}

		meta, err := r.ListAddonMeta()
		if err != nil {
			continue
		}
		addList, err := r.ListUIData(meta, pkgaddon.CLIMetaOptions)
		if err != nil {
			continue
		}
		addons = mergeProviders(addons, addList)
	}

	table := uitable.New()
	table.AddRow("NAME", "REGISTRY", "DESCRIPTION", "STATUS")

	for _, addon := range addons {
		status, err := pkgaddon.GetAddonStatus(ctx, clt, addon.Name)
		if err != nil {
			return err
		}
		table.AddRow(addon.Name, addon.RegistryName, addon.Description, status.AddonPhase)
		onlineAddon[addon.Name] = true
	}
	appList := v1alpha2.ApplicationList{}
	if err := clt.List(ctx, &appList, client.MatchingLabels{oam.LabelAddonRegistry: pkgaddon.LocalAddonRegistryName}); err != nil {
		return err
	}
	for _, app := range appList.Items {
		addonName := app.GetLabels()[oam.LabelAddonName]
		if onlineAddon[addonName] {
			continue
		}
		table.AddRow(addonName, app.GetLabels()[oam.LabelAddonRegistry], "", statusEnabled)
	}
	fmt.Println(table.String())
	return nil
}

// TransProviderName will turn addon's name from xxx/yyy to xxx-yyy
func TransProviderName(name string) string {
	return strings.ReplaceAll(name, "/", "-")
}

func mergeProviders(a1, a2 []*pkgaddon.UIData) []*pkgaddon.UIData {
	for _, item := range a2 {
		if hasProvider(a1, item.Name) {
			continue
		}
		a1 = append(a1, item)
	}
	return a1
}

func hasProvider(addons []*pkgaddon.UIData, name string) bool {
	for _, addon := range addons {
		if addon.Name == name {
			return true
		}
	}
	return false
}
