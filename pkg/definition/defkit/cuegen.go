/*
Copyright 2025 The KubeVela Authors.

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

package defkit

import (
	"fmt"
	"sort"
	"strings"
)

// sortedKeys returns the keys of a map[string]V in sorted order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// cueOpenStruct is the CUE literal for an open struct type.
const cueOpenStruct = "{...}"

// CUE field marker constants express the three field presence semantics.
const (
	fieldMarkerOptional = "?" // field?: type  — field may be absent from input
	fieldMarkerNone     = ""  // field: type   — field must have a value; defaults/merging can satisfy
	fieldMarkerRequired = "!" // field!: type  — field must be explicitly provided in user input
)

// cueLabel quotes a CUE field label if it contains characters that are not
// valid in a CUE identifier (letters, digits, underscore, $).
func cueLabel(name string) string {
	if strings.ContainsAny(name, "-./") {
		return fmt.Sprintf("%q", name)
	}
	return name
}

// CUEGenerator generates CUE definitions from Go component definitions.
type CUEGenerator struct {
	indent  string
	imports []string
}

// CUEImports defines standard imports that may be needed in CUE definitions.
var CUEImports = struct {
	Strconv  string
	Strings  string
	List     string
	Math     string
	Regexp   string
	Time     string
	Struct   string
	Encoding string
}{
	Strconv:  "strconv",
	Strings:  "strings",
	List:     "list",
	Math:     "math",
	Regexp:   "regexp",
	Time:     "time",
	Struct:   "struct",
	Encoding: "encoding/json",
}

// cueBoolTrue is the CUE literal for a true boolean value.
const cueBoolTrue = "true"

// ImportRequirer is implemented by types that require CUE imports.
// This allows the CUE generator to automatically detect and add required imports.
type ImportRequirer interface {
	// RequiredImports returns the list of CUE imports this type requires.
	RequiredImports() []string
}

// NewCUEGenerator creates a new CUE generator.
func NewCUEGenerator() *CUEGenerator {
	return &CUEGenerator{
		indent:  "\t",
		imports: []string{},
	}
}

// WithImports adds CUE imports to the generator.
// Usage: gen.WithImports(CUEImports.Strconv, CUEImports.Strings)
func (g *CUEGenerator) WithImports(imports ...string) *CUEGenerator {
	g.imports = append(g.imports, imports...)
	return g
}

// detectRequiredImports analyzes the component template and automatically adds
// any required CUE standard library imports by checking all values for ImportRequirer.
func (g *CUEGenerator) detectRequiredImports(c *ComponentDefinition) {
	// Execute the template to capture what constructs are used
	tpl := NewTemplate()
	if templateFn := c.GetTemplate(); templateFn != nil {
		templateFn(tpl)
	}

	// Collect imports from all template elements using the ImportRequirer interface
	// This is extensible - any type that implements ImportRequirer will have its imports added

	// Check concat helpers (list.Concat requires "list")
	for _, helper := range tpl.GetConcatHelpers() {
		g.collectImportsFromValue(helper)
	}

	// Check struct array helpers
	for _, helper := range tpl.GetStructArrayHelpers() {
		g.collectImportsFromValue(helper)
	}

	// Check dedupe helpers
	for _, helper := range tpl.GetDedupeHelpers() {
		g.collectImportsFromValue(helper)
	}

	// Check helper variables (before and after output)
	for _, helper := range tpl.GetHelpersBeforeOutput() {
		g.collectImportsFromValue(helper)
		g.collectImportsFromValue(helper.Collection())
	}
	for _, helper := range tpl.GetHelpersAfterOutput() {
		g.collectImportsFromValue(helper)
		g.collectImportsFromValue(helper.Collection())
	}

	// Check resource operations in output
	if output := tpl.GetOutput(); output != nil {
		g.collectImportsFromResource(output)
	}

	// Check resource operations in outputs
	for _, res := range tpl.GetOutputs() {
		g.collectImportsFromResource(res)
	}
}

// addImportIfMissing adds an import if it's not already present.
func (g *CUEGenerator) addImportIfMissing(imp string) {
	for _, existing := range g.imports {
		if existing == imp {
			return
		}
	}
	g.imports = append(g.imports, imp)
}

// collectImportsFromValue checks if a value implements ImportRequirer and adds its imports.
// It also recursively checks nested values.
func (g *CUEGenerator) collectImportsFromValue(v interface{}) {
	if v == nil {
		return
	}

	// Check if the value itself requires imports
	if ir, ok := v.(ImportRequirer); ok {
		for _, imp := range ir.RequiredImports() {
			g.addImportIfMissing(imp)
		}
	}

	// Recursively check nested structures
	switch val := v.(type) {
	case *CollectionOp:
		g.collectImportsFromValue(val.Source())
		for _, op := range val.Operations() {
			if mOp, ok := op.(*mapOp); ok {
				for _, fv := range mOp.mappings {
					g.collectImportsFromFieldValue(fv)
				}
			}
		}
	case *MultiSource:
		g.collectImportsFromValue(val.Source())
		for _, fm := range val.MapBySourceMappings() {
			for _, fv := range fm {
				g.collectImportsFromFieldValue(fv)
			}
		}
	case *ArrayBuilder:
		for _, entry := range val.Entries() {
			if entry.itemBuilder != nil {
				g.collectImportsFromItemOps(entry.itemBuilder.Ops())
			}
		}
	case *PlusExpr:
		for _, part := range val.Parts() {
			g.collectImportsFromValue(part)
		}
	}
}

// collectImportsFromFieldValue checks field values for import requirements.
func (g *CUEGenerator) collectImportsFromFieldValue(fv FieldValue) {
	if fv == nil {
		return
	}

	// Check if the field value requires imports
	if ir, ok := fv.(ImportRequirer); ok {
		for _, imp := range ir.RequiredImports() {
			g.addImportIfMissing(imp)
		}
	}

	// Check nested field values
	switch val := fv.(type) {
	case *OrFieldRef:
		g.collectImportsFromFieldValue(val.fallback)
	case *ConditionalOrFieldRef:
		g.collectImportsFromFieldValue(val.fallback)
	case *NestedField:
		for _, nested := range val.mapping {
			g.collectImportsFromFieldValue(nested)
		}
	}
}

// collectImportsFromItemOps recursively scans ItemBuilder operations for import requirements.
func (g *CUEGenerator) collectImportsFromItemOps(ops []itemOp) {
	for _, op := range ops {
		switch o := op.(type) {
		case setOp:
			g.collectImportsFromValue(o.value)
		case letOp:
			g.collectImportsFromValue(o.value)
		case setDefaultOp:
			g.collectImportsFromValue(o.defValue)
		case ifBlockOp:
			g.collectImportsFromItemOps(o.body)
		}
	}
}

// collectImportsFromResource checks all operations in a resource for import requirements.
func (g *CUEGenerator) collectImportsFromResource(res *Resource) {
	if res == nil {
		return
	}

	for _, op := range res.Ops() {
		switch o := op.(type) {
		case *SetOp:
			g.collectImportsFromValue(o.Value())
		case *SetIfOp:
			g.collectImportsFromValue(o.Value())
		case *SpreadIfOp:
			g.collectImportsFromValue(o.Value())
		case *IfBlock:
			for _, innerOp := range o.Ops() {
				switch inner := innerOp.(type) {
				case *SetOp:
					g.collectImportsFromValue(inner.Value())
				case *SetIfOp:
					g.collectImportsFromValue(inner.Value())
				}
			}
		}
	}
}

// GenerateParameterSchema generates the CUE parameter schema from a component definition.
// This generates only the `parameter: { ... }` block for comparison with original CUE.
func (g *CUEGenerator) GenerateParameterSchema(c *ComponentDefinition) string {
	var sb strings.Builder
	sb.WriteString("parameter: {\n")

	for _, param := range c.GetParams() {
		g.writeParam(&sb, param, 1)
	}

	// Write top-level validators
	for _, v := range c.GetValidators() {
		g.writeValidator(&sb, v, 1)
	}

	// Write conditional parameter blocks
	for _, block := range c.GetConditionalParamBlocks() {
		g.writeConditionalParamBlock(&sb, block, 1)
	}

	sb.WriteString("}\n")
	return sb.String()
}

// GenerateFullDefinition generates the complete CUE definition from a component.
func (g *CUEGenerator) GenerateFullDefinition(c *ComponentDefinition) string {
	// Auto-detect required imports from template
	g.detectRequiredImports(c)

	var sb strings.Builder

	// Write imports if any
	if len(g.imports) > 0 {
		sb.WriteString("import (\n")
		for _, imp := range g.imports {
			sb.WriteString(fmt.Sprintf("\t%q\n", imp))
		}
		sb.WriteString(")\n\n")
	}

	// Write component header
	sb.WriteString(fmt.Sprintf("%s: {\n", cueLabel(c.GetName())))
	sb.WriteString(fmt.Sprintf("%stype: \"component\"\n", g.indent))
	if c.GetAnnotations() != nil && len(c.GetAnnotations()) > 0 {
		keys := make([]string, 0, len(c.GetAnnotations()))
		for k := range c.GetAnnotations() {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		sb.WriteString(fmt.Sprintf("%sannotations: {\n", g.indent))
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf("%s\t%q: %q\n", g.indent, k, c.GetAnnotations()[k]))
		}
		sb.WriteString(fmt.Sprintf("%s}\n", g.indent))
	} else {
		sb.WriteString(fmt.Sprintf("%sannotations: {}\n", g.indent))
	}
	if len(c.GetLabels()) > 0 {
		labelKeys := make([]string, 0, len(c.GetLabels()))
		for k := range c.GetLabels() {
			labelKeys = append(labelKeys, k)
		}
		sort.Strings(labelKeys)
		sb.WriteString(fmt.Sprintf("%slabels: {\n", g.indent))
		for _, k := range labelKeys {
			sb.WriteString(fmt.Sprintf("%s\t%q: %q\n", g.indent, k, c.GetLabels()[k]))
		}
		sb.WriteString(fmt.Sprintf("%s}\n", g.indent))
	} else {
		sb.WriteString(fmt.Sprintf("%slabels: {}\n", g.indent))
	}
	sb.WriteString(fmt.Sprintf("%sdescription: %q\n", g.indent, c.GetDescription()))
	if c.GetVersion() != "" {
		sb.WriteString(fmt.Sprintf("%sversion: %q\n", g.indent, c.GetVersion()))
	}

	// Write attributes
	sb.WriteString(fmt.Sprintf("%sattributes: {\n", g.indent))
	g.writeWorkload(&sb, c, 2)
	g.writeStatus(&sb, c, 2)
	sb.WriteString(fmt.Sprintf("%s}\n", g.indent))

	sb.WriteString("}\n")

	// Write template section (includes parameter inside)
	sb.WriteString(g.GenerateTemplate(c))

	return sb.String()
}

// GenerateTemplate generates the CUE template block from a component's template function.
// The template block contains: helpers, output, outputs, parameter, and helper definitions.
// The order follows KubeVela conventions:
//  1. Primary helpers (before output) - mountsArray, volumesList, deDupVolumesList
//  2. output: block - the main resource
//  3. Auxiliary helpers (after output) - exposePorts and similar helpers used by outputs
//  4. outputs: block - auxiliary resources like Service
//  5. parameter: block - parameter schema
//  6. Helper type definitions - #HealthProbe and similar
func (g *CUEGenerator) GenerateTemplate(c *ComponentDefinition) string {
	var sb strings.Builder
	sb.WriteString("template: {\n")

	// Execute the template function to capture operations
	tpl := NewTemplate()
	if templateFn := c.GetTemplate(); templateFn != nil {
		templateFn(tpl)
	}

	// Generate struct-based array helpers first (mountsArray, volumesArray patterns)
	for _, helper := range tpl.GetStructArrayHelpers() {
		g.writeStructArrayHelper(&sb, helper, 1)
	}

	// Generate concat helpers (list.Concat patterns)
	for _, helper := range tpl.GetConcatHelpers() {
		g.writeConcatHelper(&sb, helper, 1)
	}

	// Generate dedupe helpers (deDupVolumesArray pattern)
	for _, helper := range tpl.GetDedupeHelpers() {
		g.writeDedupeHelper(&sb, helper, 1)
	}

	// Generate legacy helper definitions that appear BEFORE output
	for _, helper := range tpl.GetHelpersBeforeOutput() {
		g.writeHelper(&sb, helper, 1)
	}

	// Generate output block
	if output := tpl.GetOutput(); output != nil {
		g.writeResourceOutput(&sb, "output", output, nil, 1)
	}

	// Generate helper definitions that appear AFTER output (used by outputs)
	// This matches KubeVela convention where exposePorts appears between output and outputs
	for _, helper := range tpl.GetHelpersAfterOutput() {
		g.writeHelper(&sb, helper, 1)
	}

	// Generate outputs block for auxiliary resources
	if outputs := tpl.GetOutputs(); len(outputs) > 0 {
		outputNames := make([]string, 0, len(outputs))
		for name := range outputs {
			outputNames = append(outputNames, name)
		}
		sort.Strings(outputNames)
		sb.WriteString(fmt.Sprintf("%soutputs: {\n", g.indent))
		for _, name := range outputNames {
			res := outputs[name]
			g.writeResourceOutput(&sb, name, res, res.outputCondition, 2)
		}
		sb.WriteString(fmt.Sprintf("%s}\n", g.indent))
	}

	// Generate parameter section INSIDE template block (KubeVela convention)
	sb.WriteString(g.generateParameterBlock(c, 1))

	// Generate helper type definitions (like #HealthProbe)
	for _, helperDef := range c.GetHelperDefinitions() {
		g.WriteHelperDefinition(&sb, helperDef, 1)
	}

	sb.WriteString("}\n")
	return sb.String()
}

// generateParameterBlock generates the parameter schema at the specified depth.
func (g *CUEGenerator) generateParameterBlock(c *ComponentDefinition, depth int) string {
	var sb strings.Builder
	indent := strings.Repeat(g.indent, depth)

	sb.WriteString(fmt.Sprintf("%sparameter: {\n", indent))

	for _, param := range c.GetParams() {
		g.writeParam(&sb, param, depth+1)
	}

	// Write top-level validators
	for _, v := range c.GetValidators() {
		g.writeValidator(&sb, v, depth+1)
	}

	// Write conditional parameter blocks
	for _, block := range c.GetConditionalParamBlocks() {
		g.writeConditionalParamBlock(&sb, block, depth+1)
	}

	sb.WriteString(fmt.Sprintf("%s}\n", indent))
	return sb.String()
}

// writeValidator writes a CUE _validate* block.
// Example output:
//
//	_validateTenantName: {
//	    "tenantName must not end with a hyphen": true
//	    if tenantName =~ ".*-$" {
//	        "tenantName must not end with a hyphen": false
//	    }
//	}
func (g *CUEGenerator) writeValidator(sb *strings.Builder, v *Validator, depth int) {
	indent := strings.Repeat(g.indent, depth)
	inner := strings.Repeat(g.indent, depth+1)
	inner2 := strings.Repeat(g.indent, depth+2)

	// Determine the CUE variable name
	name := v.CUEName()
	if name == "" {
		// Auto-generate from message — use a simple underscore-prefixed name
		name = "_validate"
	}

	if v.GuardCondition() != nil {
		// Guarded validator: wrap in if guard { ... }
		guardCUE := g.conditionToCUE(v.GuardCondition())
		sb.WriteString(fmt.Sprintf("%sif %s {\n", indent, guardCUE))
		sb.WriteString(fmt.Sprintf("%s%s: {\n", inner, name))
		sb.WriteString(fmt.Sprintf("%s%q: true\n", inner2, v.Message()))
		if v.FailCondition() != nil {
			failCUE := g.conditionToCUE(v.FailCondition())
			sb.WriteString(fmt.Sprintf("%sif %s {\n", inner2, failCUE))
			sb.WriteString(fmt.Sprintf("%s\t%q: false\n", inner2, v.Message()))
			sb.WriteString(fmt.Sprintf("%s}\n", inner2))
		}
		sb.WriteString(fmt.Sprintf("%s}\n", inner))
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	} else {
		// Unguarded validator
		sb.WriteString(fmt.Sprintf("%s%s: {\n", indent, name))
		sb.WriteString(fmt.Sprintf("%s%q: true\n", inner, v.Message()))
		if v.FailCondition() != nil {
			failCUE := g.conditionToCUE(v.FailCondition())
			sb.WriteString(fmt.Sprintf("%sif %s {\n", inner, failCUE))
			sb.WriteString(fmt.Sprintf("%s\t%q: false\n", inner, v.Message()))
			sb.WriteString(fmt.Sprintf("%s}\n", inner))
		}
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	}
}

// writeNonEmptyCheck writes a non-empty array check.
// Example output: if len(name) == 0 { _|_("message") }
// Uses local field name (no "parameter." prefix) since the check is emitted
// inside the parameter block where the field is a local sibling.
func (g *CUEGenerator) writeNonEmptyCheck(sb *strings.Builder, paramName, message string, depth int) {
	indent := strings.Repeat(g.indent, depth)
	inner := strings.Repeat(g.indent, depth+1)
	sb.WriteString(fmt.Sprintf("%sif len(%s) == 0 {\n", indent, paramName))
	sb.WriteString(fmt.Sprintf("%s_|_(%q)\n", inner, message))
	sb.WriteString(fmt.Sprintf("%s}\n", indent))
}

// writeConditionalParamBlock writes conditional parameter branches.
// Example output:
//
//	if existingResources == false {
//	    forceDestroy: *false | bool
//	}
//	if existingResources == true {
//	    forceDestroy?: bool
//	}
func (g *CUEGenerator) writeConditionalParamBlock(sb *strings.Builder, block *ConditionalParamBlock, depth int) {
	indent := strings.Repeat(g.indent, depth)

	for _, branch := range block.Branches() {
		condCUE := g.conditionToCUE(branch.Condition())
		sb.WriteString(fmt.Sprintf("%sif %s {\n", indent, condCUE))

		// Write params inside the conditional block
		for _, param := range branch.GetParams() {
			g.writeParam(sb, param, depth+1)
		}

		// Write validators inside the conditional block
		for _, v := range branch.GetValidators() {
			g.writeValidator(sb, v, depth+1)
		}

		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	}
}

// writeConditionalStructOp writes a conditional struct block in the output.
// Example output:
//
//	if parameter["replicationConfiguration"] != _|_ {
//	    replicationConfiguration: {
//	        role: parameter.replicationConfiguration.role
//	    }
//	}
func (g *CUEGenerator) writeConditionalStructOp(sb *strings.Builder, cs *ConditionalStructOp, depth int) {
	indent := strings.Repeat(g.indent, depth)

	// Build the struct
	builder := &OutputStructBuilder{}
	cs.Builder()(builder)

	condCUE := g.conditionToCUE(cs.Cond())
	sb.WriteString(fmt.Sprintf("%sif %s {\n", indent, condCUE))

	// Split the path into segments and open nested structs
	parts := splitPath(cs.Path())
	currentIndent := indent + g.indent
	for _, part := range parts {
		sb.WriteString(fmt.Sprintf("%s%s: {\n", currentIndent, cueLabel(part)))
		currentIndent += g.indent
	}

	// Write builder operations
	for _, op := range builder.Ops() {
		switch o := op.(type) {
		case *structSetOp:
			valCUE := g.valueToCUE(o.value)
			sb.WriteString(fmt.Sprintf("%s%s: %s\n", currentIndent, cueLabel(o.field), valCUE))
		case *structSetIfOp:
			condStr := g.conditionToCUE(o.cond)
			sb.WriteString(fmt.Sprintf("%sif %s {\n", currentIndent, condStr))
			valCUE := g.valueToCUE(o.value)
			sb.WriteString(fmt.Sprintf("%s\t%s: %s\n", currentIndent, cueLabel(o.field), valCUE))
			sb.WriteString(fmt.Sprintf("%s}\n", currentIndent))
		}
	}

	// Close nested structs
	for i := len(parts) - 1; i >= 0; i-- {
		currentIndent = currentIndent[:len(currentIndent)-len(g.indent)]
		sb.WriteString(fmt.Sprintf("%s}\n", currentIndent))
	}

	// Close the if block
	sb.WriteString(fmt.Sprintf("%s}\n", indent))
}

// WriteHelperDefinition writes a CUE helper type definition like #HealthProbe.
// This method is exported so it can be used by policy and workflow step generators.
func (g *CUEGenerator) WriteHelperDefinition(sb *strings.Builder, def HelperDefinition, depth int) {
	indent := strings.Repeat(g.indent, depth)

	if def.HasParam() {
		// Generate schema from Param using fluent API
		sb.WriteString(fmt.Sprintf("%s#%s: ", indent, def.GetName()))
		g.writeHelperDefFromParam(sb, def.GetParam(), depth)
	} else {
		// Legacy: raw CUE schema
		sb.WriteString(fmt.Sprintf("%s#%s: %s\n", indent, def.GetName(), def.GetSchema()))
	}
}

// writeHelperDefFromParam writes a helper definition schema from a Param.
func (g *CUEGenerator) writeHelperDefFromParam(sb *strings.Builder, param Param, depth int) {
	switch p := param.(type) {
	case *StructParam:
		// For struct types defined with defkit.Struct(), write the fields as a struct schema
		fields := p.GetFields()
		if len(fields) > 0 {
			sb.WriteString("{\n")
			for _, field := range fields {
				g.writeStructFieldForHelper(sb, field, depth+1)
			}
			sb.WriteString(fmt.Sprintf("%s}\n", strings.Repeat(g.indent, depth)))
		} else {
			sb.WriteString("{}\n")
		}
	case *MapParam:
		// For objects, write the fields directly as a struct
		fields := p.GetFields()
		if len(fields) > 0 {
			sb.WriteString("{\n")
			for _, field := range fields {
				g.writeParam(sb, field, depth+1)
			}
			sb.WriteString(fmt.Sprintf("%s}\n", strings.Repeat(g.indent, depth)))
		} else {
			sb.WriteString("{}\n")
		}
	case *ArrayParam:
		// For arrays, write as [...{fields}]
		fields := p.GetFields()
		if len(fields) > 0 {
			sb.WriteString("[...{\n")
			for _, field := range fields {
				g.writeParam(sb, field, depth+1)
			}
			sb.WriteString(fmt.Sprintf("%s}]\n", strings.Repeat(g.indent, depth)))
		} else {
			sb.WriteString("[...]\n")
		}
	case *ClosedUnionParam:
		// For closed unions, write as: close({...}) | close({...})
		options := p.GetOptions()
		if len(options) == 0 {
			sb.WriteString("_\n")
		} else {
			indent := strings.Repeat(g.indent, depth)
			for i, opt := range options {
				if i > 0 {
					sb.WriteString(" | ")
				}
				sb.WriteString("close({\n")
				for _, field := range opt.GetFields() {
					g.writeStructFieldForHelper(sb, field, depth+1)
				}
				sb.WriteString(fmt.Sprintf("%s})", indent))
			}
			sb.WriteString("\n")
		}
	case *IntParam:
		// For int types with optional constraints: int & >=1 & <=65535
		var constraints []string
		if minVal := p.GetMin(); minVal != nil {
			constraints = append(constraints, fmt.Sprintf(">=%d", *minVal))
		}
		if maxVal := p.GetMax(); maxVal != nil {
			constraints = append(constraints, fmt.Sprintf("<=%d", *maxVal))
		}
		if len(constraints) > 0 {
			sb.WriteString(fmt.Sprintf("int & %s\n", strings.Join(constraints, " & ")))
		} else {
			sb.WriteString("int\n")
		}
	default:
		// Fallback for other types
		sb.WriteString("_\n")
	}
}

// writeStructFieldForHelper writes a struct field specifically for helper definitions.
// This handles StructField objects which are different from Param objects.
func (g *CUEGenerator) writeStructFieldForHelper(sb *strings.Builder, f *StructField, depth int) {
	indent := strings.Repeat(g.indent, depth)

	// Write description as comment if present
	if desc := f.GetDescription(); desc != "" {
		sb.WriteString(fmt.Sprintf("%s// +usage=%s\n", indent, desc))
	}

	name := f.Name()
	marker := fieldMarkerNone
	if f.IsRequired() {
		marker = fieldMarkerRequired
	} else if f.IsOptional() {
		marker = fieldMarkerOptional
	}

	// Check if this field references another helper type
	if schemaRef := f.GetSchemaRef(); schemaRef != "" {
		// Reference to a helper definition like #ResourcePolicyRuleSelector
		// Check if this is an array type - if so, output [...#SchemaRef]
		if f.FieldType() == ParamTypeArray {
			if f.HasDefault() {
				sb.WriteString(fmt.Sprintf("%s%s: *%v | [...#%s]\n", indent, name, formatCUEValue(f.GetDefault()), schemaRef))
			} else {
				sb.WriteString(fmt.Sprintf("%s%s%s: [...#%s]\n", indent, name, marker, schemaRef))
			}
		} else {
			if f.HasDefault() {
				sb.WriteString(fmt.Sprintf("%s%s: *%v | #%s\n", indent, name, formatCUEValue(f.GetDefault()), schemaRef))
			} else {
				sb.WriteString(fmt.Sprintf("%s%s%s: #%s\n", indent, name, marker, schemaRef))
			}
		}
		return
	}

	// Check for nested struct (or array of structs)
	if nested := f.GetNested(); nested != nil {
		if f.FieldType() == ParamTypeArray {
			// Array of structs: [...{fields}]
			sb.WriteString(fmt.Sprintf("%s%s%s: [...{\n", indent, name, marker))
			for _, nestedField := range nested.GetFields() {
				g.writeStructFieldForHelper(sb, nestedField, depth+1)
			}
			sb.WriteString(fmt.Sprintf("%s}]\n", indent))
		} else {
			sb.WriteString(fmt.Sprintf("%s%s%s: {\n", indent, name, marker))
			for _, nestedField := range nested.GetFields() {
				g.writeStructFieldForHelper(sb, nestedField, depth+1)
			}
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
		}
		return
	}

	// Get CUE type for the field type
	fieldType := g.cueTypeForParamType(f.FieldType())

	switch {
	case f.HasDefault():
		enumValues := f.GetEnumValues()
		switch {
		case len(enumValues) > 0:
			// Enum with default: *"default" | "other1" | "other2"
			defaultStr := fmt.Sprintf("%v", f.GetDefault())
			var enumParts []string
			enumParts = append(enumParts, fmt.Sprintf("*%s", formatCUEValue(f.GetDefault())))
			for _, v := range enumValues {
				if v != defaultStr {
					enumParts = append(enumParts, fmt.Sprintf("%q", v))
				}
			}
			sb.WriteString(fmt.Sprintf("%s%s: %s\n", indent, name, strings.Join(enumParts, " | ")))
		case f.FieldType() == ParamTypeArray && f.GetElementType() != "":
			elemCUE := g.cueTypeForParamType(f.GetElementType())
			sb.WriteString(fmt.Sprintf("%s%s: *%v | [...%s]\n", indent, name, formatCUEValue(f.GetDefault()), elemCUE))
		default:
			sb.WriteString(fmt.Sprintf("%s%s: *%v | %s\n", indent, name, formatCUEValue(f.GetDefault()), fieldType))
		}
	case f.FieldType() == ParamTypeArray && f.GetElementType() != "":
		elemCUE := g.cueTypeForParamType(f.GetElementType())
		sb.WriteString(fmt.Sprintf("%s%s%s: [...%s]\n", indent, name, marker, elemCUE))
	case len(f.GetEnumValues()) > 0:
		// Enum without default: "value1" | "value2"
		var enumParts []string
		for _, v := range f.GetEnumValues() {
			enumParts = append(enumParts, fmt.Sprintf("%q", v))
		}
		sb.WriteString(fmt.Sprintf("%s%s%s: %s\n", indent, name, marker, strings.Join(enumParts, " | ")))
	default:
		sb.WriteString(fmt.Sprintf("%s%s%s: %s\n", indent, name, marker, fieldType))
	}
}

// writeStructArrayHelper writes a struct-based array helper like mountsArray or volumesArray.
// This generates the pattern:
//
//	mountsArray: {
//	    pvc: *[for v in parameter.volumeMounts.pvc {...}] | []
//	    configMap: *[...] | []
//	    ...
//	}
func (g *CUEGenerator) writeStructArrayHelper(sb *strings.Builder, helper *StructArrayHelper, depth int) {
	indent := strings.Repeat(g.indent, depth)
	innerIndent := strings.Repeat(g.indent, depth+1)
	fieldIndent := strings.Repeat(g.indent, depth+2)
	nestedIndent := strings.Repeat(g.indent, depth+3)

	sb.WriteString(fmt.Sprintf("%s%s: {\n", indent, helper.HelperName()))

	sourceStr := g.valueToCUE(helper.Source())

	for _, field := range helper.Fields() {
		sb.WriteString(fmt.Sprintf("%s%s: *[\n", innerIndent, field.Name))
		sb.WriteString(fmt.Sprintf("%sfor v in %s.%s {\n", fieldIndent, sourceStr, field.Name))
		sb.WriteString(fmt.Sprintf("%s{\n", fieldIndent))

		// Write field mappings
		g.writeStructArrayFieldMappings(sb, field.Mappings, nestedIndent)

		sb.WriteString(fmt.Sprintf("%s}\n", fieldIndent))
		sb.WriteString(fmt.Sprintf("%s},\n", fieldIndent))
		sb.WriteString(fmt.Sprintf("%s] | []\n\n", innerIndent))
	}

	sb.WriteString(fmt.Sprintf("%s}\n", indent))
}

// writeStructArrayFieldMappings writes the field mappings inside a struct array field.
func (g *CUEGenerator) writeStructArrayFieldMappings(sb *strings.Builder, mappings FieldMap, indent string) {
	fieldNames := sortedKeys(mappings)
	for _, fieldName := range fieldNames {
		fieldVal := mappings[fieldName]
		// Handle nested objects like persistentVolumeClaim: claimName: v.claimName
		if strings.Contains(fieldName, ".") {
			parts := strings.Split(fieldName, ".")
			if len(parts) == 2 {
				valStr := g.fieldValueToCUE(fieldVal)
				sb.WriteString(fmt.Sprintf("%s%s: %s: %s\n", indent, parts[0], parts[1], valStr))
			}
		} else if nf, isNested := fieldVal.(*NestedField); isNested {
			// Handle NestedField with its own mapping
			sb.WriteString(fmt.Sprintf("%s%s: {\n", indent, fieldName))
			for _, nestedName := range sortedKeys(nf.mapping) {
				nestedVal := nf.mapping[nestedName]
				if optField, isOptional := nestedVal.(*OptionalField); isOptional {
					sb.WriteString(fmt.Sprintf("%s\tif v.%s != _|_ {\n", indent, optField.field))
					sb.WriteString(fmt.Sprintf("%s\t\t%s: v.%s\n", indent, nestedName, optField.field))
					sb.WriteString(fmt.Sprintf("%s\t}\n", indent))
				} else {
					valStr := g.fieldValueToCUE(nestedVal)
					sb.WriteString(fmt.Sprintf("%s\t%s: %s\n", indent, nestedName, valStr))
				}
			}
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
		} else if optField, isOptional := fieldVal.(*OptionalField); isOptional {
			// Handle optional fields with conditional inclusion
			sb.WriteString(fmt.Sprintf("%sif v.%s != _|_ {\n", indent, optField.field))
			sb.WriteString(fmt.Sprintf("%s\t%s: v.%s\n", indent, fieldName, optField.field))
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
		} else {
			valStr := g.fieldValueToCUE(fieldVal)
			sb.WriteString(fmt.Sprintf("%s%s: %s\n", indent, fieldName, valStr))
		}
	}
}

// writeConcatHelper writes a list.Concat helper.
// This generates: volumesList: list.Concat([volumesArray.pvc, volumesArray.configMap, ...])
func (g *CUEGenerator) writeConcatHelper(sb *strings.Builder, helper *ConcatHelper, depth int) {
	indent := strings.Repeat(g.indent, depth)

	sourceName := helper.Source().HelperName()
	fields := helper.FieldRefs()

	// Build the field references
	var refs []string
	for _, field := range fields {
		refs = append(refs, fmt.Sprintf("%s.%s", sourceName, field))
	}

	sb.WriteString(fmt.Sprintf("%s%s: list.Concat([%s])\n",
		indent, helper.HelperName(), strings.Join(refs, ", ")))
}

// writeDedupeHelper writes a deduplication helper.
// This generates the pattern:
//
//	deDupVolumesArray: [
//	    for val in [
//	        for i, vi in volumesList {
//	            for j, vj in volumesList if j < i && vi.name == vj.name {
//	                _ignore: true
//	            }
//	            vi
//	        },
//	    ] if val._ignore == _|_ {
//	        val
//	    },
//	]
func (g *CUEGenerator) writeDedupeHelper(sb *strings.Builder, helper *DedupeHelper, depth int) {
	indent := strings.Repeat(g.indent, depth)

	// Get the source name
	var sourceName string
	switch src := helper.Source().(type) {
	case *ConcatHelper:
		sourceName = src.HelperName()
	case *HelperVar:
		sourceName = src.Name()
	case *StructArrayHelper:
		sourceName = src.HelperName()
	default:
		sourceName = g.valueToCUE(helper.Source())
	}

	keyField := helper.KeyField()

	sb.WriteString(fmt.Sprintf("%s%s: [\n", indent, helper.HelperName()))
	sb.WriteString(fmt.Sprintf("%s\tfor val in [\n", indent))
	sb.WriteString(fmt.Sprintf("%s\t\tfor i, vi in %s {\n", indent, sourceName))
	sb.WriteString(fmt.Sprintf("%s\t\t\tfor j, vj in %s if j < i && vi.%s == vj.%s {\n",
		indent, sourceName, keyField, keyField))
	sb.WriteString(fmt.Sprintf("%s\t\t\t\t_ignore: true\n", indent))
	sb.WriteString(fmt.Sprintf("%s\t\t\t}\n", indent))
	sb.WriteString(fmt.Sprintf("%s\t\t\tvi\n", indent))
	sb.WriteString(fmt.Sprintf("%s\t\t},\n", indent))
	sb.WriteString(fmt.Sprintf("%s\t] if val._ignore == _|_ {\n", indent))
	sb.WriteString(fmt.Sprintf("%s\t\tval\n", indent))
	sb.WriteString(fmt.Sprintf("%s\t},\n", indent))
	sb.WriteString(fmt.Sprintf("%s]\n", indent))
}

// writeHelper writes a helper definition at the template level.
func (g *CUEGenerator) writeHelper(sb *strings.Builder, helper *HelperVar, depth int) {
	indent := strings.Repeat(g.indent, depth)

	sb.WriteString(fmt.Sprintf("%s%s: ", indent, helper.Name()))

	// Generate the collection as a CUE expression
	collection := helper.Collection()
	guard := helper.Guard()
	switch col := collection.(type) {
	case *MultiSource:
		g.writeMultiSourceHelper(sb, col, depth)
	case *CollectionOp:
		g.writeCollectionOpHelper(sb, col, depth, guard)
	case *ArrayBuilder:
		sb.WriteString(g.arrayBuilderToCUE(col, depth))
	default:
		sb.WriteString("[]")
	}

	sb.WriteString("\n")
}

// writeMultiSourceHelper writes a MultiSource as a helper array definition.
func (g *CUEGenerator) writeMultiSourceHelper(sb *strings.Builder, ms *MultiSource, depth int) {
	sourceStr := g.valueToCUE(ms.Source())
	sources := ms.Sources()
	mapBySource := ms.MapBySourceMappings()
	ops := ms.Operations()

	// Check for pickIf operations and filter conditions
	var pickIfOps []*pickIfCollectionOp
	var pickFields []string
	filterCondition := ""
	for _, op := range ops {
		if pOp, ok := op.(*pickOp); ok {
			pickFields = pOp.fields
		}
		if piOp, ok := op.(*pickIfCollectionOp); ok {
			pickIfOps = append(pickIfOps, piOp)
		}
		var cond string
		if fOp, ok := op.(*filterOp); ok {
			cond = g.predicateToCUE(fOp.pred)
		} else if fOp, ok := op.(*filterCondCollectionOp); ok {
			cond = g.conditionToCUE(fOp.Cond())
		}
		if cond != "" {
			if filterCondition == "" {
				filterCondition = cond
			} else {
				filterCondition = filterCondition + " && " + cond
			}
		}
	}

	sb.WriteString("[\n")

	for i, source := range sources {
		if i > 0 {
			sb.WriteString(",\n")
		}

		innerIndent := strings.Repeat(g.indent, depth+1)
		fieldIndent := strings.Repeat(g.indent, depth+2)
		if filterCondition != "" {
			sb.WriteString(fmt.Sprintf("%sif %s != _|_ && %s.%s != _|_ for v in %s.%s if %s {\n",
				innerIndent, sourceStr, sourceStr, source, sourceStr, source, filterCondition))
		} else {
			sb.WriteString(fmt.Sprintf("%sif %s != _|_ && %s.%s != _|_ for v in %s.%s {\n",
				innerIndent, sourceStr, sourceStr, source, sourceStr, source))
		}
		sb.WriteString(fmt.Sprintf("%s{\n", innerIndent))

		// Check if we have MapBySource mappings for this source
		if mapping, hasMapping := mapBySource[source]; hasMapping {
			g.writeFieldMapAsHelper(sb, mapping, fieldIndent)
		} else if len(pickFields) > 0 {
			// Write pick fields
			for _, field := range pickFields {
				sb.WriteString(fmt.Sprintf("%s%s: v.%s\n", fieldIndent, field, field))
			}
		} else {
			sb.WriteString(fmt.Sprintf("%sv\n", fieldIndent))
		}

		// Write conditional fields from pickIf operations
		for _, piOp := range pickIfOps {
			condStr := g.fieldConditionToCUE(piOp.Cond())
			sb.WriteString(fmt.Sprintf("%sif %s {\n", fieldIndent, condStr))
			sb.WriteString(fmt.Sprintf("%s\t%s: v.%s\n", fieldIndent, piOp.Field(), piOp.Field()))
			sb.WriteString(fmt.Sprintf("%s}\n", fieldIndent))
		}

		sb.WriteString(fmt.Sprintf("%s}\n", innerIndent))
		sb.WriteString(fmt.Sprintf("%s}", innerIndent))
	}

	sb.WriteString(fmt.Sprintf("\n%s]", strings.Repeat(g.indent, depth)))
}

// writeCollectionOpHelper writes a CollectionOp as a helper array definition.
// The guard parameter is an optional outer condition that wraps the for loop.
func (g *CUEGenerator) writeCollectionOpHelper(sb *strings.Builder, col *CollectionOp, depth int, guard Condition) {
	sourceStr := g.valueToCUE(col.Source())
	ops := col.Operations()

	// Extract filter condition if present; AND-compose multiple filters
	var filterCondition string
	for _, op := range ops {
		var cond string
		if fOp, ok := op.(*filterOp); ok {
			cond = g.predicateToCUE(fOp.pred)
		} else if fOp, ok := op.(*filterCondCollectionOp); ok {
			cond = g.conditionToCUE(fOp.Cond())
		}
		if cond != "" {
			if filterCondition == "" {
				filterCondition = cond
			} else {
				filterCondition = filterCondition + " && " + cond
			}
		}
	}

	// Build the guard prefix (if guard != _|_ for v in source)
	var guardPrefix string
	if guard != nil {
		guardPrefix = "if " + g.conditionToCUE(guard) + " "
	}

	// Check for wrap operation (e.g., imagePullSecrets)
	for _, op := range ops {
		if wOp, ok := op.(*wrapOp); ok {
			if filterCondition != "" {
				sb.WriteString(fmt.Sprintf("[%sfor v in %s if %s { %s: v }]", guardPrefix, sourceStr, filterCondition, wOp.key))
			} else {
				sb.WriteString(fmt.Sprintf("[%sfor v in %s { %s: v }]", guardPrefix, sourceStr, wOp.key))
			}
			return
		}
	}

	// Check for map operations
	for _, op := range ops {
		if mOp, ok := op.(*mapOp); ok {
			innerIndent := strings.Repeat(g.indent, depth+1)
			fieldIndent := strings.Repeat(g.indent, depth+2)

			// Include guard and filter condition in the for loop if present
			// No extra braces - the for loop body directly contains the struct fields
			if filterCondition != "" {
				sb.WriteString(fmt.Sprintf("[\n%s%sfor v in %s if %s {\n", innerIndent, guardPrefix, sourceStr, filterCondition))
			} else {
				sb.WriteString(fmt.Sprintf("[\n%s%sfor v in %s {\n", innerIndent, guardPrefix, sourceStr))
			}

			for _, fieldName := range sortedKeys(mOp.mappings) {
				fieldVal := mOp.mappings[fieldName]
				if condRef, isConditional := fieldVal.(*ConditionalOrFieldRef); isConditional {
					// Emit if/else pattern for conditional field reference
					primaryField := string(condRef.primary)
					fallbackStr := g.fieldValueToCUE(condRef.fallback)
					sb.WriteString(fmt.Sprintf("%sif v.%s != _|_ {\n", fieldIndent, primaryField))
					sb.WriteString(fmt.Sprintf("%s\t%s: v.%s\n", fieldIndent, fieldName, primaryField))
					sb.WriteString(fmt.Sprintf("%s}\n", fieldIndent))
					sb.WriteString(fmt.Sprintf("%sif v.%s == _|_ {\n", fieldIndent, primaryField))
					sb.WriteString(fmt.Sprintf("%s\t%s: %s\n", fieldIndent, fieldName, fallbackStr))
					sb.WriteString(fmt.Sprintf("%s}\n", fieldIndent))
				} else if optField, isOptional := fieldVal.(*OptionalField); isOptional {
					sb.WriteString(fmt.Sprintf("%sif v.%s != _|_ {\n", fieldIndent, optField.field))
					sb.WriteString(fmt.Sprintf("%s\t%s: v.%s\n", fieldIndent, fieldName, optField.field))
					sb.WriteString(fmt.Sprintf("%s}\n", fieldIndent))
				} else if compOpt, isCompound := fieldVal.(*CompoundOptionalField); isCompound {
					condStr := g.conditionToCUE(compOpt.additionalCond)
					sb.WriteString(fmt.Sprintf("%sif v.%s != _|_ if %s {\n", fieldIndent, compOpt.field, condStr))
					sb.WriteString(fmt.Sprintf("%s\t%s: v.%s\n", fieldIndent, fieldName, compOpt.field))
					sb.WriteString(fmt.Sprintf("%s}\n", fieldIndent))
				} else {
					valStr := g.fieldValueToCUE(fieldVal)
					sb.WriteString(fmt.Sprintf("%s%s: %s\n", fieldIndent, fieldName, valStr))
				}
			}

			sb.WriteString(fmt.Sprintf("%s},\n%s]", innerIndent, strings.Repeat(g.indent, depth)))
			return
		}
	}

	// Check for deduplication
	for _, op := range ops {
		if dedup, ok := op.(*dedupeOp); ok {
			// For dedupe, use the elegant CUE pattern with _ignore marker
			// This pattern checks if any earlier item has the same key
			var sourceName string
			if helperRef, ok := col.Source().(*HelperVar); ok {
				sourceName = helperRef.Name()
			} else if ref, ok := col.Source().(*Ref); ok {
				sourceName = ref.Path()
			}
			if sourceName != "" {
				keyField := dedup.keyField
				sb.WriteString(fmt.Sprintf(`[
		for val in [
			for i, vi in %s {
				for j, vj in %s if j < i && vi.%s == vj.%s {
					_ignore: true
				}
				vi
			},
		] if val._ignore == _|_ {
			val
		},
	]`, sourceName, sourceName, keyField, keyField))
				return
			}
		}
	}

	// Default: simple list comprehension
	sb.WriteString(fmt.Sprintf("[for v in %s { v }]", sourceStr))
}

// writeFieldMapAsHelper writes a FieldMap as CUE fields.
func (g *CUEGenerator) writeFieldMapAsHelper(sb *strings.Builder, mapping FieldMap, indent string) {
	for _, fieldName := range sortedKeys(mapping) {
		fieldVal := mapping[fieldName]
		// Handle nested paths like "persistentVolumeClaim.claimName"
		if strings.Contains(fieldName, ".") {
			g.writeNestedFieldPath(sb, fieldName, fieldVal, indent)
		} else {
			valStr := g.fieldValueToCUE(fieldVal)
			// Check if this is an optional field
			if _, isOptional := fieldVal.(*OptionalField); isOptional {
				sb.WriteString(fmt.Sprintf("%sif v.%s != _|_ {\n", indent, fieldVal.(*OptionalField).field))
				sb.WriteString(fmt.Sprintf("%s\t%s: v.%s\n", indent, fieldName, fieldVal.(*OptionalField).field))
				sb.WriteString(fmt.Sprintf("%s}\n", indent))
			} else if compOpt, isCompound := fieldVal.(*CompoundOptionalField); isCompound {
				condStr := g.conditionToCUE(compOpt.additionalCond)
				sb.WriteString(fmt.Sprintf("%sif v.%s != _|_ if %s {\n", indent, compOpt.field, condStr))
				sb.WriteString(fmt.Sprintf("%s\t%s: v.%s\n", indent, fieldName, compOpt.field))
				sb.WriteString(fmt.Sprintf("%s}\n", indent))
			} else {
				sb.WriteString(fmt.Sprintf("%s%s: %s\n", indent, fieldName, valStr))
			}
		}
	}
}

// writeNestedFieldPath writes a nested field path like "persistentVolumeClaim.claimName".
func (g *CUEGenerator) writeNestedFieldPath(sb *strings.Builder, fieldPath string, fieldVal FieldValue, indent string) {
	parts := strings.Split(fieldPath, ".")
	if len(parts) < 2 {
		valStr := g.fieldValueToCUE(fieldVal)
		sb.WriteString(fmt.Sprintf("%s%s: %s\n", indent, fieldPath, valStr))
		return
	}

	// Build nested structure: persistentVolumeClaim: claimName: v.claimName
	parent := parts[0]
	child := parts[1]
	valStr := g.fieldValueToCUE(fieldVal)
	sb.WriteString(fmt.Sprintf("%s%s: %s: %s\n", indent, parent, child, valStr))
}

// fieldConditionToCUE converts a field-level condition to CUE.
func (g *CUEGenerator) fieldConditionToCUE(cond Condition) string {
	switch c := cond.(type) {
	case *IsSetCondition:
		// For field-level conditions, use v.field instead of parameter.field
		return fmt.Sprintf("v.%s != _|_", c.ParamName())
	default:
		return g.conditionToCUE(cond)
	}
}

// writeResourceOutput writes a resource as a CUE output block.
func (g *CUEGenerator) writeResourceOutput(sb *strings.Builder, name string, res *Resource, cond Condition, depth int) {
	indent := strings.Repeat(g.indent, depth)

	// Handle conditional output (wrap in if block)
	if cond != nil {
		condStr := g.conditionToCUE(cond)
		sb.WriteString(fmt.Sprintf("%sif %s {\n", indent, condStr))
		depth++
		indent = strings.Repeat(g.indent, depth)
	}

	quotedName := cueLabel(name)
	sb.WriteString(fmt.Sprintf("%s%s: {\n", indent, quotedName))
	innerIndent := strings.Repeat(g.indent, depth+1)

	// Handle conditional apiVersion or static apiVersion
	if res.HasVersionConditionals() {
		// Write each version conditional as an if block
		for _, vc := range res.VersionConditionals() {
			condStr := g.conditionToCUE(vc.Condition)
			sb.WriteString(fmt.Sprintf("%sif %s {\n", innerIndent, condStr))
			sb.WriteString(fmt.Sprintf("%s\tapiVersion: %q\n", innerIndent, vc.ApiVersion))
			sb.WriteString(fmt.Sprintf("%s}\n", innerIndent))
		}
	} else {
		// Static apiVersion
		sb.WriteString(fmt.Sprintf("%sapiVersion: %q\n", innerIndent, res.APIVersion()))
	}

	// Write kind
	sb.WriteString(fmt.Sprintf("%skind:       %q\n", innerIndent, res.Kind()))

	// Separate ConditionalStructOps from regular ops
	var regularOps []ResourceOp
	var conditionalStructs []*ConditionalStructOp
	for _, op := range res.Ops() {
		if cs, ok := op.(*ConditionalStructOp); ok {
			conditionalStructs = append(conditionalStructs, cs)
		} else {
			regularOps = append(regularOps, op)
		}
	}

	// Build a tree structure from the regular operations
	tree := g.buildFieldTree(regularOps)

	// Write the tree as CUE
	g.writeFieldTree(sb, tree, depth+1)

	// Write conditional struct blocks
	for _, cs := range conditionalStructs {
		g.writeConditionalStructOp(sb, cs, depth+1)
	}

	sb.WriteString(fmt.Sprintf("%s}\n", indent))

	// Close conditional block
	if cond != nil {
		indent = strings.Repeat(g.indent, depth-1)
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	}
}

// condValueEntry represents an additional conditional value at a field node.
// When the same path is written with multiple different conditions (e.g.,
// volumeMounts set under both "volumeMounts is set" and "volumes is set"),
// the first write uses value/cond and additional writes append here.
type condValueEntry struct {
	value Value
	cond  Condition
}

// fieldNode represents a node in the field tree being built.
type fieldNode struct {
	value         Value     // Direct value (if leaf)
	cond          Condition // Condition for this field
	children      map[string]*fieldNode
	childOrder    []string // Track insertion order
	isArray       bool
	arrayIndex    int
	spreads       []spreadEntry    // Spread operations at this node level
	forEach       *ForEachOp       // ForEach operation (for trait patches)
	patchKey      *PatchKeyOp      // PatchKey operation (for array patches with merge key)
	spreadAll     *SpreadAllOp     // SpreadAll operation (for array constraint patches)
	patchStrategy string           // e.g. "retainKeys" → generates // +patchStrategy=retainKeys
	directives    []string         // e.g. ["patchKey=ip"] → generates // +patchKey=ip
	condValues    []condValueEntry // additional conditional values at same path
}

// spreadEntry represents a conditional spread operation.
type spreadEntry struct {
	cond  Condition
	value Value
}

func newFieldNode() *fieldNode {
	return &fieldNode{
		children: make(map[string]*fieldNode),
	}
}

// buildFieldTree builds a tree structure from resource operations.
func (g *CUEGenerator) buildFieldTree(ops []ResourceOp) *fieldNode {
	root := newFieldNode()

	for _, op := range ops {
		switch o := op.(type) {
		case *SetOp:
			g.insertIntoTree(root, o.Path(), o.Value(), nil)
		case *SetIfOp:
			g.insertIntoTree(root, o.Path(), o.Value(), o.Cond())
		case *SpreadIfOp:
			// SpreadIfOp adds a spread entry to the target node
			g.insertSpreadIntoTree(root, o.Path(), o.Value(), o.Cond())
		case *ForEachOp:
			// ForEachOp creates a for-each iteration at the target path
			g.insertForEachIntoTree(root, o, nil)
		case *PatchKeyOp:
			// PatchKeyOp creates an array patch with merge key annotation
			g.insertPatchKeyIntoTree(root, o, nil)
		case *SpreadAllOp:
			// SpreadAllOp constrains all array elements
			g.insertSpreadAllIntoTree(root, o, nil)
		case *PatchStrategyAnnotationOp:
			g.insertAnnotationIntoTree(root, o.Path(), o.Strategy())
		case *DirectiveOp:
			g.insertDirectiveIntoTree(root, o.Path(), o.GetDirective())
		case *IfBlock:
			// For if blocks, process inner ops with the block's condition
			for _, innerOp := range o.Ops() {
				switch inner := innerOp.(type) {
				case *SetOp:
					g.insertIntoTree(root, inner.Path(), inner.Value(), o.Cond())
				case *SetIfOp:
					// Combine conditions
					combinedCond := &AndCondition{left: o.Cond(), right: inner.Cond()}
					g.insertIntoTree(root, inner.Path(), inner.Value(), combinedCond)
				case *SpreadIfOp:
					// Combine conditions for spread
					combinedCond := &AndCondition{left: o.Cond(), right: inner.Cond()}
					g.insertSpreadIntoTree(root, inner.Path(), inner.Value(), combinedCond)
				case *ForEachOp:
					// ForEach inside an if block - pass the block's condition
					g.insertForEachIntoTree(root, inner, o.Cond())
				case *PatchKeyOp:
					// PatchKey inside an if block - pass the block's condition
					g.insertPatchKeyIntoTree(root, inner, o.Cond())
				case *SpreadAllOp:
					// SpreadAll inside an if block - pass the block's condition
					g.insertSpreadAllIntoTree(root, inner, o.Cond())
				case *PatchStrategyAnnotationOp:
					g.insertAnnotationIntoTree(root, inner.Path(), inner.Strategy())
				case *DirectiveOp:
					g.insertDirectiveIntoTree(root, inner.Path(), inner.GetDirective())
				}
			}
		}
	}

	return root
}

// insertDirectiveIntoTree navigates to a node by path and adds a directive annotation.
func (g *CUEGenerator) insertDirectiveIntoTree(root *fieldNode, path string, directive string) {
	parts := splitPath(path)
	current := root
	for _, part := range parts {
		if _, exists := current.children[part]; !exists {
			current.children[part] = newFieldNode()
			current.childOrder = append(current.childOrder, part)
		}
		current = current.children[part]
	}
	current.directives = append(current.directives, directive)
}

// insertAnnotationIntoTree navigates to a node by path and sets its patchStrategy annotation.
func (g *CUEGenerator) insertAnnotationIntoTree(root *fieldNode, path string, strategy string) {
	parts := splitPath(path)
	current := root
	for _, part := range parts {
		if _, exists := current.children[part]; !exists {
			current.children[part] = newFieldNode()
			current.childOrder = append(current.childOrder, part)
		}
		current = current.children[part]
	}
	current.patchStrategy = strategy
}

// insertIntoTree inserts a value at a path into the field tree.
func (g *CUEGenerator) insertIntoTree(root *fieldNode, path string, value Value, cond Condition) {
	parts := splitPath(path)
	current := root

	for i, part := range parts {
		name, key, index := parseBracketAccess(part)

		// Get or create node for the field name
		if _, exists := current.children[name]; !exists {
			current.children[name] = newFieldNode()
			current.childOrder = append(current.childOrder, name)
		}
		node := current.children[name]

		// Handle array access
		switch {
		case index >= 0:
			node.isArray = true
			idxKey := fmt.Sprintf("[%d]", index)
			if _, exists := node.children[idxKey]; !exists {
				node.children[idxKey] = newFieldNode()
				node.children[idxKey].arrayIndex = index
				node.childOrder = append(node.childOrder, idxKey)
			}
			current = node.children[idxKey]
		case key != "":
			// Map key access (e.g., labels[app.oam.dev/name])
			// Create a special child for the key
			keyNode := fmt.Sprintf("[%s]", key)
			if _, exists := node.children[keyNode]; !exists {
				node.children[keyNode] = newFieldNode()
				node.childOrder = append(node.childOrder, keyNode)
			}
			current = node.children[keyNode]
		default:
			current = node
		}

		// If this is the last part, set the value
		if i == len(parts)-1 {
			if current.value != nil && cond != nil {
				// This path already has a value. If the existing value also
				// has a condition, append this as an additional conditional
				// value instead of overwriting.
				if current.cond != nil {
					current.condValues = append(current.condValues, condValueEntry{
						value: value,
						cond:  cond,
					})
				} else {
					// Existing value is unconditional; the new conditional
					// value takes precedence (shouldn't normally happen).
					current.value = value
					current.cond = cond
				}
			} else {
				current.value = value
				current.cond = cond
			}
		}
	}
}

// insertSpreadIntoTree inserts a spread operation at a path into the field tree.
// Unlike insertIntoTree which sets a value, this adds a spread entry that will
// be written inside the struct block at the given path.
func (g *CUEGenerator) insertSpreadIntoTree(root *fieldNode, path string, value Value, cond Condition) {
	parts := splitPath(path)
	current := root

	for _, part := range parts {
		name, key, index := parseBracketAccess(part)

		// Get or create node for the field name
		if _, exists := current.children[name]; !exists {
			current.children[name] = newFieldNode()
			current.childOrder = append(current.childOrder, name)
		}
		node := current.children[name]

		// Handle array access
		switch {
		case index >= 0:
			node.isArray = true
			idxKey := fmt.Sprintf("[%d]", index)
			if _, exists := node.children[idxKey]; !exists {
				node.children[idxKey] = newFieldNode()
				node.children[idxKey].arrayIndex = index
				node.childOrder = append(node.childOrder, idxKey)
			}
			current = node.children[idxKey]
		case key != "":
			// Map key access
			keyNode := fmt.Sprintf("[%s]", key)
			if _, exists := node.children[keyNode]; !exists {
				node.children[keyNode] = newFieldNode()
				node.childOrder = append(node.childOrder, keyNode)
			}
			current = node.children[keyNode]
		default:
			current = node
		}
	}

	// Add spread entry to the final node
	current.spreads = append(current.spreads, spreadEntry{cond: cond, value: value})
}

// insertForEachIntoTree inserts a ForEach operation at the given path.
// The cond parameter is used when the ForEach is inside an IfBlock.
func (g *CUEGenerator) insertForEachIntoTree(root *fieldNode, op *ForEachOp, cond Condition) {
	parts := splitPath(op.Path())
	current := root

	for _, part := range parts {
		name, key, index := parseBracketAccess(part)

		// Get or create node for the field name
		if _, exists := current.children[name]; !exists {
			current.children[name] = newFieldNode()
			current.childOrder = append(current.childOrder, name)
		}
		node := current.children[name]

		// Handle array access
		switch {
		case index >= 0:
			node.isArray = true
			idxKey := fmt.Sprintf("[%d]", index)
			if _, exists := node.children[idxKey]; !exists {
				node.children[idxKey] = newFieldNode()
				node.children[idxKey].arrayIndex = index
				node.childOrder = append(node.childOrder, idxKey)
			}
			current = node.children[idxKey]
		case key != "":
			// Map key access
			keyNode := fmt.Sprintf("[%s]", key)
			if _, exists := node.children[keyNode]; !exists {
				node.children[keyNode] = newFieldNode()
				node.childOrder = append(node.childOrder, keyNode)
			}
			current = node.children[keyNode]
		default:
			current = node
		}
	}

	// Set forEach on the final node with its condition
	current.forEach = op
	current.cond = cond
}

// insertPatchKeyIntoTree inserts a PatchKeyOp into the field tree.
// This navigates to the target path and sets the patchKey field.
// The cond parameter is used when the PatchKey is inside an IfBlock.
func (g *CUEGenerator) insertPatchKeyIntoTree(root *fieldNode, op *PatchKeyOp, cond Condition) {
	parts := splitPath(op.Path())
	current := root

	for _, part := range parts {
		name, key, index := parseBracketAccess(part)

		// Get or create node for the field name
		if _, exists := current.children[name]; !exists {
			current.children[name] = newFieldNode()
			current.childOrder = append(current.childOrder, name)
		}
		node := current.children[name]

		// Handle array access
		switch {
		case index >= 0:
			node.isArray = true
			idxKey := fmt.Sprintf("[%d]", index)
			if _, exists := node.children[idxKey]; !exists {
				node.children[idxKey] = newFieldNode()
				node.children[idxKey].arrayIndex = index
				node.childOrder = append(node.childOrder, idxKey)
			}
			current = node.children[idxKey]
		case key != "":
			// Map key access
			keyNode := fmt.Sprintf("[%s]", key)
			if _, exists := node.children[keyNode]; !exists {
				node.children[keyNode] = newFieldNode()
				node.childOrder = append(node.childOrder, keyNode)
			}
			current = node.children[keyNode]
		default:
			current = node
		}
	}

	// Set patchKey on the final node with its condition
	current.patchKey = op
	current.cond = cond
}

// insertSpreadAllIntoTree inserts a SpreadAllOp into the field tree.
// This navigates to the target path and sets the spreadAll field.
func (g *CUEGenerator) insertSpreadAllIntoTree(root *fieldNode, op *SpreadAllOp, cond Condition) {
	parts := splitPath(op.Path())
	current := root

	for _, part := range parts {
		name, key, index := parseBracketAccess(part)

		if _, exists := current.children[name]; !exists {
			current.children[name] = newFieldNode()
			current.childOrder = append(current.childOrder, name)
		}
		node := current.children[name]

		switch {
		case index >= 0:
			node.isArray = true
			idxKey := fmt.Sprintf("[%d]", index)
			if _, exists := node.children[idxKey]; !exists {
				node.children[idxKey] = newFieldNode()
				node.children[idxKey].arrayIndex = index
				node.childOrder = append(node.childOrder, idxKey)
			}
			current = node.children[idxKey]
		case key != "":
			keyNode := fmt.Sprintf("[%s]", key)
			if _, exists := node.children[keyNode]; !exists {
				node.children[keyNode] = newFieldNode()
				node.childOrder = append(node.childOrder, keyNode)
			}
			current = node.children[keyNode]
		default:
			current = node
		}
	}

	current.spreadAll = op
	current.cond = cond
}

// writeFieldTree writes the field tree as CUE syntax.
func (g *CUEGenerator) writeFieldTree(sb *strings.Builder, node *fieldNode, depth int) {
	indent := strings.Repeat(g.indent, depth)

	// Lift child conditions to the parent field when all children share the same
	// condition and the parent has no own value/spread/foreach/patchKey. This avoids
	// emitting empty parent structs like `foo: { if cond { bar: ... } }`.
	g.liftChildConditions(node)

	// Write spread entries FIRST (user labels spread before OAM labels)
	// This matches the KubeVela convention of spreading user-provided values
	// before adding fixed OAM labels, so user values can be overridden if needed.
	for _, spread := range node.spreads {
		condStr := g.conditionToCUE(spread.cond)
		valStr := g.valueToCUE(spread.value)
		sb.WriteString(fmt.Sprintf("%sif %s {\n", indent, condStr))
		sb.WriteString(fmt.Sprintf("%s\t%s\n", indent, valStr))
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	}

	// Group fields by their condition for cleaner output
	unconditional := make([]string, 0)
	conditional := make(map[string][]string) // condition string -> field names

	for _, name := range node.childOrder {
		child := node.children[name]
		switch {
		case len(child.condValues) > 0:
			// Multi-conditional nodes render their own if blocks internally,
			// so they must be treated as unconditional at the parent level.
			unconditional = append(unconditional, name)
		case child.cond != nil:
			condStr := g.conditionToCUE(child.cond)
			conditional[condStr] = append(conditional[condStr], name)
		default:
			unconditional = append(unconditional, name)
		}
	}

	// Write unconditional fields
	for _, name := range unconditional {
		child := node.children[name]
		g.writeFieldNode(sb, name, child, depth)
	}

	// Write conditional fields grouped by condition
	for _, condStr := range sortedKeys(conditional) {
		fieldNames := conditional[condStr]
		sb.WriteString(fmt.Sprintf("%sif %s {\n", indent, condStr))
		for _, name := range fieldNames {
			child := node.children[name]
			// Clear condition since we're inside the if block
			childCopy := *child
			childCopy.cond = nil
			g.writeFieldNode(sb, name, &childCopy, depth+1)
		}
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	}
}

// liftChildConditions promotes a shared child condition to the parent node.
// It recursively processes the tree so inner nodes are normalized before parent rendering.
func (g *CUEGenerator) liftChildConditions(node *fieldNode) {
	for _, name := range node.childOrder {
		child := node.children[name]
		if child == nil {
			continue
		}
		// Recurse first so deeper nodes are normalized
		g.liftChildConditions(child)

		if child.cond != nil {
			continue
		}
		if child.value != nil || child.isArray || len(child.spreads) > 0 || child.forEach != nil || child.patchKey != nil {
			continue
		}
		if len(child.children) == 0 {
			continue
		}

		var sharedCond Condition
		var sharedCondStr string
		canLift := true
		for _, grandName := range child.childOrder {
			grand := child.children[grandName]
			if grand == nil || grand.cond == nil {
				canLift = false
				break
			}
			// Don't lift if any grandchild has multiple conditional values,
			// since those nodes manage their own condition rendering.
			if len(grand.condValues) > 0 {
				canLift = false
				break
			}
			condStr := g.conditionToCUE(grand.cond)
			if sharedCondStr == "" {
				sharedCondStr = condStr
				sharedCond = grand.cond
			} else if sharedCondStr != condStr {
				canLift = false
				break
			}
		}
		if canLift && sharedCond != nil {
			child.cond = sharedCond
			for _, grandName := range child.childOrder {
				grand := child.children[grandName]
				if grand != nil {
					grand.cond = nil
				}
			}
		}
	}
}

// tryDecomposeOrLift attempts to decompose a struct node into per-condition
// blocks or lift a shared child condition to the parent. Returns true if the
// node was handled, false if normal rendering should proceed.
func (g *CUEGenerator) tryDecomposeOrLift(sb *strings.Builder, name string, node *fieldNode, indent string, depth int) bool {
	if node.value != nil || len(node.children) == 0 || len(node.spreads) > 0 || node.forEach != nil || node.patchKey != nil {
		return false
	}

	// Decompose a struct node into per-condition blocks when every child
	// subtree shares the same uniform set of leaf conditions.
	condGroups := g.canDecomposeByCondition(node)
	if condGroups != nil {
		condStrs := make([]string, 0, len(condGroups))
		for cs := range condGroups {
			condStrs = append(condStrs, cs)
		}
		sort.Strings(condStrs)

		for _, condStr := range condStrs {
			sb.WriteString(fmt.Sprintf("%sif %s {\n", indent, condStr))
			sb.WriteString(fmt.Sprintf("%s\t%s: {\n", indent, name))
			for _, childName := range node.childOrder {
				child := node.children[childName]
				filteredChild := g.filterNodeByCondition(child, condStr)
				if filteredChild != nil {
					g.writeFieldNode(sb, childName, filteredChild, depth+2)
				}
			}
			sb.WriteString(fmt.Sprintf("%s\t}\n", indent))
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
		}
		return true
	}

	// If all children share the same condition, lift it to avoid empty parent structs.
	condStr := ""
	canLift := true
	for _, childName := range node.childOrder {
		child := node.children[childName]
		if child.cond == nil {
			canLift = false
			break
		}
		childCondStr := g.conditionToCUE(child.cond)
		if condStr == "" {
			condStr = childCondStr
		} else if condStr != childCondStr {
			canLift = false
			break
		}
	}
	if canLift && condStr != "" {
		clone := &fieldNode{
			children:   make(map[string]*fieldNode, len(node.children)),
			childOrder: append([]string(nil), node.childOrder...),
		}
		for childName, child := range node.children {
			childCopy := *child
			childCopy.cond = nil
			clone.children[childName] = &childCopy
		}
		sb.WriteString(fmt.Sprintf("%sif %s {\n", indent, condStr))
		sb.WriteString(fmt.Sprintf("%s\t%s: {\n", indent, name))
		g.writeFieldTree(sb, clone, depth+2)
		sb.WriteString(fmt.Sprintf("%s\t}\n", indent))
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
		return true
	}

	return false
}

// writeFieldNode writes a single field node as CUE.
func (g *CUEGenerator) writeFieldNode(sb *strings.Builder, name string, node *fieldNode, depth int) {
	indent := strings.Repeat(g.indent, depth)

	// Handle bracket notation in name (like [app.oam.dev/name])
	if strings.HasPrefix(name, "[") && !strings.HasPrefix(name, "[0]") {
		// This is a map key access - extract the key
		key := strings.Trim(name, "[]")
		if node.value != nil {
			valStr := g.valueToCUE(node.value)
			sb.WriteString(fmt.Sprintf("%s%q: %s\n", indent, key, valStr))
		} else if len(node.children) > 0 {
			sb.WriteString(fmt.Sprintf("%s%q: {\n", indent, key))
			g.writeFieldTree(sb, node, depth+1)
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
		}
		return
	}

	// Handle array notation
	if node.isArray {
		sb.WriteString(fmt.Sprintf("%s%s: [{\n", indent, name))
		// Write the first array element (index 0)
		if child, exists := node.children["[0]"]; exists {
			g.writeFieldTree(sb, child, depth+1)
		}
		sb.WriteString(fmt.Sprintf("%s}]\n", indent))
		return
	}

	// Try to decompose or lift conditions for cleaner CUE output.
	if g.tryDecomposeOrLift(sb, name, node, indent, depth) {
		return
	}

	// Emit directive annotations before the field
	for _, directive := range node.directives {
		sb.WriteString(fmt.Sprintf("%s// +%s\n", indent, directive))
	}

	// Regular field
	if node.value != nil && len(node.children) == 0 {
		if len(node.condValues) > 0 {
			// Multiple conditional values at the same path — render each
			// inside its own if block.
			if node.cond != nil {
				condStr := g.conditionToCUE(node.cond)
				valStr := g.valueToCUE(node.value)
				sb.WriteString(fmt.Sprintf("%sif %s {\n", indent, condStr))
				sb.WriteString(fmt.Sprintf("%s\t%s: %s\n", indent, name, valStr))
				sb.WriteString(fmt.Sprintf("%s}\n", indent))
			} else {
				valStr := g.valueToCUE(node.value)
				sb.WriteString(fmt.Sprintf("%s%s: %s\n", indent, name, valStr))
			}
			for _, cv := range node.condValues {
				condStr := g.conditionToCUE(cv.cond)
				valStr := g.valueToCUE(cv.value)
				sb.WriteString(fmt.Sprintf("%sif %s {\n", indent, condStr))
				sb.WriteString(fmt.Sprintf("%s\t%s: %s\n", indent, name, valStr))
				sb.WriteString(fmt.Sprintf("%s}\n", indent))
			}
			return
		}
		// Leaf node with value
		valStr := g.valueToCUE(node.value)
		sb.WriteString(fmt.Sprintf("%s%s: %s\n", indent, name, valStr))
	} else if len(node.children) > 0 {
		// Node with children - write as nested struct
		sb.WriteString(fmt.Sprintf("%s%s: {\n", indent, name))
		g.writeFieldTree(sb, node, depth+1)
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	}
}

// canDecomposeByCondition checks if a struct node's children can be split into
// separate conditional blocks. This is possible when every child subtree has
// the same uniform set of leaf conditions (e.g., every leaf is guarded by either
// condition A or condition B). Returns a map of conditionString -> childNames,
// or nil if decomposition is not possible.
func (g *CUEGenerator) canDecomposeByCondition(node *fieldNode) map[string][]string {
	if node.value != nil || len(node.spreads) > 0 || node.forEach != nil || node.patchKey != nil {
		return nil
	}
	if len(node.children) == 0 {
		return nil
	}

	// Collect the set of leaf conditions from each child subtree
	childCondSets := make(map[string]map[string]bool)
	for _, childName := range node.childOrder {
		child := node.children[childName]
		condSet := make(map[string]bool)
		g.collectLeafConditions(child, condSet)
		if len(condSet) == 0 {
			return nil
		}
		// If any leaf is unconditional (empty string), can't decompose
		if condSet[""] {
			return nil
		}
		childCondSets[childName] = condSet
	}

	// All children must have the same condition set
	var referenceSet map[string]bool
	for _, condSet := range childCondSets {
		if referenceSet == nil {
			referenceSet = condSet
		} else {
			if len(condSet) != len(referenceSet) {
				return nil
			}
			for k := range referenceSet {
				if !condSet[k] {
					return nil
				}
			}
		}
	}

	// Need at least 2 different conditions to warrant decomposition
	if len(referenceSet) < 2 {
		return nil
	}

	result := make(map[string][]string)
	for condStr := range referenceSet {
		result[condStr] = append(result[condStr], node.childOrder...)
	}
	return result
}

// collectLeafConditions traverses a subtree and collects the CUE string
// representations of all leaf conditions. An unconditional leaf adds "".
func (g *CUEGenerator) collectLeafConditions(node *fieldNode, condSet map[string]bool) {
	if node.cond != nil && node.value != nil {
		condSet[g.conditionToCUE(node.cond)] = true
		// Also collect conditions from condValues (additional conditional
		// values at the same path).
		for _, cv := range node.condValues {
			condSet[g.conditionToCUE(cv.cond)] = true
		}
		return
	}
	if node.cond != nil && len(node.children) > 0 {
		// Intermediate node with a condition (already lifted)
		condSet[g.conditionToCUE(node.cond)] = true
		return
	}
	if node.value != nil && node.cond == nil {
		condSet[""] = true
		return
	}
	for _, childName := range node.childOrder {
		child := node.children[childName]
		g.collectLeafConditions(child, condSet)
	}
}

// filterNodeByCondition returns a copy of the subtree containing only leaves
// that match the given condition string. Intermediate nodes have their
// conditions cleared since the caller wraps the result in the condition block.
func (g *CUEGenerator) filterNodeByCondition(node *fieldNode, condStr string) *fieldNode {
	if node.cond != nil {
		if g.conditionToCUE(node.cond) == condStr {
			nodeCopy := *node
			nodeCopy.cond = nil
			nodeCopy.condValues = nil // Strip condValues; other conditions handled by their own block
			return &nodeCopy
		}
		// Check if the target condition is in condValues instead of the primary
		for _, cv := range node.condValues {
			if g.conditionToCUE(cv.cond) == condStr {
				nodeCopy := *node
				nodeCopy.value = cv.value
				nodeCopy.cond = nil
				nodeCopy.condValues = nil
				return &nodeCopy
			}
		}
		return nil
	}

	filtered := &fieldNode{
		children:   make(map[string]*fieldNode),
		childOrder: make([]string, 0),
		value:      node.value,
	}
	for _, childName := range node.childOrder {
		child := node.children[childName]
		filteredChild := g.filterNodeByCondition(child, condStr)
		if filteredChild != nil {
			filtered.children[childName] = filteredChild
			filtered.childOrder = append(filtered.childOrder, childName)
		}
	}
	if len(filtered.children) == 0 && filtered.value == nil {
		return nil
	}
	return filtered
}

// tryRenderBuilder checks if a Value implements CUERenderer or CUEConditionRenderer
// and renders it. Returns empty string if the value is not a builder.
func (g *CUEGenerator) tryRenderBuilder(v Value) string {
	if ccr, ok := v.(CUEConditionRenderer); ok {
		return ccr.RenderCUEWithCondition(g.valueToCUE, g.conditionToCUE)
	}
	if cr, ok := v.(CUERenderer); ok {
		return cr.RenderCUE(g.valueToCUE)
	}
	return ""
}

// valueToCUE converts a Value to CUE syntax.
func (g *CUEGenerator) valueToCUE(v Value) string {
	// Check if the value can render itself (used by fluent builders).
	if s := g.tryRenderBuilder(v); s != "" {
		return s
	}

	switch val := v.(type) {
	case *Literal:
		return formatCUEValue(val.Val())
	case *ContextRef:
		return val.Path()
	case *ContextOutputRef:
		return val.Path()
	case *Ref:
		return val.Path()
	case *LocalFieldRef:
		// Local field reference — emits the bare field name (no parameter. prefix)
		return val.Name()
	case *TimeParseExpr:
		// time.Parse(layout, field) call
		return fmt.Sprintf(`time.Parse(%q, %s)`, val.Layout(), val.FieldName())
	case *HelperVar:
		// Return reference to the helper by name
		return val.Name()
	case *StringParam, *IntParam, *BoolParam, *FloatParam, *ArrayParam, *MapParam, *StringKeyMapParam, *EnumParam, *OneOfParam:
		return "parameter." + v.(Param).Name()
	case *DynamicMapParam:
		// Dynamic map parameters reference just "parameter"
		return "parameter"
	case *ParamPathRef:
		// Parameter path reference like "podAffinity.required" => "parameter.podAffinity.required"
		return "parameter." + val.Path()
	case *CollectionOp:
		return g.collectionOpToCUE(val)
	case *MultiSource:
		return g.multiSourceToCUE(val)
	case *InlineArrayValue:
		return g.inlineArrayToCUE(val)
	case *ConcatExprValue:
		return g.concatExprToCUE(val)
	case *CUEFunc:
		return g.cueFuncToCUE(val)
	case *ArrayElement:
		return g.arrayElementToCUE(val)
	case *StructArrayHelper:
		// Return reference to the helper by name
		return val.HelperName()
	case *ConcatHelper:
		// Return reference to the helper by name
		return val.HelperName()
	case *DedupeHelper:
		// Return reference to the helper by name
		return val.HelperName()
	case *LetRef:
		// Return reference to a let binding variable
		return val.Name()
	case *ArrayBuilder:
		return g.arrayBuilderToCUE(val, 1)
	case *ArrayConcatValue:
		return g.valueToCUE(val.Left()) + " + " + g.valueToCUE(val.Right())
	case *ListComprehension:
		// Return list comprehension CUE
		return g.listComprehensionToCUE(val)
	case *ParamArithExpr:
		// Arithmetic expression on a parameter: parameter.name op value
		return fmt.Sprintf("parameter.%s %s %s", val.ParamName(), val.Op(), formatCUEValue(val.ArithValue()))
	case *ParamConcatExpr:
		// String concatenation on a parameter
		if val.Prefix() != "" {
			return fmt.Sprintf("%s + parameter.%s", formatCUEValue(val.Prefix()), val.ParamName())
		}
		return fmt.Sprintf("parameter.%s + %s", val.ParamName(), formatCUEValue(val.Suffix()))
	case *ParamFieldRef:
		// Reference to a field within a struct parameter: parameter.name.field.path
		return fmt.Sprintf("parameter.%s.%s", val.ParamName(), val.FieldPath())
	case *InterpolatedString:
		return g.interpolatedStringToCUE(val)
	case *PlusExpr:
		parts := make([]string, len(val.Parts()))
		for i, p := range val.Parts() {
			parts[i] = g.valueToCUE(p)
		}
		return strings.Join(parts, " + ")
	case *IterVarRef:
		return val.VarName()
	case *IterFieldRef:
		return fmt.Sprintf("%s.%s", val.VarName(), val.FieldName())
	case *IterLetRef:
		return val.RefName()
	case *ForEachMapOp:
		return g.forEachMapOpToCUE(val)
	default:
		// Try to get name from Param interface
		if p, ok := v.(Param); ok {
			return "parameter." + p.Name()
		}
		return "_"
	}
}

// forEachMapOpToCUE converts a ForEachMapOp to CUE map comprehension syntax.
// Generates: {for k, v in source { (keyExpr): valExpr }}.
func (g *CUEGenerator) forEachMapOpToCUE(op *ForEachMapOp) string {
	keyVar := op.KeyVar()
	if keyVar == "" {
		keyVar = "k"
	}

	valVar := op.ValVar()
	if valVar == "" {
		valVar = "v"
	}

	keyExpr := op.KeyExpr()
	if keyExpr == "" {
		keyExpr = keyVar
	}

	valExpr := op.ValExpr()
	if valExpr == "" {
		valExpr = valVar
	}

	return fmt.Sprintf("{for %s, %s in %s { (%s): %s }}", keyVar, valVar, op.Source(), keyExpr, valExpr)
}

// cueFuncToCUE converts a CUE function call to CUE syntax.
func (g *CUEGenerator) cueFuncToCUE(fn *CUEFunc) string {
	args := make([]string, len(fn.Args()))
	for i, arg := range fn.Args() {
		args[i] = g.valueToCUE(arg)
	}
	return fmt.Sprintf("%s.%s(%s)", fn.Package(), fn.Function(), strings.Join(args, ", "))
}

// interpolatedStringToCUE converts an InterpolatedString to CUE string interpolation.
// Literal string values are inlined directly. All other values are wrapped in \(...).
// Example: Interpolation(vela.Namespace(), Lit(":"), name) → "\(context.namespace):\(parameter.name)"
func (g *CUEGenerator) interpolatedStringToCUE(is *InterpolatedString) string {
	var sb strings.Builder
	sb.WriteString(`"`)
	for _, part := range is.Parts() {
		if lit, ok := part.(*Literal); ok {
			if s, ok := lit.Val().(string); ok {
				sb.WriteString(s)
				continue
			}
		}
		sb.WriteString(`\(`)
		sb.WriteString(g.valueToCUE(part))
		sb.WriteString(`)`)
	}
	sb.WriteString(`"`)
	return sb.String()
}

// valueToCUEAtDepth converts a Value to CUE syntax with depth-aware indentation.
// For types that support depth (ArrayBuilder, ArrayConcatValue), it uses the given depth.
// For all other types, it falls back to the standard valueToCUE.
func (g *CUEGenerator) valueToCUEAtDepth(v Value, depth int) string {
	switch val := v.(type) {
	case *ArrayBuilder:
		return g.arrayBuilderToCUE(val, depth)
	case *ArrayConcatValue:
		return g.valueToCUEAtDepth(val.Left(), depth) + " + " + g.valueToCUE(val.Right())
	default:
		return g.valueToCUE(v)
	}
}

// arrayElementToCUE converts an ArrayElement to CUE syntax.
// Uses default depth of 1 for backwards compatibility.
func (g *CUEGenerator) arrayElementToCUE(elem *ArrayElement) string {
	return g.arrayElementToCUEWithDepth(elem, 1)
}

// arrayElementToCUEWithDepth converts an ArrayElement to CUE syntax with proper indentation.
// The depth parameter indicates the nesting level for proper indentation.
func (g *CUEGenerator) arrayElementToCUEWithDepth(elem *ArrayElement, depth int) string {
	var sb strings.Builder
	indent := strings.Repeat(g.indent, depth)
	innerIndent := strings.Repeat(g.indent, depth+1)

	sb.WriteString("{\n")
	for _, key := range sortedKeys(elem.Fields()) {
		val := elem.Fields()[key]
		valStr := indentMultilineValue(g.valueToCUE(val), innerIndent)
		sb.WriteString(fmt.Sprintf("%s%s: %s\n", innerIndent, key, valStr))
	}
	// Write conditional operations
	for _, op := range elem.Ops() {
		if setIf, ok := op.(*SetIfOp); ok {
			condStr := g.conditionToCUE(setIf.Cond())
			valStr := indentMultilineValue(g.valueToCUE(setIf.Value()), innerIndent+"\t")
			// Convert dot-separated path to CUE shorthand syntax: "a.b.c" -> "a: b: c"
			cuePath := strings.ReplaceAll(setIf.Path(), ".", ": ")
			sb.WriteString(fmt.Sprintf("%sif %s {\n", innerIndent, condStr))
			sb.WriteString(fmt.Sprintf("%s\t%s: %s\n", innerIndent, cuePath, valStr))
			sb.WriteString(fmt.Sprintf("%s}\n", innerIndent))
		}
	}
	// Write patchKey-annotated fields (nested patchKey inside array elements)
	for _, pkf := range elem.PatchKeyFields() {
		sb.WriteString(fmt.Sprintf("%s// +patchKey=%s\n", innerIndent, pkf.key))
		valStr := indentMultilineValue(g.valueToCUEAtDepth(pkf.value, depth+1), innerIndent)
		sb.WriteString(fmt.Sprintf("%s%s: %s\n", innerIndent, pkf.field, valStr))
	}
	sb.WriteString(fmt.Sprintf("%s}", indent))
	return sb.String()
}

// indentMultilineValue prepends indent to every line of s after the first.
// This is needed when embedding a multi-line valueToCUE result into an
// already-indented context: the first line sits on the same line as the
// key, but subsequent lines need the surrounding indentation added.
func indentMultilineValue(s, indent string) string {
	if !strings.Contains(s, "\n") {
		return s
	}
	lines := strings.Split(s, "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i] != "" {
			lines[i] = indent + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

// arrayBuilderToCUE converts an ArrayBuilder to CUE syntax.
// Generates: [{static}, if cond {{conditional}}, if guard for m in source {iterated}]
func (g *CUEGenerator) arrayBuilderToCUE(ab *ArrayBuilder, depth int) string {
	var sb strings.Builder
	indent := strings.Repeat(g.indent, depth)
	innerIndent := strings.Repeat(g.indent, depth+1)
	deepIndent := strings.Repeat(g.indent, depth+2)

	sb.WriteString("[\n")

	for _, entry := range ab.Entries() {
		switch entry.kind {
		case entryStatic:
			sb.WriteString(innerIndent)
			sb.WriteString(g.arrayElementToCUEWithDepth(entry.element, depth+1))
			sb.WriteString(",\n")

		case entryConditional:
			condStr := g.conditionToCUE(entry.cond)
			sb.WriteString(fmt.Sprintf("%sif %s {\n", innerIndent, condStr))
			sb.WriteString(deepIndent)
			sb.WriteString(g.arrayElementToCUEWithDepth(entry.element, depth+2))
			sb.WriteString("\n")
			sb.WriteString(fmt.Sprintf("%s},\n", innerIndent))

		case entryForEach:
			sourceStr := g.valueToCUE(entry.source)
			if entry.guard != nil {
				guardStr := g.conditionToCUE(entry.guard)
				sb.WriteString(fmt.Sprintf("%sif %s for m in %s {\n", innerIndent, guardStr, sourceStr))
			} else {
				sb.WriteString(fmt.Sprintf("%sfor m in %s {\n", innerIndent, sourceStr))
			}
			// Wrap each element in inner braces for explicit struct boundary
			extraIndent := deepIndent + "\t"
			sb.WriteString(fmt.Sprintf("%s{\n", deepIndent))
			// Write each field from the element template
			for _, key := range sortedKeys(entry.element.Fields()) {
				val := entry.element.Fields()[key]
				valStr := g.valueToCUE(val)
				sb.WriteString(fmt.Sprintf("%s%s: %s\n", extraIndent, key, valStr))
			}
			// Write conditional operations
			for _, op := range entry.element.Ops() {
				if setIf, ok := op.(*SetIfOp); ok {
					condStr := g.conditionToCUE(setIf.Cond())
					valStr := g.valueToCUE(setIf.Value())
					cuePath := strings.ReplaceAll(setIf.Path(), ".", ": ")
					sb.WriteString(fmt.Sprintf("%sif %s {\n", extraIndent, condStr))
					sb.WriteString(fmt.Sprintf("%s\t%s: %s\n", extraIndent, cuePath, valStr))
					sb.WriteString(fmt.Sprintf("%s}\n", extraIndent))
				}
			}
			sb.WriteString(fmt.Sprintf("%s}\n", deepIndent))
			sb.WriteString(fmt.Sprintf("%s},\n", innerIndent))

		case entryForEachWith:
			sourceStr := g.valueToCUE(entry.source)
			guardPrefix := ""
			if entry.guard != nil {
				guardPrefix = "if " + g.conditionToCUE(entry.guard) + " "
			}
			filterSuffix := ""
			if entry.filter != nil {
				filterSuffix = " if " + g.predicateToCUE(entry.filter)
			}
			sb.WriteString(fmt.Sprintf("%s%sfor %s in %s%s {\n", innerIndent, guardPrefix, entry.itemBuilder.VarName(), sourceStr, filterSuffix))
			g.writeItemBuilderOps(&sb, entry.itemBuilder.Ops(), depth+2)
			sb.WriteString(fmt.Sprintf("%s},\n", innerIndent))
		}
	}

	sb.WriteString(fmt.Sprintf("%s]", indent))
	return sb.String()
}

// writeItemBuilderOps writes the CUE for ItemBuilder operations.
func (g *CUEGenerator) writeItemBuilderOps(sb *strings.Builder, ops []itemOp, depth int) {
	indent := strings.Repeat(g.indent, depth)

	for _, op := range ops {
		switch o := op.(type) {
		case setOp:
			valStr := g.valueToCUE(o.value)
			sb.WriteString(fmt.Sprintf("%s%s: %s\n", indent, o.field, valStr))

		case ifBlockOp:
			condStr := g.conditionToCUE(o.cond)
			sb.WriteString(fmt.Sprintf("%sif %s {\n", indent, condStr))
			g.writeItemBuilderOps(sb, o.body, depth+1)
			sb.WriteString(fmt.Sprintf("%s}\n", indent))

		case letOp:
			valStr := g.valueToCUE(o.value)
			sb.WriteString(fmt.Sprintf("%s%s: %s\n", indent, o.name, valStr))

		case setDefaultOp:
			defStr := g.valueToCUE(o.defValue)
			sb.WriteString(fmt.Sprintf("%s%s: *%s | %s\n", indent, o.field, defStr, o.typeName))
		}
	}
}

// collectionOpToCUE generates CUE for a collection operation.
func (g *CUEGenerator) collectionOpToCUE(col *CollectionOp) string {
	sourceStr := g.valueToCUE(col.Source())
	ops := col.Operations()

	// Check for wrap operation (e.g., imagePullSecrets wrapping string to {name: string})
	for _, op := range ops {
		if wOp, ok := op.(*wrapOp); ok {
			return fmt.Sprintf("[for v in %s { %s: v }]", sourceStr, wOp.key)
		}
	}

	// Check for deduplication
	var dedupeKeyField string
	for _, op := range ops {
		if dedup, ok := op.(*dedupeOp); ok {
			dedupeKeyField = dedup.keyField
			break
		}
	}

	// Build filter condition if present; AND-compose multiple filters
	filterCondition := ""
	for _, op := range ops {
		var cond string
		if fOp, ok := op.(*filterOp); ok {
			cond = g.predicateToCUE(fOp.pred)
		} else if fOp, ok := op.(*filterCondCollectionOp); ok {
			cond = g.conditionToCUE(fOp.Cond())
		}
		if cond != "" {
			if filterCondition == "" {
				filterCondition = cond
			} else {
				filterCondition = filterCondition + " && " + cond
			}
		}
	}

	// Check if there's a Map or MapVariant operation
	hasMap := false
	hasVariant := false
	for _, op := range ops {
		if _, ok := op.(*mapOp); ok {
			hasMap = true
		}
		if _, ok := op.(*mapVariantOp); ok {
			hasVariant = true
		}
	}

	// Dedupe: render the nested-comprehension pattern.
	// This is placed after all op detection so that guard/filter/map are not bypassed.
	if dedupeKeyField != "" {
		var sb strings.Builder
		// Apply guard if present
		if guard := col.GetGuard(); guard != nil {
			guardStr := g.conditionToCUE(guard)
			sb.WriteString(fmt.Sprintf("if %s ", guardStr))
		}
		sb.WriteString(fmt.Sprintf(`[
		for val in [
			for i, vi in %s {
				for j, vj in %s if j < i && vi.%s == vj.%s {
					_ignore: true
				}
				vi
			},
		] if val._ignore == _|_ {
			val
		},
	]`, sourceStr, sourceStr, dedupeKeyField, dedupeKeyField))
		return sb.String()
	}

	// Build the list comprehension
	var sb strings.Builder
	sb.WriteString("[")

	// Add guard condition if present (wraps entire comprehension)
	if guard := col.GetGuard(); guard != nil {
		guardStr := g.conditionToCUE(guard)
		sb.WriteString("if ")
		sb.WriteString(guardStr)
		sb.WriteString(" ")
	}

	sb.WriteString("for v in ")
	sb.WriteString(sourceStr)
	if filterCondition != "" {
		sb.WriteString(" if ")
		sb.WriteString(filterCondition)
	}

	if hasMap || hasVariant {
		// Map operations: render mapped fields in a struct
		sb.WriteString(" {\n")
		sb.WriteString("\t\t\t\t{\n")
		for _, op := range ops {
			if mOp, ok := op.(*mapOp); ok {
				for _, fieldName := range sortedKeys(mOp.mappings) {
					fieldVal := mOp.mappings[fieldName]
					if optField, isOptional := fieldVal.(*OptionalField); isOptional {
						sb.WriteString(fmt.Sprintf("\t\t\t\t\tif v.%s != _|_ {\n", optField.field))
						sb.WriteString(fmt.Sprintf("\t\t\t\t\t\t%s: v.%s\n", fieldName, optField.field))
						sb.WriteString("\t\t\t\t\t}\n")
					} else if compOpt, isCompound := fieldVal.(*CompoundOptionalField); isCompound {
						condStr := g.conditionToCUE(compOpt.additionalCond)
						sb.WriteString(fmt.Sprintf("\t\t\t\t\tif v.%s != _|_ if %s {\n", compOpt.field, condStr))
						sb.WriteString(fmt.Sprintf("\t\t\t\t\t\t%s: v.%s\n", fieldName, compOpt.field))
						sb.WriteString("\t\t\t\t\t}\n")
					} else if condRef, isConditional := fieldVal.(*ConditionalOrFieldRef); isConditional {
						// Emit if/else pattern for conditional field reference
						primaryField := string(condRef.primary)
						fallbackStr := g.fieldValueToCUE(condRef.fallback)
						sb.WriteString(fmt.Sprintf("\t\t\t\t\tif v.%s != _|_ {\n", primaryField))
						sb.WriteString(fmt.Sprintf("\t\t\t\t\t\t%s: v.%s\n", fieldName, primaryField))
						sb.WriteString("\t\t\t\t\t}\n")
						sb.WriteString(fmt.Sprintf("\t\t\t\t\tif v.%s == _|_ {\n", primaryField))
						sb.WriteString(fmt.Sprintf("\t\t\t\t\t\t%s: %s\n", fieldName, fallbackStr))
						sb.WriteString("\t\t\t\t\t}\n")
					} else {
						valStr := g.fieldValueToCUE(fieldVal)
						sb.WriteString(fmt.Sprintf("\t\t\t\t\t%s: %s\n", fieldName, valStr))
					}
				}
			}
		}
		// MapVariant operations: render conditional field blocks
		for _, op := range ops {
			if mvOp, ok := op.(*mapVariantOp); ok {
				sb.WriteString(fmt.Sprintf("\t\t\t\t\tif v.%s == %q {\n", mvOp.discriminator, mvOp.variantName))
				for _, fieldName := range sortedKeys(mvOp.mappings) {
					fieldVal := mvOp.mappings[fieldName]
					if optField, isOptional := fieldVal.(*OptionalField); isOptional {
						sb.WriteString(fmt.Sprintf("\t\t\t\t\t\tif v.%s != _|_ {\n", optField.field))
						sb.WriteString(fmt.Sprintf("\t\t\t\t\t\t\t%s: v.%s\n", fieldName, optField.field))
						sb.WriteString("\t\t\t\t\t\t}\n")
					} else {
						valStr := g.fieldValueToCUE(fieldVal)
						sb.WriteString(fmt.Sprintf("\t\t\t\t\t\t%s: %s\n", fieldName, valStr))
					}
				}
				sb.WriteString("\t\t\t\t\t}\n")
			}
		}
		sb.WriteString("\t\t\t\t}\n")
		sb.WriteString("\t\t\t}]")
	} else {
		// Filter-only: pass through the iteration variable
		sb.WriteString(" {v}]")
	}

	return sb.String()
}

// predicateToCUE converts a Predicate to CUE filter condition.
func (g *CUEGenerator) predicateToCUE(pred Predicate) string {
	switch p := pred.(type) {
	case FieldEq:
		// Generate: v.field == value
		return fmt.Sprintf("v.%s == %s", p.field, formatCUEValue(p.value))
	case FieldIsSet:
		// Generate: v.field != _|_
		return fmt.Sprintf("v.%s != _|_", p.field)
	default:
		return cueBoolTrue
	}
}

// listComprehensionToCUE generates CUE for a ListComprehension.
// This generates: [for v in source { ... if v.field != _|_ { field: v.field } }]
func (g *CUEGenerator) listComprehensionToCUE(lc *ListComprehension) string {
	sourceStr := g.valueToCUE(lc.Source())
	mappings := lc.Mappings()
	conditionalFields := lc.ConditionalFields()

	var sb strings.Builder
	sb.WriteString("[\n")
	sb.WriteString("\t\tfor v in ")
	sb.WriteString(sourceStr)
	sb.WriteString(" {\n")

	// Write direct field mappings
	for _, fieldName := range sortedKeys(mappings) {
		fieldVal := mappings[fieldName]
		// Check if this field is conditional
		isConditional := false
		for _, cf := range conditionalFields {
			if cf == fieldName {
				isConditional = true
				break
			}
		}

		// Check if the field value is an OptionalField
		if optField, ok := fieldVal.(*OptionalField); ok {
			// Generate conditional: if v.field != _|_ { field: v.field }
			sb.WriteString(fmt.Sprintf("\t\t\tif v.%s != _|_ {\n", optField.field))
			sb.WriteString(fmt.Sprintf("\t\t\t\t%s: v.%s\n", fieldName, optField.field))
			sb.WriteString("\t\t\t}\n")
		} else if isConditional {
			// Field is in conditionalFields list
			sourceField := fieldName
			if fr, ok := fieldVal.(FieldRef); ok {
				sourceField = string(fr)
			}
			sb.WriteString(fmt.Sprintf("\t\t\tif v.%s != _|_ {\n", sourceField))
			sb.WriteString(fmt.Sprintf("\t\t\t\t%s: v.%s\n", fieldName, sourceField))
			sb.WriteString("\t\t\t}\n")
		} else {
			// Direct field reference
			sourceField := fieldName
			if fr, ok := fieldVal.(FieldRef); ok {
				sourceField = string(fr)
			}
			sb.WriteString(fmt.Sprintf("\t\t\t%s: v.%s\n", fieldName, sourceField))
		}
	}

	sb.WriteString("\t\t}\n")
	sb.WriteString("\t]")

	return sb.String()
}

// listPredicateToCUE converts a ListPredicate to CUE filter condition.
//
//lint:ignore U1000 planned for future use
func (g *CUEGenerator) listPredicateToCUE(pred ListPredicate) string {
	switch p := pred.(type) {
	case *ListFieldExistsPredicate:
		return fmt.Sprintf("v.%s != _|_", p.GetField())
	default:
		return cueBoolTrue
	}
}

// multiSourceToCUE generates CUE for a multi-source collection.
// This generates the complex volumeMounts-style comprehension pattern.
func (g *CUEGenerator) multiSourceToCUE(ms *MultiSource) string {
	sourceStr := g.valueToCUE(ms.Source())
	sources := ms.Sources()
	mapBySource := ms.MapBySourceMappings()

	// Extract filter condition (AND-compose multiple filters)
	filterCondition := ""
	var pickFields []string
	for _, op := range ms.Operations() {
		var cond string
		if fOp, ok := op.(*filterOp); ok {
			cond = g.predicateToCUE(fOp.pred)
		} else if fOp, ok := op.(*filterCondCollectionOp); ok {
			cond = g.conditionToCUE(fOp.Cond())
		} else if pOp, ok := op.(*pickOp); ok {
			pickFields = pOp.fields
		}
		if cond != "" {
			if filterCondition == "" {
				filterCondition = cond
			} else {
				filterCondition = filterCondition + " && " + cond
			}
		}
	}

	// Check if we have Pick operations (for volumeMounts -> container mounts)
	if len(pickFields) > 0 {
		return g.generatePickMultiSource(sourceStr, sources, pickFields, filterCondition)
	}

	// Check if we have MapBySource (for volumeMounts -> pod volumes)
	if len(mapBySource) > 0 {
		return g.generateMapBySourceCUE(sourceStr, sources, mapBySource)
	}

	// Fallback: simple flattening
	var sb strings.Builder
	sb.WriteString("[\n")
	for i, source := range sources {
		if i > 0 {
			sb.WriteString(",\n")
		}
		if filterCondition != "" {
			sb.WriteString(fmt.Sprintf("\t\t\t\t\tif %s.%s != _|_ for v in %s.%s if %s { v }", sourceStr, source, sourceStr, source, filterCondition))
		} else {
			sb.WriteString(fmt.Sprintf("\t\t\t\t\tif %s.%s != _|_ for v in %s.%s { v }", sourceStr, source, sourceStr, source))
		}
	}
	sb.WriteString("\n\t\t\t\t]")
	return sb.String()
}

// generatePickMultiSource generates CUE for picking fields from multiple sources.
func (g *CUEGenerator) generatePickMultiSource(sourceStr string, sources []string, fields []string, filterCondition string) string {
	var sb strings.Builder
	sb.WriteString("[\n")

	for i, source := range sources {
		if i > 0 {
			sb.WriteString(",\n")
		}
		if filterCondition != "" {
			sb.WriteString(fmt.Sprintf("\t\t\t\t\tif %s != _|_ && %s.%s != _|_ for v in %s.%s if %s {\n",
				sourceStr, sourceStr, source, sourceStr, source, filterCondition))
		} else {
			sb.WriteString(fmt.Sprintf("\t\t\t\t\tif %s != _|_ && %s.%s != _|_ for v in %s.%s {\n",
				sourceStr, sourceStr, source, sourceStr, source))
		}
		sb.WriteString("\t\t\t\t\t\t{\n")

		for _, field := range fields {
			// For optional fields like subPath, wrap in conditional
			if field == "subPath" {
				sb.WriteString(fmt.Sprintf("\t\t\t\t\t\t\tif v.%s != _|_ { %s: v.%s }\n", field, field, field))
			} else {
				sb.WriteString(fmt.Sprintf("\t\t\t\t\t\t\t%s: v.%s\n", field, field))
			}
		}

		sb.WriteString("\t\t\t\t\t\t}\n")
		sb.WriteString("\t\t\t\t\t}")
	}

	sb.WriteString("\n\t\t\t\t]")
	return sb.String()
}

// generateMapBySourceCUE generates CUE for mapping different sources differently.
func (g *CUEGenerator) generateMapBySourceCUE(sourceStr string, sources []string, mapBySource map[string]FieldMap) string {
	var sb strings.Builder
	sb.WriteString("[\n")

	for i, source := range sources {
		if i > 0 {
			sb.WriteString(",\n")
		}

		mapping, hasMapping := mapBySource[source]
		sb.WriteString(fmt.Sprintf("\t\t\t\t\tif %s != _|_ && %s.%s != _|_ for v in %s.%s {\n",
			sourceStr, sourceStr, source, sourceStr, source))
		sb.WriteString("\t\t\t\t\t\t{\n")

		if hasMapping {
			for _, fieldName := range sortedKeys(mapping) {
				fieldVal := mapping[fieldName]
				if nf, isNested := fieldVal.(*NestedField); isNested {
					// Inline nested field expansion with correct indentation
					sb.WriteString(fmt.Sprintf("\t\t\t\t\t\t\t%s: {\n", fieldName))
					for _, nestedName := range sortedKeys(nf.mapping) {
						nestedVal := nf.mapping[nestedName]
						if optField, isOptional := nestedVal.(*OptionalField); isOptional {
							sb.WriteString(fmt.Sprintf("\t\t\t\t\t\t\t\tif v.%s != _|_ {\n", optField.field))
							sb.WriteString(fmt.Sprintf("\t\t\t\t\t\t\t\t\t%s: v.%s\n", nestedName, optField.field))
							sb.WriteString("\t\t\t\t\t\t\t\t}\n")
						} else {
							valStr := g.fieldValueToCUE(nestedVal)
							sb.WriteString(fmt.Sprintf("\t\t\t\t\t\t\t\t%s: %s\n", nestedName, valStr))
						}
					}
					sb.WriteString("\t\t\t\t\t\t\t}\n")
				} else {
					valStr := g.fieldValueToCUE(fieldVal)
					sb.WriteString(fmt.Sprintf("\t\t\t\t\t\t\t%s: %s\n", fieldName, valStr))
				}
			}
		} else {
			sb.WriteString("\t\t\t\t\t\t\tv\n")
		}

		sb.WriteString("\t\t\t\t\t\t}\n")
		sb.WriteString("\t\t\t\t\t}")
	}

	sb.WriteString("\n\t\t\t\t]")
	return sb.String()
}

// fieldValueToCUE converts a FieldValue to CUE syntax.
func (g *CUEGenerator) fieldValueToCUE(fv FieldValue) string {
	switch val := fv.(type) {
	case FieldRef:
		return "v." + string(val)
	case *OrFieldRef:
		primary := "v." + string(val.primary)
		fallback := g.fieldValueToCUE(val.fallback)
		return fmt.Sprintf("*%s | %s", primary, fallback)
	case LitVal:
		return formatCUEValue(val.val)
	case *FormatField:
		// Generate strconv/strings expression
		return g.formatFieldToCUE(val)
	case *NestedField:
		return g.nestedFieldToCUE(val)
	default:
		return "_"
	}
}

// formatFieldToCUE converts a FormatField to CUE syntax.
func (g *CUEGenerator) formatFieldToCUE(ff *FormatField) string {
	// Simple case: "port-%v" with one arg
	if ff.format == "port-%v" && len(ff.args) == 1 {
		arg := g.fieldValueToCUE(ff.args[0])
		return fmt.Sprintf(`"port-" + strconv.FormatInt(%s, 10)`, arg)
	}
	return fmt.Sprintf("%q", ff.format)
}

// nestedFieldToCUE converts a NestedField to CUE syntax.
func (g *CUEGenerator) nestedFieldToCUE(nf *NestedField) string {
	var sb strings.Builder
	sb.WriteString("{\n")
	for _, fieldName := range sortedKeys(nf.mapping) {
		fieldVal := nf.mapping[fieldName]
		// Handle optional fields specially - generate conditional inclusion
		if optField, isOptional := fieldVal.(*OptionalField); isOptional {
			sb.WriteString(fmt.Sprintf("\t\t\t\tif v.%s != _|_ {\n", optField.field))
			sb.WriteString(fmt.Sprintf("\t\t\t\t\t%s: v.%s\n", fieldName, optField.field))
			sb.WriteString("\t\t\t\t}\n")
		} else {
			valStr := g.fieldValueToCUE(fieldVal)
			sb.WriteString(fmt.Sprintf("\t\t\t\t%s: %s\n", fieldName, valStr))
		}
	}
	sb.WriteString("\t\t\t}")
	return sb.String()
}

// concatExprToCUE converts a ConcatExprValue to CUE syntax.
// This generates: mountsArray.pvc + mountsArray.configMap + mountsArray.secret + ...
func (g *CUEGenerator) concatExprToCUE(ce *ConcatExprValue) string {
	sourceName := ce.Source().HelperName()
	fields := ce.Fields()

	if len(fields) == 0 {
		return "[]"
	}

	// Build the concatenation expression: mountsArray.pvc + mountsArray.configMap + ...
	var parts []string
	for _, field := range fields {
		parts = append(parts, fmt.Sprintf("%s.%s", sourceName, field))
	}

	return strings.Join(parts, " + ")
}

// inlineArrayToCUE converts an InlineArrayValue to CUE syntax.
// Generates: [{field1: value1, field2: value2}]
func (g *CUEGenerator) inlineArrayToCUE(arr *InlineArrayValue) string {
	var sb strings.Builder
	sb.WriteString("[{\n")
	for _, fieldName := range sortedKeys(arr.Fields()) {
		fieldVal := arr.Fields()[fieldName]
		valStr := g.valueToCUE(fieldVal)
		sb.WriteString(fmt.Sprintf("\t\t\t\t\t\t\t%s: %s\n", fieldName, valStr))
	}
	sb.WriteString("\t\t\t\t\t\t}]")
	return sb.String()
}

// conditionToCUE converts a Condition to CUE syntax.
func (g *CUEGenerator) conditionToCUE(cond Condition) string {
	switch c := cond.(type) {
	case *IsSetCondition:
		return fmt.Sprintf("parameter[%q] != _|_", c.ParamName())
	case *ParamPathIsSetCondition:
		// Check if a nested parameter path is set: parameter.path != _|_
		return fmt.Sprintf("parameter.%s != _|_", c.Path())
	case *TruthyCondition:
		return fmt.Sprintf("parameter.%s", c.ParamName())
	case *FalsyCondition:
		return fmt.Sprintf("!parameter.%s", c.ParamName())
	case *InCondition:
		return g.inConditionToCUE(c)
	case *StringContainsCondition:
		return fmt.Sprintf(`strings.Contains(parameter.%s, %q)`, c.ParamName(), c.Substr())
	case *StringStartsWithCondition:
		return fmt.Sprintf(`strings.HasPrefix(parameter.%s, %q)`, c.ParamName(), c.Prefix())
	case *StringEndsWithCondition:
		return fmt.Sprintf(`strings.HasSuffix(parameter.%s, %q)`, c.ParamName(), c.Suffix())
	case *LenCondition:
		return fmt.Sprintf("len(parameter.%s) %s %d", c.ParamName(), c.Op(), c.Length())
	case *ArrayContainsCondition:
		return fmt.Sprintf("list.Contains(parameter.%s, %s)", c.ParamName(), formatCUEValue(c.Value()))
	case *MapHasKeyCondition:
		return fmt.Sprintf("parameter.%s.%s != _|_", c.ParamName(), c.Key())
	case *ParamCompareCondition:
		// Parameter comparison: parameter.name op value
		return fmt.Sprintf("parameter.%s %s %s", c.ParamName(), c.Op(), formatCUEValue(c.CompareValue()))
	case *Comparison:
		left := g.exprToCUE(c.Left())
		right := g.exprToCUE(c.Right())
		return fmt.Sprintf("%s %s %s", left, c.Op(), right)
	case *AndCondition:
		left := g.conditionToCUE(c.left)
		right := g.conditionToCUE(c.right)
		return fmt.Sprintf("(%s) && (%s)", left, right)
	case *LogicalExpr:
		parts := make([]string, len(c.Conditions()))
		for i, sub := range c.Conditions() {
			parts[i] = g.conditionToCUE(sub)
		}
		op := " && "
		if c.Op() == OpOr {
			op = " || "
		}
		return strings.Join(parts, op)
	case *NotExpr:
		// Special case: Not(IsSet("x")) -> parameter["x"] == _|_
		if isSet, ok := c.Cond().(*IsSetCondition); ok {
			return fmt.Sprintf("parameter[%q] == _|_", isSet.ParamName())
		}
		// Special case: Not(PathExists("x")) -> x == _|_
		if pe, ok := c.Cond().(*PathExistsCondition); ok {
			return fmt.Sprintf("%s == _|_", pe.Path())
		}
		return fmt.Sprintf("!(%s)", g.conditionToCUE(c.Cond()))
	case *HasExposedPortsCondition:
		// Check if any port has expose=true
		portsStr := g.valueToCUE(c.Ports())
		return fmt.Sprintf("len([for p in %s if p.expose == true { p }]) > 0", portsStr)
	case *LenNotZeroCondition:
		// Check if len(source) != 0
		sourceStr := g.valueToCUE(c.Source())
		return fmt.Sprintf("len(%s) != 0", sourceStr)
	case *LenValueCondition:
		// Check len(source) op n for arbitrary values
		sourceStr := g.valueToCUE(c.Source())
		return fmt.Sprintf("len(%s) %s %d", sourceStr, c.Op(), c.Length())
	case *IterFieldExistsCondition:
		// Check if iteration variable field exists/not exists
		if c.IsNegated() {
			return fmt.Sprintf("%s.%s == _|_", c.VarName(), c.FieldName())
		}
		return fmt.Sprintf("%s.%s != _|_", c.VarName(), c.FieldName())
	case *PathExistsCondition:
		// Check if a path exists: path != _|_
		return fmt.Sprintf("%s != _|_", c.Path())
	case *ContextPathExistsCondition:
		// Check if a context output path exists
		return fmt.Sprintf("%s != _|_", c.FullPath())
	case *ContextOutputExistsCondition:
		// Check if a context.output path exists
		return fmt.Sprintf("context.output.%s != _|_", c.Path())
	case *AllConditionsCondition:
		// Generate compound condition: if cond1 if cond2 ...
		var parts []string
		for _, cond := range c.Conditions() {
			parts = append(parts, g.conditionToCUE(cond))
		}
		// For CUE, we generate: cond1 && cond2 && cond3
		// which will be used in a single if statement
		return strings.Join(parts, " && ")
	case *RegexMatchCondition:
		// General-purpose regex match: <value> =~ "pattern"
		return fmt.Sprintf(`%s =~ %q`, g.valueToCUE(c.Source()), c.Pattern())
	case *RawCUECondition:
		// Raw CUE expression — emit verbatim
		return c.Expr()
	default:
		return cueBoolTrue
	}
}

// inConditionToCUE converts an InCondition to CUE syntax.
// Generates: parameter.name == val1 || parameter.name == val2 || ...
func (g *CUEGenerator) inConditionToCUE(c *InCondition) string {
	parts := make([]string, len(c.Values()))
	for i, v := range c.Values() {
		parts[i] = fmt.Sprintf("parameter.%s == %s", c.ParamName(), formatCUEValue(v))
	}
	return strings.Join(parts, " || ")
}

// exprToCUE converts an Expr to CUE syntax.
func (g *CUEGenerator) exprToCUE(e Expr) string {
	if v, ok := e.(Value); ok {
		return g.valueToCUE(v)
	}
	return "_"
}

// anyToCUE converts any value to CUE syntax.
func (g *CUEGenerator) anyToCUE(v any) string {
	if val, ok := v.(Value); ok {
		return g.valueToCUE(val)
	}
	return formatCUEValue(v)
}

// writeWorkload writes the workload definition.
func (g *CUEGenerator) writeWorkload(sb *strings.Builder, c *ComponentDefinition, depth int) {
	indent := strings.Repeat(g.indent, depth)
	workload := c.GetWorkload()

	// Check for autodetect workload type
	if workload.IsAutodetect() {
		sb.WriteString(fmt.Sprintf("%sworkload: type: %q\n", indent, "autodetects.core.oam.dev"))
		return
	}

	sb.WriteString(fmt.Sprintf("%sworkload: {\n", indent))
	sb.WriteString(fmt.Sprintf("%s%sdefinition: {\n", indent, g.indent))
	sb.WriteString(fmt.Sprintf("%s%s%sapiVersion: %q\n", indent, g.indent, g.indent, workload.APIVersion()))
	sb.WriteString(fmt.Sprintf("%s%s%skind:       %q\n", indent, g.indent, g.indent, workload.Kind()))
	sb.WriteString(fmt.Sprintf("%s%s}\n", indent, g.indent))

	// Write workload type (unless suppressed)
	if !c.IsOmitWorkloadType() {
		workloadType := g.inferWorkloadType(workload)
		sb.WriteString(fmt.Sprintf("%s%stype: %q\n", indent, g.indent, workloadType))
	}
	sb.WriteString(fmt.Sprintf("%s}\n", indent))
}

// inferWorkloadType infers the workload type from API version and kind.
func (g *CUEGenerator) inferWorkloadType(w WorkloadType) string {
	switch {
	case w.APIVersion() == "apps/v1" && w.Kind() == "Deployment":
		return "deployments.apps"
	case w.APIVersion() == "apps/v1" && w.Kind() == "StatefulSet":
		return "statefulsets.apps"
	case w.APIVersion() == "apps/v1" && w.Kind() == "DaemonSet":
		return "daemonsets.apps"
	case w.APIVersion() == "batch/v1" && w.Kind() == "Job":
		return "jobs.batch"
	case w.APIVersion() == "batch/v1" && w.Kind() == "CronJob":
		return "cronjobs.batch"
	default:
		return strings.ToLower(w.Kind()) + "s." + strings.Split(w.APIVersion(), "/")[0]
	}
}

// writeStatus writes the status configuration.
func (g *CUEGenerator) writeStatus(sb *strings.Builder, c *ComponentDefinition, depth int) {
	indent := strings.Repeat(g.indent, depth)

	customStatus := c.GetCustomStatus()
	healthPolicy := c.GetHealthPolicy()
	statusDetails := c.GetStatusDetails()

	if customStatus == "" && healthPolicy == "" && statusDetails == "" {
		return
	}

	sb.WriteString(fmt.Sprintf("%sstatus: {\n", indent))

	if customStatus != "" {
		sb.WriteString(fmt.Sprintf("%s%scustomStatus: #\"\"\"\n", indent, g.indent))
		for _, line := range strings.Split(customStatus, "\n") {
			sb.WriteString(fmt.Sprintf("%s%s%s%s\n", indent, g.indent, g.indent, line))
		}
		sb.WriteString(fmt.Sprintf("%s%s%s\"\"\"#\n", indent, g.indent, g.indent))
	}

	if healthPolicy != "" {
		sb.WriteString(fmt.Sprintf("%s%shealthPolicy: #\"\"\"\n", indent, g.indent))
		for _, line := range strings.Split(healthPolicy, "\n") {
			sb.WriteString(fmt.Sprintf("%s%s%s%s\n", indent, g.indent, g.indent, line))
		}
		sb.WriteString(fmt.Sprintf("%s%s%s\"\"\"#\n", indent, g.indent, g.indent))
	}

	if statusDetails != "" {
		sb.WriteString(fmt.Sprintf("%s%sdetails: #\"\"\"\n", indent, g.indent))
		for _, line := range strings.Split(statusDetails, "\n") {
			sb.WriteString(fmt.Sprintf("%s%s%s%s\n", indent, g.indent, g.indent, line))
		}
		sb.WriteString(fmt.Sprintf("%s%s%s\"\"\"#\n", indent, g.indent, g.indent))
	}

	sb.WriteString(fmt.Sprintf("%s}\n", indent))
}

// writeParam writes a single parameter definition.
func (g *CUEGenerator) writeParam(sb *strings.Builder, param Param, depth int) {
	indent := strings.Repeat(g.indent, depth)

	// Write // +ignore directive if set (before +usage)
	if ip, ok := param.(interface{ IsIgnore() bool }); ok && ip.IsIgnore() {
		sb.WriteString(fmt.Sprintf("%s// +ignore\n", indent))
	}

	// Write description as comment if present
	if desc := param.GetDescription(); desc != "" {
		sb.WriteString(fmt.Sprintf("%s// +usage=%s\n", indent, desc))
	}

	// Write // +short=X directive if set (after +usage)
	if sp, ok := param.(interface{ GetShort() string }); ok && sp.GetShort() != "" {
		sb.WriteString(fmt.Sprintf("%s// +short=%s\n", indent, sp.GetShort()))
	}

	name := param.Name()
	marker := fieldMarkerNone
	if param.IsRequired() {
		marker = fieldMarkerRequired
	} else if param.IsOptional() {
		marker = fieldMarkerOptional
	}

	// Handle different parameter types
	switch p := param.(type) {
	case *StringParam:
		g.writeStringParam(sb, p, indent, name, marker)
	case *IntParam:
		g.writeIntParam(sb, p, indent, name, marker)
	case *BoolParam:
		g.writeBoolParam(sb, p, indent, name, marker)
	case *FloatParam:
		g.writeFloatParam(sb, p, indent, name, marker)
	case *ArrayParam:
		g.writeArrayParam(sb, p, indent, name, marker, depth)
	case *MapParam:
		g.writeMapParam(sb, p, indent, name, marker, depth)
	case *StringKeyMapParam:
		g.writeStringKeyMapParam(sb, p, indent, name, marker)
	case *StructParam:
		g.writeStructParam(sb, p, indent, name, marker, depth)
	case *EnumParam:
		g.writeEnumParam(sb, p, indent, name, marker)
	case *OneOfParam:
		g.writeOneOfParam(sb, p, indent, name, marker, depth)
	case *ClosedUnionParam:
		g.writeClosedUnionParam(sb, p, indent, name, marker, depth)
	default:
		// Generic fallback
		sb.WriteString(fmt.Sprintf("%s%s%s: _\n", indent, name, marker))
	}
}

// writeStringParam writes a string parameter.
func (g *CUEGenerator) writeStringParam(sb *strings.Builder, p *StringParam, indent, name, optional string) {
	enumValues := p.GetEnumValues()

	if len(enumValues) > 0 {
		// Build enum type: "value1" | "value2" | ...
		var enumParts []string
		defaultVal := ""
		if p.HasDefault() {
			if dv, ok := p.GetDefault().(string); ok {
				defaultVal = dv
			}
		}

		if defaultVal != "" {
			// Add default first with asterisk
			enumParts = append(enumParts, fmt.Sprintf("*%q", defaultVal))
			// Add remaining values (skip default to avoid duplication)
			for _, v := range enumValues {
				if v != defaultVal {
					enumParts = append(enumParts, fmt.Sprintf("%q", v))
				}
			}
		} else {
			// No default, list all values
			for _, v := range enumValues {
				enumParts = append(enumParts, fmt.Sprintf("%q", v))
			}
		}
		if p.IsOpenEnum() {
			enumParts = append(enumParts, "string")
		}
		sb.WriteString(fmt.Sprintf("%s%s%s: %s\n", indent, name, optional, strings.Join(enumParts, " | ")))
	} else {
		// Build constraint parts
		var constraints []string

		// NotEmpty constraint: !=""
		if p.GetNotEmpty() {
			constraints = append(constraints, `!=""`)
		}

		// Pattern constraint: =~"pattern"
		if pattern := p.GetPattern(); pattern != "" {
			constraints = append(constraints, fmt.Sprintf(`=~%q`, pattern))
		}

		// NegativePattern constraint: !~"pattern"
		if negPattern := p.GetNegativePattern(); negPattern != "" {
			constraints = append(constraints, fmt.Sprintf(`!~%q`, negPattern))
		}

		// MinLen constraint: strings.MinRunes(n)
		if minLen := p.GetMinLen(); minLen != nil {
			constraints = append(constraints, fmt.Sprintf("strings.MinRunes(%d)", *minLen))
		}

		// MaxLen constraint: strings.MaxRunes(n)
		if maxLen := p.GetMaxLen(); maxLen != nil {
			constraints = append(constraints, fmt.Sprintf("strings.MaxRunes(%d)", *maxLen))
		}

		if p.HasDefault() {
			if len(constraints) > 0 {
				sb.WriteString(fmt.Sprintf("%s%s%s: *%q | string & %s\n", indent, name, optional, p.GetDefault(), strings.Join(constraints, " & ")))
			} else {
				sb.WriteString(fmt.Sprintf("%s%s%s: *%q | string\n", indent, name, optional, p.GetDefault()))
			}
		} else {
			if len(constraints) > 0 {
				sb.WriteString(fmt.Sprintf("%s%s%s: string & %s\n", indent, name, optional, strings.Join(constraints, " & ")))
			} else {
				sb.WriteString(fmt.Sprintf("%s%s%s: string\n", indent, name, optional))
			}
		}
	}
}

// writeIntParam writes an integer parameter.
func (g *CUEGenerator) writeIntParam(sb *strings.Builder, p *IntParam, indent, name, optional string) {
	// Build constraint parts
	var constraints []string

	// Min constraint: >=n
	if minVal := p.GetMin(); minVal != nil {
		constraints = append(constraints, fmt.Sprintf(">=%d", *minVal))
	}

	// Max constraint: <=n
	if maxVal := p.GetMax(); maxVal != nil {
		constraints = append(constraints, fmt.Sprintf("<=%d", *maxVal))
	}

	if p.HasDefault() {
		if len(constraints) > 0 {
			sb.WriteString(fmt.Sprintf("%s%s%s: *%v | int & %s\n", indent, name, optional, p.GetDefault(), strings.Join(constraints, " & ")))
		} else {
			sb.WriteString(fmt.Sprintf("%s%s%s: *%v | int\n", indent, name, optional, p.GetDefault()))
		}
	} else {
		if len(constraints) > 0 {
			sb.WriteString(fmt.Sprintf("%s%s%s: int & %s\n", indent, name, optional, strings.Join(constraints, " & ")))
		} else {
			sb.WriteString(fmt.Sprintf("%s%s%s: int\n", indent, name, optional))
		}
	}
}

// writeBoolParam writes a boolean parameter.
func (g *CUEGenerator) writeBoolParam(sb *strings.Builder, p *BoolParam, indent, name, optional string) {
	if p.HasDefault() {
		sb.WriteString(fmt.Sprintf("%s%s%s: *%v | bool\n", indent, name, optional, p.GetDefault()))
	} else {
		sb.WriteString(fmt.Sprintf("%s%s%s: bool\n", indent, name, optional))
	}
}

// writeFloatParam writes a float parameter.
func (g *CUEGenerator) writeFloatParam(sb *strings.Builder, p *FloatParam, indent, name, optional string) {
	// Build constraint parts
	var constraints []string

	// Min constraint: >=n
	if minVal := p.GetMin(); minVal != nil {
		constraints = append(constraints, fmt.Sprintf(">=%v", *minVal))
	}

	// Max constraint: <=n
	if maxVal := p.GetMax(); maxVal != nil {
		constraints = append(constraints, fmt.Sprintf("<=%v", *maxVal))
	}

	if p.HasDefault() {
		if len(constraints) > 0 {
			sb.WriteString(fmt.Sprintf("%s%s%s: *%v | number & %s\n", indent, name, optional, p.GetDefault(), strings.Join(constraints, " & ")))
		} else {
			sb.WriteString(fmt.Sprintf("%s%s%s: *%v | float\n", indent, name, optional, p.GetDefault()))
		}
	} else {
		if len(constraints) > 0 {
			sb.WriteString(fmt.Sprintf("%s%s%s: number & %s\n", indent, name, optional, strings.Join(constraints, " & ")))
		} else {
			sb.WriteString(fmt.Sprintf("%s%s%s: float\n", indent, name, optional))
		}
	}
}

// writeArrayParam writes an array parameter.
func (g *CUEGenerator) writeArrayParam(sb *strings.Builder, p *ArrayParam, indent, name, optional string, depth int) {
	// Build constraint prefix for MinItems/MaxItems
	var constraints []string

	// MinItems constraint: list.MinItems(n)
	if minItems := p.GetMinItems(); minItems != nil {
		constraints = append(constraints, fmt.Sprintf("list.MinItems(%d)", *minItems))
	}

	// MaxItems constraint: list.MaxItems(n)
	if maxItems := p.GetMaxItems(); maxItems != nil {
		constraints = append(constraints, fmt.Sprintf("list.MaxItems(%d)", *maxItems))
	}

	constraintPrefix := ""
	if len(constraints) > 0 {
		constraintPrefix = strings.Join(constraints, " & ") + " & "
	}

	// Priority: schemaRef > schema > fields > elementType
	if schemaRef := p.GetSchemaRef(); schemaRef != "" {
		// Reference to a helper definition like #HealthProbe
		// For arrays, output [...#SchemaRef] to indicate an array of the helper type
		sb.WriteString(fmt.Sprintf("%s%s%s: %s[...#%s]\n", indent, name, optional, constraintPrefix, schemaRef))
	} else if schema := p.GetSchema(); schema != "" {
		// Raw CUE schema - output directly, with optional default
		if p.HasDefault() {
			defaultJSON := g.formatArrayDefault(p.GetDefault())
			if constraintPrefix != "" {
				sb.WriteString(fmt.Sprintf("%s%s%s: %s%s | *%s\n", indent, name, optional, constraintPrefix, schema, defaultJSON))
			} else {
				sb.WriteString(fmt.Sprintf("%s%s%s: %s | *%s\n", indent, name, optional, schema, defaultJSON))
			}
		} else {
			if constraintPrefix != "" {
				sb.WriteString(fmt.Sprintf("%s%s%s: %s%s\n", indent, name, optional, constraintPrefix, schema))
			} else {
				sb.WriteString(fmt.Sprintf("%s%s%s: %s\n", indent, name, optional, schema))
			}
		}
	} else {
		elemType := g.cueTypeForParamType(p.ElementType())

		// Check if array has structured fields
		if fields := p.GetFields(); len(fields) > 0 {
			sb.WriteString(fmt.Sprintf("%s%s%s: %s[...{\n", indent, name, optional, constraintPrefix))
			for _, field := range fields {
				g.writeParam(sb, field, depth+1)
			}
			// Write validators inside each array element struct
			for _, v := range p.GetValidators() {
				g.writeValidator(sb, v, depth+1)
			}
			sb.WriteString(fmt.Sprintf("%s}]\n", indent))
		} else if elemType != "" {
			if p.HasNotEmpty() {
				sb.WriteString(fmt.Sprintf("%s%s%s: %s[...(%s & !=\"\")]\n", indent, name, optional, constraintPrefix, elemType))
			} else {
				sb.WriteString(fmt.Sprintf("%s%s%s: %s[...%s]\n", indent, name, optional, constraintPrefix, elemType))
			}
		} else {
			sb.WriteString(fmt.Sprintf("%s%s%s: %s[...]\n", indent, name, optional, constraintPrefix))
		}
	}

	// Write non-empty check after the array declaration
	if msg := p.GetNonEmptyMessage(); msg != "" {
		g.writeNonEmptyCheck(sb, name, msg, depth)
	}
}

// formatArrayDefault formats an array default value as a CUE literal.
// Converts []any{"*"} to `["*"]`, []any{"Observe"} to `["Observe"]`, etc.
func (g *CUEGenerator) formatArrayDefault(val any) string {
	if val == nil {
		return "[]"
	}
	switch v := val.(type) {
	case []any:
		parts := make([]string, 0, len(v))
		for _, elem := range v {
			parts = append(parts, fmt.Sprintf("%q", elem))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprintf("%v", val)
	}
}

// writeMapParam writes a map/object parameter.
func (g *CUEGenerator) writeMapParam(sb *strings.Builder, p *MapParam, indent, name, optional string, depth int) {
	// Priority: schemaRef > schema > fields > generic
	if schemaRef := p.GetSchemaRef(); schemaRef != "" {
		// Reference to a helper definition like #HealthProbe
		sb.WriteString(fmt.Sprintf("%s%s%s: #%s\n", indent, name, optional, schemaRef))
		return
	}

	if schema := p.GetSchema(); schema != "" {
		// Raw CUE schema - output directly
		sb.WriteString(fmt.Sprintf("%s%s%s: %s\n", indent, name, optional, schema))
		return
	}

	// Check if map has structured fields, validators, or conditional fields
	hasFields := len(p.GetFields()) > 0
	hasValidators := len(p.GetValidators()) > 0
	hasConditionalFields := len(p.GetConditionalFields()) > 0

	if hasFields || hasValidators || hasConditionalFields {
		if p.IsClosed() {
			sb.WriteString(fmt.Sprintf("%s%s%s: close({\n", indent, name, optional))
		} else {
			sb.WriteString(fmt.Sprintf("%s%s%s: {\n", indent, name, optional))
		}
		for _, field := range p.GetFields() {
			g.writeParam(sb, field, depth+1)
		}
		// Write validators inside the struct
		for _, v := range p.GetValidators() {
			g.writeValidator(sb, v, depth+1)
		}
		// Write conditional fields inside the struct
		for _, branch := range p.GetConditionalFields() {
			condCUE := g.conditionToCUE(branch.Condition())
			sb.WriteString(fmt.Sprintf("%s\tif %s {\n", indent, condCUE))
			for _, param := range branch.GetParams() {
				g.writeParam(sb, param, depth+2)
			}
			for _, v := range branch.GetValidators() {
				g.writeValidator(sb, v, depth+2)
			}
			sb.WriteString(fmt.Sprintf("%s\t}\n", indent))
		}
		if p.IsClosed() {
			sb.WriteString(fmt.Sprintf("%s})\n", indent))
		} else {
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
		}
	} else if valType := p.ValueType(); valType != "" {
		// Typed map: [string]: type
		cueType := g.cueTypeForParamType(valType)
		if cueType != "" {
			sb.WriteString(fmt.Sprintf("%s%s%s: [string]: %s\n", indent, name, optional, cueType))
		} else {
			sb.WriteString(fmt.Sprintf("%s%s%s: %s\n", indent, name, optional, cueOpenStruct))
		}
	} else {
		// Generic object
		sb.WriteString(fmt.Sprintf("%s%s%s: %s\n", indent, name, optional, cueOpenStruct))
	}
}

// writeStringKeyMapParam writes a string-to-string map parameter.
// Note: description is already written by writeParam, so we don't write it here.
func (g *CUEGenerator) writeStringKeyMapParam(sb *strings.Builder, _ *StringKeyMapParam, indent, name, optional string) {
	sb.WriteString(fmt.Sprintf("%s%s%s: [string]: string\n", indent, name, optional))
}

// writeStructParam writes a struct parameter.
func (g *CUEGenerator) writeStructParam(sb *strings.Builder, p *StructParam, indent, name, optional string, depth int) {
	sb.WriteString(fmt.Sprintf("%s%s%s: {\n", indent, name, optional))
	for _, field := range p.GetFields() {
		g.writeStructField(sb, field, depth+1)
	}
	sb.WriteString(fmt.Sprintf("%s}\n", indent))
}

// writeStructField writes a struct field.
func (g *CUEGenerator) writeStructField(sb *strings.Builder, f *StructField, depth int) {
	indent := strings.Repeat(g.indent, depth)

	// Write description as comment if present
	if desc := f.GetDescription(); desc != "" {
		sb.WriteString(fmt.Sprintf("%s// +usage=%s\n", indent, desc))
	}

	name := f.Name()
	marker := fieldMarkerNone
	if f.IsRequired() {
		marker = fieldMarkerRequired
	} else if f.IsOptional() {
		marker = fieldMarkerOptional
	}

	fieldType := g.cueTypeForParamType(f.FieldType())

	// Check schemaRef first — references a helper definition like #HealthProbe
	if schemaRef := f.GetSchemaRef(); schemaRef != "" {
		if f.FieldType() == ParamTypeArray {
			sb.WriteString(fmt.Sprintf("%s%s%s: [...#%s]\n", indent, name, marker, schemaRef))
		} else {
			sb.WriteString(fmt.Sprintf("%s%s%s: #%s\n", indent, name, marker, schemaRef))
		}
		return
	}

	nested := f.GetNested()
	switch {
	case nested != nil:
		if f.FieldType() == ParamTypeArray {
			sb.WriteString(fmt.Sprintf("%s%s%s: [...{\n", indent, name, marker))
			for _, nestedField := range nested.GetFields() {
				g.writeStructField(sb, nestedField, depth+1)
			}
			sb.WriteString(fmt.Sprintf("%s}]\n", indent))
		} else {
			sb.WriteString(fmt.Sprintf("%s%s%s: {\n", indent, name, marker))
			for _, nestedField := range nested.GetFields() {
				g.writeStructField(sb, nestedField, depth+1)
			}
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
		}
	case f.HasDefault():
		enumValues := f.GetEnumValues()
		switch {
		case len(enumValues) > 0:
			// Enum with default: *"default" | "other1" | "other2"
			defaultStr := fmt.Sprintf("%v", f.GetDefault())
			var enumParts []string
			enumParts = append(enumParts, fmt.Sprintf("*%s", formatCUEValue(f.GetDefault())))
			for _, v := range enumValues {
				if v != defaultStr {
					enumParts = append(enumParts, fmt.Sprintf("%q", v))
				}
			}
			sb.WriteString(fmt.Sprintf("%s%s: %s\n", indent, name, strings.Join(enumParts, " | ")))
		case f.FieldType() == ParamTypeArray && f.GetElementType() != "":
			elemCUE := g.cueTypeForParamType(f.GetElementType())
			sb.WriteString(fmt.Sprintf("%s%s: *%v | [...%s]\n", indent, name, formatCUEValue(f.GetDefault()), elemCUE))
		default:
			sb.WriteString(fmt.Sprintf("%s%s: *%v | %s\n", indent, name, formatCUEValue(f.GetDefault()), fieldType))
		}
	case f.FieldType() == ParamTypeArray && f.GetElementType() != "":
		elemCUE := g.cueTypeForParamType(f.GetElementType())
		sb.WriteString(fmt.Sprintf("%s%s%s: [...%s]\n", indent, name, marker, elemCUE))
	case len(f.GetEnumValues()) > 0:
		// Enum without default: "value1" | "value2"
		var enumParts []string
		for _, v := range f.GetEnumValues() {
			enumParts = append(enumParts, fmt.Sprintf("%q", v))
		}
		sb.WriteString(fmt.Sprintf("%s%s%s: %s\n", indent, name, marker, strings.Join(enumParts, " | ")))
	default:
		sb.WriteString(fmt.Sprintf("%s%s%s: %s\n", indent, name, marker, fieldType))
	}
}

// writeEnumParam writes an enum parameter.
func (g *CUEGenerator) writeEnumParam(sb *strings.Builder, p *EnumParam, indent, name, optional string) {
	values := p.GetValues()
	if len(values) == 0 {
		sb.WriteString(fmt.Sprintf("%s%s%s: string\n", indent, name, optional))
		return
	}

	if p.HasDefault() {
		defaultVal := p.GetDefault()
		// Build enum with default: *"default" | "other1" | "other2"
		// Skip the default value in the list to avoid duplication
		var enumParts []string
		enumParts = append(enumParts, fmt.Sprintf("*%q", defaultVal))
		for _, v := range values {
			if v != defaultVal {
				enumParts = append(enumParts, fmt.Sprintf("%q", v))
			}
		}
		sb.WriteString(fmt.Sprintf("%s%s%s: %s\n", indent, name, optional, strings.Join(enumParts, " | ")))
	} else {
		// Build enum type without default: "value1" | "value2" | ...
		var enumParts []string
		for _, v := range values {
			enumParts = append(enumParts, fmt.Sprintf("%q", v))
		}
		sb.WriteString(fmt.Sprintf("%s%s%s: %s\n", indent, name, optional, strings.Join(enumParts, " | ")))
	}
}

// writeOneOfParam writes a discriminated union parameter.
// The param name is used as the discriminator field name (e.g., OneOf("type")).
// Generates:
//
//	type: *"default" | "variant1" | "variant2"
//	if type == "variant1" { field1: string }
//	if type == "variant2" { field2: int }
func (g *CUEGenerator) writeOneOfParam(sb *strings.Builder, p *OneOfParam, indent, name, optional string, depth int) {
	variants := p.GetVariants()

	// Build discriminator field: type: *"default" | "variant1" | "variant2"
	var enumParts []string
	if p.HasDefault() {
		defaultStr := fmt.Sprintf("%v", p.GetDefault())
		enumParts = append(enumParts, fmt.Sprintf("*%s", formatCUEValue(p.GetDefault())))
		for _, v := range variants {
			if v.Name() != defaultStr {
				enumParts = append(enumParts, fmt.Sprintf("%q", v.Name()))
			}
		}
	} else {
		for _, v := range variants {
			enumParts = append(enumParts, fmt.Sprintf("%q", v.Name()))
		}
	}

	sb.WriteString(fmt.Sprintf("%s%s%s: %s\n", indent, name, optional, strings.Join(enumParts, " | ")))

	// Write conditional blocks for each variant
	for _, variant := range variants {
		fields := variant.GetFields()
		if len(fields) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("%sif %s == %q {\n", indent, name, variant.Name()))
		for _, field := range fields {
			g.writeStructField(sb, field, depth+1)
		}
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	}
}

// writeClosedUnionParam writes a closed struct disjunction parameter.
// It generates CUE of the form: name: close({...}) | close({...})
func (g *CUEGenerator) writeClosedUnionParam(sb *strings.Builder, p *ClosedUnionParam, indent, name, optional string, depth int) {
	options := p.GetOptions()
	if len(options) == 0 {
		sb.WriteString(fmt.Sprintf("%s%s%s: _\n", indent, name, optional))
		return
	}

	// Open the field assignment
	sb.WriteString(fmt.Sprintf("%s%s%s: ", indent, name, optional))

	for i, opt := range options {
		if i > 0 {
			sb.WriteString(" | ")
		}
		sb.WriteString("close({\n")
		for _, field := range opt.GetFields() {
			g.writeStructField(sb, field, depth+1)
		}
		sb.WriteString(fmt.Sprintf("%s})", indent))
	}
	sb.WriteString("\n")
}

// cueTypeStr converts a ParamType to its CUE type string.
func cueTypeStr(pt ParamType) string {
	switch pt {
	case ParamTypeString:
		return string(ParamTypeString)
	case ParamTypeInt:
		return "int"
	case ParamTypeBool:
		return "bool"
	case ParamTypeFloat:
		return "float"
	case ParamTypeArray:
		return "[...]"
	case ParamTypeMap, ParamTypeStruct, ParamTypeOneOf, ParamTypeClosedUnion:
		return cueOpenStruct
	default:
		return "_"
	}
}

// cueTypeForParamType converts a ParamType to its CUE type string.
func (g *CUEGenerator) cueTypeForParamType(pt ParamType) string {
	return cueTypeStr(pt)
}

// formatCUEValue formats a Go value as a CUE literal.
func formatCUEValue(v any) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case int, int32, int64, float32, float64:
		return fmt.Sprintf("%v", val)
	case bool:
		return fmt.Sprintf("%v", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
