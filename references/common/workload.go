/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/oam-dev/kubevela/pkg/utils/common"

	"cuelang.org/go/cue"
	"github.com/spf13/pflag"

	"github.com/oam-dev/kubevela/references/appfile"
	"github.com/oam-dev/kubevela/references/appfile/api"
	"github.com/oam-dev/kubevela/references/docgen"
)

// InitApplication will load Application from cluster
func InitApplication(namespace string, c common.Args, workloadName string, appGroup string) (*api.Application, error) {
	var appName string
	if appGroup != "" {
		appName = appGroup
	} else {
		appName = workloadName
	}
	// TODO(wonderflow): we should load the existing application from cluster and convert to appfile
	// app, err := appfile.LoadApplication(env.Namespace, appName, c)
	// compatible application not found
	app, err := appfile.NewEmptyApplication(namespace, c)
	if err != nil {
		return nil, err
	}
	app.Name = appName

	return app, nil
}

// BaseComplete will construct an Application from cli parameters.
func BaseComplete(namespace string, c common.Args, workloadName string, appName string, flagSet *pflag.FlagSet, workloadType string) (*api.Application, error) {
	app, err := InitApplication(namespace, c, workloadName, appName)
	if err != nil {
		return nil, err
	}
	tp, workloadData := appfile.GetWorkload(app, workloadName)
	if tp == "" {
		if workloadType == "" {
			return nil, fmt.Errorf("must specify workload type for application %s", workloadName)
		}
		// Not exist
		tp = workloadType
	}
	template, err := docgen.LoadCapabilityByName(tp, namespace, c)
	if err != nil {
		return nil, err
	}

	for _, v := range template.Parameters {
		name := v.Name
		if v.Alias != "" {
			name = v.Alias
		}
		// Cli can check required flag before make a request to backend, but API itself could not, so validate flags here
		flag := flagSet.Lookup(name)
		if name == "name" {
			continue
		}
		if flag == nil || flag.Value.String() == "" {
			if v.Required {
				return nil, fmt.Errorf("required flag(s) \"%s\" not set", name)
			}
			continue
		}
		// nolint:exhaustive
		switch v.Type {
		case cue.IntKind:
			workloadData[v.Name], err = flagSet.GetInt64(name)
		case cue.StringKind:
			workloadData[v.Name], err = flagSet.GetString(name)
		case cue.BoolKind:
			workloadData[v.Name], err = flagSet.GetBool(name)
		case cue.NumberKind, cue.FloatKind:
			workloadData[v.Name], err = flagSet.GetFloat64(name)
		default:
			// Currently we don't support get value from complex type
			continue
		}
		if err != nil {
			if strings.Contains(err.Error(), "of flag of type string") {
				data, _ := flagSet.GetString(name)
				// nolint:exhaustive
				switch v.Type {
				case cue.IntKind:
					workloadData[v.Name], err = strconv.ParseInt(data, 10, 64)
				case cue.BoolKind:
					workloadData[v.Name], err = strconv.ParseBool(data)
				case cue.NumberKind, cue.FloatKind:
					workloadData[v.Name], err = strconv.ParseFloat(data, 64)
				default:
					return nil, fmt.Errorf("should not get string from type(%s) for parameter \"%s\"", v.Type.String(), name)
				}
				if err != nil {
					return nil, fmt.Errorf("get flag(s) \"%s\" err %w", v.Name, err)
				}
				continue
			}
			return nil, fmt.Errorf("get flag(s) \"%s\" err %w", v.Name, err)
		}
	}
	if err = appfile.SetWorkload(app, workloadName, tp, workloadData); err != nil {
		return app, err
	}
	return app, nil
}
