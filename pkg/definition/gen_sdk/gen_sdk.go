package gen_sdk

import (
	"fmt"
	"github.com/oam-dev/kubevela/pkg/definition"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime/debug"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/encoding/openapi"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	velacue "github.com/oam-dev/kubevela/pkg/cue"
	openapigenerator "github.com/oam-dev/kubevela/pkg/definition/openapi-generator"
	"github.com/oam-dev/kubevela/pkg/stdlib"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/system"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
)

type GenMeta struct {
	config *rest.Config

	Output   string
	Lang     string
	Template string
	File     []string
	Verbose  bool

	cuePaths      []string
	generatorPath string
	templatePath  string
}

// Generator is used to generate SDK code from CUE template for one language.
type Generator struct {
	meta          *GenMeta
	name          string
	kind          string
	def           definition.Definition
	openapiSchema []byte
	modifiers     []Modifier
}

type Modifier interface {
	Modify() error
	Name() string
}

func (meta *GenMeta) Init(c common.Args) (err error) {
	meta.config, err = c.GetConfig()
	if err != nil {
		return err
	}
	err = stdlib.SetupBuiltinImports()
	if err != nil {
		return err
	}
	if _, ok := openapigenerator.SupportedLangs[meta.Lang]; !ok {
		return fmt.Errorf("language %s is not supported", meta.Lang)
	}

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

	return nil
}

func (meta *GenMeta) PrepareGeneratorAndTemplate() error {
	var err error
	homeDir, err := system.GetVelaHomeDir()
	if err != nil {
		return err
	}
	sdkDir := path.Join(homeDir, "sdk")
	if err = os.MkdirAll(sdkDir, 0755); err != nil {
		return err
	}

	meta.generatorPath = path.Join(sdkDir, "openapi-generator")
	err = os.WriteFile(meta.generatorPath, openapigenerator.OpenapiGenerator, 0744)
	if err != nil {
		return err
	}

	// copy embeded templates/{meta.Lang} to sdkDir
	if meta.Template == "" {
		langDir := path.Join(sdkDir, "templates", meta.Lang)
		if err = os.MkdirAll(langDir, 0755); err != nil {
			return err
		}
		langTemplateDir := path.Join("templates", meta.Lang)
		langTemplateFiles, err := openapigenerator.Tempaltes.ReadDir(langTemplateDir)
		if err != nil {
			return err
		}
		for _, langTemplateFile := range langTemplateFiles {
			src, err := openapigenerator.Tempaltes.Open(path.Join(langTemplateDir, langTemplateFile.Name()))
			if err != nil {
				return err
			}
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
		meta.templatePath = meta.Template
	}
	return nil
}

func (meta *GenMeta) Run() error {
	for _, cuePath := range meta.cuePaths {
		fmt.Println("Generating SDK for", cuePath)
		g := NewModifiableGenerator(meta)
		cueBytes, err := os.ReadFile(cuePath)
		if err != nil {
			return errors.Wrapf(err, "failed to read %s", cuePath)
		}
		template, err := g.GetDefinitionValue(cueBytes)
		if err != nil {
			return err
		}
		err = g.GenOpenAPISchema(template)
		if err != nil {
			return errors.Wrapf(err, "generate OpenAPI schema")
		}
		err = g.GenerateCode()
		if err != nil {
			return err
		}
	}
	return nil
}

func (g *Generator) GetDefinitionValue(cueBytes []byte) (*value.Value, error) {
	g.def = definition.Definition{Unstructured: unstructured.Unstructured{}}
	if err := g.def.FromCUEString(string(cueBytes), g.meta.config); err != nil {
		return nil, errors.Wrapf(err, "failed to parse CUE")
	}
	g.name = g.def.GetName()
	g.kind = g.def.GetKind()

	templateString, _, err := unstructured.NestedString(g.def.Object, definition.DefinitionTemplateKeys...)
	if err != nil {
		return nil, err
	}
	template, err := value.NewValue(templateString+velacue.BaseTemplate, nil, "")
	if err != nil {
		return nil, err
	}
	return template, nil
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
	return err
}

func (g *Generator) completeOpenAPISchema(doc *openapi3.T) {
	for key, schema := range doc.Components.Schemas {
		switch key {
		case "parameter":
			spec := g.name + "-spec"
			schema.Value.Title = spec
			completeFreeFormSchema(schema)
			completeSchemas(schema.Value.Properties)
			doc.Components.Schemas[spec] = schema
			delete(doc.Components.Schemas, key)
		default:
			completeSchema(key, schema)
		}
	}
}

func (g *Generator) GenerateCode() (err error) {
	tmpFile, err := os.CreateTemp("/tmp", g.name+"-*.json")
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
	defDir := path.Join(g.meta.Output, definition.DefinitionKindToType[g.kind], g.name)

	cmd := exec.Command(g.meta.generatorPath, "generate",
		"-i", tmpFile.Name(),
		"-g", g.meta.Lang,
		"-o", defDir,
		"-t", g.meta.templatePath,
		"--skip-validate-spec",
		"--enable-post-process-file",
		"--generate-alias-as-model",
		"--inline-schema-name-defaults", "arrayItemSuffix=,mapItemSuffix=",
		"--additional-properties", fmt.Sprintf("isGoSubmodule=true,packageName=%s", strings.ReplaceAll(g.name, "-", "_")),
		"--global-property", "modelDocs=false,models,supportingFiles=utils.go",
	)
	if g.meta.Verbose {
		fmt.Println(cmd.String())
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrap(err, string(output))
	}
	if g.meta.Verbose {
		fmt.Println(string(output))
	}

	// Adjust the generated files and code
	for _, m := range g.modifiers {
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
		if v.Type == "object" {
			schema.Value.AdditionalProperties = &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type:     "object",
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

// fixSchemaWithOneAnyAllOf move properties in the root schema to sub schema in OneOf, AnyOf, AllOf
// See https://github.com/OpenAPITools/openapi-generator/issues/14250
func fixSchemaWithOneAnyAllOf(schema *openapi3.SchemaRef) {
	var schemaNeedFix []*openapi3.Schema
	refs := []openapi3.SchemaRefs{schema.Value.OneOf, schema.Value.AnyOf, schema.Value.AllOf}
	for _, ref := range refs {
		if ref != nil {
			for _, s := range ref {
				completeSchemas(s.Value.Properties)
				// If the schema is without type or ref. It may need to be fixed.
				// Cases can be:
				// 1. A non-ref sub-schema maybe have no properties and the needed properties is in the root schema.
				// 2. A sub-schema maybe have no type and the needed type is in the root schema.
				// In both cases, we need to complete the sub-schema with the properties or type in the root schema if any of them is missing.
				if s.Ref == "" || s.Value.Type == "" {
					schemaNeedFix = append(schemaNeedFix, s.Value)
				}
			}
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
}

func completeSchema(key string, schema *openapi3.SchemaRef) {
	schema.Value.Title = key
	if schema.Value.OneOf != nil || schema.Value.AnyOf != nil || schema.Value.AllOf != nil {
		fixSchemaWithOneAnyAllOf(schema)
		return
	}

	switch schema.Value.Type {
	case "object":
		completeSchemas(schema.Value.Properties)
	case "array":
		completeSchema(key, schema.Value.Items)
	}

}

func completeSchemas(schemas openapi3.Schemas) {
	for k, schema := range schemas {
		completeSchema(k, schema)
	}
}

func NewModifiableGenerator(meta *GenMeta) *Generator {
	g := &Generator{
		meta:      meta,
		modifiers: []Modifier{},
	}
	mo := newModifierOnLanguage(meta.Lang, g)
	g.modifiers = append(g.modifiers, mo)
	return g
}

func newModifierOnLanguage(lang string, generator *Generator) Modifier {
	switch lang {
	case "go":
		return &GoModifier{g: generator}
	default:
		panic("unsupported language: " + lang)
	}
}
