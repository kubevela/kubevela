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
	"github.com/oam-dev/kubevela/pkg/oam/util"
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

// NewComponentsCommand creates `components` command
func NewComponentsCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	var isDiscover bool
	var registry string
	var url string
	cmd := &cobra.Command{
		Use:                   "components",
		Aliases:               []string{"comp", "component"},
		DisableFlagsInUseLine: true,
		Short:                 "List components",
		Long:                  "List components",
		Example:               `vela components`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if isDiscover {
				return PrintComponentListFromRegistry(registry, ioStreams)
			} else {
				return PrintInstalledCompDef(ioStreams)
			}
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
	cmd.Flags().StringVar(&url, "url", "", "specify the registry URL")
	cmd.Flags().StringVar(&registry, "registry", plugins.DefaultRegistry, "specify the registry name")
	cmd.Flags().String(types.LabelArg, "", "a label to filter components, the format is `--label type=terraform`")
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
			url, _ := cmd.Flags().GetString("url")
			return InstallCompByName(c, ioStreams, name, url)
		},
	}
	return cmd
}

// PrintComponentListFromRegistry print a table which shows all components from registry
func PrintComponentListFromRegistry(regName string, ioStreams cmdutil.IOStreams) error {
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

	_, _ = ioStreams.Out.Write([]byte(fmt.Sprintf("Showing components from registry: %s\n", regName)))
	caps, err := getCapsFromRegistry(regName)
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

// InstallCompByName will install given componentName comp to cluster from registry
func InstallCompByName(args common2.Args, ioStream cmdutil.IOStreams, compName, regURL string) error {

	g, err := plugins.NewRegistry(context.Background(), "", "url-registry", regURL)
	if err != nil {
		return err
	}
	capObj, data, err := g.GetCap(compName)
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

func PrintInstalledCompDef(io cmdutil.IOStreams) error {
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
		ref, err := util.ConvertWorkloadGVK2Definition(dm, cd.Spec.Workload.Definition)
		if err != nil {
			table.AddRow(cd.Name, "")
			continue
		}
		table.AddRow(cd.Name, ref.Name)
	}
	io.Infof(table.String())
	return nil
}
