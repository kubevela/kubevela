package main

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"time"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/spf13/cobra"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	coreoamdevv1alpha2 "github.com/cloud-native-application/rudrx/api/v1alpha2"
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

	// RudrxVersion is the version of cli.
	RudrxVersion = "UNKNOWN"

	// GitRevision is the commit of repo
	GitRevision = "UNKNOWN"
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = coreoamdevv1alpha2.AddToScheme(scheme)

	_ = core.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	rand.Seed(time.Now().UnixNano())

	command := newCommand(os.Args[1:])

	logs.InitLogs()
	defer logs.FlushLogs()

	command.Execute()
}

func newCommand(args []string) *cobra.Command {
	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}

	cmds := &cobra.Command{
		Use:          "rudrx",
		Short:        "rudrx is a command-line tool to use OAM based micro-app engine.",
		Long:         "rudrx is a command-line tool to use OAM based micro-app engine.",
		Run:          runHelp,
		SilenceUsage: true,
	}

	flags := cmds.PersistentFlags()
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	kubeConfigFlags.AddFlags(flags)
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

	cmds.AddCommand(
		cmd.NewRunCommand(f, client, ioStream, args),
		cmd.NewTraitsCommand(f, client, ioStream, args),
		cmd.NewBindCommand(f, client, ioStream, args),
		cmd.NewInitCommand(f, client, ioStream),
		cmd.NewAppsCommand(f, client, ioStream),
		NewVersionCommand(),
	)

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
