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

	"github.com/spf13/cobra"
	"k8s.io/klog"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/system"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/a/preimport"
	"github.com/oam-dev/kubevela/version"
)

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
			PrintHelpByTag(cmd, allCommands, types.TypeCap)
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
	cmds.PersistentFlags().StringP("env", "e", "", "specify environment name for application")

	commandArgs := common.Args{
		Schema: common.Scheme,
	}

	if err := system.InitDirs(); err != nil {
		fmt.Println("InitDir err", err)
		os.Exit(1)
	}

	preimport.SuppressLogging()
	_, _ = commandArgs.GetClient()
	preimport.ResumeLogging()

	cmds.AddCommand(
		// Getting Start
		NewInitCommand(commandArgs, ioStream),
		NewUpCommand(commandArgs, ioStream),
		NewExportCommand(commandArgs, ioStream),
		NewCapabilityShowCommand(commandArgs, ioStream),

		// Apps
		NewListCommand(commandArgs, ioStream),
		NewDeleteCommand(commandArgs, ioStream),
		NewAppStatusCommand(commandArgs, ioStream),
		NewExecCommand(commandArgs, ioStream),
		NewPortForwardCommand(commandArgs, ioStream),
		NewLogsCommand(commandArgs, ioStream),
		NewEnvCommand(commandArgs, ioStream),

		// Workflows
		NewWorkflowCommand(commandArgs, ioStream),

		NewRegistryCommand(ioStream),
		NewTemplateCommand(ioStream),
		NewTraitCommand(commandArgs, ioStream),
		NewComponentsCommand(commandArgs, ioStream),
		NewWorkloadsCommand(commandArgs, ioStream),
		DefinitionCommandGroup(commandArgs),

		// Addons
		NewAddonCommand(commandArgs, ioStream),

		// live-diff
		NewLiveDiffCommand(commandArgs, ioStream),

		// dry-run
		NewDryRunCommand(commandArgs, ioStream),

		// cue-packages
		NewCUEPackageCommand(commandArgs, ioStream),

		// cluster
		ClusterCommandGroup(commandArgs),

		// Helper
		SystemCommandGroup(commandArgs, ioStream),
		NewCompletionCommand(),
		NewVersionCommand(),
		NewHelpCommand(),
	)

	// this is for mute klog
	fset := flag.NewFlagSet("logs", flag.ContinueOnError)
	klog.InitFlags(fset)
	_ = fset.Set("v", "-1")

	return cmds
}

// NewVersionCommand print client version
func NewVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Prints out build version information",
		Long:  "Prints out build version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf(`Version: %v
GitRevision: %v
GolangVersion: %v
`,
				version.VelaVersion,
				version.GitRevision,
				runtime.Version())
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
}
