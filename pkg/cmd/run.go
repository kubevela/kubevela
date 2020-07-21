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

	"github.com/cloud-native-application/rudrx/api/v1alpha2"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
)

type runOptions struct {
	Namespace string
	Template  v1alpha2.Template
	Component corev1alpha2.Component
	AppConfig corev1alpha2.ApplicationConfiguration
	client    client.Client
	cmdutil.IOStreams
}

func newRunOptions(ioStreams cmdutil.IOStreams) *runOptions {
	return &runOptions{IOStreams: ioStreams}
}

func NewRunCommand(f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams, args []string) *cobra.Command {
	cmd := newRunCommand()
	cmd.SetArgs(args)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runSubRunCommand(f, c, ioStreams, args)
	}
	return cmd
}

func runSubRunCommand(f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams, args []string) error {
	ctx := context.Background()
	workloadNames := []string{}
	o := newRunOptions(ioStreams)
	o.client = c

	// init new cmd, append sub cmd
	runCmd := newRunCommand()
	runCmd.SetOutput(o.Out)
	runCmd.PersistentFlags().StringP("namespace", "n", "default", "namespace for apps")
	if len(args) > 0 {
		runCmd.SetArgs(args[1:])
	}

	var workloadDefs corev1alpha2.WorkloadDefinitionList
	err := c.List(ctx, &workloadDefs)
	if err != nil {
		return fmt.Errorf("list workload Definition err %s", err)
	}
	workloadDefsItem := workloadDefs.Items
	if len(workloadDefsItem) == 0 {
		// TODO(zzxwill) Refine this prompt message
		return errors.New("somehow Workload definitions are NOT preconfigured, please report this to OAM maintainers")
	}

	for _, wd := range workloadDefsItem {
		name := wd.ObjectMeta.Annotations["short"]
		if name == "" {
			name = wd.Name
		}
		workloadNames = append(workloadNames, name)
		templateRef, ok := wd.ObjectMeta.Annotations["defatultTemplateRef"]
		if !ok {
			continue
		}

		var tmp v1alpha2.Template
		err = c.Get(ctx, client.ObjectKey{Namespace: "", Name: templateRef}, &tmp)
		if err != nil {
			return fmt.Errorf("list workload Definition err: %v", err)
		}

		subcmd := &cobra.Command{
			Use:                   name + " [args]",
			DisableFlagsInUseLine: true,
			Short:                 "Run " + name + " workloads",
			Long:                  "Run " + name + " workloads",
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := o.Complete(f, cmd, args); err != nil {
					return err
				}
				return o.Run(f, cmd)
			},
		}
		subcmd.SetOutput(o.Out)
		for _, v := range tmp.Spec.Parameters {
			if tmp.Spec.LastCommandParam != v.Name {
				runCmd.PersistentFlags().StringP(v.Name, v.Short, v.Default, v.Usage)
			}
		}

		runCmd.AddCommand(subcmd)
	}

	runCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return errors.New("You must specify a workload, like " + strings.Join(workloadNames, ", ") +
			"\nSee 'rudr run -h' for help and examples")
	}
	return runCmd.Execute()
}

func (o *runOptions) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string) error {
	namespace, explicitNamespace, err := f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	} else if !explicitNamespace {
		namespace = "default"
	}

	argsLenght := len(args)
	lastCommandParam := o.Template.Spec.LastCommandParam

	if argsLenght < 1 {
		return errors.New("must specify name for workload")
	} else if argsLenght < 2 && lastCommandParam != "" {
		// TODO(zzxwill): Could not determine whether the argument is the workload name or image name if without image tag
		return fmt.Errorf("You must specify `%s` as the last command.\nSee 'rudr run -h' for help and examples",
			lastCommandParam)
	}

	o.Namespace = namespace
	pvd := fieldpath.Pave(o.Template.Spec.Object.Object)
	for _, v := range o.Template.Spec.Parameters {
		lastCommandValue := args[argsLenght-1]
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

	pvd.SetString("metadata.name", args[0])

	namespaceCover := cmd.Flag("namespace").Value.String()
	if namespaceCover != "" {
		namespace = namespaceCover
	}
	o.Component.Spec.Workload.Object = &unstructured.Unstructured{Object: pvd.UnstructuredContent()}
	o.Component.Name = args[0]
	o.Component.Namespace = namespace
	o.AppConfig.Name = args[0]
	o.AppConfig.Namespace = namespace
	o.AppConfig.Spec.Components = append(o.AppConfig.Spec.Components, corev1alpha2.ApplicationConfigurationComponent{ComponentName: args[0]})
	return nil
}

func (o *runOptions) Run(f cmdutil.Factory, cmd *cobra.Command) error {
	o.Infof("Creating AppConfig %s", o.AppConfig.Name)
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
		Long:                  "Create and Run one component one AppConfig OAM APP",
		Example: `
  rudrx run containerized frontend -p 80 oam-dev/demo:v1
`}
}
