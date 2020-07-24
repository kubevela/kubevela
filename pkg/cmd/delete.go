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
		Use:                   "delete [WORKLOAD_KIND] [WORKLOAD_NAME]",
		DisableFlagsInUseLine: true,
		Short:                 "Delete OAM workloads",
		Long:                  "Delete OAM workloads",
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

//runSubDeleteCommand is init a new command and run independent
func runSubDeleteCommand(parentCmd *cobra.Command, f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams, args []string) error {
	ctx := context.Background()
	workloadNames := []string{}
	o := newDeleteOptions(ioStreams)
	o.client = c
	deleteCommand := newDeleteCommand()
	deleteCommand.SilenceUsage = true
	deleteCommand.SilenceErrors = true
	deleteCommand.DisableAutoGenTag = true
	deleteCommand.DisableFlagsInUseLine = true
	deleteCommand.DisableSuggestions = true

	// set args from parent
	if len(args) > 0 {
		deleteCommand.SetArgs(args)
	} else {
		deleteCommand.SetArgs([]string{})
	}
	deleteCommand.SetOutput(o.Out)
	deleteCommand.RunE = func(cmd *cobra.Command, args []string) error {
		return errors.New("You must specify a workload, like " + strings.Join(workloadNames, ", ") +
			"\nSee 'rudrx delete -h' for help and examples")
	}
	deleteCommand.PersistentFlags().StringP("namespace", "n", "", "namespace for apps")

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
			Use:                   name + " [WORKLOAD_NAME]",
			DisableFlagsInUseLine: true,
			Short:                 "Delete " + name + " workloads",
			Long:                  "Delete " + name + " workloads",
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := o.Complete(f, cmd, args); err != nil {
					return err
				}
				return o.Delete()
			},
		}
		subcmd.SetOutput(o.Out)

		deleteCommand.AddCommand(subcmd)
	}
	return deleteCommand.Execute()
}

func (o *deleteOptions) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string) error {
	namespace, _, err := f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	if len(args) < 1 {
		return errors.New("must specify name for workload")
	}

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

func (o *deleteOptions) Delete() error {
	o.Infof("Deleting AppConfig \"%s\"\n", o.AppConfig.Name)
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
