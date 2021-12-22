/*
 Copyright 2021. The KubeVela Authors.

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
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/cli"
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
			cmd.Printf("A Highly Extensible Platform Engine based on Kubernetes and Open Application BaseModel.\n\nUsage:\n  kubectl vela [flags]\n  kubectl vela [command]\n\nAvailable Commands:\n\n")
			cli.PrintHelpByTag(cmd, allCommands, types.TypePlugin)
			cmd.Println("Flags:")
			cmd.Println("  -h, --help   help for vela")
			cmd.Println()
			cmd.Println(`Use "kubectl vela [command] --help" for more information about a command.`)
		},
		SilenceUsage: true,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
	}

	commandArgs := common.Args{
		Schema: common.Scheme,
	}

	cmds.AddCommand(
		NewDryRunCommand(commandArgs, ioStream),
		NewLiveDiffCommand(commandArgs, ioStream),
		NewCapabilityShowCommand(commandArgs, ioStream),
		cli.NewComponentsCommand(commandArgs, ioStream),
		cli.NewTraitCommand(commandArgs, ioStream),
		cli.NewRegistryCommand(ioStream),
		NewVersionCommand(),
		NewHelpCommand(),
	)

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
			types.TagCommandType: types.TypePlugin,
		},
	}
}

// NewHelpCommand get any command help
func NewHelpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "kubectl vela help [command] ",
		DisableFlagsInUseLine: true,
		Short:                 "Help about any command",
		Run:                   RunHelp,
	}
	return cmd
}

// RunHelp exec help [command]
func RunHelp(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		allCommands := cmd.Root().Commands()
		// print error message at first, since it can contain suggestions
		cmd.Printf("A Highly Extensible Platform Engine based on Kubernetes and Open Application BaseModel.\n\nUsage:\n  kubectl vela [flags]\n  kubectl vela [command]\n\nAvailable Commands:\n\n")
		cli.PrintHelpByTag(cmd, allCommands, types.TypePlugin)
	} else {
		foundCmd, _, err := cmd.Root().Find(args)
		if foundCmd != nil && err == nil {
			foundCmd.HelpFunc()(cmd, args)
		}
	}
}
