package main

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"math/rand"
	"os"
	"runtime"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/cloud-native-application/rudrx/pkg/utils/system"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/spf13/cobra"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

	// VelaVersion is the version of cli.
	VelaVersion = "UNKNOWN"

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
		Use:          "vela",
		Short:        "✈️  A Micro App Plafrom for Kubernetes.",
		Long:         "✈️  A Micro App Plafrom for Kubernetes.",
		Run:          runHelp,
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
	newClient, err := client.New(restConf, client.Options{Scheme: scheme})
	err = cmds.RegisterFlagCompletionFunc("namespace", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Choose a long enough timeout that the user notices somethings is not working
		// but short enough that the user is not made to wait very long
		to := int64(3)
		listOpt := &client.ListOptions{
			Raw: &metav1.ListOptions{TimeoutSeconds: &to},
		}
		nsNames := []string{}
		namespaces := v1.NamespaceList{}
		if err = newClient.List(context.Background(), &namespaces, listOpt); err == nil {
			for _, ns := range namespaces.Items {
				nsNames = append(nsNames, ns.Name)
			}
			return nsNames, cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveDefault
	})
	if err != nil {
		fmt.Println("create client from kubeconfig err", err)
		os.Exit(1)
	}
	if err := system.InitApplicationDir(); err != nil {
		fmt.Println("InitApplicationDir err", err)
		os.Exit(1)
	}
	if err := system.InitDefinitionDir(); err != nil {
		fmt.Println("InitDefinitionDir err", err)
		os.Exit(1)
	}

	cmds.AddCommand(
		cmd.NewTraitsCommand(newClient, ioStream, []string{}),
		cmd.NewWorkloadsCommand(newClient, ioStream, os.Args[1:]),
		cmd.NewAdminInitCommand(newClient, ioStream),
		cmd.NewAdminInfoCommand(VelaVersion, ioStream),
		cmd.NewDeleteCommand(newClient, ioStream, os.Args[1:]),
		cmd.NewAppsCommand(newClient, ioStream),
		cmd.NewEnvInitCommand(newClient, ioStream),
		cmd.NewEnvSwitchCommand(ioStream),
		cmd.NewEnvDeleteCommand(ioStream),
		cmd.NewEnvCommand(ioStream),
		NewVersionCommand(),
		cmd.NewAppStatusCommand(newClient, ioStream),
		cmd.NewAddonConfigCommand(ioStream),
		cmd.NewAddonListCommand(newClient, ioStream),
		cmd.NewCompletionCommand(),
	)
	if err = cmd.AddWorkloadPlugins(cmds, newClient, ioStream); err != nil {
		fmt.Println("Add plugins from workloadDefinition err", err)
		os.Exit(1)
	}
	if err = cmd.AddTraitPlugins(cmds, newClient, ioStream); err != nil {
		fmt.Println("Add plugins from traitDefinition err", err)
		os.Exit(1)
	}
	if err = cmd.DetachTraitPlugins(cmds, newClient, ioStream); err != nil {
		fmt.Println("Add plugins from traitDefinition err", err)
		os.Exit(1)
	}
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
				VelaVersion,
				GitRevision,
				runtime.Version())
		},
	}
}
