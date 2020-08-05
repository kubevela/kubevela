package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cloud-native-application/rudrx/api/types"

	"github.com/cloud-native-application/rudrx/pkg/utils/system"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
)

func NewEnvInitCommand(f cmdutil.Factory, ioStreams cmdutil.IOStreams) *cobra.Command {
	var envArgs types.EnvMeta
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "env:init",
		DisableFlagsInUseLine: true,
		Short:                 "Create environments",
		Long:                  "Create environment and switch to it",
		Example:               `rudr env:init test --namespace test`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return CreateOrUpdateEnv(ctx, &envArgs, args, ioStreams)
		},
	}
	cmd.SetOut(ioStreams.Out)
	cmd.Flags().StringVar(&envArgs.Namespace, "namespace", "default", "specify K8s namespace for env")
	return cmd
}

func NewEnvDeleteCommand(f cmdutil.Factory, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "env:delete",
		DisableFlagsInUseLine: true,
		Short:                 "Delete environment",
		Long:                  "Delete environment",
		Example:               `rudr env:delete test`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return DeleteEnv(ctx, args, ioStreams)
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func NewEnvCommand(f cmdutil.Factory, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "env",
		DisableFlagsInUseLine: true,
		Short:                 "List environments",
		Long:                  "List all environments",
		Example:               `rudr env [env-name]`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ListEnvs(ctx, args, ioStreams)
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func NewEnvSwitchCommand(f cmdutil.Factory, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "env:sw",
		DisableFlagsInUseLine: true,
		Short:                 "Switch environments",
		Long:                  "switch to another environment",
		Example:               `rudr env test`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return SwitchEnv(ctx, args, ioStreams)
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func ListEnvs(ctx context.Context, args []string, ioStreams cmdutil.IOStreams) error {
	table := uitable.New()
	table.MaxColWidth = 60
	table.AddRow("NAME", "NAMESPACE")
	if len(args) > 0 {
		envName := args[0]
		env, err := getEnvByName(envName)
		if err != nil {
			return err
		}
		table.AddRow(envName, env.Namespace)
		ioStreams.Infof(table.String())
		return nil
	}
	envDir, err := system.GetEnvDir()
	if err != nil {
		return err
	}
	files, err := ioutil.ReadDir(envDir)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		data, err := ioutil.ReadFile(filepath.Join(envDir, f.Name()))
		if err != nil {
			continue
		}
		var envMeta types.EnvMeta
		if err = json.Unmarshal(data, &envMeta); err != nil {
			continue
		}
		table.AddRow(f.Name(), envMeta.Namespace)
	}
	ioStreams.Infof(table.String())
	return nil
}

func DeleteEnv(ctx context.Context, args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) < 1 {
		return fmt.Errorf("you must specify env name for rudr env:delete command")
	}
	envname := args[0]
	curEnv, err := GetCurrentEnvName()
	if err != nil {
		return err
	}
	if envname == curEnv {
		return fmt.Errorf("you can't delete current using env %s", curEnv)
	}
	envdir, err := system.GetEnvDir()
	if err != nil {
		return err
	}
	if err = os.Remove(filepath.Join(envdir, envname)); err != nil {
		return err
	}
	ioStreams.Info(envname + " deleted")
	return nil
}

func CreateOrUpdateEnv(ctx context.Context, envArgs *types.EnvMeta, args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) < 1 {
		return fmt.Errorf("you must specify env name for rudr env:init command")
	}
	envname := args[0]
	data, err := json.Marshal(envArgs)
	if err != nil {
		return err
	}
	envdir, err := system.GetEnvDir()
	if err != nil {
		return err
	}
	if err = ioutil.WriteFile(filepath.Join(envdir, envname), data, 0644); err != nil {
		return err
	}
	curEnvPath, err := system.GetCurrentEnvPath()
	if err != nil {
		return err
	}
	if err = ioutil.WriteFile(curEnvPath, []byte(envname), 0644); err != nil {
		return err
	}
	ioStreams.Info("Create env succeed, current env is " + envname)
	return nil
}

func SwitchEnv(ctx context.Context, args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) < 1 {
		return fmt.Errorf("you must specify env name for rudr env command")
	}
	envname := args[0]
	currentEnvPath, err := system.GetCurrentEnvPath()
	if err != nil {
		return err
	}
	_, err = getEnvByName(envname)
	if err != nil {
		return err
	}
	if err = ioutil.WriteFile(currentEnvPath, []byte(envname), 0644); err != nil {
		return err
	}
	ioStreams.Info("Switch env succeed, current env is " + envname)
	return nil
}

func GetCurrentEnvName() (string, error) {
	currentEnvPath, err := system.GetCurrentEnvPath()
	if err != nil {
		return "", err
	}
	data, err := ioutil.ReadFile(currentEnvPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func GetEnv() (*types.EnvMeta, error) {
	envName, err := GetCurrentEnvName()
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		if err = system.InitDefaultEnv(); err != nil {
			return nil, err
		}
		envName = types.DefaultEnvName
	}
	return getEnvByName(envName)
}

func getEnvByName(name string) (*types.EnvMeta, error) {
	envdir, err := system.GetEnvDir()
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadFile(filepath.Join(envdir, name))
	if err != nil {
		return nil, err
	}
	var meta types.EnvMeta
	if err = json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}
