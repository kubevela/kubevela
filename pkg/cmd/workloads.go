package cmd

import (
	"context"
	"fmt"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewWorkloadsCommand(f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams, args []string) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "workloads",
		DisableFlagsInUseLine: true,
		Short:                 "List workloads",
		Long:                  "List workloads",
		Example:               `rudr workloads`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printWorkloadList(ctx, c, ioStreams)
		},
	}

	cmd.SetOut(ioStreams.Out)
	return cmd
}

func printWorkloadList(ctx context.Context, c client.Client, ioStreams cmdutil.IOStreams) error {
	workloadList, err := ListWorkloads(ctx, c)

	table := uitable.New()
	table.MaxColWidth = 60

	if err != nil {
		return fmt.Errorf("Listing Trait DefinitionPath hit an issue: %s", err)
	}

	table.AddRow("NAME", "SHORT", "DEFINITION")
	for _, r := range workloadList {
		table.AddRow(r.Name, r.Short, r.Definition)
	}
	ioStreams.Info(table.String())

	return nil
}

type WorkloadData struct {
	Name       string `json:"name"`
	Short      string `json:"shot"`
	Definition string `json:"definition,omitempty"`
}

func ListWorkloads(ctx context.Context, c client.Client) ([]WorkloadData, error) {
	var workloadList []WorkloadData
	var workloadDefinitionList corev1alpha2.WorkloadDefinitionList
	err := c.List(ctx, &workloadDefinitionList)

	for _, r := range workloadDefinitionList.Items {

		workloadList = append(workloadList, WorkloadData{
			Name:       r.Name,
			Short:      r.ObjectMeta.Annotations["short"],
			Definition: r.Spec.Reference.Name,
		})
	}

	return workloadList, err
}
