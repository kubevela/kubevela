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
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	core "github.com/oam-dev/kubevela/apis/core.oam.dev"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
	"github.com/oam-dev/kubevela/references/plugins"
)

// NewComponentsCommand creates `components` command
func NewComponentsCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
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
			isDiscover, _ := cmd.Flags().GetBool("discover")
			env, err := GetFlagEnvOrCurrent(cmd, c)
			if err != nil {
				return err
			}

			label, err := cmd.Flags().GetString(types.LabelArg)
			if err != nil {
				return err
			}
			if label != "" && len(strings.Split(label, "=")) != 2 {
				return fmt.Errorf("label %s is not in the right format", label)
			}

			if !isDiscover {
				return printComponentList(env.Namespace, c, ioStreams, label)
			}
			option := types.TypeComponentDefinition
			err = printCenterCapabilities(env.Namespace, "", c, ioStreams, &option, label)
			if err != nil {
				return err
			}

			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCap,
		},
	}
	cmd.Flags().Bool("discover", false, "discover traits in capability centers")
	cmd.Flags().String(types.LabelArg, "", "a label to filter components, the format is `--label type=terraform`")
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func printComponentList(userNamespace string, c common2.Args, ioStreams cmdutil.IOStreams, label string) error {
	def, err := common.ListRawComponentDefinitions(userNamespace, c)
	if err != nil {
		return err
	}

	dm, err := c.GetDiscoveryMapper()
	if err != nil {
		return fmt.Errorf("get discoveryMapper error %w", err)
	}

	table := newUITable()
	table.AddRow("NAME", "NAMESPACE", "WORKLOAD", "DESCRIPTION")

	for _, r := range def {
		if label != "" && !common.CheckLabelExistence(r.Labels, label) {
			continue
		}
		var workload string
		if r.Spec.Workload.Type != "" {
			workload = r.Spec.Workload.Type
		} else {
			definition, err := oamutil.ConvertWorkloadGVK2Definition(dm, r.Spec.Workload.Definition)
			if err != nil {
				return fmt.Errorf("get workload definitionReference error %w", err)
			}
			workload = definition.Name
		}
		table.AddRow(r.Name, r.Namespace, workload, plugins.GetDescription(r.Annotations))
	}
	ioStreams.Info(table.String())
	return nil
}

// PrintComponentListFromRegistry print a table which shows all components from registry
func PrintComponentListFromRegistry(isDiscover bool, url string, ioStreams cmdutil.IOStreams) error {
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

	_, _ = ioStreams.Out.Write([]byte(fmt.Sprintf("Showing components from registry: %s\n", url)))
	caps, err := getCapsFromRegistry(url)
	if err != nil {
		return err
	}

	var installedList v1beta1.ComponentDefinitionList
	err = k8sClient.List(context.Background(), &installedList, client.InNamespace(types.DefaultKubeVelaNS))
	if err != nil {
		return err
	}
	table := newUITable()
	if isDiscover {
		table.AddRow("NAME", "REGISTRY", "DEFINITION")
	} else {
		table.AddRow("NAME", "DEFINITION")
	}
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

		if c.Status == uninstalled && isDiscover {
			table.AddRow(c.Name, "default", c.CrdName)
		}
		if c.Status == installed && !isDiscover {
			table.AddRow(c.Name, c.CrdName)
		}
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
