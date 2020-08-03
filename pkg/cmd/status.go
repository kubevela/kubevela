package cmd

import (
	"context"
	"fmt"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"

	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewAppStatusCommand(f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:     "status",
		Short:   "get status of an application",
		Long:    "get status of an application, including its workload and trait",
		Example: `rudr status <APPLICATION-NAME>`,
		Run: func(cmd *cobra.Command, args []string) {
			argsLength := len(args)
			if argsLength == 0 {
				cmdutil.PrintErrorMessage("Hint: please specify an application", 1)
			} else {
				appName := args[0]
				if env, err := GetEnv(); err != nil {
					errMsg := fmt.Sprintf("Error: retrieving namespace from env hit an issue:%s", err)
					cmdutil.PrintErrorMessage(errMsg, 1)
				} else {
					namespace := env.Namespace
					printApplicationStatus(ctx, c, appName, namespace)
				}
			}
		},
	}
	return cmd
}

func printApplicationStatus(ctx context.Context, c client.Client, appName string, namespace string) {
	application := cmdutil.RetrieveApplicationStatusByName(ctx, c, appName, namespace)

	// TODO(zzxwill) When application.Trait.Name is "", find a way not to print trait status
	out, err := yaml.Marshal(application)

	if err != nil {
		errMsg := fmt.Sprintf("Error: priting workload status hit an issue:%s", err)
		cmdutil.PrintErrorMessage(errMsg, 1)
	}
	fmt.Print(string(out))
}
