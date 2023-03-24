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

package gen_sdk

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/encoding/openapi"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kubevela/pkg/util/slices"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	velacue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/stdlib"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

type byteHandler func([]byte) []byte

var (
	defaultAPIDir = map[string]string{
		"go": "pkg/apis",
	}
	// LangArgsRegistry is used to store the argument info
	LangArgsRegistry = map[string]map[langArgKey]LangArg{}
)

// GenMeta stores the metadata for generator.
type GenMeta struct {
	config *rest.Config
	name   string
	kind   string

	Output       string
	APIDirectory string
	IsSubModule  bool
	Lang         string
	Package      string
	Template     string
	File         []string
	InitSDK      bool
	Verbose      bool

	LangArgs LanguageArgs

	cuePaths     []string
	templatePath string
	packageFunc  byteHandler
}

// Generator is used to generate SDK code from CUE template for one language.
type Generator struct {
	meta          *GenMeta
	def           definition.Definition
	openapiSchema []byte
	// defModifiers are the modifiers for each definition.
	defModifiers []Modifier
	// moduleModifiers are the modifiers for the whole module. It will be executed after generating all definitions.
	moduleModifiers []Modifier
}

// LanguageArgs is used to store the arguments for the language.
type LanguageArgs interface {
	Get(key langArgKey) string
	Set(key langArgKey, value string)
}

// langArgKey is language argument key.
type langArgKey string

// LangArg is language-specific argument.
type LangArg struct {
	Name    langArgKey
	Desc    string
	Default string
}

// registerLangArg should be called in init() function of each language.
func registerLangArg(lang string, arg ...LangArg) {
	if _, ok := LangArgsRegistry[lang]; !ok {
		LangArgsRegistry[lang] = map[langArgKey]LangArg{}
	}
	for _, a := range arg {
		LangArgsRegistry[lang][a.Name] = a
	}
}

// NewLanguageArgs parses the language arguments and returns a LanguageArgs.
func NewLanguageArgs(lang string, langArgs []string) (LanguageArgs, error) {
	availableArgs := LangArgsRegistry[lang]
	res := languageArgs{}
	for _, arg := range langArgs {
		parts := strings.Split(arg, "=")
		if len(parts) != 2 {
			return nil, errors.Errorf("argument %s is not in the format of key=value", arg)
		}
		if _, ok := availableArgs[langArgKey(parts[0])]; !ok {
			return nil, errors.Errorf("argument %s is not supported for language %s", parts[0], lang)
		}
		res.Set(langArgKey(parts[0]), parts[1])
	}
	for k, v := range availableArgs {
		if res.Get(k) == "" {
			res.Set(k, v.Default)
		}
	}
	return res, nil
}

type languageArgs map[string]string

func (l languageArgs) Get(key langArgKey) string {
	return l[string(key)]
}

func (l languageArgs) Set(key langArgKey, value string) {
	l[string(key)] = value
}

// Modifier is used to modify the generated code.
type Modifier interface {
	Modify() error
	Name() string
}

// Init initializes the generator.
// It will validate the param, analyze the CUE files, read them to memory, mkdir for output.
func (meta *GenMeta) Init(c common.Args, langArgs []string) (err error) {
	meta.config, err = c.GetConfig()
	if err != nil {
		klog.Info("No kubeconfig found, skipping")
	}
	err = stdlib.SetupBuiltinImports()
	if err != nil {
		return err
	}
	if _, ok := SupportedLangs[meta.Lang]; !ok {
		return fmt.Errorf("language %s is not supported", meta.Lang)
	}

	// Init arguments
	if meta.APIDirectory == "" {
		meta.APIDirectory = defaultAPIDir[meta.Lang]
	}

	meta.LangArgs, err = NewLanguageArgs(meta.Lang, langArgs)
	if err != nil {
		return err
	}
	packageFuncs := map[string]byteHandler{
		"go": func(b []byte) []byte {
			return bytes.ReplaceAll(b, []byte(PackagePlaceHolder), []byte(meta.Package))
		},
	}

	meta.packageFunc = packageFuncs[meta.Lang]

	// Analyze the all cue files from meta.File. It can be file or directory. If directory is given, it will recursively
	// analyze all cue files in the directory.
	for _, f := range meta.File {
		info, err := os.Stat(f)
		if err != nil {
			return err
		}
		if info.IsDir() {
			err = filepath.Walk(f, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() && strings.HasSuffix(path, ".cue") {
					meta.cuePaths = append(meta.cuePaths, path)
				}
				return nil
			})
			if err != nil {
				return err
			}
		} else if strings.HasSuffix(f, ".cue") {
			meta.cuePaths = append(meta.cuePaths, f)
		}

	}
	return os.MkdirAll(meta.Output, 0750)
}

// CreateScaffold will create a scaffold for the given language.
// It will copy all files from embedded scaffold/{meta.Lang} to meta.Output.
func (meta *GenMeta) CreateScaffold() error {
	if !meta.InitSDK {
		return nil
	}
	klog.Info("Flag --init is set, creating scaffold...")
	langDirPrefix := fmt.Sprintf("%s/%s", ScaffoldDir, meta.Lang)
	err := fs.WalkDir(Scaffold, ScaffoldDir, func(_path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if !strings.HasPrefix(_path, langDirPrefix) && _path != ScaffoldDir {
				return fs.SkipDir
			}
			return nil
		}
		fileContent, err := Scaffold.ReadFile(_path)
		if err != nil {
			return err
		}
		fileContent = meta.packageFunc(fileContent)
		fileName := path.Join(meta.Output, strings.TrimPrefix(_path, langDirPrefix))
		// go.mod_ is a special file name, it will be renamed to go.mod. Go will ignore directory containing go.mod during the build process.
		fileName = strings.ReplaceAll(fileName, "go.mod_", "go.mod")
		fileDir := path.Dir(fileName)
		if err = os.MkdirAll(fileDir, 0750); err != nil {
			return err
		}
		return os.WriteFile(fileName, fileContent, 0600)
	})
	return err
}

// PrepareGeneratorAndTemplate will make a copy of the embedded openapi-generator-cli and templates/{meta.Lang} to local
func (meta *GenMeta) PrepareGeneratorAndTemplate() error {
	var err error
	ogImageName := "openapitools/openapi-generator-cli"
	ogImageTag := "v6.3.0"
	ogImage := fmt.Sprintf("%s:%s", ogImageName, ogImageTag)
	homeDir, err := system.GetVelaHomeDir()
	if err != nil {
		return err
	}
	sdkDir := path.Join(homeDir, "sdk")
	if err = os.MkdirAll(sdkDir, 0750); err != nil {
		return err
	}

	// nolint:gosec
	output, err := exec.Command("docker", "image", "ls", ogImage).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "failed to check image %s: %s", ogImage, output)
	}
	if !strings.Contains(string(output), ogImageName) {
		// nolint:gosec
		output, err = exec.Command("docker", "pull", ogImage).CombinedOutput()
		if err != nil {
			return errors.Wrapf(err, "failed to pull %s: %s", ogImage, output)
		}
	}

	// copy embedded templates/{meta.Lang} to sdkDir
	if meta.Template == "" {
		langDir := path.Join(sdkDir, "templates", meta.Lang)
		if err = os.MkdirAll(langDir, 0750); err != nil {
			return err
		}
		langTemplateDir := path.Join("openapi-generator", "templates", meta.Lang)
		langTemplateFiles, err := Templates.ReadDir(langTemplateDir)
		if err != nil {
			return err
		}
		for _, langTemplateFile := range langTemplateFiles {
			src, err := Templates.Open(path.Join(langTemplateDir, langTemplateFile.Name()))
			if err != nil {
				return err
			}
			// nolint:gosec
			dst, err := os.Create(path.Join(langDir, langTemplateFile.Name()))
			if err != nil {
				return err
			}
			_, err = io.Copy(dst, src)
			_ = dst.Close()
			_ = src.Close()
			if err != nil {
				return err
			}
		}
		meta.templatePath = langDir
	} else {
		meta.templatePath, err = filepath.Abs(meta.Template)
		if err != nil {
			return errors.Wrap(err, "failed to get absolute path of template")
		}
	}
	return nil
}

// Run will generally do two thing:
// 1. Generate OpenAPI schema from cue files
// 2. Generate code from OpenAPI schema
func (meta *GenMeta) Run() error {
	g := NewModifiableGenerator(meta)
	if len(meta.cuePaths) == 0 {
		return nil
	}
	APIGenerated := false
	for _, cuePath := range meta.cuePaths {
		klog.Infof("Generating API for %s", cuePath)
		// nolint:gosec
		cueBytes, err := os.ReadFile(cuePath)
		if err != nil {
			return errors.Wrapf(err, "failed to read %s", cuePath)
		}
		template, defName, defKind, err := g.GetDefinitionValue(cueBytes)
		if err != nil {
			return err
		}
		g.meta.SetDefinition(defName, defKind)

		err = g.GenOpenAPISchema(template)
		if err != nil {
			if strings.Contains(err.Error(), "unsupported node string (*ast.Ident)") {
				// https://github.com/cue-lang/cue/issues/2259
				klog.Warningf("Skip generating OpenAPI schema for %s, known issue: %s", cuePath, err.Error())
				continue
			}
			return errors.Wrapf(err, "generate OpenAPI schema")
		}

		err = g.GenerateCode()
		if err != nil {
			return err
		}
		APIGenerated = true
	}
	if !APIGenerated {
		return nil
	}
	for _, m := range g.moduleModifiers {
		err := m.Modify()
		if err != nil {
			return err
		}
	}

	return nil
}

// SetDefinition sets definition name and kind
func (meta *GenMeta) SetDefinition(defName, defKind string) {
	meta.name = defName
	meta.kind = defKind
}

// GetDefinitionValue returns a value.Value definition name, definition kind from cue bytes
func (g *Generator) GetDefinitionValue(cueBytes []byte) (*value.Value, string, string, error) {
	g.def = definition.Definition{Unstructured: unstructured.Unstructured{}}
	if err := g.def.FromCUEString(string(cueBytes), g.meta.config); err != nil {
		return nil, "", "", errors.Wrapf(err, "failed to parse CUE")
	}

	templateString, _, err := unstructured.NestedString(g.def.Object, definition.DefinitionTemplateKeys...)
	if err != nil {
		return nil, "", "", err
	}
	if templateString == "" {
		return nil, "", "", errors.New("definition doesn't include cue schematic")
	}
	template, err := value.NewValue(templateString+velacue.BaseTemplate, nil, "")
	if err != nil {
		return nil, "", "", err
	}
	return template, g.def.GetName(), g.def.GetKind(), nil
}

// GenOpenAPISchema generates OpenAPI json schema from cue.Instance
func (g *Generator) GenOpenAPISchema(val *value.Value) error {
	var err error
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("invalid cue definition to generate open api: %v", r)
			debug.PrintStack()
			return
		}
	}()
	if val.CueValue().Err() != nil {
		return val.CueValue().Err()
	}
	paramOnlyVal, err := common.RefineParameterValue(val)
	if err != nil {
		return err
	}
	defaultConfig := &openapi.Config{ExpandReferences: false, NameFunc: func(val cue.Value, path cue.Path) string {
		sels := path.Selectors()
		lastLabel := sels[len(sels)-1].String()
		return strings.TrimPrefix(lastLabel, "#")
	}, DescriptionFunc: func(v cue.Value) string {
		for _, d := range v.Doc() {
			if strings.HasPrefix(d.Text(), "+usage=") {
				return strings.TrimPrefix(d.Text(), "+usage=")
			}
		}
		return ""
	}}
	b, err := openapi.Gen(paramOnlyVal, defaultConfig)
	if err != nil {
		return err
	}
	doc, err := openapi3.NewLoader().LoadFromData(b)
	if err != nil {
		return err
	}

	g.completeOpenAPISchema(doc)
	openapiSchema, err := doc.MarshalJSON()
	g.openapiSchema = openapiSchema
	if g.meta.Verbose {
		klog.Info("OpenAPI schema:")
		klog.Info(string(g.openapiSchema))
	}
	return err
}

func (g *Generator) completeOpenAPISchema(doc *openapi3.T) {
	for key, schema := range doc.Components.Schemas {
		switch key {
		case "parameter":
			spec := g.meta.name + "-spec"
			schema.Value.Title = spec
			completeFreeFormSchema(schema)
			completeSchema(key, schema)
			doc.Components.Schemas[spec] = schema
			delete(doc.Components.Schemas, key)
		case g.meta.name + "-spec":
			continue
		default:
			completeSchema(key, schema)
		}
	}
}

// GenerateCode will call openapi-generator to generate code and modify it
func (g *Generator) GenerateCode() (err error) {
	tmpFile, err := os.CreateTemp("", g.meta.name+"-*.json")
	_, err = tmpFile.Write(g.openapiSchema)
	if err != nil {
		return errors.Wrap(err, "write openapi schema to temporary file")
	}
	defer func() {
		_ = tmpFile.Close()
		if err == nil {
			_ = os.Remove(tmpFile.Name())
		}
	}()
	apiDir, err := filepath.Abs(path.Join(g.meta.Output, g.meta.APIDirectory))
	if err != nil {
		return errors.Wrapf(err, "get absolute path of %s", apiDir)
	}
	err = os.MkdirAll(path.Join(apiDir, definition.DefinitionKindToType[g.meta.kind]), 0750)
	if err != nil {
		return errors.Wrapf(err, "create directory %s", apiDir)
	}

	// nolint:gosec
	cmd := exec.Command("docker", "run",
		"-v", fmt.Sprintf("%s:/local/output", apiDir),
		"-v", fmt.Sprintf("%s:/local/input", filepath.Dir(tmpFile.Name())),
		"-v", fmt.Sprintf("%s:/local/template", g.meta.templatePath),
		"-u", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"--rm",
		"openapitools/openapi-generator-cli:v6.3.0",
		"generate",
		"-i", "/local/input/"+filepath.Base(tmpFile.Name()),
		"-g", g.meta.Lang,
		"-o", fmt.Sprintf("/local/output/%s/%s", definition.DefinitionKindToType[g.meta.kind], g.meta.name),
		"-t", "/local/template",
		"--skip-validate-spec",
		"--enable-post-process-file",
		"--generate-alias-as-model",
		"--inline-schema-name-defaults", "arrayItemSuffix=,mapItemSuffix=",
		"--additional-properties", fmt.Sprintf("packageName=%s", strings.ReplaceAll(g.meta.name, "-", "_")),
		"--global-property", "modelDocs=false,models,supportingFiles=utils.go",
	)
	if g.meta.Verbose {
		klog.Info(cmd.String())
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrap(err, string(output))
	}
	if g.meta.Verbose {
		klog.Info(string(output))
	}

	// Adjust the generated files and code
	for _, m := range g.defModifiers {
		err := m.Modify()
		if err != nil {
			return errors.Wrapf(err, "modify fail by %s", m.Name())
		}
	}
	return nil
}

// completeFreeFormSchema can complete the schema of free form parameter, such as `parameter: {...}`
// This is a workaround for openapi-generator, which won't generate the correct code for free form parameter.
func completeFreeFormSchema(schema *openapi3.SchemaRef) {
	v := schema.Value
	if v.OneOf == nil && v.AnyOf == nil && v.AllOf == nil && v.Properties == nil {
		if v.Type == openapi3.TypeObject {
			schema.Value.AdditionalProperties = &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type:     openapi3.TypeObject,
					Nullable: true,
				},
			}
		} else if v.Type == "string" {
			schema.Value.AdditionalProperties = &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: "string",
				},
			}
		}
	}
}

// fixSchemaWithOneOf do some fix for schema with OneOf.
// 1. move properties in the root schema to sub schema in OneOf. See https://github.com/OpenAPITools/openapi-generator/issues/14250
// 2. move default value to sub schema in OneOf.
// 3. remove duplicated type in OneOf.
func fixSchemaWithOneOf(schema *openapi3.SchemaRef) {
	var schemaNeedFix []*openapi3.Schema

	oneOf := schema.Value.OneOf
	typeSet := make(map[string]struct{})
	duplicateIndex := make([]int, 0)
	// If the schema have default value, it should be moved to sub-schema with right type.
	defaultValue := schema.Value.Default
	schema.Value.Default = nil
	for _, s := range oneOf {
		completeSchemas(s.Value.Properties)
		if defaultValueMatchOneOfItem(s.Value, defaultValue) {
			s.Value.Default = defaultValue
		}

		// If the schema is without type or ref. It may need to be fixed.
		// Cases can be:
		// 1. A non-ref sub-schema maybe have no properties and the needed properties is in the root schema.
		// 2. A sub-schema maybe have no type and the needed type is in the root schema.
		// In both cases, we need to complete the sub-schema with the properties or type in the root schema if any of them is missing.
		if s.Value.Properties == nil || s.Value.Type == "" {
			schemaNeedFix = append(schemaNeedFix, s.Value)
		}
	}

	if schemaNeedFix == nil {
		return // no non-ref schema found
	}
	for _, s := range schemaNeedFix {
		if s.Properties == nil {
			s.Properties = schema.Value.Properties
		}
		if s.Type == "" {
			s.Type = schema.Value.Type
		}
	}
	schema.Value.Properties = nil

	// remove duplicated type
	for i, s := range oneOf {
		if s.Value.Type == "" {
			continue
		}
		if _, ok := typeSet[s.Value.Type]; ok && s.Value.Type != openapi3.TypeObject {
			duplicateIndex = append(duplicateIndex, i)
		} else {
			typeSet[s.Value.Type] = struct{}{}
		}
	}
	if len(duplicateIndex) > 0 {
		newRefs := make(openapi3.SchemaRefs, 0, len(oneOf)-len(duplicateIndex))
		for i, s := range oneOf {
			if !slices.Contains(duplicateIndex, i) {
				newRefs = append(newRefs, s)
			}
		}
		schema.Value.OneOf = newRefs
	}

}

func completeSchema(key string, schema *openapi3.SchemaRef) {
	schema.Value.Title = key
	if schema.Value.OneOf != nil {
		fixSchemaWithOneOf(schema)
		return
	}

	// allow all the fields to be empty to avoid this case:
	// A field is initialized with empty value and marshalled to JSON with empty value (e.g. empty string)
	// However, the empty value is not allowed on the server side when it is conflict with the default value in CUE.
	// schema.Value.Required = []string{}

	switch schema.Value.Type {
	case openapi3.TypeObject:
		completeSchemas(schema.Value.Properties)
	case openapi3.TypeArray:
		completeSchema(key, schema.Value.Items)
	}

}

func completeSchemas(schemas openapi3.Schemas) {
	for k, schema := range schemas {
		completeSchema(k, schema)
	}
}

// NewModifiableGenerator returns a new Generator with modifiers
func NewModifiableGenerator(meta *GenMeta) *Generator {
	g := &Generator{
		meta:            meta,
		defModifiers:    []Modifier{},
		moduleModifiers: []Modifier{},
	}
	appendModifiersByLanguage(g, meta)
	return g
}

func appendModifiersByLanguage(g *Generator, meta *GenMeta) {
	switch meta.Lang {
	case "go":
		g.defModifiers = append(g.defModifiers, &GoDefModifier{GenMeta: meta})
		g.moduleModifiers = append(g.moduleModifiers, &GoModuleModifier{GenMeta: meta})
	default:
		panic(fmt.Sprintf("unsupported language: %s", meta.Lang))
	}
}

// getValueType returns the cue type of the value
func getValueType(i interface{}) CUEType {
	if i == nil {
		return ""
	}
	switch i.(type) {
	case string:
		return "string"
	case int:
		return "integer"
	case float64, float32:
		return "number"
	case bool:
		return "boolean"
	case map[string]interface{}:
		return "object"
	case []interface{}:
		return "array"
	default:
		return ""
	}
}

// CUEType is the possible types in CUE
type CUEType string

func (t CUEType) fit(schema *openapi3.Schema) bool {
	openapiType := schema.Type
	switch t {
	case "string":
		return openapiType == "string"
	case "integer":
		return openapiType == "integer" || openapiType == "number"
	case "number":
		return openapiType == "number"
	case "boolean":
		return openapiType == "boolean"
	case "array":
		return openapiType == "array"
	default:
		return false
	}
}

// defaultValueMatchOneOfItem checks if the default value matches one of the items in the oneOf schema.
func defaultValueMatchOneOfItem(item *openapi3.Schema, defaultValue interface{}) bool {
	if item.Default != nil {
		return false
	}
	defaultValueType := getValueType(defaultValue)
	// let's skip the case that default value is object because it's hard to match now.
	if defaultValueType == "" || defaultValueType == openapi3.TypeObject {
		return false
	}
	if defaultValueType != "" && defaultValueType.fit(item) && item.Default == nil {
		return true
	}
	return false
}

func fnName(fn interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}
