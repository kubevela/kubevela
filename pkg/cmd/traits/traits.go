package traits

import (
	"context"
	"fmt"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

func NewCmdTraits(f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "traits [-workload WORKLOADNAME]",
		DisableFlagsInUseLine: true,
		Short:                 "List traits",
		Long:                  "List traits",
		Example:               `rudr traits`,
		Run: func(cmd *cobra.Command, args []string) {
			workloadName := cmd.Flag("workload").Value.String()
			retrieveTraits(ctx, c, workloadName)
		},
	}

	cmd.PersistentFlags().StringP("workload", "w", "", "Workload name")

	return cmd
}

func retrieveTraits(ctx context.Context, c client.Client, workloadName string) {
	table := uitable.New()
	table.MaxColWidth = 60
	// TODO(zzxwill) `STATUS` might not be proper as I'd like to describe where the trait is, in cluster or in registry
	table.AddRow("Name", "DEFINITION", "APPLIES TO", "STATUS")

	var traitDefinitionList corev1alpha2.TraitDefinitionList
	err := c.List(ctx, &traitDefinitionList)
	if err != nil {
		fmt.Println("Listing Trait Definition hit an issue: ", err)
		os.Exit(1)
	}

	for _, r := range traitDefinitionList.Items {
		applied2Workloads := r.Spec.AppliesToWorkloads
		if workloadName == "" {
			table.AddRow(r.Name, r.Spec.Reference.Name, strings.Join(applied2Workloads, ", "), "-")
		} else {
			flag := false
			for _, w := range applied2Workloads {
				if workloadName == w {
					flag = true
					break
				}
			}
			if flag == true {
				table.AddRow(r.Name, r.Spec.Reference.Name, workloadName, "-")
			}
		}
	}

	fmt.Println(table)
}
