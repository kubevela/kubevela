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
	"fmt"
	"strings"

	"github.com/gosuri/uitable"
	tcv1beta1 "github.com/oam-dev/terraform-controller/api/v1beta1"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

const (

	// ProviderNamespace is the namespace of Terraform Cloud Provider
	ProviderNamespace = "default"
	// ProviderTerraformProviderNameArgument is the argument name of addon terraform Provider
	ProviderTerraformProviderNameArgument = "providerName"
)

// NewProviderCommand create `addon` command
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
		NewProviderListCommand(c),
		NewProviderAddCommand(c),
	)
	return cmd
}

// NewProviderListCommand create addon list command
func NewProviderListCommand(c common.Args) *cobra.Command {
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
			err = listProviders(context.Background(), k8sClient)
			if err != nil {
				return err
			}
			return nil
		},
	}
}

// NewProviderAddCommand create a Provider
func NewProviderAddCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add",
		Short:   "add a Terraform Cloud Provider",
		Long:    "add a Terraform Cloud Provider by creating a credential secret and a Terraform Controller Provider",
		Example: "vela Provider add <Provider-type>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify a Terraform Cloud Provider type")
			}
			return nil
		},
	}
	return cmd
}

type Provider struct {
	Type string
	Name string
	Age  metav1.Time
}

func listProviders(ctx context.Context, k8sClient client.Client) error {
	var labelVal = "terraform-provider"
	var providers []Provider
	tcProviders := &tcv1beta1.ProviderList{}
	if err := k8sClient.List(ctx, tcProviders, client.InNamespace(ProviderNamespace),
		client.MatchingLabels{"config.oam.dev/type": labelVal}); err != nil {
		if kerrors.IsNotFound(err) {
			defs := &v1beta1.ComponentDefinitionList{}
			if err := k8sClient.List(ctx, defs, client.InNamespace(types.DefaultKubeVelaNS),
				client.MatchingLabels{definition.UserPrefix + "type.config.oam.dev": labelVal}); err != nil {
				if kerrors.IsNotFound(err) {
					return errors.New("no Terraform Cloud Provider found, please run `vela addon enable` first")
				}
				return errors.Wrap(err, "failed to retrieve providers")
			}
			for _, d := range defs.Items {
				providers = append(providers, Provider{
					Type: d.Name,
				})
			}
		}
		return errors.Wrap(err, "failed to retrieve providers")
	}
	for _, p := range tcProviders.Items {
		providers = append(providers, Provider{
			Type: p.Labels[oam.WorkloadTypeLabel],
			Name: p.Name,
			Age:  p.CreationTimestamp,
		})
	}

	table := uitable.New()
	table.AddRow("TYPE", "NAME", "CREATED-TIME")

	for _, p := range providers {
		table.AddRow(p.Type, p.Name, p.Age)
	}
	fmt.Println(table.String())
	return nil
}

// TransProviderName will turn addon's name from xxx/yyy to xxx-yyy
func TransProviderName(name string) string {
	return strings.ReplaceAll(name, "/", "-")
}

func mergeProviders(a1, a2 []*pkgaddon.UIData) []*pkgaddon.UIData {
	for _, item := range a2 {
		if hasProvider(a1, item.Name) {
			continue
		}
		a1 = append(a1, item)
	}
	return a1
}

func hasProvider(addons []*pkgaddon.UIData, name string) bool {
	for _, addon := range addons {
		if addon.Name == name {
			return true
		}
	}
	return false
}
