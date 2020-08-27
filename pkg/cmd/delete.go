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
	appName  string
	compName string
	client   client.Client
	cmdutil.IOStreams
	Env *types.EnvMeta
}

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
		o := &deleteOptions{IOStreams: ioStreams}
		o.client = newClient
		o.Env, err = GetEnv(cmd)
		if err != nil {
			return err
		}
		if len(args) < 1 {
			return errors.New("must specify name for the app")
		}
		o.appName = args[0]

		return o.DeleteApp()
	}
	return cmd
}

func (o *deleteOptions) DeleteApp() error {
	o.Infof("Deleting Application \"%s\"\n", o.appName)
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
		err = o.client.Delete(ctx, &c)
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete component err: %s", err)
		}
	}
	err = o.client.Delete(ctx, &appConfig)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete appconfig err %s", err)
	}
	o.Info("DELETE SUCCEED")
	return nil
}

// NewCompDeleteCommand delete component
func NewCompDeleteCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "delete <ComponentName>",
		DisableFlagsInUseLine: true,
		Short:                 "Delete Component From Application",
		Long:                  "Delete Component From Application",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
		Example: "vela comp delete frontend",
	}
	cmd.SetOut(ioStreams.Out)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
		if err != nil {
			return err
		}
		o := &deleteOptions{IOStreams: ioStreams}
		o.client = newClient
		o.Env, err = GetEnv(cmd)
		if err != nil {
			return err
		}
		if len(args) < 1 {
			return errors.New("must specify name for the app")
		}
		o.compName = args[0]
		appName, err := cmd.Flags().GetString(App)
		if err != nil {
			return err
		}
		o.appName = appName
		return o.DeleteComponent()
	}
	return cmd
}

func (o *deleteOptions) DeleteComponent() error {
	var app *application.Application
	var err error
	if o.appName != "" {
		app, err = application.Load(o.Env.Name, o.appName)
	} else {
		app, err = application.MatchAppByComp(o.Env.Name, o.compName)
	}
	if err != nil {
		return err
	}

	if len(app.GetComponents()) <= 1 {
		return o.DeleteApp()
	}

	o.Infof("Deleting Component '%s' from Application '%s'\n", o.compName, o.appName)

	// Remove component from local appfile
	if err := app.RemoveComponent(o.compName); err != nil {
		return err
	}
	if err := app.Save(o.Env.Name); err != nil {
		return err
	}

	// Remove component from appConfig in k8s cluster
	ctx := context.Background()
	if err := app.Run(ctx, o.client, o.Env); err != nil {
		return err
	}

	// Remove component in k8s cluster
	var c corev1alpha2.Component
	c.Name = o.compName
	c.Namespace = o.Env.Namespace
	err = o.client.Delete(context.Background(), &c)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete component err: %s", err)
	}

	o.Info("DELETE SUCCEED")
	return nil
}
