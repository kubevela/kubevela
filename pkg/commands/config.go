package commands

import (
	"bufio"
	"bytes"
	b64 "encoding/base64"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/api/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/utils/config"
)

// Notes about config dir layout:
// Under each env dir, there are individual files for each config.
// The format is the same as k8s Secret.Data field with value base64 encoded.

func NewConfigCommand(args types.Args, io cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "config",
		DisableFlagsInUseLine: true,
		Short:                 "Manage application configurations",
		Long:                  "Manage application configurations under given env",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(io.Out)
	cmd.AddCommand(
		NewConfigListCommand(io),
		NewConfigGetCommand(io),
		NewConfigSetCommand(io),
		NewConfigDeleteCommand(io),
	)
	return cmd
}

func NewConfigListCommand(io cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "ls",
		Aliases:               []string{"list"},
		DisableFlagsInUseLine: true,
		Short:                 "List configs",
		Long:                  "List all configs",
		Example:               `vela config ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := getConfigDir(cmd)
			if err != nil {
				return err
			}
			return ListConfigs(io, d)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(io.Out)
	return cmd
}
func getConfigDir(cmd *cobra.Command) (string, error) {
	e, err := GetEnv(cmd)
	if err != nil {
		return "", err
	}
	return config.GetConfigsDir(e.Name)
}
func ListConfigs(ioStreams cmdutil.IOStreams, cfgDir string) error {
	table := uitable.New()
	table.MaxColWidth = 60
	table.AddRow("NAME")
	cfgList, err := listConfigs(cfgDir)
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

func NewConfigGetCommand(io cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "get",
		Aliases:               []string{"get"},
		DisableFlagsInUseLine: true,
		Short:                 "Get data for a config",
		Long:                  "Get data for a config",
		Example:               `vela config get <config-name>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := GetEnv(cmd)
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
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(io.Out)
	return cmd
}

func NewConfigSetCommand(io cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "set",
		Aliases:               []string{"set"},
		DisableFlagsInUseLine: true,
		Short:                 "Set data for a config",
		Long:                  "Set data for a config",
		Example:               `vela config set <config-name> KEY=VALUE K2=V2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := GetEnv(cmd)
			if err != nil {
				return err
			}
			return setConfig(args, e.Name, io)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(io.Out)
	return cmd
}

func setConfig(args []string, envName string, io cmdutil.IOStreams) error {
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

	cfgData, err := config.ReadConfig(envName, configName)
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

	err = config.WriteConfig(envName, configName, out.Bytes())
	if err != nil {
		return err
	}
	io.Infof("config data saved successfully %s\n", emojiSucceed)
	return nil
}

func NewConfigDeleteCommand(io cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "del",
		Aliases:               []string{"del"},
		DisableFlagsInUseLine: true,
		Short:                 "Delete config",
		Long:                  "Delete config",
		Example:               `vela config del <config-name>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := GetEnv(cmd)
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
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(io.Out)
	return cmd
}
