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

// PatchResource represents a patch being built for traits.
// It uses the same fluent API as Resource but generates a patch: block.
type PatchResource struct {
	ops       []ResourceOp
	currentIf *IfBlock // tracks current If block being built
}

// NewPatchResource creates a new patch resource builder.
func NewPatchResource() *PatchResource {
	return &PatchResource{
		ops: make([]ResourceOp, 0),
	}
}

// Set records a field assignment in the patch.
// Example: p.Set("spec.replicas", replicas)
func (p *PatchResource) Set(path string, value Value) *PatchResource {
	op := &SetOp{path: path, value: value}
	if p.currentIf != nil {
		p.currentIf.ops = append(p.currentIf.ops, op)
	} else {
		p.ops = append(p.ops, op)
	}
	return p
}

// SetIf records a conditional field assignment in the patch.
// Example: p.SetIf(cpu.IsSet(), "spec.resources.limits.cpu", cpu)
func (p *PatchResource) SetIf(cond Condition, path string, value Value) *PatchResource {
	op := &SetIfOp{path: path, value: value, cond: cond}
	if p.currentIf != nil {
		p.currentIf.ops = append(p.currentIf.ops, op)
	} else {
		p.ops = append(p.ops, op)
	}
	return p
}

// SpreadIf records a conditional spread operation inside a struct block in the patch.
// Example: p.SpreadIf(labels.IsSet(), "metadata.labels", labels)
func (p *PatchResource) SpreadIf(cond Condition, path string, value Value) *PatchResource {
	op := &SpreadIfOp{path: path, value: value, cond: cond}
	if p.currentIf != nil {
		p.currentIf.ops = append(p.currentIf.ops, op)
	} else {
		p.ops = append(p.ops, op)
	}
	return p
}

// ForEach adds a for-each spread operation to the patch.
// This generates: for k, v in parameter { (k): v }
// Used for traits like labels that spread map keys dynamically.
//
// Example:
//
//	tpl.Patch().ForEach(labels, "metadata.labels")
//	// Generates: metadata: labels: { for k, v in parameter { (k): v } }
func (p *PatchResource) ForEach(source Value, path string) *PatchResource {
	op := &ForEachOp{path: path, source: source}
	if p.currentIf != nil {
		p.currentIf.ops = append(p.currentIf.ops, op)
	} else {
		p.ops = append(p.ops, op)
	}
	return p
}

// If starts a conditional block. Operations until EndIf are conditional.
func (p *PatchResource) If(cond Condition) *PatchResource {
	p.currentIf = &IfBlock{
		cond: cond,
		ops:  make([]ResourceOp, 0),
	}
	return p
}

// EndIf ends the current conditional block.
func (p *PatchResource) EndIf() *PatchResource {
	if p.currentIf != nil {
		p.ops = append(p.ops, p.currentIf)
		p.currentIf = nil
	}
	return p
}

// PatchKey adds an array patch with a merge key annotation.
// This generates: // +patchKey=key
//
//	path: [element1, element2, ...]
//
// Used for merging arrays by key (e.g., containers by name).
//
// Example:
//
//	tpl.Patch().PatchKey("spec.template.spec.containers", "name", container)
func (p *PatchResource) PatchKey(path string, key string, elements ...Value) *PatchResource {
	op := &PatchKeyOp{path: path, key: key, elements: elements}
	if p.currentIf != nil {
		p.currentIf.ops = append(p.currentIf.ops, op)
	} else {
		p.ops = append(p.ops, op)
	}
	return p
}

// SpreadAll adds a spread constraint that applies to all array elements.
// This generates: path: [...{element1}, ...{element2}]
// Used for applying the same patch to every element in an array.
//
// Example:
//
//	lifecycleObj := defkit.NewArrayElement().
//	    SetIf(postStart.IsSet(), "lifecycle.postStart", postStart)
//	tpl.Patch().SpreadAll("spec.template.spec.containers", lifecycleObj)
//	// Generates: containers: [...{lifecycle: { if ... { postStart: ... } }}]
func (p *PatchResource) SpreadAll(path string, elements ...Value) *PatchResource {
	op := &SpreadAllOp{path: path, elements: elements}
	if p.currentIf != nil {
		p.currentIf.ops = append(p.currentIf.ops, op)
	} else {
		p.ops = append(p.ops, op)
	}
	return p
}

// PatchStrategyAnnotation annotates a specific field path with // +patchStrategy=strategy.
// This generates a CUE comment annotation before the field.
// Example: p.PatchStrategyAnnotation("spec.strategy", "retainKeys")
// Generates: // +patchStrategy=retainKeys
//
//	strategy: { ... }
func (p *PatchResource) PatchStrategyAnnotation(path string, strategy string) *PatchResource {
	op := &PatchStrategyAnnotationOp{path: path, strategy: strategy}
	if p.currentIf != nil {
		p.currentIf.ops = append(p.currentIf.ops, op)
	} else {
		p.ops = append(p.ops, op)
	}
	return p
}

// Ops returns all recorded operations.
func (p *PatchResource) Ops() []ResourceOp { return p.ops }

// Passthrough sets the patch to pass through the entire parameter.
// This generates CUE like: patch: parameter
// Used for json-patch and json-merge-patch traits where the parameter IS the patch.
func (p *PatchResource) Passthrough() *PatchResource {
	p.ops = append(p.ops, &PassthroughOp{})
	return p
}

// PassthroughOp represents a passthrough operation where the parameter becomes the patch.
type PassthroughOp struct{}

func (p *PassthroughOp) resourceOp() {}

// PatchStrategyAnnotationOp records a patchStrategy annotation on a field path.
type PatchStrategyAnnotationOp struct {
	path     string
	strategy string
}

func (p *PatchStrategyAnnotationOp) resourceOp() {}

// Path returns the path being annotated.
func (p *PatchStrategyAnnotationOp) Path() string { return p.path }

// Strategy returns the patch strategy value.
func (p *PatchStrategyAnnotationOp) Strategy() string { return p.strategy }

// ForEachOp represents a for-each spread operation in a patch.
// This generates CUE like: for k, v in source { (k): v }
type ForEachOp struct {
	path   string
	source Value
}

func (f *ForEachOp) resourceOp() {}

// Path returns the parent path for the for-each operation.
func (f *ForEachOp) Path() string { return f.path }

// Source returns the source value to iterate over.
func (f *ForEachOp) Source() Value { return f.source }

// --- Context output introspection for traits ---

// ContextOutputRef represents a reference to context.output for trait introspection.
// Traits can use this to conditionally apply patches based on the workload's structure.
type ContextOutputRef struct {
	path string
}

// ContextOutput returns a reference to the component's output (context.output).
// Use this in traits to introspect the workload being modified.
//
// Example:
//
//	// Check if workload has spec.template (like Deployment)
//	hasTemplate := ContextOutput().HasPath("spec.template")
//	tpl.Patch().
//	    If(hasTemplate).
//	        Set("spec.template.metadata.labels", labels).
//	    EndIf()
func ContextOutput() *ContextOutputRef {
	return &ContextOutputRef{path: "context.output"}
}

func (c *ContextOutputRef) value() {}
func (c *ContextOutputRef) expr()  {}

// Path returns the full CUE path for this reference.
func (c *ContextOutputRef) Path() string { return c.path }

// Field returns a reference to a specific field in context.output.
// Example: ContextOutput().Field("spec.template.spec.containers")
func (c *ContextOutputRef) Field(path string) *ContextOutputRef {
	return &ContextOutputRef{path: c.path + "." + path}
}

// HasPath returns a condition that checks if a path exists in context.output.
// This generates CUE: context.output.path != _|_
//
// Example:
//
//	hasTemplate := ContextOutput().HasPath("spec.template")
func (c *ContextOutputRef) HasPath(path string) Condition {
	return &ContextPathExistsCondition{basePath: c.path, fieldPath: path}
}

// IsSet returns a condition that checks if this context path exists.
func (c *ContextOutputRef) IsSet() Condition {
	return &ContextPathExistsCondition{basePath: "", fieldPath: c.path}
}

// ContextPathExistsCondition checks if a path exists in context.output.
type ContextPathExistsCondition struct {
	baseCondition
	basePath  string
	fieldPath string
}

// BasePath returns the base path (e.g., "context.output").
func (c *ContextPathExistsCondition) BasePath() string { return c.basePath }

// FieldPath returns the field path being checked.
func (c *ContextPathExistsCondition) FieldPath() string { return c.fieldPath }

// FullPath returns the complete path.
func (c *ContextPathExistsCondition) FullPath() string {
	if c.basePath == "" {
		return c.fieldPath
	}
	return c.basePath + "." + c.fieldPath
}
