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
	"regexp"
	"strings"

	"cuelang.org/go/cue/format"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/definition/defkit/placement"
)

// topLevelTemplateRegex matches a top-level "template:" block in CUE.
// It looks for "template:" at the start of a line (with optional leading whitespace)
// that is NOT preceded by a colon (which would indicate it's part of a path like "spec: template:").
var topLevelTemplateRegex = regexp.MustCompile(`(?m)^[^\S\n]*template:`)

// TraitDefinition represents a KubeVela TraitDefinition.
// Traits modify or enhance components by either patching the workload resource
// or creating additional auxiliary resources.
//
// TraitDefinition embeds baseDefinition for common fields and methods shared
// with ComponentDefinition and other definition types.
type TraitDefinition struct {
	baseDefinition                       // embedded common fields (name, description, params, template, etc.)
	appliesToWorkloads []string          // e.g., ["deployments.apps", "statefulsets.apps"]
	conflictsWith      []string          // traits that conflict with this one
	conflictsWithSet   bool              // true when ConflictsWith() was explicitly called
	podDisruptive      bool              // whether applying this trait causes pod restart
	stage              string            // "PreDispatch" or "PostDispatch" (default: "")
	templateBlock      string            // raw CUE for template: block only (uses fluent API for header)
	labels             map[string]string // metadata labels for the trait definition
	workloadRefPath    *string           // workloadRefPath attribute (nil = omit, pointer to distinguish from empty)
}

// NewTrait creates a new TraitDefinition builder.
func NewTrait(name string) *TraitDefinition {
	return &TraitDefinition{
		baseDefinition: baseDefinition{
			name:   name,
			params: make([]Param, 0),
		},
		appliesToWorkloads: make([]string, 0),
		conflictsWith:      make([]string, 0),
	}
}

// Description sets the trait description.
func (t *TraitDefinition) Description(desc string) *TraitDefinition {
	t.setDescription(desc)
	return t
}

// AppliesTo specifies which workload types this trait can be applied to.
// Common values: "deployments.apps", "statefulsets.apps", "daemonsets.apps"
func (t *TraitDefinition) AppliesTo(workloads ...string) *TraitDefinition {
	t.appliesToWorkloads = append(t.appliesToWorkloads, workloads...)
	return t
}

// ConflictsWith specifies traits that cannot be used together with this trait.
func (t *TraitDefinition) ConflictsWith(traits ...string) *TraitDefinition {
	t.conflictsWithSet = true
	t.conflictsWith = append(t.conflictsWith, traits...)
	return t
}

// PodDisruptive marks whether applying this trait causes pod restarts.
func (t *TraitDefinition) PodDisruptive(disruptive bool) *TraitDefinition {
	t.podDisruptive = disruptive
	return t
}

// WorkloadRefPath sets the workloadRefPath attribute for the trait.
func (t *TraitDefinition) WorkloadRefPath(path string) *TraitDefinition {
	t.workloadRefPath = &path
	return t
}

// Stage sets the trait application stage.
// Use "PreDispatch" for traits that must run before dispatch,
// "PostDispatch" for traits that run after (e.g., creating Services).
func (t *TraitDefinition) Stage(stage string) *TraitDefinition {
	t.stage = stage
	return t
}

// Params adds parameter definitions to the trait.
func (t *TraitDefinition) Params(params ...Param) *TraitDefinition {
	t.addParams(params...)
	return t
}

// Param adds a single parameter definition to the trait.
func (t *TraitDefinition) Param(param Param) *TraitDefinition {
	t.addParams(param)
	return t
}

// Template sets the unified template function for the trait.
// This is the preferred way to define trait behavior.
// Use tpl.Patch() for modifying the workload, and tpl.Outputs() for creating auxiliary resources.
//
// Example (patch-only):
//
//	defkit.NewTrait("scaler").
//	    Params(defkit.Int("replicas").Default(1)).
//	    Template(func(tpl *defkit.Template) {
//	        replicas := defkit.Int("replicas")
//	        tpl.PatchStrategy("retainKeys").
//	            Patch().Set("spec.replicas", replicas)
//	    })
//
// Example (outputs-only):
//
//	defkit.NewTrait("expose").
//	    Params(defkit.Array("ports")...).
//	    Template(func(tpl *defkit.Template) {
//	        service := defkit.NewResource("v1", "Service").
//	            Set("spec.type", "ClusterIP")
//	        tpl.Outputs("service", service)
//	    })
//
// Example (patch and outputs):
//
//	defkit.NewTrait("ingress").
//	    Template(func(tpl *defkit.Template) {
//	        // Patch the workload
//	        tpl.Patch().Set("metadata.annotations[ingress]", "enabled")
//	        // Create auxiliary resource
//	        ingress := defkit.NewResource("networking.k8s.io/v1", "Ingress")
//	        tpl.Outputs("ingress", ingress)
//	    })
func (t *TraitDefinition) Template(fn func(tpl *Template)) *TraitDefinition {
	t.setTemplate(fn)
	return t
}

// CustomStatus sets the custom status CUE expression for the trait.
func (t *TraitDefinition) CustomStatus(expr string) *TraitDefinition {
	t.setCustomStatus(expr)
	return t
}

// HealthPolicy sets the health policy CUE expression for the trait.
func (t *TraitDefinition) HealthPolicy(expr string) *TraitDefinition {
	t.setHealthPolicy(expr)
	return t
}

// HealthPolicyExpr sets the health policy using a composable HealthExpression.
func (t *TraitDefinition) HealthPolicyExpr(expr HealthExpression) *TraitDefinition {
	t.setHealthPolicyExpr(expr)
	return t
}

// RawCUE sets raw CUE for complex trait definitions that don't fit the builder pattern.
// When set, this bypasses all other template settings and outputs the raw CUE directly.
func (t *TraitDefinition) RawCUE(cue string) *TraitDefinition {
	t.setRawCUE(cue)
	return t
}

// TemplateBlock sets raw CUE for the template: block only.
// Unlike RawCUE which bypasses everything, TemplateBlock uses the fluent API for:
// - Trait header (name, type, description, annotations, labels)
// - Attributes (podDisruptive, appliesToWorkloads, conflictsWith, stage, status)
//
// But uses raw CUE for the template: block content (patch, outputs, parameter, helpers).
// This is useful when the template logic is too complex for the fluent API but you
// still want type-safe metadata and attributes.
//
// Example:
//
//	defkit.NewTrait("command").
//	    Description("Add command on K8s pod for your workload").
//	    AppliesTo("deployments.apps", "statefulsets.apps").
//	    TemplateBlock(`
//	        #PatchParams: {
//	            containerName: *"" | string
//	            command: *null | [...string]
//	        }
//	        PatchContainer: { ... }
//	        patch: spec: template: spec: { ... }
//	        parameter: #PatchParams
//	    `)
func (t *TraitDefinition) TemplateBlock(cue string) *TraitDefinition {
	t.templateBlock = cue
	return t
}

// GetTemplateBlock returns the raw CUE template block if set.
func (t *TraitDefinition) GetTemplateBlock() string { return t.templateBlock }

// HasTemplateBlock returns true if the trait has a raw CUE template block.
func (t *TraitDefinition) HasTemplateBlock() bool { return t.templateBlock != "" }

// WithImports adds CUE imports to the trait definition.
// Usage: trait.WithImports("strconv", "strings")
func (t *TraitDefinition) WithImports(imports ...string) *TraitDefinition {
	t.addImports(imports...)
	return t
}

// Helper adds a helper type definition like #HealthProbe or #labelSelector.
// The param can be a StructParam, MapParam, or ArrayParam that defines the schema.
// Usage: trait.Helper("HealthProbe", defkit.Struct("probe").Fields(...))
func (t *TraitDefinition) Helper(name string, param Param) *TraitDefinition {
	t.addHelper(name, param)
	return t
}

// Labels sets metadata labels for the trait definition.
// These labels appear in the definition's labels block.
// Usage: trait.Labels(map[string]string{"ui-hidden": "true"})
func (t *TraitDefinition) Labels(labels map[string]string) *TraitDefinition {
	t.labels = labels
	return t
}

// GetLabels returns the trait's metadata labels.
func (t *TraitDefinition) GetLabels() map[string]string { return t.labels }

// RunOn adds placement conditions specifying which clusters this trait should run on.
// Use the placement package's fluent API to build conditions.
//
// Example:
//
//	defkit.NewTrait("eks-scaler").
//	    RunOn(placement.Label("provider").Eq("aws"))
//
// Multiple RunOn calls are combined with AND semantics (all conditions must match).
func (t *TraitDefinition) RunOn(conditions ...placement.Condition) *TraitDefinition {
	t.addRunOn(conditions...)
	return t
}

// NotRunOn adds placement conditions specifying which clusters this trait should NOT run on.
// Use the placement package's fluent API to build conditions.
//
// Example:
//
//	defkit.NewTrait("no-vclusters").
//	    NotRunOn(placement.Label("cluster-type").Eq("vcluster"))
//
// If any NotRunOn condition matches, the trait is ineligible for that cluster.
func (t *TraitDefinition) NotRunOn(conditions ...placement.Condition) *TraitDefinition {
	t.addNotRunOn(conditions...)
	return t
}

// DefName implements Definition.DefName.
func (t *TraitDefinition) DefName() string { return t.name }

// DefType implements Definition.DefType.
func (t *TraitDefinition) DefType() DefinitionType { return DefinitionTypeTrait }

// GetAppliesToWorkloads returns the workload types this trait applies to.
func (t *TraitDefinition) GetAppliesToWorkloads() []string { return t.appliesToWorkloads }

// GetConflictsWith returns traits that conflict with this one.
func (t *TraitDefinition) GetConflictsWith() []string { return t.conflictsWith }

// IsPodDisruptive returns whether this trait is pod-disruptive.
func (t *TraitDefinition) IsPodDisruptive() bool { return t.podDisruptive }

// GetStage returns the trait stage.
func (t *TraitDefinition) GetStage() string { return t.stage }

// Note: The following methods are inherited from baseDefinition:
// - GetDescription() string
// - GetParams() []Param
// - GetCustomStatus() string
// - GetHealthPolicy() string
// - GetHelperDefinitions() []HelperDefinition
// - GetTemplate() func(tpl *Template)
// - HasTemplate() bool

// ToCue generates the complete CUE definition string for this trait.
func (t *TraitDefinition) ToCue() string {
	gen := NewTraitCUEGenerator()
	if len(t.imports) > 0 {
		gen.WithImports(t.imports...)
	}

	var result string

	// If raw CUE is set, determine how to handle it
	if t.rawCUE != "" {
		// If raw CUE contains a complete definition (has a top-level "template:" block),
		// return it with the name rewritten to match the name set via NewTrait().
		// This ensures the fluent API name takes precedence over any name in the raw CUE.
		// We use regex to check for "template:" at the start of a line to avoid false
		// positives from paths like "patch: spec: template: spec:" which contain "template:"
		// but not as a top-level block.
		if hasTopLevelTemplateBlock(t.rawCUE) {
			result = t.GetRawCUEWithName()
		} else {
			// Otherwise, it's template content only - generate header with fluent API metadata
			result = gen.GenerateDefinitionWithRawTemplate(t, t.rawCUE)
		}
	} else {
		result = gen.GenerateFullDefinition(t)
	}

	// Format the CUE output for consistency
	return formatCUE(result)
}

// formatCUE formats a CUE string using the standard CUE formatter.
// If formatting fails, it returns the original string unchanged.
func formatCUE(cue string) string {
	formatted, err := format.Source([]byte(cue), format.Simplify())
	if err != nil {
		// If formatting fails, return the original string
		return cue
	}
	return string(formatted)
}

// hasTopLevelTemplateBlock checks if the CUE content has a top-level "template:" block.
// It returns true if "template:" appears at the start of a line (with optional leading whitespace),
// which indicates a complete definition. This distinguishes from cases like "patch: spec: template: spec:"
// where "template:" is part of a path, not a top-level block.
func hasTopLevelTemplateBlock(cue string) bool {
	return topLevelTemplateRegex.MatchString(cue)
}

// ToYAML generates the Kubernetes YAML representation of the TraitDefinition.
func (t *TraitDefinition) ToYAML() ([]byte, error) {
	cueStr := t.ToCue()

	// Build the TraitDefinition CR structure
	cr := map[string]any{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       "TraitDefinition",
		"metadata": map[string]any{
			"name": t.name,
			"annotations": map[string]any{
				"definition.oam.dev/description": t.description,
			},
		},
		"spec": map[string]any{
			"appliesToWorkloads": t.appliesToWorkloads,
			"podDisruptive":      t.podDisruptive,
			"schematic": map[string]any{
				"cue": map[string]any{
					"template": cueStr,
				},
			},
		},
	}

	// Add conflictsWith if present
	if len(t.conflictsWith) > 0 {
		cr["spec"].(map[string]any)["conflictsWith"] = t.conflictsWith
	}

	// Add stage if present
	if t.stage != "" {
		cr["spec"].(map[string]any)["stage"] = t.stage
	}

	return yaml.Marshal(cr)
}

// --- TraitCUEGenerator ---

// TraitCUEGenerator generates CUE definitions for traits.
type TraitCUEGenerator struct {
	indent  string
	imports []string
}

// NewTraitCUEGenerator creates a new trait CUE generator.
func NewTraitCUEGenerator() *TraitCUEGenerator {
	return &TraitCUEGenerator{
		indent:  "\t",
		imports: []string{},
	}
}

// WithImports adds CUE imports.
func (g *TraitCUEGenerator) WithImports(imports ...string) *TraitCUEGenerator {
	g.imports = append(g.imports, imports...)
	return g
}

// GenerateFullDefinition generates the complete CUE definition for a trait.
func (g *TraitCUEGenerator) GenerateFullDefinition(t *TraitDefinition) string {
	var sb strings.Builder

	// Write imports if any
	if len(g.imports) > 0 {
		sb.WriteString("import (\n")
		for _, imp := range g.imports {
			sb.WriteString(fmt.Sprintf("\t%q\n", imp))
		}
		sb.WriteString(")\n\n")
	}

	// Write trait header
	sb.WriteString(fmt.Sprintf("%s: {\n", cueLabel(t.GetName())))
	sb.WriteString(fmt.Sprintf("%stype: \"trait\"\n", g.indent))
	sb.WriteString(fmt.Sprintf("%sannotations: {}\n", g.indent))

	// Write labels block when Labels() was explicitly called (nil check distinguishes unset from empty)
	if t.labels != nil {
		if len(t.labels) > 0 {
			sb.WriteString(fmt.Sprintf("%slabels: {\n", g.indent))
			for k, v := range t.labels {
				sb.WriteString(fmt.Sprintf("%s\t%q: %q\n", g.indent, k, v))
			}
			sb.WriteString(fmt.Sprintf("%s}\n", g.indent))
		} else {
			sb.WriteString(fmt.Sprintf("%slabels: {}\n", g.indent))
		}
	}
	sb.WriteString(fmt.Sprintf("%sdescription: %q\n", g.indent, t.GetDescription()))

	// Write attributes
	sb.WriteString(fmt.Sprintf("%sattributes: {\n", g.indent))
	g.writeAttributes(&sb, t, 2)
	sb.WriteString(fmt.Sprintf("%s}\n", g.indent))

	sb.WriteString("}\n")

	// Write template section
	sb.WriteString(g.GenerateTemplate(t))

	return sb.String()
}

// writeAttributes writes the trait attributes block.
func (g *TraitCUEGenerator) writeAttributes(sb *strings.Builder, t *TraitDefinition, depth int) {
	indent := strings.Repeat(g.indent, depth)

	// podDisruptive (always emit — vela CUE always includes this attribute)
	sb.WriteString(fmt.Sprintf("%spodDisruptive: %v\n", indent, t.IsPodDisruptive()))

	// stage (if set)
	if t.GetStage() != "" {
		sb.WriteString(fmt.Sprintf("%sstage: %q\n", indent, t.GetStage()))
	}

	// appliesToWorkloads
	if len(t.GetAppliesToWorkloads()) > 0 {
		workloads := make([]string, len(t.GetAppliesToWorkloads()))
		for i, w := range t.GetAppliesToWorkloads() {
			workloads[i] = fmt.Sprintf("%q", w)
		}
		sb.WriteString(fmt.Sprintf("%sappliesToWorkloads: [%s]\n", indent, strings.Join(workloads, ", ")))
	}

	// conflictsWith (emit when explicitly set, even if empty)
	if t.conflictsWithSet {
		if len(t.GetConflictsWith()) > 0 {
			conflicts := make([]string, len(t.GetConflictsWith()))
			for i, c := range t.GetConflictsWith() {
				conflicts[i] = fmt.Sprintf("%q", c)
			}
			sb.WriteString(fmt.Sprintf("%sconflictsWith: [%s]\n", indent, strings.Join(conflicts, ", ")))
		} else {
			sb.WriteString(fmt.Sprintf("%sconflictsWith: []\n", indent))
		}
	}

	// status (customStatus and healthPolicy)
	if t.GetCustomStatus() != "" || t.GetHealthPolicy() != "" {
		sb.WriteString(fmt.Sprintf("%sstatus: {\n", indent))
		innerIndent := strings.Repeat(g.indent, depth+1)

		if t.GetCustomStatus() != "" {
			sb.WriteString(fmt.Sprintf("%scustomStatus: #\"\"\"\n", innerIndent))
			for _, line := range strings.Split(t.GetCustomStatus(), "\n") {
				sb.WriteString(fmt.Sprintf("%s\t%s\n", innerIndent, line))
			}
			sb.WriteString(fmt.Sprintf("%s\t\"\"\"#\n", innerIndent))
		}

		if t.GetHealthPolicy() != "" {
			sb.WriteString(fmt.Sprintf("%shealthPolicy: #\"\"\"\n", innerIndent))
			for _, line := range strings.Split(t.GetHealthPolicy(), "\n") {
				sb.WriteString(fmt.Sprintf("%s\t%s\n", innerIndent, line))
			}
			sb.WriteString(fmt.Sprintf("%s\t\"\"\"#\n", innerIndent))
		}

		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	}

	// workloadRefPath (if explicitly set)
	if t.workloadRefPath != nil {
		sb.WriteString(fmt.Sprintf("%sworkloadRefPath: %q\n", indent, *t.workloadRefPath))
	}
}

// GenerateTemplate generates the template block for a trait.
func (g *TraitCUEGenerator) GenerateTemplate(t *TraitDefinition) string {
	var sb strings.Builder

	// If TemplateBlock is set, use it directly (raw CUE for template section)
	if t.HasTemplateBlock() {
		sb.WriteString("template: {\n")
		// Indent each line of the template block
		for _, line := range strings.Split(strings.TrimSpace(t.GetTemplateBlock()), "\n") {
			sb.WriteString(fmt.Sprintf("%s%s\n", g.indent, line))
		}
		sb.WriteString("}\n")
		return sb.String()
	}

	sb.WriteString("template: {\n")

	// Check if using Template API
	usedPatchContainer := false
	if t.HasTemplate() {
		// Check if PatchContainer is configured (it generates its own parameter block)
		tpl := NewTemplate()
		t.GetTemplate()(tpl)
		usedPatchContainer = tpl.GetPatchContainerConfig() != nil

		g.writeUnifiedTemplate(&sb, t, 1)
	}

	// Generate parameter section (skip if PatchContainer already generated it)
	if !usedPatchContainer {
		sb.WriteString(g.generateParameterBlock(t, 1))
	}

	// Generate helper type definitions (like #HealthProbe, #labelSelector)
	gen := NewCUEGenerator()
	for _, helperDef := range t.GetHelperDefinitions() {
		gen.WriteHelperDefinition(&sb, helperDef, 1)
	}

	sb.WriteString("}\n")
	return sb.String()
}

// writeUnifiedTemplate writes the template block using the new unified Template API.
// This handles both patch and outputs in a single template function.
func (g *TraitCUEGenerator) writeUnifiedTemplate(sb *strings.Builder, t *TraitDefinition, depth int) {
	// Execute the unified template function to capture all operations
	tpl := NewTemplate()
	if t.template != nil {
		t.template(tpl)
	}

	indent := strings.Repeat(g.indent, depth)
	gen := NewCUEGenerator()

	// Generate PatchContainer pattern if configured
	if config := tpl.GetPatchContainerConfig(); config != nil {
		g.writePatchContainerPattern(sb, config, depth)
		return
	}

	// Handle raw blocks (for init-container, expose, hpa and similar traits)
	// Check if any raw block is set
	hasRawBlocks := tpl.GetRawPatchBlock() != "" || tpl.GetRawOutputsBlock() != "" ||
		tpl.GetRawParameterBlock() != "" || tpl.GetRawHeaderBlock() != ""
	if hasRawBlocks {
		g.writeRawBlocks(sb, tpl, depth)
		return
	}

	// Render let bindings before patch/outputs
	for _, lb := range tpl.GetLetBindings() {
		exprStr := gen.valueToCUE(lb.Expr())
		sb.WriteString(fmt.Sprintf("%slet %s = %s\n", indent, lb.Name(), exprStr))
	}

	// Generate patch block if present
	if tpl.HasPatch() {
		// Write patch strategy comment if set
		if tpl.GetPatchStrategy() != "" {
			sb.WriteString(fmt.Sprintf("%s// +patchStrategy=%s\n", indent, tpl.GetPatchStrategy()))
		}

		sb.WriteString(fmt.Sprintf("%spatch: ", indent))
		g.writePatchResourceOps(sb, gen, tpl.GetPatch().Ops(), depth)
		sb.WriteString("\n")
	}

	// Generate outputs block if present
	if outputs := tpl.GetOutputs(); len(outputs) > 0 {
		sb.WriteString(fmt.Sprintf("%soutputs: {\n", indent))
		for name, res := range outputs {
			g.writeTraitResourceOutput(sb, gen, name, res, depth+1)
		}
		// Render output groups (multiple outputs under one condition)
		for _, group := range tpl.GetOutputGroups() {
			condStr := gen.conditionToCUE(group.cond)
			sb.WriteString(fmt.Sprintf("%s\tif %s {\n", indent, condStr))
			for gName, gRes := range group.outputs {
				g.writeTraitResourceOutput(sb, gen, gName, gRes, depth+2)
			}
			sb.WriteString(fmt.Sprintf("%s\t}\n", indent))
		}
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	}
}

// writeRawBlocks writes raw CUE blocks for header, patch, outputs, and parameter.
// This is used for traits with complex structures that cannot be
// expressed using the fluent API (like init-container, expose, hpa, etc.).
// Order: header (let bindings) -> patch -> outputs -> parameter
func (g *TraitCUEGenerator) writeRawBlocks(sb *strings.Builder, tpl *Template, depth int) {
	indent := strings.Repeat(g.indent, depth)

	// Write raw header block (let bindings and pre-output declarations)
	if rawHeader := tpl.GetRawHeaderBlock(); rawHeader != "" {
		for _, line := range strings.Split(strings.TrimSpace(rawHeader), "\n") {
			sb.WriteString(fmt.Sprintf("%s%s\n", indent, line))
		}
	}

	// Write raw patch block
	if rawPatch := tpl.GetRawPatchBlock(); rawPatch != "" {
		for _, line := range strings.Split(strings.TrimSpace(rawPatch), "\n") {
			sb.WriteString(fmt.Sprintf("%s%s\n", indent, line))
		}
	}

	// Write raw outputs block
	if rawOutputs := tpl.GetRawOutputsBlock(); rawOutputs != "" {
		for _, line := range strings.Split(strings.TrimSpace(rawOutputs), "\n") {
			sb.WriteString(fmt.Sprintf("%s%s\n", indent, line))
		}
	}

	// Write raw parameter block
	if rawParam := tpl.GetRawParameterBlock(); rawParam != "" {
		for _, line := range strings.Split(strings.TrimSpace(rawParam), "\n") {
			sb.WriteString(fmt.Sprintf("%s%s\n", indent, line))
		}
	}
}

// writePatchResourceOps writes PatchResource operations as CUE.
func (g *TraitCUEGenerator) writePatchResourceOps(sb *strings.Builder, gen *CUEGenerator, ops []ResourceOp, depth int) {
	if len(ops) == 0 {
		sb.WriteString("{}")
		return
	}

	// Check for passthrough operation (patch: parameter)
	for _, op := range ops {
		if _, isPassthrough := op.(*PassthroughOp); isPassthrough {
			sb.WriteString("parameter")
			return
		}
	}

	// Separate IfBlocks from other ops
	var bareOps []ResourceOp
	var ifBlocks []*IfBlock
	for _, op := range ops {
		if ib, ok := op.(*IfBlock); ok {
			ifBlocks = append(ifBlocks, ib)
		} else {
			bareOps = append(bareOps, op)
		}
	}

	// If 0 or 1 IfBlock, use existing merged approach (no path conflicts possible)
	if len(ifBlocks) <= 1 {
		tree := gen.buildFieldTree(ops)
		gen.liftChildConditions(tree)
		g.writePatchFieldTree(sb, gen, tree, depth)
		return
	}

	// Multiple IfBlocks: process each separately to avoid path conflicts
	// when different IfBlocks target the same field paths with different conditions.
	prefix := g.findIfBlockCommonPrefix(ifBlocks, bareOps)
	var prefixParts []string
	if prefix != "" {
		prefixParts = strings.Split(prefix, ".")
	}

	// Write the common prefix inline (e.g., "spec: ")
	for _, p := range prefixParts {
		sb.WriteString(fmt.Sprintf("%s: ", p))
	}

	innerDepth := depth + len(prefixParts)

	// Open block for the children
	sb.WriteString("{\n")

	indent := strings.Repeat(g.indent, innerDepth+1)
	first := true

	// Render bare ops first (if any)
	if len(bareOps) > 0 {
		bareTree := gen.buildFieldTree(bareOps)
		// Navigate to subtree below common prefix
		subtree := bareTree
		for _, p := range prefixParts {
			if child, ok := subtree.children[p]; ok {
				subtree = child
			}
		}
		gen.liftChildConditions(subtree)
		for _, key := range subtree.childOrder {
			node := subtree.children[key]
			sb.WriteString(indent)
			g.writePatchFieldNode(sb, gen, key, node, innerDepth+1)
			sb.WriteString("\n")
		}
		first = false
	}

	// Render each IfBlock as a separate subtree
	for _, ib := range ifBlocks {
		if !first {
			sb.WriteString("\n")
		}

		// Build tree from inner ops only (without the outer If condition)
		innerTree := gen.buildFieldTree(ib.Ops())

		// Navigate to subtree below common prefix
		subtree := innerTree
		for _, p := range prefixParts {
			if child, ok := subtree.children[p]; ok {
				subtree = child
			}
		}

		// Normalize conditional nodes in the subtree
		gen.liftChildConditions(subtree)

		// Emit: if condition { ... }
		condStr := gen.conditionToCUE(ib.Cond())
		sb.WriteString(fmt.Sprintf("%sif %s {\n", indent, condStr))

		// Render subtree children inside the if block
		innerBlockIndent := strings.Repeat(g.indent, innerDepth+2)
		for _, key := range subtree.childOrder {
			node := subtree.children[key]
			sb.WriteString(innerBlockIndent)
			g.writePatchFieldNode(sb, gen, key, node, innerDepth+2)
			sb.WriteString("\n")
		}

		sb.WriteString(fmt.Sprintf("%s}", indent))
		first = false
	}

	// Close outer block
	sb.WriteString(fmt.Sprintf("\n%s}", strings.Repeat(g.indent, innerDepth)))
}

// findIfBlockCommonPrefix finds the longest common path prefix across all IfBlocks and bare ops.
// For example, if all paths start with "spec.", the prefix is "spec".
func (g *TraitCUEGenerator) findIfBlockCommonPrefix(ifBlocks []*IfBlock, bareOps []ResourceOp) string {
	var allPaths []string

	for _, ib := range ifBlocks {
		for _, op := range ib.Ops() {
			switch o := op.(type) {
			case *SetOp:
				allPaths = append(allPaths, o.Path())
			case *SetIfOp:
				allPaths = append(allPaths, o.Path())
			case *PatchStrategyAnnotationOp:
				allPaths = append(allPaths, o.Path())
			}
		}
	}

	for _, op := range bareOps {
		switch o := op.(type) {
		case *SetOp:
			allPaths = append(allPaths, o.Path())
		case *SetIfOp:
			allPaths = append(allPaths, o.Path())
		}
	}

	if len(allPaths) == 0 {
		return ""
	}

	firstParts := strings.Split(allPaths[0], ".")
	commonLen := len(firstParts)

	for _, p := range allPaths[1:] {
		parts := strings.Split(p, ".")
		minLen := commonLen
		if len(parts) < minLen {
			minLen = len(parts)
		}
		matched := 0
		for i := 0; i < minLen; i++ {
			if parts[i] != firstParts[i] {
				break
			}
			matched++
		}
		commonLen = matched
	}

	if commonLen == 0 {
		return ""
	}

	// Don't include leaf-level parts — only include the common ancestor path
	// (at least one level must remain below the prefix for each IfBlock)
	return strings.Join(firstParts[:commonLen], ".")
}

// writePatchFieldTree writes a field tree as CUE patch syntax.
// This method reuses the CUEGenerator's writeFieldTree with patch-specific handling.
func (g *TraitCUEGenerator) writePatchFieldTree(sb *strings.Builder, gen *CUEGenerator, tree *fieldNode, depth int) {
	// For a single-path case with one child, generate inline nested syntax
	if len(tree.childOrder) == 1 {
		key := tree.childOrder[0]
		node := tree.children[key]
		g.writePatchFieldNode(sb, gen, key, node, depth)
		return
	}

	// Multiple keys - write as block
	sb.WriteString("{\n")
	indent := strings.Repeat(g.indent, depth+1)
	for _, key := range tree.childOrder {
		node := tree.children[key]
		sb.WriteString(indent)
		g.writePatchFieldNode(sb, gen, key, node, depth+1)
		sb.WriteString("\n")
	}
	sb.WriteString(fmt.Sprintf("%s}", strings.Repeat(g.indent, depth)))
}

// writePatchFieldNode writes a single field node for patches.
func (g *TraitCUEGenerator) writePatchFieldNode(sb *strings.Builder, gen *CUEGenerator, key string, node *fieldNode, depth int) {
	// Handle ForEach operations specially - pass condition if present
	if node.forEach != nil {
		g.writeForEachOp(sb, gen, key, node.forEach, node.cond, depth)
		return
	}

	// Handle PatchKey operations specially (array patches with merge key) - pass condition if present
	if node.patchKey != nil {
		g.writePatchKeyOp(sb, gen, key, node.patchKey, node.cond, depth)
		return
	}

	// Handle SpreadAll operations (array constraint patches)
	if node.spreadAll != nil {
		g.writeSpreadAllOp(sb, gen, key, node.spreadAll, node.cond, depth)
		return
	}

	// Handle conditional fields
	if node.cond != nil {
		condStr := gen.conditionToCUE(node.cond)
		sb.WriteString(fmt.Sprintf("if %s {\n", condStr))
		innerIndent := strings.Repeat(g.indent, depth+1)
		// Emit patchStrategy annotation inside the if block
		if node.patchStrategy != "" {
			sb.WriteString(fmt.Sprintf("%s// +patchStrategy=%s\n", innerIndent, node.patchStrategy))
		}
		sb.WriteString(fmt.Sprintf("%s%s: ", innerIndent, key))
		if len(node.children) > 0 {
			g.writePatchFieldTreeFromChildren(sb, gen, node, depth+1)
		} else if node.value != nil {
			sb.WriteString(gen.valueToCUE(node.value))
		}
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("%s}", strings.Repeat(g.indent, depth)))
		return
	}

	// Regular field
	if node.patchStrategy != "" {
		sb.WriteString(fmt.Sprintf("// +patchStrategy=%s\n%s", node.patchStrategy, strings.Repeat(g.indent, depth)))
	}
	sb.WriteString(fmt.Sprintf("%s: ", key))
	if len(node.children) > 0 {
		g.writePatchFieldTreeFromChildren(sb, gen, node, depth)
	} else if node.value != nil {
		sb.WriteString(gen.valueToCUE(node.value))
	}
}

// writePatchFieldTreeFromChildren writes children of a field node.
func (g *TraitCUEGenerator) writePatchFieldTreeFromChildren(sb *strings.Builder, gen *CUEGenerator, parent *fieldNode, depth int) {
	// For a single child without condition, generate inline nested syntax
	// If child has condition, we must use block format to allow proper if-wrapping
	if len(parent.childOrder) == 1 {
		key := parent.childOrder[0]
		node := parent.children[key]
		// Check if this node or any descendant has a condition - if so, use block format
		if !g.hasConditionalDescendant(node) {
			g.writePatchFieldNode(sb, gen, key, node, depth)
			return
		}
	}

	// Multiple children or conditional child - write as block
	sb.WriteString("{\n")
	indent := strings.Repeat(g.indent, depth+1)
	for _, key := range parent.childOrder {
		node := parent.children[key]
		sb.WriteString(indent)
		g.writePatchFieldNode(sb, gen, key, node, depth+1)
		sb.WriteString("\n")
	}
	sb.WriteString(fmt.Sprintf("%s}", strings.Repeat(g.indent, depth)))
}

// hasConditionalDescendant checks if a node or any of its descendants has a condition,
// patchKey, or forEach operation that requires block format output.
func (g *TraitCUEGenerator) hasConditionalDescendant(node *fieldNode) bool {
	if node.cond != nil {
		return true
	}
	// PatchKey, ForEach, and patchStrategy annotations require block format
	// because they need to write comments or special syntax that can't appear inline
	if node.patchKey != nil || node.forEach != nil || node.patchStrategy != "" {
		return true
	}
	for _, child := range node.children {
		if g.hasConditionalDescendant(child) {
			return true
		}
	}
	return false
}

// writeForEachOp writes a ForEach operation as CUE.
// Generates: for k, v in source { (k): v }
// If cond is provided, wraps in: if cond { ... }
func (g *TraitCUEGenerator) writeForEachOp(sb *strings.Builder, gen *CUEGenerator, key string, op *ForEachOp, cond Condition, depth int) {
	sourceStr := gen.valueToCUE(op.Source())
	indent := strings.Repeat(g.indent, depth)

	// Wrap in condition if present
	if cond != nil {
		condStr := gen.conditionToCUE(cond)
		sb.WriteString(fmt.Sprintf("if %s {\n", condStr))
		sb.WriteString(fmt.Sprintf("%s\t%s: {\n", indent, key))
		sb.WriteString(fmt.Sprintf("%s\t\tfor k, v in %s {\n", indent, sourceStr))
		sb.WriteString(fmt.Sprintf("%s\t\t\t(k): v\n", indent))
		sb.WriteString(fmt.Sprintf("%s\t\t}\n", indent))
		sb.WriteString(fmt.Sprintf("%s\t}\n", indent))
		sb.WriteString(fmt.Sprintf("%s}", indent))
	} else {
		sb.WriteString(fmt.Sprintf("%s: {\n", key))
		innerIndent := strings.Repeat(g.indent, depth+1)
		sb.WriteString(fmt.Sprintf("%sfor k, v in %s {\n", innerIndent, sourceStr))
		sb.WriteString(fmt.Sprintf("%s\t(k): v\n", innerIndent))
		sb.WriteString(fmt.Sprintf("%s}\n", innerIndent))
		sb.WriteString(fmt.Sprintf("%s}", indent))
	}
}

// writePatchKeyOp writes a PatchKey operation as CUE.
// Generates: // +patchKey=key
//
//	path: [...]
//
// If cond is provided, wraps in: if cond { ... }
func (g *TraitCUEGenerator) writePatchKeyOp(sb *strings.Builder, gen *CUEGenerator, key string, op *PatchKeyOp, cond Condition, depth int) {
	indent := strings.Repeat(g.indent, depth)
	elements := op.Elements()
	useArrayValue := len(elements) == 1
	if useArrayValue {
		_, useArrayValue = elements[0].(*ArrayParam)
	}

	// Wrap in condition if present
	if cond != nil {
		condStr := gen.conditionToCUE(cond)
		sb.WriteString(fmt.Sprintf("if %s {\n", condStr))
		sb.WriteString(fmt.Sprintf("%s\t// +patchKey=%s\n", indent, op.Key()))
		if useArrayValue {
			valStr := gen.valueToCUEAtDepth(elements[0], depth+1)
			sb.WriteString(fmt.Sprintf("%s\t%s: %s\n", indent, key, valStr))
		} else {
			sb.WriteString(fmt.Sprintf("%s\t%s: [", indent, key))
			for i, elem := range elements {
				if i > 0 {
					sb.WriteString(", ")
				}
				// Use depth-aware formatting for ArrayElement
				if arrElem, ok := elem.(*ArrayElement); ok {
					sb.WriteString(gen.arrayElementToCUEWithDepth(arrElem, depth+1))
				} else {
					sb.WriteString(gen.valueToCUE(elem))
				}
			}
			sb.WriteString("]\n")
		}
		sb.WriteString(fmt.Sprintf("%s}", indent))
	} else {
		// Write the patchKey annotation comment
		sb.WriteString(fmt.Sprintf("// +patchKey=%s\n", op.Key()))
		if useArrayValue {
			valStr := gen.valueToCUEAtDepth(elements[0], depth)
			sb.WriteString(fmt.Sprintf("%s%s: %s", indent, key, valStr))
		} else {
			sb.WriteString(fmt.Sprintf("%s%s: [", indent, key))

			// Write elements
			for i, elem := range elements {
				if i > 0 {
					sb.WriteString(", ")
				}
				// Use depth-aware formatting for ArrayElement
				if arrElem, ok := elem.(*ArrayElement); ok {
					sb.WriteString(gen.arrayElementToCUEWithDepth(arrElem, depth))
				} else {
					sb.WriteString(gen.valueToCUE(elem))
				}
			}
			sb.WriteString("]")
		}
	}
}

// writeSpreadAllOp writes a SpreadAll operation as CUE.
// Generates: path: [...{element}]
// If cond is provided, wraps in: if cond { ... }
func (g *TraitCUEGenerator) writeSpreadAllOp(sb *strings.Builder, gen *CUEGenerator, key string, op *SpreadAllOp, cond Condition, depth int) {
	indent := strings.Repeat(g.indent, depth)

	writeElements := func(sb *strings.Builder, elemDepth int) {
		for i, elem := range op.Elements() {
			if i > 0 {
				sb.WriteString(", ")
			}
			if arrElem, ok := elem.(*ArrayElement); ok {
				// Build a field tree from the element's ops for proper nesting
				tree := gen.buildFieldTree(arrElem.Ops())
				gen.liftChildConditions(tree)
				// Also add direct field assignments
				for fk, fv := range arrElem.Fields() {
					gen.insertIntoTree(tree, fk, fv, nil)
				}
				sb.WriteString("...{\n")
				// Write tree children with explicit indentation
				// (avoid writePatchFieldTree's single-child inline optimization)
				innerIndent := strings.Repeat(g.indent, elemDepth+1)
				for _, tk := range tree.childOrder {
					tn := tree.children[tk]
					sb.WriteString(innerIndent)
					g.writePatchFieldNode(sb, gen, tk, tn, elemDepth+1)
					sb.WriteString("\n")
				}
				sb.WriteString(fmt.Sprintf("%s}", strings.Repeat(g.indent, elemDepth)))
			} else {
				sb.WriteString("...")
				sb.WriteString(gen.valueToCUE(elem))
			}
		}
	}

	if cond != nil {
		condStr := gen.conditionToCUE(cond)
		sb.WriteString(fmt.Sprintf("if %s {\n", condStr))
		sb.WriteString(fmt.Sprintf("%s\t%s: [", indent, key))
		writeElements(sb, depth+1)
		sb.WriteString("]\n")
		sb.WriteString(fmt.Sprintf("%s}", indent))
	} else {
		sb.WriteString(fmt.Sprintf("%s: [", key))
		writeElements(sb, depth)
		sb.WriteString("]")
	}
}

// writeTraitResourceOutput writes a resource as CUE for trait outputs.
// This handles OutputsIf conditions and VersionConditionals, which the old
// writeResourceOutput method did not support.
func (g *TraitCUEGenerator) writeTraitResourceOutput(sb *strings.Builder, gen *CUEGenerator, name string, res *Resource, depth int) {
	gen.writeResourceOutput(sb, name, res, res.outputCondition, depth)
}

// generateParameterBlock generates the parameter schema for the trait.
func (g *TraitCUEGenerator) generateParameterBlock(t *TraitDefinition, depth int) string {
	var sb strings.Builder
	indent := strings.Repeat(g.indent, depth)

	// Check for special parameter types that change the entire parameter structure
	for _, param := range t.GetParams() {
		// OpenStructParam: parameter: {...}
		if _, ok := param.(*OpenStructParam); ok {
			sb.WriteString(fmt.Sprintf("%sparameter: {...}\n", indent))
			return sb.String()
		}

		// DynamicMapParam: parameter: [string]: T
		if dynMap, ok := param.(*DynamicMapParam); ok {
			var typeStr string
			if dynMap.GetValueTypeUnion() != "" {
				typeStr = dynMap.GetValueTypeUnion()
			} else {
				typeStr = cueTypeStr(dynMap.GetValueType())
			}
			sb.WriteString(fmt.Sprintf("%sparameter: [string]: %s\n", indent, typeStr))
			return sb.String()
		}
	}

	// Standard parameter block
	sb.WriteString(fmt.Sprintf("%sparameter: {\n", indent))

	gen := NewCUEGenerator()
	for _, param := range t.GetParams() {
		// Handle OpenArrayParam specially
		if openArr, ok := param.(*OpenArrayParam); ok {
			innerIndent := strings.Repeat(g.indent, depth+1)
			sb.WriteString(fmt.Sprintf("%s%s: [...{...}]\n", innerIndent, openArr.Name()))
			continue
		}
		gen.writeParam(&sb, param, depth+1)
	}

	sb.WriteString(fmt.Sprintf("%s}\n", indent))
	return sb.String()
}

// writePatchContainerPattern generates the complete PatchContainer pattern in CUE.
// This generates the #PatchParams helper, PatchContainer definition, patch block,
// parameter schema, and errs aggregation.
func (g *TraitCUEGenerator) writePatchContainerPattern(sb *strings.Builder, config *PatchContainerConfig, depth int) {
	indent := strings.Repeat(g.indent, depth)
	innerIndent := strings.Repeat(g.indent, depth+1)
	deepIndent := strings.Repeat(g.indent, depth+2)

	// Determine the helper type name (default: "PatchParams")
	paramsTypeName := config.ParamsTypeName
	if paramsTypeName == "" {
		paramsTypeName = "PatchParams"
	}

	// Generate helper definition
	if config.CustomParamsBlock != "" {
		// Use custom params block (for complex schemas like startup-probe)
		sb.WriteString(fmt.Sprintf("%s#%s: {\n", indent, paramsTypeName))
		sb.WriteString(fmt.Sprintf("%s// +usage=Specify the name of the target container, if not set, use the component name\n", innerIndent))
		sb.WriteString(fmt.Sprintf("%scontainerName: *\"\" | string\n", innerIndent))
		// Write each line of custom params block with proper indentation
		for _, line := range strings.Split(strings.TrimSpace(config.CustomParamsBlock), "\n") {
			sb.WriteString(fmt.Sprintf("%s%s\n", innerIndent, strings.TrimSpace(line)))
		}
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	} else {
		sb.WriteString(fmt.Sprintf("%s#%s: {\n", indent, paramsTypeName))
		sb.WriteString(fmt.Sprintf("%s// +usage=Specify the name of the target container, if not set, use the component name\n", innerIndent))
		sb.WriteString(fmt.Sprintf("%scontainerName: *\"\" | string\n", innerIndent))

		// Generate patch field parameters
		for _, field := range config.PatchFields {
			g.writePatchParamField(sb, field, innerIndent)
		}

		// Generate group field parameters
		for _, group := range config.Groups {
			g.writePatchParamGroup(sb, group, innerIndent)
		}
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	}

	// Generate PatchContainer definition
	if config.CustomPatchContainerBlock != "" {
		// Use custom PatchContainer block for complex merge logic
		sb.WriteString(fmt.Sprintf("%sPatchContainer: {\n", indent))
		// Write each line of custom block with proper indentation
		for _, line := range strings.Split(strings.TrimSpace(config.CustomPatchContainerBlock), "\n") {
			sb.WriteString(fmt.Sprintf("%s%s\n", innerIndent, line))
		}
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	} else {
		sb.WriteString(fmt.Sprintf("%sPatchContainer: {\n", indent))
		sb.WriteString(fmt.Sprintf("%s_params:         #%s\n", innerIndent, paramsTypeName))
		sb.WriteString(fmt.Sprintf("%sname:            _params.containerName\n", innerIndent))
		sb.WriteString(fmt.Sprintf("%s_baseContainers: context.output.spec.template.spec.containers\n", innerIndent))
		sb.WriteString(fmt.Sprintf("%s_matchContainers_: [for _container_ in _baseContainers if _container_.name == name {_container_}]\n", innerIndent))

		// Container not found error
		sb.WriteString(fmt.Sprintf("%sif len(_matchContainers_) == 0 {\n", innerIndent))
		sb.WriteString(fmt.Sprintf("%s\terr: \"container \\(name) not found\"\n", innerIndent))
		sb.WriteString(fmt.Sprintf("%s}\n", innerIndent))

		// Container found - apply patches
		sb.WriteString(fmt.Sprintf("%sif len(_matchContainers_) > 0 {\n", innerIndent))

		// Write flat fields
		for _, field := range config.PatchFields {
			g.writePatchContainerField(sb, field, deepIndent)
		}

		// Write grouped fields (e.g., startupProbe: { ... })
		for _, group := range config.Groups {
			g.writePatchContainerGroup(sb, group, deepIndent)
		}

		sb.WriteString(fmt.Sprintf("%s}\n", innerIndent))
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	}

	// Generate patch block
	if config.CustomPatchBlock != "" {
		// Use custom patch block for complex patch structures
		sb.WriteString(fmt.Sprintf("%s// +patchStrategy=open\n", indent))
		sb.WriteString(fmt.Sprintf("%spatch: spec: template: spec: {\n", indent))
		for _, line := range strings.Split(strings.TrimSpace(config.CustomPatchBlock), "\n") {
			sb.WriteString(fmt.Sprintf("%s%s\n", innerIndent, line))
		}
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	} else {
		if config.PatchStrategy != "" {
			sb.WriteString(fmt.Sprintf("%s// +patchStrategy=%s\n", indent, config.PatchStrategy))
		}
		sb.WriteString(fmt.Sprintf("%spatch: spec: template: spec: {\n", indent))

		// Determine the multi-container parameter name
		multiParam := config.ContainersParam
		if multiParam == "" && config.MultiContainerParam != "" {
			multiParam = config.MultiContainerParam
		}

		if config.AllowMultiple && multiParam != "" {
			// Multi-container mode
			sb.WriteString(fmt.Sprintf("%sif parameter.%s == _|_ {\n", innerIndent, multiParam))
			sb.WriteString(fmt.Sprintf("%s\t// +patchKey=name\n", innerIndent))
			sb.WriteString(fmt.Sprintf("%s\tcontainers: [{\n", innerIndent))
			sb.WriteString(fmt.Sprintf("%s\t\tPatchContainer & {_params: {\n", innerIndent))

			// Default containerName to context.name
			if config.DefaultToContextName {
				sb.WriteString(fmt.Sprintf("%s\t\t\tif parameter.%s == \"\" {\n", innerIndent, config.ContainerNameParam))
				sb.WriteString(fmt.Sprintf("%s\t\t\t\tcontainerName: context.name\n", innerIndent))
				sb.WriteString(fmt.Sprintf("%s\t\t\t}\n", innerIndent))
				sb.WriteString(fmt.Sprintf("%s\t\t\tif parameter.%s != \"\" {\n", innerIndent, config.ContainerNameParam))
				sb.WriteString(fmt.Sprintf("%s\t\t\t\tcontainerName: parameter.%s\n", innerIndent, config.ContainerNameParam))
				sb.WriteString(fmt.Sprintf("%s\t\t\t}\n", innerIndent))
			}

			// Map flat parameters
			for _, field := range config.PatchFields {
				g.writePatchParamMapping(sb, field, innerIndent+"\t\t\t", "parameter.")
			}

			// Map grouped parameters
			for _, group := range config.Groups {
				g.writePatchGroupMapping(sb, group, innerIndent+"\t\t\t", "parameter.")
			}

			sb.WriteString(fmt.Sprintf("%s\t\t}}\n", innerIndent))
			sb.WriteString(fmt.Sprintf("%s\t}]\n", innerIndent))
			sb.WriteString(fmt.Sprintf("%s}\n", innerIndent))

			// Multiple containers mode
			sb.WriteString(fmt.Sprintf("%sif parameter.%s != _|_ {\n", innerIndent, multiParam))
			sb.WriteString(fmt.Sprintf("%s\t// +patchKey=name\n", innerIndent))
			sb.WriteString(fmt.Sprintf("%s\tcontainers: [for c in parameter.%s {\n", innerIndent, multiParam))
			checkField := "containerName"
			if config.MultiContainerCheckField != "" {
				checkField = config.MultiContainerCheckField
			}
			errMsg := fmt.Sprintf("containerName must be set for %s", multiParam)
			if config.MultiContainerErrMsg != "" {
				errMsg = config.MultiContainerErrMsg
			}
			sb.WriteString(fmt.Sprintf("%s\t\tif c.%s == \"\" {\n", innerIndent, checkField))
			sb.WriteString(fmt.Sprintf("%s\t\t\terr: \"%s\"\n", innerIndent, errMsg))
			sb.WriteString(fmt.Sprintf("%s\t\t}\n", innerIndent))
			sb.WriteString(fmt.Sprintf("%s\t\tif c.%s != \"\" {\n", innerIndent, checkField))
			sb.WriteString(fmt.Sprintf("%s\t\t\tPatchContainer & {_params: c}\n", innerIndent))
			sb.WriteString(fmt.Sprintf("%s\t\t}\n", innerIndent))
			sb.WriteString(fmt.Sprintf("%s\t}]\n", innerIndent))
			sb.WriteString(fmt.Sprintf("%s}\n", innerIndent))
		} else {
			// Single container mode
			sb.WriteString(fmt.Sprintf("%s// +patchKey=name\n", innerIndent))
			sb.WriteString(fmt.Sprintf("%scontainers: [{\n", innerIndent))
			sb.WriteString(fmt.Sprintf("%s\tPatchContainer & {_params: {\n", innerIndent))

			// Default containerName to context.name
			if config.DefaultToContextName {
				sb.WriteString(fmt.Sprintf("%s\t\tif parameter.%s == \"\" {\n", innerIndent, config.ContainerNameParam))
				sb.WriteString(fmt.Sprintf("%s\t\t\tcontainerName: context.name\n", innerIndent))
				sb.WriteString(fmt.Sprintf("%s\t\t}\n", innerIndent))
				sb.WriteString(fmt.Sprintf("%s\t\tif parameter.%s != \"\" {\n", innerIndent, config.ContainerNameParam))
				sb.WriteString(fmt.Sprintf("%s\t\t\tcontainerName: parameter.%s\n", innerIndent, config.ContainerNameParam))
				sb.WriteString(fmt.Sprintf("%s\t\t}\n", innerIndent))
			}

			// Map flat parameters
			for _, field := range config.PatchFields {
				g.writePatchParamMapping(sb, field, innerIndent+"\t\t", "parameter.")
			}

			// Map grouped parameters
			for _, group := range config.Groups {
				g.writePatchGroupMapping(sb, group, innerIndent+"\t\t", "parameter.")
			}

			sb.WriteString(fmt.Sprintf("%s\t}}\n", innerIndent))
			sb.WriteString(fmt.Sprintf("%s}]\n", innerIndent))
		}

		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	}

	// Determine the multi-container parameter name (for parameter block)
	multiParam := config.ContainersParam
	if multiParam == "" && config.MultiContainerParam != "" {
		multiParam = config.MultiContainerParam
	}

	// Generate parameter block
	switch {
	case config.CustomParameterBlock != "":
		// Use custom parameter block
		sb.WriteString(fmt.Sprintf("%sparameter: ", indent))
		for i, line := range strings.Split(strings.TrimSpace(config.CustomParameterBlock), "\n") {
			if i == 0 {
				sb.WriteString(fmt.Sprintf("%s\n", line))
			} else {
				sb.WriteString(fmt.Sprintf("%s%s\n", indent, line))
			}
		}
	case config.AllowMultiple && multiParam != "":
		defaultMarker := "*"
		if config.NoDefaultDisjunction {
			defaultMarker = ""
		}
		sb.WriteString(fmt.Sprintf("%sparameter: %s#%s | close({\n", indent, defaultMarker, paramsTypeName))
		containersDesc := config.ContainersDescription
		if containersDesc == "" {
			containersDesc = "Specify the settings for multiple containers"
		}
		sb.WriteString(fmt.Sprintf("%s// +usage=%s\n", innerIndent, containersDesc))
		sb.WriteString(fmt.Sprintf("%s%s: [...#%s]\n", innerIndent, multiParam, paramsTypeName))
		sb.WriteString(fmt.Sprintf("%s})\n", indent))
	default:
		sb.WriteString(fmt.Sprintf("%sparameter: #%s\n", indent, paramsTypeName))
	}

	// Generate errs aggregation
	sb.WriteString(fmt.Sprintf("%serrs: [for c in patch.spec.template.spec.containers if c.err != _|_ {c.err}]\n", indent))
}

// writePatchParamField writes a single field in the #PatchParams schema.
func (g *TraitCUEGenerator) writePatchParamField(sb *strings.Builder, field PatchContainerField, indent string) {
	desc := field.Description
	if desc == "" {
		desc = fmt.Sprintf("Specify the %s of the container", field.ParamName)
	}
	sb.WriteString(fmt.Sprintf("%s// +usage=%s\n", indent, desc))

	// Determine the type string
	typeStr := field.ParamType
	if typeStr == "" {
		// Infer type from field name/target
		switch field.TargetField {
		case "image":
			typeStr = string(ParamTypeString)
		case "imagePullPolicy":
			typeStr = "\"IfNotPresent\" | \"Always\" | \"Never\""
		case "command", "args":
			typeStr = "[...string]"
		default:
			typeStr = string(ParamTypeString)
		}
	}

	// Determine default value and optionality
	defaultVal := field.ParamDefault
	optional := ""
	if defaultVal == "" && field.Condition != "" {
		// Has condition, likely optional - choose appropriate default
		if field.Condition == "!= \"\"" {
			// String-equality condition: default to empty string
			defaultVal = "\"\""
		} else {
			// Non-string condition (e.g. != _|_): make field optional
			optional = "?"
		}
	}

	if defaultVal != "" {
		sb.WriteString(fmt.Sprintf("%s%s: *%s | %s\n", indent, field.ParamName, defaultVal, typeStr))
	} else {
		sb.WriteString(fmt.Sprintf("%s%s%s: %s\n", indent, field.ParamName, optional, typeStr))
	}
}

// writePatchParamGroup writes a group of fields in the #PatchParams schema.
func (g *TraitCUEGenerator) writePatchParamGroup(sb *strings.Builder, group PatchContainerGroup, indent string) {
	for _, field := range group.Fields {
		g.writePatchParamField(sb, field, indent)
	}
	for _, subGroup := range group.SubGroups {
		g.writePatchParamGroup(sb, subGroup, indent)
	}
}

// writePatchContainerField writes a field assignment in the PatchContainer body.
func (g *TraitCUEGenerator) writePatchContainerField(sb *strings.Builder, field PatchContainerField, indent string) {
	if field.Condition != "" {
		// Conditional field
		sb.WriteString(fmt.Sprintf("%sif _params.%s %s {\n", indent, field.ParamName, field.Condition))
		if field.PatchStrategy != "" {
			sb.WriteString(fmt.Sprintf("%s\t// +patchStrategy=%s\n", indent, field.PatchStrategy))
		}
		sb.WriteString(fmt.Sprintf("%s\t%s: _params.%s\n", indent, field.TargetField, field.ParamName))
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	} else {
		// Always apply
		if field.PatchStrategy != "" {
			sb.WriteString(fmt.Sprintf("%s// +patchStrategy=%s\n", indent, field.PatchStrategy))
		}
		sb.WriteString(fmt.Sprintf("%s%s: _params.%s\n", indent, field.TargetField, field.ParamName))
	}
}

// writePatchContainerGroup writes a grouped field block in the PatchContainer body.
// Generates: targetField: { field1: _params.field1, if _params.field2 != _|_ { field2: _params.field2 } }
func (g *TraitCUEGenerator) writePatchContainerGroup(sb *strings.Builder, group PatchContainerGroup, indent string) {
	sb.WriteString(fmt.Sprintf("%s%s: {\n", indent, group.TargetField))
	innerIndent := indent + "\t"

	// Write fields in this group
	for _, field := range group.Fields {
		if field.Condition != "" {
			sb.WriteString(fmt.Sprintf("%sif _params.%s %s {\n", innerIndent, field.ParamName, field.Condition))
			sb.WriteString(fmt.Sprintf("%s\t%s: _params.%s\n", innerIndent, field.TargetField, field.ParamName))
			sb.WriteString(fmt.Sprintf("%s}\n", innerIndent))
		} else {
			sb.WriteString(fmt.Sprintf("%s%s: _params.%s\n", innerIndent, field.TargetField, field.ParamName))
		}
	}

	// Write nested subgroups
	for _, subGroup := range group.SubGroups {
		g.writePatchContainerGroup(sb, subGroup, innerIndent)
	}

	sb.WriteString(fmt.Sprintf("%s}\n", indent))
}

// writePatchParamMapping writes a parameter mapping in the patch block.
// Fields with a "!= _|_" existence condition and no ParamDefault are truly optional
// (defined as field?: type in the schema) and need conditional guards to avoid
// propagating _|_ when the parameter is unset. Fields with defaults or value-based
// conditions (like '!= ""') always have a value and can be mapped unconditionally.
func (g *TraitCUEGenerator) writePatchParamMapping(sb *strings.Builder, field PatchContainerField, indent string, prefix string) {
	if field.Condition == "!= _|_" && field.ParamDefault == "" && field.ParamType == "" {
		sb.WriteString(fmt.Sprintf("%sif %s%s %s {\n", indent, prefix, field.ParamName, field.Condition))
		sb.WriteString(fmt.Sprintf("%s\t%s: %s%s\n", indent, field.ParamName, prefix, field.ParamName))
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	} else {
		sb.WriteString(fmt.Sprintf("%s%s: %s%s\n", indent, field.ParamName, prefix, field.ParamName))
	}
}

// writePatchGroupMapping writes a group parameter mapping in the patch block.
func (g *TraitCUEGenerator) writePatchGroupMapping(sb *strings.Builder, group PatchContainerGroup, indent string, prefix string) {
	for _, field := range group.Fields {
		g.writePatchParamMapping(sb, field, indent, prefix)
	}
	for _, subGroup := range group.SubGroups {
		g.writePatchGroupMapping(sb, subGroup, indent, prefix)
	}
}

// GenerateDefinitionWithRawTemplate generates a trait definition with fluent API header and raw CUE template.
// This combines the best of both worlds:
// - Header (name, type, description, labels, attributes) from fluent API
// - Template content (patch, outputs, parameter) from raw CUE string
func (g *TraitCUEGenerator) GenerateDefinitionWithRawTemplate(t *TraitDefinition, rawTemplate string) string {
	var sb strings.Builder

	// Write imports if any
	if len(g.imports) > 0 {
		sb.WriteString("import (\n")
		for _, imp := range g.imports {
			sb.WriteString(fmt.Sprintf("\t%q\n", imp))
		}
		sb.WriteString(")\n\n")
	}

	// Write trait header
	sb.WriteString(fmt.Sprintf("%s: {\n", cueLabel(t.GetName())))
	sb.WriteString(fmt.Sprintf("%stype: \"trait\"\n", g.indent))
	sb.WriteString(fmt.Sprintf("%sannotations: {}\n", g.indent))

	// Write labels block when Labels() was explicitly called (nil check distinguishes unset from empty)
	if t.labels != nil {
		if len(t.labels) > 0 {
			sb.WriteString(fmt.Sprintf("%slabels: {\n", g.indent))
			for k, v := range t.labels {
				sb.WriteString(fmt.Sprintf("%s\t%q: %q\n", g.indent, k, v))
			}
			sb.WriteString(fmt.Sprintf("%s}\n", g.indent))
		} else {
			sb.WriteString(fmt.Sprintf("%slabels: {}\n", g.indent))
		}
	}
	sb.WriteString(fmt.Sprintf("%sdescription: %q\n", g.indent, t.GetDescription()))

	// Write attributes
	sb.WriteString(fmt.Sprintf("%sattributes: {\n", g.indent))
	g.writeAttributes(&sb, t, 2)
	sb.WriteString(fmt.Sprintf("%s}\n", g.indent))

	sb.WriteString("}\n")

	// Write template section with raw CUE content
	sb.WriteString("template: {\n")
	// Indent each line of the raw template
	for _, line := range strings.Split(strings.TrimSpace(rawTemplate), "\n") {
		sb.WriteString(fmt.Sprintf("%s%s\n", g.indent, line))
	}
	sb.WriteString("}\n")

	return sb.String()
}
