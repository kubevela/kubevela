package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"gopkg.in/yaml.v3"

	"cuelang.org/go/cue"

	"github.com/cloud-native-application/rudrx/pkg/utils/system"

	"github.com/cloud-native-application/rudrx/api/types"

	"github.com/cloud-native-application/rudrx/pkg/plugins"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	mycue "github.com/cloud-native-application/rudrx/pkg/cue"

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

// ComponentWorkloadDefLabel indicate which workloaddefinition generate from
const ComponentWorkloadDefLabel = "vela.oam.dev/workloadDef"

type runOptions struct {
	Template     types.Template
	Env          *types.EnvMeta
	workloadName string
	client       client.Client
	app          *types.Application
	cmdutil.IOStreams
}

func newRunOptions(ioStreams cmdutil.IOStreams) *runOptions {
	return &runOptions{IOStreams: ioStreams}
}

func AddWorkloadPlugins(parentCmd *cobra.Command, c types.Args, ioStreams cmdutil.IOStreams) error {
	dir, _ := system.GetDefinitionDir()
	templates, err := plugins.LoadTempFromLocal(filepath.Join(dir, "workloads"))
	if err != nil {
		return err
	}

	for _, tmp := range templates {
		var name = tmp.Name
		o := newRunOptions(ioStreams)
		o.Env, _ = GetEnv()
		pluginCmd := &cobra.Command{
			Use:                   name + ":run <appname> [args]",
			DisableFlagsInUseLine: true,
			Short:                 "Run " + name + " workloads",
			Long:                  "Run " + name + " workloads",
			Example:               `vela deployment:run frontend -i nginx:latest`,
			RunE: func(cmd *cobra.Command, args []string) error {
				newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
				if err != nil {
					return err
				}
				o.client = newClient
				if err := o.Complete(cmd, args, context.TODO()); err != nil {
					return err
				}
				return o.Run(cmd)
			},
		}
		pluginCmd.SetOut(o.Out)
		for _, v := range tmp.Parameters {
			types.SetFlagBy(pluginCmd, v)
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
	// TODO(wonderflow): load application from file
	var app = &types.Application{Name: workloadName}

	if app.Components == nil {
		app.Components = make(map[string]map[string]interface{})
	}
	tp, workloadData, err := app.GetWorkload(workloadName)
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
	workloadData["name"] = strings.ToLower(workloadName)
	app.Components[workloadName] = map[string]interface{}{
		tp: workloadData,
	}
	o.workloadName = workloadName
	o.app = app
	appDir, _ := system.GetApplicationDir()
	out, err := yaml.Marshal(app)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(appDir, workloadName), out, 0644)
}

func (o *runOptions) Run(cmd *cobra.Command) error {
	var component corev1alpha2.Component
	var appconfig corev1alpha2.ApplicationConfiguration
	tp, data, _ := o.app.GetWorkload(o.workloadName)
	jsondata, err := mycue.Eval(o.Template.DefinitionPath, tp, data)
	if err != nil {
		return err
	}
	var obj = make(map[string]interface{})
	if err = json.Unmarshal([]byte(jsondata), &obj); err != nil {
		return err
	}

	component.Spec.Workload.Object = &unstructured.Unstructured{Object: obj}
	component.Name = o.workloadName
	component.Namespace = o.Env.Namespace
	component.Labels = map[string]string{ComponentWorkloadDefLabel: o.workloadName}

	appconfig.Name = o.workloadName
	appconfig.Namespace = o.Env.Namespace
	appconfig.Spec.Components = append(appconfig.Spec.Components, corev1alpha2.ApplicationConfigurationComponent{ComponentName: o.workloadName})

	//TODO(wonderflow): we should also support update here

	o.Infof("Creating AppConfig %s\n", appconfig.Name)
	err = o.client.Create(context.Background(), &component)
	if err != nil {
		return fmt.Errorf("create component err: %s", err)
	}
	err = o.client.Create(context.Background(), &appconfig)
	if err != nil {
		return fmt.Errorf("create appconfig err %s", err)
	}
	o.Info("SUCCEED")
	return nil
}
