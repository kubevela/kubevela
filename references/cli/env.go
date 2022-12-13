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
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/env"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// NewEnvCommand creates `env` command and its nested children
func NewEnvCommand(c common.Args, order string, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "env",
		DisableFlagsInUseLine: true,
		Short:                 "Manage environments for vela applications to run.",
		Long:                  "Manage environments for vela applications to run.",
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeStart,
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
		Short:                 "List environments for vela applications to run.",
		Long:                  "List all environments for vela applications to run.",
		Example:               `vela env ls [env-name]`,
		RunE: func(cmd *cobra.Command, args []string) error {
			clt, err := c.GetClient()
			if err != nil {
				return err
			}
			err = common.SetGlobalClient(clt)
			if err != nil {
				return err
			}
			return ListEnvs(args, ioStream)
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
	cmd := &cobra.Command{
		Use:                   "init <envName>",
		DisableFlagsInUseLine: true,
		Short:                 "Create environment for vela applications to run.",
		Long:                  "Create environment for vela applications to run.",
		Example:               `vela env init test --namespace test`,
		RunE: func(cmd *cobra.Command, args []string) error {
			clt, err := c.GetClient()
			if err != nil {
				return err
			}
			err = common.SetGlobalClient(clt)
			if err != nil {
				return err
			}
			return CreateEnv(&envArgs, args, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(ioStreams.Out)
	cmd.Flags().StringVar(&envArgs.Namespace, "namespace", "", "specify K8s namespace for env")
	return cmd
}

// NewEnvDeleteCommand creates `env delete` command for deleting environments
func NewEnvDeleteCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "delete",
		DisableFlagsInUseLine: true,
		Short:                 "Delete an environment.",
		Long:                  "Delete an environment.",
		Example:               `vela env delete test`,
		RunE: func(cmd *cobra.Command, args []string) error {
			clt, err := c.GetClient()
			if err != nil {
				return err
			}
			err = common.SetGlobalClient(clt)
			if err != nil {
				return err
			}
			return DeleteEnv(args, ioStreams)
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
	var envArgs types.EnvMeta
	cmd := &cobra.Command{
		Use:                   "set",
		Aliases:               []string{"sw"},
		DisableFlagsInUseLine: true,
		Short:                 "Set an environment.",
		Long:                  "Set an environment as the default one for running vela applications.",
		Example:               `vela env set test`,
		RunE: func(cmd *cobra.Command, args []string) error {
			clt, err := c.GetClient()
			if err != nil {
				return err
			}
			err = common.SetGlobalClient(clt)
			if err != nil {
				return err
			}
			return SetEnv(&envArgs, args, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(ioStreams.Out)
	cmd.Flags().StringVar(&envArgs.Labels, "labels", "", "set labels for namespace")
	return cmd
}

// ListEnvs shows info of all environments
func ListEnvs(args []string, ioStreams cmdutil.IOStreams) error {
	table := newUITable()
	table.AddRow("NAME", "NAMESPACE", "CURRENT")
	var envName = ""
	if len(args) > 0 {
		envName = args[0]
	}
	envList, err := env.ListEnvs(envName)
	if err != nil {
		return err
	}
	for _, env := range envList {
		table.AddRow(env.Name, env.Namespace, env.Current)
	}
	ioStreams.Info(table.String())
	return nil
}

// DeleteEnv deletes an environment
func DeleteEnv(args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) < 1 {
		return fmt.Errorf("you must specify environment name for 'vela env delete' command")
	}
	for _, envName := range args {
		msg, err := env.DeleteEnv(envName)
		if err != nil {
			return err
		}
		ioStreams.Info(msg)
	}
	return nil
}

// CreateEnv creates an environment
func CreateEnv(envArgs *types.EnvMeta, args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) > 0 {
		envArgs.Name = args[0]
	}
	err := env.CreateEnv(envArgs)
	if err != nil {
		return err
	}
	ioStreams.Infof("environment %s with namespace %s created\n", envArgs.Name, envArgs.Namespace)
	return env.SetCurrentEnv(envArgs)
}

// SetEnv sets current environment
func SetEnv(envArgs *types.EnvMeta, args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) < 1 {
		return fmt.Errorf("you must specify environment name for vela env command")
	}
	envName := args[0]
	envMeta, err := env.GetEnvByName(envName)
	if err != nil {
		return err
	}
	if envArgs.Labels != "" {
		envArgs.Name = envMeta.Name
		// just set labels, not change current env
		return env.SetEnvLabels(envArgs)
	}
	err = env.SetCurrentEnv(envMeta)
	if err != nil {
		return err
	}
	ioStreams.Info(fmt.Sprintf("Current environment switched to %s (namespace = %s)", envName, envMeta.Namespace))
	return nil
}

// GetFlagEnvOrCurrent gets environment by name or current environment
// if no env exists, return default namespace as env
func GetFlagEnvOrCurrent(cmd *cobra.Command, args common.Args) (*types.EnvMeta, error) {
	clt, err := args.GetClient()
	if err != nil {
		return nil, err
	}
	err = common.SetGlobalClient(clt)
	if err != nil {
		return nil, errors.Wrap(err, "get flag env fail")
	}
	var envName string
	if cmd != nil {
		envName = cmd.Flag("env").Value.String()
	}
	if envName != "" {
		return env.GetEnvByName(envName)
	}
	cur, err := env.GetCurrentEnv()
	if err != nil {
		// ignore this error and return a default value
		// nolint:nilerr
		ns := args.GetNamespaceFromConfig()
		if ns == "" {
			ns = types.DefaultAppNamespace
		}
		return &types.EnvMeta{Name: "", Namespace: ns}, nil
	}
	return cur, nil
}
