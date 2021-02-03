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
	"bufio"
	"bytes"
	"context"
	b64 "encoding/base64"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/config"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// Notes about config dir layout:
// Under each env dir, there are individual files for each config.
// The format is the same as k8s Secret.Data field with value base64 encoded.

// NewConfigCommand will create command for config management for AppFile
func NewConfigCommand(c common.Args, io cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "config",
		DisableFlagsInUseLine: true,
		Short:                 "Manage configurations",
		Long:                  "Manage configurations",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	cmd.SetOut(io.Out)
	cmd.AddCommand(
		NewConfigListCommand(c, io),
		NewConfigGetCommand(c, io),
		NewConfigSetCommand(c, io),
		NewConfigDeleteCommand(c, io),
	)
	return cmd
}

// NewConfigListCommand list all created configs
func NewConfigListCommand(c common.Args, io cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "ls",
		Aliases:               []string{"list"},
		DisableFlagsInUseLine: true,
		Short:                 "List configs",
		Long:                  "List all configs",
		Example:               `vela config ls`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := c.GetClient()
			if err != nil {
				return err
			}
			return ListConfigs(context.Background(), newClient, cmd.Flag("env").Value.String(), io)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(io.Out)
	return cmd
}

func getConfigDir(ctx context.Context, c client.Client, envName string) (string, error) {
	e, err := GetEnv(ctx, c, envName)
	if err != nil {
		return "", err
	}
	return config.GetConfigsDir(e.Name)
}

// ListConfigs will list all configs
func ListConfigs(ctx context.Context, c client.Client, envName string, ioStreams cmdutil.IOStreams) error {
	d, err := getConfigDir(ctx, c, envName)
	if err != nil {
		return err
	}
	table := newUITable()
	table.AddRow("NAME")
	cfgList, err := listConfigs(d)
	if err != nil {
		return err
	}

	for _, name := range cfgList {
		table.AddRow(name)
	}
	ioStreams.Info(table.String())
	return nil
}

func listConfigs(dir string) ([]string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	l := []string{}
	for _, f := range files {
		l = append(l, f.Name())
	}
	return l, nil
}

// NewConfigGetCommand get config from local
func NewConfigGetCommand(c common.Args, io cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "get",
		Aliases:               []string{"get"},
		DisableFlagsInUseLine: true,
		Short:                 "Get data for a config",
		Long:                  "Get data for a config",
		Example:               `vela config get <config-name>`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := c.GetClient()
			if err != nil {
				return err
			}
			return getConfig(context.Background(), newClient, cmd.Flag("env").Value.String(), args, io)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(io.Out)
	return cmd
}

func getConfig(ctx context.Context, c client.Client, envName string, args []string, io cmdutil.IOStreams) error {
	e, err := GetEnv(ctx, c, envName)
	if err != nil {
		return err
	}
	if len(args) < 1 {
		return fmt.Errorf("must specify config name, vela config get <name>")
	}
	configName := args[0]
	cfgData, err := config.ReadConfig(e.Name, configName)
	if err != nil {
		return err
	}
	io.Infof("Data:\n")
	scanner := bufio.NewScanner(bytes.NewReader(cfgData))
	for scanner.Scan() {
		k, v, err := config.ReadConfigLine(scanner.Text())
		if err != nil {
			return err
		}
		io.Infof("  %s: %s\n", k, v)
	}
	return nil
}

// NewConfigSetCommand set a config data in local
func NewConfigSetCommand(c common.Args, io cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "set",
		Aliases:               []string{"set"},
		DisableFlagsInUseLine: true,
		Short:                 "Set data for a config",
		Long:                  "Set data for a config",
		Example:               `vela config set <config-name> KEY=VALUE K2=V2`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := c.GetClient()
			if err != nil {
				return err
			}
			return getConfig(context.Background(), newClient, cmd.Flag("env").Value.String(), args, io)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(io.Out)
	return cmd
}

func setConfig(ctx context.Context, c client.Client, envName string, args []string, io cmdutil.IOStreams) error {
	e, err := GetEnv(ctx, c, envName)
	if err != nil {
		return err
	}

	if len(args) < 1 {
		return fmt.Errorf("must specify config name, vela config set <name> KEY=VALUE")
	}
	configName := args[0]

	input := map[string]string{}
	for _, arg := range args[1:] {
		ss := strings.SplitN(arg, "=", 2)
		if len(ss) != 2 {
			return fmt.Errorf("KV argument malformed: %s, should be KEY=VALUE", arg)
		}
		k := strings.TrimSpace(ss[0])
		v := strings.TrimSpace(ss[1])
		if _, ok := input[k]; ok {
			return fmt.Errorf("KEY is not unique: %s", arg)
		}
		input[k] = v
	}

	cfgData, err := config.ReadConfig(e.Name, configName)
	if err != nil {
		return err
	}

	io.Infof("reading existing config data and merging with user input\n")
	scanner := bufio.NewScanner(bytes.NewReader(cfgData))
	for scanner.Scan() {
		k, v, err := config.ReadConfigLine(scanner.Text())
		if err != nil {
			return err
		}
		input[k] = v
	}

	var out bytes.Buffer
	for k, v := range input {
		vEnc := b64.StdEncoding.EncodeToString([]byte(v))
		out.WriteString(fmt.Sprintf("%s: %s\n", k, vEnc))
	}
	err = config.WriteConfig(e.Name, configName, out.Bytes())
	if err != nil {
		return err
	}
	io.Infof("config data saved successfully %s\n", emojiSucceed)
	return nil
}

// NewConfigDeleteCommand delete a config from local
func NewConfigDeleteCommand(c common.Args, io cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "del",
		Aliases:               []string{"del"},
		DisableFlagsInUseLine: true,
		Short:                 "Delete config",
		Long:                  "Delete config",
		Example:               `vela config del <config-name>`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := c.GetClient()
			if err != nil {
				return err
			}
			return deleteConfig(context.Background(), newClient, cmd.Flag("env").Value.String(), args, io)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(io.Out)
	return cmd
}

func deleteConfig(ctx context.Context, c client.Client, envName string, args []string, io cmdutil.IOStreams) error {
	e, err := GetEnv(ctx, c, envName)
	if err != nil {
		return err
	}
	if len(args) < 1 {
		return fmt.Errorf("must specify config name, vela config get <name>")
	}
	configName := args[0]
	err = config.DeleteConfig(e.Name, configName)
	if err != nil {
		return err
	}
	io.Infof("config (%s) deleted successfully\n", configName)
	return nil
}
