package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/cloud-native-application/rudrx/version"

	"github.com/gosuri/uitable"

	"k8s.io/klog"

	"github.com/cloud-native-application/rudrx/api/types"

	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/cloud-native-application/rudrx/pkg/utils/system"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/spf13/cobra"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/cloud-native-application/rudrx/pkg/cmd"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/cloud-native-application/rudrx/pkg/utils/logs"
)

// noUsageError suppresses usage printing when it occurs
// (since cobra doesn't provide a good way to avoid printing
// out usage in only certain situations).
type noUsageError struct{ error }

var (
	scheme = k8sruntime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = core.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	rand.Seed(time.Now().UnixNano())

	command := newCommand()
	logs.InitLogs()
	defer logs.FlushLogs()

	command.Execute()
}

func newCommand() *cobra.Command {
	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}

	cmds := &cobra.Command{
		Use:                "vela",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			allCommands := cmd.Commands()
			cmd.Printf("✈️  A Micro App Platform for Kubernetes.\n\nUsage:\n  vela [flags]\n  vela [command]\n\nAvailable Commands:\n\n")
			PrintHelpByTag(cmd, allCommands, types.TypeStart)
			PrintHelpByTag(cmd, allCommands, types.TypeApp)
			PrintHelpByTag(cmd, allCommands, types.TypeWorkloads)
			PrintHelpByTag(cmd, allCommands, types.TypeTraits)
			PrintHelpByTag(cmd, allCommands, types.TypeRelease)
			PrintHelpByTag(cmd, allCommands, types.TypeOthers)
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
	restConf, err := config.GetConfig()
	if err != nil {
		fmt.Println("get kubeconfig err", err)
		os.Exit(1)
	}

	commandArgs := types.Args{
		Config: restConf,
		Schema: scheme,
	}

	if err := system.InitDirs(); err != nil {
		fmt.Println("InitDir err", err)
		os.Exit(1)
	}

	// Getting Start
	cmd.EnvCommandGroup(cmds, commandArgs, ioStream)
	// Others
	cmd.AddonCommandGroup(cmds, commandArgs, ioStream)
	// System
	cmd.SystemCommandGroup(cmds, commandArgs, ioStream)

	cmds.AddCommand(
		// Getting Start
		NewVersionCommand(),

		// Apps
		cmd.NewAppsCommand(commandArgs, ioStream),
		cmd.NewDeleteCommand(commandArgs, ioStream, os.Args[1:]),
		cmd.NewAppStatusCommand(commandArgs, ioStream),
		cmd.NewAppShowCommand(commandArgs, ioStream),

		// System
		cmd.NewRefreshCommand(commandArgs, ioStream),
		cmd.NewCompletionCommand(),

		cmd.NewTraitsCommand(ioStream),
		cmd.NewWorkloadsCommand(ioStream),
	)

	// Workloads
	if err = cmd.AddWorkloadPlugins(cmds, commandArgs, ioStream); err != nil {
		fmt.Println("Add plugins from workloadDefinition err", err)
		os.Exit(1)
	}

	// Traits
	if err = cmd.AddTraitPlugins(cmds, commandArgs, ioStream); err != nil {
		fmt.Println("Add plugins from traitDefinition err", err)
		os.Exit(1)
	}
	if err = cmd.DetachTraitPlugins(cmds, commandArgs, ioStream); err != nil {
		fmt.Println("Add plugins from traitDefinition err", err)
		os.Exit(1)
	}
	// this is for mute klog
	fset := flag.NewFlagSet("logs", flag.ContinueOnError)
	klog.InitFlags(fset)
	fset.Set("v", "-1")
	return cmds
}

func PrintHelpByTag(cmd *cobra.Command, all []*cobra.Command, tag string) {
	cmd.Printf("  %s:\n", tag)
	table := uitable.New()
	for _, c := range all {
		useline := strings.TrimPrefix(c.UseLine(), "vela ")
		if val, ok := c.Annotations[types.TagCommandType]; ok && val == tag {
			table.AddRow("    "+useline, c.Long)
		}
	}
	cmd.Println(table.String())
	cmd.Println()
}

func runHelp(cmd *cobra.Command, args []string) {
	cmd.Help()
}

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
			types.TagCommandType: types.TypeStart,
		},
	}
}
