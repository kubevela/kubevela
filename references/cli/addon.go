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
	"time"

	"k8s.io/client-go/rest"

	"github.com/gosuri/uitable"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	types2 "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	// DescAnnotation records the description of addon
	DescAnnotation = "addons.oam.dev/description"

	// DependsOnWorkFlowStepName is workflow step name which is used to check dependsOn app
	DependsOnWorkFlowStepName = "depends-on-app"

	// AddonTerraformProviderNamespace is the namespace of addon terraform provider
	AddonTerraformProviderNamespace = "default"
	// AddonTerraformProviderNameArgument is the argument name of addon terraform provider
	AddonTerraformProviderNameArgument = "providerName"
)

const (
	statusEnabling = "enabling"
)

var clt client.Client
var clientArgs common.Args

// var legacyAddonNamespace map[string]string

func init() {
	clientArgs, _ = common.InitBaseRestConfig()
	clt, _ = clientArgs.GetClient()

	// assume KubeVela 1.2 needn't consider the compatibility of 1.1
	// legacyAddonNamespace = map[string]string{
	//	"fluxcd":                     types.DefaultKubeVelaNS,
	//	"ns-flux-system":             types.DefaultKubeVelaNS,
	//	"kruise":                     types.DefaultKubeVelaNS,
	//	"prometheus":                 types.DefaultKubeVelaNS,
	//	"observability":              "observability",
	//	"observability-asset":        types.DefaultKubeVelaNS,
	//	"istio":                      "istio-system",
	//	"ns-istio-system":            types.DefaultKubeVelaNS,
	//	"keda":                       types.DefaultKubeVelaNS,
	//	"ocm-cluster-manager":        types.DefaultKubeVelaNS,
	//	"terraform":                  types.DefaultKubeVelaNS,
	//	"terraform-provider/alibaba": "default",
	//	"terraform-provider/azure":   "default",
	// }
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
		NewAddonEnableCommand(c, ioStreams),
		NewAddonDisableCommand(ioStreams),
		NewAddonStatusCommand(ioStreams),
		NewAddonRegistryCommand(c, ioStreams),
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
			err := listAddons(context.Background(), "")
			if err != nil {
				return err
			}
			return nil
		},
	}
}

// NewAddonEnableCommand create addon enable command
func NewAddonEnableCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	return &cobra.Command{
		Use:     "enable",
		Short:   "enable an addon",
		Long:    "enable an addon in cluster",
		Example: "vela addon enable <addon-name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			k8sClient, err := c.GetClient()
			if err != nil {
				return err
			}

			if len(args) < 1 {
				return fmt.Errorf("must specify addon name")
			}
			name := args[0]
			addonArgs, err := parseToMap(args[1:])
			if err != nil {
				return err
			}
			err = enableAddon(ctx, k8sClient, c.Config, name, addonArgs)
			if err != nil {
				return err
			}
			fmt.Printf("Successfully enable addon:%s\n", name)
			if name == "velaux" {
				fmt.Println(`Please use command: "vela port-forward -n vela-system addon-velaux 9082:80" and Select "Cluster: local | Namespace: vela-system | Component: velaux | Kind: Service" to check the dashboard`)
			}
			return nil
		},
	}
}

func parseToMap(args []string) (map[string]interface{}, error) {
	res := map[string]interface{}{}
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

// NewAddonStatusCommand create addon status command
func NewAddonStatusCommand(ioStream cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Short:   "get an addon's status",
		Long:    "get an addon's status from cluster",
		Example: "vela addon status <addon-name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify addon name")
			}
			name := args[0]
			err := statusAddon(name)
			if err != nil {
				return err
			}
			return nil
		},
	}
}

func enableAddon(ctx context.Context, k8sClient client.Client, config *rest.Config, name string, args map[string]interface{}) error {
	var err error
	registryDS := pkgaddon.NewRegistryDataStore(k8sClient)
	registries, err := registryDS.ListRegistries(ctx)
	if err != nil {
		return err
	}

	for _, registry := range registries {
		err = pkgaddon.EnableAddon(ctx, name, k8sClient, apply.NewAPIApplicator(k8sClient), config, registry, args, nil)
		if errors.Is(err, pkgaddon.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err = waitApplicationRunning(name); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("addon: %s not found in registrys", name)
}

func disableAddon(name string) error {
	if err := pkgaddon.DisableAddon(context.Background(), clt, name); err != nil {
		return err
	}
	return nil
}

func statusAddon(name string) error {
	status, err := pkgaddon.GetAddonStatus(context.Background(), clt, name)
	if err != nil {
		return err
	}
	fmt.Printf("addon %s status is %s \n", name, status.AddonPhase)
	if status.AddonPhase == statusEnabling {
		fmt.Printf("this addon is still enabling, please run \"vela status %s -n vela-system \" to check the status of the addon related app", pkgaddon.Convert2AppName(name))
	}
	return nil
}

func listAddons(ctx context.Context, registry string) error {
	var addons []*pkgaddon.UIData
	var err error
	registryDS := pkgaddon.NewRegistryDataStore(clt)
	registries, err := registryDS.ListRegistries(ctx)
	if err != nil {
		return err
	}

	for _, r := range registries {
		if registry != "" && r.Name != registry {
			continue
		}

		var source pkgaddon.Source
		if r.Oss != nil {
			source = r.Oss
		} else {
			source = r.Git
		}
		meta, err := source.ListRegistryMeta()
		if err != nil {
			continue
		}
		addList, err := source.ListUIData(meta, pkgaddon.CLIMetaOptions)
		if err != nil {
			continue
		}
		addons = mergeAddons(addons, addList)
	}

	table := uitable.New()
	table.AddRow("NAME", "DESCRIPTION", "STATUS")

	for _, addon := range addons {
		status, err := pkgaddon.GetAddonStatus(ctx, clt, addon.Name)
		if err != nil {
			return err
		}
		table.AddRow(addon.Name, addon.Description, status.AddonPhase)
	}
	fmt.Println(table.String())
	return nil
}

func waitApplicationRunning(addonName string) error {
	trackInterval := 5 * time.Second
	timeout := 600 * time.Second
	start := time.Now()
	ctx := context.Background()
	var app v1beta1.Application
	spinner := newTrackingSpinnerWithDelay("Waiting addon running ...", 1*time.Second)
	spinner.Start()
	defer spinner.Stop()

	for {
		err := clt.Get(ctx, types2.NamespacedName{Name: pkgaddon.Convert2AppName(addonName), Namespace: types.DefaultKubeVelaNS}, &app)
		if err != nil {
			return client.IgnoreNotFound(err)
		}
		phase := app.Status.Phase
		if phase == common2.ApplicationRunning {
			return nil
		}
		timeConsumed := int(time.Since(start).Seconds())
		applySpinnerNewSuffix(spinner, fmt.Sprintf("Waiting addon application running. It is now in phase: %s (timeout %d/%d seconds)...",
			phase, timeConsumed, int(timeout.Seconds())))
		if timeConsumed > int(timeout.Seconds()) {
			return errors.Errorf("Enabling timeout, please run \"vela status %s -n vela-system\" to check the status of the addon", addonName)
		}
		time.Sleep(trackInterval)
	}

}

// TransAddonName will turn addon's name from xxx/yyy to xxx-yyy
func TransAddonName(name string) string {
	return strings.ReplaceAll(name, "/", "-")
}


func mergeAddons(a1, a2 []*pkgaddon.UIData) []*pkgaddon.UIData {
	for _, item := range a2 {
		if hasAddon(a1, item.Name) {
			continue
		}
		a1 = append(a1, item)
	}
	return a1
}

func hasAddon(addons []*pkgaddon.UIData, name string) bool {
	for _, addon := range addons {
		if addon.Name == name {
			return true
		}
	}
	return false
}

// TODO(wangyike) addon can support multi-tenancy, an addon can be enabled multi times and will create many times
// func checkWhetherTerraformProviderExist(ctx context.Context, k8sClient client.Client, addonName string, args map[string]string) (string, bool, error) {
//	_, providerName := getTerraformProviderArgumentValue(addonName, args)
//
//	providerNames, err := getTerraformProviderNames(ctx, k8sClient)
//	if err != nil {
//		return "", false, err
//	}
//	for _, name := range providerNames {
//		if providerName == name {
//			return providerName, true, nil
//		}
//	}
//	return providerName, false, nil
// }

//  func getTerraformProviderNames(ctx context.Context, k8sClient client.Client) ([]string, error) {
//	var names []string
//	providerList := &terraformv1beta1.ProviderList{}
//	err := k8sClient.List(ctx, providerList, client.InNamespace(AddonTerraformProviderNamespace))
//	if err != nil {
//		if apimeta.IsNoMatchError(err) || kerrors.IsNotFound(err) {
//			return nil, nil
//		}
//		return nil, err
//	}
//	for _, provider := range providerList.Items {
//		names = append(names, provider.Name)
//	}
//	return names, nil
// }
//
// Get the value of argument AddonTerraformProviderNameArgument
// func getTerraformProviderArgumentValue(addonName string, args map[string]string) (map[string]string, string) {
//	providerName, ok := args[AddonTerraformProviderNameArgument]
//	if !ok {
//		switch addonName {
//		case "terraform-alibaba":
//			providerName = "default"
//		case "terraform-aws":
//			providerName = "aws"
//		case "terraform-azure":
//			providerName = "azure"
//		}
//		args[AddonTerraformProviderNameArgument] = providerName
//	}
//	return args, providerName
// }
