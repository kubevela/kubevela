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

package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	velacue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/terraform"
)

const (
	// BaseRefPath is the target path for reference docs
	BaseRefPath = "docs/en/end-user"
	// KubeVelaIOTerraformPath is the target path for kubevela.io terraform docs
	KubeVelaIOTerraformPath = "kubevela.io/docs/end-user/components/cloud-services/terraform"
	// ReferenceSourcePath is the location for source reference
	ReferenceSourcePath = "hack/references"
	// ComponentDefinitionTypePath is the URL path for component typed capability
	ComponentDefinitionTypePath = "components"
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
	Client client.Client
}

// MarkdownReference is the struct for capability information in
type MarkdownReference struct {
	DefinitionName string `json:"definitionName"`
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
	"annotations": `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: myapp
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000
      traits:
        - type: labels
          properties:
            "release": "stable"
        - type: annotations
          properties:
            "description": "web application"
`,

	"ingress": `
kind: Application
metadata:
  name: first-vela-app
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000
      traits:
        - type: ingress
          properties:
            domain: testsvc.example.com
            http:
              "/": 8000
`,

	"labels": `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: myapp
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000
      traits:
        - type: labels
          properties:
            "release": "stable"
        - type: annotations
          properties:
            "description": "web application"
`,

	"metrics": `
...
    format: "prometheus"
    port: 8080
    path: "/metrics"
    scheme:  "http"
    enabled: true
`,

	"route": `
...
    domain: example.com
    issuer: tls
    rules:
      - path: /testapp
        rewriteTarget: /
`,

	"scaler": `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: website
spec:
  components:
    - name: frontend
      type: webservice
      properties:
        image: nginx
      traits:
        - type: scaler
          properties:
            replicas: 2
        - type: sidecar
          properties:
            name: "sidecar-test"
            image: "fluentd"
    - name: backend
      type: worker
      properties:
        image: busybox
        cmd:
          - sleep
          - '1000'
`,

	"sidecar": `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: vela-app-with-sidecar
spec:
  components:
    - name: log-gen-worker
      type: worker
      properties:
        image: busybox
        cmd:
          - /bin/sh
          - -c
          - >
            i=0;
            while true;
            do
              echo "$i: $(date)" >> /var/log/date.log;
              i=$((i+1));
              sleep 1;
            done
        volumes:
          - name: varlog
            mountPath: /var/log
            type: emptyDir
      traits:
        - type: sidecar
          properties:
            name: count-log
            image: busybox
            cmd: [ /bin/sh, -c, 'tail -n+1 -f /var/log/date.log']
            volumes:
              - name: varlog
                path: /var/log
`,

	"task": `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-worker
spec:
  components:
    - name: mytask
      type: task
      properties:
        image: perl
	    count: 10
	    cmd: ["perl",  "-Mbignum=bpi", "-wle", "print bpi(2000)"]
`,

	"volumes": `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-worker
spec:
  components:
    - name: myworker
      type: worker
      properties:
        image: "busybox"
        cmd:
          - sleep
          - "1000"
      traits:
        - type: aws-ebs-volume
          properties:
            name: "my-ebs"
            mountPath: "/myebs"
            volumeID: "my-ebs-id"
`,

	"webservice": `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: website
spec:
  components:
    - name: frontend
      type: webservice
      properties:
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
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-worker
spec:
  components:
    - name: myworker
      type: worker
      properties:
        image: "busybox"
        cmd:
          - sleep
          - "1000"
`,

	"alibaba-vpc": `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-vpc-sample
spec:
  components:
    - name: sample-vpc
      type: alibaba-vpc
      properties:
        vpc_cidr: "172.16.0.0/12"

        writeConnectionSecretToRef:
          name: vpc-conn
`,
}

// BaseOpenAPIV3Template is Standard OpenAPIV3 Template
var BaseOpenAPIV3Template = `{
    "openapi": "3.0.0",
    "info": {
        "title": "definition-parameter",
        "version": "1.0"
    },
    "paths": {},
    "components": {
        "schemas": {
			"parameter": %s
		}
	}
}`

// ReferenceParameter is the parameter section of CUE template
type ReferenceParameter struct {
	types.Parameter `json:",inline,omitempty"`
	// PrintableType is same to `parameter.Type` which could be printable
	PrintableType string `json:"printableType"`
}

// ReferenceParameterTable stores the information of a bunch of ReferenceParameter in a table style
type ReferenceParameterTable struct {
	Name       string
	Parameters []ReferenceParameter
	Depth      *int
}

var refContent string
var propertyConsole []ConsoleReference
var displayFormat *string
var commonRefs []CommonReference

func setDisplayFormat(format string) {
	displayFormat = &format
}

// GenerateReferenceDocs generates reference docs
func (ref *MarkdownReference) GenerateReferenceDocs(ctx context.Context, c common.Args, baseRefPath string, namespace string) error {
	var (
		caps []types.Capability
		err  error
	)

	if ref.DefinitionName == "" {
		caps, err = LoadAllInstalledCapability("default", c)
		if err != nil {
			return fmt.Errorf("failed to get all capabilityes: %w", err)
		}
	} else {
		cap, err := GetCapabilityByName(ctx, c, ref.DefinitionName, namespace)
		if err != nil {
			return fmt.Errorf("failed to get capability capability %s: %w", ref.DefinitionName, err)
		}
		caps = []types.Capability{*cap}
	}

	return ref.CreateMarkdown(ctx, caps, baseRefPath, ReferenceSourcePath)
}

// CreateMarkdown creates markdown based on capabilities
func (ref *MarkdownReference) CreateMarkdown(ctx context.Context, caps []types.Capability, baseRefPath, referenceSourcePath string) error {
	setDisplayFormat("markdown")
	for i, c := range caps {
		if c.Type != types.TypeWorkload && c.Type != types.TypeComponentDefinition && c.Type != types.TypeTrait {
			return fmt.Errorf("the type of the capability is not right")
		}

		fileName := fmt.Sprintf("%s.md", c.Name)
		if _, err := os.Stat(baseRefPath); err != nil && os.IsNotExist(err) {
			if err := os.MkdirAll(baseRefPath, 0750); err != nil {
				return err
			}
		}

		markdownFile := filepath.Join(baseRefPath, fileName)
		f, err := os.OpenFile(filepath.Clean(markdownFile), os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", markdownFile, err)
		}
		if err = os.Truncate(markdownFile, 0); err != nil {
			return fmt.Errorf("failed to truncate file %s: %w", markdownFile, err)
		}
		capName := c.Name
		refContent = ""
		capNameInTitle := makeReadableTitle(capName)
		switch c.Category {
		case types.CUECategory:
			cueValue, err := common.GetCUEParameterValue(c.CueTemplate)
			if err != nil {
				return fmt.Errorf("failed to retrieve `parameters` value from %s with err: %w", c.Name, err)
			}
			var defaultDepth = 0
			if err := ref.parseParameters(cueValue, "Properties", defaultDepth); err != nil {
				return err
			}
		case types.HelmCategory:
			properties, _, err := ref.GenerateHelmAndKubeProperties(ctx, &caps[i])
			if err != nil {
				return fmt.Errorf("failed to retrieve `parameters` value from %s with err: %w", c.Name, err)
			}
			for _, property := range properties {
				refContent += ref.prepareParameter("###"+property.Name, property.Parameters, types.HelmCategory)
			}
		case types.KubeCategory:
			properties, _, err := ref.GenerateHelmAndKubeProperties(ctx, &caps[i])
			if err != nil {
				return fmt.Errorf("failed to retrieve `parameters` value from %s with err: %w", c.Name, err)
			}
			for _, property := range properties {
				refContent += ref.prepareParameter("###"+property.Name, property.Parameters, types.KubeCategory)
			}
		case types.TerraformCategory:
			refContent, err = ref.GenerateTerraformCapabilityPropertiesAndOutputs(c)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupport capability category %s", c.Category)
		}
		title := fmt.Sprintf("---\ntitle:  %s\n---", capNameInTitle)

		description := fmt.Sprintf("\n\n## Description\n\n%s", c.Description)
		var sample string
		sampleContent := ref.generateSample(capName)
		if sampleContent != "" {
			sample = fmt.Sprintf("\n\n## Samples\n\n%s", sampleContent)
		}
		specification := fmt.Sprintf("\n\n## Specification\n%s", refContent)

		// it's fine if the conflict info files not found
		conflictWithAndMoreSection, _ := ref.generateConflictWithAndMore(capName, referenceSourcePath)

		refContent = title + description + sample + conflictWithAndMoreSection + specification
		if _, err := f.WriteString(refContent); err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

func makeReadableTitle(title string) string {
	const alibabaCloud = "alibaba-"
	if strings.HasPrefix(title, alibabaCloud) {
		cloudResource := strings.Replace(title, alibabaCloud, "", 1)
		return "Alibaba Cloud " + strings.ToUpper(cloudResource)
	}
	return strings.Title(title)
}

// prepareParameter prepares the table content for each property
func (ref *MarkdownReference) prepareParameter(tableName string, parameterList []ReferenceParameter, category types.CapabilityCategory) string {
	refContent := fmt.Sprintf("\n\n%s\n\n", tableName)
	refContent += "Name | Description | Type | Required | Default \n"
	refContent += "------------ | ------------- | ------------- | ------------- | ------------- \n"
	switch category {
	case types.CUECategory:
		for _, p := range parameterList {
			if !p.Ignore {
				printableDefaultValue := ref.getCUEPrintableDefaultValue(p.Default)
				refContent += fmt.Sprintf(" %s | %s | %s | %t | %s \n", p.Name, p.Usage, p.PrintableType, p.Required, printableDefaultValue)
			}
		}
	case types.HelmCategory:
		for _, p := range parameterList {
			printableDefaultValue := ref.getJSONPrintableDefaultValue(p.JSONType, p.Default)
			refContent += fmt.Sprintf(" %s | %s | %s | %t | %s \n", p.Name, strings.ReplaceAll(p.Usage, "\n", ""), p.PrintableType, p.Required, printableDefaultValue)
		}
	case types.KubeCategory:
		for _, p := range parameterList {
			// Kubeparameter doesn't have default value
			refContent += fmt.Sprintf(" %s | %s | %s | %t | %s \n", p.Name, strings.ReplaceAll(p.Usage, "\n", ""), p.PrintableType, p.Required, "")
		}
	case types.TerraformCategory:
		// Terraform doesn't have default value
		for _, p := range parameterList {
			refContent += fmt.Sprintf(" %s | %s | %s | %t | %s \n", p.Name, strings.ReplaceAll(p.Usage, "\n", ""), p.PrintableType, p.Required, "")
		}
	default:
	}
	return refContent
}

// prepareParameter prepares the table content for each property
func (ref *MarkdownReference) prepareTerraformOutputs(tableName string, parameterList []ReferenceParameter) string {
	if len(parameterList) == 0 {
		return ""
	}
	refContent := fmt.Sprintf("\n\n%s\n\n", tableName)
	refContent += "Name | Description\n"
	refContent += "------------ | ------------- \n"

	for _, p := range parameterList {
		refContent += fmt.Sprintf(" %s | %s\n", p.Name, p.Usage)
	}

	return refContent
}

// prepareParameter prepares the table content for each property
func (ref *ParseReference) prepareParameter(tableName string, parameterList []ReferenceParameter, category types.CapabilityCategory) ConsoleReference {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetColWidth(100)
	table.SetHeader([]string{"Name", "Description", "Type", "Required", "Default"})
	switch category {
	case types.CUECategory:
		for _, p := range parameterList {
			if !p.Ignore {
				printableDefaultValue := ref.getCUEPrintableDefaultValue(p.Default)
				table.Append([]string{p.Name, p.Usage, p.PrintableType, strconv.FormatBool(p.Required), printableDefaultValue})
			}
		}
	case types.HelmCategory:
		for _, p := range parameterList {
			printableDefaultValue := ref.getJSONPrintableDefaultValue(p.JSONType, p.Default)
			table.Append([]string{p.Name, p.Usage, p.PrintableType, strconv.FormatBool(p.Required), printableDefaultValue})
		}
	case types.KubeCategory:
		for _, p := range parameterList {
			printableDefaultValue := ref.getJSONPrintableDefaultValue(p.JSONType, p.Default)
			refContent += fmt.Sprintf(" %s | %s | %s | %t | %s \n", p.Name, strings.ReplaceAll(p.Usage, "\n", ""), p.PrintableType, p.Required, printableDefaultValue)
		}
	case types.TerraformCategory:
		// Terraform doesn't have default value
		for _, p := range parameterList {
			table.Append([]string{p.Name, p.Usage, p.PrintableType, strconv.FormatBool(p.Required), ""})
		}
	default:
	}

	return ConsoleReference{TableName: tableName, TableObject: table}
}

// parseParameters parses every parameter
func (ref *ParseReference) parseParameters(paraValue cue.Value, paramKey string, depth int) error {
	var params []ReferenceParameter
	switch paraValue.Kind() {
	case cue.StructKind:
		arguments, err := paraValue.Struct()
		if err != nil {
			return fmt.Errorf("arguments not defined as struct %w", err)
		}
		if arguments.Len() == 0 {
			var param ReferenceParameter
			param.Name = "-"
			param.Required = true
			tl := paraValue.Template()
			if tl != nil { // is map type
				param.PrintableType = fmt.Sprintf("map[string]%s", tl("").IncompleteKind().String())
			}
			params = append(params, param)
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
				param.Default = velacue.GetDefault(def)
			}
			param.Short, param.Usage, param.Alias, param.Ignore = velacue.RetrieveComments(val)
			param.Type = val.IncompleteKind()
			switch val.IncompleteKind() {
			case cue.StructKind:
				if subField, _ := val.Struct(); subField.Len() == 0 { // err cannot be not nil,so ignore it
					if mapValue, ok := val.Elem(); ok {
						// In the future we could recursive call to surpport complex map-value(struct or list)
						param.PrintableType = fmt.Sprintf("map[string]%s", mapValue.IncompleteKind().String())
					} else {
						return fmt.Errorf("failed to got Map kind from %s", param.Name)
					}
				} else {
					if err := ref.parseParameters(val, name, depth+1); err != nil {
						return err
					}
					param.PrintableType = fmt.Sprintf("[%s](#%s)", name, name)
				}
			case cue.ListKind:
				elem, success := val.Elem()
				if !success {
					// fail to get elements, use the value of ListKind to be the type
					param.Type = val.Kind()
					param.PrintableType = val.IncompleteKind().String()
					break
				}
				switch elem.Kind() {
				case cue.StructKind:
					param.PrintableType = fmt.Sprintf("[[]%s](#%s)", name, name)
					if err := ref.parseParameters(elem, name, depth+1); err != nil {
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
		// markdown defines the contents that display in web
		tableName := fmt.Sprintf("%s %s", strings.Repeat("#", depth+3), paramKey)
		ref := MarkdownReference{}
		refContent = ref.prepareParameter(tableName, params, types.CUECategory) + refContent
	case "console":
		ref := ConsoleReference{}
		tableName := fmt.Sprintf("%s %s", strings.Repeat("#", depth+1), paramKey)
		console := ref.prepareParameter(tableName, params, types.CUECategory)
		propertyConsole = append([]ConsoleReference{console}, propertyConsole...)
	}
	return nil
}

// getCUEPrintableDefaultValue converts the value in `interface{}` type to be printable
func (ref *ParseReference) getCUEPrintableDefaultValue(v interface{}) string {
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

func (ref *ParseReference) getJSONPrintableDefaultValue(dataType string, value interface{}) string {
	if value != nil {
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}
	defaultValueMap := map[string]string{
		"number":  "0",
		"boolean": "false",
		"string":  "\"\"",
		"object":  "{}",
		"array":   "[]",
	}
	return defaultValueMap[dataType]
}

// generateSample generates Specification part for reference docs
func (ref *MarkdownReference) generateSample(capabilityName string) string {
	// TODO(zzxwill): we should generate the sample automatically instead of maintain hardcode example.
	if _, ok := ConfigurationYamlSample[capabilityName]; ok {
		return fmt.Sprintf("```yaml%s```", ConfigurationYamlSample[capabilityName])
	}
	return ""
}

// generateConflictWithAndMore generates Section `Conflicts With` and more like `How xxx works` in reference docs
func (ref *MarkdownReference) generateConflictWithAndMore(capabilityName string, referenceSourcePath string) (string, error) {
	conflictWithFile, err := filepath.Abs(filepath.Join(referenceSourcePath, "conflictsWithAndMore", fmt.Sprintf("%s.md", capabilityName)))
	if err != nil {
		return "", fmt.Errorf("failed to locate conflictWith file: %w", err)
	}
	data, err := os.ReadFile(filepath.Clean(conflictWithFile))
	if err != nil {
		return "", err
	}
	return "\n" + string(data), nil
}

// GenerateCUETemplateProperties get all properties of a capability
func (ref *ConsoleReference) GenerateCUETemplateProperties(capability *types.Capability) ([]ConsoleReference, error) {
	setDisplayFormat("console")
	capName := capability.Name

	cueValue, err := common.GetCUEParameterValue(capability.CueTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve `parameters` value from %s with err: %w", capName, err)
	}
	var defaultDepth = 0
	if err := ref.parseParameters(cueValue, "Properties", defaultDepth); err != nil {
		return nil, err
	}

	return propertyConsole, nil
}

// CommonReference contains parameters info of HelmCategory and KubuCategory type capability at present
type CommonReference struct {
	Name       string
	Parameters []ReferenceParameter
	Depth      int
}

// CommonSchema is a struct contains *openapi3.Schema style parameter
type CommonSchema struct {
	Name    string
	Schemas *openapi3.Schema
}

// GenerateHelmAndKubeProperties get all properties of a Helm/Kube Category type capability
func (ref *ParseReference) GenerateHelmAndKubeProperties(ctx context.Context, capability *types.Capability) ([]CommonReference, []ConsoleReference, error) {
	cmName := fmt.Sprintf("%s%s", types.CapabilityConfigMapNamePrefix, capability.Name)
	switch capability.Type {
	case types.TypeComponentDefinition:
		cmName = fmt.Sprintf("component-%s", cmName)
	case types.TypeTrait:
		cmName = fmt.Sprintf("trait-%s", cmName)
	default:
	}
	var cm v1.ConfigMap
	commonRefs = make([]CommonReference, 0)
	if err := ref.Client.Get(ctx, client.ObjectKey{Namespace: capability.Namespace, Name: cmName}, &cm); err != nil {
		return nil, nil, err
	}
	data, ok := cm.Data[types.OpenapiV3JSONSchema]
	if !ok {
		return nil, nil, errors.Errorf("configMap doesn't have openapi-v3-json-schema data")
	}
	parameterJSON := fmt.Sprintf(BaseOpenAPIV3Template, data)
	swagger, err := openapi3.NewSwaggerLoader().LoadSwaggerFromData(json.RawMessage(parameterJSON))
	if err != nil {
		return nil, nil, err
	}
	parameters := swagger.Components.Schemas[model.ParameterFieldName].Value
	WalkParameterSchema(parameters, "Properties", 0)

	var consoleRefs []ConsoleReference
	for _, item := range commonRefs {
		consoleRefs = append(consoleRefs, ref.prepareParameter(item.Name, item.Parameters, types.HelmCategory))
	}
	return commonRefs, consoleRefs, err
}

// GenerateTerraformCapabilityProperties generates Capability properties for Terraform ComponentDefinition
func (ref *ParseReference) parseTerraformCapabilityParameters(capability types.Capability) ([]ReferenceParameterTable, []ReferenceParameterTable, error) {
	var (
		tables                                       []ReferenceParameterTable
		refParameterList                             []ReferenceParameter
		writeConnectionSecretToRefReferenceParameter ReferenceParameter
		configuration                                string
		err                                          error
		outputsList                                  []ReferenceParameter
		outputsTables                                []ReferenceParameterTable
	)

	writeConnectionSecretToRefReferenceParameter.Name = terraform.TerraformWriteConnectionSecretToRefName
	writeConnectionSecretToRefReferenceParameter.PrintableType = terraform.TerraformWriteConnectionSecretToRefType
	writeConnectionSecretToRefReferenceParameter.Required = false
	writeConnectionSecretToRefReferenceParameter.Usage = terraform.TerraformWriteConnectionSecretToRefDescription

	if capability.ConfigurationType == "remote" {
		configuration, err = utils.GetTerraformConfigurationFromRemote(capability.Name, capability.TerraformConfiguration, capability.Path)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to retrieve Terraform configuration from %s: %w", capability.Name, err)
		}
	} else {
		configuration = capability.TerraformConfiguration
	}

	variables, outputs, err := common.ParseTerraformVariables(configuration)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to generate capability properties")
	}
	for _, v := range variables {
		var refParam ReferenceParameter
		refParam.Name = v.Name
		refParam.PrintableType = v.Type
		refParam.Usage = v.Description
		refParam.Required = true
		refParameterList = append(refParameterList, refParam)
	}
	refParameterList = append(refParameterList, writeConnectionSecretToRefReferenceParameter)

	propertiesTableName := fmt.Sprintf("%s %s", strings.Repeat("#", 3), "Properties")
	tables = append(tables, ReferenceParameterTable{
		Name:       propertiesTableName,
		Parameters: refParameterList,
	})

	var (
		writeSecretRefNameParam      ReferenceParameter
		writeSecretRefNameSpaceParam ReferenceParameter
	)

	// prepare `## writeConnectionSecretToRef`
	writeSecretRefNameParam.Name = "name"
	writeSecretRefNameParam.PrintableType = "string"
	writeSecretRefNameParam.Required = true
	writeSecretRefNameParam.Usage = terraform.TerraformSecretNameDescription

	writeSecretRefNameSpaceParam.Name = "namespace"
	writeSecretRefNameSpaceParam.PrintableType = "string"
	writeSecretRefNameSpaceParam.Required = false
	writeSecretRefNameSpaceParam.Usage = terraform.TerraformSecretNamespaceDescription

	writeSecretRefParameterList := []ReferenceParameter{writeSecretRefNameParam, writeSecretRefNameSpaceParam}
	writeSecretTableName := fmt.Sprintf("%s %s", strings.Repeat("#", 4), terraform.TerraformWriteConnectionSecretToRefName)

	tables = append(tables, ReferenceParameterTable{
		Name:       writeSecretTableName,
		Parameters: writeSecretRefParameterList,
	})

	// outputs
	for _, v := range outputs {
		var refParam ReferenceParameter
		refParam.Name = v.Name
		refParam.Usage = v.Description
		outputsList = append(outputsList, refParam)
	}

	outputsTableName := fmt.Sprintf("%s %s\n\nIf `writeConnectionSecretToRef` is set, a secret will be generated with these keys as below:", strings.Repeat("#", 3), "Outputs")

	outputsTables = append(outputsTables, ReferenceParameterTable{
		Name:       outputsTableName,
		Parameters: outputsList,
	})

	return tables, outputsTables, nil
}

// WalkParameterSchema will extract properties from *openapi3.Schema
func WalkParameterSchema(parameters *openapi3.Schema, name string, depth int) {
	if parameters == nil {
		return
	}
	var schemas []CommonSchema
	var commonParameters []ReferenceParameter
	for k, v := range parameters.Properties {
		p := ReferenceParameter{
			Parameter: types.Parameter{
				Name:     k,
				Default:  v.Value.Default,
				Usage:    v.Value.Description,
				JSONType: v.Value.Type,
			},
			PrintableType: v.Value.Type,
		}
		required := false
		for _, requiredType := range parameters.Required {
			if k == requiredType {
				required = true
				break
			}
		}
		p.Required = required
		if v.Value.Type == "object" {
			if v.Value.Properties != nil {
				schemas = append(schemas, CommonSchema{
					Name:    k,
					Schemas: v.Value,
				})
			}
			p.PrintableType = fmt.Sprintf("[%s](#%s)", k, k)
		}
		commonParameters = append(commonParameters, p)
	}

	commonRefs = append(commonRefs, CommonReference{
		Name:       fmt.Sprintf("%s %s", strings.Repeat("#", depth+1), name),
		Parameters: commonParameters,
		Depth:      depth + 1,
	})

	for _, schema := range schemas {
		WalkParameterSchema(schema.Schemas, schema.Name, depth+1)
	}
}

// GenerateTerraformCapabilityProperties generates Capability properties for Terraform ComponentDefinition in Cli console
func (ref *ConsoleReference) GenerateTerraformCapabilityProperties(capability types.Capability) ([]ConsoleReference, error) {
	var references []ConsoleReference

	variableTables, _, err := ref.parseTerraformCapabilityParameters(capability)
	if err != nil {
		return nil, err
	}
	for _, t := range variableTables {
		references = append(references, ref.prepareParameter(t.Name, t.Parameters, types.TerraformCategory))
	}
	return references, nil
}

// GenerateTerraformCapabilityPropertiesAndOutputs generates Capability properties and outputs for Terraform ComponentDefinition
func (ref *MarkdownReference) GenerateTerraformCapabilityPropertiesAndOutputs(capability types.Capability) (string, error) {
	var references string

	variableTables, outputsTable, err := ref.parseTerraformCapabilityParameters(capability)
	if err != nil {
		return "", err
	}
	for _, t := range variableTables {
		references += ref.prepareParameter(t.Name, t.Parameters, types.CUECategory)
	}
	for _, t := range outputsTable {
		references += ref.prepareTerraformOutputs(t.Name, t.Parameters)
	}
	return references, nil
}
