package cmd

import (
	"context"
	"fmt"
	"strings"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewTraitsCommand(f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams, args []string) *cobra.Command {
	ctx := context.Background()
	var workloadName string
	cmd := &cobra.Command{
		Use:                   "traits [--apply-to WORKLOADNAME]",
		DisableFlagsInUseLine: true,
		Short:                 "List traits",
		Long:                  "List traits",
		Example:               `vela traits`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printTraitList(ctx, c, &workloadName, ioStreams)
		},
	}

	cmd.SetOut(ioStreams.Out)
	cmd.Flags().StringVar(&workloadName, "apply-to", "", "Workload name")
	return cmd
}

func printTraitList(ctx context.Context, c client.Client, workloadName *string, ioStreams cmdutil.IOStreams) error {
	traitList, err := RetrieveTraitsByWorkload(ctx, c, "", *workloadName)

	table := uitable.New()
	table.MaxColWidth = 60

	if err != nil {
		return fmt.Errorf("Listing Trait DefinitionPath hit an issue: %s", err)
	}

	table.AddRow("NAME", "ALIAS", "DEFINITION", "APPLIES TO", "STATUS")
	for _, r := range traitList {
		wdList := strings.Split(r.AppliesTo, ",")
		if len(wdList) > 1 {
			isFirst := true
			for _, wd := range wdList {
				wd = strings.Trim(wd, " ")
				if isFirst {
					table.AddRow(r.Name, r.Short, r.Definition, wd, r.Status)
					isFirst = false
				} else {
					table.AddRow("", "", "", wd, "")
				}
			}
		} else {
			table.AddRow(r.Name, r.Short, r.Definition, r.AppliesTo, r.Status)
		}
	}
	ioStreams.Info(table.String())

	return nil
}

type TraitMeta struct {
	Name       string `json:"name"`
	Short      string `json:"shot"`
	Definition string `json:"definition,omitempty"`
	AppliesTo  string `json:"appliesTo,omitempty"`
	Status     string `json:"status,omitempty"`
}

// RetrieveTraitsByWorkload Get trait list by optional filter `workloadName`
func RetrieveTraitsByWorkload(ctx context.Context, c client.Client, namespace string, workloadName string) ([]TraitMeta, error) {
	var traitList []TraitMeta
	var traitDefinitionList corev1alpha2.TraitDefinitionList
	if namespace == "" {
		namespace = "default"
	}
	err := c.List(ctx, &traitDefinitionList, client.InNamespace(namespace))

	for _, r := range traitDefinitionList.Items {
		var appliesTo string
		if workloadName == "" {
			appliesTo = strings.Join(r.Spec.AppliesToWorkloads, ", ")
			if appliesTo == "" {
				continue
			}
		} else {
			flag := false
			for _, w := range r.Spec.AppliesToWorkloads {
				if workloadName == w {
					flag = true
					break
				}
			}
			if !flag {
				continue
			}
			appliesTo = workloadName
		}

		// TODO(zzxwill) `Status` might not be proper as I'd like to describe where the trait is, in cluster or in registry
		traitList = append(traitList, TraitMeta{
			Name:       r.Name,
			Short:      r.ObjectMeta.Annotations["short"],
			Definition: r.Spec.Reference.Name,
			AppliesTo:  appliesTo,
			Status:     "-",
		})
	}

	return traitList, err
}
