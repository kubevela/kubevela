package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloud-native-application/rudrx/api/types"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/cloud-native-application/rudrx/pkg/utils/system"

	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
)

func EnvCommandGroup(parentCmd *cobra.Command, c types.Args, ioStream cmdutil.IOStreams) {
	parentCmd.AddCommand(NewEnvInitCommand(c, ioStream),
		NewEnvSwitchCommand(ioStream),
		NewEnvDeleteCommand(ioStream),
		NewEnvCommand(ioStream),
	)
}

func NewEnvCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "env",
		DisableFlagsInUseLine: true,
		Short:                 "List environments",
		Long:                  "List all environments",
		Example:               `vela env [env-name]`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ListEnvs(ctx, args, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(ioStreams.Out)
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		cmdutil.PrintUsageIntroduce(cmd, "Prepare environments for applications")
		subcmds := []*cobra.Command{cmd, NewEnvInitCommand(types.Args{}, ioStreams), NewEnvSwitchCommand(ioStreams), NewEnvDeleteCommand(ioStreams)}
		cmdutil.PrintUsage(cmd, subcmds)
		cmdutil.PrintExample(cmd, subcmds)
		cmdutil.PrintFlags(cmd, subcmds)
	})
	return cmd
}

func NewEnvInitCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	var envArgs types.EnvMeta
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "env:init",
		DisableFlagsInUseLine: true,
		Short:                 "Create environments",
		Long:                  "Create environment and switch to it",
		Example:               `vela env:init test --namespace test`,
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
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
	cmd.Flags().StringVar(&envArgs.Namespace, "namespace", "default", "specify K8s namespace for env")
	return cmd
}

func NewEnvDeleteCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "env:delete",
		DisableFlagsInUseLine: true,
		Short:                 "Delete environment",
		Long:                  "Delete environment",
		Example:               `vela env:delete test`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return DeleteEnv(ctx, args, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func NewEnvSwitchCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "env:sw",
		DisableFlagsInUseLine: true,
		Short:                 "Switch environments",
		Long:                  "switch to another environment",
		Example:               `vela env:sw test`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return SwitchEnv(ctx, args, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func ListEnvs(ctx context.Context, args []string, ioStreams cmdutil.IOStreams) error {
	table := uitable.New()
	table.MaxColWidth = 60
	table.AddRow("NAME", "CURRENT", "NAMESPACE")
	if len(args) > 0 {
		envName := args[0]
		env, err := getEnvByName(envName)
		if err != nil {
			if os.IsNotExist(err) {
				ioStreams.Info(fmt.Sprintf("env %s not exist", envName))
				return nil
			}
			return err
		}
		table.AddRow(envName, env.Namespace)
		ioStreams.Info(table.String())
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
	curEnv, err := GetCurrentEnvName()
	if err != nil {
		curEnv = types.DefaultEnvName
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
		if curEnv == f.Name() {
			table.AddRow(f.Name(), "*", envMeta.Namespace)
		} else {
			table.AddRow(f.Name(), "", envMeta.Namespace)
		}
	}
	ioStreams.Info(table.String())
	return nil
}

func DeleteEnv(ctx context.Context, args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) < 1 {
		return fmt.Errorf("you must specify env name for vela env:delete command")
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

func CreateOrUpdateEnv(ctx context.Context, c client.Client, envArgs *types.EnvMeta, args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) < 1 {
		return fmt.Errorf("you must specify env name for vela env:init command")
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
	if err := c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: envArgs.Namespace}}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	if err = ioutil.WriteFile(curEnvPath, []byte(envname), 0644); err != nil {
		return err
	}
	ioStreams.Info("Create env succeed, current env is " + envname + " namespace is " + envArgs.Namespace + ", use --namespace=<namespace> to specify namespace with env:init")
	return nil
}

func SwitchEnv(ctx context.Context, args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) < 1 {
		return fmt.Errorf("you must specify env name for vela env command")
	}
	envname := args[0]
	currentEnvPath, err := system.GetCurrentEnvPath()
	if err != nil {
		return err
	}
	envMeta, err := getEnvByName(envname)
	if err != nil {
		return err
	}
	if err = ioutil.WriteFile(currentEnvPath, []byte(envname), 0644); err != nil {
		return err
	}
	ioStreams.Info("Switch env succeed, current env is " + envname + ", namespace is " + envMeta.Namespace)
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
