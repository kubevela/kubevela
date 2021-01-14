package plugins

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cuelang.org/go/cue"

	"github.com/oam-dev/kubevela/apis/types"
	mycue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

const (
	// BaseRefPath is the target path for reference docs
	BaseRefPath = "docs/en/developers/references"
	// ReferenceSourcePath is the location for source reference
	ReferenceSourcePath = "hack/references"
)

// Int64Type is int64 type
type Int64Type int64

// StringType is string type
type StringType string

// BoolType is bool type
type BoolType bool

// ReferenceMarkdown is the struct for capability information
type ReferenceMarkdown struct {
	// CapabilityName is the name of a capability
	CapabilityName string `json:"capabilityName"`
	// CapabilityType is the type of a capability
	CapabilityType string `json:"capabilityType"`
}

// Parameter is the parameter section of CUE template
type Parameter struct {
	types.Parameter `json:",inline,omitempty"`
	// PrintableType is same to `parameter.Type` which could be printable
	PrintableType string `json:"printableType"`
	// Depth marks the depth for calling of function `parseParameters`
	Depth *int `json:"depth"`
}

var refContent string
var recurseDepth *int

// GenerateReferenceDocs generates reference docs
func GenerateReferenceDocs() error {

	caps, err := LoadAllInstalledCapability()
	if err != nil {
		return fmt.Errorf("failed to generate reference docs for all capabilities: %s", err)
	}

	return CreateMarkdown(caps, BaseRefPath, ReferenceSourcePath)
}

// CreateMarkdown creates markdown based on capabilities
func CreateMarkdown(caps []types.Capability, baseRefPath, referenceSourcePath string) error {
	var capabilityType string
	var specificationType string
	for _, c := range caps {
		switch c.Type {
		case types.TypeWorkload:
			capabilityType = "workload-types"
			specificationType = "workload type"
		case types.TypeTrait:
			capabilityType = "traits"
			specificationType = "trait"
		default:
			return fmt.Errorf("the type of the capability is not right")
		}

		fileName := fmt.Sprintf("%s.md", c.Name)
		filePath := filepath.Join(baseRefPath, capabilityType)
		if _, err := os.Stat(filePath); err != nil && os.IsNotExist(err) {
			if err := os.MkdirAll(filePath, 0750); err != nil {
				return err
			}
		}

		markdownFile := filepath.Join(baseRefPath, capabilityType, fileName)
		f, err := os.OpenFile(filepath.Clean(markdownFile), os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %s", markdownFile, err)
		}
		if err = os.Truncate(markdownFile, 0); err != nil {
			return fmt.Errorf("failed to truncate file %s: %s", markdownFile, err)
		}
		capName := c.Name
		ref := ReferenceMarkdown{
			CapabilityName: capName,
			CapabilityType: capabilityType,
		}

		cueValue, err := common.GetCUEParameterValue(c.CueTemplate)
		if err != nil {
			return fmt.Errorf("failed to retrieve `parameters` value from %s with err: %s", c.Name, err)
		}
		refContent = ""
		var defaultDepth = 0
		recurseDepth = &defaultDepth
		capNameInTitle := strings.Title(capName)
		if err := ref.parseParameters(cueValue, "Properties", defaultDepth); err != nil {
			return err
		}
		title := fmt.Sprintf("# %s", capNameInTitle)
		description := fmt.Sprintf("\n\n## Description\n\n%s", c.Description)
		specificationIntro := fmt.Sprintf("List of all configuration options for a `%s` %s.", capNameInTitle, specificationType)
		specificationContent, err := generateSpecification(capName, referenceSourcePath)
		if err != nil {
			return err
		}
		specification := fmt.Sprintf("\n\n## Specification\n\n%s\n\n%s", specificationIntro, specificationContent)

		conflictWithAndMoreSection, err := generateConflictWithAndMore(capName, referenceSourcePath)
		if err != nil {
			return err
		}
		refContent = title + description + specification + refContent + conflictWithAndMoreSection
		if _, err := f.WriteString(refContent); err != nil {
			return nil
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

// prepareParameterTable prepares the table content for each property
func (ref *ReferenceMarkdown) prepareParameterTable(tableName string, parameterList []Parameter) string {
	refContent := fmt.Sprintf("\n\n%s\n\n", tableName)
	refContent += "Name | Description | Type | Required | Default \n"
	refContent += "------------ | ------------- | ------------- | ------------- | ------------- \n"
	for _, p := range parameterList {
		printableDefaultValue := getPrintableDefaultValue(p.Default)
		refContent += fmt.Sprintf(" %s | %s | %s | %t | %s \n", p.Name, p.Usage, p.PrintableType, p.Required, printableDefaultValue)
	}
	return refContent
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
	default:
		//
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
	case Int64Type:
		return strconv.FormatInt(v.(int64), 10)
	case StringType:
		if v == "" {
			return "empty"
		}
		return v.(string)
	case BoolType:
		return strconv.FormatBool(v.(bool))
	}
	return ""
}

// generateSpecification generates Specification part for reference docs
func generateSpecification(capability string, referenceSourcePath string) (string, error) {
	configurationPath, err := filepath.Abs(filepath.Join(referenceSourcePath, "configurations", fmt.Sprintf("%s.yaml", capability)))
	if err != nil {
		return "", fmt.Errorf("failed to get configuration path: %w", err)
	}

	spec, err := ioutil.ReadFile(filepath.Clean(configurationPath))
	// skip if Configuration usage of a capability doesn't exist.
	if err != nil {
		spec = nil
	}
	return fmt.Sprintf("```yaml\n%s```", string(spec)), nil
}

// generateConflictWithAndMore generates Section `Conflicts With` and more like `How xxx works` in reference docs
func generateConflictWithAndMore(capabilityName string, referenceSourcePath string) (string, error) {
	conflictWithFile, err := filepath.Abs(filepath.Join(referenceSourcePath, "conflictsWithAndMore", fmt.Sprintf("%s.md", capabilityName)))
	if err != nil {
		return "", fmt.Errorf("failed to locate conflictWith file: %w", err)
	}
	data, err := ioutil.ReadFile(filepath.Clean(conflictWithFile))
	if err != nil {
		return "", nil
	}
	return "\n" + string(data), nil
}
