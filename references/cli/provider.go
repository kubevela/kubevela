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
	"os"
	"strings"
	"time"

	"github.com/gosuri/uitable"
	tcv1beta1 "github.com/oam-dev/terraform-controller/api/v1beta1"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/config"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/plugins"
)

const (
	providerNameParam       = "name"
	errAuthenticateProvider = "failed to authenticate Terraform cloud provider %s err: %w"
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
	if err != nil {
		ioStreams.Errorf("fail to init the provider command:%s \n", err.Error())
	}
	if add != nil {
		cmd.AddCommand(add)
	}

	delete, err := prepareProviderDeleteCommand(c, ioStreams)
	if err != nil {
		ioStreams.Errorf("fail to init the provider command:%s \n", err.Error())
	}
	if delete != nil {
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
	if len(os.Args) < 2 || os.Args[1] != "provider" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	defer cancel()
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	defer cancel()
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
					return fmt.Errorf(errAuthenticateProvider, providerType, err)
				}

				if err := config.CreateApplication(ctx, k8sClient, name, providerType, string(data), config.UIParam{}); err != nil {
					return fmt.Errorf(errAuthenticateProvider, providerType, err)
				}
				ioStreams.Infof("Successfully authenticate provider %s for %s\n", name, providerType)
				return nil
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
	l, err := config.ListTerraformProviders(ctx, k8sClient)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve providers")
	}

	for _, p := range l {
		// The first condition matches the providers created by `vela provider` in 1.3.2 and earlier.
		if p.Labels[types.LabelConfigType] == types.TerraformProvider || p.Labels[types.LabelConfigCatalog] == types.VelaCoreConfig {
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
	if len(os.Args) < 2 || os.Args[1] != "provider" {
		return nil, nil
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
		k8sClient, err := c.GetClient()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
		defer cancel()
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	defer cancel()
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
				if err := config.DeleteApplication(ctx, k8sClient, name, true); err != nil {
					return errors.Wrapf(err, "failed to delete Terraform Cloud Provider %s", name)
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
