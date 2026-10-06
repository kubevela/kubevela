/*
Copyright 2026 The KubeVela Authors.

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

package schema

import (
	"context"
	"fmt"
	"sort"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kubevela/pkg/cue/cuex"

	"github.com/kubevela/pkg/util/stringtools"

	"github.com/oam-dev/kubevela/pkg/cue/process"
	uischema "github.com/oam-dev/kubevela/pkg/utils/schema"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
)

// schemaContext declares the context fields a parameter default may read, so
// `*context.namespace | string` resolves to a string rather than an undefined
// reference.
const schemaContext = `
context: {
	name:             string
	namespace:        string
	cluster:          string
	appName:          string
	appRevision:      string
	appRevisionNum:   int
	appLabels: [string]:      string
	appAnnotations: [string]: string
	publishVersion:   string
	workflowName:     string
	outputSecretName: string
	revision:         string
	componentName:    string
	componentType:    string
	traitType:        string
	stepName:         string
	stepType:         string
	replicaKey:       string
	policyName:       string
	policyType:       string
	config?: [...{
		name:  string
		value: string
	}]
	...
}
`

// ParameterSchemas are the two schemas generated from a template's parameter.
type ParameterSchemas struct {
	OpenAPI *openapi3.Schema
	UI      uischema.UISchema
}

// GenerateParameterSchemas reads a template's `parameter` into both its
// OpenAPI schema and the default UI schema VelaUX renders a form from.
func GenerateParameterSchemas(ctx context.Context, template string) (*ParameterSchemas, error) {
	val, src, err := compilePruned(ctx, template, process.ParameterFieldName)
	if err != nil {
		return nil, err
	}
	return GenerateParameterSchemasFromValue(val, src)
}

// SourceFieldName is the block of a SourceDefinition declaring the value it
// resolves to.
const SourceFieldName = "schema"

// SourceSchemas are the schemas generated from a SourceDefinition template.
type SourceSchemas struct {
	Parameter *ParameterSchemas
	// Output is the OpenAPI schema of the template's `schema`, the value an
	// Application reads with $(source.<name>); nil when there is none.
	Output *openapi3.Schema
}

// GenerateSourceSchemas reads a SourceDefinition template's `parameter` and
// `schema` from one compile.
func GenerateSourceSchemas(ctx context.Context, template string) (*SourceSchemas, error) {
	val, src, err := compilePruned(ctx, template, process.ParameterFieldName, SourceFieldName)
	if err != nil {
		return nil, err
	}
	param, err := GenerateParameterSchemasFromValue(val, src)
	if err != nil {
		return nil, err
	}
	ss := &SourceSchemas{Parameter: param}
	out := val.LookupPath(cue.ParsePath(SourceFieldName))
	if !out.Exists() {
		return ss, nil
	}
	if err := out.Validate(); err != nil {
		return nil, err
	}
	ss.Output = BuildField(out, src).OpenAPI()
	return ss, nil
}

// compilePruned compiles a template reduced to the named top-level fields, with
// the context stub their defaults may read. Plain CUE compiles it unless an
// import needs the providers.
func compilePruned(ctx context.Context, template string, fields ...string) (cue.Value, string, error) {
	src := template
	if pruned, err := PruneTo(template, fields...); err == nil {
		src = pruned
	}
	src = InstrumentClauses(src)
	full := src + "\n" + schemaContext
	val := cuecontext.New().CompileString(full)
	if err := val.Err(); err != nil {
		if !needsCuex(src) {
			return cue.Value{}, "", err
		}
		if val, err = providers.DefaultCompiler.Get().CompileStringWithOptions(ctx, full, cuex.DisableResolveProviderFunctions{}); err != nil {
			return cue.Value{}, "", err
		}
	}
	return val, src, nil
}

// GenerateParameterSchemasAt is GenerateParameterSchemas for a template whose
// parameter sits under path rather than at the top, such as a config
// template's `template.parameter`.
func GenerateParameterSchemasAt(ctx context.Context, template, path string) (*ParameterSchemas, error) {
	src := InstrumentClauses(template)
	full := src + "\n" + schemaContext
	val := cuecontext.New().CompileString(full)
	if err := val.Err(); err != nil {
		if !needsCuex(src) {
			return nil, err
		}
		val, err = providers.DefaultCompiler.Get().CompileStringWithOptions(ctx, full, cuex.DisableResolveProviderFunctions{})
		if err != nil {
			return nil, err
		}
	}
	at := val.LookupPath(cue.ParsePath(path))
	if !at.Exists() {
		return nil, fmt.Errorf("template has no %q", path)
	}
	return GenerateParameterSchemasFromValue(at, src)
}

// GenerateParameterSchemasFromValue is GenerateParameterSchemas for a template
// already compiled, such as one that extends another.
func GenerateParameterSchemasFromValue(val cue.Value, src string) (*ParameterSchemas, error) {
	param := val.LookupPath(cue.ParsePath(process.ParameterFieldName))
	model := &Field{Kind: KindObject}
	if param.Exists() {
		// An error in a field surfaces only when the field is evaluated, so
		// the walk alone would read past it.
		if err := param.Validate(); err != nil {
			return nil, err
		}
		model = BuildField(param, src)
	}
	return &ParameterSchemas{OpenAPI: model.OpenAPI(), UI: model.UIParameters()}, nil
}

// OpenAPI renders the field as an OpenAPI 3.0 schema.
func (f *Field) OpenAPI() *openapi3.Schema {
	s := &openapi3.Schema{Title: f.Name, Description: f.Description, Default: f.Default, Nullable: f.Nullable}
	if f.Immutable {
		s.Extensions = map[string]any{ExtensionImmutable: true}
	}
	if !f.UI.empty() {
		if s.Extensions == nil {
			s.Extensions = map[string]any{}
		}
		s.Extensions[ExtensionUI] = f.UI
	}
	switch f.Kind {
	case KindString:
		s.Type = &openapi3.Types{openapi3.TypeString}
	case KindBytes:
		s.Type, s.Format = &openapi3.Types{openapi3.TypeString}, "binary"
	case KindInt:
		s.Type = &openapi3.Types{openapi3.TypeInteger}
	case KindNumber:
		s.Type = &openapi3.Types{openapi3.TypeNumber}
	case KindBool:
		s.Type = &openapi3.Types{openapi3.TypeBoolean}
	case KindArray:
		s.Type = &openapi3.Types{openapi3.TypeArray}
		items := &openapi3.Schema{}
		if f.Items != nil {
			items = f.Items.OpenAPI()
			items.Title = ""
		}
		s.Items = openapi3.NewSchemaRef("", items)
	case KindMap:
		s.Type = &openapi3.Types{openapi3.TypeObject}
		values := &openapi3.Schema{}
		if f.Values != nil {
			values = f.Values.OpenAPI()
			values.Title = ""
		}
		s.AdditionalProperties = openapi3.AdditionalProperties{Schema: openapi3.NewSchemaRef("", values)}
	case KindObject:
		s.Type = &openapi3.Types{openapi3.TypeObject}
		if f.Recursive {
			open := true
			s.AdditionalProperties = openapi3.AdditionalProperties{Has: &open}
			break
		}
		switch {
		case f.Values != nil:
			values := f.Values.OpenAPI()
			values.Title = ""
			s.AdditionalProperties = openapi3.AdditionalProperties{Schema: openapi3.NewSchemaRef("", values)}
		case f.Open:
			s.AdditionalProperties = openapi3.AdditionalProperties{Schema: openapi3.NewSchemaRef("", &openapi3.Schema{})}
		}
		s.Properties = properties(f.Fields)
		for _, c := range f.Fields {
			if !c.Optional && len(c.Conditions) == 0 {
				s.Required = append(s.Required, c.Name)
			}
		}
		switch len(f.Discriminators) {
		case 0:
		case 1:
			s.OneOf = f.branches(f.Discriminators[0])
			s.Discriminator = &openapi3.Discriminator{PropertyName: f.Discriminators[0]}
		default:
			// A schema has one oneOf and one discriminator, so each further
			// discriminator's branches go in an allOf entry of their own.
			for _, d := range f.Discriminators {
				s.AllOf = append(s.AllOf, openapi3.NewSchemaRef("", &openapi3.Schema{OneOf: f.branches(d)}))
			}
		}
	case KindOneOf:
		if f.allObjects() {
			// A choice between structs keeps the layout existing readers
			// expect: every property at the top, each branch naming the ones
			// it requires.
			s.Type = &openapi3.Types{openapi3.TypeObject}
			s.Properties = properties(f.formFields())
			for _, v := range f.Variants {
				b := &openapi3.Schema{}
				for _, c := range v.Fields {
					if !c.Optional && len(c.Conditions) == 0 {
						b.Required = append(b.Required, c.Name)
					}
				}
				s.OneOf = append(s.OneOf, openapi3.NewSchemaRef("", b))
			}
			break
		}
		for _, v := range f.Variants {
			vs := v.OpenAPI()
			vs.Title = ""
			s.OneOf = append(s.OneOf, openapi3.NewSchemaRef("", vs))
		}
	default:
		// Other kinds need nothing here.
	}
	s.Enum = f.Enum
	s.Min, s.Max, s.ExclusiveMin, s.ExclusiveMax = f.Min, f.Max, f.ExclusiveMin, f.ExclusiveMax
	s.MultipleOf, s.UniqueItems = f.MultipleOf, f.UniqueItems
	pattern, more := splitPatterns(f.Patterns)
	s.Pattern, s.AllOf = pattern, append(s.AllOf, more...)
	if f.MinLength != nil {
		s.MinLength = *f.MinLength
	}
	s.MaxLength = f.MaxLength
	if f.MinItems != nil {
		s.MinItems = *f.MinItems
	}
	s.MaxItems = f.MaxItems
	if f.MinProperties != nil {
		s.MinProps = *f.MinProperties
	}
	switch {
	case len(f.NotPatterns) > 0:
		s.Not = openapi3.NewSchemaRef("", &openapi3.Schema{Pattern: anyPattern(f.NotPatterns)})
	case len(f.NotEqual) > 0:
		s.Not = openapi3.NewSchemaRef("", &openapi3.Schema{Enum: f.NotEqual})
	}
	return s
}

// branches renders one oneOf entry per discriminator value, pinning the value
// and requiring the conditional fields that value alone brings.
func (f *Field) branches(name string) openapi3.SchemaRefs {
	var disc *Field
	for _, c := range f.Fields {
		if c.Name == name {
			disc = c
		}
	}
	if disc == nil {
		return nil
	}
	values := disc.Enum
	if disc.Kind == KindBool {
		values = []any{true, false}
	}
	var out openapi3.SchemaRefs
	for _, v := range values {
		b := &openapi3.Schema{Properties: openapi3.Schemas{
			name: openapi3.NewSchemaRef("", &openapi3.Schema{Enum: []any{v}}),
		}}
		for _, c := range f.Fields {
			if !c.Optional && len(c.Conditions) > 0 && onlyOn(c.Conditions, name, v) {
				b.Required = append(b.Required, c.Name)
			}
		}
		out = append(out, openapi3.NewSchemaRef("", b))
	}
	return out
}

// onlyOn reports whether conds all hold when the field name takes the value v,
// whatever the other fields are.
func onlyOn(conds []Condition, name string, v any) bool {
	for _, cond := range conds {
		if cond.Ref != name || !containsValue(cond.Values, v) {
			return false
		}
	}
	return true
}

// UIParameters renders an object's fields as the default VelaUX form, in the
// order the template declares them.
func (f *Field) UIParameters() uischema.UISchema {
	var out uischema.UISchema
	for i, c := range placeByOrder(f.formFields()) {
		p := c.uiParameter()
		p.Sort = uint(100 + i)
		out = append(out, p)
	}
	return out
}

// placeByOrder puts a field with `+ui:order=N` at position N among its
// siblings; the rest fill the other positions in declaration order.
func placeByOrder(fields []*Field) []*Field {
	var ordered, rest []*Field
	for _, f := range fields {
		if f.UI.Order != nil {
			ordered = append(ordered, f)
		} else {
			rest = append(rest, f)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool { return *ordered[i].UI.Order < *ordered[j].UI.Order })
	out := make([]*Field, 0, len(fields))
	for len(ordered) > 0 || len(rest) > 0 {
		if len(ordered) > 0 && (len(rest) == 0 || *ordered[0].UI.Order <= len(out)) {
			out, ordered = append(out, ordered[0]), ordered[1:]
			continue
		}
		out, rest = append(out, rest[0]), rest[1:]
	}
	return out
}

func (f *Field) uiParameter() *uischema.UIParameter {
	p := &uischema.UIParameter{
		JSONKey:     f.Name,
		Label:       stringtools.Capitalize(f.Name),
		Description: f.Description,
		Validate: &uischema.Validate{
			Required:     !f.Optional,
			DefaultValue: f.Default,
			Min:          f.Min,
			Max:          f.Max,
			MaxLength:    f.MaxLength,
			Pattern:      allPatterns(f.Patterns),
			Immutable:    f.Immutable,
		},
	}
	if f.MinLength != nil {
		p.Validate.MinLength = *f.MinLength
	}
	for _, e := range f.Enum {
		p.Validate.Options = append(p.Validate.Options, uischema.Option{Label: optionLabel(e), Value: e})
	}
	for _, c := range f.Conditions {
		p.Conditions = append(p.Conditions, c.ui())
	}

	switch f.Kind {
	case KindString, KindBytes:
		p.UIType = "Input"
		if len(f.Enum) > 1 {
			p.UIType = "Select"
		} else if len(f.UI.Suggest) > 0 {
			// Suggestions, unlike an enum's options, do not limit the value.
			p.UIType = "Suggest"
			for _, s := range f.UI.Suggest {
				p.Validate.Options = append(p.Validate.Options, uischema.Option{Label: s, Value: s})
			}
		}
	case KindInt, KindNumber:
		p.UIType = "Number"
		if len(f.Enum) > 1 {
			p.UIType = "Select"
		}
	case KindBool:
		p.UIType = "Switch"
	case KindArray:
		p.UIType = "Structs"
		if f.Items != nil {
			switch f.Items.Kind {
			case KindString:
				p.UIType = "Strings"
			case KindInt, KindNumber:
				p.UIType = "Numbers"
			case KindObject:
				p.SubParameters = f.Items.UIParameters()
			case KindOneOf:
				if f.Items.allObjects() {
					p.SubParameters = f.Items.UIParameters()
				}
			default:
				// Other kinds need nothing here.
			}
		}
	case KindMap:
		p.UIType = "KV"
		if f.Values != nil {
			additional := true
			p.Additional = &additional
			p.AdditionalParameter = f.Values.uiParameter()
			if f.Values.Kind == KindObject {
				// A map of structs is entered as rows: a key and the struct.
				p.UIType = "StructMap"
				p.SubParameters = f.Values.UIParameters()
			}
		}
	case KindObject:
		p.UIType = "Group"
		if len(f.Fields) == 0 {
			p.UIType = "KV"
		}
		p.SubParameters = f.UIParameters()
	case KindOneOf:
		// No widget chooses between shapes, so show the union, as VelaUX
		// does for the OpenAPI it derives from.
		p.UIType = "Input"
		if sub := f.UIParameters(); len(sub) > 0 {
			p.UIType = "Group"
			p.SubParameters = sub
		}
	default:
		p.UIType = "Input"
	}
	f.UI.applyTo(p)
	return p
}

func optionLabel(v any) string {
	if s, ok := v.(string); ok {
		return stringtools.Capitalize(s)
	}
	return fmt.Sprint(v)
}

// formFields are the fields a form shows for a value: an object's own, or
// for a choice between objects, the union of theirs, since no widget chooses
// between shapes. A field some variant lacks is optional in the union, and a
// name the variants give different shapes is one field that is one of them. A
// map shows none; its values are entered as KV.
func (f *Field) formFields() []*Field {
	switch f.Kind {
	case KindObject:
		return f.Fields
	case KindOneOf:
		var names []string
		shapes := map[string][]*Field{}
		in := map[string]int{}
		for _, v := range f.Variants {
			seen := map[string]bool{}
			for _, c := range v.formFields() {
				if !seen[c.Name] {
					seen[c.Name] = true
					in[c.Name]++
				}
				if len(shapes[c.Name]) == 0 {
					names = append(names, c.Name)
				}
				if !hasShape(shapes[c.Name], c.shape()) {
					shapes[c.Name] = append(shapes[c.Name], c)
				}
			}
		}
		var out []*Field
		for _, name := range names {
			fs := shapes[name]
			missing := in[name] < len(f.Variants)
			// Shapes with conditions stay apart, each shown when its own hold.
			if len(fs) > 1 && !conditional(fs) {
				u := &Field{Name: name, Kind: KindOneOf, Variants: fs, Description: fs[0].Description, Optional: missing}
				for _, c := range fs {
					u.Optional = u.Optional || c.Optional
				}
				out = append(out, u)
				continue
			}
			for _, c := range fs {
				if missing && !c.Optional {
					optional := *c
					optional.Optional = true
					c = &optional
				}
				out = append(out, c)
			}
		}
		return out
	default:
		// Other kinds need nothing here.
	}
	return nil
}

func hasShape(fields []*Field, shape string) bool {
	for _, f := range fields {
		if f.shape() == shape {
			return true
		}
	}
	return false
}

func conditional(fields []*Field) bool {
	for _, f := range fields {
		if len(f.Conditions) > 0 {
			return true
		}
	}
	return false
}

func (f *Field) allObjects() bool {
	for _, v := range f.Variants {
		if v.Kind != KindObject {
			return false
		}
	}
	return len(f.Variants) > 0
}

// properties renders an object's fields. Fields that share a name, because
// the name takes a different shape under different conditions, become one
// property that is one of those shapes.
func properties(fields []*Field) openapi3.Schemas {
	out := openapi3.Schemas{}
	for _, c := range fields {
		s := c.OpenAPI()
		if len(c.Conditions) > 0 {
			if s.Extensions == nil {
				s.Extensions = map[string]any{}
			}
			var conds []map[string]any
			for _, cond := range c.Conditions {
				u := cond.ui()
				m := map[string]any{"field": u.JSONKey, "value": u.Value}
				if u.Op != "" {
					m["op"] = u.Op
				}
				conds = append(conds, m)
			}
			s.Extensions[ExtensionConditions] = conds
		}
		prior, ok := out[c.Name]
		if !ok {
			out[c.Name] = openapi3.NewSchemaRef("", s)
			continue
		}
		if len(prior.Value.OneOf) == 0 || prior.Value.Title != "" {
			first := prior.Value
			prior = openapi3.NewSchemaRef("", &openapi3.Schema{OneOf: openapi3.SchemaRefs{openapi3.NewSchemaRef("", first)}})
			out[c.Name] = prior
		}
		prior.Value.OneOf = append(prior.Value.OneOf, openapi3.NewSchemaRef("", s))
	}
	return out
}

// ExtensionConditions is the OpenAPI extension listing the conditions under
// which a property exists, in the form VelaUX's ui-schema conditions take.
const ExtensionConditions = "x-vela-conditions"

// ui renders the condition as VelaUX evaluates it. A presence condition
// compares with null, which VelaUX's loose comparison also matches for an
// unset field.
func (c Condition) ui() uischema.Condition {
	switch {
	case c.Exists != nil && *c.Exists:
		return uischema.Condition{JSONKey: c.Ref, Op: "!=", Value: nil}
	case c.Exists != nil:
		return uischema.Condition{JSONKey: c.Ref, Op: "==", Value: nil}
	case len(c.Values) == 1:
		return uischema.Condition{JSONKey: c.Ref, Value: c.Values[0]}
	}
	return uischema.Condition{JSONKey: c.Ref, Op: "in", Value: c.Values}
}

// needsCuex reports whether a template imports a package plain CUE lacks, such
// as a vela/ provider or an external package, so only the cuex compiler can
// read it.
func needsCuex(src string) bool {
	f, err := parser.ParseFile("template", src, parser.ImportsOnly)
	if err != nil {
		return false
	}
	ctx := cuecontext.New()
	for _, spec := range f.Imports {
		probe := fmt.Sprintf("import p %s\n_probe: p", spec.Path.Value)
		if ctx.CompileString(probe).Err() != nil {
			return true
		}
	}
	return false
}

// applyTo sets what the hints say on a rendered parameter.
func (h UIHints) applyTo(p *uischema.UIParameter) {
	if h.Type != "" {
		p.UIType = h.Type
	}
	if h.Label != "" {
		p.Label = h.Label
	}
	if h.Hidden {
		hidden := true
		p.Disable = &hidden
	}
	if h.Error != "" && p.Validate != nil {
		p.Validate.Message = h.Error
	}
	style := uischema.Style{
		ColSpan:     h.ColSpan,
		Format:      h.Format,
		RowKey:      h.RowKey,
		ItemLabel:   h.ItemLabel,
		Placeholder: h.Placeholder,
		Advanced:    h.Advanced,
		Section:     h.Section,
		OptionsFrom: h.OptionsFrom,
		Expression:  h.Expression,
	}
	if style != (uischema.Style{}) {
		p.Style = &style
	}
}
