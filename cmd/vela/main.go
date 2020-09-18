package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"time"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/cmd/vela/fake"
	"github.com/oam-dev/kubevela/pkg/commands"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/utils/system"
	"github.com/oam-dev/kubevela/version"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/gosuri/uitable"
	certmanager "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha2"
	"github.com/spf13/cobra"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	scheme = k8sruntime.NewScheme()
)

// chartTGZSource is a base64-encoded, gzipped tarball of the default Helm chart.
// Its value is initialized at build time.
var chartTGZSource string

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = certmanager.AddToScheme(scheme)
	_ = core.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	rand.Seed(time.Now().UnixNano())

	command := newCommand()

	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
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
			PrintHelpByTag(cmd, allCommands, types.TypeTraits)
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
	cmds.PersistentFlags().StringP("env", "e", "", "specify env name for application")
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

	cmds.AddCommand(
		// Getting Start
		commands.NewInstallCommand(commandArgs, chartTGZSource, ioStream),
		commands.NewEnvCommand(commandArgs, ioStream),

		// Getting Start
		NewVersionCommand(),

		// Apps
		commands.NewAppsCommand(commandArgs, ioStream),

		// Workloads
		commands.AddCompCommands(commandArgs, ioStream),

		// Capability Systems
		commands.CapabilityCommandGroup(commandArgs, ioStream),

		// System
		commands.SystemCommandGroup(commandArgs, ioStream),
		commands.NewCompletionCommand(),

		commands.NewTraitsCommand(ioStream),
		commands.NewWorkloadsCommand(ioStream),

		commands.NewDashboardCommand(commandArgs, ioStream, fake.FrontendSource),

		commands.NewLogsCommand(commandArgs, ioStream),
	)

	// Traits
	if err = commands.AddTraitCommands(cmds, commandArgs, ioStream); err != nil {
		fmt.Println("Add trait commands from traitDefinition err", err)
		os.Exit(1)
	}

	// this is for mute klog
	fset := flag.NewFlagSet("logs", flag.ContinueOnError)
	klog.InitFlags(fset)
	_ = fset.Set("v", "-1")

	return cmds
}

func PrintHelpByTag(cmd *cobra.Command, all []*cobra.Command, tag string) {
	cmd.Printf("  %s:\n\n", tag)
	table := uitable.New()
	for _, c := range all {
		if val, ok := c.Annotations[types.TagCommandType]; ok && val == tag {
			table.AddRow("    "+c.Use, c.Long)
			for _, subcmd := range c.Commands() {
				table.AddRow("      "+subcmd.Use, "  "+subcmd.Long)
			}
		}
	}
	cmd.Println(table.String())
	if tag == types.TypeTraits {
		if len(table.Rows) > 0 {
			cmd.Println()
		}
		cmd.Println("    Want more? < install more capabilities by `vela cap` >")
	}
	cmd.Println()
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
