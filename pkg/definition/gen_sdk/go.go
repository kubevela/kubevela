package gen_sdk

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"

	. "github.com/dave/jennifer/jen"
	"github.com/ettle/strcase"
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	pkgdef "github.com/oam-dev/kubevela/pkg/definition"
)

var (
	DefinitionKindToBaseType = map[string]string{
		v1beta1.ComponentDefinitionKind:    "ComponentBase",
		v1beta1.TraitDefinitionKind:        "TraitBase",
		v1beta1.WorkflowStepDefinitionKind: "WorkflowStepBase",
		v1beta1.PolicyDefinitionKind:       "PolicyBase",
	}
	DefinitionKindToStatement = map[string]func(s *Statement){
		v1beta1.ComponentDefinitionKind: func(s *Statement) {
			s.Qual("common", "ApplicationComponent")
		},
		v1beta1.TraitDefinitionKind: func(s *Statement) {
			s.Qual("common", "ApplicationTrait")
		},
		v1beta1.WorkflowStepDefinitionKind: func(s *Statement) {
			s.Qual("v1beta1", "WorkflowStep")
		},
		v1beta1.PolicyDefinitionKind: func(s *Statement) {
			s.Qual("v1beta1", "AppPolicy")
		},
	}
)

type GoModifier struct {
	g *Generator

	defName string
	defKind string
	output  string
	verbose bool

	defDir   string
	utilsDir string
	// def name of different cases
	nameInSnakeCase  string
	nameInCamelCase  string
	nameInPascalCase string
	defStructName    string
	defFuncReceiver  string
}

func (m *GoModifier) Name() string {
	return "GoModifier"
}

func (m *GoModifier) Modify() error {
	m.init()
	m.clean()
	if err := m.moveUtils(); err != nil {
		return err
	}
	if err := m.addDefAPI(); err != nil {
		return err
	}
	if err := m.exportMethods(); err != nil {
		return err
	}
	if err := m.format(); err != nil {
		fmt.Println("format fail:", err)
	}
	return nil
}

func (m *GoModifier) init() {
	m.defName = m.g.name
	m.defKind = m.g.kind
	m.output = m.g.meta.Output
	m.verbose = m.g.meta.Verbose

	m.defDir = path.Join(m.output, pkgdef.DefinitionKindToType[m.defKind], m.defName)
	m.utilsDir = path.Join(m.output, "utils")
	m.nameInSnakeCase = strcase.ToSnake(m.defName)
	m.nameInPascalCase = strcase.ToPascal(m.defName)
	m.defStructName = strcase.ToGoPascal(m.defName + "-" + pkgdef.DefinitionKindToType[m.defKind])
	m.defFuncReceiver = m.defName[:1]
	_ = os.Mkdir(m.utilsDir, 0755)
}

func (m *GoModifier) clean() {
	_ = os.RemoveAll(path.Join(m.defDir, ".openapi-generator"))
	_ = os.RemoveAll(path.Join(m.defDir, "api"))
	_ = os.Rename(path.Join(m.defDir, "utils.go"), path.Join(m.utilsDir, "utils.go"))

	files, _ := os.ReadDir(m.defDir)
	for _, f := range files {
		dst := strings.TrimPrefix(f.Name(), "model_")
		if dst == m.nameInSnakeCase+"_spec.go" {
			dst = m.nameInSnakeCase + ".go"
		}
		_ = os.Rename(path.Join(m.defDir, f.Name()), path.Join(m.defDir, dst))
	}

}

func (m *GoModifier) moveUtils() error {
	// Adjust the generated files and code
	utilsFile := path.Join(m.utilsDir, "utils.go")

	utilsBytes, err := os.ReadFile(utilsFile)
	if err != nil {
		return err
	}
	utilsBytes = bytes.Replace(utilsBytes, []byte(fmt.Sprintf("package %s", m.defName)), []byte("package utils"), 1)
	err = os.WriteFile(utilsFile, utilsBytes, 0644)
	if err != nil {
		return err
	}
	// read all files in definition directory, replace the Nullable* Struct
	files, err := os.ReadDir(m.defDir)
	if err != nil {
		return err
	}
	for _, f := range files {
		loc := path.Join(m.defDir, f.Name())
		before, err := os.ReadFile(loc)
		if err != nil {
			return errors.Wrapf(err, "read file")
		}
		after := regexp.MustCompile("Nullable(String|(Float|Int)(32|64)|Bool)").ReplaceAllString(string(before), "utils.Nullable$1")
		_ = os.WriteFile(loc, []byte(after), 0644)
	}
	return nil
}

// addDefAPI will add component/trait/workflowstep/policy Object to the api
func (m *GoModifier) addDefAPI() error {
	file, err := os.OpenFile(path.Join(m.defDir, m.nameInSnakeCase+".go"), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	renderGroup := make([]*Statement, 0)
	renderGroup = append(renderGroup, m.genCommonFunc()...)
	renderGroup = append(renderGroup, m.genDedicatedFunc()...)
	renderGroup = append(renderGroup, m.genNameTypeFunc()...)

	buf := new(bytes.Buffer)
	for _, r := range renderGroup {
		//write content at the end of file
		err := r.Render(buf)
		buf.WriteString("\n\n")
		if err != nil {
			return errors.Wrap(err, "render code")
		}
	}
	_, err = file.Write(buf.Bytes())
	if err != nil {
		return errors.Wrap(err, "append content to file")
	}
	return nil
}

func (m *GoModifier) genCommonFunc() []*Statement {
	kind := m.defKind
	typeName := func(s *Statement) { s.Id(m.nameInPascalCase + "Type") }
	typeConst := Const().Do(typeName).Op("=").Lit(m.defName)
	Op("=").Lit(m.defName)
	defStruct := Type().Id(m.defStructName).Struct(
		Id("Base").Id("apis").Dot(DefinitionKindToBaseType[kind]),
		Id("Properties").Id(m.nameInPascalCase+"Spec"),
	)

	structPointer := func(s *Statement) { s.Op("*").Id(m.defStructName) }
	defStructConstructor := Func().Id(m.nameInPascalCase).Params(
		Do(func(s *Statement) {
			switch kind {
			case v1beta1.ComponentDefinitionKind, v1beta1.PolicyDefinitionKind, v1beta1.WorkflowStepDefinitionKind:
				s.Id("name").String()
			}
		}),
	).Do(structPointer).Block(
		Id(m.defFuncReceiver).Op(":=").Op("&").Id(m.defStructName).Values(Dict{
			Id("Base"): Id("apis").Dot(DefinitionKindToBaseType[kind]).BlockFunc(
				func(g *Group) {
					switch kind {
					case v1beta1.ComponentDefinitionKind, v1beta1.PolicyDefinitionKind, v1beta1.WorkflowStepDefinitionKind:
						g.Id("Name").Op(":").Id("name").Op(",")
					}
				}),
		}),
		Return(Id(m.defFuncReceiver)),
	)
	traitType := DefinitionKindToStatement[v1beta1.TraitDefinitionKind]
	stepType := DefinitionKindToStatement[v1beta1.WorkflowStepDefinitionKind]
	builderDict := Dict{
		// all definition have type and properties
		Id("Type"):       Do(typeName),
		Id("Properties"): Qual("util", "Object2RawExtension").Params(Id(m.defFuncReceiver).Dot("Properties")),
	}
	builderDictValues := map[string][]string{
		v1beta1.PolicyDefinitionKind:       {"Name"},
		v1beta1.ComponentDefinitionKind:    {"Name", "DependsOn", "Inputs", "Outputs"},
		v1beta1.WorkflowStepDefinitionKind: {"Name", "DependsOn", "Inputs", "Outputs", "If", "Timeout", "Meta"},
	}
	for _, v := range builderDictValues[kind] {
		builderDict[Id(v)] = Id(m.defFuncReceiver).Dot("Base").Dot(v)
	}
	switch kind {
	case v1beta1.ComponentDefinitionKind:
		builderDict[Id("Traits")] = Id("traits")
	case v1beta1.WorkflowStepDefinitionKind:
		builderDict[Id("SubSteps")] = Id("subSteps")
	}
	buildFunc := Func().
		Params(Id(m.defFuncReceiver).Do(structPointer)).
		Id("Build").Params().
		Do(DefinitionKindToStatement[kind]).BlockFunc(func(g *Group) {
		switch kind {
		case v1beta1.ComponentDefinitionKind:
			g.Add(Id("traits").Op(":=").Make(Index().Do(traitType), Lit(0)))
			g.Add(For(List(Id("_"), Id("trait")).Op(":=").Range().Id(m.defFuncReceiver).Dot("Base").Dot("Traits")).Block(
				Id("traits").Op("=").Append(Id("traits"), Id("trait").Dot("Build").Call()),
			))
		case v1beta1.WorkflowStepDefinitionKind:
			g.Add(Id("_subSteps").Op(":=").Make(Index().Do(stepType), Lit(0)))
			g.Add(For(List(Id("_"), Id("subStep")).Op(":=").Range().Id(m.defFuncReceiver).Dot("Base").Dot("SubSteps")).Block(
				Id("_subSteps").Op("=").Append(Id("_subSteps"), Id("subStep").Dot("Build").Call()),
			))
			g.Add(Id("subSteps").Op(":=").Make(Index().Qual("common", "WorkflowSubStep"), Lit(0)))
			g.Add(For(List(Id("_"), Id("_s").Op(":=").Range().Id("_subSteps"))).Block(
				Id("subSteps").Op("=").Append(Id("subSteps"), Qual("common", "WorkflowSubStep").ValuesFunc(
					func(_g *Group) {
						for _, v := range []string{"Name", "DependsOn", "Inputs", "Outputs", "If", "Timeout", "Meta", "Properties"} {
							_g.Add(Id(v).Op(":").Id("_s").Dot(v))
						}
					}),
				)),
			)
		}
		// comp only now
		g.Add(Id("res").Op(":=").Do(DefinitionKindToStatement[kind]).Values(builderDict))
		g.Add(Return(Id("res")))
	})

	return []*Statement{typeConst, defStruct, defStructConstructor, buildFunc}
}

// genDedicatedFunc generate functions for different definition kinds
func (m *GoModifier) genDedicatedFunc() []*Statement {
	structPointer := func(s *Statement) { s.Op("*").Id(m.defStructName) }

	switch m.defKind {
	case v1beta1.ComponentDefinitionKind:
		traitFunc := Func().
			Params(Id(m.defFuncReceiver).Do(structPointer)).
			Id("AddTrait").
			Params(Id("traits").Op("...").Qual("apis", "Trait")).
			Do(structPointer).
			Block(
				Id(m.defFuncReceiver).Dot("Base").Dot("Traits").Op("=").Append(Id(m.defFuncReceiver).Dot("Base").Dot("Traits"), Id("traits").Op("...")),
				Return(Id(m.defFuncReceiver)),
			)
		return []*Statement{traitFunc}

	case v1beta1.WorkflowStepDefinitionKind:
		workflowBaseFuncArgs := []struct {
			funcName string
			argName  string
			argType  *Statement
			dst      *Statement
		}{
			{funcName: "If", argName: "_if", argType: String()},
			{funcName: "Alias", argName: "alias", argType: String(), dst: Dot("Meta").Dot("Alias")},
			{funcName: "Timeout", argName: "timeout", argType: String()},
			{funcName: "DependsOn", argName: "dependsOn", argType: Index().String()},
			{funcName: "Inputs", argName: "input", argType: Qual("common", "StepInputs")},
			{funcName: "Outputs", argName: "output", argType: Qual("common", "StepOutputs")},
		}
		workflowBaseFuncs := make([]*Statement, 0)
		for _, fn := range workflowBaseFuncArgs {
			if fn.dst == nil {
				fn.dst = Dot(fn.funcName)
			}
			f := Func().
				Params(Id(m.defFuncReceiver).Do(structPointer)).
				Id(fn.funcName).
				Params(Id(fn.argName).Add(fn.argType)).
				Do(structPointer).
				Block(
					Id(m.defFuncReceiver).Dot("Base").Add(fn.dst).Op("=").Id(fn.argName),
					Return(Id(m.defFuncReceiver)),
				)
			workflowBaseFuncs = append(workflowBaseFuncs, f)
		}
		workflowBaseFuncs = append(workflowBaseFuncs, m.genAddSubStepFunc())
		return workflowBaseFuncs
	}
	return nil
}

func (m *GoModifier) genNameTypeFunc() []*Statement {
	structPointer := func(s *Statement) { s.Op("*").Id(m.defStructName) }
	nameFunc := Func().Params(Id(m.defFuncReceiver).Do(structPointer)).Id("Name").Params().String().Block(
		Return(Id(m.defFuncReceiver).Dot("Base").Dot("Name")),
	)
	typeFunc := Func().Params(Id(m.defFuncReceiver).Do(structPointer)).Id("Type").Params().String().Block(
		Return(Id(m.nameInPascalCase + "Type")),
	)
	switch m.defKind {
	case v1beta1.ComponentDefinitionKind, v1beta1.WorkflowStepDefinitionKind:
		return []*Statement{nameFunc, typeFunc}
	case v1beta1.PolicyDefinitionKind, v1beta1.TraitDefinitionKind:
		return []*Statement{typeFunc}
	}
	return nil
}

func (m *GoModifier) genAddSubStepFunc() *Statement {
	structPointer := func(s *Statement) { s.Op("*").Id(m.defStructName) }
	if m.defName != "step-group" || m.defKind != v1beta1.WorkflowStepDefinitionKind {
		return Null()
	}
	subList := Id(m.defFuncReceiver).Dot("Base").Dot("SubSteps")
	return Func().
		Params(Id(m.defFuncReceiver).Do(structPointer)).
		Id("AddSubStep").
		Params(Id("subStep").Qual("apis", "WorkflowStep")).
		Do(structPointer).
		Block(
			subList.Clone().Op("=").Append(subList.Clone(), Id("subStep")),
			Return(Id(m.defFuncReceiver)),
		)
}

// exportMethods will export methods from definition spec struct to definition struct
func (m *GoModifier) exportMethods() error {
	fileLoc := path.Join(m.defDir, m.nameInSnakeCase+".go")
	file, err := os.ReadFile(fileLoc)
	if err != nil {
		return err
	}
	var fileStr = string(file)
	from := fmt.Sprintf("*%sSpec", m.nameInPascalCase)
	to := fmt.Sprintf("*%s", m.defStructName)
	// replace all the function receiver but not below functions
	// New{m.nameInPascalCase}SpecWith
	// New{m.nameInPascalCase}Spec
	fileStr = regexp.MustCompile(fmt.Sprintf(`func \(o \%s\) ([()\[\]{}\w ]+)\%s([ )])`, from, from)).ReplaceAllString(fileStr, fmt.Sprintf("func (o %s) $1%s$2", to, to))
	fileStr = strings.ReplaceAll(fileStr, "func (o "+from, "func (o "+to)

	// replace all function receiver in function body
	// o.foo -> o.Properties.foo
	// seek the MarshalJSON function, replace functions before it
	from = "o."
	to = "o.Properties."
	parts := strings.SplitN(fileStr, "MarshalJSON", 2)
	if len(parts) != 2 {
		return fmt.Errorf("can't find MarshalJSON function")
	}
	fileStr = strings.ReplaceAll(parts[0], from, to) + "MarshalJSON" + parts[1]

	return os.WriteFile(fileLoc, []byte(fileStr), 0644)
}

func (m *GoModifier) format() error {
	// check if gofmt is installed

	formatters := []string{"gofmt", "goimports"}
	formatterPaths := []string{}
	allFormattersInstalled := true
	for _, formatter := range formatters {
		p, err := exec.LookPath(formatter)
		if err != nil {
			allFormattersInstalled = false
			break
		}
		formatterPaths = append(formatterPaths, p)
	}
	if allFormattersInstalled {
		for _, fmter := range formatterPaths {
			if m.verbose {
				fmt.Printf("Use %s to format code\n", fmter)
			}
			cmd := exec.Command(fmter, "-w", m.defDir)
			output, err := cmd.CombinedOutput()
			if err != nil {
				return errors.Wrap(err, string(output))
			}
		}
		return nil
	}
	// fallback to use go lib
	if m.verbose {
		fmt.Println("At least one of linters is not installed, use go/format lib to format code")
	}
	files, err := os.ReadDir(m.defDir)
	if err != nil {
		return errors.Wrap(err, "read dir")
	}
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".go") {
			continue
		}
		filePath := path.Join(m.defDir, f.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			return errors.Wrapf(err, "read file %s", filePath)
		}
		formatted, err := format.Source(content)
		if err != nil {
			return errors.Wrapf(err, "format file %s", filePath)
		}
		err = os.WriteFile(filePath, formatted, 0644)
		if err != nil {
			return errors.Wrapf(err, "write file %s", filePath)
		}
	}
	return nil
}
