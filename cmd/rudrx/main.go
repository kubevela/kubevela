package main

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"time"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloud-native-application/rudrx/pkg/cmd"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/cloud-native-application/rudrx/pkg/template"
	"github.com/cloud-native-application/rudrx/pkg/utils/logs"
)

// noUsageError suppresses usage printing when it occurs
// (since cobra doesn't provide a good way to avoid printing
// out usage in only certain situations).
type noUsageError struct{ error }

var (
	scheme = k8sruntime.NewScheme()

	// RudrxVersion is the version of cli.
	RudrxVersion = "UNKNOWN"

	// GitRevision is the commit of repo
	GitRevision = "UNKNOWN"
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
		Use:          "rudr",
		Short:        "rudr is a command-line tool to use OAM based micro-app engine.",
		Long:         "rudr is a command-line tool to use OAM based micro-app engine.",
		Run:          runHelp,
		SilenceUsage: true,
	}

	// flags of contorller-runtime
	globalFlags := pflag.NewFlagSet("global", pflag.ContinueOnError)

	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	kubeConfigFlags.AddFlags(globalFlags)
	f := cmdutil.NewFactory(kubeConfigFlags)
	restConf, err := f.ToRESTConfig()
	if err != nil {
		fmt.Println("get kubeconfig err", err)
		os.Exit(1)
	}
	client, err := client.New(restConf, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Println("create client from kubeconfig err", err)
		os.Exit(1)
	}

	// base usage commands
	baseCommands := []*cobra.Command{
		cmd.NewTraitsCommand(f, client, ioStream, []string{}),
		cmd.NewWorkloadsCommand(f, client, ioStream, os.Args[1:]),
		cmd.NewInitCommand(f, client, ioStream),
		cmd.NewDeleteCommand(f, client, ioStream, os.Args[1:]),
		cmd.NewAppsCommand(f, client, ioStream),
		cmd.NewEnvInitCommand(f, ioStream),
		cmd.NewEnvSwitchCommand(f, ioStream),
		cmd.NewEnvDeleteCommand(f, ioStream),
		cmd.NewEnvCommand(f, ioStream),
		cmd.NewAppStatusCommand(client, ioStream),

		NewVersionCommand(),
	}

	// advanced commands
	advancedCommands := []*cobra.Command{}

	// show global flags used by controller-runtime
	optionsCommand := cmd.NewOptionsCommand(globalFlags, ioStream)

	workloadPluginCommands, err := cmd.AddWorkloadPlugins(client, ioStream)
	if err != nil {
		fmt.Println("Add plugins from workloadDefinition err", err)
		os.Exit(1)
	}
	traitPluginCommands, err := cmd.AddTraitPlugins(client, ioStream)
	if err != nil {
		fmt.Println("Add plugins from traitDefinition err", err)
		os.Exit(1)
	}

	templater := template.NewTemplater(cmds, baseCommands, advancedCommands, workloadPluginCommands,
		traitPluginCommands, optionsCommand, globalFlags, ioStream)
	templater.AddCommandsAndFlags()

	cmds.SetUsageFunc(templater.UsageFunc())
	cmds.SetHelpFunc(templater.HelpFunc())

	return cmds
}

func runHelp(cmd *cobra.Command, args []string) {
	cmd.Help()
}

func NewVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Prints out build version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf(`Version: %v
GitRevision: %v
GolangVersion: %v
`,
				RudrxVersion,
				GitRevision,
				runtime.Version())
		},
	}
}
