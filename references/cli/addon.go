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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gosuri/uitable"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/strvals"
	types2 "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/service"
	"github.com/oam-dev/kubevela/pkg/oam"
	addonutil "github.com/oam-dev/kubevela/pkg/utils/addon"
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

var verboseStatus bool

var skipValidate bool

var overrideDefs bool

// NewAddonCommand create `addon` command
func NewAddonCommand(c common.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "addon",
		Short: "Manage addons for extension.",
		Long:  "Manage addons for extension.",
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeApp,
		},
	}
	cmd.AddCommand(
		NewAddonListCommand(c),
		NewAddonEnableCommand(c, ioStreams),
		NewAddonDisableCommand(c, ioStreams),
		NewAddonStatusCommand(c, ioStreams),
		NewAddonRegistryCommand(c, ioStreams),
		NewAddonUpgradeCommand(c, ioStreams),
		NewAddonPackageCommand(c),
		NewAddonInitCommand(),
		NewAddonPushCommand(c),
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
		Example: `  Enable addon by:
	vela addon enable <addon-name>
  Enable addon with specify version:
	vela addon enable <addon-name> --version <addon-version>
  Enable addon for specific clusters, (local means control plane):
	vela addon enable <addon-name> --clusters={local,cluster1,cluster2}
  Enable addon locally:
	vela addon enable <your-local-addon-path>
  Enable addon with specified args (the args should be defined in addon's parameters):
	vela addon enable <addon-name> <my-parameter-of-addon>=<my-value>
`,
		RunE: func(cmd *cobra.Command, args []string) error {

			if len(args) < 1 {
				return fmt.Errorf("must specify addon name")
			}
			addonArgs, err := parseAddonArgsToMap(args[1:])
			if err != nil {
				return err
			}
			clusterArgs := transClusters(addonClusters)
			if len(clusterArgs) != 0 {
				addonArgs[types.ClustersArg] = clusterArgs
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
			addonOrDir := args[0]
			var name = addonOrDir
			if file, err := os.Stat(addonOrDir); err == nil {
				if !file.IsDir() {
					return fmt.Errorf("%s is not addon dir", addonOrDir)
				}
				ioStream.Infof("enable addon by local dir: %s \n", addonOrDir)
				// args[0] is a local path install with local dir, use base dir name as addonName
				abs, err := filepath.Abs(addonOrDir)
				if err != nil {
					return errors.Wrapf(err, "directory %s is invalid", addonOrDir)
				}
				name = filepath.Base(abs)
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
			fmt.Printf("Addon %s enabled successfully.\n", name)
			AdditionalEndpointPrinter(ctx, c, k8sClient, name, false)
			return nil
		},
	}

	cmd.Flags().StringVarP(&addonVersion, "version", "v", "", "specify the addon version to enable")
	cmd.Flags().StringVarP(&addonClusters, types.ClustersArg, "c", "", "specify the runtime-clusters to enable")
	cmd.Flags().BoolVarP(&skipValidate, "skip-version-validating", "s", false, "skip validating system version requirement")
	cmd.Flags().BoolVarP(&overrideDefs, "override-definitions", "", false, "override existing definitions if conflict with those contained in this addon")
	return cmd
}

// AdditionalEndpointPrinter will print endpoints
func AdditionalEndpointPrinter(ctx context.Context, c common.Args, k8sClient client.Client, name string, isUpgrade bool) {
	err := printAppEndpoints(ctx, addonutil.Addon2AppName(name), types.DefaultKubeVelaNS, Filter{}, c, true)
	if err != nil {
		fmt.Println("Get application endpoints error:", err)
		return
	}
	if name == "velaux" {
		if !isUpgrade {
			fmt.Printf("\nInitialized admin username and password: admin / %s \n\n", service.InitAdminPassword)
		}
		fmt.Println(`To open the dashboard directly by port-forward:`)
		fmt.Println()
		fmt.Println(`    vela port-forward -n vela-system addon-velaux 9082:80`)
		fmt.Println()
		fmt.Println(`Select "local | velaux | velaux" from the prompt.`)
		fmt.Println()
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
		Example: `
  Upgrade addon by:
	vela addon upgrade <addon-name>
  Upgrade addon with specify version:
	vela addon upgrade <addon-name> --version <addon-version>
  Upgrade addon for specific clusters, (local means control plane):
	vela addon upgrade <addon-name> --clusters={local,cluster1,cluster2}
  Upgrade addon locally:
	vela addon upgrade <your-local-addon-path>
  Upgrade addon with specified args (the args should be defined in addon's parameters):
	vela addon upgrade <addon-name> <my-parameter-of-addon>=<my-value>
  The specified args will be merged with legacy args, what user specified in 'vela addon enable', and non-empty legacy arg will be overridden by
non-empty new arg
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
			addonInputArgs, err := parseAddonArgsToMap(args[1:])
			if err != nil {
				return err
			}
			clusterArgs := transClusters(addonClusters)
			if len(clusterArgs) != 0 {
				addonInputArgs[types.ClustersArg] = clusterArgs
			}
			addonOrDir := args[0]
			var name string
			if file, err := os.Stat(addonOrDir); err == nil {
				if !file.IsDir() {
					return fmt.Errorf("%s is not addon dir", addonOrDir)
				}
				ioStream.Infof("enable addon by local dir: %s \n", addonOrDir)
				// args[0] is a local path install with local dir
				abs, err := filepath.Abs(addonOrDir)
				if err != nil {
					return errors.Wrapf(err, "cannot open directory %s", addonOrDir)
				}
				name = filepath.Base(abs)
				_, err = pkgaddon.FetchAddonRelatedApp(context.Background(), k8sClient, name)
				if err != nil {
					return errors.Wrapf(err, "cannot fetch addon related addon %s", name)
				}
				addonArgs, err := pkgaddon.MergeAddonInstallArgs(ctx, k8sClient, name, addonInputArgs)
				if err != nil {
					return err
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
				addonArgs, err := pkgaddon.MergeAddonInstallArgs(ctx, k8sClient, name, addonInputArgs)
				if err != nil {
					return err
				}
				err = enableAddon(ctx, k8sClient, dc, config, addonOrDir, addonVersion, addonArgs)
				if err != nil {
					return err
				}
			}

			fmt.Printf("Addon %s enabled successfully.", name)
			AdditionalEndpointPrinter(ctx, c, k8sClient, name, true)
			return nil
		},
	}
	cmd.Flags().StringVarP(&addonVersion, "version", "v", "", "specify the addon version to upgrade")
	cmd.Flags().BoolVarP(&skipValidate, "skip-version-validating", "s", false, "skip validating system version requirement")
	cmd.Flags().BoolVarP(&overrideDefs, "override-definitions", "", false, "override existing definitions if conflict with those contained in this addon")
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
	cmd := &cobra.Command{
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
	cmd.Flags().BoolVarP(&verboseStatus, "verbose", "v", false, "show addon descriptions and parameters in addition to status")
	return cmd
}

// NewAddonInitCommand creates an addon scaffold
func NewAddonInitCommand() *cobra.Command {
	var path string
	initCmd := pkgaddon.InitCmd{}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "create an addon scaffold",
		Long:  "Create an addon scaffold for quick starting.",
		Example: `  Store the scaffold in a different directory:
	vela addon init mongodb -p path/to/addon

  Add a Helm component:
	vela addon init mongodb --helm-repo https://marketplace.azurecr.io/helm/v1/repo --chart mongodb --chart-version 12.1.16

  Add resources from URL using ref-objects component
	vela addon init my-addon --url https://domain.com/resource.yaml

  Use --no-samples options to skip creating sample files
	vela addon init my-addon --no-sample

  You can combine all the options together.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("an addon name is required")
			}

			addonName := args[0]

			// Scaffold will be created in ./addonName, unless the user specifies a path
			// validity of addon names will be checked later
			addonPath := addonName
			if len(path) > 0 {
				addonPath = path
			}

			if addonName == "" || addonPath == "" {
				return fmt.Errorf("addon name or path should not be empty")
			}

			initCmd.AddonName = addonName
			initCmd.Path = addonPath

			return initCmd.CreateScaffold()
		},
	}

	f := cmd.Flags()
	f.StringVar(&initCmd.HelmRepoURL, "helm-repo", "", "URL that points to a Helm repo")
	f.StringVar(&initCmd.HelmChartName, "chart", "", "Helm Chart name")
	f.StringVar(&initCmd.HelmChartVersion, "chart-version", "", "version of the Chart")
	f.StringVarP(&path, "path", "p", "", "path to the addon directory (default is ./<addon-name>)")
	f.StringArrayVarP(&initCmd.RefObjURLs, "url", "u", []string{}, "add URL resources using ref-object component")
	f.BoolVarP(&initCmd.NoSamples, "no-samples", "", false, "do not generate sample files")
	f.BoolVarP(&initCmd.Overwrite, "force", "f", false, "overwrite existing addon files")

	return cmd
}

// NewAddonPushCommand pushes an addon dir/package to a ChartMuseum
func NewAddonPushCommand(c common.Args) *cobra.Command {
	p := &pkgaddon.PushCmd{}
	cmd := &cobra.Command{
		Use:   "push",
		Short: "uploads an addon package to ChartMuseum",
		Long: `Uploads an addon package to ChartMuseum.

Two arguments are needed <addon directory/package> and <name/URL of ChartMuseum>.

The first argument <addon directory/package> can be:
	- your conventional addon directory (containing metadata.yaml). We will package it for you.
	- packaged addon (.tgz) generated by 'vela addon package' command

The second argument <name/URL of ChartMuseum> can be:
	- registry name (helm type). You can add your ChartMuseum registry using 'vela addon registry add'.
	- ChartMuseum URL, e.g. http://localhost:8080`,
		Example: `# Push the addon in directory <your-addon> to a ChartMuseum registry named <localcm>
$ vela addon push your-addon localcm

# Push packaged addon mongo-1.0.0.tgz to a ChartMuseum registry at http://localhost:8080
$ vela addon push mongo-1.0.0.tgz http://localhost:8080

# Force push, overwriting existing ones
$ vela addon push your-addon localcm -f

# If you already written your own Chart.yaml and don't want us to generate it for you:
$ vela addon push your-addon localcm --keep-chartmeta
# Note: when using .tgz packages, we will always keep the original Chart.yaml

# In addition to cli flags, you can also use environment variables
$ HELM_REPO_USERNAME=name HELM_REPO_PASSWORD=pswd vela addon push mongo-1.0.0.tgz http://localhost:8080`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("two arguments are needed: addon directory/package, name/URL of Chart repository")
			}

			c, err := c.GetClient()
			if err != nil {
				return err
			}

			p.Client = c
			p.Out = cmd.OutOrStdout()
			p.ChartName = args[0]
			p.RepoName = args[1]
			p.SetFieldsFromEnv()

			return p.Push(context.Background())
		},
	}

	f := cmd.Flags()
	f.StringVarP(&p.ChartVersion, "version", "v", "", "override chart version pre-push")
	f.StringVarP(&p.AppVersion, "app-version", "a", "", "override app version pre-push")
	f.StringVarP(&p.Username, "username", "u", "", "override HTTP basic auth username [$HELM_REPO_USERNAME]")
	f.StringVarP(&p.Password, "password", "p", "", "override HTTP basic auth password [$HELM_REPO_PASSWORD]")
	f.StringVarP(&p.AccessToken, "access-token", "", "", "send token in Authorization header [$HELM_REPO_ACCESS_TOKEN]")
	f.StringVarP(&p.AuthHeader, "auth-header", "", "", "alternative header to use for token auth [$HELM_REPO_AUTH_HEADER]")
	f.StringVarP(&p.ContextPath, "context-path", "", "", "ChartMuseum context path [$HELM_REPO_CONTEXT_PATH]")
	f.StringVarP(&p.CaFile, "ca-file", "", "", "verify certificates of HTTPS-enabled servers using this CA bundle [$HELM_REPO_CA_FILE]")
	f.StringVarP(&p.CertFile, "cert-file", "", "", "identify HTTPS client using this SSL certificate file [$HELM_REPO_CERT_FILE]")
	f.StringVarP(&p.KeyFile, "key-file", "", "", "identify HTTPS client using this SSL key file [$HELM_REPO_KEY_FILE]")
	f.BoolVarP(&p.InsecureSkipVerify, "insecure", "", false, "connect to server with an insecure way by skipping certificate verification [$HELM_REPO_INSECURE]")
	f.BoolVarP(&p.ForceUpload, "force", "f", false, "force upload even if chart version exists")
	f.BoolVarP(&p.UseHTTP, "use-http", "", false, "use HTTP")
	f.BoolVarP(&p.KeepChartMetadata, "keep-chartmeta", "", false, "do not update Chart.yaml automatically according to addon metadata (only when addon dir provided)")
	f.Int64VarP(&p.Timeout, "timeout", "t", 30, "The duration (in seconds) vela cli will wait to get response from ChartMuseum")

	return cmd
}

func enableAddon(ctx context.Context, k8sClient client.Client, dc *discovery.DiscoveryClient, config *rest.Config, name string, version string, args map[string]interface{}) error {
	var err error
	registryDS := pkgaddon.NewRegistryDataStore(k8sClient)
	registries, err := registryDS.ListRegistries(ctx)
	if err != nil {
		return err
	}

	for _, registry := range registries {
		var opts []pkgaddon.InstallOption
		if skipValidate {
			opts = append(opts, pkgaddon.SkipValidateVersion)
		}
		if overrideDefs {
			opts = append(opts, pkgaddon.OverrideDefinitions)
		}
		err = pkgaddon.EnableAddon(ctx, name, version, k8sClient, dc, apply.NewAPIApplicator(k8sClient), config, registry, args, nil, opts...)
		if errors.Is(err, pkgaddon.ErrNotExist) {
			continue
		}
		if unMatchErr := new(pkgaddon.VersionUnMatchError); errors.As(err, unMatchErr) {
			// Get available version of the addon
			availableVersion, err := unMatchErr.GetAvailableVersion()
			if err != nil {
				return err
			}
			input := NewUserInput()
			if input.AskBool(unMatchErr.Error(), &UserInputOptions{AssumeYes: false}) {
				err = pkgaddon.EnableAddon(ctx, name, availableVersion, k8sClient, dc, apply.NewAPIApplicator(k8sClient), config, registry, args, nil)
				return err
			}
			// The user does not agree to use the version provided by us
			return fmt.Errorf("you can try another version by command: \"vela addon enable %s --version <version> \" ", name)
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
	var opts []pkgaddon.InstallOption
	if skipValidate {
		opts = append(opts, pkgaddon.SkipValidateVersion)
	}
	if overrideDefs {
		opts = append(opts, pkgaddon.OverrideDefinitions)
	}
	if err := pkgaddon.EnableAddonByLocalDir(ctx, name, dir, k8sClient, dc, apply.NewAPIApplicator(k8sClient), config, args, opts...); err != nil {
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

	statusString, status, err := generateAddonInfo(k8sClient, name)
	if err != nil {
		return err
	}

	fmt.Print(statusString)

	if status.AddonPhase != statusEnabled && status.AddonPhase != statusDisabled {
		fmt.Printf("diagnose addon info from application %s", addonutil.Addon2AppName(name))
		err := printAppStatus(context.Background(), k8sClient, ioStreams, addonutil.Addon2AppName(name), types.DefaultKubeVelaNS, cmd, c)
		if err != nil {
			return err
		}
	}
	return nil
}

// generateAddonInfo will get addon status, description, version, dependencies (and whether they are installed),
// and parameters (and their current values).
// The first return value is the formatted string for printing.
// The second return value is just for diagnostic purposes, as it is needed in statusAddon to print diagnostic info.
func generateAddonInfo(c client.Client, name string) (string, pkgaddon.Status, error) {
	var res string
	var phase string
	var installed bool
	var addonPackage *pkgaddon.WholeAddonPackage

	// Get addon install package
	if verboseStatus {
		// We need the metadata to get descriptions about parameters
		addonPackages, err := pkgaddon.FindWholeAddonPackagesFromRegistry(context.Background(), c, []string{name}, nil)
		// Not found error can be ignored, because the user can define their own addon. Others can't.
		if err != nil && !errors.Is(err, pkgaddon.ErrNotExist) && !errors.Is(err, pkgaddon.ErrRegistryNotExist) {
			return "", pkgaddon.Status{}, err
		}
		if len(addonPackages) != 0 {
			addonPackage = addonPackages[0]
		}
	}

	// Check current addon status
	status, err := pkgaddon.GetAddonStatus(context.Background(), c, name)
	if err != nil {
		return res, status, err
	}

	switch status.AddonPhase {
	case statusEnabled:
		installed = true
		c := color.New(color.FgGreen)
		phase = c.Sprintf("%s", status.AddonPhase)
	case statusSuspend:
		installed = true
		c := color.New(color.FgRed)
		phase = c.Sprintf("%s", status.AddonPhase)
	case statusDisabled:
		c := color.New(color.Faint)
		phase = c.Sprintf("%s", status.AddonPhase)
		// If the addon is
		// 1. disabled,
		// 2. does not exist in the registry,
		// 3. verbose is on (when off, it is not possible to know whether the addon is in registry or not),
		// means the addon does not exist at all.
		// So, no need to go further, we return error message saying that we can't find it.
		if addonPackage == nil && verboseStatus {
			return res, pkgaddon.Status{}, fmt.Errorf("addon %s is not found in registries nor locally installed", name)
		}
	default:
		c := color.New(color.Faint)
		phase = c.Sprintf("%s", status.AddonPhase)
	}

	// Addon name
	res += color.New(color.Bold).Sprintf("%s", name)
	res += fmt.Sprintf(": %s ", phase)
	if installed {
		res += fmt.Sprintf("(%s)", status.InstalledVersion)
	}
	res += "\n"

	// Description
	// Skip this if addon is installed from local sources.
	// Description is fetched from the Internet, which is not useful for local sources.
	if status.InstalledRegistry != pkgaddon.LocalAddonRegistryName && addonPackage != nil {
		res += fmt.Sprintln(addonPackage.Description)
	}

	// Installed Clusters
	if len(status.Clusters) != 0 {
		res += color.New(color.FgHiBlue).Sprint("==> ") + color.New(color.Bold).Sprintln("Installed Clusters")
		var ic []string
		for c := range status.Clusters {
			ic = append(ic, c)
		}
		sort.Strings(ic)
		res += fmt.Sprintln(ic)
	}

	// Registry name
	registryName := status.InstalledRegistry
	// Disabled addons will have empty InstalledRegistry, so if the addon exists in the registry, we use the registry name.
	if registryName == "" && addonPackage != nil {
		registryName = addonPackage.RegistryName
	}
	if registryName != "" {
		res += color.New(color.FgHiBlue).Sprint("==> ") + color.New(color.Bold).Sprintln("Registry Name")
		res += fmt.Sprintln(registryName)
	}

	// If the addon is installed from local sources, or does not exist at all, stop here!
	// The following information is fetched from the Internet, which is not useful for local sources.
	if registryName == pkgaddon.LocalAddonRegistryName || registryName == "" || addonPackage == nil {
		return res, status, nil
	}

	// Available Versions
	res += color.New(color.FgHiBlue).Sprint("==> ") + color.New(color.Bold).Sprintln("Available Versions")
	res += genAvailableVersionInfo(addonPackage.AvailableVersions, status.InstalledVersion, 8)
	res += "\n"

	// Dependencies
	dependenciesString, allInstalled := generateDependencyString(c, addonPackage.Dependencies)
	res += color.New(color.FgHiBlue).Sprint("==> ") + color.New(color.Bold).Sprint("Dependencies ")
	if allInstalled {
		res += color.GreenString("✔")
	} else {
		res += color.RedString("✘")
	}
	res += "\n"
	res += dependenciesString
	res += "\n"

	// Parameters
	parameterString := generateParameterString(status, addonPackage)
	if len(parameterString) != 0 {
		res += color.New(color.FgHiBlue).Sprint("==> ") + color.New(color.Bold).Sprintln("Parameters")
		res += parameterString
	}

	return res, status, nil
}

func generateParameterString(status pkgaddon.Status, addonPackage *pkgaddon.WholeAddonPackage) string {
	ret := ""

	if addonPackage.APISchema == nil {
		return ret
	}

	ret = printSchema(addonPackage.APISchema, status.Parameters, 0)

	return ret
}

// printSchema prints the parameters in an addon recursively to a string
// Deeper the parameter is nested, more the indentations.
func printSchema(ref *openapi3.Schema, currentParams map[string]interface{}, indent int) string {
	ret := ""

	if ref == nil {
		return ret
	}

	addIndent := func(n int) string {
		r := ""
		for i := 0; i < n; i++ {
			r += "\t"
		}
		return r
	}

	// Required parameters
	required := make(map[string]bool)
	for _, k := range ref.Required {
		required[k] = true
	}

	for propKey, propValue := range ref.Properties {
		desc := propValue.Value.Description
		defaultValue := propValue.Value.Default
		if defaultValue == nil {
			defaultValue = ""
		}
		required := required[propKey]

		// Extra indentation on nested objects
		addedIndent := addIndent(indent)

		var currentValue string
		thisParam, hasParam := currentParams[propKey]
		if hasParam {
			currentValue = fmt.Sprintf("%#v", thisParam)
			switch thisParam.(type) {
			case int:
			case int64:
			case int32:
			case float32:
			case float64:
			case string:
			case bool:
			default:
				if js, err := json.MarshalIndent(thisParam, "", "  "); err == nil {
					currentValue = strings.ReplaceAll(string(js), "\n", "\n\t         "+addedIndent)
				}
			}
		}

		// Header: addon: description
		ret += addedIndent
		ret += color.New(color.FgCyan).Sprintf("-> ")
		ret += color.New(color.Bold).Sprint(propKey) + ": "
		ret += fmt.Sprintf("%s\n", desc)

		// Show current value
		if currentValue != "" {
			ret += addedIndent
			ret += "\tcurrent: " + color.New(color.FgGreen).Sprintf("%s\n", currentValue)
		}
		// Show default value
		if defaultValue != "" {
			ret += addedIndent
			ret += "\tdefault: " + fmt.Sprintf("%#v\n", defaultValue)
		}
		// Show required or not
		if required {
			ret += addedIndent
			ret += "\trequired: "
			ret += color.GreenString("✔\n")
		}

		// Object type param, we will get inside the object.
		// To show what's inside nested objects.
		if propValue.Value.Type == "object" {
			nestedParam := make(map[string]interface{})
			if hasParam {
				nestedParam = currentParams[propKey].(map[string]interface{})
			}
			ret += printSchema(propValue.Value, nestedParam, indent+1)
		}
	}

	return ret
}

func generateDependencyString(c client.Client, dependencies []*pkgaddon.Dependency) (string, bool) {
	if len(dependencies) == 0 {
		return "[]", true
	}

	ret := "["
	allDependenciesInstalled := true

	for idx, d := range dependencies {
		name := d.Name

		// Checks if the dependency is enabled, and mark it
		status, err := pkgaddon.GetAddonStatus(context.Background(), c, name)
		if err != nil {
			continue
		}

		var enabledString string
		switch status.AddonPhase {
		case statusEnabled:
			enabledString = color.GreenString("✔")
		case statusSuspend:
			enabledString = color.RedString("✔")
		default:
			enabledString = color.RedString("✘")
			allDependenciesInstalled = false
		}
		ret += fmt.Sprintf("%s %s", name, enabledString)

		if idx != len(dependencies)-1 {
			ret += ", "
		}
	}

	ret += "]"

	return ret, allDependenciesInstalled
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
			versionedRegistry := pkgaddon.BuildVersionedRegistry(r.Name, r.Helm.URL, &common.HTTPOption{
				Username:        r.Helm.Username,
				Password:        r.Helm.Password,
				InsecureSkipTLS: r.Helm.InsecureSkipTLS,
			})
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
		table.AddRow(addonName, app.GetLabels()[oam.LabelAddonRegistry], "", genAvailableVersionInfo([]string{addonVersion}, addonVersion, 3), statusEnabled)
		locallyInstalledAddons[addonName] = true
	}

	for _, addon := range addons {
		// if the addon with same name has already installed locally, display the registry one as not installed
		if locallyInstalledAddons[addon.Name] {
			table.AddRow(addon.Name, addon.RegistryName, limitStringLength(addon.Description, 60), genAvailableVersionInfo(addon.AvailableVersions, "", 3), "disabled")
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
		table.AddRow(addon.Name, addon.RegistryName, limitStringLength(addon.Description, 60), genAvailableVersionInfo(addon.AvailableVersions, status.InstalledVersion, 3), statusRow)
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
		err := k8sClient.Get(ctx, types2.NamespacedName{Name: addonutil.Addon2AppName(addonName), Namespace: types.DefaultKubeVelaNS}, &app)
		if err != nil {
			return client.IgnoreNotFound(err)
		}
		phase := app.Status.Phase
		switch app.Status.Phase {
		case common2.ApplicationRunning:
			return nil
		case common2.ApplicationWorkflowSuspending:
			fmt.Printf("Enabling suspend, please run \"vela workflow resume %s -n vela-system\" to continue", addonutil.Addon2AppName(addonName))
			return nil
		case common2.ApplicationWorkflowTerminated:
			return errors.Errorf("Enabling failed, please run \"vela status %s -n vela-system\" to check the status of the addon", addonutil.Addon2AppName(addonName))
		default:
		}
		timeConsumed := int(time.Since(start).Seconds())
		applySpinnerNewSuffix(spinner, fmt.Sprintf("Waiting addon application running. It is now in phase: %s (timeout %d/%d seconds)...",
			phase, timeConsumed, int(timeout.Seconds())))
		if timeConsumed > int(timeout.Seconds()) {
			return errors.Errorf("Enabling timeout, please run \"vela status %s -n vela-system\" to check the status of the addon", addonutil.Addon2AppName(addonName))
		}
		time.Sleep(trackInterval)
	}

}

// generate the available version
// this func put the installed version as the first version and keep the origin order
// print ... if available version too much
func genAvailableVersionInfo(versions []string, installedVersion string, limit int) string {
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
		if count == limit {
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

// limitStringLength limits the length of the string, and add ... if it is too long
func limitStringLength(str string, length int) string {
	if length <= 0 {
		return str
	}
	if len(str) > length {
		return str[:length] + "..."
	}
	return str
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
	if len(cstr) == 0 {
		return nil
	}
	cstr = strings.TrimPrefix(strings.TrimSuffix(cstr, "}"), "{")
	var clusterL []string
	clusterList := strings.Split(cstr, ",")
	for _, v := range clusterList {
		clusterL = append(clusterL, strings.TrimSpace(v))
	}
	return clusterL
}

// NewAddonPackageCommand create addon package command
func NewAddonPackageCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "package",
		Short:   "package an addon directory",
		Long:    "package an addon directory into a helm chart archive.",
		Example: "vela addon package <addon directory>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify addon directory path")
			}
			addonDict, err := filepath.Abs(args[0])
			if err != nil {
				return err
			}

			archive, err := pkgaddon.PackageAddon(addonDict)
			if err != nil {
				return errors.Wrapf(err, "fail to package %s into helm chart archive", addonDict)
			}

			fmt.Printf("Successfully package addon to: %s\n", archive)
			return nil
		},
	}
	return cmd
}
