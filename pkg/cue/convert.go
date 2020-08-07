package cue

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cloud-native-application/rudrx/api/types"

	"cuelang.org/go/cue"
	"cuelang.org/go/pkg/encoding/json"
)

const Template = "#Template"

func Eval(templatePath, workloadType string, value map[string]interface{}) (string, error) {
	r := cue.Runtime{}
	template, err := r.Compile(templatePath, nil)
	if err != nil {
		return "", err
	}

	tempValue := template.Value()
	appValue, err := tempValue.Fill(value, workloadType).Eval().Struct()
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

func Parse(templatePath, workloadType string, value map[string]interface{}) error {
	r := cue.Runtime{}
	template, err := r.Compile(templatePath, nil)
	if err != nil {
		return err
	}

	tempValue := template.Value()
	appValue, err := tempValue.Fill(value, workloadType).Eval().Struct()
	if err != nil {
		return err
	}

	final, err := appValue.FieldByName(Template, true)
	if err != nil {
		return err
	}
	if err := final.Value.Validate(cue.Concrete(true), cue.Final()); err != nil {
		return err
	}
	data, err := json.Marshal(final.Value)
	if err != nil {
		return err
	}
	println(string(data))

	return nil
}

func GetParameters(templatePath string) ([]types.Parameter, string, error) {
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
	var params []types.Parameter
	for i := 0; i < str.Len(); i++ {
		fi := str.Field(i)
		if fi.IsDefinition {
			continue
		}
		var param = types.Parameter{
			Name:     fi.Name,
			Required: true,
		}
		val := fi.Value
		param.Type = fi.Value.IncompleteKind()
		if def, ok := val.Default(); ok && def.IsConcrete() {
			param.Required = false
			param.Type = def.Kind()
			param.Default = GetDefault(def)
		}
		if param.Default == nil {
			param.Default = getDefaultByKind(param.Type)
		}

		short, usage := RetrieveComments(val)
		if short != "" {
			param.Short = short
		}
		if usage != "" {
			param.Usage = usage
		}
		params = append(params, param)
	}
	return params, workloadType, nil
}

func getDefaultByKind(k cue.Kind) interface{} {
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
	}
	// assume other cue kind won't be valid parameter
	return nil
}

func GetDefault(val cue.Value) interface{} {
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
	}
	return getDefaultByKind(val.Kind())
}

const (
	UsagePrefix = "+usage="
	ShortPrefix = "+short="
)

func RetrieveComments(value cue.Value) (string, string) {
	var short, usage string
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
			if strings.HasPrefix(line, UsagePrefix) {
				usage = strings.TrimPrefix(line, UsagePrefix)
			}
		}
	}
	return short, usage
}
