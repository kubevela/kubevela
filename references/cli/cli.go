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
	"strings"

	gov "github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
	"github.com/oam-dev/kubevela/pkg/utils/system"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/version"
)

var assumeYes bool

// NewCommand will contain all commands
func NewCommand() *cobra.Command {
	return NewCommandWithIOStreams(util.NewDefaultIOStreams())
}

// NewCommandWithIOStreams will contain all commands and initialize them with given ioStream
func NewCommandWithIOStreams(ioStream util.IOStreams) *cobra.Command {
	cmds := &cobra.Command{
		Use:                "vela",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			runHelp(cmd, cmd.Commands(), nil)
		},
		SilenceUsage: true,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			name := cmd.Name()
			cmd.VisitParents(func(command *cobra.Command) {
				name = fmt.Sprintf("%s.%s", command.Name(), name)
			})
			InitClients(strings.Split(name, "."))
		},
	}

	scheme := common.Scheme
	err := workflowv1alpha1.AddToScheme(scheme)
	if err != nil {
		klog.Fatal(err)
	}
	f := velacmd.NewDeferredFactory(ctrlconfig.GetConfig)

	if err := system.InitDirs(); err != nil {
		fmt.Println("InitDir err", err)
		os.Exit(1)
	}

	cmds.AddCommand(
		// Getting Start
		NewInitCommand("1", ioStream),
		NewUpCommand(f, "2", ioStream),
		NewAppStatusCommand("3", ioStream),
		NewListCommand("4", ioStream),
		NewDeleteCommand(f, "5"),
		NewEnvCommand("6", ioStream),
		NewCapabilityShowCommand("7", ioStream),

		// Manage Apps
		NewDryRunCommand("1", ioStream),
		NewLiveDiffCommand("2", ioStream),
		NewLogsCommand("3", ioStream),
		NewPortForwardCommand("4", ioStream),
		NewExecCommand("5", ioStream),
		RevisionCommandGroup("6"),
		NewDebugCommand("7", ioStream),

		// Continuous Delivery
		NewWorkflowCommand("1", ioStream),
		NewAdoptCommand(f, "2", ioStream),

		// Platform
		NewTopCommand("1"),
		ClusterCommandGroup(f, "2", ioStream),
		AuthCommandGroup(f, "3", ioStream),
		// Config management
		ConfigCommandGroup(f, "4", ioStream),
		TemplateCommandGroup(f, "5", ioStream),

		// Extension
		// Addon
		NewAddonCommand("1", ioStream),
		NewUISchemaCommand("2", ioStream),
		// Definitions
		NewComponentsCommand("3", ioStream),
		NewTraitCommand("4", ioStream),
		DefinitionCommandGroup("5", ioStream),

		// System
		NewInstallCommand("1", ioStream),
		NewUnInstallCommand("2", ioStream),
		NewSystemCommand("3"),
		NewVersionCommand(ioStream, "4"),

		// aux
		KubeCommandGroup(f, "1", ioStream),
		CueXCommandGroup(f, "2"),
		NewQlCommand("3", ioStream),
		NewCompletionCommand("4"),
		NewHelpCommand("5"),

		// hide (below commands will not be displayed in help command but still
		// can be used by direct call)
		NewWorkloadsCommand(ioStream),
		NewExportCommand(ioStream),
		NewRegistryCommand(ioStream, ""),
		NewProviderCommand("", ioStream),
	)

	fset := flag.NewFlagSet("logs", flag.ContinueOnError)
	klog.InitFlags(fset)

	// init global flags
	cmds.PersistentFlags().BoolVarP(&assumeYes, "yes", "y", false, "Assume yes for all user prompts")
	return cmds
}

// NewVersionCommand print cli version
func NewVersionCommand(ioStream util.IOStreams, order string) *cobra.Command {
	version := &cobra.Command{
		Use:   "version",
		Short: "Prints vela build version information.",
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
			types.TagCommandType:  types.TypeSystem,
			types.TagCommandOrder: order,
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
			versions, err := helmHelper.ListVersions(kubevelaInstallerHelmRepoURL, kubeVelaChartName, true, nil)
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
