package cmd

import (
	"context"
	"os"
	"time"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/cloud-native-application/rudrx/pkg/application"

	"github.com/cloud-native-application/rudrx/api/types"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewCompStatusCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:     "status <APPLICATION-NAME>",
		Short:   "get status of an application",
		Long:    "get status of an application, including its workload and trait",
		Example: `vela status <APPLICATION-NAME>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsLength := len(args)
			if argsLength == 0 {
				ioStreams.Errorf("Hint: please specify an application")
				os.Exit(1)
			}
			compName := args[0]
			env, err := GetEnv(cmd)
			if err != nil {
				ioStreams.Errorf("Error: failed to get Env: %s", err)
				return err
			}
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			appName, _ := cmd.Flags().GetString(App)
			return printComponentStatus(ctx, newClient, ioStreams, compName, appName, env)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func printComponentStatus(ctx context.Context, c client.Client, ioStreams cmdutil.IOStreams, compName, appName string, env *types.EnvMeta) error {
	ioStreams.Infof("Showing status of Component %s deployed in Environment %s\n", compName, env.Name)
	var app *application.Application
	var err error
	if appName != "" {
		app, err = application.Load(env.Name, appName)
	} else {
		app, err = application.MatchAppByComp(env.Name, compName)
	}
	if err != nil {
		return err
	}

	var health v1alpha2.HealthScope
	if err = c.Get(ctx, client.ObjectKey{Namespace: env.Namespace, Name: application.FormatDefaultHealthScopeName(app.Name)}, &health); err != nil {
		return err
	}
	ioStreams.Info("Component Status:")
	//TODO(wonderflow): add more information from health scope
	ioStreams.Infof("\n   %s \n\n", health.Status.Health)

	var appConfig v1alpha2.ApplicationConfiguration
	if err = c.Get(ctx, client.ObjectKey{Namespace: env.Namespace, Name: app.Name}, &appConfig); err != nil {
		return err
	}
	ioStreams.Infof("Last Deployment:\n\n")
	ioStreams.Infof("\tCreated at:\t%v\n", appConfig.CreationTimestamp)
	ioStreams.Infof("\tUpdated at:\t%v\n", app.UpdateTime.Format(time.RFC3339))
	return nil
}
