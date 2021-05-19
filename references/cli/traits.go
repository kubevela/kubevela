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
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
	"github.com/oam-dev/kubevela/references/plugins"
)

// NewTraitsCommand creates `traits` command
func NewTraitsCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "traits",
		DisableFlagsInUseLine: true,
		Short:                 "List traits",
		Long:                  "List traits",
		Example:               `vela traits`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := GetEnv(cmd)
			if err != nil {
				return err
			}
			return printTraitList(env.Namespace, c, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCap,
		},
	}

	cmd.SetOut(ioStreams.Out)
	return cmd
}

func printTraitList(userNamespace string, c common2.Args, ioStreams cmdutil.IOStreams) error {
	table := newUITable()
	table.Wrap = true

	traitDefinitionList, err := common.ListRawTraitDefinitions(userNamespace, c)
	if err != nil {
		return err
	}
	table.AddRow("NAME", "NAMESPACE", "APPLIES-TO", "CONFLICTS-WITH", "POD-DISRUPTIVE", "DESCRIPTION")
	for _, t := range traitDefinitionList {
		table.AddRow(t.Name, t.Namespace, strings.Join(t.Spec.AppliesToWorkloads, ","), strings.Join(t.Spec.ConflictsWith, ","), t.Spec.PodDisruptive, plugins.GetDescription(t.Annotations))
	}
	ioStreams.Info(table.String())
	return nil
}

// PrintTraitList print a table which shows all traits from default registry
func PrintTraitList(isDiscover bool, ioStreams cmdutil.IOStreams) error {
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

	_, _ = ioStreams.Out.Write([]byte(fmt.Sprintf("Showing traits from default registry:%s\n", defaultCenter)))
	caps, err := getCapsFromDefaultCenter()
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

// getCapsFromDefaultCenter will retrieve caps from default registry
func getCapsFromDefaultCenter() ([]types.Capability, error) {
	g, err := getDefaultGithubCenter()
	if err != nil {
		return []types.Capability{}, err
	}
	caps, err := g.GetCaps()
	if err != nil {
		return []types.Capability{}, err
	}
	return caps, nil
}

// getDefaultGithubCenter will return GH center object for defaultCenter
func getDefaultGithubCenter() (*plugins.GithubCenter, error) {
	_, ghContent, _ := plugins.Parse(defaultCenter)
	g, err := plugins.NewGithubCenter(context.Background(), "", "default-cap-center", ghContent)
	return g, err
}

// InstallTraitByName will install given traitName trait to cluter
func InstallTraitByName(args common2.Args, ioStream cmdutil.IOStreams, traitName string) error {

	g, err := getDefaultGithubCenter()
	if err != nil {
		return err
	}
	capObj, data, err := g.GetCapAndFileContent(traitName)
	if err != nil {
		return err
	}

	client, err := args.GetClient()
	if err != nil {
		return err
	}
	mapper, err := args.GetDiscoveryMapper()
	if err != nil {
		return err
	}

	err = common.InstallTraitDefinition(client, mapper, data, ioStream, &capObj)
	if err != nil {
		return err
	}
	fmt.Printf("Successfully install trait: %s\n", traitName)
	return nil
}

// defaultCenter is default capability center of kubectl-vela
var defaultCenter = "https://github.com/oam-dev/catalog/tree/master/registry"

const installed = "installed"
const uninstalled = "uninstalled"
