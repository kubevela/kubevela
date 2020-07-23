package cmd

import (
	"context"
	"errors"
	"fmt"
	"github.com/cloud-native-application/rudrx/api/v1alpha2"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

type deleteOptions struct {
	Namespace string
	Template  v1alpha2.Template
	Component corev1alpha2.Component
	AppConfig corev1alpha2.ApplicationConfiguration
	client    client.Client
	cmdutil.IOStreams
}

func newDeleteOptions(ioStreams cmdutil.IOStreams) *deleteOptions {
	return &deleteOptions{IOStreams: ioStreams}
}

func newDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:                   "delete [WORKLOAD_KIND] [args]",
		DisableFlagsInUseLine: true,
		Short:                 "Delete OAM workloads",
		Long:                  "Delete OAM APP",
		Example: `
  rudrx delete containerized frontend
`}
}


// NewDeleteCommand init new command
func NewDeleteCommand(f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams, args []string) *cobra.Command {
	cmd := newDeleteCommand()
	// flags pass to new command directly
	cmd.DisableFlagParsing = true
	cmd.SetArgs(args)
	cmd.SetOut(ioStreams.Out)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runSubDeleteCommand(cmd, f, c, ioStreams, args)
	}
	return cmd
}

// runSubDeleteCommand is init a new command and run independent
func runSubDeleteCommand(parentCmd *cobra.Command, f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams, args []string) error {
	ctx := context.Background()
	workloadNames := []string{}
	o := newDeleteOptions(ioStreams)
	o.client = c
	// init fake command and pass args to fake command
	// flags and subcommand append to fake comand and parent command
	// run fake command only, show tips in parent command only
	fakeCommand := newDeleteCommand()
	fakeCommand.SilenceUsage = true
	fakeCommand.SilenceErrors = true
	fakeCommand.DisableAutoGenTag = true
	fakeCommand.DisableFlagsInUseLine = true
	fakeCommand.DisableSuggestions = true

	// set args from parent
	if len(args) > 0 {
		fakeCommand.SetArgs(args)
	} else {
		fakeCommand.SetArgs([]string{})
	}
	fakeCommand.SetOutput(o.Out)
	fakeCommand.RunE = func(cmd *cobra.Command, args []string) error {
		return errors.New("You must specify a workload, like " + strings.Join(workloadNames, ", ") +
			"\nSee 'rudr delete -h' for help and examples")
	}
	fakeCommand.PersistentFlags().StringP("namespace", "n", "default", "namespace for apps")
	parentCmd.PersistentFlags().StringP("namespace", "n", "default", "namespace for apps")

	var workloadDefs corev1alpha2.WorkloadDefinitionList
	err := c.List(ctx, &workloadDefs)
	if err != nil {
		return fmt.Errorf("list workload Definition err %s", err)
	}
	workloadDefsItem := workloadDefs.Items

	for _, wd := range workloadDefsItem {
		name := wd.ObjectMeta.Annotations["short"]
		if name == "" {
			name = wd.Name
		}
		workloadNames = append(workloadNames, name)
		templateRef, ok := wd.ObjectMeta.Annotations[TemplateLabel]
		if !ok {
			continue
		}

		var tmp v1alpha2.Template
		err = c.Get(ctx, client.ObjectKey{Namespace: "default", Name: templateRef}, &tmp)
		if err != nil {
			return fmt.Errorf("get workload Definition err: %v", err)
		}

		subcmd := &cobra.Command{
			Use:                   name + " [args]",
			DisableFlagsInUseLine: true,
			Short:                 "Delete " + name + " workloads",
			Long:                  "Delete " + name + " workloads",
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := o.Complete(f, cmd, args); err != nil {
					return err
				}
				return o.Delete(f, cmd)
			},
		}
		subcmd.SetOutput(o.Out)

		tmp.DeepCopyInto(&o.Template)
		fakeCommand.AddCommand(subcmd)
		parentCmd.AddCommand(subcmd)
	}
	return fakeCommand.Execute()
}

func (o *deleteOptions) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string) error {
	namespace, explicitNamespace, err := f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	} else if !explicitNamespace {
		namespace = "default"
	}

	argsLenght := len(args)
	if argsLenght < 1 {
		return errors.New("must specify name for workload")
	}

	o.Namespace = namespace
	namespaceCover := cmd.Flag("namespace").Value.String()
	if namespaceCover != "" {
		namespace = namespaceCover
	}
	o.Component.Name = args[0]
	o.Component.Namespace = namespace
	o.AppConfig.Name = args[0]
	o.AppConfig.Namespace = namespace
	return nil
}

func (o *deleteOptions) Delete(f cmdutil.Factory, cmd *cobra.Command) error {
	o.Infof("Deleting AppConfig %s\n", o.AppConfig.Name)
	err := o.client.Delete(context.Background(), &o.Component)
	if err != nil {
		return fmt.Errorf("delete component err: %s", err)
	}
	err = o.client.Delete(context.Background(), &o.AppConfig)
	if err != nil {
		return fmt.Errorf("delete appconfig err %s", err)
	}
	o.Info("DELETE SUCCEED")
	return nil
}