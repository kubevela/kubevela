package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/oam-dev/kubevela/api/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/system"

	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewEnvCommand(c types.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "env",
		DisableFlagsInUseLine: true,
		Short:                 "Manage application environments",
		Long:                  "Manage application environments",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(ioStream.Out)
	cmd.AddCommand(NewEnvListCommand(ioStream), NewEnvInitCommand(c, ioStream), NewEnvSwitchCommand(ioStream), NewEnvDeleteCommand(ioStream))
	return cmd
}

func NewEnvListCommand(ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "ls",
		Aliases:               []string{"list"},
		DisableFlagsInUseLine: true,
		Short:                 "List environments",
		Long:                  "List all environments",
		Example:               `vela env list [env-name]`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ListEnvs(args, ioStream)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}
	cmd.SetOut(ioStream.Out)
	return cmd
}

func NewEnvInitCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	var envArgs types.EnvMeta
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "init <envName>",
		DisableFlagsInUseLine: true,
		Short:                 "Create environments",
		Long:                  "Create environment and switch to it",
		Example:               `vela env init test --namespace test`,
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
		Use:                   "delete",
		DisableFlagsInUseLine: true,
		Short:                 "Delete environment",
		Long:                  "Delete environment",
		Example:               `vela env delete test`,
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
		Use:                   "switch",
		Aliases:               []string{"sw"},
		DisableFlagsInUseLine: true,
		Short:                 "Switch environments",
		Long:                  "switch to another environment",
		Example:               `vela env switch test`,
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

func ListEnvs(args []string, ioStreams cmdutil.IOStreams) error {
	table := uitable.New()
	table.MaxColWidth = 60
	table.AddRow("NAME", "CURRENT", "NAMESPACE")
	var envName = ""
	if len(args) > 0 {
		envName = args[0]
	}
	envList, err := oam.ListEnvs(envName)
	if err != nil {
		return err
	}
	for _, env := range envList {
		table.AddRow(env.Name, env.Current, env.Namespace)
	}
	ioStreams.Info(table.String())
	return nil
}

func DeleteEnv(ctx context.Context, args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) < 1 {
		return fmt.Errorf("you must specify env name for vela env delete command")
	}
	envName := args[0]
	msg, err := oam.DeleteEnv(envName)
	if err == nil {
		ioStreams.Info(msg)
	}
	return err
}

func CreateOrUpdateEnv(ctx context.Context, c client.Client, envArgs *types.EnvMeta, args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) < 1 {
		return fmt.Errorf("you must specify env name for vela env init command")
	}
	envName := args[0]
	namespace := envArgs.Namespace
	msg, err := oam.CreateOrUpdateEnv(ctx, c, envName, namespace)
	if err != nil {
		return err
	}
	ioStreams.Info(msg)
	return nil
}

func SwitchEnv(ctx context.Context, args []string, ioStreams cmdutil.IOStreams) error {
	if len(args) < 1 {
		return fmt.Errorf("you must specify env name for vela env command")
	}
	envName := args[0]
	msg, err := oam.SwitchEnv(envName)
	if err != nil {
		return err
	}
	ioStreams.Info(msg)
	return nil
}

func GetEnv(cmd *cobra.Command) (*types.EnvMeta, error) {
	var envName string
	var err error
	if cmd != nil {
		envName = cmd.Flag("env").Value.String()
	}
	if envName != "" {
		return oam.GetEnvByName(envName)
	}
	envName, err = oam.GetCurrentEnvName()
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		if err = system.InitDefaultEnv(); err != nil {
			return nil, err
		}
		envName = types.DefaultEnvName
	}
	return oam.GetEnvByName(envName)
}
