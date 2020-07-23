package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewAppsCommand(f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "apps",
		DisableFlagsInUseLine: true,
		Short:                 "List apps",
		Long:                  "List apps",
		Example:               `rudr apps`,
		Run: func(cmd *cobra.Command, args []string) {
			workloadName := cmd.Flag("application").Value.String()
			printApplicationList(ctx, c, workloadName)
		},
	}

	cmd.PersistentFlags().StringP("application", "a", "", "Application name")
	return cmd
}

func printApplicationList(ctx context.Context, c client.Client, appName string) {
	applicationMetaList, err := RetrieveApplicationsByApplicationName(ctx, c, appName)

	table := uitable.New()
	table.MaxColWidth = 60

	if err != nil {
		fmt.Errorf("listing Trait Definition hit an issue: %s", err)
	}

	table.AddRow("NAME", "WORKLOAD", "TRAITS", "STATUS", "CREATE-TIME")
	for _, a := range applicationMetaList {
		traitNames := strings.Join(a.Traits, ",")
		table.AddRow(a.Name, a.Workload, traitNames, a.Status, a.CreatedTime)
	}
	fmt.Print(table.String())
}

type ApplicationMeta struct {
	Name        string   `json:"name"`
	Workload    string   `json:"workload,omitempty"`
	Traits      []string `json:"traits,omitempty"`
	Status      string   `json:"status,omitempty"`
	CreatedTime string   `json:"created,omitempty"`
}

/*
	Get application list by optional filter `applicationName`
	Application name is equal to Component name as currently rudrx only supports one component exists in one application
*/
func RetrieveApplicationsByApplicationName(ctx context.Context, c client.Client, workloadName string) ([]ApplicationMeta, error) {
	var applicationMetaList []ApplicationMeta
	namespace := "default"

	var applicationList corev1alpha2.ApplicationConfigurationList

	if workloadName != "" {
		var application corev1alpha2.ApplicationConfiguration
		err := c.Get(ctx, client.ObjectKey{Name: workloadName, Namespace: namespace}, &application)

		if err != nil {
			return applicationMetaList, err
		}

		applicationList.Items = append(applicationList.Items, application)
	} else {
		err := c.List(ctx, &applicationList)
		if err != nil {
			return applicationMetaList, err
		}
	}

	for _, a := range applicationList.Items {
		componentName := a.Spec.Components[0].ComponentName

		var component corev1alpha2.Component
		err := c.Get(ctx, client.ObjectKey{Name: componentName, Namespace: namespace}, &component)
		if err != nil {
			return applicationMetaList, err
		}

		var workload corev1alpha2.WorkloadDefinition
		json.Unmarshal(component.Spec.Workload.Raw, &workload)
		workloadName := workload.TypeMeta.Kind

		var traitNames []string
		for _, t := range a.Spec.Components[0].Traits {
			var trait corev1alpha2.TraitDefinition
			json.Unmarshal(t.Trait.Raw, &trait)
			traitNames = append(traitNames, trait.Kind)
		}

		applicationMetaList = append(applicationMetaList, ApplicationMeta{
			Name:        a.Name,
			Workload:    workloadName,
			Traits:      traitNames,
			Status:      string(a.Status.Conditions[0].Status),
			CreatedTime: a.ObjectMeta.CreationTimestamp.String(),
		})
	}

	return applicationMetaList, nil
}
