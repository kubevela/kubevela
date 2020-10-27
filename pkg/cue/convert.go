package cue

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	"cuelang.org/go/cue"
	cueJson "cuelang.org/go/pkg/encoding/json"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/api/types"
)

// OutputFieldName is the name of the struct contains the CR data
const OutputFieldName = "output"

// para struct contains the parameter
const specValue = "parameter"

// Eval evaluates the spec with the parameter values
func Eval(templatePath string, value map[string]interface{}) (*unstructured.Unstructured, error) {
	r := cue.Runtime{}
	b, err := ioutil.ReadFile(templatePath)
	if err != nil {
		return nil, err
	}
	template, err := r.Compile("", string(b)+BaseTemplate)
	if err != nil {
		return nil, fmt.Errorf("compile %s err %v", templatePath, err)
	}
	// fill in the parameter values and evaluate
	tempValue := template.Value()
	appValue, err := tempValue.Fill(value, specValue).Eval().Struct()
	if err != nil {
		return nil, fmt.Errorf("fill value to template err %v", err)
	}
	// fetch the spec struct content
	final, err := appValue.FieldByName(OutputFieldName, true)
	if err != nil {
		return nil, fmt.Errorf("get template %s err %v", OutputFieldName, err)
	}
	if err := final.Value.Validate(cue.Concrete(true), cue.Final()); err != nil {
		return nil, err
	}
	data, err := cueJson.Marshal(final.Value)
	if err != nil {
		return nil, fmt.Errorf("marshal final value err %v", err)
	}
	// need to unmarshal it to a map to get rid of the outer spec name
	obj := make(map[string]interface{})
	if err = json.Unmarshal([]byte(data), &obj); err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: obj}, nil
}

func GetParameters(templatePath string) ([]types.Parameter, error) {
	r := cue.Runtime{}
	b, err := ioutil.ReadFile(templatePath)
	if err != nil {
		return nil, err
	}
	template, err := r.Compile("", string(b)+BaseTemplate)
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
		if paraDef.Name == specValue {
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("arguments not exist")
	}
	arguments, err := paraDef.Value.Struct()
	if err != nil {
		return nil, fmt.Errorf("arguments not defined as struct %v", err)
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
		if def, ok := val.Default(); ok && def.IsConcrete() {
			param.Required = false
			param.Type = def.Kind()
			param.Default = GetDefault(def)
		}
		if param.Default == nil {
			param.Default = getDefaultByKind(param.Type)
		}
		param.Short, param.Usage = RetrieveComments(val)
		params = append(params, param)
	}
	return params, nil
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
