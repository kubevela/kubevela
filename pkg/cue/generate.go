package cue

import (
	"errors"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/pkg/encoding/json"
)

const Template = "#Template"

func Eval(templatePath, appPath, workloadType string) (string, error) {
	r := cue.Runtime{}
	template, err := r.Compile(templatePath, nil)
	if err != nil {
		return "", err
	}
	app, err := r.Compile(appPath, nil)
	if err != nil {
		return "", err
	}

	tempValue := template.Value()
	appinfo, err := app.Value().FieldByName(workloadType, false)
	if err != nil {
		return "", err
	}

	appValue, err := tempValue.Fill(appinfo.Value, workloadType).Eval().Struct()
	if err != nil {
		return "", err
	}

	final, err := appValue.FieldByName(Template, true)
	if err != nil {
		return "", err
	}
	if err := final.Value.Validate(cue.Concrete(true), cue.Final()); err != nil {
		return "", err
	}
	data, err := json.Marshal(final.Value)
	if err != nil {
		return "", err
	}

	return data, nil
}

type CueParameter struct {
	Name    string
	Default interface{}
}

func GetParameters(templatePath string) ([]CueParameter, string, error) {
	r := cue.Runtime{}
	template, err := r.Compile(templatePath, nil)
	if err != nil {
		return nil, "", err
	}
	tempStruct, err := template.Value().Struct()
	if err != nil {
		return nil, "", err
	}
	var info cue.FieldInfo
	var found bool
	for i := 0; i < tempStruct.Len(); i++ {
		info = tempStruct.Field(i)
		if info.IsDefinition {
			continue
		}
		found = true
		break
	}
	if !found {
		return nil, "", errors.New("arguments not exist")
	}
	str, err := info.Value.Struct()
	if err != nil {
		return nil, "", fmt.Errorf("arguments not defined as struct %v", err)
	}
	var workloadType = info.Name
	var params []CueParameter
	for i := 0; i < str.Len(); i++ {
		fi := str.Field(i)
		if fi.IsDefinition {
			continue
		}
		var param = CueParameter{
			Name: fi.Name,
		}

		if def, ok := fi.Value.Default(); ok && def.IsConcrete() {
			if data, err := def.MarshalJSON(); err == nil {
				param.Default = string(data)
			}
		}
		params = append(params, param)
	}
	return params, workloadType, nil
}
