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
		Use:                   "app:delete <APPLICATION_NAME>",
		Aliases:               []string{"delete"},
		DisableFlagsInUseLine: true,
		Short:                 "Delete OAM Applications",
		Long:                  "Delete OAM Applications",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
		Example: "vela delete frontend"}
}

// NewDeleteCommand init new command
func NewDeleteCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := newDeleteCommand()
	cmd.SetOut(ioStreams.Out)
	o := newDeleteOptions(ioStreams)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
		if err != nil {
			return err
		}
		o.client = newClient
		o.Env, err = GetEnv(cmd)
		if err != nil {
			return err
		}

		if err := o.Complete(cmd, args); err != nil {
			return err
		}
		return o.Delete()
	}
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		ctx := context.Background()
		env, err := GetEnv(cmd)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return compListApplication(ctx, newClient, "", env.Namespace)
	}
	return cmd
}

func (o *deleteOptions) Complete(cmd *cobra.Command, args []string) error {

	if len(args) < 1 {
		return errors.New("must specify name for the app")
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
