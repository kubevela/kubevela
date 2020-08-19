package oam

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/pflag"

	"github.com/cloud-native-application/rudrx/api/types"
	"github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"cuelang.org/go/cue"
	"github.com/cloud-native-application/rudrx/pkg/application"
)

type RunOptions struct {
	Template     types.Capability
	Env          *types.EnvMeta
	WorkloadType string
	WorkloadName string
	KubeClient   client.Client
	App          *application.Application
	AppName      string
	Staging      bool
	util.IOStreams
}

func BaseComplete(envName string, workloadName string, appGroup string, flagSet *pflag.FlagSet, template types.Capability) (*application.Application, error) {
	var appName string
	if appGroup != "" {
		appName = appGroup
	} else {
		appName = workloadName
	}
	app, err := application.Load(envName, appName)
	if err != nil {
		return app, err
	}
	app.Name = appName

	if app.Components == nil {
		app.Components = make(map[string]map[string]interface{})
	}
	tp, workloadData := app.GetWorkload(workloadName)
	if tp == "" {
		// Not exist
		tp = template.Name
	}

	for _, v := range template.Parameters {
		flagValue, _ := flagSet.GetString(v.Name)
		// Cli can check required flag before make a request to backend, but API itself could not, so validate flags here
		if v.Required && v.Name != "name" && flagValue == "" {
			return app, fmt.Errorf("required flag(s) \"%s\" not set", v.Name)
		}
		switch v.Type {
		case cue.IntKind:
			d, _ := strconv.ParseInt(flagValue, 10, 64)
			workloadData[v.Name] = d
		case cue.StringKind:
			workloadData[v.Name] = flagValue
		case cue.BoolKind:
			d, _ := strconv.ParseBool(flagValue)
			workloadData[v.Name] = d
		case cue.NumberKind, cue.FloatKind:
			d, _ := strconv.ParseFloat(flagValue, 64)
			workloadData[v.Name] = d
		}
	}
	if err = app.SetWorkload(workloadName, tp, workloadData); err != nil {
		return app, err
	}
	return app, app.Save(envName, appName)
}

func BaseRun(staging bool, App *application.Application, kubeClient client.Client, Env *types.EnvMeta) (string, error) {
	if staging {
		return "Staging saved", nil
	}
	var msg string
	msg = fmt.Sprintf("Creating App %s\n", App.Name)
	if err := App.Run(context.Background(), kubeClient, Env); err != nil {
		err = fmt.Errorf("create app err: %s", err)
		return "", err
	}
	msg += "SUCCEED"
	return msg, nil
}
