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
	"github.com/oam-dev/kubevela/pkg/cue/model/sets"
	errors2 "github.com/pkg/errors"
	"strings"

	"cuelang.org/go/cue"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/model"
)

// GetParameters get parameter from cue template
func GetParameters(templateStr string) ([]types.Parameter, error) {
	r := cue.Runtime{}
	template, err := r.Compile("", templateStr+BaseTemplate)
	if err != nil {
		return nil, err
	}
	tempStruct, err := template.Value().Struct()
	if err != nil {
		return nil, err
	}
	// find the parameter definition
	var paraDef cue.FieldInfo
	var found bool
	for i := 0; i < tempStruct.Len(); i++ {
		paraDef = tempStruct.Field(i)
		if paraDef.Name == model.ParameterFieldName {
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("arguments not exist")
	}
	arguments, err := paraDef.Value.Struct()
	if err != nil {
		return nil, fmt.Errorf("arguments not defined as struct %w", err)
	}
	// parse each fields in the parameter fields
	var params []types.Parameter
	for i := 0; i < arguments.Len(); i++ {
		fi := arguments.Field(i)
		if fi.IsDefinition {
			continue
		}
		var param = types.Parameter{
			Name:     fi.Name,
			Required: !fi.IsOptional,
		}
		val := fi.Value
		param.Type = fi.Value.IncompleteKind()
		param.Short, param.Usage, param.Alias, param.Ignore, param.Required = RetrieveComments(val)

		if def, ok := val.Default(); ok && def.IsConcrete() && !param.Required {
			param.Type = def.Kind()
			param.Default = GetDefault(def)
		}
		params = append(params, param)
	}
	return params, nil
}

// GetNestedParameters helps generate NestedParameters from cue's parameter section
func GetNestedParameters(parameter string) ([]types.NestedParameter, error) {
	r := cue.Runtime{}
	paramValue, err := r.Compile("", parameter+BaseTemplate)
	if err != nil {
		return nil, err
	}
	pValue, err := paramValue.Lookup("parameter").Struct()
	if err != nil {
		return nil, errors2.Wrap(err, "parameters not defined as struct")
	}
	var params []types.NestedParameter
	for i := 0; i < pValue.Len(); i++ {
		pi := pValue.Field(i)
		if pi.IsDefinition {
			continue
		}
		var param = types.NestedParameter{
			Parameter: types.Parameter{
				Name:     pi.Name,
				Required: !pi.IsOptional,
			},
			SubParam: nil,
		}
		val := pi.Value
		param.Type = val.Kind()
		switch param.Type {
		case cue.StructKind:
			subStr, err := sets.ToString(val)
			if err != nil {
				return nil, err
			}
			subParam, err := GetNestedParameters(subStr)
			if err != nil {
				return nil, err
			}
			param.SubParam = subParam
		default:
			param.JSONType = val.IncompleteKind().String()
			param.Type = val.IncompleteKind()
			param.Short, param.Usage, param.Alias, param.Ignore, param.Required = RetrieveComments(val)
			if def, ok := val.Default(); ok && def.IsConcrete() && !param.Required {
				param.Type = def.Kind()
				param.Default = GetDefault(def)
			}
		}
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
	// OptionalPrefix explicitly defines parameter is optional, with this flag, a default value should be provided
	// For instance
	// //+optional
	// domain: *"" | string
	OptionalPrefix = "+optional"
)

// RetrieveComments will retrieve Usage, Short, Alias and Ignore from CUE Value
func RetrieveComments(value cue.Value) (string, string, string, bool, bool) {
	var short, usage, alias string
	var ignore, required = false, true
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
			if strings.HasPrefix(line, OptionalPrefix) {
				required = false
			}
		}
	}
	return short, usage, alias, ignore, required
}
