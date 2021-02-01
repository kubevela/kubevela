package plugins

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"github.com/olekukonko/tablewriter"

	"github.com/oam-dev/kubevela/apis/types"
	mycue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

const (
	// BaseRefPath is the target path for reference docs
	BaseRefPath = "docs/en/developers/references"
	// ReferenceSourcePath is the location for source reference
	ReferenceSourcePath = "hack/references"

	// WorkloadTypePath is the URL path for workload typed capability
	WorkloadTypePath = "workload-types"
	// TraitPath is the URL path for trait typed capability
	TraitPath = "traits"
)

// Int64Type is int64 type
type Int64Type = int64

// StringType is string type
type StringType = string

// BoolType is bool type
type BoolType = bool

// Reference is the struct for capability information
type Reference interface {
	prepareParameter(tableName string, parameterList []ReferenceParameter) string
}

// ParseReference is used to include the common function `parseParameter`
type ParseReference struct {
}

// MarkdownReference is the struct for capability information in
type MarkdownReference struct {
	ParseReference
}

// ConsoleReference is the struct for capability information in console
type ConsoleReference struct {
	ParseReference
	TableName   string             `json:"tableName"`
	TableObject *tablewriter.Table `json:"tableObject"`
}

// ConfigurationYamlSample stores the configuration yaml sample for capabilities
var ConfigurationYamlSample = map[string]string{
	"autoscale": `
name: testapp

services:
  express-server:
    ...

    autoscale:
      min: 1
      max: 4
      cron:
        startAt:  "14:00"
        duration: "2h"
        days:     "Monday, Thursday"
        replicas: 2
        timezone: "America/Los_Angeles"
      cpuPercent: 10
`,
	"metrics": `
name: my-app-name

services:
  my-service-name:
    ...
    metrics:
      format: "prometheus"
      port: 8080
      path: "/metrics"
      scheme:  "http"
      enabled: true
`,
	"rollout": `
servcies:
  express-server:
    ...

    rollout:
      replicas: 2
      stepWeight: 50
      interval: "10s"
`,
	"route": `
name: my-app-name

services:
  my-service-name:
    ...
    route:
      domain: example.com
      issuer: tls
      rules:
        - path: /testapp
          rewriteTarget: /
`,
	"scaler": `
name: my-app-name

services:
  my-service-name:
    ...
    scaler:
      replicas: 100
`,
	"task": `
name: my-app-name

services:
  my-service-name:
    type: task
    image: perl
    count: 10
    cmd: ["perl",  "-Mbignum=bpi", "-wle", "print bpi(2000)"]
`,
	"webservice": `
name: my-app-name

services:
  my-service-name:
    type: webservice # could be skipped
    image: oamdev/testapp:v1
    cmd: ["node", "server.js"]
    port: 8080
    cpu: "0.1"
    env:
      - name: FOO
        value: bar
      - name: FOO
        valueFrom:
          secretKeyRef:
            name: bar
            key: bar
`,
	"worker": `
name: my-app-name

services:
  my-service-name:
    type: worker
    image: oamdev/testapp:v1
    cmd: ["node", "server.js"]
`,
}

// ReferenceParameter is the parameter section of CUE template
type ReferenceParameter struct {
	types.Parameter `json:",inline,omitempty"`
	// PrintableType is same to `parameter.Type` which could be printable
	PrintableType string `json:"printableType"`
	// Depth marks the depth for calling of function `parseParameters`
	Depth *int `json:"depth"`
}

var refContent string
var recurseDepth *int
var propertyConsole []ConsoleReference
var displayFormat *string

func setDisplayFormat(format string) {
	displayFormat = &format
}

// GenerateReferenceDocs generates reference docs
func (ref *MarkdownReference) GenerateReferenceDocs(baseRefPath string) error {
	caps, err := LoadAllInstalledCapability()
	if err != nil {
		return fmt.Errorf("failed to generate reference docs for all capabilities: %w", err)
	}
	if baseRefPath == "" {
		baseRefPath = BaseRefPath
	}
	return ref.CreateMarkdown(caps, baseRefPath, ReferenceSourcePath)
}

// CreateMarkdown creates markdown based on capabilities
func (ref *MarkdownReference) CreateMarkdown(caps []types.Capability, baseRefPath, referenceSourcePath string) error {
	setDisplayFormat("markdown")
	var capabilityType string
	var specificationType string
	for _, c := range caps {
		switch c.Type {
		case types.TypeWorkload:
			capabilityType = WorkloadTypePath
			specificationType = "workload type"
		case types.TypeTrait:
			capabilityType = TraitPath
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
			return fmt.Errorf("failed to open file %s: %w", markdownFile, err)
		}
		if err = os.Truncate(markdownFile, 0); err != nil {
			return fmt.Errorf("failed to truncate file %s: %w", markdownFile, err)
		}
		capName := c.Name

		cueValue, err := common.GetCUEParameterValue(c.CueTemplate)
		if err != nil {
			return fmt.Errorf("failed to retrieve `parameters` value from %s with err: %w", c.Name, err)
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
		specificationContent := ref.generateSpecification(capName)
		if err != nil {
			return err
		}
		specification := fmt.Sprintf("\n\n## Specification\n\n%s\n\n%s", specificationIntro, specificationContent)

		conflictWithAndMoreSection, err := ref.generateConflictWithAndMore(capName, referenceSourcePath)
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

// prepareParameter prepares the table content for each property
func (ref *MarkdownReference) prepareParameter(tableName string, parameterList []ReferenceParameter) string {
	refContent := fmt.Sprintf("\n\n%s\n\n", tableName)
	refContent += "Name | Description | Type | Required | Default \n"
	refContent += "------------ | ------------- | ------------- | ------------- | ------------- \n"
	for _, p := range parameterList {
		printableDefaultValue := ref.getPrintableDefaultValue(p.Default)
		refContent += fmt.Sprintf(" %s | %s | %s | %t | %s \n", p.Name, p.Usage, p.PrintableType, p.Required, printableDefaultValue)
	}
	return refContent
}

// prepareParameter prepares the table content for each property
func (ref *ConsoleReference) prepareParameter(tableName string, parameterList []ReferenceParameter) ConsoleReference {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetColWidth(100)
	table.SetHeader([]string{"Name", "Description", "Type", "Required", "Default"})
	for _, p := range parameterList {
		printableDefaultValue := ref.getPrintableDefaultValue(p.Default)
		table.Append([]string{p.Name, p.Usage, p.PrintableType, strconv.FormatBool(p.Required), printableDefaultValue})
	}
	return ConsoleReference{TableName: tableName, TableObject: table}
}

// parseParameters parses every parameter
func (ref *ParseReference) parseParameters(paraValue cue.Value, paramKey string, depth int) error {
	var params []ReferenceParameter
	*recurseDepth++
	switch paraValue.Kind() {
	case cue.StructKind:
		arguments, err := paraValue.Struct()
		if err != nil {
			return fmt.Errorf("arguments not defined as struct %w", err)
		}
		for i := 0; i < arguments.Len(); i++ {
			var param ReferenceParameter
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

	switch *displayFormat {
	case "markdown":
		tableName := fmt.Sprintf("%s %s", strings.Repeat("#", depth+2), paramKey)
		ref := MarkdownReference{}
		refContent = ref.prepareParameter(tableName, params) + refContent
	case "console":
		ref := ConsoleReference{}
		tableName := fmt.Sprintf("%s %s", strings.Repeat("#", depth+1), paramKey)
		console := ref.prepareParameter(tableName, params)
		propertyConsole = append([]ConsoleReference{console}, propertyConsole...)
	}
	return nil
}

// getPrintableDefaultValue converts the value in `interface{}` type to be printable
func (ref *ParseReference) getPrintableDefaultValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch value := v.(type) {
	case Int64Type:
		return strconv.FormatInt(value, 10)
	case StringType:
		if v == "" {
			return "empty"
		}
		return value
	case BoolType:
		return strconv.FormatBool(value)
	}
	return ""
}

// generateSpecification generates Specification part for reference docs
func (ref *MarkdownReference) generateSpecification(capabilityName string) string {
	return fmt.Sprintf("```yaml%s```", ConfigurationYamlSample[capabilityName])
}

// generateConflictWithAndMore generates Section `Conflicts With` and more like `How xxx works` in reference docs
func (ref *MarkdownReference) generateConflictWithAndMore(capabilityName string, referenceSourcePath string) (string, error) {
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

// GenerateCapabilityProperties get all properties of a capability
func (ref *ConsoleReference) GenerateCapabilityProperties(capability *types.Capability) ([]ConsoleReference, error) {
	setDisplayFormat("console")
	capName := capability.Name

	cueValue, err := common.GetCUEParameterValue(capability.CueTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve `parameters` value from %s with err: %w", capName, err)
	}
	var defaultDepth = 0
	recurseDepth = &defaultDepth
	if err := ref.parseParameters(cueValue, "Properties", defaultDepth); err != nil {
		return nil, err
	}

	return propertyConsole, nil
}
