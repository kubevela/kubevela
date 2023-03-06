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
	"regexp"
	"strings"

	// we need dot import here to make the complex go code generating simpler
	// nolint:revive
	j "github.com/dave/jennifer/jen"
	"github.com/ettle/strcase"
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	pkgdef "github.com/oam-dev/kubevela/pkg/definition"
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

// GoModifier is the Modifier for golang
type GoModifier struct {
	g *Generator

	defName string
	defKind string
	verbose bool

	defDir   string
	utilsDir string
	// def name of different cases
	nameInSnakeCase      string
	nameInPascalCase     string
	specNameInPascalCase string
	typeVarName          string
	defStructName        string
	defFuncReceiver      string
	defStructPointer     *j.Statement
}

// Name the name of modifier
func (m *GoModifier) Name() string {
	return "GoModifier"
}

// Modify the modification of generated code
func (m *GoModifier) Modify() error {
	for _, fn := range []func() error{
		m.init,
		m.clean,
		m.moveUtils,
		m.modifyDefs,
		m.addDefAPI,
		m.addValidateTraits,
		m.exportMethods,
		m.format,
	} {
		if err := fn(); err != nil {
			return err
		}
	}
	return nil
}

func (m *GoModifier) init() error {
	m.defName = m.g.name
	m.defKind = m.g.kind
	m.verbose = m.g.meta.Verbose

	pkgAPIDir := path.Join(m.g.meta.Output, "pkg", "apis")
	m.defDir = path.Join(pkgAPIDir, pkgdef.DefinitionKindToType[m.defKind], m.defName)
	m.utilsDir = path.Join(pkgAPIDir, "utils")
	m.nameInSnakeCase = strcase.ToSnake(m.defName)
	m.nameInPascalCase = strcase.ToPascal(m.defName)
	m.typeVarName = m.nameInPascalCase + "Type"
	m.specNameInPascalCase = m.nameInPascalCase + "Spec"
	m.defStructName = strcase.ToGoPascal(m.defName + "-" + pkgdef.DefinitionKindToType[m.defKind])
	m.defStructPointer = j.Op("*").Id(m.defStructName)
	m.defFuncReceiver = m.defName[:1]
	err := os.MkdirAll(m.utilsDir, 0750)
	return err
}

func (m *GoModifier) clean() error {
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

// read all files in definition directory,
// 1. replace the Nullable* Struct
// 2. replace the package name
func (m *GoModifier) modifyDefs() error {
	changeNullableType := func(b []byte) []byte {
		return regexp.MustCompile("Nullable(String|(Float|Int)(32|64)|Bool)").ReplaceAll(b, []byte("utils.Nullable$1"))
	}

	files, err := os.ReadDir(m.defDir)
	defHandleFunc := []byteHandler{
		m.g.meta.packageFunc,
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

func (m *GoModifier) moveUtils() error {
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
	utilsBytes = bytes.Replace(utilsBytes, []byte(fmt.Sprintf("package %s", strcase.ToSnake(m.defName))), []byte("package utils"), 1)
	utilsBytes = bytes.ReplaceAll(utilsBytes, []byte("isNil"), []byte("IsNil"))
	err = os.WriteFile(utilsFile, utilsBytes, 0600)
	if err != nil {
		return err
	}
	return nil
}

// addDefAPI will add component/trait/workflowstep/policy Object to the api
func (m *GoModifier) addDefAPI() error {
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

func (m *GoModifier) genCommonFunc() []*j.Statement {
	kind := m.defKind
	typeName := j.Id(m.nameInPascalCase + "Type")
	typeConst := j.Const().Add(typeName).Op("=").Lit(m.defName)
	j.Op("=").Lit(m.defName)
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

func (m *GoModifier) genFromFunc() []*j.Statement {
	kind := m.g.kind
	kindBaseProperties := map[string][]string{
		v1beta1.ComponentDefinitionKind:    {"Name", "DependsOn", "Inputs", "Outputs"},
		v1beta1.WorkflowStepDefinitionKind: {"Name", "DependsOn", "Inputs", "Outputs", "If", "Timeout", "Meta"},
		v1beta1.PolicyDefinitionKind:       {"Name"},
		v1beta1.TraitDefinitionKind:        {},
	}

	// fromFuncRsv means build from a part of K8s Object (e.g. v1beta1.Application.spec.component[*] to internal presentation (e.g. Component)
	// fromFuncRsv will have a function receiver
	getSubSteps := func(sub bool) func(g *j.Group) {
		if m.defKind != v1beta1.WorkflowStepDefinitionKind || sub {
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
		if m.defKind != v1beta1.WorkflowStepDefinitionKind || sub {
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
						j.List(j.Id("_t"), j.Err()).Op(":=").Qual("sdkcommon", "FromTrait").Call(j.Op("&").Id("trait")),
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
	if m.defKind == v1beta1.WorkflowStepDefinitionKind {
		res = append(res, fromFuncRsv(true), fromSubFunc)
	}
	return res
}

// genDedicatedFunc generate functions for definition kinds
func (m *GoModifier) genDedicatedFunc() []*j.Statement {
	switch m.defKind {
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

func (m *GoModifier) genNameTypeFunc() []*j.Statement {
	nameFunc := j.Func().Params(j.Id(m.defFuncReceiver).Add(m.defStructPointer)).Id(DefinitionKindToPascal[m.defKind] + "Name").Params().String().Block(
		j.Return(j.Id(m.defFuncReceiver).Dot("Base").Dot("Name")),
	)
	typeFunc := j.Func().Params(j.Id(m.defFuncReceiver).Add(m.defStructPointer)).Id("DefType").Params().String().Block(
		j.Return(j.Id(m.typeVarName)),
	)
	switch m.defKind {
	case v1beta1.ComponentDefinitionKind, v1beta1.WorkflowStepDefinitionKind, v1beta1.PolicyDefinitionKind:
		return []*j.Statement{nameFunc, typeFunc}
	case v1beta1.TraitDefinitionKind:
		return []*j.Statement{typeFunc}
	}
	return nil
}

func (m *GoModifier) genUnmarshalFunc() []*j.Statement {
	return []*j.Statement{j.Null()}
}

func (m *GoModifier) genBaseSetterFunc() []*j.Statement {
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
	for _, fn := range baseFuncArgs[m.defKind] {
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

func (m *GoModifier) genAddSubStepFunc() *j.Statement {
	if m.defName != "step-group" || m.defKind != v1beta1.WorkflowStepDefinitionKind {
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
func (m *GoModifier) exportMethods() error {
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

func (m *GoModifier) addValidateTraits() error {
	if m.defKind != v1beta1.ComponentDefinitionKind {
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
			// nolint:gosec
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
		// nolint:gosec
		content, err := os.ReadFile(filePath)
		if err != nil {
			return errors.Wrapf(err, "read file %s", filePath)
		}
		formatted, err := format.Source(content)
		if err != nil {
			return errors.Wrapf(err, "format file %s", filePath)
		}
		err = os.WriteFile(filePath, formatted, 0600)
		if err != nil {
			return errors.Wrapf(err, "write file %s", filePath)
		}
	}
	return nil
}
