/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cli

import (
	"context"

	"github.com/gosuri/uitable"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta1"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// NewProviderCommand create `addon` command
// +Deprecated
func NewProviderCommand(c common.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Authenticate Terraform Cloud Providers",
		Long:  "Authenticate Terraform Cloud Providers by managing Terraform Controller Providers with its credential secret",
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeExtension,
		},
	}
	cmd.AddCommand(
		NewProviderListCommand(c, ioStreams),
	)
	cmd.AddCommand(prepareProviderAddCommand())
	cmd.AddCommand(prepareProviderDeleteCommand())
	return cmd
}

// NewProviderListCommand create addon list command
func NewProviderListCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List Terraform Cloud Providers",
		Long:    "List Terraform Cloud Providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			k8sClient, err := c.GetClient()
			if err != nil {
				return err
			}
			err = listProviders(context.Background(), k8sClient, ioStreams)
			if err != nil {
				return err
			}
			return nil
		},
	}
}

func prepareProviderAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:        "add",
		Deprecated: "Please use the vela config command: \n vela config create <name> --template <provider-type> [Properties]",
	}
	return cmd
}

func listProviders(ctx context.Context, k8sClient client.Client, ioStreams cmdutil.IOStreams) error {
	l := &terraformapi.ProviderList{}
	if err := k8sClient.List(ctx, l, client.InNamespace(types.ProviderNamespace)); err != nil {
		return err
	}

	table := uitable.New()
	table.AddRow("TYPE", "PROVIDER", "NAME", "REGION", "CREATED-TIME")

	for _, p := range l.Items {
		table.AddRow(p.Labels["config.oam.dev/provider"], p.Spec.Provider, p.Name, p.Spec.Region, p.CreationTimestamp)
	}
	ioStreams.Info(table.String())
	return nil
}

func prepareProviderDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:        "delete",
		Aliases:    []string{"rm", "del"},
		Deprecated: "Please use the vela config command: \n  vela config delete <provider-name>",
	}
	return cmd
}
