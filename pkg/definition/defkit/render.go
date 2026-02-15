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

// Render executes the component template with the given test context
// and returns the rendered primary output resource.
func (c *ComponentDefinition) Render(ctx *TestContextBuilder) *RenderedResource {
	// Build the runtime context
	rtCtx := ctx.Build()

	// Set up the current test context for parameter resolution
	setCurrentTestContext(rtCtx)
	defer clearCurrentTestContext()

	// Create and execute template
	tpl := NewTemplate()
	if c.template != nil {
		c.template(tpl)
	}

	// Render the output resource with resolved values
	return renderResource(tpl.output, rtCtx)
}

// RenderAll executes the component template and returns all outputs.
func (c *ComponentDefinition) RenderAll(ctx *TestContextBuilder) *RenderedOutputs {
	rtCtx := ctx.Build()
	setCurrentTestContext(rtCtx)
	defer clearCurrentTestContext()

	tpl := NewTemplate()
	if c.template != nil {
		c.template(tpl)
	}

	outputs := &RenderedOutputs{
		Primary:   renderResource(tpl.output, rtCtx),
		Auxiliary: make(map[string]*RenderedResource),
	}

	for name, res := range tpl.outputs {
		// Check if the resource has an output condition
		if res.outputCondition != nil {
			if !evaluateCondition(res.outputCondition, rtCtx) {
				// Condition is false, skip this output
				continue
			}
		}
		outputs.Auxiliary[name] = renderResource(res, rtCtx)
	}

	return outputs
}

// RenderedOutputs contains all rendered resources from a template.
type RenderedOutputs struct {
	Primary   *RenderedResource
	Auxiliary map[string]*RenderedResource
}

// RenderedResource represents a fully rendered Kubernetes resource
// with all parameter values resolved.
type RenderedResource struct {
	apiVersion string
	kind       string
	data       map[string]any
}

// APIVersion returns the resource's API version.
func (r *RenderedResource) APIVersion() string {
	if r == nil {
		return ""
	}
	return r.apiVersion
}

// Kind returns the resource's kind.
func (r *RenderedResource) Kind() string {
	if r == nil {
		return ""
	}
	return r.kind
}

// Get retrieves a value at the given path (e.g., "spec.replicas").
func (r *RenderedResource) Get(path string) any {
	if r == nil || r.data == nil {
		return nil
	}
	return getNestedValue(r.data, path)
}

// Data returns the full rendered resource data.
func (r *RenderedResource) Data() map[string]any {
	if r == nil {
		return nil
	}
	return r.data
}

// renderResource converts a Resource with operations into a RenderedResource
// with all values resolved from the test context.
func renderResource(res *Resource, ctx *TestRuntimeContext) *RenderedResource {
	if res == nil {
		return nil
	}

	rendered := &RenderedResource{
		apiVersion: res.apiVersion,
		kind:       res.kind,
		data: map[string]any{
			"apiVersion": res.apiVersion,
			"kind":       res.kind,
			"metadata": map[string]any{
				"name": ctx.Name(),
			},
		},
	}

	// Process all operations
	for _, op := range res.ops {
		processOp(rendered.data, op, ctx)
	}

	return rendered
}

// processOp processes a single resource operation.
func processOp(data map[string]any, op ResourceOp, ctx *TestRuntimeContext) {
	switch o := op.(type) {
	case *SetOp:
		value := resolveValue(o.value, ctx)
		setNestedValue(data, o.path, value)

	case *SetIfOp:
		if evaluateCondition(o.cond, ctx) {
			value := resolveValue(o.value, ctx)
			setNestedValue(data, o.path, value)
		}

	case *IfBlock:
		if evaluateCondition(o.cond, ctx) {
			for _, innerOp := range o.ops {
				processOp(data, innerOp, ctx)
			}
		}
	}
}

// resolveValue resolves a Value to its actual value using the test context.
func resolveValue(v Value, ctx *TestRuntimeContext) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case *StringParam:
		return ctx.GetParamOr(val.Name(), val.GetDefault())
	case *IntParam:
		return ctx.GetParamOr(val.Name(), val.GetDefault())
	case *BoolParam:
		return ctx.GetParamOr(val.Name(), val.GetDefault())
	case *FloatParam:
		return ctx.GetParamOr(val.Name(), val.GetDefault())
	case *ArrayParam:
		return ctx.GetParamOr(val.Name(), val.GetDefault())
	case *MapParam:
		return ctx.GetParamOr(val.Name(), val.GetDefault())
	case *StructParam:
		return ctx.GetParamOr(val.Name(), val.GetDefault())
	case *EnumParam:
		return ctx.GetParamOr(val.Name(), val.GetDefault())
	case *ContextRef:
		return resolveContextRef(val, ctx)
	case *Literal:
		return val.Val()
	case *TransformedValue:
		// Resolve the source value, then apply the transformation
		sourceValue := resolveValue(val.source, ctx)
		if val.transform != nil {
			return val.transform(sourceValue)
		}
		return sourceValue
	case *CollectionOp:
		// Resolve source and apply collection operations
		sourceValue := resolveValue(val.source, ctx)
		return applyCollectionOps(sourceValue, val.ops)
	case *MultiSource:
		// Combine items from multiple fields and apply operations
		return resolveMultiSource(val, ctx)
	case *StringKeyMapParam:
		return ctx.GetParamOr(val.Name(), val.GetDefault())
	default:
		// For any Param interface, use method access
		if p, ok := v.(Param); ok {
			return ctx.GetParamOr(p.Name(), p.GetDefault())
		}
		return v
	}
}

// resolveContextRef resolves a context reference to its value.
func resolveContextRef(ref *ContextRef, ctx *TestRuntimeContext) any {
	switch ref.Path() {
	case "context.name":
		return ctx.Name()
	case "context.namespace":
		return ctx.Namespace()
	case "context.appName":
		return ctx.AppName()
	case "context.appRevision":
		return ctx.AppRevision()
	default:
		return ref.String()
	}
}

// evaluateCondition evaluates a Condition using the test context.
func evaluateCondition(cond Condition, ctx *TestRuntimeContext) bool {
	if cond == nil {
		return true
	}

	switch c := cond.(type) {
	case *IsSetCondition:
		return ctx.IsParamSet(c.paramName)
	case *CompareCondition:
		left := resolveConditionValue(c.left, ctx)
		right := resolveConditionValue(c.right, ctx)
		return compareValues(left, right, c.op)
	case *Comparison:
		left := resolveConditionValue(c.Left(), ctx)
		right := resolveConditionValue(c.Right(), ctx)
		return compareValues(left, right, string(c.Op()))
	case *AndCondition:
		return evaluateCondition(c.left, ctx) && evaluateCondition(c.right, ctx)
	case *OrCondition:
		return evaluateCondition(c.left, ctx) || evaluateCondition(c.right, ctx)
	case *NotCondition:
		return !evaluateCondition(c.inner, ctx)
	case *LogicalExpr:
		if c.Op() == OpAnd {
			for _, sub := range c.Conditions() {
				if !evaluateCondition(sub, ctx) {
					return false
				}
			}
			return true
		} else { // OpOr
			for _, sub := range c.Conditions() {
				if evaluateCondition(sub, ctx) {
					return true
				}
			}
			return false
		}
	case *NotExpr:
		return !evaluateCondition(c.Cond(), ctx)
	case *HasExposedPortsCondition:
		// Resolve the ports value and check if any have expose=true
		portsValue := resolveValue(c.ports, ctx)
		return hasExposedPorts(portsValue)
	default:
		// For parameter-based conditions (param used as condition)
		if v, ok := cond.(Value); ok {
			resolved := resolveValue(v, ctx)
			return resolved != nil
		}
		return true
	}
}

// hasExposedPorts checks if a ports array has any port with expose=true.
func hasExposedPorts(ports any) bool {
	portList, ok := ports.([]any)
	if !ok {
		return false
	}
	for _, p := range portList {
		if portMap, ok := p.(map[string]any); ok {
			if expose, ok := portMap["expose"].(bool); ok && expose {
				return true
			}
		}
	}
	return false
}

// resolveConditionValue resolves a value used in a condition.
func resolveConditionValue(v any, ctx *TestRuntimeContext) any {
	if val, ok := v.(Value); ok {
		return resolveValue(val, ctx)
	}
	return v
}

// compareValues compares two values with the given operator.
func compareValues(left, right any, op string) bool {
	switch op {
	case "==":
		return left == right
	case "!=":
		return left != right
	case "<":
		return compareNumeric(left, right) < 0
	case "<=":
		return compareNumeric(left, right) <= 0
	case ">":
		return compareNumeric(left, right) > 0
	case ">=":
		return compareNumeric(left, right) >= 0
	default:
		return false
	}
}

// compareNumeric compares two numeric values.
func compareNumeric(left, right any) int {
	l := toFloat64(left)
	r := toFloat64(right)
	if l < r {
		return -1
	}
	if l > r {
		return 1
	}
	return 0
}

// toFloat64 converts a value to float64 for comparison.
func toFloat64(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case float32:
		return float64(n)
	case float64:
		return n
	default:
		return 0
	}
}

// setNestedValue sets a value at a nested path in a map.
func setNestedValue(data map[string]any, path string, value any) {
	parts := splitPath(path)
	current := data

	for i, part := range parts[:len(parts)-1] {
		// Handle bracket notation: containers[0] (array) or labels[app.oam.dev/name] (map key)
		name, key, index := parseBracketAccess(part)

		switch {
		case index >= 0:
			// Array access
			arr, ok := current[name].([]any)
			if !ok {
				arr = make([]any, index+1)
				current[name] = arr
			}
			for len(arr) <= index {
				arr = append(arr, make(map[string]any)) //nolint:makezero // Only extends existing arrays; new arrays have len > index
				current[name] = arr
			}
			if m, ok := arr[index].(map[string]any); ok {
				current = m
			} else {
				m := make(map[string]any)
				arr[index] = m
				current[name] = arr
				current = m
			}
		case key != "":
			// Map key access like labels[app.oam.dev/name]
			if _, exists := current[name]; !exists {
				current[name] = make(map[string]any)
			}
			if m, ok := current[name].(map[string]any); ok {
				if _, exists := m[key]; !exists {
					m[key] = make(map[string]any)
				}
				if next, ok := m[key].(map[string]any); ok {
					current = next
				} else {
					// The key exists but is not a map - create nested structure
					newMap := make(map[string]any)
					m[key] = newMap
					current = newMap
				}
			}
		default:
			// Regular map access
			if _, exists := current[name]; !exists {
				current[name] = make(map[string]any)
			}
			if next, ok := current[name].(map[string]any); ok {
				current = next
			} else {
				// Path conflict - overwrite
				m := make(map[string]any)
				current[name] = m
				current = m
			}
		}
		_ = i // suppress unused warning
	}

	// Set the final value
	lastPart := parts[len(parts)-1]
	name, key, index := parseBracketAccess(lastPart)
	switch {
	case index >= 0:
		arr, ok := current[name].([]any)
		if !ok {
			arr = make([]any, index+1)
		}
		for len(arr) <= index {
			arr = append(arr, nil) //nolint:makezero // Only extends existing arrays; new arrays have len > index
		}
		arr[index] = value
		current[name] = arr
	case key != "":
		// Map key access like labels[app.oam.dev/name]
		if _, exists := current[name]; !exists {
			current[name] = make(map[string]any)
		}
		if m, ok := current[name].(map[string]any); ok {
			m[key] = value
		}
	default:
		current[name] = value
	}
}

// getNestedValue retrieves a value at a nested path.
func getNestedValue(data map[string]any, path string) any {
	parts := splitPath(path)
	current := any(data)

	for _, part := range parts {
		name, _, index := parseBracketAccess(part)

		switch c := current.(type) {
		case map[string]any:
			if index >= 0 {
				if arr, ok := c[name].([]any); ok && index < len(arr) {
					current = arr[index]
				} else {
					return nil
				}
			} else {
				var ok bool
				current, ok = c[name]
				if !ok {
					return nil
				}
			}
		default:
			return nil
		}
	}

	return current
}

// splitPath splits a dot-separated path.
func splitPath(path string) []string {
	var parts []string
	var current string
	bracketDepth := 0

	for _, c := range path {
		switch {
		case c == '[':
			bracketDepth++
			current += string(c)
		case c == ']':
			bracketDepth--
			current += string(c)
		case c == '.' && bracketDepth == 0:
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		default:
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// parseBracketAccess parses "name[key]" or "name[index]" and returns:
// - name: the field name before the bracket
// - key: the string key if it's a map access (empty string if array)
// - index: the numeric index if it's an array access (-1 if map access or no brackets)
func parseBracketAccess(part string) (name string, key string, index int) {
	for i, c := range part {
		if c == '[' {
			if part[len(part)-1] != ']' {
				return part, "", -1
			}
			name = part[:i]
			bracketContent := part[i+1 : len(part)-1]
			// Check if the content is numeric (array index)
			isNumeric := len(bracketContent) > 0
			for _, d := range bracketContent {
				if d < '0' || d > '9' {
					isNumeric = false
					break
				}
			}
			if !isNumeric {
				// This is a map key notation
				return name, bracketContent, -1
			}
			// Parse as array index
			idx := 0
			for _, d := range bracketContent {
				idx = idx*10 + int(d-'0')
			}
			return name, "", idx
		}
	}
	return part, "", -1
}

// applyCollectionOps applies a series of collection operations to a value.
func applyCollectionOps(source any, ops []collectionOperation) any {
	// Handle both []any and []map[string]any (Go doesn't automatically convert slices)
	var items []any
	switch v := source.(type) {
	case []any:
		items = v
	case []map[string]any:
		// Convert []map[string]any to []any
		items = make([]any, len(v))
		for i, m := range v {
			items[i] = m
		}
	default:
		return source
	}
	result := items
	for _, op := range ops {
		result = op.apply(result)
	}
	return result
}

// resolveMultiSource resolves a MultiSource by combining items from multiple fields.
func resolveMultiSource(ms *MultiSource, ctx *TestRuntimeContext) any {
	sourceValue := resolveValue(ms.source, ctx)
	sourceMap, ok := sourceValue.(map[string]any)
	if !ok {
		return []any{}
	}

	// Get per-source mappings if defined
	mapBySource := ms.MapBySourceMappings()

	// Collect all items from the specified source fields
	var allItems []any
	for _, field := range ms.sources {
		// Handle both []any and []map[string]any (Go doesn't automatically convert slices)
		var items []any
		switch v := sourceMap[field].(type) {
		case []any:
			items = v
		case []map[string]any:
			// Convert []map[string]any to []any
			items = make([]any, len(v))
			for i, m := range v {
				items[i] = m
			}
		default:
			continue
		}

		// If MapBySource is defined, apply the mapping for this source type
		if mapping, hasMapping := mapBySource[field]; hasMapping {
			for _, item := range items {
				if itemMap, ok := item.(map[string]any); ok {
					mappedItem := applyFieldMap(itemMap, mapping)
					allItems = append(allItems, mappedItem)
				}
			}
		} else {
			allItems = append(allItems, items...)
		}
	}

	// Apply operations
	result := allItems
	for _, op := range ms.ops {
		result = op.apply(result)
	}

	return result
}

// applyFieldMap applies a FieldMap to transform an item.
func applyFieldMap(item map[string]any, mapping FieldMap) map[string]any {
	result := make(map[string]any)
	for key, fieldVal := range mapping {
		resolved := fieldVal.resolve(item)
		if resolved != nil {
			result[key] = resolved
		}
	}
	return result
}
