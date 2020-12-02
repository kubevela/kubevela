// nolint:golint
// We will soon deprecate this command, we should use 'vela up' as the only deploy command.
package commands

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/commands/util"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/plugins"
	"github.com/oam-dev/kubevela/pkg/serverlib"
)

// constants used in `svc` command
const (
	Staging      = "staging"
	App          = "app"
	WorkloadType = "type"
	TraitDetach  = "detach"
	Service      = "svc"
)

type runOptions serverlib.RunOptions

func newRunOptions(ioStreams util.IOStreams) *runOptions {
	return &runOptions{IOStreams: ioStreams}
}

// AddCompCommands creates `svc` command and its nested children command
func AddCompCommands(c types.Args, ioStreams util.IOStreams) *cobra.Command {
	compCommands := &cobra.Command{
		Use:                   "svc",
		DisableFlagsInUseLine: true,
		Short:                 "Manage services",
		Long:                  "Manage services",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
	}
	compCommands.PersistentFlags().StringP(App, "a", "", "specify the name of application containing the services")

	compCommands.AddCommand(
		NewCompDeployCommands(c, ioStreams),
	)
	return compCommands
}

// NewCompDeployCommands creates `deploy` command
func NewCompDeployCommands(c types.Args, ioStreams util.IOStreams) *cobra.Command {
	runCmd := &cobra.Command{
		Use:                   "deploy [args]",
		DisableFlagsInUseLine: true,
		// Dynamic flag parse in compeletion
		DisableFlagParsing: true,
		Short:              "Initialize and run a service",
		Long:               "Initialize and run a service. The app name would be the same as service name, if it's not specified.",
		Example:            "vela svc deploy -t <SERVICE_TYPE>",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || args[0] == "-h" {
				err := cmd.Help()
				if err != nil {
					return err
				}
				return nil
			}
			o := newRunOptions(ioStreams)
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			o.KubeClient = newClient
			o.Env, err = GetEnv(cmd)
			if err != nil {
				return err
			}
			if err := o.Complete(cmd, args); err != nil {
				return err
			}
			return o.Run(cmd, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	runCmd.SetOut(ioStreams.Out)

	runCmd.Flags().BoolP(Staging, "s", false, "only save changes locally without real update application")
	runCmd.Flags().StringP(WorkloadType, "t", "", "specify workload type of the service")

	return runCmd
}

// GetWorkloadNameFromArgs gets workload from the args
func GetWorkloadNameFromArgs(args []string) (string, error) {
	argsLength := len(args)
	if argsLength < 1 {
		return "", errors.New("must specify the name of service")
	}
	return args[0], nil

}

func (o *runOptions) Complete(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()
	flags.AddFlagSet(cmd.PersistentFlags())
	flags.ParseErrorsWhitelist.UnknownFlags = true

	// First parse, figure out which workloadType it is.
	if err := flags.Parse(args); err != nil {
		return err
	}

	workloadName, err := GetWorkloadNameFromArgs(flags.Args())
	if err != nil {
		return err
	}

	appName, err := flags.GetString(App)
	if err != nil {
		return err
	}
	workloadType, err := flags.GetString(WorkloadType)
	if err != nil {
		return err
	}
	if workloadType == "" {
		workloads, err := plugins.LoadInstalledCapabilityWithType(types.TypeWorkload)
		if err != nil {
			return err
		}
		var workloadList []string
		for _, w := range workloads {
			workloadList = append(workloadList, w.Name)
		}
		errMsg := "can not find workload, check workloads by `vela workloads` and choose a suitable one."
		if workloadList != nil {
			errMsg = fmt.Sprintf("must specify the workload type of service, please use `-t` and choose from %v.", workloadList)
		}
		return errors.New(errMsg)
	}
	envName := o.Env.Name

	// Dynamic load flags
	template, err := plugins.LoadCapabilityByName(workloadType)
	if err != nil {
		return err
	}
	for _, v := range template.Parameters {
		types.SetFlagBy(flags, v)
	}
	// Second parse, parse parameters of this workload.
	if err = flags.Parse(args); err != nil {
		return err
	}
	app, err := serverlib.BaseComplete(envName, workloadName, appName, flags, workloadType)
	if err != nil {
		return err
	}

	o.App = app
	return err
}

func (o *runOptions) Run(cmd *cobra.Command, io cmdutil.IOStreams) error {
	staging, err := cmd.Flags().GetBool(Staging)
	if err != nil {
		return err
	}
	msg, err := serverlib.BaseRun(staging, o.App, o.KubeClient, o.Env, io)
	if err != nil {
		return err
	}
	o.Info(msg)
	return nil
}
