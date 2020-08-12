package cmd

import (
	"context"
	"errors"
	"strconv"

	"github.com/cloud-native-application/rudrx/pkg/application"

	"cuelang.org/go/cue"

	"github.com/cloud-native-application/rudrx/pkg/plugins"

	"github.com/cloud-native-application/rudrx/api/types"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type commandOptions struct {
	Template   types.Capability
	Client     client.Client
	TraitAlias string
	Detach     bool
	Env        *types.EnvMeta

	workloadName string
	appName      string
	staging      bool
	app          *application.Application
	cmdutil.IOStreams
}

func NewCommandOptions(ioStreams cmdutil.IOStreams) *commandOptions {
	return &commandOptions{IOStreams: ioStreams}
}

func AddTraitCommands(parentCmd *cobra.Command, c types.Args, ioStreams cmdutil.IOStreams) error {
	templates, err := plugins.LoadInstalledCapabilityWithType(types.TypeTrait)
	if err != nil {
		return err
	}
	ctx := context.Background()
	for _, tmp := range templates {
		var name = tmp.Name
		o := NewCommandOptions(ioStreams)
		o.Env, _ = GetEnv()
		pluginCmd := &cobra.Command{
			Use:                   name + " <appname> [args]",
			DisableFlagsInUseLine: true,
			Short:                 "Attach " + name + " trait to an app",
			Long:                  "Attach " + name + " trait to an app",
			Example:               "vela " + name + " frontend",
			RunE: func(cmd *cobra.Command, args []string) error {
				newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
				if err != nil {
					return err
				}
				o.Client = newClient
				if err := o.AddOrUpdateTrait(cmd, args); err != nil {
					return err
				}
				return o.Run(cmd, ctx)
			},
			Annotations: map[string]string{
				types.TagCommandType: types.TypeTraits,
			},
		}
		pluginCmd.SetOut(o.Out)
		for _, v := range tmp.Parameters {
			types.SetFlagBy(pluginCmd, v)
		}
		pluginCmd.Flags().StringP(App, "a", "", "create or add into an existing application group")
		pluginCmd.Flags().BoolP(Staging, "s", false, "only save changes locally without real update application")

		o.Template = tmp
		parentCmd.AddCommand(pluginCmd)
	}
	return nil
}

func (o *commandOptions) Prepare(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return errors.New("please specify the name of the app")
	}
	o.workloadName = args[0]
	if app := cmd.Flag(App).Value.String(); app != "" {
		o.appName = app
	} else {
		o.appName = o.workloadName
	}
	return nil
}

func (o *commandOptions) AddOrUpdateTrait(cmd *cobra.Command, args []string) error {
	if err := o.Prepare(cmd, args); err != nil {
		return err
	}
	app, err := application.Load(o.Env.Name, o.appName)
	if err != nil {
		return err
	}
	var traitType = o.Template.Name
	traitData, err := app.GetTraitsByType(o.workloadName, traitType)
	if err != nil {
		return err
	}
	for _, v := range o.Template.Parameters {
		flagSet := cmd.Flag(v.Name)
		switch v.Type {
		case cue.IntKind:
			d, _ := strconv.ParseInt(flagSet.Value.String(), 10, 64)
			traitData[v.Name] = d
		case cue.StringKind:
			traitData[v.Name] = flagSet.Value.String()
		case cue.BoolKind:
			d, _ := strconv.ParseBool(flagSet.Value.String())
			traitData[v.Name] = d
		case cue.NumberKind, cue.FloatKind:
			d, _ := strconv.ParseFloat(flagSet.Value.String(), 64)
			traitData[v.Name] = d
		}
	}
	if err = app.SetTrait(o.workloadName, traitType, traitData); err != nil {
		return err
	}
	o.app = app
	return app.Save(o.Env.Name, o.appName)
}

func AddTraitDetachCommands(parentCmd *cobra.Command, c types.Args, ioStreams cmdutil.IOStreams) error {
	templates, err := plugins.LoadInstalledCapabilityWithType(types.TypeTrait)
	if err != nil {
		return err
	}
	ctx := context.Background()
	for _, tmp := range templates {
		var name = tmp.Name
		o := NewCommandOptions(ioStreams)
		o.Env, _ = GetEnv()
		pluginCmd := &cobra.Command{
			Use:                   name + ":detach <appname>",
			DisableFlagsInUseLine: true,
			Short:                 "Detach " + name + " trait from an app",
			Long:                  "Detach " + name + " trait from an app",
			Example:               "vela " + name + ":detach frontend",
			RunE: func(cmd *cobra.Command, args []string) error {
				newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
				if err != nil {
					return err
				}
				o.Client = newClient
				if err := o.DetachTrait(cmd, args); err != nil {
					return err
				}
				return o.Run(cmd, ctx)
			},
			Annotations: map[string]string{
				types.TagCommandType: types.TypeTraits,
			},
		}
		pluginCmd.SetOut(o.Out)
		o.TraitAlias = name
		o.Detach = true
		parentCmd.AddCommand(pluginCmd)
	}
	return nil
}

func (o *commandOptions) DetachTrait(cmd *cobra.Command, args []string) error {
	if err := o.Prepare(cmd, args); err != nil {
		return err
	}
	app, err := application.Load(o.Env.Name, o.appName)
	if err != nil {
		return err
	}
	var traitType = o.Template.Name
	if err = app.RemoveTrait(o.workloadName, traitType); err != nil {
		return err
	}
	return app.Save(o.Env.Name, o.appName)
}

func (o *commandOptions) Run(cmd *cobra.Command, ctx context.Context) error {
	if o.Detach {
		o.Infof("Detaching %s from app %s\n", o.Template.Name, o.workloadName)
	} else {
		o.Infof("Adding %s for app %s \n", o.Template.Name, o.workloadName)
	}
	staging, err := strconv.ParseBool(cmd.Flag(Staging).Value.String())
	if err != nil {
		return err
	}
	if staging {
		o.Info("Staging saved")
		return nil
	}
	err = o.app.Run(ctx, o.Client, o.Env)
	if err != nil {
		return err
	}
	o.Info("Succeeded!")
	return nil
}
