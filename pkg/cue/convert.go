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

package cue

import (
	"errors"
	"fmt"
	"strings"

	"cuelang.org/go/cue"

	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/cue/packages"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/process"
)

// ErrParameterNotExist represents the parameter field is not exist in CUE template
var ErrParameterNotExist = errors.New("parameter not exist")

// GetParameters get parameter from cue template
func GetParameters(templateStr string, pd *packages.PackageDiscover) ([]types.Parameter, error) {
	template, err := value.NewValue(templateStr+BaseTemplate, pd, "")
	if err != nil {
		return nil, fmt.Errorf("error in template: %w", err)
	}
	paramVal, err := template.LookupValue(process.ParameterFieldName)
	if err != nil || !paramVal.CueValue().Exists() {
		return nil, ErrParameterNotExist
	}
	iter, err := paramVal.CueValue().Fields(cue.Definitions(true), cue.Hidden(true), cue.All())
	if err != nil {
		return nil, err
	}
	// parse each fields in the parameter fields
	var params []types.Parameter
	for iter.Next() {
		if iter.Selector().IsDefinition() {
			continue
		}
		var param = types.Parameter{
			Name:     iter.Label(),
			Required: !iter.IsOptional(),
		}
		val := iter.Value()
		param.Type = val.IncompleteKind()
		if def, ok := val.Default(); ok && def.IsConcrete() {
			param.Required = false
			param.Type = def.Kind()
			param.Default = GetDefault(def)
		}
		if param.Default == nil {
			param.Default = getDefaultByKind(param.Type)
		}
		param.Short, param.Usage, param.Alias, param.Ignore = RetrieveComments(val)

		params = append(params, param)
	}
	return params, nil
}

func getDefaultByKind(k cue.Kind) interface{} {
	// nolint:exhaustive
	switch k {
	case cue.IntKind:
		var d int64
		return d
	case cue.StringKind:
		var d string
		return d
	case cue.BoolKind:
		var d bool
		return d
	case cue.NumberKind, cue.FloatKind:
		var d float64
		return d
	default:
		// assume other cue kind won't be valid parameter
	}
	return nil
}

// GetDefault evaluate default Go value from CUE
func GetDefault(val cue.Value) interface{} {
	// nolint:exhaustive
	switch val.Kind() {
	case cue.IntKind:
		if d, err := val.Int64(); err == nil {
			return d
		}
	case cue.StringKind:
		if d, err := val.String(); err == nil {
			return d
		}
	case cue.BoolKind:
		if d, err := val.Bool(); err == nil {
			return d
		}
	case cue.NumberKind, cue.FloatKind:
		if d, err := val.Float64(); err == nil {
			return d
		}
	default:
	}
	return getDefaultByKind(val.Kind())
}

const (
	// UsagePrefix defines the usage display for KubeVela CLI
	UsagePrefix = "+usage="
	// ShortPrefix defines the short argument for KubeVela CLI
	ShortPrefix = "+short="
	// AliasPrefix is an alias of the name of a parameter element, in order to making it more friendly to Cli users
	AliasPrefix = "+alias="
	// IgnorePrefix defines parameter in system level which we don't want our end user to see for KubeVela CLI
	IgnorePrefix = "+ignore"
)

// RetrieveComments will retrieve Usage, Short, Alias and Ignore from CUE Value
func RetrieveComments(value cue.Value) (string, string, string, bool) {
	var short, usage, alias string
	var ignore bool
	docs := value.Doc()
	for _, doc := range docs {
		lines := strings.Split(doc.Text(), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			line = strings.TrimPrefix(line, "//")
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, ShortPrefix) {
				short = strings.TrimPrefix(line, ShortPrefix)
			}
			if strings.HasPrefix(line, IgnorePrefix) {
				ignore = true
			}
			if strings.HasPrefix(line, UsagePrefix) {
				usage = strings.TrimPrefix(line, UsagePrefix)
			}
			if strings.HasPrefix(line, AliasPrefix) {
				alias = strings.TrimPrefix(line, AliasPrefix)
			}
		}
	}
	return short, usage, alias, ignore
}

// IsFieldNotExist check whether the error type is the field not found
func IsFieldNotExist(err error) bool {
	return strings.Contains(err.Error(), "not exist")
}
