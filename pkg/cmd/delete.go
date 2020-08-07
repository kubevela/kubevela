package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloud-native-application/rudrx/api/types"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type deleteOptions struct {
	Component corev1alpha2.Component
	AppConfig corev1alpha2.ApplicationConfiguration
	client    client.Client
	cmdutil.IOStreams
	Env *types.EnvMeta
}

func newDeleteOptions(ioStreams cmdutil.IOStreams) *deleteOptions {
	return &deleteOptions{IOStreams: ioStreams}
}

func newDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:                   "app:delete [APPLICATION_NAME]",
		DisableFlagsInUseLine: true,
		Short:                 "Delete OAM Applications",
		Long:                  "Delete OAM Applications",
		Example: `
  vela delete frontend
`}
}

// NewDeleteCommand init new command
func NewDeleteCommand(c client.Client, ioStreams cmdutil.IOStreams, args []string) *cobra.Command {
	cmd := newDeleteCommand()
	cmd.SetArgs(args)
	cmd.SetOut(ioStreams.Out)
	o := newDeleteOptions(ioStreams)
	o.client = c
	o.Env, _ = GetEnv()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.Complete(cmd, args); err != nil {
			return err
		}
		return o.Delete()
	}
	return cmd
}

func (o *deleteOptions) Complete(cmd *cobra.Command, args []string) error {

	if len(args) < 1 {
		return errors.New("must specify name for workload")
	}

	namespace := o.Env.Namespace

	o.Component.Name = args[0]
	o.Component.Namespace = namespace
	o.AppConfig.Name = args[0]
	o.AppConfig.Namespace = namespace
	return nil
}

func (o *deleteOptions) Delete() error {
	o.Infof("Deleting AppConfig \"%s\"\n", o.AppConfig.Name)
	err := o.client.Delete(context.Background(), &o.AppConfig)
	if err != nil {
		return fmt.Errorf("delete appconfig err %s", err)
	}
	err = o.client.Delete(context.Background(), &o.Component)
	if err != nil {
		return fmt.Errorf("delete component err: %s", err)
	}
	o.Info("DELETE SUCCEED")
	return nil
}
