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

package docgen

import (
	"context"
	"fmt"

	"github.com/olekukonko/tablewriter"

	"github.com/kubevela/workflow/pkg/cue/packages"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
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

// FromCluster is the struct for input Namespace
type FromCluster struct {
	Namespace string `json:"namespace"`
	Rev       int64  `json:"revision"`
	PD        *packages.PackageDiscover
}

// FromLocal is the struct for input Definition Path
type FromLocal struct {
	Paths []string `json:"paths"`
}

// ConsoleReference is the struct for capability information in console
type ConsoleReference struct {
	ParseReference
	TableName   string             `json:"tableName"`
	TableObject *tablewriter.Table `json:"tableObject"`
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

var commonRefs []CommonReference

// GenerateCUETemplateProperties get all properties of a capability
func (ref *ConsoleReference) GenerateCUETemplateProperties(capability *types.Capability, pd *packages.PackageDiscover) (string, []ConsoleReference, error) {
	ref.DisplayFormat = "console"
	capName := capability.Name

	cueValue, err := common.GetCUEParameterValue(capability.CueTemplate, pd)
	if err != nil {
		return "", nil, fmt.Errorf("failed to retrieve `parameters` value from %s with err: %w", capName, err)
	}
	var defaultDepth = 0
	doc, console, err := ref.parseParameters(capName, cueValue, Specification, defaultDepth, false)
	if err != nil {
		return "", nil, err
	}
	return doc, console, nil
}

// GenerateTerraformCapabilityProperties generates Capability properties for Terraform ComponentDefinition in Cli console
func (ref *ConsoleReference) GenerateTerraformCapabilityProperties(capability types.Capability) ([]ConsoleReference, error) {
	var references []ConsoleReference

	variableTables, _, err := ref.parseTerraformCapabilityParameters(capability)
	if err != nil {
		return nil, err
	}
	for _, t := range variableTables {
		references = append(references, ref.prepareConsoleParameter(t.Name, t.Parameters, types.TerraformCategory))
	}
	return references, nil
}

// Show will show capability reference in console
func (ref *ConsoleReference) Show(ctx context.Context, c common.Args, ioStreams cmdutil.IOStreams, capabilityName string, ns string, rev int64) error {
	caps, err := ref.getCapabilities(ctx, c)
	if err != nil {
		return err
	}
	if len(caps) < 1 {
		return fmt.Errorf("no capability found with name %s namespace %s", capabilityName, ns)
	}
	capability := &caps[0]
	var propertyConsole []ConsoleReference
	switch capability.Category {
	case types.CUECategory:
		var pd *packages.PackageDiscover
		if ref.Remote != nil {
			pd = ref.Remote.PD
		}
		_, propertyConsole, err = ref.GenerateCUETemplateProperties(capability, pd)
		if err != nil {
			return err
		}
	case types.TerraformCategory:
		propertyConsole, err = ref.GenerateTerraformCapabilityProperties(*capability)
		if err != nil {
			return err
		}
	case types.HelmCategory, types.KubeCategory:
		_, propertyConsole, err = ref.GenerateHelmAndKubeProperties(ctx, capability)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupport capability category %s", capability.Category)
	}

	for _, p := range propertyConsole {
		ioStreams.Info(p.TableName)
		p.TableObject.Render()
		ioStreams.Info("\n")
	}
	return nil
}
