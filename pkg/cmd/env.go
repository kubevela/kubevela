package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
)

const DefaultEnvName = "default"

func NewEnvInitCommand(f cmdutil.Factory, ioStreams cmdutil.IOStreams) *cobra.Command {
	var envArgs EnvMeta
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

type EnvMeta struct {
	Namespace string `json:"namespace"`
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
	envDir, err := getEnvDir()
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
		var envMeta EnvMeta
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
	envdir, err := getEnvDir()
	if err != nil {
		return err
	}
	if err = os.Remove(filepath.Join(envdir, envname)); err != nil {
		return err
	}
	ioStreams.Info(envname + " deleted")
	return nil
}

func InitDefaultEnv() error {
	envDir, err := getEnvDir()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(envDir, 0755); err != nil {
		return err
	}
	data, _ := json.Marshal(&EnvMeta{Namespace: DefaultEnvName})
	if err = ioutil.WriteFile(filepath.Join(envDir, DefaultEnvName), data, 0644); err != nil {
		return err
	}
	curEnvPath, err := getCurrentEnvPath()
	if err != nil {
		return err
	}
	if err = ioutil.WriteFile(curEnvPath, []byte(DefaultEnvName), 0644); err != nil {
		return err
	}
	return nil
}

func CreateOrUpdateEnv(ctx context.Context, envArgs *EnvMeta, args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) < 1 {
		return fmt.Errorf("you must specify env name for rudr env:init command")
	}
	envname := args[0]
	data, err := json.Marshal(envArgs)
	if err != nil {
		return err
	}
	envdir, err := getEnvDir()
	if err != nil {
		return err
	}
	if err = ioutil.WriteFile(filepath.Join(envdir, envname), data, 0644); err != nil {
		return err
	}
	curEnvPath, err := getCurrentEnvPath()
	if err != nil {
		return err
	}
	if err = ioutil.WriteFile(curEnvPath, []byte(envname), 0644); err != nil {
		return err
	}
	ioStreams.Info("Create env succeed, current env is " + envname)
	return nil
}

func getCurrentEnvPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".rudr", "curenv"), nil
}

func getEnvDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".rudr", "envs"), nil
}

func SwitchEnv(ctx context.Context, args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) < 1 {
		return fmt.Errorf("you must specify env name for rudr env command")
	}
	envname := args[0]
	currentEnvPath, err := getCurrentEnvPath()
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
	currentEnvPath, err := getCurrentEnvPath()
	if err != nil {
		return "", err
	}
	data, err := ioutil.ReadFile(currentEnvPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func GetEnv() (*EnvMeta, error) {
	envName, err := GetCurrentEnvName()
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		if err = InitDefaultEnv(); err != nil {
			return nil, err
		}
		envName = DefaultEnvName
	}
	return getEnvByName(envName)
}

func getEnvByName(name string) (*EnvMeta, error) {
	envdir, err := getEnvDir()
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadFile(filepath.Join(envdir, name))
	if err != nil {
		return nil, err
	}
	var meta EnvMeta
	if err = json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}
