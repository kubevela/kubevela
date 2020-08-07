package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloud-native-application/rudrx/api/types"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewAppsCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "app:ls",
		DisableFlagsInUseLine: true,
		Short:                 "List applications",
		Long:                  "List applications with workloads, traits, status and created time",
		Example:               `vela ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := GetEnv()
			if err != nil {
				return err
			}
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			printApplicationList(ctx, newClient, "", env.Namespace)
			return nil
		},
	}

	cmd.PersistentFlags().StringP("app", "a", "", "Application name")
	return cmd
}

func printApplicationList(ctx context.Context, c client.Client, appName string, namespace string) {
	applicationMetaList, err := cmdutil.RetrieveApplicationsByName(ctx, c, appName, namespace)

	table := uitable.New()
	table.MaxColWidth = 60

	if err != nil {
		fmt.Printf("listing Trait DefinitionPath hit an issue: %s\n", err)
		return
	}

	table.AddRow("NAME", "WORKLOAD", "TRAITS", "STATUS", "CREATED-TIME")
	for _, a := range applicationMetaList {
		traitNames := strings.Join(a.Traits, ",")
		table.AddRow(a.Name, a.Workload, traitNames, a.Status, a.CreatedTime)
	}
	fmt.Print(table.String())
}
