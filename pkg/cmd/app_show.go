package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/cloud-native-application/rudrx/api/types"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewAppShowCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:     "app:show",
		Short:   "get detail spec of your app",
		Long:    "get detail spec of your app, including its workload and trait",
		Example: `vela app:show <APPLICATION-NAME>`,
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

			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}

			return printApplication(ctx, newClient, cmd, env, appName)
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func printApplication(ctx context.Context, c client.Client, cmd *cobra.Command, env *types.EnvMeta, appName string) error {
	var application corev1alpha2.ApplicationConfiguration

	if err := c.Get(ctx, client.ObjectKey{Name: appName, Namespace: env.Namespace}, &application); err != nil {
		return fmt.Errorf("Fetch application with Err: %s", err)
	}

	workload, err := cmdutil.GetWorkloadDefinitionByName(context.TODO(), c, env.Namespace, appName)
	if err != nil {
		return fmt.Errorf("Fetch WorkloadDefinitionByName with Err: %s", err)
	}

	traitDefinitions := cmdutil.ListTraitDefinitionsByApplicationConfiguration(application)

	cmd.Println("About:")
	cmd.Println(workload.Name)
	cmd.Println(len(traitDefinitions))
	return nil
}
