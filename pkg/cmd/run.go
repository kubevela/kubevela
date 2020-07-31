package cmd

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
)

type runOptions struct {
	Template  cmdutil.Template
	Env       *EnvMeta
	Component corev1alpha2.Component
	AppConfig corev1alpha2.ApplicationConfiguration
	client    client.Client
	cmdutil.IOStreams
}

func newRunOptions(ioStreams cmdutil.IOStreams) *runOptions {
	return &runOptions{IOStreams: ioStreams}
}

// NewRunCommand init new command
func NewRunCommand(f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams, args []string) *cobra.Command {
	cmd := newRunCommand()
	// flags pass to new command directly
	cmd.DisableFlagParsing = true
	cmd.SetArgs(args)
	cmd.SetOut(ioStreams.Out)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runSubRunCommand(cmd, f, c, ioStreams, args)
	}
	return cmd
}

// runSubRunCommand is init a new command and run independent
func runSubRunCommand(parentCmd *cobra.Command, f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams, args []string) error {
	ctx := context.Background()
	workloadNames := []string{}
	o := newRunOptions(ioStreams)
	o.client = c
	o.Env, _ = GetEnv()

	// init fake command and pass args to fake command
	// flags and subcommand append to fake comand and parent command
	// run fake command only, show tips in parent command only
	fakeCommand := newRunCommand()
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
	fakeCommand.SetOut(o.Out)
	fakeCommand.RunE = func(cmd *cobra.Command, args []string) error {
		return errors.New("You must specify a workload, like " + strings.Join(workloadNames, ", ") +
			"\nSee 'rudr run -h' for help and examples")
	}
	fakeCommand.PersistentFlags().StringP("namespace", "n", "default", "namespace for apps")
	parentCmd.PersistentFlags().StringP("namespace", "n", "default", "namespace for apps")

	var workloadDefs corev1alpha2.WorkloadDefinitionList
	err := c.List(ctx, &workloadDefs)
	if err != nil {
		return fmt.Errorf("listing Workload definition hit an issue: %s", err)
	}

	for _, wd := range workloadDefs.Items {
		var tmp cmdutil.Template
		tmp, err := cmdutil.ConvertTemplateJson2Object(wd.Spec.Extension)
		if err != nil {
			fmt.Printf("extract template from traitDefinition %v err: %v, ignore it\n", wd.Name, err)
			continue
		}
		name := tmp.Alias
		workloadNames = append(workloadNames, name)

		subcmd := &cobra.Command{
			Use:                   name + " [args]",
			DisableFlagsInUseLine: true,
			Short:                 "Run " + name + " workloads",
			Long:                  "Run " + name + " workloads",
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := o.Complete(f, cmd, args, ctx); err != nil {
					return err
				}
				return o.Run(f, cmd)
			},
		}
		subcmd.SetOut(o.Out)
		for _, v := range tmp.Parameters {
			if tmp.LastCommandParam != v.Name {
				subcmd.PersistentFlags().StringP(v.Name, v.Short, v.Default, v.Usage)
			}
		}

		// Comment this line as template content will get mixed when there are more than two WorkloadDefinitions
		// tmp.DeepCopyInto(&o.Template)
		o.Template = tmp
		fakeCommand.AddCommand(subcmd)
		parentCmd.AddCommand(subcmd)
	}

	return fakeCommand.Execute()
}

func (o *runOptions) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string, ctx context.Context) error {

	argsLength := len(args)
	lastCommandParam := o.Template.LastCommandParam

	if argsLength < 1 {
		return errors.New("must specify name for workload")
	} else if argsLength >= 1 {
		workloadName := args[0]

		switch {
		case argsLength < 2 && lastCommandParam != "":
			// TODO(zzxwill): Could not determine whether the argument is the workload name or image name if without image tag
			return fmt.Errorf("You must specify `%s` as the last command.\nSee 'rudr run -h' for help and examples",
				lastCommandParam)
		case argsLength == 2:
			workloadTemplate := o.Template
			pvd := fieldpath.Pave(workloadTemplate.Object.Object)
			for _, v := range workloadTemplate.Parameters {
				lastCommandValue := args[argsLength-1]
				var paraV string
				if v.Name == lastCommandParam {
					paraV = lastCommandValue
				} else {
					flagSet := cmd.Flag(v.Name)
					paraV = flagSet.Value.String()
				}

				if paraV == "" {
					return fmt.Errorf("Flag `%s` is NOT set, please check and try again. \nSee 'rudr run -h' for help and examples", v.Name)
				}

				for _, path := range v.FieldPaths {
					if v.Type == "int" {
						portValue, _ := strconv.ParseFloat(paraV, 64)
						pvd.SetNumber(path, portValue)
						break
					}
					pvd.SetString(path, paraV)
				}
			}

			pvd.SetString("metadata.name", strings.ToLower(workloadName))
			namespace := o.Env.Namespace
			o.Component.Spec.Workload.Object = &unstructured.Unstructured{Object: pvd.UnstructuredContent()}
			o.Component.Name = args[0]
			o.Component.Namespace = namespace
			o.AppConfig.Name = args[0]
			o.AppConfig.Namespace = namespace
			o.AppConfig.Spec.Components = append(o.AppConfig.Spec.Components, corev1alpha2.ApplicationConfigurationComponent{ComponentName: args[0]})

		case argsLength > 2:
			return fmt.Errorf("there are more commands than needed, please try again")
		}
	}
	return nil
}

func (o *runOptions) Run(f cmdutil.Factory, cmd *cobra.Command) error {
	o.Infof("Creating AppConfig %s\n", o.AppConfig.Name)
	err := o.client.Create(context.Background(), &o.Component)
	if err != nil {
		return fmt.Errorf("create component err: %s", err)
	}
	err = o.client.Create(context.Background(), &o.AppConfig)
	if err != nil {
		return fmt.Errorf("create appconfig err %s", err)
	}
	o.Info("SUCCEED")
	return nil
}

func newRunCommand() *cobra.Command {
	return &cobra.Command{
		Use:                   "run [WORKLOAD_KIND] [args]",
		DisableFlagsInUseLine: true,
		Short:                 "Run OAM workloads",
		Long:                  "Create and Run one Workload one AppConfig OAM APP",
		Example: `
  rudr run containerized frontend -p 80 oam-dev/demo:v1
`}
}
