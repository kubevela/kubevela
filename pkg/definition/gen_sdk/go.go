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
	"go/format"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	j "github.com/dave/jennifer/jen"
	"github.com/ettle/strcase"
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	pkgdef "github.com/oam-dev/kubevela/pkg/definition"
)

var (
	mainModuleVersionKey langArgKey = "MainModuleVersion"
	goProxyKey           langArgKey = "GoProxy"

	mainModuleVersion = LangArg{
		Name: mainModuleVersionKey,
		Desc: "The version of main module, it will be used in go get command. For example, tag, commit id, branch name",
		// default hash of main module. This is a commit hash of kubevela-contrib/kubvela-go-sdk. It will be used in go get command.
		Default: "cd431bb25a9a",
	}
	goProxy = LangArg{
		Name:    goProxyKey,
		Desc:    "The proxy for go get/go mod tidy command",
		Default: "https://goproxy.cn,direct",
	}
)

func init() {
	registerLangArg("go", mainModuleVersion, goProxy)
}

const (
	// PackagePlaceHolder is the package name placeholder
	PackagePlaceHolder = "github.com/kubevela/vela-go-sdk"
)

var (
	// DefinitionKindToPascal is the map of definition kind to pascal case
	DefinitionKindToPascal = map[string]string{
		v1beta1.ComponentDefinitionKind:    "Component",
		v1beta1.TraitDefinitionKind:        "Trait",
		v1beta1.WorkflowStepDefinitionKind: "WorkflowStep",
		v1beta1.PolicyDefinitionKind:       "Policy",
	}
	// DefinitionKindToBaseType is the map of definition kind to base type
	DefinitionKindToBaseType = map[string]string{
		v1beta1.ComponentDefinitionKind:    "ComponentBase",
		v1beta1.TraitDefinitionKind:        "TraitBase",
		v1beta1.WorkflowStepDefinitionKind: "WorkflowStepBase",
		v1beta1.PolicyDefinitionKind:       "PolicyBase",
	}
	// DefinitionKindToStatement is the map of definition kind to statement
	DefinitionKindToStatement = map[string]*j.Statement{
		v1beta1.ComponentDefinitionKind:    j.Qual("common", "ApplicationComponent"),
		v1beta1.TraitDefinitionKind:        j.Qual("common", "ApplicationTrait"),
		v1beta1.WorkflowStepDefinitionKind: j.Qual("v1beta1", "WorkflowStep"),
		v1beta1.PolicyDefinitionKind:       j.Qual("v1beta1", "AppPolicy"),
	}
)

// GoDefModifier is the Modifier for golang, modify code for each definition
type GoDefModifier struct {
	*GenMeta
	*goArgs

	defStructPointer *j.Statement
}

// GoModuleModifier is the Modifier for golang, modify code for each module which contains multiple definitions
type GoModuleModifier struct {
	*GenMeta
	*goArgs
}

type goArgs struct {
	apiDir   string
	defDir   string
	utilsDir string
	// def name of different cases
	nameInSnakeCase      string
	nameInPascalCase     string
	specNameInPascalCase string
	typeVarName          string
	defStructName        string
	defFuncReceiver      string
}

func (a *goArgs) init(m *GenMeta) error {
	var err error
	a.apiDir, err = filepath.Abs(path.Join(m.Output, m.APIDirectory))
	if err != nil {
		return err
	}
	a.defDir = path.Join(a.apiDir, pkgdef.DefinitionKindToType[m.kind], m.name)
	a.utilsDir = path.Join(m.Output, "pkg", "apis", "utils")
	a.nameInSnakeCase = strcase.ToSnake(m.name)
	a.nameInPascalCase = strcase.ToPascal(m.name)
	a.typeVarName = a.nameInPascalCase + "Type"
	a.specNameInPascalCase = a.nameInPascalCase + "Spec"
	a.defStructName = strcase.ToGoPascal(m.name + "-" + pkgdef.DefinitionKindToType[m.kind])
	a.defFuncReceiver = m.name[:1]
	return nil
}

// Modify implements Modifier
func (m *GoModuleModifier) Modify() error {
	for _, fn := range []func() error{
		m.init,
		m.format,
		m.addSubGoMod,
		m.tidyMainMod,
	} {
		if err := fn(); err != nil {
			return errors.Wrap(err, fnName(fn))
		}
	}
	return nil
}

func (m *GoModuleModifier) init() error {
	m.goArgs = &goArgs{}
	return m.goArgs.init(m.GenMeta)
}

// Name the name of modifier
func (m *GoModuleModifier) Name() string {
	return "goModuleModifier"
}

// Name the name of modifier
func (m *GoDefModifier) Name() string {
	return "GoDefModifier"
}

// Modify the modification of generated code
func (m *GoDefModifier) Modify() error {
	for _, fn := range []func() error{
		m.init,
		m.clean,
		m.moveUtils,
		m.modifyDefs,
		m.addDefAPI,
		m.addValidateTraits,
		m.exportMethods,
	} {
		if err := fn(); err != nil {
			return errors.Wrap(err, fnName(fn))
		}
	}
	return nil
}

func (m *GoDefModifier) init() error {
	m.goArgs = &goArgs{}
	err := m.goArgs.init(m.GenMeta)
	if err != nil {
		return err
	}

	m.defStructPointer = j.Op("*").Id(m.defStructName)

	err = os.MkdirAll(m.utilsDir, 0750)
	return err
}

func (m *GoDefModifier) clean() error {
	err := os.RemoveAll(path.Join(m.defDir, ".openapi-generator"))
	if err != nil {
		return err
	}
	err = os.RemoveAll(path.Join(m.defDir, "api"))
	if err != nil {
		return err
	}

	files, _ := os.ReadDir(m.defDir)
	for _, f := range files {
		dst := strings.TrimPrefix(f.Name(), "model_")
		if dst == m.nameInSnakeCase+"_spec.go" {
			dst = m.nameInSnakeCase + ".go"
		}
		err = os.Rename(path.Join(m.defDir, f.Name()), path.Join(m.defDir, dst))
		if err != nil {
			return err
		}
	}
	return nil

}

// addSubGoMod will add a go.mod and go.sum in the api directory if user mark that the api is a submodule
func (m *GoModuleModifier) addSubGoMod() error {
	if !m.IsSubModule {
		return nil
	}
	files := map[string]string{
		"go.mod_": "go.mod",
		"go.sum":  "go.sum",
	}
	for src, dst := range files {
		srcContent, err := Scaffold.ReadFile(path.Join(ScaffoldDir, "go", src))
		if err != nil {
			return errors.Wrap(err, "read "+src)
		}
		subModuleName := strings.TrimSuffix(fmt.Sprintf("%s/%s", m.Package, m.APIDirectory), "/")
		srcContent = bytes.ReplaceAll(srcContent, []byte("module "+PackagePlaceHolder), []byte("module "+subModuleName))
		srcContent = bytes.ReplaceAll(srcContent, []byte("// require "+PackagePlaceHolder), []byte("require "+m.Package))

		err = os.WriteFile(path.Join(m.apiDir, dst), srcContent, 0600)
		if err != nil {
			return errors.Wrap(err, "write "+dst)
		}
	}

	cmds := make([]*exec.Cmd, 0)
	if m.LangArgs.Get(mainModuleVersionKey) != mainModuleVersion.Default {
		// nolint:gosec
		cmds = append(cmds, exec.Command("docker", "run",
			"--rm",
			"-v", m.apiDir+":/api",
			"-w", "/api",
			"golang:1.19-alpine",
			"go", "get", fmt.Sprintf("%s@%s", m.Package, m.LangArgs.Get(mainModuleVersionKey)),
		))
	}
	// nolint:gosec
	cmds = append(cmds, exec.Command("docker", "run",
		"--rm",
		"-v", m.apiDir+":/api",
		"-w", "/api",
		"--env", "GOPROXY="+m.LangArgs.Get(goProxyKey),
		"golang:1.19-alpine",
		"go", "mod", "tidy",
	))
	for _, cmd := range cmds {
		if m.Verbose {
			fmt.Println(cmd.String())
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}

		err := cmd.Run()
		if err != nil {
			return errors.Wrapf(err, "fail to run command %s", cmd.String())
		}
	}
	return nil
}

// tidyMainMod will run go mod tidy in the main module
func (m *GoModuleModifier) tidyMainMod() error {
	if !m.InitSDK {
		return nil
	}
	outDir, err := filepath.Abs(m.GenMeta.Output)
	if err != nil {
		return err
	}
	// nolint:gosec
	cmd := exec.Command("docker", "run",
		"--rm",
		"-v", outDir+":/api",
		"-w", "/api",
		"golang:1.19-alpine",
		"go", "mod", "tidy",
	)
	if m.Verbose {
		fmt.Println(cmd.String())
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

// read all files in definition directory,
// 1. replace the Nullable* Struct
// 2. replace the package name
func (m *GoDefModifier) modifyDefs() error {
	changeNullableType := func(b []byte) []byte {
		return regexp.MustCompile("Nullable(String|(Float|Int)(32|64)|Bool)").ReplaceAll(b, []byte("utils.Nullable$1"))
	}

	files, err := os.ReadDir(m.defDir)
	defHandleFunc := []byteHandler{
		m.packageFunc,
		changeNullableType,
	}
	if err != nil {
		return err
	}
	for _, f := range files {
		loc := path.Join(m.defDir, f.Name())
		// nolint:gosec
		b, err := os.ReadFile(loc)
		if err != nil {
			return errors.Wrapf(err, "read file")
		}
		for _, h := range defHandleFunc {
			b = h(b)
		}

		_ = os.WriteFile(loc, b, 0600)
	}
	return nil
}

func (m *GoDefModifier) moveUtils() error {
	// Adjust the generated files and code
	err := os.Rename(path.Join(m.defDir, "utils.go"), path.Join(m.utilsDir, "utils.go"))
	if err != nil {
		return err
	}
	utilsFile := path.Join(m.utilsDir, "utils.go")

	// nolint:gosec
	utilsBytes, err := os.ReadFile(utilsFile)
	if err != nil {
		return err
	}
	utilsBytes = bytes.Replace(utilsBytes, []byte(fmt.Sprintf("package %s", strcase.ToSnake(m.name))), []byte("package utils"), 1)
	utilsBytes = bytes.ReplaceAll(utilsBytes, []byte("isNil"), []byte("IsNil"))
	err = os.WriteFile(utilsFile, utilsBytes, 0600)
	if err != nil {
		return err
	}
	return nil
}

// addDefAPI will add component/trait/workflowstep/policy Object to the api
func (m *GoDefModifier) addDefAPI() error {
	file, err := os.OpenFile(path.Join(m.defDir, m.nameInSnakeCase+".go"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()
	renderGroup := make([]*j.Statement, 0)
	renderGroup = append(renderGroup, m.genCommonFunc()...)
	renderGroup = append(renderGroup, m.genFromFunc()...)
	renderGroup = append(renderGroup, m.genDedicatedFunc()...)
	renderGroup = append(renderGroup, m.genNameTypeFunc()...)
	renderGroup = append(renderGroup, m.genUnmarshalFunc()...)
	renderGroup = append(renderGroup, m.genBaseSetterFunc()...)
	renderGroup = append(renderGroup, m.genAddSubStepFunc())

	buf := new(bytes.Buffer)
	for _, r := range renderGroup {
		// write content at the end of file
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

func (m *GoDefModifier) genCommonFunc() []*j.Statement {
	kind := m.kind
	typeName := j.Id(m.nameInPascalCase + "Type")
	typeConst := j.Const().Add(typeName).Op("=").Lit(m.name)
	j.Op("=").Lit(m.name)
	defStruct := j.Type().Id(m.defStructName).Struct(
		j.Id("Base").Id("apis").Dot(DefinitionKindToBaseType[kind]),
		j.Id("Properties").Id(m.specNameInPascalCase),
	)

	initFunc := j.Func().Id("init").Params().BlockFunc(func(g *j.Group) {
		g.Add(j.Qual("sdkcommon", "Register"+DefinitionKindToPascal[kind]).Call(j.Add(typeName), j.Id("From"+DefinitionKindToPascal[kind])))
		if kind == v1beta1.WorkflowStepDefinitionKind {
			g.Add(j.Qual("sdkcommon", "RegisterWorkflowSubStep").Call(j.Add(typeName), j.Id("FromWorkflowSubStep")))
		}
	},
	)

	defStructConstructor := j.Func().Id(m.nameInPascalCase).Params(
		j.Do(func(s *j.Statement) {
			switch kind {
			case v1beta1.ComponentDefinitionKind, v1beta1.PolicyDefinitionKind, v1beta1.WorkflowStepDefinitionKind:
				s.Id("name").String()
			}
		}),
	).Add(m.defStructPointer).Block(
		j.Id(m.defFuncReceiver).Op(":=").Op("&").Id(m.defStructName).Values(j.Dict{
			j.Id("Base"): j.Id("apis").Dot(DefinitionKindToBaseType[kind]).BlockFunc(
				func(g *j.Group) {
					switch kind {
					case v1beta1.ComponentDefinitionKind, v1beta1.PolicyDefinitionKind, v1beta1.WorkflowStepDefinitionKind:
						g.Id("Name").Op(":").Id("name").Op(",")
						g.Id("Type").Op(":").Add(typeName).Op(",")
					}
				}),
		}),
		j.Return(j.Id(m.defFuncReceiver)),
	)
	traitType := DefinitionKindToStatement[v1beta1.TraitDefinitionKind]
	stepType := DefinitionKindToStatement[v1beta1.WorkflowStepDefinitionKind]
	builderDict := j.Dict{
		// all definition have type and properties
		j.Id("Type"):       j.Add(typeName),
		j.Id("Properties"): j.Qual("util", "Object2RawExtension").Params(j.Id(m.defFuncReceiver).Dot("Properties")),
	}
	builderDictValues := map[string][]string{
		v1beta1.PolicyDefinitionKind:       {"Name"},
		v1beta1.ComponentDefinitionKind:    {"Name", "DependsOn", "Inputs", "Outputs"},
		v1beta1.WorkflowStepDefinitionKind: {"Name", "DependsOn", "Inputs", "Outputs", "If", "Timeout", "Meta"},
	}
	for _, v := range builderDictValues[kind] {
		builderDict[j.Id(v)] = j.Id(m.defFuncReceiver).Dot("Base").Dot(v)
	}
	switch kind {
	case v1beta1.ComponentDefinitionKind:
		builderDict[j.Id("Traits")] = j.Id("traits")
	case v1beta1.WorkflowStepDefinitionKind:
		builderDict[j.Id("SubSteps")] = j.Id("subSteps")
	}
	buildFunc := j.Func().
		Params(j.Id(m.defFuncReceiver).Add(m.defStructPointer)).
		Id("Build").Params().
		Add(DefinitionKindToStatement[kind]).BlockFunc(func(g *j.Group) {
		switch kind {
		case v1beta1.ComponentDefinitionKind:
			g.Add(j.Id("traits").Op(":=").Make(j.Index().Add(traitType), j.Lit(0)))
			g.Add(j.For(j.List(j.Id("_"), j.Id("trait")).Op(":=").Range().Id(m.defFuncReceiver).Dot("Base").Dot("Traits")).Block(
				j.Id("traits").Op("=").Append(j.Id("traits"), j.Id("trait").Dot("Build").Call()),
			))
		case v1beta1.WorkflowStepDefinitionKind:
			g.Add(j.Id("_subSteps").Op(":=").Make(j.Index().Add(stepType), j.Lit(0)))
			g.Add(j.For(j.List(j.Id("_"), j.Id("subStep")).Op(":=").Range().Id(m.defFuncReceiver).Dot("Base").Dot("SubSteps")).Block(
				j.Id("_subSteps").Op("=").Append(j.Id("_subSteps"), j.Id("subStep").Dot("Build").Call()),
			))
			g.Add(j.Id("subSteps").Op(":=").Make(j.Index().Qual("common", "WorkflowSubStep"), j.Lit(0)))
			g.Add(j.For(j.List(j.Id("_"), j.Id("_s").Op(":=").Range().Id("_subSteps"))).Block(
				j.Id("subSteps").Op("=").Append(j.Id("subSteps"), j.Qual("common", "WorkflowSubStep").ValuesFunc(
					func(_g *j.Group) {
						for _, v := range []string{"Name", "DependsOn", "Inputs", "Outputs", "If", "Timeout", "Meta", "Properties", "Type"} {
							_g.Add(j.Id(v).Op(":").Id("_s").Dot(v))
						}
					}),
				)),
			)
		}
		g.Add(j.Id("res").Op(":=").Add(DefinitionKindToStatement[kind]).Values(builderDict))
		g.Add(j.Return(j.Id("res")))
	})

	return []*j.Statement{typeConst, initFunc, defStruct, defStructConstructor, buildFunc}
}

func (m *GoDefModifier) genFromFunc() []*j.Statement {
	kind := m.kind
	kindBaseProperties := map[string][]string{
		v1beta1.ComponentDefinitionKind:    {"Name", "DependsOn", "Inputs", "Outputs"},
		v1beta1.WorkflowStepDefinitionKind: {"Name", "DependsOn", "Inputs", "Outputs", "If", "Timeout", "Meta"},
		v1beta1.PolicyDefinitionKind:       {"Name"},
		v1beta1.TraitDefinitionKind:        {},
	}

	// fromFuncRsv means build from a part of K8s Object (e.g. v1beta1.Application.spec.component[*] to internal presentation (e.g. Component)
	// fromFuncRsv will have a function receiver
	getSubSteps := func(sub bool) func(g *j.Group) {
		if m.kind != v1beta1.WorkflowStepDefinitionKind || sub {
			return func(g *j.Group) {}
		}
		return func(g *j.Group) {
			g.Add(j.Id("subSteps").Op(":=").Make(j.Index().Qual("apis", DefinitionKindToPascal[kind]), j.Lit(0)))
			g.Add(
				j.For(
					j.List(j.Id("_"), j.Id("_s")).Op(":=").Range().Id("from").Dot("SubSteps")).Block(
					j.List(j.Id("subStep"), j.Err()).Op(":=").Id(m.defFuncReceiver).Dot("FromWorkflowSubStep").Call(j.Id("_s")),
					j.If(j.Err().Op("!=").Nil()).Block(
						j.Return(j.Nil(), j.Err()),
					),
					j.Id("subSteps").Op("=").Append(j.Id("subSteps"), j.Id("subStep")),
				),
			)
		}
	}
	assignSubSteps := func(sub bool) func(g *j.Group) {
		if m.kind != v1beta1.WorkflowStepDefinitionKind || sub {
			return func(g *j.Group) {}
		}
		return func(g *j.Group) {
			g.Add(j.Id(m.defFuncReceiver).Dot("Base").Dot("SubSteps").Op("=").Id("subSteps"))
		}
	}
	fromFuncRsv := func(sub bool) *j.Statement {
		funcName := "From" + DefinitionKindToPascal[kind]
		params := DefinitionKindToStatement[kind]
		if sub {
			funcName = "FromWorkflowSubStep"
			params = j.Qual("common", "WorkflowSubStep")
		}
		return j.Func().
			Params(j.Id(m.defFuncReceiver).Add(m.defStructPointer)).
			Id(funcName).
			Params(j.Id("from").Add(params)).Params(j.Add(m.defStructPointer), j.Error()).
			BlockFunc(func(g *j.Group) {
				if kind == v1beta1.ComponentDefinitionKind {
					g.Add(j.For(j.List(j.Id("_"), j.Id("trait")).Op(":=").Range().Id("from").Dot("Traits")).Block(
						j.List(j.Id("_t"), j.Err()).Op(":=").Qual("sdkcommon", "FromTrait").Call(j.Id("trait")),
						j.If(j.Err().Op("!=").Nil()).Block(
							j.Return(j.Nil(), j.Err()),
						),
						j.Id(m.defFuncReceiver).Dot("Base").Dot("Traits").Op("=").Append(j.Id(m.defFuncReceiver).Dot("Base").Dot("Traits"), j.Id("_t")),
					))
				}
				g.Add(j.Var().Id("properties").Id(m.specNameInPascalCase))
				g.Add(
					j.If(j.Id("from").Dot("Properties").Op("!=").Nil()).Block(
						j.Err().Op(":=").Qual("json", "Unmarshal").Call(j.Id("from").Dot("Properties").Dot("Raw"), j.Op("&").Id("properties")),
						j.If(j.Err().Op("!=").Nil()).Block(
							j.Return(j.Nil(), j.Err()),
						),
					),
				)
				getSubSteps(sub)(g)

				for _, prop := range kindBaseProperties[kind] {
					g.Add(j.Id(m.defFuncReceiver).Dot("Base").Dot(prop).Op("=").Id("from").Dot(prop))
				}
				g.Add(j.Id(m.defFuncReceiver).Dot("Base").Dot("Type").Op("=").Id(m.typeVarName))
				g.Add(j.Id(m.defFuncReceiver).Dot("Properties").Op("=").Id("properties"))

				assignSubSteps(sub)(g)
				g.Add(j.Return(j.Id(m.defFuncReceiver), j.Nil()))
			},
			)
	}

	// fromFunc is like fromFuncRsv but not having function receiver, returning an internal presentation
	fromFunc := j.Func().
		Id("From"+DefinitionKindToPascal[kind]).
		Params(j.Id("from").Add(DefinitionKindToStatement[kind])).Params(j.Qual("apis", DefinitionKindToPascal[kind]), j.Error()).
		Block(
			j.Id(m.defFuncReceiver).Op(":=").Op("&").Id(m.defStructName).Values(j.Dict{}),
			j.Return(j.Id(m.defFuncReceiver).Dot("From"+DefinitionKindToPascal[kind]).Call(j.Id("from"))),
		)
	fromSubFunc := j.Func().Id("FromWorkflowSubStep").
		Params(j.Id("from").Qual("common", "WorkflowSubStep")).Params(j.Qual("apis", DefinitionKindToPascal[kind]), j.Error()).
		Block(
			j.Id(m.defFuncReceiver).Op(":=").Op("&").Id(m.defStructName).Values(j.Dict{}),
			j.Return(j.Id(m.defFuncReceiver).Dot("FromWorkflowSubStep").Call(j.Id("from"))),
		)

	res := []*j.Statement{fromFuncRsv(false), fromFunc}
	if m.kind == v1beta1.WorkflowStepDefinitionKind {
		res = append(res, fromFuncRsv(true), fromSubFunc)
	}
	return res
}

// genDedicatedFunc generate functions for definition kinds
func (m *GoDefModifier) genDedicatedFunc() []*j.Statement {
	switch m.kind {
	case v1beta1.ComponentDefinitionKind:
		setTraitFunc := j.Func().
			Params(j.Id(m.defFuncReceiver).Add(m.defStructPointer)).
			Id("SetTraits").
			Params(j.Id("traits").Op("...").Qual("apis", "Trait")).
			Add(m.defStructPointer).
			Block(
				j.For(j.List(j.Id("_"), j.Id("addTrait")).Op(":=").Range().Id("traits")).Block(
					j.Id("found").Op(":=").False(),
					j.For(j.List(j.Id("i"), j.Id("_t")).Op(":=").Range().Id(m.defFuncReceiver).Dot("Base").Dot("Traits")).Block(
						j.If(j.Id("_t").Dot("DefType").Call().Op("==").Id("addTrait").Dot("DefType").Call()).Block(
							j.Id(m.defFuncReceiver).Dot("Base").Dot("Traits").Index(j.Id("i")).Op("=").Id("addTrait"),
							j.Id("found").Op("=").True(),
							j.Break(),
						),
						j.If(j.Op("!").Id("found")).Block(
							j.Id(m.defFuncReceiver).Dot("Base").Dot("Traits").Op("=").Append(j.Id(m.defFuncReceiver).Dot("Base").Dot("Traits"), j.Id("addTrait")),
						),
					),
				),
				j.Return(j.Id(m.defFuncReceiver)),
			)
		getTraitFunc := j.Func().
			Params(j.Id(m.defFuncReceiver).Add(m.defStructPointer)).
			Id("GetTrait").
			Params(j.Id("typ").String()).
			Params(j.Qual("apis", "Trait")).
			Block(
				j.For(j.List(j.Id("_"), j.Id("_t")).Op(":=").Range().Id(m.defFuncReceiver).Dot("Base").Dot("Traits")).Block(
					j.If(j.Id("_t").Dot("DefType").Call().Op("==").Id("typ")).Block(
						j.Return(j.Id("_t")),
					),
				),
				j.Return(j.Nil()),
			)
		getAllTraitFunc := j.Func().
			Params(j.Id(m.defFuncReceiver).Add(m.defStructPointer)).
			Id("GetAllTraits").
			Params().
			Params(j.Index().Qual("apis", "Trait")).
			Block(
				j.Return(j.Id(m.defFuncReceiver).Dot("Base").Dot("Traits")),
			)

		return []*j.Statement{setTraitFunc, getTraitFunc, getAllTraitFunc}
	case v1beta1.WorkflowStepDefinitionKind:
	}
	return nil
}

func (m *GoDefModifier) genNameTypeFunc() []*j.Statement {
	nameFunc := j.Func().Params(j.Id(m.defFuncReceiver).Add(m.defStructPointer)).Id(DefinitionKindToPascal[m.kind] + "Name").Params().String().Block(
		j.Return(j.Id(m.defFuncReceiver).Dot("Base").Dot("Name")),
	)
	typeFunc := j.Func().Params(j.Id(m.defFuncReceiver).Add(m.defStructPointer)).Id("DefType").Params().String().Block(
		j.Return(j.Id(m.typeVarName)),
	)
	switch m.kind {
	case v1beta1.ComponentDefinitionKind, v1beta1.WorkflowStepDefinitionKind, v1beta1.PolicyDefinitionKind:
		return []*j.Statement{nameFunc, typeFunc}
	case v1beta1.TraitDefinitionKind:
		return []*j.Statement{typeFunc}
	}
	return nil
}

func (m *GoDefModifier) genUnmarshalFunc() []*j.Statement {
	return []*j.Statement{j.Null()}
}

func (m *GoDefModifier) genBaseSetterFunc() []*j.Statement {
	baseFuncArgs := map[string][]struct {
		funcName string
		argName  string
		argType  *j.Statement
		dst      *j.Statement
		isAppend bool
	}{
		v1beta1.ComponentDefinitionKind: {
			{funcName: "DependsOn", argName: "dependsOn", argType: j.Index().String()},
			{funcName: "Inputs", argName: "input", argType: j.Qual("common", "StepInputs")},
			{funcName: "Outputs", argName: "output", argType: j.Qual("common", "StepOutputs")},
			{funcName: "AddDependsOn", argName: "dependsOn", argType: j.String(), isAppend: true, dst: j.Dot("DependsOn")},
			// TODO: uncomment this after https://github.com/kubevela/workflow/pull/125 is released.
			// {funcName: "AddInput", argName: "input", argType: Qual("common", "StepInputs"), isAppend: true, dst: "Inputs"},
			// {funcName: "AddOutput", argName: "output", argType: Qual("common", "StepOutputs"), isAppend: true, dst: "Outputs"},
		},
		v1beta1.WorkflowStepDefinitionKind: {
			{funcName: "If", argName: "_if", argType: j.String()},
			{funcName: "Alias", argName: "alias", argType: j.String(), dst: j.Dot("Meta").Dot("Alias")},
			{funcName: "Timeout", argName: "timeout", argType: j.String()},
			{funcName: "DependsOn", argName: "dependsOn", argType: j.Index().String()},
			{funcName: "Inputs", argName: "input", argType: j.Qual("common", "StepInputs")},
			{funcName: "Outputs", argName: "output", argType: j.Qual("common", "StepOutputs")},
			// {funcName: "AddInput", argName: "input", argType: Qual("common", "StepInputs"), isAppend: true, dst: "Inputs"},
			// {funcName: "AddOutput", argName: "output", argType: Qual("common", "StepOutputs"), isAppend: true, dst: "Outputs"},
		},
	}
	baseFuncs := make([]*j.Statement, 0)
	for _, fn := range baseFuncArgs[m.kind] {
		if fn.dst == nil {
			fn.dst = j.Dot(fn.funcName)
		}
		f := j.Func().
			Params(j.Id(m.defFuncReceiver).Add(m.defStructPointer)).
			Id(fn.funcName).
			Params(j.Id(fn.argName).Add(fn.argType)).
			Add(m.defStructPointer).
			BlockFunc(func(g *j.Group) {
				field := j.Id(m.defFuncReceiver).Dot("Base").Add(fn.dst)
				if fn.isAppend {
					g.Add(field.Clone().Op("=").Append(field.Clone(), j.Id(fn.argName)))
				} else {
					g.Add(field.Clone().Op("=").Id(fn.argName))
				}
				g.Add(j.Return(j.Id(m.defFuncReceiver)))
			})
		baseFuncs = append(baseFuncs, f)
	}
	return baseFuncs
}

func (m *GoDefModifier) genAddSubStepFunc() *j.Statement {
	if m.name != "step-group" || m.kind != v1beta1.WorkflowStepDefinitionKind {
		return j.Null()
	}
	subList := j.Id(m.defFuncReceiver).Dot("Base").Dot("SubSteps")
	return j.Func().
		Params(j.Id(m.defFuncReceiver).Add(m.defStructPointer)).
		Id("AddSubStep").
		Params(j.Id("subStep").Qual("apis", "WorkflowStep")).
		Add(m.defStructPointer).
		Block(
			subList.Clone().Op("=").Append(subList.Clone(), j.Id("subStep")),
			j.Return(j.Id(m.defFuncReceiver)),
		)
}

// exportMethods will export methods from definition spec struct to definition struct
func (m *GoDefModifier) exportMethods() error {
	fileLoc := path.Join(m.defDir, m.nameInSnakeCase+".go")
	// nolint:gosec
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
	// o.Base keeps the same
	// seek the MarshalJSON function, replace functions before it
	parts := strings.SplitN(fileStr, "MarshalJSON", 2)
	if len(parts) != 2 {
		return fmt.Errorf("can't find MarshalJSON function")
	}
	parts[0] = strings.ReplaceAll(parts[0], "o.", "o.Properties.")
	parts[0] = strings.ReplaceAll(parts[0], "o.Properties.Base", "o.Base")
	fileStr = parts[0] + "MarshalJSON" + parts[1]

	return os.WriteFile(fileLoc, []byte(fileStr), 0600)
}

func (m *GoDefModifier) addValidateTraits() error {
	if m.kind != v1beta1.ComponentDefinitionKind {
		return nil
	}
	fileLoc := path.Join(m.defDir, m.nameInSnakeCase+".go")
	// nolint:gosec
	file, err := os.ReadFile(fileLoc)
	if err != nil {
		return err
	}
	var fileStr = string(file)
	buf := bytes.Buffer{}

	err = j.For(j.List(j.Id("i"), j.Id("v").Op(":=").Range().Id("o").Dot("Base").Dot("Traits"))).Block(
		j.If(j.Id("err").Op(":=").Id("v").Dot("Validate").Call().Op(";").Id("err").Op("!=").Nil()).Block(
			j.Return(j.Qual("fmt", "Errorf").Call(j.Lit("traits[%d] %s in %s component is invalid: %w"), j.Id("i"), j.Id("v").Dot("DefType").Call(), j.Id(m.typeVarName), j.Id("err"))),
		),
	).Render(&buf)
	if err != nil {
		return err
	}
	// add validate trait part in Validate function
	exp := regexp.MustCompile(`Validate\(\)((.|\n)*?)(return nil)`)
	s := buf.String()
	fileStr = exp.ReplaceAllString(fileStr, fmt.Sprintf("Validate()$1\n%s\n$3", s))

	return os.WriteFile(fileLoc, []byte(fileStr), 0600)
}
func (m *GoModuleModifier) format() error {
	// check if gofmt is installed
	// todo (chivalryq): support go mod tidy for sub-module

	formatters := []string{"gofmt", "goimports"}
	var formatterPaths []string
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
			if m.Verbose {
				fmt.Printf("Use %s to format code\n", fmter)
			}
			// nolint:gosec
			cmd := exec.Command(fmter, "-w", m.apiDir)
			output, err := cmd.CombinedOutput()
			if err != nil {
				return errors.Wrap(err, string(output))
			}
		}
		return nil
	}
	// fallback to use go lib
	if m.Verbose {
		fmt.Println("At least one of linters is not installed, use go/format lib to format code")
	}

	// format all .go files
	return filepath.Walk(m.apiDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// nolint:gosec
		content, err := os.ReadFile(path)
		if err != nil {
			return errors.Wrapf(err, "read file %s", path)
		}
		formatted, err := format.Source(content)
		if err != nil {
			return errors.Wrapf(err, "format file %s", path)
		}
		err = os.WriteFile(path, formatted, 0600)
		if err != nil {
			return errors.Wrapf(err, "write file %s", path)
		}
		return nil
	})
}
