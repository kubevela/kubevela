/*
 Copyright 2022 The KubeVela Authors.

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
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/rogpeppe/go-internal/modfile"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/cue/packages"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	velacue "github.com/oam-dev/kubevela/pkg/cue"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	pkgdef "github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	pkgUtils "github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/terraform"
	"github.com/oam-dev/kubevela/references/docgen/fix"

	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

// ParseReference is used to include the common function `parseParameter`
type ParseReference struct {
	Client         client.Client
	I18N           *I18n        `json:"i18n"`
	Remote         *FromCluster `json:"remote"`
	Local          *FromLocal   `json:"local"`
	DefinitionName string       `json:"definitionName"`
	DisplayFormat  string
}

func (ref *ParseReference) getCapabilities(ctx context.Context, c common.Args) ([]types.Capability, error) {
	var (
		caps []types.Capability
		pd   *packages.PackageDiscover
	)
	switch {
	case ref.Local != nil:
		lcaps := make([]*types.Capability, 0)
		for _, path := range ref.Local.Paths {
			caps, err := ParseLocalFiles(path, c)
			if err != nil {
				return nil, fmt.Errorf("failed to get capability from local file %s: %w", path, err)
			}
			lcaps = append(lcaps, caps...)
		}
		for _, lcap := range lcaps {
			caps = append(caps, *lcap)
		}
	case ref.Remote != nil:
		config, err := c.GetConfig()
		if err != nil {
			return nil, err
		}
		pd, err = packages.NewPackageDiscover(config)
		if err != nil {
			return nil, err
		}
		ref.Remote.PD = pd
		if ref.DefinitionName == "" {
			caps, err = LoadAllInstalledCapability("default", c)
			if err != nil {
				return nil, fmt.Errorf("failed to get all capabilityes: %w", err)
			}
		} else {
			var rcap *types.Capability
			if ref.Remote.Rev == 0 {
				rcap, err = GetCapabilityByName(ctx, c, ref.DefinitionName, ref.Remote.Namespace, pd)
				if err != nil {
					return nil, fmt.Errorf("failed to get capability %s: %w", ref.DefinitionName, err)
				}
			} else {
				rcap, err = GetCapabilityFromDefinitionRevision(ctx, c, pd, ref.Remote.Namespace, ref.DefinitionName, ref.Remote.Rev)
				if err != nil {
					return nil, fmt.Errorf("failed to get revision %v of capability %s: %w", ref.Remote.Rev, ref.DefinitionName, err)
				}
			}
			caps = []types.Capability{*rcap}
		}
	default:
		return nil, fmt.Errorf("failed to get capability %s without namespace or local filepath", ref.DefinitionName)
	}
	return caps, nil
}

func (ref *ParseReference) prettySentence(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return ref.I18N.Get(s) + ref.I18N.Get(".")
}
func (ref *ParseReference) formatTableString(s string) string {
	return strings.ReplaceAll(s, "|", `&#124;`)
}

// prepareConsoleParameter prepares the table content for each property
// nolint:staticcheck
func (ref *ParseReference) prepareConsoleParameter(tableName string, parameterList []ReferenceParameter, category types.CapabilityCategory) ConsoleReference {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetColWidth(100)
	table.SetHeader([]string{ref.I18N.Get("Name"), ref.I18N.Get("Description"), ref.I18N.Get("Type"), ref.I18N.Get("Required"), ref.I18N.Get("Default")})
	switch category {
	case types.CUECategory:
		for _, p := range parameterList {
			if !p.Ignore {
				printableDefaultValue := ref.getCUEPrintableDefaultValue(p.Default)
				table.Append([]string{ref.I18N.Get(p.Name), ref.prettySentence(p.Usage), ref.I18N.Get(p.PrintableType), ref.I18N.Get(strconv.FormatBool(p.Required)), ref.I18N.Get(printableDefaultValue)})
			}
		}
	case types.HelmCategory, types.KubeCategory:
		for _, p := range parameterList {
			printableDefaultValue := ref.getJSONPrintableDefaultValue(p.JSONType, p.Default)
			table.Append([]string{ref.I18N.Get(p.Name), ref.prettySentence(p.Usage), ref.I18N.Get(p.PrintableType), ref.I18N.Get(strconv.FormatBool(p.Required)), ref.I18N.Get(printableDefaultValue)})
		}
	case types.TerraformCategory:
		// Terraform doesn't have default value
		for _, p := range parameterList {
			table.Append([]string{ref.I18N.Get(p.Name), ref.prettySentence(p.Usage), ref.I18N.Get(p.PrintableType), ref.I18N.Get(strconv.FormatBool(p.Required)), ""})
		}
	default:
	}

	return ConsoleReference{TableName: tableName, TableObject: table}
}

func cueValue2Ident(val cue.Value) *ast.Ident {
	var ident *ast.Ident
	if source, ok := val.Source().(*ast.Ident); ok {
		ident = source
	}
	if source, ok := val.Source().(*ast.Field); ok {
		if v, ok := source.Value.(*ast.Ident); ok {
			ident = v
		}
	}
	return ident
}

func getIndentName(val cue.Value) string {
	ident := cueValue2Ident(val)
	if ident != nil && len(ident.Name) != 0 {
		return strings.TrimPrefix(ident.Name, "#")
	}
	return val.IncompleteKind().String()
}

func getConcreteOrValueType(val cue.Value) string {
	op, elements := val.Expr()
	if op != cue.OrOp {
		return val.IncompleteKind().String()
	}
	var printTypes []string
	for _, ev := range elements {
		incompleteKind := ev.IncompleteKind().String()
		if !ev.IsConcrete() {
			return incompleteKind
		}
		ident := cueValue2Ident(ev)
		if ident != nil && len(ident.Name) != 0 {
			printTypes = append(printTypes, strings.TrimPrefix(ident.Name, "#"))
		} else {
			// only convert string in `or` operator for now
			opName, err := ev.String()
			if err != nil {
				return incompleteKind
			}
			opName = `"` + opName + `"`
			printTypes = append(printTypes, opName)
		}
	}
	return strings.Join(printTypes, " or ")
}

func getSuffix(capName string, containSuffix bool) (string, string) {
	var suffixTitle = " (" + capName + ")"
	var suffixRef = "-" + strings.ToLower(capName)
	if !containSuffix || capName == "" {
		suffixTitle = ""
		suffixRef = ""
	}
	return suffixTitle, suffixRef
}

// parseParameters parses every parameter to docs
// TODO(wonderflow): refactor the code to reduce the complexity
// nolint:staticcheck,gocyclo
func (ref *ParseReference) parseParameters(capName string, paraValue cue.Value, paramKey string, depth int, containSuffix bool) (string, []ConsoleReference, error) {
	var doc string
	var console []ConsoleReference
	var params []ReferenceParameter

	if !paraValue.Exists() {
		return "", console, nil
	}
	suffixTitle, suffixRef := getSuffix(capName, containSuffix)

	switch paraValue.Kind() {
	case cue.StructKind:
		arguments, err := paraValue.Struct()
		if err != nil {
			return "", nil, fmt.Errorf("field %s not defined as struct %w", paramKey, err)
		}

		if arguments.Len() == 0 {
			var param ReferenceParameter
			param.Name = "\\-"
			param.Required = true
			tl := paraValue.Template()
			if tl != nil { // is map type
				param.PrintableType = fmt.Sprintf("map[string]:%s", tl("").IncompleteKind().String())
			} else {
				param.PrintableType = "{}"
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
			name := fi.Selector
			param.Name = name
			if def, ok := val.Default(); ok && def.IsConcrete() {
				param.Default = velacue.GetDefault(def)
			}
			param.Required = !fi.IsOptional && (param.Default == nil)
			param.Short, param.Usage, param.Alias, param.Ignore = velacue.RetrieveComments(val)
			param.Type = val.IncompleteKind()
			switch val.IncompleteKind() {
			case cue.StructKind:
				if subField, err := val.Struct(); err == nil && subField.Len() == 0 { // err cannot be not nil,so ignore it
					if mapValue, ok := val.Elem(); ok {
						indentName := getIndentName(mapValue)
						_, err := mapValue.Fields()
						if err == nil {
							subDoc, subConsole, err := ref.parseParameters(capName, mapValue, indentName, depth+1, containSuffix)
							if err != nil {
								return "", nil, err
							}
							param.PrintableType = fmt.Sprintf("map[string]%s(#%s%s)", indentName, strings.ToLower(indentName), suffixRef)
							doc += subDoc
							console = append(console, subConsole...)
						} else {
							param.PrintableType = "map[string]" + mapValue.IncompleteKind().String()
						}
					} else {
						param.PrintableType = val.IncompleteKind().String()
					}
				} else {
					op, elements := val.Expr()
					if op == cue.OrOp {
						var printTypes []string
						for idx, ev := range elements {
							opName := getIndentName(ev)
							if opName == "struct" {
								opName = fmt.Sprintf("type-option-%d", idx+1)
							}
							subDoc, subConsole, err := ref.parseParameters(capName, ev, opName, depth+1, containSuffix)
							if err != nil {
								return "", nil, err
							}
							printTypes = append(printTypes, fmt.Sprintf("[%s](#%s%s)", opName, strings.ToLower(opName), suffixRef))
							doc += subDoc
							console = append(console, subConsole...)
						}
						param.PrintableType = strings.Join(printTypes, " or ")
					} else {
						subDoc, subConsole, err := ref.parseParameters(capName, val, name, depth+1, containSuffix)
						if err != nil {
							return "", nil, err
						}
						param.PrintableType = fmt.Sprintf("[%s](#%s%s)", name, strings.ToLower(name), suffixRef)
						doc += subDoc
						console = append(console, subConsole...)
					}
				}
			case cue.ListKind:
				elem := val.LookupPath(cue.MakePath(cue.AnyIndex))
				if !elem.Exists() {
					// fail to get elements, use the value of ListKind to be the type
					param.Type = val.Kind()
					param.PrintableType = val.IncompleteKind().String()
					break
				}
				switch elem.Kind() {
				case cue.StructKind:
					param.PrintableType = fmt.Sprintf("[[]%s](#%s%s)", name, strings.ToLower(name), suffixRef)
					subDoc, subConsole, err := ref.parseParameters(capName, elem, name, depth+1, containSuffix)
					if err != nil {
						return "", nil, err
					}
					doc += subDoc
					console = append(console, subConsole...)
				default:
					param.Type = elem.Kind()
					param.PrintableType = fmt.Sprintf("[]%s", elem.IncompleteKind().String())
				}
			default:
				param.PrintableType = getConcreteOrValueType(val)
			}
			params = append(params, param)
		}
	default:
		var param ReferenceParameter
		op, elements := paraValue.Expr()
		if op == cue.OrOp {
			var printTypes []string
			for idx, ev := range elements {
				opName := getIndentName(ev)
				if opName == "struct" {
					opName = fmt.Sprintf("type-option-%d", idx+1)
				}
				subDoc, subConsole, err := ref.parseParameters(capName, ev, opName, depth+1, containSuffix)
				if err != nil {
					return "", nil, err
				}
				printTypes = append(printTypes, fmt.Sprintf("[%s](#%s%s)", opName, strings.ToLower(opName), suffixRef))
				doc += subDoc
				console = append(console, subConsole...)
			}
			param.PrintableType = strings.Join(printTypes, " or ")
		} else {
			// TODO more composition type to be handle here
			param.Name = "--"
			param.Usage = "Unsupported Composition Type"
			param.PrintableType = extractTypeFromError(paraValue)
		}
		params = append(params, param)
	}

	switch ref.DisplayFormat {
	case Markdown, "":
		// markdown defines the contents that display in web
		var tableName string
		if paramKey != Specification {
			length := depth + 3
			if length >= 5 {
				length = 5
			}
			tableName = fmt.Sprintf("%s %s%s", strings.Repeat("#", length), paramKey, suffixTitle)
		}
		mref := MarkdownReference{}
		mref.I18N = ref.I18N
		doc = mref.getParameterString(tableName, params, types.CUECategory) + doc
	case Console:
		length := depth + 1
		if length >= 3 {
			length = 3
		}
		cref := ConsoleReference{}
		tableName := fmt.Sprintf("%s %s", strings.Repeat("#", length), paramKey)
		console = append([]ConsoleReference{cref.prepareConsoleParameter(tableName, params, types.CUECategory)}, console...)
	}
	return doc, console, nil
}

func extractTypeFromError(paraValue cue.Value) string {
	str, err := paraValue.String()
	if err == nil {
		return str
	}
	str = err.Error()
	sll := strings.Split(str, "cannot use value (")
	if len(sll) < 2 {
		return str
	}
	str = sll[1]
	sll = strings.Split(str, " (type")
	return sll[0]
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
	swagger, err := openapi3.NewLoader().LoadFromData(json.RawMessage(parameterJSON))
	if err != nil {
		return nil, nil, err
	}
	parameters := swagger.Components.Schemas[velaprocess.ParameterFieldName].Value
	WalkParameterSchema(parameters, Specification, 0)

	var consoleRefs []ConsoleReference
	for _, item := range commonRefs {
		consoleRefs = append(consoleRefs, ref.prepareConsoleParameter(item.Name, item.Parameters, types.HelmCategory))
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
		outputsTableName                             string
	)
	outputsTableName = fmt.Sprintf("%s %s\n\n%s", strings.Repeat("#", 3), ref.I18N.Get("Outputs"), ref.I18N.Get("WriteConnectionSecretToRefIntroduction"))

	writeConnectionSecretToRefReferenceParameter.Name = terraform.TerraformWriteConnectionSecretToRefName
	writeConnectionSecretToRefReferenceParameter.PrintableType = terraform.TerraformWriteConnectionSecretToRefType
	writeConnectionSecretToRefReferenceParameter.Required = false
	writeConnectionSecretToRefReferenceParameter.Usage = terraform.TerraformWriteConnectionSecretToRefDescription

	if capability.ConfigurationType == "remote" {
		var publicKey *gitssh.PublicKeys
		publicKey = nil
		if ref.Client != nil {
			compDefNamespacedName := k8stypes.NamespacedName{Name: capability.Name, Namespace: capability.Namespace}
			compDef := &v1beta1.ComponentDefinition{}
			ctx := context.Background()
			if err := ref.Client.Get(ctx, compDefNamespacedName, compDef); err != nil {
				return nil, nil, fmt.Errorf("failed to  get git component definition: %w", err)
			}
			gitCredentialsSecretReference := compDef.Spec.Schematic.Terraform.GitCredentialsSecretReference
			if gitCredentialsSecretReference != nil {
				publicKey, err = utils.GetGitSSHPublicKey(ctx, ref.Client, gitCredentialsSecretReference)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to  get publickey git credentials secret: %w", err)
				}
			}
		}
		configuration, err = utils.GetTerraformConfigurationFromRemote(capability.Name, capability.TerraformConfiguration, capability.Path, publicKey)
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
		refParam.PrintableType = strings.ReplaceAll(v.Type, "\n", `\n`)
		refParam.Usage = strings.ReplaceAll(v.Description, "\n", `\n`)
		refParam.Required = v.Required
		refParameterList = append(refParameterList, refParam)
	}
	refParameterList = append(refParameterList, writeConnectionSecretToRefReferenceParameter)
	sort.SliceStable(refParameterList, func(i, j int) bool {
		return refParameterList[i].Name < refParameterList[j].Name
	})

	tables = append(tables, ReferenceParameterTable{
		Name:       "",
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

	sort.SliceStable(writeSecretRefParameterList, func(i, j int) bool {
		return writeSecretRefParameterList[i].Name < writeSecretRefParameterList[j].Name
	})
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

	sort.SliceStable(outputsList, func(i, j int) bool {
		return outputsList[i].Name < outputsList[j].Name
	})
	outputsTables = append(outputsTables, ReferenceParameterTable{
		Name:       outputsTableName,
		Parameters: outputsList,
	})
	return tables, outputsTables, nil
}

// ParseLocalFiles parse the local files in directory and get name, configuration from local ComponentDefinition file
func ParseLocalFiles(localFilePath string, c common.Args) ([]*types.Capability, error) {
	lcaps := make([]*types.Capability, 0)
	if modfile.IsDirectoryPath(localFilePath) {
		// walk the dir and get files
		err := filepath.WalkDir(localFilePath, func(path string, info fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(info.Name(), ".yaml") && !strings.HasSuffix(info.Name(), ".cue") {
				return nil
			}
			// FIXME: remove this temporary fix when https://github.com/cue-lang/cue/issues/2047 is fixed
			if strings.Contains(path, "container-image") {
				lcaps = append(lcaps, fix.CapContainerImage)
				return nil
			}
			lcap, err := ParseLocalFile(path, c)
			if err != nil {
				return err
			}
			lcaps = append(lcaps, lcap)
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		lcap, err := ParseLocalFile(localFilePath, c)
		if err != nil {
			return nil, err
		}
		lcaps = append(lcaps, lcap)
	}
	return lcaps, nil
}

// ParseLocalFile parse the local file and get name, configuration from local ComponentDefinition file
func ParseLocalFile(localFilePath string, c common.Args) (*types.Capability, error) {
	data, err := pkgUtils.ReadRemoteOrLocalPath(localFilePath, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read local file or url")
	}

	if strings.HasSuffix(localFilePath, "yaml") {
		jsonData, err := yaml.YAMLToJSON(data)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert yaml data into k8s valid json format")
		}
		var localDefinition v1beta1.ComponentDefinition
		if err = json.Unmarshal(jsonData, &localDefinition); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal data into componentDefinition")
		}
		desc := localDefinition.ObjectMeta.Annotations["definition.oam.dev/description"]
		lcap := &types.Capability{
			Name:                   localDefinition.ObjectMeta.Name,
			Description:            desc,
			TerraformConfiguration: localDefinition.Spec.Schematic.Terraform.Configuration,
			ConfigurationType:      localDefinition.Spec.Schematic.Terraform.Type,
			Path:                   localDefinition.Spec.Schematic.Terraform.Path,
		}
		lcap.Type = types.TypeComponentDefinition
		lcap.Category = types.TerraformCategory
		return lcap, nil
	}

	// local definition for general definition in CUE format
	def := pkgdef.Definition{Unstructured: unstructured.Unstructured{}}
	config, err := c.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "get kubeconfig")
	}

	if err = def.FromCUEString(string(data), config); err != nil {
		return nil, errors.Wrapf(err, "failed to parse CUE for definition")
	}
	pd, err := c.GetPackageDiscover()
	if err != nil {
		klog.Warning("fail to build package discover, use local info instead", err)
	}
	mapper, err := c.GetDiscoveryMapper()
	if err != nil {
		klog.Warning("fail to build discover mapper, use local info instead", err)
	}
	lcap, err := ParseCapabilityFromUnstructured(mapper, pd, def.Unstructured)
	if err != nil {
		return nil, errors.Wrapf(err, "fail to parse definition to capability %s", def.GetName())
	}
	return &lcap, nil

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

// GetBaseResourceKinds helps get resource.group string of components' base resource
func GetBaseResourceKinds(cueStr string, pd *packages.PackageDiscover, dm discoverymapper.DiscoveryMapper) (string, error) {
	t, err := value.NewValue(cueStr+velacue.BaseTemplate, pd, "")
	if err != nil {
		return "", errors.Wrap(err, "failed to parse base template")
	}
	tmpl := t.CueValue()

	kindValue := tmpl.LookupPath(cue.ParsePath("output.kind"))
	kind, err := kindValue.String()
	if err != nil {
		return "", err
	}
	apiVersionValue := tmpl.LookupPath(cue.ParsePath("output.apiVersion"))
	apiVersion, err := apiVersionValue.String()
	if err != nil {
		return "", err
	}
	GroupAndVersion := strings.Split(apiVersion, "/")
	if len(GroupAndVersion) == 1 {
		GroupAndVersion = append([]string{""}, GroupAndVersion...)
	}
	gvr, err := dm.ResourcesFor(schema.GroupVersionKind{
		Group:   GroupAndVersion[0],
		Version: GroupAndVersion[1],
		Kind:    kind,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("- %s.%s", gvr.Resource, gvr.Group), nil
}
