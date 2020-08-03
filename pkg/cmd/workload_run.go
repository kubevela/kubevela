package cmd

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cloud-native-application/rudrx/api/types"

	"github.com/cloud-native-application/rudrx/pkg/plugins"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
)

// ComponentWorkloadDefLabel indicate which workloaddefinition generate from
const ComponentWorkloadDefLabel = "rudrx.oam.dev/workloadDef"

type runOptions struct {
	Template  types.Template
	Env       *EnvMeta
	Component corev1alpha2.Component
	AppConfig corev1alpha2.ApplicationConfiguration
	client    client.Client
	cmdutil.IOStreams
}

func newRunOptions(ioStreams cmdutil.IOStreams) *runOptions {
	return &runOptions{IOStreams: ioStreams}
}

func AddWorkloadPlugins(parentCmd *cobra.Command, c client.Client, ioStreams cmdutil.IOStreams) error {
	templates, err := plugins.GetWorkloadsFromCluster(context.TODO(), types.DefaultOAMNS, c)
	if err != nil {
		return err
	}

	for _, tmp := range templates {
		var name = tmp.Alias
		o := newRunOptions(ioStreams)
		o.client = c
		o.Env, _ = GetEnv()
		pluginCmd := &cobra.Command{
			Use:                   name + ":run <appname> [args]",
			DisableFlagsInUseLine: true,
			Short:                 "Run " + name + " workloads",
			Long:                  "Run " + name + " workloads",
			Example:               `rudr deployment:run frontend -i nginx:latest`,
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := o.Complete(cmd, args, context.TODO()); err != nil {
					return err
				}
				return o.Run(cmd)
			},
		}
		pluginCmd.SetOut(o.Out)
		for _, v := range tmp.Parameters {
			pluginCmd.Flags().StringP(v.Name, v.Short, v.Default, v.Usage)
			if v.Required {
				pluginCmd.MarkFlagRequired(v.Name)
			}
		}

		o.Template = tmp
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

	workloadTemplate := o.Template
	pvd := fieldpath.Pave(workloadTemplate.Object)
	for _, v := range workloadTemplate.Parameters {
		var paraV string

		flagSet := cmd.Flag(v.Name)
		paraV = flagSet.Value.String()

		if paraV == "" {
			continue
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
	o.Component.Labels[ComponentWorkloadDefLabel] = workloadName

	o.AppConfig.Name = args[0]
	o.AppConfig.Namespace = namespace
	o.AppConfig.Spec.Components = append(o.AppConfig.Spec.Components, corev1alpha2.ApplicationConfigurationComponent{ComponentName: args[0]})

	return nil
}

func (o *runOptions) Run(cmd *cobra.Command) error {
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
