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
	"strings"

	"github.com/gosuri/uitable"
	tcv1beta1 "github.com/oam-dev/terraform-controller/api/v1beta1"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	coreapi "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/plugins"
)

const (

	// ProviderNamespace is the namespace of Terraform Cloud Provider
	ProviderNamespace = "default"

	providerNameParam = "name"
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
	add, err := prepareProviderAddCommand(c, ioStreams)
	if err == nil {
		cmd.AddCommand(add)
	}

	delete, err := prepareProviderDeleteCommand(c, ioStreams)
	if err == nil {
		cmd.AddCommand(delete)
	}

	cmd.AddCommand(
		NewProviderListCommand(c, ioStreams),
	)
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

func prepareProviderAddCommand(c common.Args, ioStreams cmdutil.IOStreams) (*cobra.Command, error) {
	ctx := context.Background()
	k8sClient, err := c.GetClient()
	if err != nil {
		return nil, err
	}

	cmd := &cobra.Command{
		Use:     "add",
		Short:   "Authenticate Terraform Cloud Provider",
		Long:    "Authenticate Terraform Cloud Provider by creating a credential secret and a Terraform Controller Provider",
		Example: "vela provider add <provider-type>",
	}

	addSubCommands, err := prepareProviderAddSubCommand(c, ioStreams)
	if err != nil {
		return nil, err
	}
	cmd.AddCommand(addSubCommands...)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		defs, err := getTerraformProviderTypes(ctx, k8sClient)
		if len(args) < 1 {
			errMsg := "must specify a Terraform Cloud Provider type"
			if err == nil {
				if len(defs) > 0 {
					providerDefNames := make([]string, len(defs))
					for i, def := range defs {
						providerDefNames[i] = def.Name
					}
					names := strings.Join(providerDefNames, ", ")
					errMsg += fmt.Sprintf(": select one from %s", names)
				} else {
					errMsg += "\nNo Terraform Cloud Provider types exist. Please run `vela addon enable` first"
				}
			}
			return errors.New(errMsg)
		} else if err == nil {
			var found bool
			for _, def := range defs {
				if def.Name == args[0] {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("%s is not valid", args[0])
			}
		}
		return nil
	}
	return cmd, nil
}

func prepareProviderAddSubCommand(c common.Args, ioStreams cmdutil.IOStreams) ([]*cobra.Command, error) {
	ctx := context.Background()
	k8sClient, err := c.GetClient()
	if err != nil {
		return nil, err
	}
	defs, err := getTerraformProviderTypes(ctx, k8sClient)
	if err == nil {
		cmds := make([]*cobra.Command, len(defs))
		for i, d := range defs {
			providerType := d.Name
			cmd := &cobra.Command{
				Use:     providerType,
				Short:   fmt.Sprintf("Authenticate Terraform Cloud Provider %s", providerType),
				Long:    fmt.Sprintf("Authenticate Terraform Cloud Provider %s by creating a credential secret and a Terraform Controller Provider", providerType),
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
				name, err := cmd.Flags().GetString(providerNameParam)
				if err != nil || name == "" {
					return fmt.Errorf("must specify a name for the Terraform Cloud Provider %s", providerType)
				}
				var properties = make(map[string]string, len(parameters))
				for _, p := range parameters {
					value, err := cmd.Flags().GetString(p.Name)
					if err != nil {
						return err
					}
					if value == "" && p.Required {
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
										Type: providerType,
										Properties: &runtime.RawExtension{
											Raw: data,
										},
									},
								},
							},
						}
						if err := k8sClient.Create(ctx, a); err != nil {
							return fmt.Errorf("failed to authentiate Terraform cloud provier %s", providerType)
						}
						ioStreams.Infof("Successfully authentiate provider %s for %s\n", name, providerType)
						return nil
					}
					return fmt.Errorf("failed to authentiate Terraform cloud provier %s", providerType)
				}
				return fmt.Errorf("terraform provider %s for %s already exists", name, providerType)
			}
			cmds[i] = cmd
		}
		return cmds, nil
	}
	return nil, nil
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

// ProviderMeta is the metadata for a Terraform Cloud Provider
type ProviderMeta struct {
	Type string
	Name string
	Age  string
}

func listProviders(ctx context.Context, k8sClient client.Client, ioStreams cmdutil.IOStreams) error {
	var (
		providers        []ProviderMeta
		currentProviders []tcv1beta1.Provider
		legacyProviders  []tcv1beta1.Provider
	)
	tcProviders := &tcv1beta1.ProviderList{}
	// client.MatchingLabels{: }
	if err := k8sClient.List(ctx, tcProviders, client.InNamespace(ProviderNamespace)); err != nil {
		return errors.Wrap(err, "failed to retrieve providers")
	}

	for _, p := range tcProviders.Items {
		if p.Labels["config.oam.dev/type"] == types.TerraformProvider {
			currentProviders = append(currentProviders, p)
		} else {
			// if not labeled, the provider is manually created or created by `vela addon enable`.
			legacyProviders = append(legacyProviders, p)
		}
	}

	defs, err := getTerraformProviderTypes(ctx, k8sClient)
	if err != nil {
		if kerrors.IsNotFound(err) {
			ioStreams.Info("no Terraform Cloud Provider found, please run `vela addon enable` first")
		}
		return errors.Wrap(err, "failed to retrieve providers")
	}

	for _, d := range defs {
		var found bool
		for _, p := range currentProviders {
			if p.Labels[oam.WorkloadTypeLabel] == d.Name {
				found = true
				providers = append(providers, ProviderMeta{
					Type: p.Labels[oam.WorkloadTypeLabel],
					Name: p.Name,
					Age:  p.CreationTimestamp.String(),
				})
			}
		}
		if !found {
			providers = append(providers, ProviderMeta{
				Type: d.Name,
			})
		}
	}

	for _, p := range legacyProviders {
		providers = append(providers, ProviderMeta{
			Type: "-",
			Name: p.Name + "(legacy)",
			Age:  p.CreationTimestamp.String(),
		})
	}

	if len(providers) == 0 {
		return errors.New("no Terraform Cloud Provider found, please run `vela addon enable` first")
	}

	table := uitable.New()
	table.AddRow("TYPE", "NAME", "CREATED-TIME")

	for _, p := range providers {
		table.AddRow(p.Type, p.Name, p.Age)
	}
	ioStreams.Info(table.String())
	return nil
}

// getTerraformProviderTypes retrieves all ComponentDefinition for Terraform Cloud Providers which are delivered by
// Terraform Cloud provider addons
func getTerraformProviderTypes(ctx context.Context, k8sClient client.Client) ([]v1beta1.ComponentDefinition, error) {
	defs := &v1beta1.ComponentDefinitionList{}
	if err := k8sClient.List(ctx, defs, client.InNamespace(types.DefaultKubeVelaNS),
		client.MatchingLabels{definition.UserPrefix + "type.config.oam.dev": types.TerraformProvider}); err != nil {
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

func prepareProviderDeleteCommand(c common.Args, ioStreams cmdutil.IOStreams) (*cobra.Command, error) {
	ctx := context.Background()
	k8sClient, err := c.GetClient()
	if err != nil {
		return nil, err
	}

	cmd := &cobra.Command{
		Use:     "delete",
		Aliases: []string{"rm", "del"},
		Short:   "Delete Terraform Cloud Provider",
		Long:    "Delete Terraform Cloud Provider",
		Example: "vela provider delete <provider-type> -name <provider-name>",
	}

	deleteSubCommands, err := prepareProviderDeleteSubCommand(c, ioStreams)
	if err != nil {
		return nil, err
	}
	cmd.AddCommand(deleteSubCommands...)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		defs, err := getTerraformProviderTypes(ctx, k8sClient)
		if len(args) < 1 {
			errMsg := "must specify a Terraform Cloud Provider type"
			if err == nil {
				if len(defs) > 0 {
					providerDefNames := make([]string, len(defs))
					for i, def := range defs {
						providerDefNames[i] = def.Name
					}
					names := strings.Join(providerDefNames, ", ")
					errMsg += fmt.Sprintf(": select one from %s", names)
				} else {
					errMsg += "\nNo Terraform Cloud Provider types exist. Please run `vela addon enable` first"
				}
			}
			return errors.New(errMsg)
		} else if err == nil {
			var found bool
			for _, def := range defs {
				if def.Name == args[0] {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("%s is not valid", args[0])
			}
		}
		return nil
	}
	return cmd, nil
}

func prepareProviderDeleteSubCommand(c common.Args, ioStreams cmdutil.IOStreams) ([]*cobra.Command, error) {
	ctx := context.Background()
	k8sClient, err := c.GetClient()
	if err != nil {
		return nil, err
	}
	defs, err := getTerraformProviderTypes(ctx, k8sClient)
	if err == nil {
		cmds := make([]*cobra.Command, len(defs))
		for i, d := range defs {
			providerType := d.Name
			cmd := &cobra.Command{
				Use:     providerType,
				Short:   fmt.Sprintf("Delete Terraform Cloud Provider %s", providerType),
				Long:    fmt.Sprintf("Delete Terraform Cloud Provider %s", providerType),
				Example: fmt.Sprintf("vela provider delete %s", providerType),
			}
			parameters, err := getParameters(ctx, k8sClient, providerType)
			if err != nil {
				return nil, err
			}
			for _, p := range parameters {
				if p.Name == providerNameParam {
					cmd.Flags().String(p.Name, fmt.Sprint(p.Default), p.Usage)
				}
			}
			cmd.RunE = func(cmd *cobra.Command, args []string) error {
				name, err := cmd.Flags().GetString(providerNameParam)
				if err != nil || name == "" {
					return fmt.Errorf("must specify a name for the Terraform Cloud Provider %s", providerType)
				}
				providerAppName := fmt.Sprintf("config-terraform-provider-%s", name)
				a := &v1beta1.Application{}
				if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: providerAppName}, a); err != nil {
					if kerrors.IsNotFound(err) {
						return fmt.Errorf("provider %s for %s does not exist", name, providerType)
					}
				}
				if err := k8sClient.Delete(ctx, a); err != nil {
					return err
				}
				ioStreams.Infof("Successfully delete provider %s for %s\n", name, providerType)
				return nil
			}
			cmds[i] = cmd
		}
		return cmds, nil
	}
	return nil, nil
}
