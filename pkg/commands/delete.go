package commands

import (
	"errors"

	"github.com/oam-dev/kubevela/api/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewDeleteCommand Delete App
func NewDeleteCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "delete <APPLICATION_NAME>",
		DisableFlagsInUseLine: true,
		Short:                 "Delete Applications",
		Long:                  "Delete Applications",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
		Example: "vela app delete frontend",
	}
	cmd.SetOut(ioStreams.Out)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
		if err != nil {
			return err
		}
		o := &oam.DeleteOptions{}
		o.Client = newClient
		o.Env, err = GetEnv(cmd)
		if err != nil {
			return err
		}
		if len(args) < 1 {
			return errors.New("must specify name for the app")
		}
		o.AppName = args[0]

		ioStreams.Infof("Deleting Application \"%s\"\n", o.AppName)
		info, err := o.DeleteApp()
		if err != nil {
			if apierrors.IsNotFound(err) {
				ioStreams.Info("Already deleted")
				return nil
			}
			return err
		}
		ioStreams.Info(info)
		return nil
	}
	return cmd
}

// NewCompDeleteCommand delete component
func NewCompDeleteCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "delete <SERVICE_NAME>",
		DisableFlagsInUseLine: true,
		Short:                 "Delete a service from an application",
		Long:                  "Delete a service from an application",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
		Example: "vela svc delete frontend",
	}
	cmd.SetOut(ioStreams.Out)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
		if err != nil {
			return err
		}
		o := &oam.DeleteOptions{}
		o.Client = newClient
		o.Env, err = GetEnv(cmd)
		if err != nil {
			return err
		}
		if len(args) < 1 {
			return errors.New("must specify the service name")
		}
		o.CompName = args[0]
		appName, err := cmd.Flags().GetString(App)
		if err != nil {
			return err
		}
		if appName != "" {
			o.AppName = appName
		} else {
			o.AppName = o.CompName
		}

		ioStreams.Infof("Deleting service '%s' from Application '%s'\n", o.CompName, o.AppName)
		message, err := o.DeleteComponent(ioStreams)
		if err != nil {
			return err
		}
		ioStreams.Info(message)
		return nil
	}
	return cmd
}
