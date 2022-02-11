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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam"

	"k8s.io/client-go/rest"

	"github.com/gosuri/uitable"
	"github.com/olekukonko/tablewriter"

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
	statusEnabled  = "enabled"
	statusDisabled = "disabled"
)

// NewAddonCommand create `addon` command
func NewAddonCommand(c common.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
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
		NewAddonListCommand(c),
		NewAddonEnableCommand(c, ioStreams),
		NewAddonDisableCommand(c, ioStreams),
		NewAddonStatusCommand(c, ioStreams),
		NewAddonRegistryCommand(c, ioStreams),
		NewAddonUpgradeCommand(c, ioStreams),
	)
	return cmd
}

// NewAddonListCommand create addon list command
func NewAddonListCommand(c common.Args) *cobra.Command {
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
			err = listAddons(context.Background(), k8sClient, "")
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
	cmd := &cobra.Command{
		Use:     "enable",
		Short:   "enable an addon",
		Long:    "enable an addon in cluster.",
		Example: "vela addon enable <addon-name>",
		RunE: func(cmd *cobra.Command, args []string) error {

			if len(args) < 1 {
				return fmt.Errorf("must specify addon name")
			}
			addonArgs, err := parseToMap(args[1:])
			if err != nil {
				return err
			}
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			k8sClient, err := c.GetClient()
			if err != nil {
				return err
			}
			addonOrDir := args[0]
			var name = addonOrDir
			if file, err := os.Stat(addonOrDir); err == nil {
				if !file.IsDir() {
					return fmt.Errorf("%s is not addon dir", addonOrDir)
				}
				ioStream.Infof("enable addon by local dir: %s", addonOrDir)
				// args[0] is a local path install with local dir, use base dir name as addonName
				name = filepath.Base(addonOrDir)
				err = enableAddonByLocal(ctx, name, addonOrDir, k8sClient, config, addonArgs)
				if err != nil {
					return err
				}
			} else {
				err = enableAddon(ctx, k8sClient, config, name, addonArgs)
				if err != nil {
					return err
				}
			}
			fmt.Printf("Addon: %s enabled Successfully.\n", name)
			AdditionalEndpointPrinter(ctx, c, k8sClient, name)
			return nil
		},
	}
	return cmd
}

// AdditionalEndpointPrinter will print endpoints
func AdditionalEndpointPrinter(ctx context.Context, c common.Args, k8sClient client.Client, name string) {
	endpoints, _ := GetServiceEndpoints(ctx, k8sClient, pkgaddon.Convert2AppName(name), types.DefaultKubeVelaNS, c)
	if len(endpoints) > 0 {
		table := tablewriter.NewWriter(os.Stdout)
		table.SetColWidth(100)
		table.SetHeader([]string{"Cluster", "Ref(Kind/Namespace/Name)", "Endpoint"})
		for _, endpoint := range endpoints {
			table.Append([]string{endpoint.Cluster, fmt.Sprintf("%s/%s/%s", endpoint.Ref.Kind, endpoint.Ref.Namespace, endpoint.Ref.Name), endpoint.String()})
		}
		fmt.Printf("Please access the %s from the following endpoints:\n", name)
		table.Render()
		return
	}
	if name == "velaux" {
		fmt.Println(`Please use command: "vela port-forward -n vela-system addon-velaux 9082:80" and Select "Cluster: local | Namespace: vela-system | Component: velaux | Kind: Service" to check the dashboard.`)
	}
}

// NewAddonUpgradeCommand create addon upgrade command
func NewAddonUpgradeCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:     "upgrade",
		Short:   "upgrade an addon",
		Long:    "upgrade an addon in cluster.",
		Example: "vela addon upgrade <addon-name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify addon name")
			}
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			k8sClient, err := c.GetClient()
			if err != nil {
				return err
			}
			addonArgs, err := parseToMap(args[1:])
			if err != nil {
				return err
			}
			addonOrDir := args[0]
			var name string
			if file, err := os.Stat(addonOrDir); err == nil {
				if !file.IsDir() {
					return fmt.Errorf("%s is not addon dir", addonOrDir)
				}
				ioStream.Infof("enable addon by local dir: %s", addonOrDir)
				// args[0] is a local path install with local dir
				name := filepath.Base(addonOrDir)
				_, err = pkgaddon.FetchAddonRelatedApp(context.Background(), k8sClient, name)
				if err != nil {
					return errors.Wrapf(err, "cannot fetch addon related addon %s", name)
				}
				err = enableAddonByLocal(ctx, name, addonOrDir, k8sClient, config, addonArgs)
				if err != nil {
					return err
				}
			} else {
				_, err = pkgaddon.FetchAddonRelatedApp(context.Background(), k8sClient, addonOrDir)
				if err != nil {
					return errors.Wrapf(err, "cannot fetch addon related addon %s", addonOrDir)
				}
				err = enableAddon(ctx, k8sClient, config, addonOrDir, addonArgs)
				if err != nil {
					return err
				}
			}

			fmt.Printf("Addon: %s\n enabled Successfully.", name)
			AdditionalEndpointPrinter(ctx, c, k8sClient, name)
			return nil
		},
	}
	return cmd
}

func parseToMap(args []string) (map[string]interface{}, error) {
	res := map[string]interface{}{}
	for _, pair := range args {
		line := strings.Split(pair, "=")
		if len(line) < 2 {
			return nil, fmt.Errorf("parameter format should be foo=bar, %s not match", pair)
		}
		k := strings.TrimSpace(line[0])
		v := strings.TrimSpace(strings.Join(line[1:], "="))
		if k != "" && v != "" {
			res[k] = v
		}
	}
	return res, nil
}

// NewAddonDisableCommand create addon disable command
func NewAddonDisableCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "disable",
		Short:   "disable an addon",
		Long:    "disable an addon in cluster.",
		Example: "vela addon disable <addon-name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify addon name")
			}
			name := args[0]
			k8sClient, err := c.GetClient()
			if err != nil {
				return err
			}
			err = disableAddon(k8sClient, name)
			if err != nil {
				return err
			}
			fmt.Printf("Successfully disable addon:%s\n", name)
			return nil
		},
	}
}

// NewAddonStatusCommand create addon status command
func NewAddonStatusCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Short:   "get an addon's status.",
		Long:    "get an addon's status from cluster.",
		Example: "vela addon status <addon-name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify addon name")
			}
			name := args[0]
			err := statusAddon(name, ioStream, cmd, c)
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
		if err = waitApplicationRunning(k8sClient, name); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("addon: %s not found in registries", name)
}

// enableAddonByLocal enable addon in local dir and return the addon name
func enableAddonByLocal(ctx context.Context, name string, dir string, k8sClient client.Client, config *rest.Config, args map[string]interface{}) error {
	if err := pkgaddon.EnableAddonByLocalDir(ctx, name, dir, k8sClient, apply.NewAPIApplicator(k8sClient), config, args); err != nil {
		return err
	}
	if err := waitApplicationRunning(k8sClient, name); err != nil {
		return err
	}
	return nil
}

func disableAddon(client client.Client, name string) error {
	if err := pkgaddon.DisableAddon(context.Background(), client, name); err != nil {
		return err
	}
	return nil
}

func statusAddon(name string, ioStreams cmdutil.IOStreams, cmd *cobra.Command, c common.Args) error {
	k8sClient, err := c.GetClient()
	if err != nil {
		return err
	}
	status, err := pkgaddon.GetAddonStatus(context.Background(), k8sClient, name)
	if err != nil {
		return err
	}
	fmt.Printf("addon %s status is %s \n", name, status.AddonPhase)
	if status.AddonPhase != statusEnabled && status.AddonPhase != statusDisabled {
		fmt.Printf("diagnose addon info from application %s", pkgaddon.Convert2AppName(name))
		err := printAppStatus(context.Background(), k8sClient, ioStreams, pkgaddon.Convert2AppName(name), types.DefaultKubeVelaNS, cmd, c)
		if err != nil {
			return err
		}
	}
	return nil
}

func listAddons(ctx context.Context, clt client.Client, registry string) error {
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
		addons = mergeAddons(addons, addList)
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

func waitApplicationRunning(k8sClient client.Client, addonName string) error {
	trackInterval := 5 * time.Second
	timeout := 600 * time.Second
	start := time.Now()
	ctx := context.Background()
	var app v1beta1.Application
	spinner := newTrackingSpinnerWithDelay("Waiting addon running ...", 1*time.Second)
	spinner.Start()
	defer spinner.Stop()

	for {
		err := k8sClient.Get(ctx, types2.NamespacedName{Name: pkgaddon.Convert2AppName(addonName), Namespace: types.DefaultKubeVelaNS}, &app)
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
			return errors.Errorf("Enabling timeout, please run \"vela status %s -n vela-system\" to check the status of the addon", pkgaddon.Convert2AppName(addonName))
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
