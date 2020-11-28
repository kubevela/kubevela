package commands

import (
	"errors"

	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/serverlib"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewDeleteCommand Delete App
func NewDeleteCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "delete APP_NAME",
		DisableFlagsInUseLine: true,
		Short:                 "Delete an application",
		Long:                  "Delete an application",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
		Example: "vela delete frontend",
	}
	cmd.SetOut(ioStreams.Out)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
		if err != nil {
			return err
		}
		o := &serverlib.DeleteOptions{}
		o.Client = newClient
		o.Env, err = GetEnv(cmd)
		if err != nil {
			return err
		}
		if len(args) < 1 {
			return errors.New("must specify name for the app")
		}
		o.AppName = args[0]
		svcname, err := cmd.Flags().GetString(Service)
		if err != nil {
			return err
		}
		if svcname == "" {
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
		} else {
			ioStreams.Infof("Deleting Service %s from Application \"%s\"\n", svcname, o.AppName)
			o.CompName = svcname
			message, err := o.DeleteComponent(ioStreams)
			if err != nil {
				return err
			}
			ioStreams.Info(message)
		}
		return nil
	}
	cmd.PersistentFlags().StringP(Service, "", "", "delete only the specified service in this app")
	return cmd
}
