package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/gosuri/uitable"

	"github.com/cloud-native-application/rudrx/api/types"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ApplicationMeta struct {
	Name        string   `json:"name"`
	Workload    string   `json:"workload,omitempty"`
	Traits      []string `json:"traits,omitempty"`
	Status      string   `json:"status,omitempty"`
	CreatedTime string   `json:"created,omitempty"`
}

func NewAppsCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "app:ls",
		DisableFlagsInUseLine: true,
		Short:                 "List applications",
		Long:                  "List applications with workloads, traits, status and created time",
		Example:               `vela app:ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := GetEnv()
			if err != nil {
				return err
			}
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			printApplicationList(ctx, newClient, ioStreams, "", env.Namespace)
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}

	return cmd
}

func printApplicationList(ctx context.Context, c client.Client, ioStreams cmdutil.IOStreams, appName string, namespace string) {
	applicationMetaList, err := RetrieveApplicationsByName(ctx, c, appName, namespace)
	if err != nil {
		fmt.Printf("listing Trait DefinitionPath hit an issue: %s\n", err)
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
		os.Exit(1)
	}
}

/*
	Get application list by optional filter `applicationName`
	Application name is equal to Component name as currently vela only supports one component exists in one application
*/
func RetrieveApplicationsByName(ctx context.Context, c client.Client, applicationName string, namespace string) ([]ApplicationMeta, error) {
	var applicationMetaList []ApplicationMeta
	var applicationList corev1alpha2.ApplicationConfigurationList

	if applicationName != "" {
		var application corev1alpha2.ApplicationConfiguration
		err := c.Get(ctx, client.ObjectKey{Name: applicationName, Namespace: namespace}, &application)

		if err != nil {
			return applicationMetaList, err
		}

		applicationList.Items = append(applicationList.Items, application)
	} else {
		err := c.List(ctx, &applicationList, &client.ListOptions{Namespace: namespace})
		if err != nil {
			return applicationMetaList, err
		}
	}

	for _, a := range applicationList.Items {
		for _, com := range a.Spec.Components {
			componentName := com.ComponentName
			component, err := cmdutil.GetComponent(ctx, c, componentName, namespace)
			if err != nil {
				return applicationMetaList, err
			}
			_, _, k := GetGVKFromRawExtension(component.Spec.Workload)

			workloadAlias, err := cmdutil.GetWorkloadDefinitionAliasByKind(ctx, c, k)
			if err != nil {
				return applicationMetaList, err
			}

			traitAlias := GetTraitAliasByComponentTraitList(ctx, c, com.Traits)
			var status = "UNKNOWN"
			if len(a.Status.Conditions) != 0 {
				status = string(a.Status.Conditions[0].Status)
			}
			applicationMetaList = append(applicationMetaList, ApplicationMeta{
				Name:        a.Name,
				Workload:    workloadAlias,
				Traits:      traitAlias,
				Status:      status,
				CreatedTime: a.ObjectMeta.CreationTimestamp.String(),
			})

		}
	}
	return applicationMetaList, nil
}
