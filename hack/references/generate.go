package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"github.com/oam-dev/kubevela/apis/types"
	mycue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/plugins"
)

const BaseRefPath = "docs/en/developers/references"

type ReferenceMarkdown struct {
	CapabilityName string `json:"capabilityName"`
	CapabilityType string `json:"capabilityType"`
}
type Parameter struct {
	types.Parameter `json:",inline,omitempty"`
	// PrintableType is same to `parameter.Type` which could be printable
	PrintableType string `json:"printableType"`
	// Depth marks the depth for calling of function `parseParameters`
	Depth *int `json:"depth"`
}

var refContent string
var recurseDepth *int

func main() {
	var capabilityType string
	var specificationType string
	caps, err := plugins.LoadAllInstalledCapability()
	if err != nil {
		fmt.Printf("failed to generate reference docs for all capabilities: %s", err)
		os.Exit(1)
	}
	for _, c := range caps {
		switch c.Type {
		case "workload":
			capabilityType = "workload-types"
			specificationType = "workload type"
		case "trait":
			capabilityType = "traits"
			specificationType = "trait"
		default:
			fmt.Printf("the type of the capability is not right")
			os.Exit(1)
		}

		fileName := fmt.Sprintf("%s.md", c.Name)
		filePath := filepath.Join(BaseRefPath, capabilityType, fileName)
		f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			fmt.Printf("failed to open file %s: %s", filePath, err)
			os.Exit(1)
		}
		if err = os.Truncate(filePath, 0); err != nil {
			fmt.Printf("failed to truncate file %s: %s", filePath, err)
			os.Exit(1)
		}
		capName := c.Name
		ref := ReferenceMarkdown{
			CapabilityName: capName,
			CapabilityType: capabilityType,
		}

		cueValue, err := getCUEParameterValue(c.CueTemplate)
		if err != nil {
			fmt.Printf("failed to retrieve `parameters` value from %s with err: %s", c.Name, err)
			os.Exit(1)
		}
		refContent = ""
		var defaultDepth = 0
		recurseDepth = &defaultDepth
		capNameInTitle := strings.Title(capName)
		if err := ref.parseParameters(cueValue, "Properties", defaultDepth); err != nil {
			fmt.Printf(err.Error())
			os.Exit(1)
		}
		title := fmt.Sprintf("# %s", capNameInTitle)
		description := fmt.Sprintf("\n\n## Description\n\n%s", c.Description)
		specificationIntro := fmt.Sprintf("List of all configuration options for a `%s` %s.", capNameInTitle, specificationType)
		specificationContent, err := generateSpecification(capName)
		if err != nil {
			fmt.Printf(err.Error())
			os.Exit(1)
		}
		specification := fmt.Sprintf("\n\n## Specification\n\n%s\n\n%s", specificationIntro, specificationContent)
		refContent = title + description + specification + refContent
		f.WriteString(refContent)
		f.Close()
	}
}

// prepareParameterTable prepares the table content for each property
func (ref *ReferenceMarkdown) prepareParameterTable(tableName string, parameterList []Parameter) string {
	refContent := fmt.Sprintf("\n\n%s\n\n", tableName)
	refContent += "Name | Description | Type | Required | Default \n"
	refContent += "------------ | ------------- | ------------- | ------------- | ------------- \n"
	for _, p := range parameterList {
		//defaultValue := p.Default
		//if defaultValue == nil {
		//	defaultValue = ""
		//}
		printableDefaultValue := getPrintableDefaultValue(p.Default)
		refContent += fmt.Sprintf(" %s | %s | %s | %t | %s \n", p.Name, p.Usage, p.PrintableType, p.Required, printableDefaultValue)
	}
	return refContent
}

// getCUEParameterValue converts definitions to cue format
func getCUEParameterValue(cueStr string) (cue.Value, error) {
	r := cue.Runtime{}
	template, err := r.Compile("", cueStr+mycue.BaseTemplate)
	if err != nil {
		return cue.Value{}, err
	}
	tempStruct, err := template.Value().Struct()
	if err != nil {
		return cue.Value{}, err
	}
	// find the parameter definition
	var paraDef cue.FieldInfo
	var found bool
	for i := 0; i < tempStruct.Len(); i++ {
		paraDef = tempStruct.Field(i)
		if paraDef.Name == "parameter" {
			found = true
			break
		}
	}
	if !found {
		return cue.Value{}, errors.New("arguments not exist")
	}
	arguments := paraDef.Value

	return arguments, nil
}

// parseParameters parses every parameter
func (ref *ReferenceMarkdown) parseParameters(paraValue cue.Value, paramKey string, depth int) error {
	var params []Parameter
	*recurseDepth++
	switch paraValue.Kind() {
	case cue.StructKind:
		arguments, err := paraValue.Struct()
		if err != nil {
			return fmt.Errorf("arguments not defined as struct %w", err)
		}
		for i := 0; i < arguments.Len(); i++ {
			var param Parameter
			fi := arguments.Field(i)
			if fi.IsDefinition {
				continue
			}
			val := fi.Value
			name := fi.Name
			param.Name = name
			param.Required = !fi.IsOptional
			if def, ok := val.Default(); ok && def.IsConcrete() {
				param.Default = mycue.GetDefault(def)
			}
			param.Short, param.Usage, param.Alias = mycue.RetrieveComments(val)
			param.Type = val.IncompleteKind()
			switch val.IncompleteKind() {
			case cue.StructKind:
				depth := *recurseDepth
				// TODO(zzxwill) this case not processed  `selector?: [string]: string`
				if name == "selector" {
					param.PrintableType = "map[string]string"
				} else {
					if err := ref.parseParameters(val, name, depth); err != nil {
						return err
					}
					param.PrintableType = fmt.Sprintf("[%s](#%s)", name, name)
				}
			case cue.ListKind:
				elem, success := val.Elem()
				if !success {
					return fmt.Errorf("failed to get elements from %s", val)
				}
				switch elem.Kind() {
				case cue.StructKind:
					param.PrintableType = fmt.Sprintf("[[]%s](#%s)", name, name)
					depth := *recurseDepth
					if err := ref.parseParameters(elem, name, depth); err != nil {
						return err
					}
				default:
					param.Type = elem.Kind()
					param.PrintableType = fmt.Sprintf("[]%s", elem.IncompleteKind().String())
				}
			default:
				param.PrintableType = param.Type.String()
			}
			params = append(params, param)
		}
	}

	tableName := fmt.Sprintf("%s %s", strings.Repeat("#", depth+2), paramKey)
	refContent = ref.prepareParameterTable(tableName, params) + refContent
	return nil
}

// getPrintableDefaultValue converts the value in `interface{}` type to be printable
func getPrintableDefaultValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch v.(type) {
	case int64:
		return strconv.FormatInt(v.(int64), 10)
	case string:
		if v == "" {
			return "empty"
		}
		return v.(string)
	case bool:
		return strconv.FormatBool(v.(bool))
	}
	return ""
}

// generateSpecification generates Specification part for reference docs
func generateSpecification(capability string) (string, error) {
	configurationPath, err := filepath.Abs(filepath.Join("hack/references/configurations",
		fmt.Sprintf("%s.yaml", capability)))
	if err != nil {
		return "", fmt.Errorf("failed to get configuration path: %w", err)
	}
	f, err := os.Open(configurationPath)
	if err != nil {
		return "", fmt.Errorf("failed to open configuration file: %w", err)
	}
	spec, err := ioutil.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("failed to read configuration file: %w", err)
	}
	return fmt.Sprintf("```yaml\n%s```", spec), nil
}
