package cmd

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/cloud-native-application/rudrx/api/types"
	"github.com/cloud-native-application/rudrx/pkg/application"
	"github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/cloud-native-application/rudrx/pkg/plugins"

	"cuelang.org/go/cue"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const Staging = "staging"
const App = "app"

type runOptions struct {
	Template     types.Capability
	Env          *types.EnvMeta
	workloadName string
	client       client.Client
	app          *application.Application
	appName      string
	staging      bool
	util.IOStreams
}

func newRunOptions(ioStreams util.IOStreams) *runOptions {
	return &runOptions{IOStreams: ioStreams}
}

func AddWorkloadCommands(parentCmd *cobra.Command, c types.Args, ioStreams util.IOStreams) error {
	templates, err := plugins.LoadInstalledCapabilityWithType(types.TypeWorkload)
	if err != nil {
		return err
	}

	for _, tmp := range templates {
		tmp := tmp

		var name = tmp.Name

		pluginCmd := &cobra.Command{
			Use:                   name + ":run <appname> [args]",
			DisableFlagsInUseLine: true,
			Short:                 "Run " + name + " workloads",
			Long:                  "Run " + name + " workloads",
			Example:               "vela " + name + ":run frontend",
			RunE: func(cmd *cobra.Command, args []string) error {
				o := newRunOptions(ioStreams)
				newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
				if err != nil {
					return err
				}
				o.client = newClient
				o.Env, err = GetEnv(cmd)
				if err != nil {
					return err
				}
				o.Template = tmp
				if err := o.Complete(cmd, args, context.TODO()); err != nil {
					return err
				}
				return o.Run(cmd)
			},
			Annotations: map[string]string{
				types.TagCommandType: types.TypeWorkloads,
			},
		}
		pluginCmd.SetOut(ioStreams.Out)
		for _, v := range tmp.Parameters {
			types.SetFlagBy(pluginCmd, v)
		}
		pluginCmd.Flags().StringP(App, "a", "", "create or add into an existing application group")
		pluginCmd.Flags().BoolP(Staging, "s", false, "only save changes locally without real update application")

		parentCmd.AddCommand(pluginCmd)
	}
	return nil
}

func (o *runOptions) Complete(cmd *cobra.Command, args []string, ctx context.Context) error {
	argsLength := len(args)
	if argsLength < 1 {
		return errors.New("must specify name for workload")
	}
	o.workloadName = args[0]
	if app := cmd.Flag(App).Value.String(); app != "" {
		o.appName = app
	} else {
		o.appName = o.workloadName
	}
	app, err := application.Load(o.Env.Name, o.appName)
	if err != nil {
		return err
	}
	app.Name = o.appName

	if app.Components == nil {
		app.Components = make(map[string]map[string]interface{})
	}
	tp, workloadData, err := app.GetWorkload(o.workloadName)
	if err != nil {
		// Not exist
		tp = o.Template.Name
		workloadData = make(map[string]interface{})
	}

	for _, v := range o.Template.Parameters {
		flagSet := cmd.Flag(v.Name)
		switch v.Type {
		case cue.IntKind:
			d, _ := strconv.ParseInt(flagSet.Value.String(), 10, 64)
			workloadData[v.Name] = d
		case cue.StringKind:
			workloadData[v.Name] = flagSet.Value.String()
		case cue.BoolKind:
			d, _ := strconv.ParseBool(flagSet.Value.String())
			workloadData[v.Name] = d
		case cue.NumberKind, cue.FloatKind:
			d, _ := strconv.ParseFloat(flagSet.Value.String(), 64)
			workloadData[v.Name] = d
		}
	}
	if err = app.SetWorkload(o.workloadName, tp, workloadData); err != nil {
		return err
	}
	o.app = app
	return app.Save(o.Env.Name, o.appName)
}

func (o *runOptions) Run(cmd *cobra.Command) error {
	staging, err := cmd.Flags().GetBool(Staging)
	if err != nil {
		return err
	}
	if staging {
		o.Info("Staging saved")
		return nil
	}
	o.Infof("Creating App %s\n", o.app.Name)
	if err := o.app.Run(context.Background(), o.client, o.Env); err != nil {
		return fmt.Errorf("create app err: %s", err)
	}
	o.Info("SUCCEED")
	return nil
}
