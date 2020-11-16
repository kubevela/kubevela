package commands

import (
	"context"
	"errors"
	"fmt"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/application"
	"github.com/oam-dev/kubevela/pkg/commands/util"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/plugins"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type commandOptions struct {
	Template types.Capability
	Client   client.Client
	Detach   bool
	Env      *types.EnvMeta

	workloadName string
	appName      string
	app          *application.Application
	traitType    string
	cmdutil.IOStreams
}

func AddTraitCommands(parentCmd *cobra.Command, c types.Args, ioStreams cmdutil.IOStreams) error {
	templates, err := plugins.LoadInstalledCapabilityWithType(types.TypeTrait)
	if err != nil {
		return err
	}
	ctx := context.Background()
	for _, tmp := range templates {
		tmp := tmp

		var name = tmp.Name
		pluginCmd := &cobra.Command{
			// We can't hide these command, if so, cli docs will also disappear from auto-gen
			// Hidden:                true,
			Use:                   name + " <appname> [args]",
			DisableFlagsInUseLine: true,
			Short:                 "Attach " + name + " trait to an app",
			Long:                  "Attach " + name + " trait to an app",
			Example:               "vela " + name + " frontend",
			RunE: func(cmd *cobra.Command, args []string) error {
				o := &commandOptions{IOStreams: ioStreams, traitType: name}
				o.Template = tmp
				newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
				if err != nil {
					return err
				}
				o.Client = newClient
				o.Env, err = GetEnv(cmd)
				if err != nil {
					return err
				}
				detach, _ := cmd.Flags().GetBool(TraitDetach)
				if detach {
					if err := o.DetachTrait(cmd, args); err != nil {
						return err
					}
					o.Detach = true
				} else {
					if err := o.AddOrUpdateTrait(cmd, args); err != nil {
						return err
					}
				}
				return o.Run(ctx, cmd, ioStreams)
			},
		}
		pluginCmd.SetOut(ioStreams.Out)
		for _, v := range tmp.Parameters {
			types.SetFlagBy(pluginCmd.Flags(), v)
		}
		pluginCmd.Flags().StringP(Service, "", "", "specify one service belonging to the application")
		pluginCmd.Flags().BoolP(Staging, "s", false, "only save changes locally without real update application")
		pluginCmd.Flags().BoolP(TraitDetach, "", false, "detach trait from service")

		parentCmd.AddCommand(pluginCmd)
	}
	return nil
}

func (o *commandOptions) Prepare(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return errors.New("please specify the name of the app")
	}
	o.appName = args[0]
	// get application
	app, err := application.Load(o.Env.Name, o.appName)
	if err != nil {
		return err
	}
	if len(app.Name) == 0 {
		return fmt.Errorf("the application %s doesn't exist in current env %s", o.appName, o.Env.Name)
	}

	// get service name
	serviceNames := app.GetComponents()
	if svcName := cmd.Flag(Service).Value.String(); svcName != "" {
		for _, v := range serviceNames {
			if v == svcName {
				o.workloadName = svcName
				return nil
			}
		}
		return fmt.Errorf("the service %s doesn't exist in the application %s", svcName, o.appName)
	}
	svcName, err := util.AskToChooseOneService(serviceNames)
	if err != nil {
		return err
	}
	o.workloadName = svcName
	return nil
}

func (o *commandOptions) AddOrUpdateTrait(cmd *cobra.Command, args []string) error {
	var err error
	if err = o.Prepare(cmd, args); err != nil {
		return err
	}
	flags := cmd.Flags()

	if o.app, err = oam.AddOrUpdateTrait(o.Env, o.appName, o.workloadName, flags, o.Template); err != nil {
		return err
	}
	return nil
}

func (o *commandOptions) DetachTrait(cmd *cobra.Command, args []string) error {
	var err error
	if err = o.Prepare(cmd, args); err != nil {
		return err
	}
	if o.app, err = oam.PrepareDetachTrait(o.Env.Name, o.traitType, o.workloadName, o.appName); err != nil {
		return err
	}
	var traitType = o.Template.Name
	if err = o.app.RemoveTrait(o.workloadName, traitType); err != nil {
		return err
	}
	return o.app.Save(o.Env.Name)
}

func (o *commandOptions) Run(ctx context.Context, cmd *cobra.Command, io cmdutil.IOStreams) error {
	if o.Detach {
		o.Infof("Detaching %s from app %s\n", o.traitType, o.workloadName)
	} else {
		o.Infof("Adding %s for app %s \n", o.Template.Name, o.workloadName)
	}
	staging, err := cmd.Flags().GetBool(Staging)
	if err != nil {
		return err
	}
	_, err = oam.TraitOperationRun(ctx, o.Client, o.Env, o.app, staging, io)
	if err != nil {
		return err
	}
	deployStatus, err := printTrackingDeployStatus(ctx, o.Client, o.IOStreams, o.workloadName, o.appName, o.Env)
	if err != nil {
		return err
	}
	if deployStatus != compStatusDeployed {
		return nil
	}

	return printComponentStatus(context.Background(), o.Client, o.IOStreams, o.workloadName, o.appName, o.Env)
}
