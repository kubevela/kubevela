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

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/env"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// NewEnvCommand creates `env` command and its nested children
func NewEnvCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "env",
		DisableFlagsInUseLine: true,
		Short:                 "Manage environments",
		Long:                  "Manage environments",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	cmd.SetOut(ioStream.Out)
	cmd.AddCommand(NewEnvListCommand(c, ioStream), NewEnvInitCommand(c, ioStream), NewEnvSetCommand(c, ioStream), NewEnvDeleteCommand(c, ioStream))
	return cmd
}

// NewEnvListCommand creates `env list` command for listing all environments
func NewEnvListCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "ls",
		Aliases:               []string{"list"},
		DisableFlagsInUseLine: true,
		Short:                 "List environments",
		Long:                  "List all environments",
		Example:               `vela env ls [env-name]`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			return ListEnvs(context.Background(), newClient, args, ioStream)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(ioStream.Out)
	return cmd
}

// NewEnvInitCommand creates `env init` command for initializing environments
func NewEnvInitCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	var envArgs types.EnvMeta
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "init <envName>",
		DisableFlagsInUseLine: true,
		Short:                 "Create environments",
		Long:                  "Create environment and set the currently using environment",
		Example:               `vela env init test --namespace test --email my@email.com`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := c.GetClient()
			if err != nil {
				return err
			}

			return CreateOrUpdateEnv(ctx, newClient, &envArgs, args, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(ioStreams.Out)
	cmd.Flags().StringVar(&envArgs.Namespace, "namespace", "", "specify K8s namespace for env")
	cmd.Flags().StringVar(&envArgs.Email, "email", "", "specify email for production TLS Certificate notification")
	cmd.Flags().StringVar(&envArgs.Domain, "domain", "", "specify domain your applications")
	return cmd
}

// NewEnvDeleteCommand creates `env delete` command for deleting environments
func NewEnvDeleteCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "delete",
		DisableFlagsInUseLine: true,
		Short:                 "Delete environment",
		Long:                  "Delete environment",
		Example:               `vela env delete test`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			return DeleteEnv(context.Background(), newClient, args, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

// NewEnvSetCommand creates `env set` command for setting current environment
func NewEnvSetCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "set",
		Aliases:               []string{"sw"},
		DisableFlagsInUseLine: true,
		Short:                 "Set an environment",
		Long:                  "Set an environment as the current using one",
		Example:               `vela env set test`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			return SetEnv(context.Background(), newClient, args, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

// ListEnvs shows info of all environments
func ListEnvs(ctx context.Context, c client.Client, args []string, ioStreams cmdutil.IOStreams) error {
	table := newUITable()
	table.AddRow("NAME", "CURRENT", "NAMESPACE", "EMAIL", "DOMAIN")
	var envName = ""
	if len(args) > 0 {
		envName = args[0]
	}
	envList, err := env.ListEnvs(ctx, c, envName)
	if err != nil {
		return err
	}
	for _, env := range envList {
		table.AddRow(env.Name, env.Current, env.Namespace, env.Email, env.Domain)
	}
	ioStreams.Info(table.String())
	return nil
}

// DeleteEnv deletes an environment
func DeleteEnv(ctx context.Context, c client.Client, args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) < 1 {
		return fmt.Errorf("you must specify environment name for 'vela env delete' command")
	}
	for _, envName := range args {
		msg, err := env.DeleteEnv(ctx, c, envName)
		if err != nil {
			return err
		}
		ioStreams.Info(msg)
	}
	return nil
}

// CreateOrUpdateEnv creates or updates an environment
func CreateOrUpdateEnv(ctx context.Context, c client.Client, envArgs *types.EnvMeta, args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) < 1 {
		return fmt.Errorf("you must specify environment name for 'vela env init' command")
	}
	envArgs.Name = args[0]
	msg, err := env.CreateOrUpdateEnv(ctx, c, envArgs)
	if err != nil {
		return err
	}
	ioStreams.Info(msg)
	return nil
}

// SetEnv sets current environment
func SetEnv(ctx context.Context, c client.Client, args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) < 1 {
		return fmt.Errorf("you must specify environment name for vela env command")
	}
	envName := args[0]
	msg, err := env.SetEnv(ctx, c, envName)
	if err != nil {
		return err
	}
	ioStreams.Info(msg)
	return nil
}

// GetEnv gets environment by name or current environment
// if no env exists, then init default environment
func GetEnv(ctx context.Context, c client.Client, envName string) (*types.EnvMeta, error) {
	var err error
	if envName != "" {
		return env.GetEnvByName(ctx, c, envName)
	}
	envMeta, err := env.GetCurrentEnv(ctx, c)
	if err != nil {
		if err = env.InitDefaultEnv(ctx, c); err != nil {
			return nil, err
		}
		return env.GetEnvByName(ctx, c, types.DefaultEnvName)
	}
	return envMeta, nil
}
