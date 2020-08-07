package cmd

import (
	"context"
	"os"

	"github.com/cloud-native-application/rudrx/api/types"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"

	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewAppStatusCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:     "app:status",
		Short:   "get status of an application",
		Long:    "get status of an application, including its workload and trait",
		Example: `vela status <APPLICATION-NAME>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsLength := len(args)
			if argsLength == 0 {
				ioStreams.Errorf("Hint: please specify an application")
				os.Exit(1)
			}
			appName := args[0]
			env, err := GetEnv()
			if err != nil {
				ioStreams.Errorf("Error: failed to get Env: %s", err)
				return err
			}
			namespace := env.Namespace
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			return printApplicationStatus(ctx, newClient, ioStreams, appName, namespace)
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func printApplicationStatus(ctx context.Context, c client.Client, ioStreams cmdutil.IOStreams, appName string, namespace string) error {
	application, err := cmdutil.RetrieveApplicationStatusByName(ctx, c, appName, namespace)
	if err != nil {
		return err
	}
	// TODO(zzxwill) When application.Trait.Name is "", find a way not to print trait status
	out, err := yaml.Marshal(application)
	if err != nil {
		return err
	}
	ioStreams.Info(string(out))
	return nil
}
