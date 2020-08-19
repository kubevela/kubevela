package cmd

import (
	"context"
	"errors"

	"github.com/cloud-native-application/rudrx/pkg/oam"

	"github.com/cloud-native-application/rudrx/api/types"
	"github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/cloud-native-application/rudrx/pkg/plugins"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const Staging = "staging"
const App = "app"

type runOptions oam.RunOptions

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
				o.WorkloadType = name
				newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
				if err != nil {
					return err
				}
				o.KubeClient = newClient
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
	workloadName := args[0]
	template := o.Template
	appGroup := cmd.Flag(App).Value.String()

	envName := o.Env.Name
	var flagSet = cmd.Flags()
	app, err := oam.BaseComplete(envName, workloadName, appGroup, flagSet, template)
	o.App = app
	return err
}

func (o *runOptions) Run(cmd *cobra.Command) error {
	staging, err := cmd.Flags().GetBool(Staging)
	if err != nil {
		return err
	}
	msg, err := oam.BaseRun(staging, o.App, o.KubeClient, o.Env)
	if err != nil {
		return err
	}
	o.Info(msg)
	return nil
}
