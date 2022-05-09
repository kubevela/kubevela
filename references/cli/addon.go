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
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"

	"k8s.io/client-go/discovery"

	"helm.sh/helm/v3/pkg/strvals"

	"github.com/oam-dev/kubevela/pkg/oam"

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
	statusEnabled  = "enabled"
	statusDisabled = "disabled"
	statusSuspend  = "suspend"
)

var forceDisable bool
var addonVersion string

var addonClusters string

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
			table, err := listAddons(context.Background(), k8sClient, "")
			if err != nil {
				return err
			}
			fmt.Println(table.String())
			return nil
		},
	}
}

// NewAddonEnableCommand create addon enable command
func NewAddonEnableCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:   "enable",
		Short: "enable an addon",
		Long:  "enable an addon in cluster.",
		Example: `\
Enable addon by:
	vela addon enable <addon-name>
Enable addon with specify version:
	vela addon enable <addon-name> --version <addon-version>
Enable addon for specific clusters, (local means control plane):
	vela addon enable <addon-name> --clusters={local,cluster1,cluster2}
`,
		RunE: func(cmd *cobra.Command, args []string) error {

			if len(args) < 1 {
				return fmt.Errorf("must specify addon name")
			}
			addonArgs, err := parseAddonArgsToMap(args[1:])
			if err != nil {
				return err
			}
			addonArgs[types.ClustersArg] = transClusters(addonClusters)
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			k8sClient, err := c.GetClient()
			if err != nil {
				return err
			}
			dc, err := c.GetDiscoveryClient()
			if err != nil {
				return err
			}
			addonOrDir := args[0]
			var name = addonOrDir
			if file, err := os.Stat(addonOrDir); err == nil {
				if !file.IsDir() {
					return fmt.Errorf("%s is not addon dir", addonOrDir)
				}
				ioStream.Infof("enable addon by local dir: %s \n", addonOrDir)
				// args[0] is a local path install with local dir, use base dir name as addonName
				name = filepath.Base(addonOrDir)
				err = enableAddonByLocal(ctx, name, addonOrDir, k8sClient, dc, config, addonArgs)
				if err != nil {
					return err
				}
			} else {
				if filepath.IsAbs(addonOrDir) || strings.HasPrefix(addonOrDir, ".") || strings.HasSuffix(addonOrDir, "/") {
					return fmt.Errorf("addon directory %s not found in local", addonOrDir)
				}

				err = enableAddon(ctx, k8sClient, dc, config, name, addonVersion, addonArgs)
				if err != nil {
					return err
				}
			}
			fmt.Printf("Addon: %s enabled Successfully.\n", name)
			AdditionalEndpointPrinter(ctx, c, k8sClient, name, false)
			return nil
		},
	}

	cmd.Flags().StringVarP(&addonVersion, "version", "v", "", "specify the addon version to enable")
	cmd.Flags().StringVarP(&addonClusters, types.ClustersArg, "c", "", "specify the runtime-clusters to enable")
	return cmd
}

// AdditionalEndpointPrinter will print endpoints
func AdditionalEndpointPrinter(ctx context.Context, c common.Args, k8sClient client.Client, name string, isUpgrade bool) {
	fmt.Printf("Please access the %s from the following endpoints:\n", name)
	err := printAppEndpoints(ctx, k8sClient, pkgaddon.Convert2AppName(name), types.DefaultKubeVelaNS, Filter{}, c)
	if err != nil {
		fmt.Println("Get application endpoints error:", err)
		return
	}
	if name == "velaux" {
		if !isUpgrade {
			fmt.Println(`To check the initialized admin user name and password by:`)
			fmt.Println(`    vela logs -n vela-system --name apiserver addon-velaux | grep "initialized admin username"`)
		}
		fmt.Println(`To open the dashboard directly by port-forward:`)
		fmt.Println(`    vela port-forward -n vela-system addon-velaux 9082:80`)
		fmt.Println(`Select "Cluster: local | Namespace: vela-system | Kind: Service | Name: velaux" from the prompt.`)
		fmt.Println(`Please refer to https://kubevela.io/docs/reference/addons/velaux for more VelaUX addon installation and visiting method.`)
	}
}

// NewAddonUpgradeCommand create addon upgrade command
func NewAddonUpgradeCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "upgrade an addon",
		Long:  "upgrade an addon in cluster.",
		Example: `\
Upgrade addon by:
	vela addon upgrade <addon-name>
Upgrade addon with specify version:
	vela addon upgrade <addon-name> --version <addon-version>
Upgrade addon for specific clusters, (local means control plane):
	vela addon upgrade <addon-name> --clusters={local,cluster1,cluster2}
`,
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
			dc, err := c.GetDiscoveryClient()
			if err != nil {
				return err
			}
			addonArgs, err := parseAddonArgsToMap(args[1:])
			if err != nil {
				return err
			}
			addonArgs[types.ClustersArg] = transClusters(addonClusters)
			addonOrDir := args[0]
			var name string
			if file, err := os.Stat(addonOrDir); err == nil {
				if !file.IsDir() {
					return fmt.Errorf("%s is not addon dir", addonOrDir)
				}
				ioStream.Infof("enable addon by local dir: %s \n", addonOrDir)
				// args[0] is a local path install with local dir
				name = filepath.Base(addonOrDir)
				_, err = pkgaddon.FetchAddonRelatedApp(context.Background(), k8sClient, name)
				if err != nil {
					return errors.Wrapf(err, "cannot fetch addon related addon %s", name)
				}
				err = enableAddonByLocal(ctx, name, addonOrDir, k8sClient, dc, config, addonArgs)
				if err != nil {
					return err
				}
			} else {
				if filepath.IsAbs(addonOrDir) || strings.HasPrefix(addonOrDir, ".") || strings.HasSuffix(addonOrDir, "/") {
					return fmt.Errorf("addon directory %s not found in local", addonOrDir)
				}
				name = addonOrDir
				_, err = pkgaddon.FetchAddonRelatedApp(context.Background(), k8sClient, addonOrDir)
				if err != nil {
					return errors.Wrapf(err, "cannot fetch addon related addon %s", addonOrDir)
				}
				err = enableAddon(ctx, k8sClient, dc, config, addonOrDir, addonVersion, addonArgs)
				if err != nil {
					return err
				}
			}

			fmt.Printf("Addon: %s\n enabled Successfully.", name)
			AdditionalEndpointPrinter(ctx, c, k8sClient, name, true)
			return nil
		},
	}
	cmd.Flags().StringVarP(&addonVersion, "version", "v", "", "specify the addon version to upgrade")
	return cmd
}

func parseAddonArgsToMap(args []string) (map[string]interface{}, error) {
	res := map[string]interface{}{}
	for _, arg := range args {
		if err := strvals.ParseInto(arg, res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// NewAddonDisableCommand create addon disable command
func NewAddonDisableCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
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
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			err = disableAddon(k8sClient, name, config, forceDisable)
			if err != nil {
				return err
			}
			fmt.Printf("Successfully disable addon:%s\n", name)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&forceDisable, "force", "f", false, "skip checking if applications are still using this addon")
	return cmd
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

func enableAddon(ctx context.Context, k8sClient client.Client, dc *discovery.DiscoveryClient, config *rest.Config, name string, version string, args map[string]interface{}) error {
	var err error
	registryDS := pkgaddon.NewRegistryDataStore(k8sClient)
	registries, err := registryDS.ListRegistries(ctx)
	if err != nil {
		return err
	}

	for _, registry := range registries {
		err = pkgaddon.EnableAddon(ctx, name, version, k8sClient, dc, apply.NewAPIApplicator(k8sClient), config, registry, args, nil)
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
func enableAddonByLocal(ctx context.Context, name string, dir string, k8sClient client.Client, dc *discovery.DiscoveryClient, config *rest.Config, args map[string]interface{}) error {
	if err := pkgaddon.EnableAddonByLocalDir(ctx, name, dir, k8sClient, dc, apply.NewAPIApplicator(k8sClient), config, args); err != nil {
		return err
	}
	if err := waitApplicationRunning(k8sClient, name); err != nil {
		return err
	}
	return nil
}

func disableAddon(client client.Client, name string, config *rest.Config, force bool) error {
	if err := pkgaddon.DisableAddon(context.Background(), client, name, config, force); err != nil {
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

	fmt.Print(generateAddonInfo(name, status))

	if status.AddonPhase != statusEnabled && status.AddonPhase != statusDisabled {
		fmt.Printf("diagnose addon info from application %s", pkgaddon.Convert2AppName(name))
		err := printAppStatus(context.Background(), k8sClient, ioStreams, pkgaddon.Convert2AppName(name), types.DefaultKubeVelaNS, cmd, c)
		if err != nil {
			return err
		}
	}
	return nil
}

func generateAddonInfo(name string, status pkgaddon.Status) string {
	var res string
	var phase string

	switch status.AddonPhase {
	case statusEnabled:
		c := color.New(color.FgGreen)
		phase = c.Sprintf("%s", status.AddonPhase)
	case statusSuspend:
		c := color.New(color.FgRed)
		phase = c.Sprintf("%s", status.AddonPhase)
	default:
		phase = status.AddonPhase
	}
	res += fmt.Sprintf("addon %s status is %s \n", name, phase)
	if len(status.InstalledVersion) != 0 {
		res += fmt.Sprintf("installedVersion: %s \n", status.InstalledVersion)
	}

	if len(status.Clusters) != 0 {
		var ic []string
		for c := range status.Clusters {
			ic = append(ic, c)
		}
		sort.Strings(ic)
		res += fmt.Sprintf("installedClusters: %s \n", ic)
	}
	return res
}

func listAddons(ctx context.Context, clt client.Client, registry string) (*uitable.Table, error) {
	var addons []*pkgaddon.UIData
	var err error
	registryDS := pkgaddon.NewRegistryDataStore(clt)
	registries, err := registryDS.ListRegistries(ctx)
	if err != nil {
		return nil, err
	}

	for _, r := range registries {
		if registry != "" && r.Name != registry {
			continue
		}
		var addonList []*pkgaddon.UIData
		var err error
		if !pkgaddon.IsVersionRegistry(r) {
			meta, err := r.ListAddonMeta()
			if err != nil {
				continue
			}
			addonList, err = r.ListUIData(meta, pkgaddon.CLIMetaOptions)
			if err != nil {
				continue
			}
		} else {
			versionedRegistry := pkgaddon.BuildVersionedRegistry(r.Name, r.Helm.URL)
			addonList, err = versionedRegistry.ListAddon()
			if err != nil {
				continue
			}
		}
		addons = mergeAddons(addons, addonList)
	}

	table := uitable.New()
	table.AddRow("NAME", "REGISTRY", "DESCRIPTION", "AVAILABLE-VERSIONS", "STATUS")

	// get locally installed addons first
	locallyInstalledAddons := map[string]bool{}
	appList := v1beta1.ApplicationList{}
	if err := clt.List(ctx, &appList, client.MatchingLabels{oam.LabelAddonRegistry: pkgaddon.LocalAddonRegistryName}); err != nil {
		return table, err
	}
	for _, app := range appList.Items {
		labels := app.GetLabels()
		addonName := labels[oam.LabelAddonName]
		addonVersion := labels[oam.LabelAddonVersion]
		table.AddRow(addonName, app.GetLabels()[oam.LabelAddonRegistry], "", genAvailableVersionInfo([]string{addonVersion}, addonVersion), statusEnabled)
		locallyInstalledAddons[addonName] = true
	}

	for _, addon := range addons {
		// if the addon with same name has already installed locally, display the registry one as not installed
		if locallyInstalledAddons[addon.Name] {
			table.AddRow(addon.Name, addon.RegistryName, addon.Description, addon.AvailableVersions, "disabled")
			continue
		}
		status, err := pkgaddon.GetAddonStatus(ctx, clt, addon.Name)
		if err != nil {
			return table, err
		}
		statusRow := status.AddonPhase
		if len(status.InstalledVersion) != 0 {
			statusRow += fmt.Sprintf(" (%s)", status.InstalledVersion)
		}
		table.AddRow(addon.Name, addon.RegistryName, addon.Description, genAvailableVersionInfo(addon.AvailableVersions, status.InstalledVersion), statusRow)
	}

	return table, nil
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

// generate the available version
// this func put the installed version as the first version and keep the origin order
// print ... if available version too much
func genAvailableVersionInfo(versions []string, installedVersion string) string {
	var v []string

	// put installed-version as the first version and keep the origin order
	if len(installedVersion) != 0 {
		for i, version := range versions {
			if version == installedVersion {
				v = append(v, version)
				versions = append(versions[:i], versions[i+1:]...)
			}
		}
	}
	v = append(v, versions...)

	res := "["
	var count int
	for _, version := range v {
		if count == 3 {
			// just show  newest 3 versions
			res += "..."
			break
		}
		if version == installedVersion {
			col := color.New(color.Bold, color.FgGreen)
			res += col.Sprintf("%s", version)
		} else {
			res += version
		}
		res += ", "
		count++
	}
	res = strings.TrimSuffix(res, ", ")
	res += "]"
	return res
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

func transClusters(cstr string) []string {
	cstr = strings.TrimPrefix(strings.TrimSuffix(cstr, "}"), "{")
	var clusterL []string
	clusterList := strings.Split(cstr, ",")
	for _, v := range clusterList {
		clusterL = append(clusterL, strings.TrimSpace(v))
	}
	return clusterL
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
