package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewTraitsCommand(f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams, args []string) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "traits [-workload WORKLOADNAME]",
		DisableFlagsInUseLine: true,
		Short:                 "List traits",
		Long:                  "List traits",
		Example:               `rudr traits`,
		Run: func(cmd *cobra.Command, args []string) {
			workloadName := cmd.Flag("workload").Value.String()
			printTraitList(ctx, c, workloadName)
		},
	}

	cmd.PersistentFlags().StringP("workload", "w", "", "Workload name")

	return cmd
}

func printTraitList(ctx context.Context, c client.Client, workloadName string) {
	traitList, err := RetrieveTraitsByWorkload(ctx, c, workloadName)

	table := uitable.New()
	table.MaxColWidth = 60

	if err != nil {
		fmt.Println("Listing Trait Definition hit an issue: ", err)
		os.Exit(1)
	}

	table.AddRow("NAME", "SHORT", "DEFINITION", "APPLIES TO", "STATUS")
	for _, r := range traitList {
		table.AddRow(r.Name, r.Short, r.Definition, r.AppliesTo, r.Status)
	}

	fmt.Println(table)
}

type TraitMeta struct {
	Name       string `json:"name"`
	Short      string `json:"shot"`
	Definition string `json:"definition,omitempty"`
	AppliesTo  string `json:"appliesTo,omitempty"`
	Status     string `json:"status,omitempty"`
}

func RetrieveTraitsByWorkload(ctx context.Context, c client.Client, workloadName string) ([]TraitMeta, error) {
	/*
		Get trait list by optional filter `workloadName`
	*/
	var traitList []TraitMeta

	var traitDefinitionList corev1alpha2.TraitDefinitionList
	err := c.List(ctx, &traitDefinitionList)

	for _, r := range traitDefinitionList.Items {
		var appliesTo string
		if workloadName == "" {
			appliesTo = strings.Join(r.Spec.AppliesToWorkloads, ", ")
		} else {
			flag := false
			for _, w := range r.Spec.AppliesToWorkloads {
				if workloadName == w {
					flag = true
					break
				}
			}
			if flag == true {
				appliesTo = workloadName
			}
		}

		if appliesTo != "" {
			// TODO(zzxwill) `Status` might not be proper as I'd like to describe where the trait is, in cluster or in registry
			traitList = append(traitList, TraitMeta{
				Name:       r.Name,
				Short:      r.ObjectMeta.Annotations["short"],
				Definition: r.Spec.Reference.Name,
				AppliesTo:  appliesTo,
				Status:     "-",
			})
		}
	}

	return traitList, err
}
