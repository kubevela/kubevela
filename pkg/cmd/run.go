package cmd

import (
	"context"
	"errors"

	"github.com/cloud-native-application/rudrx/pkg/application"

	"github.com/cloud-native-application/rudrx/api/types"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type appRunOptions struct {
	client client.Client
	cmdutil.IOStreams
	Env *types.EnvMeta

	app     *application.Application
	appName string
}

// NewRunCommand run application directly
func NewRunCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	o := &appRunOptions{IOStreams: ioStreams}
	cmd := &cobra.Command{
		Use:                   "run <APPLICATION_BUNDLE_NAME> [args]",
		DisableFlagsInUseLine: true,
		Short:                 "Run a bundle of OAM Applications",
		Long:                  "Run a bundle of OAM Applications",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
		Example: "vela app run myAppBundle",
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			o.client = newClient
			if err != nil {
				return err
			}
			o.Env, err = GetEnv(cmd)
			if err != nil {
				return err
			}
			if err = o.LoadApp(cmd, args); err != nil {
				return err
			}
			return o.Run()
		},
	}
	cmd.Flags().StringP("file", "f", "", "launch application from provided appfile")
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func (o *appRunOptions) LoadApp(cmd *cobra.Command, args []string) error {
	filePath := cmd.Flag("file").Value.String()
	if filePath != "" {
		app, err := application.LoadFromFile(filePath)
		if err != nil {
			return err
		}
		o.appName = app.Name
		o.app = app
		return nil
	}
	if len(args) < 1 {
		return errors.New("must specify name for the app")
	}
	o.appName = args[0]
	app, err := application.Load(o.Env.Name, o.appName)
	if err != nil {
		return err
	}
	o.app = app
	return nil
}

func (o *appRunOptions) Run() error {
	o.Infof("Launching App Bundle \"%s\"\n", o.appName)
	if err := o.app.Run(context.Background(), o.client, o.Env); err != nil {
		return err
	}
	o.Info("SUCCEED")
	return nil
}
