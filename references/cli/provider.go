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
	"encoding/json"
	"fmt"
	"github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/references/plugins"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"strings"

	"github.com/gosuri/uitable"
	tcv1beta1 "github.com/oam-dev/terraform-controller/api/v1beta1"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	coreapi "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

const (

	// ProviderNamespace is the namespace of Terraform Cloud Provider
	ProviderNamespace = "default"
	// ProviderTerraformProviderNameArgument is the argument name of addon terraform Provider
	ProviderTerraformProviderNameArgument = "providerName"

	labelVal = "terraform-provider"
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
	subCMDs, err := prepareProviderAddCommand(c)
	if err == nil {
		cmd.AddCommand(subCMDs...)
	}
	cmd.AddCommand(
		NewProviderListCommand(c),
		//NewProviderAddCommand(c),
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

func prepareProviderAddCommand(c common.Args) ([]*cobra.Command, error) {
	ctx := context.Background()
	k8sClient, err := c.GetClient()
	if err != nil {
		return nil, err
	}
	defs, err := getTerraformProviderTypes(ctx, k8sClient)
	if err != nil || len(defs) == 0 {
		cmd := &cobra.Command{
			Use:     "add",
			Short:   "add a Terraform Cloud Provider",
			Long:    "add a Terraform Cloud Provider by creating a credential secret and a Terraform Controller Provider",
			Example: "vela Provider add <Provider-type>",
			RunE: func(cmd *cobra.Command, args []string) error {
				if len(args) < 1 {
					errMsg := "must specify a Terraform Cloud Provider type"
					defs, err := getTerraformProviderTypes(ctx, k8sClient)
					if err == nil && len(defs) > 0 {
						providerDefNames := make([]string, len(defs))
						for i, def := range defs {
							providerDefNames[i] = def.Name
						}
						names := strings.Join(providerDefNames, ", ")
						errMsg += fmt.Sprintf(": select one from %s", names)
					}
					return errors.New(errMsg)
				}
				return nil
			},
		}
		return []*cobra.Command{cmd}, err
	}
	cmds := make([]*cobra.Command, len(defs))
	for i, d := range defs {
		providerType := d.Name
		cmd := &cobra.Command{
			Use:     fmt.Sprintf("add %s", providerType),
			Short:   fmt.Sprintf("authenticate Terraform Cloud Provider %s", providerType),
			Long:    fmt.Sprintf("authenticate Terraform Cloud Provider %s by creating a credential secret and a Terraform Controller Provider", providerType),
			Example: fmt.Sprintf("vela provider add %s", providerType),
		}
		parameters, err := getParameters(ctx, k8sClient, providerType)
		if err != nil {
			return nil, err
		}
		for _, p := range parameters {
			cmd.Flags().String(p.Name, fmt.Sprint(p.Default), p.Usage)
		}
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			name, err := cmd.Flags().GetString("name")
			if err != nil || name == "" {
				return errors.New("must specify a name for the Terraform Cloud Provider")
			}
			var properties = make(map[string]string, len(parameters))
			for _, p := range parameters {
				value, err := cmd.Flags().GetString(p.Name)
				if err != nil {
					return err
				}
				if value == "" {
					return fmt.Errorf("must specify a value for %s", p.Name)
				}
				properties[p.Name] = value
			}
			data, err := json.Marshal(properties)
			if err != nil {
				return fmt.Errorf("failed to authentiate Terraform cloud provier %s", providerType)
			}
			providerAppName := fmt.Sprintf("config-terraform-provider-%s", name)
			a := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: providerAppName}, a); err != nil {
				if kerrors.IsNotFound(err) {
					a = &v1beta1.Application{
						ObjectMeta: metav1.ObjectMeta{
							Name:      providerAppName,
							Namespace: types.DefaultKubeVelaNS,
						},
						Spec: v1beta1.ApplicationSpec{
							Components: []coreapi.ApplicationComponent{
								{
									Name: providerAppName,
									Type: d.Name,
									Properties: &runtime.RawExtension{
										Raw: data,
									},
								},
							},
						},
					}
					if err := k8sClient.Create(ctx, a); err != nil {
						return err
					}
				}
			}
			return fmt.Errorf("terraform provider %s for %s already exists", name, providerType)
		}
		cmds[i] = cmd
	}
	return cmds, nil
}

// NewProviderAddCommand create a Provider
func NewProviderAddCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add",
		Short:   "add a Terraform Cloud Provider",
		Long:    "add a Terraform Cloud Provider by creating a credential secret and a Terraform Controller Provider",
		Example: "vela Provider add <Provider-type>",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			k8sClient, err := c.GetClient()
			if err != nil {
				return err
			}
			switch {
			case len(args) < 1:
				errMsg := "must specify a Terraform Cloud Provider type"
				defs, err := getTerraformProviderTypes(ctx, k8sClient)
				if err == nil && len(defs) > 0 {
					providerDefNames := make([]string, len(defs))
					for i, def := range defs {
						providerDefNames[i] = def.Name
					}
					names := strings.Join(providerDefNames, ", ")
					errMsg += fmt.Sprintf(": select one from %s", names)
				}
				return errors.New(errMsg)
			case len(args) >= 1:
				// should also support `vela provider add <provider-type> -h`
				providerType := args[0]
				// validate whether providerType is a valid Terraform provider type
				var found bool
				defs, err := getTerraformProviderTypes(ctx, k8sClient)
				if err != nil {
					return fmt.Errorf("failed to validate provider type: %s", providerType)
				}
				for _, def := range defs {
					if def.Name == providerType {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("provider type: %s is invalid", providerType)
				}

				parameters, err := getParameters(ctx, k8sClient, providerType)
				if err != nil {
					return fmt.Errorf("failed to validate provider type: %s", providerType)
				}
				errMsg := fmt.Sprintf("must set properties for the Terraform Cloud Provider %s\n", providerType)
				for _, p := range parameters {
					cmd.Flags().String(p.Name, fmt.Sprint(p.Default), p.Usage)
				}
				cmd.HelpFunc()(cmd, args)
				return errors.New(errMsg)
			}

			return nil
		},
	}

	return cmd
}

// getParameters gets parameter from a Terraform Cloud Provider, ie the ComponentDefinition
func getParameters(ctx context.Context, k8sClient client.Client, providerType string) ([]types.Parameter, error) {
	def, err := getTerraformProviderType(ctx, k8sClient, providerType)
	if err != nil {
		return nil, err
	}
	cap, err := plugins.GetCapabilityByComponentDefinitionObject(*def, "")
	if err != nil {
		return nil, err
	}
	return cap.Parameters, nil
}

func prepareParameterFlags(parameters []types.Parameter) string {
	var guide string
	for _, p := range parameters {
		guide += fmt.Sprintf("    -%s: Required: %v, Description: %s\n", p.Name, p.Required, p.Usage)
	}
	return guide
}

type Provider struct {
	Type string
	Name string
	Age  string
}

func listProviders(ctx context.Context, k8sClient client.Client) error {
	var providers []Provider
	tcProviders := &tcv1beta1.ProviderList{}
	if err := k8sClient.List(ctx, tcProviders, client.InNamespace(ProviderNamespace),
		client.MatchingLabels{"config.oam.dev/type": labelVal}); err != nil {
		if kerrors.IsNotFound(err) {
			defs, err := getTerraformProviderTypes(ctx, k8sClient)
			if err != nil {
				if kerrors.IsNotFound(err) {
					return errors.New("no Terraform Cloud Provider found, please run `vela addon enable` first")
				}
				return errors.Wrap(err, "failed to retrieve providers")
			}
			for _, d := range defs {
				providers = append(providers, Provider{
					Type: d.Name,
				})
			}
		}
		return errors.Wrap(err, "failed to retrieve providers")
	}
	if len(tcProviders.Items) == 0 {
		defs := &v1beta1.ComponentDefinitionList{}
		if err := k8sClient.List(ctx, defs, client.InNamespace(types.DefaultKubeVelaNS),
			client.MatchingLabels{definition.UserPrefix + "type.config.oam.dev": labelVal}); err != nil {
			return errors.Wrap(err, "failed to retrieve providers")
		}
		if len(defs.Items) == 0 {
			return errors.New("no Terraform Cloud Provider found, please run `vela addon enable` first")
		}
		for _, d := range defs.Items {
			providers = append(providers, Provider{
				Type: d.Name,
			})
		}
	} else {
		for _, p := range tcProviders.Items {
			providers = append(providers, Provider{
				Type: p.Labels[oam.WorkloadTypeLabel],
				Name: p.Name,
				Age:  p.CreationTimestamp.String(),
			})
		}
	}

	table := uitable.New()
	table.AddRow("TYPE", "NAME", "CREATED-TIME")

	for _, p := range providers {
		table.AddRow(p.Type, p.Name, p.Age)
	}
	fmt.Println(table.String())
	return nil
}

// getTerraformProviderTypes retrieves all ComponentDefinition for Terraform Cloud Providers which are delivered by
// Terraform Cloud provider addons
func getTerraformProviderTypes(ctx context.Context, k8sClient client.Client) ([]v1beta1.ComponentDefinition, error) {
	defs := &v1beta1.ComponentDefinitionList{}
	if err := k8sClient.List(ctx, defs, client.InNamespace(types.DefaultKubeVelaNS),
		client.MatchingLabels{definition.UserPrefix + "type.config.oam.dev": labelVal}); err != nil {
		return nil, err
	}
	return defs.Items, nil
}

// getTerraformProviderType retrieves the ComponentDefinition for a Terraform Cloud Provider which is delivered by
// the Terraform Cloud provider addon
func getTerraformProviderType(ctx context.Context, k8sClient client.Client, name string) (*v1beta1.ComponentDefinition, error) {
	def := &v1beta1.ComponentDefinition{}
	if err := k8sClient.Get(ctx, k8stypes.NamespacedName{Namespace: types.DefaultKubeVelaNS, Name: name}, def); err != nil {
		return nil, err
	}
	return def, nil
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
