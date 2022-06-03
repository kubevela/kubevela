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
	"encoding/json"
	"strings"

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
)

var (
	regName string
	regURL  string
	token   string
	label   string
	filter  filterFunc
)

// NewTraitCommand creates `traits` command
func NewTraitCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	var isDiscover bool
	cmd := &cobra.Command{
		Use:     "trait",
		Aliases: []string{"traits"},
		Short:   "List/get traits.",
		Long:    "List trait types installed and discover more in registry.",
		Example: `vela trait`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// parse label filter
			if label != "" {
				words := strings.Split(label, "=")
				if len(words) < 2 {
					return errors.New("label is invalid")
				}
				filter = createLabelFilter(words[0], words[1])
			}

			var registry Registry
			var err error
			if isDiscover {
				if regURL != "" {
					ioStreams.Infof("Showing trait definition from url: %s\n", regURL)
					registry, err = NewRegistry(context.Background(), token, "temporary-registry", regURL)
					if err != nil {
						return errors.Wrap(err, "creating registry err, please check registry url")
					}
				} else {
					ioStreams.Infof("Showing trait definition from registry: %s\n", regName)
					registry, err = GetRegistry(regName)
					if err != nil {
						return errors.Wrap(err, "get registry err")
					}
				}
				return PrintTraitListFromRegistry(registry, ioStreams, filter)

			}
			return PrintInstalledTraitDef(c, ioStreams, filter)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeExtension,
		},
	}
	cmd.SetOut(ioStreams.Out)
	cmd.AddCommand(
		NewTraitGetCommand(c, ioStreams),
	)
	cmd.Flags().BoolVar(&isDiscover, "discover", false, "discover traits in registries")
	cmd.PersistentFlags().StringVar(&regURL, "url", "", "specify the registry URL")
	cmd.PersistentFlags().StringVar(&token, "token", "", "specify token when using --url to specify registry url")
	cmd.PersistentFlags().StringVar(&regName, "registry", DefaultRegistry, "specify the registry name")
	cmd.Flags().StringVar(&label, types.LabelArg, "", "a label to filter components, the format is `--label type=terraform`")
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
			var registry Registry
			var err error

			if regURL != "" {
				ioStreams.Infof("Getting trait definition from url: %s\n", regURL)
				registry, err = NewRegistry(context.Background(), token, "temporary-registry", regURL)
				if err != nil {
					return errors.Wrap(err, "creating registry err, please check registry url")
				}
			} else {
				ioStreams.Infof("Getting trait definition from registry: %s\n", regName)
				registry, err = GetRegistry(regName)
				if err != nil {
					return errors.Wrap(err, "get registry err")
				}
			}
			return errors.Wrap(InstallTraitByNameFromRegistry(c, ioStreams, name, registry), "install trait definition err")
		},
	}
	return cmd
}

// PrintTraitListFromRegistry print a table which shows all traits from registry
func PrintTraitListFromRegistry(registry Registry, ioStreams cmdutil.IOStreams, filter filterFunc) error {
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

	caps, err := registry.ListCaps()
	if err != nil {
		return err
	}

	table := newUITable()

	var installedList v1beta1.TraitDefinitionList
	err = k8sClient.List(context.Background(), &installedList, client.InNamespace(types.DefaultKubeVelaNS))
	if err != nil {
		return err
	}

	table.AddRow("NAME", "REGISTRY", "DEFINITION", "APPLIES-TO", "STATUS")
	for _, c := range caps {
		if filter != nil && !filter(c) {
			continue
		}
		if c.Type != types.TypeTrait {
			continue
		}
		c.Status = uninstalled
		for _, ins := range installedList.Items {
			if ins.Name == c.Name {
				c.Status = installed
			}
		}
		table.AddRow(c.Name, "default", c.CrdName, c.AppliesTo, c.Status)
	}
	ioStreams.Info(table.String())

	return nil
}

// InstallTraitByNameFromRegistry will install given traitName trait to cluster
func InstallTraitByNameFromRegistry(args common2.Args, ioStream cmdutil.IOStreams, traitName string, registry Registry) error {
	capObj, data, err := registry.GetCap(traitName)
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

// PrintInstalledTraitDef will print all TraitDefinition in cluster
func PrintInstalledTraitDef(c common2.Args, io cmdutil.IOStreams, filter filterFunc) error {
	var list v1beta1.TraitDefinitionList
	clt, err := c.GetClient()
	if err != nil {
		return err
	}
	err = clt.List(context.Background(), &list)
	if err != nil {
		return errors.Wrap(err, "get trait definition list error")
	}
	dm, err := (&common2.Args{}).GetDiscoveryMapper()
	if err != nil {
		return errors.Wrap(err, "get discovery mapper error")
	}

	table := newUITable()
	table.AddRow("NAME", "APPLIES-TO")
	table.AddRow("NAME", "APPLIES-TO", "DESCRIPTION")

	for _, td := range list.Items {
		data, err := json.Marshal(td)
		if err != nil {
			io.Infof("error encoding definition: %s\n", td.Name)
			continue
		}
		capa, err := ParseCapability(dm, data)
		if err != nil {
			io.Errorf("error parsing capability: %s (message: %s)\n", td.Name, err.Error())
			continue
		}
		if filter != nil && !filter(capa) {
			continue
		}
		table.AddRow(capa.Name, capa.AppliesTo, capa.Description)
	}
	io.Info(table.String())
	return nil
}

const installed = "installed"
const uninstalled = "uninstalled"
