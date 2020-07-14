package main

import (
	"fmt"
	"github.com/cloud-native-application/rudrx/pkg/cmd/traits"
	"math/rand"
	"os"
	"time"

	coreoamdevv1alpha2 "github.com/cloud-native-application/rudrx/api/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/cloud-native-application/rudrx/pkg/cmd/run"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/cloud-native-application/rudrx/pkg/utils/logs"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// noUsageError suppresses usage printing when it occurs
// (since cobra doesn't provide a good way to avoid printing
// out usage in only certain situations).
type noUsageError struct{ error }

var (
	scheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = coreoamdevv1alpha2.AddToScheme(scheme)

	_ = core.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	rand.Seed(time.Now().UnixNano())

	command := newCommand()

	logs.InitLogs()
	defer logs.FlushLogs()

	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}

func newCommand() *cobra.Command {
	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}

	cmds := &cobra.Command{
		Use:   "rudrx",
		Short: "rudrx is a command-line tool to use OAM based micro-app engine.",
		Long:  "rudrx is a command-line tool to use OAM based micro-app engine.",
		Run:   runHelp,
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
		run.NewCmdRun(f, client, ioStream),
		traits.NewCmdTraits(f, client, ioStream),
	)

	return cmds
}

func runHelp(cmd *cobra.Command, args []string) {
	cmd.Help()
}
