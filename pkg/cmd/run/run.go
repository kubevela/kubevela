package run

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"os"
	"strings"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"

	"github.com/cloud-native-application/rudrx/api/v1alpha2"

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func NewCmdRun(f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams) *cobra.Command {

	ctx := context.Background()

	var workloadDefs corev1alpha2.WorkloadDefinitionList
	err := c.List(ctx, &workloadDefs)
	if err != nil {
		fmt.Println("list workload Definition err", err)
		os.Exit(1)
	}

	var workloadShortNames []string

	workloadDefsItem := workloadDefs.Items

	for _, wd := range workloadDefsItem {
		n := wd.ObjectMeta.Annotations["short"]
		if n == "" {
			n = wd.Name
		}
		workloadShortNames = append(workloadShortNames, n)
		wd.ObjectMeta.Annotations["short"] = n
	}

	cmd := &cobra.Command{
		Use:                   "run [WORKLOAD_KIND] [args]",
		DisableFlagsInUseLine: true,
		Short:                 "Run OAM workloads",
		Long:                  "Create and Run one component one AppConfig OAM APP",
		Example: `
	rudrx run containerized frontend -p 80 oam-dev/demo:v1
`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("You must specify a workload, like " + strings.Join(workloadShortNames, ", ") +
				"\nSee 'rudr run -h' for help and examples")
		},
	}

	if len(workloadDefsItem) == 0 {
		// TODO(zzxwill) Refine this prompt message
		fmt.Println("Somehow Workload Definitions are NOT preconfigured, please report this to OAM maintainers.")
		os.Exit(1)
	}

	for _, wd := range workloadDefsItem {
		templateRef, ok := wd.ObjectMeta.Annotations["defatultTemplateRef"]
		if !ok {
			continue
		}
		short := wd.ObjectMeta.Annotations["short"]
		var tmp v1alpha2.Template
		err = c.Get(ctx, client.ObjectKey{Namespace: "default", Name: templateRef}, &tmp)
		if err != nil {
			fmt.Println("list workload Definition err", err)
			os.Exit(1)
		}
		var workloadName = wd.Name
		if short != "" {
			workloadName = short
		}
		o := newRunOptions(ioStreams)
		o.client = c
		subcmd := &cobra.Command{
			Use:                   workloadName + " [args]",
			DisableFlagsInUseLine: true,
			Short:                 "Run " + workloadName + " workloads",
			Long:                  "Run " + workloadName + " workloads",
			Run: func(cmd *cobra.Command, args []string) {
				cmdutil.CheckErr(o.Complete(f, cmd, args))
				cmdutil.CheckErr(o.Run(f, cmd))
			},
		}
		cmd.PersistentFlags().StringP("namespace", "n", "", "namespace for apps")
		for _, v := range tmp.Spec.Parameters {
			if tmp.Spec.LastCommandParam != v.Name {
				cmd.PersistentFlags().StringP(v.Name, v.Short, v.Default, v.Usage)
			}
		}
		tmp.DeepCopyInto(&o.Template)
		cmd.AddCommand(subcmd)
	}
	return cmd
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
		fmt.Println("must specify name for workload")
		os.Exit(1)
	} else if argsLenght < 2 && lastCommandParam != "" {
		// TODO(zzxwill): Could not determine whether the argument is the workload name or image name if without image tag
		errMsg := fmt.Sprintf("You must specify `%s` as the last command.\nSee 'rudr run -h' for help and examples",
			lastCommandParam)
		fmt.Println(errMsg)
		os.Exit(1)
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
			errMsg := fmt.Sprintf("Flag `%s` is NOT set, please check and try again. \nSee 'rudr run -h' for help and examples", v.Name)
			fmt.Println(errMsg)
			os.Exit(1)
		}

		for _, path := range v.FieldPaths {
			pvd.SetString(path, paraV)
		}
	}
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
	fmt.Println("Creating AppConfig", o.AppConfig.Name)
	err := o.client.Create(context.Background(), &o.Component)
	if err != nil {
		fmt.Println("create component err", err)
		os.Exit(1)
	}
	err = o.client.Create(context.Background(), &o.AppConfig)
	if err != nil {
		fmt.Println("create appconfig err", err)
		os.Exit(1)
	}
	fmt.Println("SUCCEED")
	return nil
}
