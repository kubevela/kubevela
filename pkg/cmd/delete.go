package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/cloud-native-application/rudrx/pkg/application"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/cloud-native-application/rudrx/api/types"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type deleteOptions struct {
	appName string
	client  client.Client
	cmdutil.IOStreams
	Env *types.EnvMeta
}

func newDeleteOptions(ioStreams cmdutil.IOStreams) *deleteOptions {
	return &deleteOptions{IOStreams: ioStreams}
}

func newDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:                   "delete <APPLICATION_NAME>",
		DisableFlagsInUseLine: true,
		Short:                 "Delete Applications",
		Long:                  "Delete Applications",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
		Example: "vela app delete frontend"}
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
		if len(args) < 1 {
			return errors.New("must specify name for the app")
		}
		o.appName = args[0]

		return o.Delete()
	}
	return cmd
}

func (o *deleteOptions) Delete() error {
	o.Infof("Deleting AppConfig \"%s\"\n", o.appName)
	if err := application.Delete(o.Env.Name, o.appName); err != nil && !os.IsNotExist(err) {
		return err
	}
	ctx := context.Background()
	var appConfig corev1alpha2.ApplicationConfiguration
	err := o.client.Get(ctx, client.ObjectKey{Name: o.appName, Namespace: o.Env.Namespace}, &appConfig)
	if err != nil {
		if apierrors.IsNotFound(err) {
			o.Info("Already deleted")
			return nil
		}
		return fmt.Errorf("delete appconfig err %s", err)
	}
	for _, comp := range appConfig.Status.Workloads {
		var c corev1alpha2.Component
		c.Name = comp.ComponentName
		c.Namespace = o.Env.Namespace
		err = o.client.Delete(context.Background(), &c)
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete component err: %s", err)
		}
	}
	err = o.client.Delete(context.Background(), &appConfig)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete appconfig err %s", err)
	}
	o.Info("DELETE SUCCEED")
	return nil
}
