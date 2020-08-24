package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloud-native-application/rudrx/api/types"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/cloud-native-application/rudrx/pkg/oam"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var appName string

func NewAppsCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "app",
		DisableFlagsInUseLine: true,
		Short:                 "Manage applications",
		Long:                  "Manage applications with ls, show, delete, run",
		Example:               `vela app <command>`,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}

	cmd.AddCommand(NewAppsListCommand(c, ioStreams),
		NewDeleteCommand(c, ioStreams),
		NewAppStatusCommand(c, ioStreams),
		NewAppShowCommand(c, ioStreams),
		NewRunCommand(c, ioStreams))
	return cmd
}

func NewAppsListCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "ls",
		DisableFlagsInUseLine: true,
		Short:                 "List applications",
		Long:                  "List applications with workloads, traits, status and created time",
		Example:               `vela app ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := GetEnv(cmd)
			if err != nil {
				return err
			}
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			printApplicationList(ctx, newClient, appName, env.Namespace, ioStreams)
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	cmd.Flags().StringVarP(&appName, "app", "a", "", "Application name")
	return cmd
}

func printApplicationList(ctx context.Context, c client.Client, appName string, namespace string, ioStreams cmdutil.IOStreams) {
	applicationMetaList, err := oam.RetrieveApplicationsByName(ctx, c, appName, namespace)
	if err != nil {
		ioStreams.Infof("listing Trait DefinitionPath hit an issue: %s\n", err)
		return
	}
	if applicationMetaList == nil {
		fmt.Printf("No application found in %s namespace.\n", namespace)
		return
	} else {
		table := uitable.New()
		table.MaxColWidth = 60
		table.AddRow("NAME", "WORKLOAD", "TRAITS", "STATUS", "CREATED-TIME")
		for _, a := range applicationMetaList {
			traitAlias := strings.Join(a.Traits, ",")
			table.AddRow(a.Name, a.Workload, traitAlias, a.Status, a.CreatedTime)
		}
		ioStreams.Info(table.String())
	}
}
