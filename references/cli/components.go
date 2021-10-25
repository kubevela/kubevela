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

// NewComponentsCommand creates `components` command
func NewComponentsCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	var isDiscover bool
	cmd := &cobra.Command{
		Use:     "components",
		Aliases: []string{"comp", "component"},
		Short:   "List/get components",
		Long:    "List components & get components in registry",
		Example: `vela comp`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// parse label filter
			if label != "" {
				words := strings.Split(label, "=")
				if len(words) != 2 {
					return errors.New("label is invalid")
				}
				filter = createLabelFilter(words[0], words[1])
			}

			var registry Registry
			var err error
			if isDiscover {
				if regURL != "" {
					ioStreams.Infof("Listing component definition from url: %s\n", regURL)
					registry, err = NewRegistry(context.Background(), token, "temporary-registry", regURL)
					if err != nil {
						return errors.Wrap(err, "creating registry err, please check registry url")
					}
				} else {
					ioStreams.Infof("Listing component definition from registry: %s\n", regName)
					registry, err = GetRegistry(regName)
					if err != nil {
						return errors.Wrap(err, "get registry err")
					}
				}
				return PrintComponentListFromRegistry(registry, ioStreams, filter)
			}
			return PrintInstalledCompDef(ioStreams, filter)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCap,
		},
	}
	cmd.SetOut(ioStreams.Out)
	cmd.AddCommand(
		NewCompGetCommand(c, ioStreams),
	)
	cmd.Flags().BoolVar(&isDiscover, "discover", false, "discover traits in registries")
	cmd.PersistentFlags().StringVar(&regURL, "url", "", "specify the registry URL")
	cmd.PersistentFlags().StringVar(&regName, "registry", DefaultRegistry, "specify the registry name")
	cmd.PersistentFlags().StringVar(&token, "token", "", "specify token when using --url to specify registry url")
	cmd.Flags().StringVar(&label, types.LabelArg, "", "a label to filter components, the format is `--label type=terraform`")
	cmd.SetOut(ioStreams.Out)
	return cmd
}

// NewCompGetCommand creates `comp get` command
func NewCompGetCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <component>",
		Short:   "get component from registry",
		Long:    "get component from registry",
		Example: "vela comp get <component>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				ioStreams.Error("you must specify a component name")
				return nil
			}
			name := args[0]
			var registry Registry
			var err error

			if regURL != "" {
				ioStreams.Infof("Getting component definition from url: %s\n", regURL)
				registry, err = NewRegistry(context.Background(), token, "temporary-registry", regURL)
				if err != nil {
					return errors.Wrap(err, "creating registry err, please check registry url")
				}
			} else {
				ioStreams.Infof("Getting component definition from registry: %s\n", regName)
				registry, err = GetRegistry(regName)
				if err != nil {
					return errors.Wrap(err, "get registry err")
				}
			}
			return errors.Wrap(InstallCompByNameFromRegistry(c, ioStreams, name, registry), "install component definition err")

		},
	}
	return cmd
}

// filterFunc to filter whether to print the capability
type filterFunc func(capability types.Capability) bool

func createLabelFilter(key, value string) filterFunc {
	return func(capability types.Capability) bool {
		return capability.Labels[key] == value
	}
}

// PrintComponentListFromRegistry print a table which shows all components from registry
func PrintComponentListFromRegistry(registry Registry, ioStreams cmdutil.IOStreams, filter filterFunc) error {
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

	var installedList v1beta1.ComponentDefinitionList
	err = k8sClient.List(context.Background(), &installedList, client.InNamespace(types.DefaultKubeVelaNS))
	if err != nil {
		return err
	}
	table := newUITable()
	table.AddRow("NAME", "REGISTRY", "DEFINITION", "STATUS")
	for _, c := range caps {

		if filter != nil && !filter(c) {
			continue
		}
		c.Status = uninstalled
		if c.Type != types.TypeComponentDefinition {
			continue
		}
		for _, ins := range installedList.Items {
			if ins.Name == c.Name {
				c.Status = installed
			}
		}

		table.AddRow(c.Name, "default", c.CrdName, c.Status)
	}
	ioStreams.Info(table.String())

	return nil
}

// InstallCompByNameFromRegistry will install given componentName comp to cluster from registry
func InstallCompByNameFromRegistry(args common2.Args, ioStream cmdutil.IOStreams, compName string, registry Registry) error {
	capObj, data, err := registry.GetCap(compName)
	if err != nil {
		return err
	}

	k8sClient, err := args.GetClient()
	if err != nil {
		return err
	}

	err = common.InstallComponentDefinition(k8sClient, data, ioStream, &capObj)
	if err != nil {
		return err
	}

	ioStream.Info("Successfully install component:", compName)

	return nil
}

// PrintInstalledCompDef will print all ComponentDefinition in cluster
func PrintInstalledCompDef(io cmdutil.IOStreams, filter filterFunc) error {
	var list v1beta1.ComponentDefinitionList
	err := clt.List(context.Background(), &list)
	if err != nil {
		return errors.Wrap(err, "get component definition list error")
	}
	dm, err := (&common2.Args{}).GetDiscoveryMapper()
	if err != nil {
		return errors.Wrap(err, "get discovery mapper error")
	}

	table := newUITable()
	table.AddRow("NAME", "DEFINITION")

	for _, cd := range list.Items {
		data, err := json.Marshal(cd)
		if err != nil {
			io.Infof("error encoding definition: %s\n", cd.Name)
			continue
		}
		capa, err := ParseCapability(dm, data)
		if err != nil {
			io.Errorf("error parsing capability: %s\n", cd.Name)
			continue
		}
		if filter != nil && !filter(capa) {
			continue
		}
		table.AddRow(capa.Name, capa.CrdName)
	}
	io.Infof(table.String())
	return nil
}
