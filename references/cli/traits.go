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
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	core "github.com/oam-dev/kubevela/apis/core.oam.dev"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
	"github.com/oam-dev/kubevela/references/plugins"
)

// NewTraitsCommand creates `traits` command
func NewTraitsCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "traits",
		Aliases:               []string{"trait"},
		DisableFlagsInUseLine: true,
		Short:                 "List traits",
		Long:                  "List traits",
		Example:               `vela traits`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			isDiscover, _ := cmd.Flags().GetBool("discover")
			url, _ := cmd.PersistentFlags().GetString("url")
			err := PrintTraitListFromRegistry(isDiscover, url, ioStreams)
			return err
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCap,
		},
	}
	cmd.SetOut(ioStreams.Out)
	cmd.AddCommand(
		NewTraitGetCommand(c, ioStreams),
	)
	cmd.Flags().Bool("discover", false, "discover traits in registries")
	cmd.PersistentFlags().String("url", DefaultRegistry, "specify the registry URL")

	cmd.Flags().String(types.LabelArg, "", "a label to filter components, the format is `--label type=terraform`")
	cmd.SetOut(ioStreams.Out)
	return cmd
}

// NewTraitGetCommand creates `trait get` command
func NewTraitGetCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <trait>",
		Short:   "get trait from registry",
		Long:    "get trait from registry",
		Example: "vela trait get <trait>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				ioStreams.Error("you must specify the trait name")
				return nil
			}
			name := args[0]
			url, _ := cmd.Flags().GetString("url")

			return InstallTraitByName(c, ioStreams, name, url)
		},
	}
	return cmd
}

// PrintTraitListFromRegistry print a table which shows all traits from registry
func PrintTraitListFromRegistry(isDiscover bool, regName string, ioStreams cmdutil.IOStreams) error {
	var scheme = runtime.NewScheme()
	err := core.AddToScheme(scheme)
	if err != nil {
		return err
	}
	err = clientgoscheme.AddToScheme(scheme)
	if err != nil {
		return err
	}
	k8sClient, err := client.New(config.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	_, _ = ioStreams.Out.Write([]byte(fmt.Sprintf("Showing traits from registry: %s\n", regName)))
	caps, err := getCapsFromRegistry(regName)
	if err != nil {
		return err
	}

	table := newUITable()

	var installedList v1beta1.TraitDefinitionList
	err = k8sClient.List(context.Background(), &installedList, client.InNamespace(types.DefaultKubeVelaNS))
	if err != nil {
		return err
	}
	if isDiscover {
		table.AddRow("NAME", "REGISTRY", "DEFINITION", "APPLIES-TO")
	} else {
		table.AddRow("NAME", "DEFINITION", "APPLIES-TO")
	}
	for _, c := range caps {
		if c.Type != types.TypeTrait {
			continue
		}
		c.Status = uninstalled
		for _, ins := range installedList.Items {
			if ins.Name == c.Name {
				c.Status = installed
			}
		}
		if c.Status == uninstalled && isDiscover {
			table.AddRow(c.Name, "default", c.CrdName, c.AppliesTo)
		}
		if c.Status == installed && !isDiscover {
			table.AddRow(c.Name, c.CrdName, c.AppliesTo)
		}
	}
	ioStreams.Info(table.String())

	return nil
}

// getCapsFromRegistry will retrieve caps from registry
func getCapsFromRegistry(regName string) ([]types.Capability, error) {
	reg, err := plugins.GetRegistry(regName)
	if err != nil {
		return nil, errors.Wrap(err, "get registry fail")
	}
	caps, err := reg.ListCaps()
	if err != nil {
		return []types.Capability{}, err
	}
	return caps, nil
}

// InstallTraitByName will install given traitName trait to cluster
func InstallTraitByName(args common2.Args, ioStream cmdutil.IOStreams, traitName, regURL string) error {

	g, err := plugins.NewRegistry(context.Background(), "", "url-registry", regURL)
	if err != nil {
		return err
	}
	capObj, data, err := g.GetCap(traitName)
	if err != nil {
		return err
	}

	k8sClient, err := args.GetClient()
	if err != nil {
		return err
	}
	mapper, err := args.GetDiscoveryMapper()
	if err != nil {
		return err
	}

	err = common.InstallTraitDefinition(k8sClient, mapper, data, ioStream, &capObj)
	if err != nil {
		return err
	}
	ioStream.Info("Successfully install trait:", traitName)
	return nil
}

// DefaultRegistry is default capability center of kubectl-vela
var DefaultRegistry = "oss://registry.kubevela.net"

const installed = "installed"
const uninstalled = "uninstalled"
