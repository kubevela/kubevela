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

// baseParam provides common parameter functionality.
type baseParam struct {
	name          string
	paramType     ParamType
	required      bool
	defaultValue  any
	description   string
	forceOptional bool   // when true, field stays optional even with a default value
	short         string // short flag alias (e.g. "i" → // +short=i)
	ignore        bool   // when true, emits // +ignore directive
}

func (p *baseParam) expr()      {}
func (p *baseParam) value()     {}
func (p *baseParam) condition() {}

func (p *baseParam) Name() string           { return p.name }
func (p *baseParam) IsRequired() bool       { return p.required }
func (p *baseParam) IsOptional() bool       { return !p.required }
func (p *baseParam) HasDefault() bool       { return p.defaultValue != nil }
func (p *baseParam) GetDefault() any        { return p.defaultValue }
func (p *baseParam) GetDescription() string { return p.description }
func (p *baseParam) IsForceOptional() bool  { return p.forceOptional }
func (p *baseParam) GetShort() string       { return p.short }
func (p *baseParam) IsIgnore() bool         { return p.ignore }

// IsSet returns a condition that checks if the parameter has a value.
// This is used with SetIf for conditional field assignment.
func (p *baseParam) IsSet() Condition {
	return &IsSetCondition{paramName: p.name}
}

// NotSet returns a condition that checks if the parameter is not set.
// This generates `if parameter["name"] == _|_` in CUE.
func (p *baseParam) NotSet() Condition {
	return &NotCondition{inner: &IsSetCondition{paramName: p.name}}
}

// Eq creates a condition that compares this parameter to a literal value.
// Example: replicas.Eq(3) generates: parameter.replicas == 3
func (p *baseParam) Eq(val any) Condition {
	return &ParamCompareCondition{paramName: p.name, op: "==", value: val}
}

// Ne creates a condition that checks if this parameter is not equal to a value.
// Example: status.Ne("error") generates: parameter.status != "error"
func (p *baseParam) Ne(val any) Condition {
	return &ParamCompareCondition{paramName: p.name, op: "!=", value: val}
}

// Gt creates a condition that checks if this parameter is greater than a value.
// Example: replicas.Gt(1) generates: parameter.replicas > 1
func (p *baseParam) Gt(val any) Condition {
	return &ParamCompareCondition{paramName: p.name, op: ">", value: val}
}

// Gte creates a condition that checks if this parameter is greater than or equal to a value.
// Example: replicas.Gte(1) generates: parameter.replicas >= 1
func (p *baseParam) Gte(val any) Condition {
	return &ParamCompareCondition{paramName: p.name, op: ">=", value: val}
}

// Lt creates a condition that checks if this parameter is less than a value.
// Example: replicas.Lt(10) generates: parameter.replicas < 10
func (p *baseParam) Lt(val any) Condition {
	return &ParamCompareCondition{paramName: p.name, op: "<", value: val}
}

// Lte creates a condition that checks if this parameter is less than or equal to a value.
// Example: replicas.Lte(10) generates: parameter.replicas <= 10
func (p *baseParam) Lte(val any) Condition {
	return &ParamCompareCondition{paramName: p.name, op: "<=", value: val}
}

// ParamCompareCondition represents a comparison between a parameter and a value.
type ParamCompareCondition struct {
	baseCondition
	paramName string
	op        string
	value     any
}

// ParamName returns the parameter name being compared.
func (c *ParamCompareCondition) ParamName() string { return c.paramName }

// Op returns the comparison operator.
func (c *ParamCompareCondition) Op() string { return c.op }

// CompareValue returns the comparison value.
func (c *ParamCompareCondition) CompareValue() any { return c.value }

// StringParam represents a string parameter.
type StringParam struct {
	baseParam
	enumValues []string // allowed enum values
	pattern    string   // regex pattern constraint
	minLen     *int     // minimum length constraint
	maxLen     *int     // maximum length constraint
}

// String creates a new string parameter with the given name.
func String(name string) *StringParam {
	return &StringParam{
		baseParam: baseParam{
			name:      name,
			paramType: ParamTypeString,
		},
	}
}

// Required marks the parameter as required.
func (p *StringParam) Required() *StringParam {
	p.required = true
	return p
}

// Optional marks the parameter as optional (default behavior).
func (p *StringParam) Optional() *StringParam {
	p.required = false
	return p
}

// ForceOptional makes the field optional even when it has a default value.
// Normally, fields with defaults are treated as always-present (no ? in CUE).
// This generates field?: *default | type instead of field: *default | type.
func (p *StringParam) ForceOptional() *StringParam {
	p.forceOptional = true
	return p
}

// Short sets a short flag alias for the parameter.
// This generates a // +short=X directive in the CUE output.
func (p *StringParam) Short(s string) *StringParam {
	p.short = s
	return p
}

// Ignore marks the parameter as ignored by the UI.
// This generates a // +ignore directive in the CUE output.
func (p *StringParam) Ignore() *StringParam {
	p.ignore = true
	return p
}

// Default sets a default value for the parameter.
func (p *StringParam) Default(value string) *StringParam {
	p.defaultValue = value
	return p
}

// Description sets the parameter description.
func (p *StringParam) Description(desc string) *StringParam {
	p.description = desc
	return p
}

// Enum restricts the parameter to specific allowed values.
func (p *StringParam) Enum(values ...string) *StringParam {
	p.enumValues = values
	return p
}

// GetEnumValues returns the allowed enum values.
func (p *StringParam) GetEnumValues() []string {
	return p.enumValues
}

// Pattern sets a regex pattern constraint for the parameter.
// This generates CUE like: string & =~"pattern"
func (p *StringParam) Pattern(regex string) *StringParam {
	p.pattern = regex
	return p
}

// GetPattern returns the regex pattern constraint.
func (p *StringParam) GetPattern() string {
	return p.pattern
}

// MinLen sets the minimum length constraint for the parameter.
// This generates CUE like: strings.MinRunes(n)
func (p *StringParam) MinLen(n int) *StringParam {
	p.minLen = &n
	return p
}

// GetMinLen returns the minimum length constraint, or nil if not set.
func (p *StringParam) GetMinLen() *int {
	return p.minLen
}

// MaxLen sets the maximum length constraint for the parameter.
// This generates CUE like: strings.MaxRunes(n)
func (p *StringParam) MaxLen(n int) *StringParam {
	p.maxLen = &n
	return p
}

// GetMaxLen returns the maximum length constraint, or nil if not set.
func (p *StringParam) GetMaxLen() *int {
	return p.maxLen
}

// Concat creates a string concatenation expression.
// Example: name.Concat("-suffix") generates: parameter.name + "-suffix"
func (p *StringParam) Concat(suffix string) Value {
	return &ParamConcatExpr{paramName: p.name, suffix: suffix}
}

// Prepend creates a string concatenation expression with a prefix.
// Example: name.Prepend("prefix-") generates: "prefix-" + parameter.name
func (p *StringParam) Prepend(prefix string) Value {
	return &ParamConcatExpr{paramName: p.name, prefix: prefix}
}

// --- StringParam Runtime Condition Methods ---

// Contains creates a condition that checks if this string parameter contains a substring.
// Example: name.Contains("prod") generates: strings.Contains(parameter.name, "prod")
func (p *StringParam) Contains(substr string) Condition {
	return &StringContainsCondition{paramName: p.name, substr: substr}
}

// Matches creates a condition that checks if this string parameter matches a regex pattern.
// Example: name.Matches("^prod-") generates: parameter.name =~ "^prod-"
func (p *StringParam) Matches(pattern string) Condition {
	return &StringMatchesCondition{paramName: p.name, pattern: pattern}
}

// StartsWith creates a condition that checks if this string parameter starts with a prefix.
// Example: name.StartsWith("prod-") generates: strings.HasPrefix(parameter.name, "prod-")
func (p *StringParam) StartsWith(prefix string) Condition {
	return &StringStartsWithCondition{paramName: p.name, prefix: prefix}
}

// EndsWith creates a condition that checks if this string parameter ends with a suffix.
// Example: name.EndsWith("-prod") generates: strings.HasSuffix(parameter.name, "-prod")
func (p *StringParam) EndsWith(suffix string) Condition {
	return &StringEndsWithCondition{paramName: p.name, suffix: suffix}
}

// LenEq creates a condition that checks if this string parameter has exactly n characters.
// Example: name.LenEq(5) generates: len(parameter.name) == 5
func (p *StringParam) LenEq(n int) Condition {
	return &LenCondition{paramName: p.name, op: "==", length: n}
}

// LenGt creates a condition that checks if this string parameter has more than n characters.
// Example: name.LenGt(5) generates: len(parameter.name) > 5
func (p *StringParam) LenGt(n int) Condition {
	return &LenCondition{paramName: p.name, op: ">", length: n}
}

// LenGte creates a condition that checks if this string parameter has n or more characters.
// Example: name.LenGte(5) generates: len(parameter.name) >= 5
func (p *StringParam) LenGte(n int) Condition {
	return &LenCondition{paramName: p.name, op: ">=", length: n}
}

// LenLt creates a condition that checks if this string parameter has fewer than n characters.
// Example: name.LenLt(5) generates: len(parameter.name) < 5
func (p *StringParam) LenLt(n int) Condition {
	return &LenCondition{paramName: p.name, op: "<", length: n}
}

// LenLte creates a condition that checks if this string parameter has n or fewer characters.
// Example: name.LenLte(5) generates: len(parameter.name) <= 5
func (p *StringParam) LenLte(n int) Condition {
	return &LenCondition{paramName: p.name, op: "<=", length: n}
}

// In creates a condition that checks if this string parameter is one of the given values.
// Example: name.In("api", "web", "worker") generates: parameter.name == "api" || parameter.name == "web" || parameter.name == "worker"
func (p *StringParam) In(values ...string) Condition {
	anyVals := make([]any, len(values))
	for i, v := range values {
		anyVals[i] = v
	}
	return &InCondition{paramName: p.name, values: anyVals}
}

// IntParam represents an integer parameter.
type IntParam struct {
	baseParam
	minVal *int // minimum value constraint
	maxVal *int // maximum value constraint
}

// Int creates a new integer parameter with the given name.
func Int(name string) *IntParam {
	return &IntParam{
		baseParam: baseParam{
			name:      name,
			paramType: ParamTypeInt,
		},
	}
}

// Required marks the parameter as required.
func (p *IntParam) Required() *IntParam {
	p.required = true
	return p
}

// Optional marks the parameter as optional (default behavior).
func (p *IntParam) Optional() *IntParam {
	p.required = false
	return p
}

// Default sets a default value for the parameter.
func (p *IntParam) Default(value int) *IntParam {
	p.defaultValue = value
	return p
}

// Description sets the parameter description.
func (p *IntParam) Description(desc string) *IntParam {
	p.description = desc
	return p
}

// Short sets a short flag alias for the parameter.
// This generates a // +short=X directive in the CUE output.
func (p *IntParam) Short(s string) *IntParam {
	p.short = s
	return p
}

// Ignore marks the parameter as ignored by the UI.
// This generates a // +ignore directive in the CUE output.
func (p *IntParam) Ignore() *IntParam {
	p.ignore = true
	return p
}

// Min sets the minimum value constraint for the parameter.
// This generates CUE like: int & >=n
func (p *IntParam) Min(n int) *IntParam {
	p.minVal = &n
	return p
}

// GetMin returns the minimum value constraint, or nil if not set.
func (p *IntParam) GetMin() *int {
	return p.minVal
}

// Max sets the maximum value constraint for the parameter.
// This generates CUE like: int & <=n
func (p *IntParam) Max(n int) *IntParam {
	p.maxVal = &n
	return p
}

// GetMax returns the maximum value constraint, or nil if not set.
func (p *IntParam) GetMax() *int {
	return p.maxVal
}

// In creates a condition that checks if this int parameter is one of the given values.
// Example: port.In(80, 443, 8080) generates: parameter.port == 80 || parameter.port == 443 || parameter.port == 8080
func (p *IntParam) In(values ...int) Condition {
	anyVals := make([]any, len(values))
	for i, v := range values {
		anyVals[i] = v
	}
	return &InCondition{paramName: p.name, values: anyVals}
}

// Add creates an arithmetic expression that adds a value to this parameter.
// Example: replicas.Add(1) generates: parameter.replicas + 1
func (p *IntParam) Add(val int) Value {
	return &ParamArithExpr{paramName: p.name, op: "+", arithVal: val}
}

// Sub creates an arithmetic expression that subtracts a value from this parameter.
// Example: replicas.Sub(1) generates: parameter.replicas - 1
func (p *IntParam) Sub(val int) Value {
	return &ParamArithExpr{paramName: p.name, op: "-", arithVal: val}
}

// Mul creates an arithmetic expression that multiplies this parameter by a value.
// Example: replicas.Mul(2) generates: parameter.replicas * 2
func (p *IntParam) Mul(val int) Value {
	return &ParamArithExpr{paramName: p.name, op: "*", arithVal: val}
}

// Div creates an arithmetic expression that divides this parameter by a value.
// Example: replicas.Div(2) generates: parameter.replicas / 2
func (p *IntParam) Div(val int) Value {
	return &ParamArithExpr{paramName: p.name, op: "/", arithVal: val}
}

// BoolParam represents a boolean parameter.
type BoolParam struct {
	baseParam
}

// Bool creates a new boolean parameter with the given name.
func Bool(name string) *BoolParam {
	return &BoolParam{
		baseParam: baseParam{
			name:      name,
			paramType: ParamTypeBool,
		},
	}
}

// Required marks the parameter as required.
func (p *BoolParam) Required() *BoolParam {
	p.required = true
	return p
}

// Optional marks the parameter as optional (default behavior).
func (p *BoolParam) Optional() *BoolParam {
	p.required = false
	return p
}

// Default sets a default value for the parameter.
func (p *BoolParam) Default(value bool) *BoolParam {
	p.defaultValue = value
	return p
}

// Description sets the parameter description.
func (p *BoolParam) Description(desc string) *BoolParam {
	p.description = desc
	return p
}

// Short sets a short flag alias for the parameter.
// This generates a // +short=X directive in the CUE output.
func (p *BoolParam) Short(s string) *BoolParam {
	p.short = s
	return p
}

// Ignore marks the parameter as ignored by the UI.
// This generates a // +ignore directive in the CUE output.
func (p *BoolParam) Ignore() *BoolParam {
	p.ignore = true
	return p
}

// IsTrue returns a condition that checks if the bool parameter is truthy.
// In CUE, this generates `if parameter.name` instead of `if parameter.name == true`.
func (p *BoolParam) IsTrue() Condition {
	return &TruthyCondition{paramName: p.name}
}

// IsFalse returns a condition that checks if the bool parameter is falsy.
// In CUE, this generates `if !parameter.name` instead of `if parameter.name == false`.
func (p *BoolParam) IsFalse() Condition {
	return &FalsyCondition{paramName: p.name}
}

// FloatParam represents a floating-point number parameter.
type FloatParam struct {
	baseParam
	minVal *float64 // minimum value constraint
	maxVal *float64 // maximum value constraint
}

// Float creates a new float parameter with the given name.
func Float(name string) *FloatParam {
	return &FloatParam{
		baseParam: baseParam{
			name:      name,
			paramType: ParamTypeFloat,
		},
	}
}

// Required marks the parameter as required.
func (p *FloatParam) Required() *FloatParam {
	p.required = true
	return p
}

// Optional marks the parameter as optional (default behavior).
func (p *FloatParam) Optional() *FloatParam {
	p.required = false
	return p
}

// Default sets a default value for the parameter.
func (p *FloatParam) Default(value float64) *FloatParam {
	p.defaultValue = value
	return p
}

// Description sets the parameter description.
func (p *FloatParam) Description(desc string) *FloatParam {
	p.description = desc
	return p
}

// Min sets the minimum value constraint for the parameter.
// This generates CUE like: number & >=n
func (p *FloatParam) Min(n float64) *FloatParam {
	p.minVal = &n
	return p
}

// GetMin returns the minimum value constraint, or nil if not set.
func (p *FloatParam) GetMin() *float64 {
	return p.minVal
}

// Max sets the maximum value constraint for the parameter.
// This generates CUE like: number & <=n
func (p *FloatParam) Max(n float64) *FloatParam {
	p.maxVal = &n
	return p
}

// GetMax returns the maximum value constraint, or nil if not set.
func (p *FloatParam) GetMax() *float64 {
	return p.maxVal
}

// In creates a condition that checks if this float parameter is one of the given values.
// Example: ratio.In(0.5, 1.0, 2.0) generates: parameter.ratio == 0.5 || parameter.ratio == 1.0 || parameter.ratio == 2.0
func (p *FloatParam) In(values ...float64) Condition {
	anyVals := make([]any, len(values))
	for i, v := range values {
		anyVals[i] = v
	}
	return &InCondition{paramName: p.name, values: anyVals}
}

// ArrayParam represents an array/list parameter.
type ArrayParam struct {
	baseParam
	elementType ParamType
	fields      []Param // fields for structured array elements
	schema      string  // raw CUE schema for the array elements
	schemaRef   string  // reference to a helper definition (e.g., "HealthProbe")
	minItems    *int    // minimum number of items
	maxItems    *int    // maximum number of items
}

// Array creates a new array parameter with the given name.
func Array(name string) *ArrayParam {
	return &ArrayParam{
		baseParam: baseParam{
			name:      name,
			paramType: ParamTypeArray,
		},
	}
}

// Of specifies the element type for the array.
func (p *ArrayParam) Of(elemType ParamType) *ArrayParam {
	p.elementType = elemType
	return p
}

// Required marks the parameter as required.
func (p *ArrayParam) Required() *ArrayParam {
	p.required = true
	return p
}

// Optional marks the parameter as optional (default behavior).
func (p *ArrayParam) Optional() *ArrayParam {
	p.required = false
	return p
}

// Default sets a default value for the parameter.
func (p *ArrayParam) Default(value []any) *ArrayParam {
	p.defaultValue = value
	return p
}

// Description sets the parameter description.
func (p *ArrayParam) Description(desc string) *ArrayParam {
	p.description = desc
	return p
}

// ElementType returns the array element type.
func (p *ArrayParam) ElementType() ParamType {
	return p.elementType
}

// WithFields adds field definitions for structured array elements.
// This allows defining the schema for objects within the array.
func (p *ArrayParam) WithFields(fields ...Param) *ArrayParam {
	p.fields = append(p.fields, fields...)
	return p
}

// GetFields returns the field definitions for array elements.
func (p *ArrayParam) GetFields() []Param {
	return p.fields
}

// WithSchema sets a raw CUE schema for the array elements.
// This takes precedence over WithFields for schema generation.
func (p *ArrayParam) WithSchema(schema string) *ArrayParam {
	p.schema = schema
	return p
}

// GetSchema returns the raw CUE schema for array elements.
func (p *ArrayParam) GetSchema() string {
	return p.schema
}

// WithSchemaRef sets a reference to a helper type definition (e.g., "#HealthProbe").
// This is used when the schema is defined elsewhere as a helper definition.
func (p *ArrayParam) WithSchemaRef(ref string) *ArrayParam {
	p.schemaRef = ref
	return p
}

// GetSchemaRef returns the schema reference for this parameter.
func (p *ArrayParam) GetSchemaRef() string {
	return p.schemaRef
}

// MinItems sets the minimum number of items constraint for the array.
// This generates CUE like: list.MinItems(n)
func (p *ArrayParam) MinItems(n int) *ArrayParam {
	p.minItems = &n
	return p
}

// GetMinItems returns the minimum items constraint, or nil if not set.
func (p *ArrayParam) GetMinItems() *int {
	return p.minItems
}

// MaxItems sets the maximum number of items constraint for the array.
// This generates CUE like: list.MaxItems(n)
func (p *ArrayParam) MaxItems(n int) *ArrayParam {
	p.maxItems = &n
	return p
}

// GetMaxItems returns the maximum items constraint, or nil if not set.
func (p *ArrayParam) GetMaxItems() *int {
	return p.maxItems
}

// --- ArrayParam Runtime Condition Methods ---

// LenEq creates a condition that checks if this array has exactly n elements.
// Example: tags.LenEq(5) generates: len(parameter.tags) == 5
func (p *ArrayParam) LenEq(n int) Condition {
	return &LenCondition{paramName: p.name, op: "==", length: n}
}

// LenGt creates a condition that checks if this array has more than n elements.
// Example: tags.LenGt(0) generates: len(parameter.tags) > 0
func (p *ArrayParam) LenGt(n int) Condition {
	return &LenCondition{paramName: p.name, op: ">", length: n}
}

// LenGte creates a condition that checks if this array has n or more elements.
// Example: tags.LenGte(1) generates: len(parameter.tags) >= 1
func (p *ArrayParam) LenGte(n int) Condition {
	return &LenCondition{paramName: p.name, op: ">=", length: n}
}

// LenLt creates a condition that checks if this array has fewer than n elements.
// Example: tags.LenLt(10) generates: len(parameter.tags) < 10
func (p *ArrayParam) LenLt(n int) Condition {
	return &LenCondition{paramName: p.name, op: "<", length: n}
}

// LenLte creates a condition that checks if this array has n or fewer elements.
// Example: tags.LenLte(10) generates: len(parameter.tags) <= 10
func (p *ArrayParam) LenLte(n int) Condition {
	return &LenCondition{paramName: p.name, op: "<=", length: n}
}

// Contains creates a condition that checks if this array contains a specific value.
// Example: tags.Contains("gpu") generates: list.Contains(parameter.tags, "gpu")
func (p *ArrayParam) Contains(val any) Condition {
	return &ArrayContainsCondition{paramName: p.name, value: val}
}

// IsEmpty creates a condition that checks if this array is empty.
// Example: tags.IsEmpty() generates: len(parameter.tags) == 0
func (p *ArrayParam) IsEmpty() Condition {
	return &LenCondition{paramName: p.name, op: "==", length: 0}
}

// IsNotEmpty creates a condition that checks if this array is not empty.
// Example: tags.IsNotEmpty() generates: len(parameter.tags) > 0
func (p *ArrayParam) IsNotEmpty() Condition {
	return &LenCondition{paramName: p.name, op: ">", length: 0}
}

// MapParam represents a map/dictionary parameter.
type MapParam struct {
	baseParam
	keyType   ParamType
	valueType ParamType
	fields    []Param // fields for structured map values
	schema    string  // raw CUE schema for the map structure
	schemaRef string  // reference to a helper definition (e.g., "HealthProbe")
}

// Map creates a new map parameter with the given name.
func Map(name string) *MapParam {
	return &MapParam{
		baseParam: baseParam{
			name:      name,
			paramType: ParamTypeMap,
		},
		keyType: ParamTypeString, // default key type
	}
}

// Of specifies the value type for the map.
func (p *MapParam) Of(valueType ParamType) *MapParam {
	p.valueType = valueType
	return p
}

// Required marks the parameter as required.
func (p *MapParam) Required() *MapParam {
	p.required = true
	return p
}

// Optional marks the parameter as optional (default behavior).
func (p *MapParam) Optional() *MapParam {
	p.required = false
	return p
}

// Default sets a default value for the parameter.
func (p *MapParam) Default(value map[string]any) *MapParam {
	p.defaultValue = value
	return p
}

// Description sets the parameter description.
func (p *MapParam) Description(desc string) *MapParam {
	p.description = desc
	return p
}

// ValueType returns the map value type.
func (p *MapParam) ValueType() ParamType {
	return p.valueType
}

// WithFields adds field definitions for structured map values.
// This allows defining the schema for objects within the map.
func (p *MapParam) WithFields(fields ...Param) *MapParam {
	p.fields = append(p.fields, fields...)
	return p
}

// GetFields returns the field definitions for map values.
func (p *MapParam) GetFields() []Param {
	return p.fields
}

// WithSchema sets a raw CUE schema for the map structure.
// This takes precedence over WithFields for schema generation.
func (p *MapParam) WithSchema(schema string) *MapParam {
	p.schema = schema
	return p
}

// GetSchema returns the raw CUE schema for the map.
func (p *MapParam) GetSchema() string {
	return p.schema
}

// WithSchemaRef sets a reference to a helper type definition (e.g., "#HealthProbe").
// This is used when the schema is defined elsewhere as a helper definition.
func (p *MapParam) WithSchemaRef(ref string) *MapParam {
	p.schemaRef = ref
	return p
}

// GetSchemaRef returns the schema reference for this parameter.
func (p *MapParam) GetSchemaRef() string {
	return p.schemaRef
}

// Field returns a reference to a nested field within this map parameter.
// This allows map parameters to be used as variables with field access.
// Example: requests := Map("requests").WithSchema(...); requests.Field("cpu") => parameter.requests.cpu
func (p *MapParam) Field(fieldPath string) *ParamFieldRef {
	return &ParamFieldRef{paramName: p.name, fieldPath: fieldPath}
}

// --- MapParam Runtime Condition Methods ---

// HasKey creates a condition that checks if this map has a specific key.
// Example: config.HasKey("debug") generates: parameter.config.debug != _|_
func (p *MapParam) HasKey(key string) Condition {
	return &MapHasKeyCondition{paramName: p.name, key: key}
}

// LenEq creates a condition that checks if this map has exactly n entries.
// Example: config.LenEq(5) generates: len(parameter.config) == 5
func (p *MapParam) LenEq(n int) Condition {
	return &LenCondition{paramName: p.name, op: "==", length: n}
}

// LenGt creates a condition that checks if this map has more than n entries.
// Example: config.LenGt(0) generates: len(parameter.config) > 0
func (p *MapParam) LenGt(n int) Condition {
	return &LenCondition{paramName: p.name, op: ">", length: n}
}

// IsEmpty creates a condition that checks if this map is empty.
// Example: config.IsEmpty() generates: len(parameter.config) == 0
func (p *MapParam) IsEmpty() Condition {
	return &LenCondition{paramName: p.name, op: "==", length: 0}
}

// IsNotEmpty creates a condition that checks if this map is not empty.
// Example: config.IsNotEmpty() generates: len(parameter.config) > 0
func (p *MapParam) IsNotEmpty() Condition {
	return &LenCondition{paramName: p.name, op: ">", length: 0}
}

// StructField represents a field within a struct parameter.
type StructField struct {
	name         string
	fieldType    ParamType
	required     bool
	defaultValue any
	description  string
	nested       *StructParam // for nested structs
	schemaRef    string       // reference to a helper definition (e.g., "HealthProbe")
	enumValues   []string     // allowed enum values for string fields
	elementType  ParamType    // for array fields: element type (e.g., ParamTypeString for [...string])
}

// Field creates a new struct field definition.
func Field(name string, fieldType ParamType) *StructField {
	return &StructField{
		name:      name,
		fieldType: fieldType,
	}
}

// Required marks the field as required.
func (f *StructField) Required() *StructField {
	f.required = true
	return f
}

// Optional marks the field as optional (default behavior).
func (f *StructField) Optional() *StructField {
	f.required = false
	return f
}

// Default sets a default value for the field.
func (f *StructField) Default(value any) *StructField {
	f.defaultValue = value
	return f
}

// Description sets the field description.
func (f *StructField) Description(desc string) *StructField {
	f.description = desc
	return f
}

// Nested sets a nested struct definition for this field.
// For ParamTypeStruct fields, this defines the struct's shape.
// For ParamTypeArray fields, this defines the array element struct shape (generates [...{fields}]).
func (f *StructField) Nested(s *StructParam) *StructField {
	f.nested = s
	if f.fieldType == ParamTypeArray {
		// Preserve array type; this indicates an array of nested structs.
		if f.elementType == "" {
			f.elementType = ParamTypeStruct
		}
	} else {
		f.fieldType = ParamTypeStruct
	}
	return f
}

// Name returns the field name.
func (f *StructField) Name() string { return f.name }

// FieldType returns the field type.
func (f *StructField) FieldType() ParamType { return f.fieldType }

// IsRequired returns true if the field is required.
func (f *StructField) IsRequired() bool { return f.required }

// HasDefault returns true if the field has a default value.
func (f *StructField) HasDefault() bool { return f.defaultValue != nil }

// GetDefault returns the default value.
func (f *StructField) GetDefault() any { return f.defaultValue }

// GetDescription returns the field description.
func (f *StructField) GetDescription() string { return f.description }

// GetNested returns the nested struct definition, if any.
func (f *StructField) GetNested() *StructParam { return f.nested }

// WithSchemaRef sets a reference to a helper type definition (e.g., "#RuleSelector").
// This is used when the field type is defined elsewhere as a helper definition.
func (f *StructField) WithSchemaRef(ref string) *StructField {
	f.schemaRef = ref
	return f
}

// GetSchemaRef returns the schema reference for this field.
func (f *StructField) GetSchemaRef() string { return f.schemaRef }

// Enum restricts the field to specific allowed values.
// This is only meaningful for string fields.
func (f *StructField) Enum(values ...string) *StructField {
	f.enumValues = values
	return f
}

// GetEnumValues returns the allowed enum values.
func (f *StructField) GetEnumValues() []string { return f.enumValues }

// ArrayOf sets the element type for array fields (e.g., ParamTypeString for [...string]).
func (f *StructField) ArrayOf(elemType ParamType) *StructField {
	f.elementType = elemType
	return f
}

// GetElementType returns the element type for array fields.
func (f *StructField) GetElementType() ParamType { return f.elementType }

// StructParam represents a structured parameter with named fields.
type StructParam struct {
	baseParam
	fields    []*StructField
	schemaRef string // reference to a helper definition (e.g., "HealthProbe")
}

// Struct creates a new struct parameter with the given name.
func Struct(name string) *StructParam {
	return &StructParam{
		baseParam: baseParam{
			name:      name,
			paramType: ParamTypeStruct,
		},
		fields: make([]*StructField, 0),
	}
}

// Fields adds field definitions to the struct.
func (p *StructParam) Fields(fields ...*StructField) *StructParam {
	p.fields = append(p.fields, fields...)
	return p
}

// Required marks the parameter as required.
func (p *StructParam) Required() *StructParam {
	p.required = true
	return p
}

// Optional marks the parameter as optional (default behavior).
func (p *StructParam) Optional() *StructParam {
	p.required = false
	return p
}

// Description sets the parameter description.
func (p *StructParam) Description(desc string) *StructParam {
	p.description = desc
	return p
}

// GetFields returns all field definitions.
func (p *StructParam) GetFields() []*StructField {
	return p.fields
}

// GetField returns a field by name, or nil if not found.
func (p *StructParam) GetField(name string) *StructField {
	for _, f := range p.fields {
		if f.name == name {
			return f
		}
	}
	return nil
}

// WithSchemaRef sets a reference to a helper type definition (e.g., "#RuleSelector").
// This is used when the struct type is defined elsewhere as a helper definition.
func (p *StructParam) WithSchemaRef(ref string) *StructParam {
	p.schemaRef = ref
	return p
}

// GetSchemaRef returns the schema reference for this struct.
func (p *StructParam) GetSchemaRef() string { return p.schemaRef }

// Field returns a reference to a nested field within this struct parameter.
// This allows struct parameters to be used as variables with field access.
// Example: config := Struct("config").Fields(...); config.Field("port") => parameter.config.port
func (p *StructParam) Field(fieldPath string) *ParamFieldRef {
	return &ParamFieldRef{paramName: p.name, fieldPath: fieldPath}
}

// EnumParam represents an enumeration parameter with allowed values.
type EnumParam struct {
	baseParam
	values []string
}

// Enum creates a new enum parameter with the given name.
func Enum(name string) *EnumParam {
	return &EnumParam{
		baseParam: baseParam{
			name:      name,
			paramType: ParamTypeEnum,
		},
		values: make([]string, 0),
	}
}

// Values sets the allowed enum values.
func (p *EnumParam) Values(values ...string) *EnumParam {
	p.values = values
	return p
}

// Required marks the parameter as required.
func (p *EnumParam) Required() *EnumParam {
	p.required = true
	return p
}

// Optional marks the parameter as optional (default behavior).
func (p *EnumParam) Optional() *EnumParam {
	p.required = false
	return p
}

// Default sets a default value for the parameter.
func (p *EnumParam) Default(value string) *EnumParam {
	p.defaultValue = value
	return p
}

// Description sets the parameter description.
func (p *EnumParam) Description(desc string) *EnumParam {
	p.description = desc
	return p
}

// Short sets a short flag alias for the parameter.
// This generates a // +short=X directive in the CUE output.
func (p *EnumParam) Short(s string) *EnumParam {
	p.short = s
	return p
}

// Ignore marks the parameter as ignored by the UI.
// This generates a // +ignore directive in the CUE output.
func (p *EnumParam) Ignore() *EnumParam {
	p.ignore = true
	return p
}

// GetValues returns the allowed enum values.
func (p *EnumParam) GetValues() []string {
	return p.values
}

// OneOfVariant represents a variant in a discriminated union.
type OneOfVariant struct {
	name   string
	fields []*StructField
}

// Variant creates a new variant for a OneOf parameter.
func Variant(name string) *OneOfVariant {
	return &OneOfVariant{
		name:   name,
		fields: make([]*StructField, 0),
	}
}

// Fields adds field definitions to the variant.
func (v *OneOfVariant) Fields(fields ...*StructField) *OneOfVariant {
	v.fields = append(v.fields, fields...)
	return v
}

// Name returns the variant name.
func (v *OneOfVariant) Name() string { return v.name }

// GetFields returns the variant's field definitions.
func (v *OneOfVariant) GetFields() []*StructField { return v.fields }

// OneOfParam represents a discriminated union parameter.
type OneOfParam struct {
	baseParam
	discriminator string
	variants      []*OneOfVariant
}

// OneOf creates a new discriminated union parameter with the given name.
func OneOf(name string) *OneOfParam {
	return &OneOfParam{
		baseParam: baseParam{
			name:      name,
			paramType: ParamTypeOneOf,
		},
		discriminator: "type", // default discriminator field
		variants:      make([]*OneOfVariant, 0),
	}
}

// Discriminator sets the field name used to distinguish variants.
func (p *OneOfParam) Discriminator(field string) *OneOfParam {
	p.discriminator = field
	return p
}

// Variants adds variant definitions to the union.
func (p *OneOfParam) Variants(variants ...*OneOfVariant) *OneOfParam {
	p.variants = append(p.variants, variants...)
	return p
}

// Default sets the default variant name for the discriminator.
func (p *OneOfParam) Default(value string) *OneOfParam {
	p.defaultValue = value
	return p
}

// Required marks the parameter as required.
func (p *OneOfParam) Required() *OneOfParam {
	p.required = true
	return p
}

// Optional marks the parameter as optional (default behavior).
func (p *OneOfParam) Optional() *OneOfParam {
	p.required = false
	return p
}

// Description sets the parameter description.
func (p *OneOfParam) Description(desc string) *OneOfParam {
	p.description = desc
	return p
}

// GetDiscriminator returns the discriminator field name.
func (p *OneOfParam) GetDiscriminator() string {
	return p.discriminator
}

// GetVariants returns all variant definitions.
func (p *OneOfParam) GetVariants() []*OneOfVariant {
	return p.variants
}

// GetVariant returns a variant by name, or nil if not found.
func (p *OneOfParam) GetVariant(name string) *OneOfVariant {
	for _, v := range p.variants {
		if v.name == name {
			return v
		}
	}
	return nil
}

// Convenience functions for common parameter patterns

// StringList creates a string array parameter.
func StringList(name string) *ArrayParam {
	return Array(name).Of(ParamTypeString)
}

// IntList creates an integer array parameter.
func IntList(name string) *ArrayParam {
	return Array(name).Of(ParamTypeInt)
}

// List creates a generic list parameter (array of any).
func List(name string) *ArrayParam {
	return Array(name)
}

// Object creates a generic object/struct parameter.
// This is useful when the structure is complex or dynamic.
func Object(name string) *MapParam {
	return Map(name)
}

// StringKeyMapParam represents a map with string keys and string values.
// In CUE: [string]: string
type StringKeyMapParam struct {
	baseParam
}

// StringKeyMap creates a new string-to-string map parameter.
// Generates CUE like: labels?: [string]: string
func StringKeyMap(name string) *StringKeyMapParam {
	return &StringKeyMapParam{
		baseParam: baseParam{
			name:      name,
			paramType: ParamTypeMap,
		},
	}
}

// Required marks the parameter as required.
func (p *StringKeyMapParam) Required() *StringKeyMapParam {
	p.required = true
	return p
}

// Optional marks the parameter as optional (default behavior).
func (p *StringKeyMapParam) Optional() *StringKeyMapParam {
	p.required = false
	return p
}

// Default sets a default value for the parameter.
func (p *StringKeyMapParam) Default(value map[string]string) *StringKeyMapParam {
	p.defaultValue = value
	return p
}

// Description sets the parameter description.
func (p *StringKeyMapParam) Description(desc string) *StringKeyMapParam {
	p.description = desc
	return p
}

// GetType returns the parameter type.
func (p *StringKeyMapParam) GetType() ParamType { return p.paramType }

// DynamicMapParam represents a parameter where the parameter itself is a dynamic map.
// In CUE: parameter: [string]: T (where T is the value type)
// This is used for traits like labels where all user values become map keys.
type DynamicMapParam struct {
	baseParam
	valueType      ParamType // type of values in the map
	valueTypeUnion string    // for union types like "string | null"
}

// DynamicMap creates a new dynamic map parameter.
// This is used when the entire parameter schema is a map with dynamic keys.
// Generates CUE like: parameter: [string]: string | null
//
// Example usage:
//
//	defkit.DynamicMap().ValueType(defkit.ParamTypeString)  // parameter: [string]: string
//	defkit.DynamicMap().ValueTypeUnion("string | null")   // parameter: [string]: string | null
func DynamicMap() *DynamicMapParam {
	return &DynamicMapParam{
		baseParam: baseParam{
			name:      "", // Dynamic maps don't have a field name
			paramType: ParamTypeMap,
		},
		valueType: ParamTypeString, // default to string values
	}
}

// ValueType sets the value type for the dynamic map.
func (p *DynamicMapParam) ValueType(t ParamType) *DynamicMapParam {
	p.valueType = t
	return p
}

// ValueTypeUnion sets a union type for the values (e.g., "string | null").
func (p *DynamicMapParam) ValueTypeUnion(union string) *DynamicMapParam {
	p.valueTypeUnion = union
	return p
}

// Description sets the parameter description.
func (p *DynamicMapParam) Description(desc string) *DynamicMapParam {
	p.description = desc
	return p
}

// GetValueType returns the value type for this dynamic map.
func (p *DynamicMapParam) GetValueType() ParamType {
	return p.valueType
}

// GetValueTypeUnion returns the union type string if set.
func (p *DynamicMapParam) GetValueTypeUnion() string {
	return p.valueTypeUnion
}

// IsDynamicMap returns true (used for type detection).
func (p *DynamicMapParam) IsDynamicMap() bool {
	return true
}

// --- Parameter Path Reference ---

// ParamPathRef represents a reference to a nested parameter path.
// This is used to reference nested parameter values like "podAffinity.required".
// It generates CUE like `parameter.podAffinity.required`.
type ParamPathRef struct {
	path string // e.g., "podAffinity.required"
}

func (p *ParamPathRef) expr()  {}
func (p *ParamPathRef) value() {}

// Path returns the parameter path.
func (p *ParamPathRef) Path() string { return p.path }

// IsSet returns a condition that checks if this parameter path has a value.
// This is used with SetIf for conditional field assignment.
// Generates: if parameter.path != _|_
func (p *ParamPathRef) IsSet() Condition {
	return &ParamPathIsSetCondition{path: p.path}
}

// ParamPath creates a reference to a nested parameter path.
// This is used for accessing nested parameters in conditions and expressions.
//
// Usage:
//
//	defkit.ParamPath("podAffinity.required").IsSet() // generates: if parameter.podAffinity.required != _|_
//	defkit.From(defkit.ParamPath("podAffinity.required")).Map(...) // generates: [for v in parameter.podAffinity.required {...}]
func ParamPath(path string) *ParamPathRef {
	return &ParamPathRef{path: path}
}

// ParamPathIsSetCondition checks if a parameter path has a value.
type ParamPathIsSetCondition struct {
	baseCondition
	path string
}

// Path returns the parameter path being checked.
func (c *ParamPathIsSetCondition) Path() string { return c.path }

// --- Open Struct Parameter ---

// OpenStructParam represents an open struct parameter that accepts any fields.
// This generates CUE like: parameter: {...}
// Used for json-patch and json-merge-patch traits where the entire parameter
// is passed through as the patch content.
type OpenStructParam struct {
	baseParam
	isOpen bool // always true for this type
}

// OpenStruct creates a new open struct parameter.
// This is used when the parameter schema should accept any fields.
// Generates CUE like: parameter: {...}
//
// Example usage:
//
//	defkit.OpenStruct() // generates: parameter: {...}
func OpenStruct() *OpenStructParam {
	return &OpenStructParam{
		isOpen: true,
	}
}

// Description sets the parameter description.
func (p *OpenStructParam) Description(desc string) *OpenStructParam {
	p.description = desc
	return p
}

// IsOpen returns true (used for type detection).
func (p *OpenStructParam) IsOpen() bool {
	return true
}

// GetName returns an empty string (open structs don't have names).
func (p *OpenStructParam) GetName() string { return "" }

// GetDescription returns the parameter description.
func (p *OpenStructParam) GetDescription() string { return p.description }

// GetType returns the parameter type.
func (p *OpenStructParam) GetType() ParamType { return ParamTypeStruct }

// IsRequired returns false (open structs are inherently optional).
func (p *OpenStructParam) IsRequired() bool { return false }

// GetDefault returns nil (open structs don't have defaults).
func (p *OpenStructParam) GetDefault() any { return nil }

// --- Open Array Parameter ---

// OpenArrayParam represents an open array parameter that accepts any elements.
// This generates CUE like: operations: [...{...}]
// Used for json-patch trait where operations array accepts any operations.
type OpenArrayParam struct {
	baseParam
}

// OpenArray creates a new open array parameter.
// This is used when the parameter schema should accept any array elements.
// Generates CUE like: name: [...{...}]
//
// Example usage:
//
//	defkit.OpenArray("operations") // generates: operations: [...{...}]
func OpenArray(name string) *OpenArrayParam {
	return &OpenArrayParam{
		baseParam: baseParam{name: name},
	}
}

// Description sets the parameter description.
func (p *OpenArrayParam) Description(desc string) *OpenArrayParam {
	p.description = desc
	return p
}

// GetDescription returns the parameter description.
func (p *OpenArrayParam) GetDescription() string { return p.description }

// GetType returns the parameter type.
func (p *OpenArrayParam) GetType() ParamType { return ParamTypeArray }

// IsRequired returns false (open arrays are inherently optional).
func (p *OpenArrayParam) IsRequired() bool { return false }

// GetDefault returns nil (open arrays don't have defaults).
func (p *OpenArrayParam) GetDefault() any { return nil }

// --- Parameter Expression Types ---

// ParamArithExpr represents an arithmetic expression on a parameter.
// Example: parameter.replicas + 1
type ParamArithExpr struct {
	paramName string
	op        string
	arithVal  any
}

func (e *ParamArithExpr) expr()  {}
func (e *ParamArithExpr) value() {}

// ParamName returns the parameter name.
func (e *ParamArithExpr) ParamName() string { return e.paramName }

// Op returns the arithmetic operator.
func (e *ParamArithExpr) Op() string { return e.op }

// ArithValue returns the value for the arithmetic operation.
func (e *ParamArithExpr) ArithValue() any { return e.arithVal }

// ParamConcatExpr represents a string concatenation expression on a parameter.
// Example: parameter.name + "-suffix"
type ParamConcatExpr struct {
	paramName string
	suffix    string
	prefix    string
}

func (e *ParamConcatExpr) expr()  {}
func (e *ParamConcatExpr) value() {}

// ParamName returns the parameter name.
func (e *ParamConcatExpr) ParamName() string { return e.paramName }

// Suffix returns the suffix to append.
func (e *ParamConcatExpr) Suffix() string { return e.suffix }

// Prefix returns the prefix to prepend.
func (e *ParamConcatExpr) Prefix() string { return e.prefix }

// ParamFieldRef represents a reference to a field within a struct parameter.
// Example: parameter.config.name
type ParamFieldRef struct {
	paramName string
	fieldPath string
}

func (r *ParamFieldRef) expr()      {}
func (r *ParamFieldRef) value()     {}
func (r *ParamFieldRef) condition() {}

// ParamName returns the parameter name.
func (r *ParamFieldRef) ParamName() string { return r.paramName }

// FieldPath returns the field path within the struct.
func (r *ParamFieldRef) FieldPath() string { return r.fieldPath }

// IsSet returns a condition that checks if this field is set.
func (r *ParamFieldRef) IsSet() Condition {
	return &ParamPathIsSetCondition{path: r.paramName + "." + r.fieldPath}
}

// Eq creates a condition that compares this field to a value.
func (r *ParamFieldRef) Eq(val any) Condition {
	return &ParamCompareCondition{paramName: r.paramName + "." + r.fieldPath, op: "==", value: val}
}

// Ne creates a condition that checks inequality.
func (r *ParamFieldRef) Ne(val any) Condition {
	return &ParamCompareCondition{paramName: r.paramName + "." + r.fieldPath, op: "!=", value: val}
}
