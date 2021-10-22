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

	"github.com/gosuri/uitable"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	addonutil "github.com/oam-dev/kubevela/pkg/utils/addon"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

var legacyAddonNamespace map[string]string
var clt client.Client

func init() {
	clientArgs, _ := common.InitBaseRestConfig()
	clt, _ = clientArgs.GetClient()
	legacyAddonNamespace = map[string]string{
		"fluxcd":                     types.DefaultKubeVelaNS,
		"ns-flux-system":             types.DefaultKubeVelaNS,
		"kruise":                     types.DefaultKubeVelaNS,
		"prometheus":                 types.DefaultKubeVelaNS,
		"observability":              "observability",
		"observability-asset":        types.DefaultKubeVelaNS,
		"istio":                      "istio-system",
		"ns-istio-system":            types.DefaultKubeVelaNS,
		"keda":                       types.DefaultKubeVelaNS,
		"ocm-cluster-manager":        types.DefaultKubeVelaNS,
		"terraform":                  types.DefaultKubeVelaNS,
		"terraform-provider/alibaba": "default",
		"terraform-provider/azure":   "default",
	}
}

// NewAddonCommand create `addon` command
func NewAddonCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "addon",
		Short: "List and get addon in KubeVela",
		Long:  "List and get addon in KubeVela",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	cmd.AddCommand(
		NewAddonListCommand(),
		NewAddonEnableCommand(ioStreams),
		NewAddonDisableCommand(ioStreams),
	)
	return cmd
}

// NewAddonListCommand create addon list command
func NewAddonListCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List addons",
		Long:    "List addons in KubeVela",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := listAddons()
			if err != nil {
				return err
			}
			return nil
		},
	}
}

// NewAddonEnableCommand create addon enable command
func NewAddonEnableCommand(ioStream cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "enable",
		Short:   "enable an addon",
		Long:    "enable an addon in cluster",
		Example: "vela addon enable <addon-name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify addon name")
			}
			name := args[0]
			addonArgs, err := parseToMap(args[1:])
			if err != nil {
				return err
			}
			err = enableAddon(name, addonArgs)
			if err != nil {
				return err
			}
			fmt.Printf("Successfully enable addon:%s\n", name)
			return nil
		},
	}
}

func parseToMap(args []string) (map[string]string, error) {
	res := map[string]string{}
	for _, pair := range args {
		line := strings.Split(pair, "=")
		if len(line) != 2 {
			return nil, fmt.Errorf("parameter format should be foo=bar, %s not match", pair)
		}
		k := strings.TrimSpace(line[0])
		v := strings.TrimSpace(line[1])
		if k != "" && v != "" {
			res[k] = v
		}
	}
	return res, nil
}

// NewAddonDisableCommand create addon disable command
func NewAddonDisableCommand(ioStream cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "disable",
		Short:   "disable an addon",
		Long:    "disable an addon in cluster",
		Example: "vela addon disable <addon-name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify addon name")
			}
			name := args[0]
			err := disableAddon(name)
			if err != nil {
				return err
			}
			fmt.Printf("Successfully disable addon:%s\n", name)
			return nil
		},
	}
}

func listAddons() error {
	repo, err := addonutil.NewAddonRepo()
	if err != nil {
		return err
	}
	addons := repo.ListAddons()
	table := uitable.New()
	table.AddRow("NAME", "DESCRIPTION", "STATUS")
	for _, addon := range addons {
		table.AddRow(addon.Name, addon.Description, addon.GetStatus())
	}
	fmt.Println(table.String())
	return nil
}

func enableAddon(name string, args map[string]string) error {
	repo, err := addonutil.NewAddonRepo()
	if err != nil {
		return err
	}
	addon, err := repo.GetAddon(name)
	if err != nil {
		return err
	}
	addon.SetArgs(args)
	err = addon.Enable()
	return err
}

func disableAddon(name string) error {
	if isLegacyAddonExist(name) {
		return tryDisableInitializerAddon(name)
	}
	repo, err := addonutil.NewAddonRepo()
	if err != nil {
		return err
	}
	addon, err := repo.GetAddon(name)
	if err != nil {
		return errors.Wrap(err, "get addon err")
	}
	if addon.GetStatus() == addonutil.StatusUninstalled {
		fmt.Printf("Addon %s is not installed\n", addon.Name)
		return nil
	}
	return addon.Disable()

}

func isLegacyAddonExist(name string) bool {
	if namespace, ok := legacyAddonNamespace[name]; ok {
		convertedAddonName := addonutil.TransAddonName(name)
		init := unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "core.oam.dev/v1beta1",
				"kind":       "Initializer",
			},
		}
		err := clt.Get(context.TODO(), client.ObjectKey{
			Namespace: namespace,
			Name:      convertedAddonName,
		}, &init)
		return err == nil
	}
	return false
}

func tryDisableInitializerAddon(addonName string) error {
	fmt.Printf("Trying to disable addon in initializer implementation...\n")
	init := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "core.oam.dev/v1beta1",
			"kind":       "Initializer",
			"metadata": map[string]interface{}{
				"name":      addonutil.TransAddonName(addonName),
				"namespace": legacyAddonNamespace[addonName],
			},
		},
	}
	return clt.Delete(context.TODO(), &init)

}
