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
	"flag"
	"fmt"
	"os"
	"runtime"

	gov "github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/klog"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
	"github.com/oam-dev/kubevela/pkg/utils/system"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/version"
)

var assumeYes bool

// NewCommand will contain all commands
func NewCommand() *cobra.Command {
	ioStream := util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}

	cmds := &cobra.Command{
		Use:                "vela",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			allCommands := cmd.Commands()
			cmd.Printf("A Highly Extensible Platform Engine based on Kubernetes and Open Application Model.\n\nUsage:\n  vela [flags]\n  vela [command]\n\nAvailable Commands:\n\n")
			PrintHelpByTag(cmd, allCommands, types.TypeStart)
			PrintHelpByTag(cmd, allCommands, types.TypeApp)
			PrintHelpByTag(cmd, allCommands, types.TypeCD)
			PrintHelpByTag(cmd, allCommands, types.TypeExtension)
			PrintHelpByTag(cmd, allCommands, types.TypeSystem)
			cmd.Println("Flags:")
			cmd.Println("  -h, --help   help for vela")
			cmd.Println()
			cmd.Println(`Use "vela [command] --help" for more information about a command.`)
		},
		SilenceUsage: true,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
	}

	commandArgs := common.Args{
		Schema: common.Scheme,
	}

	if err := system.InitDirs(); err != nil {
		fmt.Println("InitDir err", err)
		os.Exit(1)
	}

	cmds.AddCommand(
		// Getting Start
		NewEnvCommand(commandArgs, "3", ioStream),
		NewInitCommand(commandArgs, "2", ioStream),
		NewUpCommand(commandArgs, "1", ioStream),
		NewCapabilityShowCommand(commandArgs, ioStream),

		// Manage Apps
		NewListCommand(commandArgs, "9", ioStream),
		NewAppStatusCommand(commandArgs, "8", ioStream),
		NewDeleteCommand(commandArgs, "7", ioStream),
		NewExecCommand(commandArgs, "6", ioStream),
		NewPortForwardCommand(commandArgs, "5", ioStream),
		NewLogsCommand(commandArgs, "4", ioStream),
		NewLiveDiffCommand(commandArgs, "3", ioStream),
		NewDryRunCommand(commandArgs, ioStream),
		NewDiffCommand(commandArgs),

		// Workflows
		NewWorkflowCommand(commandArgs, ioStream),
		ClusterCommandGroup(commandArgs, ioStream),

		// Extension
		NewAddonCommand(commandArgs, "9", ioStream),
		NewUISchemaCommand(commandArgs, "8", ioStream),
		DefinitionCommandGroup(commandArgs, "7"),
		NewRegistryCommand(ioStream, "6"),
		NewTraitCommand(commandArgs, ioStream),
		NewComponentsCommand(commandArgs, ioStream),

		// System
		NewInstallCommand(commandArgs, "1", ioStream),
		NewUnInstallCommand(commandArgs, "2", ioStream),
		NewExportCommand(commandArgs, ioStream),
		NewCUEPackageCommand(commandArgs, ioStream),
		NewVersionCommand(ioStream),
		NewCompletionCommand(),

		// helper
		NewHelpCommand(),

		// hide
		NewTemplateCommand(ioStream),
		NewWorkloadsCommand(commandArgs, ioStream),
	)

	// this is for mute klog
	fset := flag.NewFlagSet("logs", flag.ContinueOnError)
	klog.InitFlags(fset)
	_ = fset.Set("v", "-1")

	// init global flags
	cmds.PersistentFlags().BoolVarP(&assumeYes, "yes", "y", false, "Assume yes for all user prompts")
	return cmds
}

// NewVersionCommand print client version
func NewVersionCommand(ioStream util.IOStreams) *cobra.Command {
	version := &cobra.Command{
		Use:   "version",
		Short: "Prints vela build version information",
		Long:  "Prints vela build version information.",
		Run: func(cmd *cobra.Command, args []string) {
			clusterVersion, _ := GetOAMReleaseVersion(types.DefaultKubeVelaNS)
			fmt.Printf(`CLI Version: %v
Core Version: %s
GitRevision: %v
GolangVersion: %v
`,
				version.VelaVersion,
				clusterVersion,
				version.GitRevision,
				runtime.Version())
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	version.AddCommand(NewVersionListCommand(ioStream))
	return version
}

// NewVersionListCommand show all versions command
func NewVersionListCommand(ioStream util.IOStreams) *cobra.Command {
	var showAll bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all available versions",
		Long:  "Query all available versions from remote server.",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			helmHelper := helm.NewHelper()
			versions, err := helmHelper.ListVersions(kubevelaInstallerHelmRepoURL, kubeVelaChartName)
			if err != nil {
				return err
			}
			clusterVersion, err := GetOAMReleaseVersion(types.DefaultKubeVelaNS)
			if err != nil {
				clusterVersion = version.VelaVersion
			}
			currentV, err := gov.NewVersion(clusterVersion)
			if err != nil && !showAll {
				return fmt.Errorf("can not parse current version %s", clusterVersion)
			}
			for _, chartV := range versions {
				if chartV != nil {
					v, err := gov.NewVersion(chartV.Version)
					if err != nil {
						continue
					}
					if v.GreaterThan(currentV) {
						ioStream.Info("Newer Version:", v.String())
					} else if showAll {
						ioStream.Info("Older Version:", v.String())
					}
				}
			}
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	cmd.PersistentFlags().BoolVarP(&showAll, "all", "a", false, "List all available versions, if not, only list newer version")
	return cmd
}

// GetOAMReleaseVersion gets version of vela-core runtime helm release
func GetOAMReleaseVersion(ns string) (string, error) {
	results, err := helm.GetHelmRelease(ns)
	if err != nil {
		return "", err
	}

	for _, result := range results {
		if result.Chart.ChartFullPath() == types.DefaultKubeVelaChartName {
			return result.Chart.AppVersion(), nil
		}
	}
	return "", errors.New("kubevela chart not found in your kubernetes cluster,  refer to 'https://kubevela.io/docs/install' for installation")
}
