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

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/schema"
)

// Definition type names as users and the CLI spell them.
const (
	DefinitionTypeComponent    = "component"
	DefinitionTypeTrait        = "trait"
	DefinitionTypePolicy       = "policy"
	DefinitionTypeWorkflowStep = "workflow-step"
)

var definitionTypes = []string{DefinitionTypeComponent, DefinitionTypeTrait, DefinitionTypePolicy, DefinitionTypeWorkflowStep}

// DefinitionSummary is one row of list_definitions.
type DefinitionSummary struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Namespace      string `json:"namespace"`
	Description    string `json:"description,omitempty"`
	Version        string `json:"version,omitempty"`
	LatestRevision string `json:"latestRevision,omitempty"`
}

// ListDefinitionsInput filters list_definitions.
type ListDefinitionsInput struct {
	// Type limits the result to one definition type: component, trait, policy or workflow-step.
	Type string `json:"type,omitempty" jsonschema:"Limit to one definition type: component, trait, policy or workflow-step. Empty lists all four."`
	// Namespace limits the result to one namespace plus the system namespace.
	Namespace string `json:"namespace,omitempty" jsonschema:"Namespace to list in addition to the system namespace. Empty lists every namespace the caller can read."`
}

// ListDefinitionsOutput is the structured result of list_definitions.
type ListDefinitionsOutput struct {
	Definitions []DefinitionSummary `json:"definitions"`
	Meta        Meta                `json:"meta"`
}

// DescribeDefinitionInput names one definition.
type DescribeDefinitionInput struct {
	Name      string `json:"name" jsonschema:"Definition name as returned by list_definitions."`
	Type      string `json:"type,omitempty" jsonschema:"Definition type: component, trait, policy or workflow-step. When empty every type is tried in that order."`
	Namespace string `json:"namespace,omitempty" jsonschema:"Namespace to look in before the system namespace."`
}

// DescribeDefinitionOutput is the structured result of describe_definition.
type DescribeDefinitionOutput struct {
	DefinitionSummary
	// Parameters is the JSON Schema of the definition's parameter block.
	Parameters map[string]any `json:"parameters"`
	// Required lists the top-level parameters that have no default and so
	// must be supplied. The schema's own required list also includes
	// defaulted fields, as CUE reports them.
	Required []string `json:"required"`
	Meta     Meta     `json:"meta"`
}

func (d *deps) listDefinitions(ctx context.Context, _ *sdk.CallToolRequest, in ListDefinitionsInput) (*sdk.CallToolResult, ListDefinitionsOutput, error) {
	out := ListDefinitionsOutput{Definitions: []DefinitionSummary{}}
	typesToList := definitionTypes
	if in.Type != "" {
		if !validDefinitionType(in.Type) {
			return nil, out, fmt.Errorf("unknown definition type %q; use one of %s", in.Type, strings.Join(definitionTypes, ", "))
		}
		typesToList = []string{in.Type}
	}
	cli := d.f.Client()
	if cli == nil {
		return nil, out, errNoConfig
	}
	for _, ns := range d.listNamespaces(in.Namespace) {
		for _, t := range typesToList {
			items, err := d.listDefinitionType(ctx, cli, t, ns)
			if err != nil {
				return nil, out, fmt.Errorf("list %s definitions in %q: %w", t, nsLabel(ns), err)
			}
			out.Definitions = append(out.Definitions, items...)
		}
	}
	sort.SliceStable(out.Definitions, func(i, j int) bool {
		a, b := out.Definitions[i], out.Definitions[j]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		return a.Name < b.Name
	})
	out.Meta = d.meta(ctx)
	return nil, out, nil
}

func (d *deps) describeDefinition(ctx context.Context, _ *sdk.CallToolRequest, in DescribeDefinitionInput) (*sdk.CallToolResult, DescribeDefinitionOutput, error) {
	var out DescribeDefinitionOutput
	if in.Name == "" {
		return nil, out, fmt.Errorf("name is required")
	}
	if in.Type != "" && !validDefinitionType(in.Type) {
		return nil, out, fmt.Errorf("unknown definition type %q; use one of %s", in.Type, strings.Join(definitionTypes, ", "))
	}
	cli := d.f.Client()
	if cli == nil {
		return nil, out, errNoConfig
	}
	summary, template, err := d.findDefinition(ctx, cli, in)
	if err != nil {
		return nil, out, err
	}
	out.DefinitionSummary = summary
	out.Parameters, out.Required, err = parameterSchema(ctx, template)
	if err != nil {
		return nil, out, fmt.Errorf("parameter schema for %s %q: %w", summary.Type, summary.Name, err)
	}
	out.Meta = d.meta(ctx)
	return nil, out, nil
}

// listNamespaces decides which namespaces a list covers. An explicit
// namespace is listed together with the system namespace; otherwise every
// namespace, narrowed by Options.Namespaces when set.
func (d *deps) listNamespaces(requested string) []string {
	if requested != "" {
		if requested == d.opts.SystemNamespace {
			return []string{requested}
		}
		return []string{requested, d.opts.SystemNamespace}
	}
	if len(d.opts.Namespaces) == 0 {
		return []string{""} // all namespaces
	}
	nss := append([]string{}, d.opts.Namespaces...)
	for _, ns := range nss {
		if ns == d.opts.SystemNamespace {
			return nss
		}
	}
	return append(nss, d.opts.SystemNamespace)
}

func (d *deps) listDefinitionType(ctx context.Context, cli client.Client, defType, ns string) ([]DefinitionSummary, error) {
	opts := []client.ListOption{}
	if ns != "" {
		opts = append(opts, client.InNamespace(ns))
	}
	var out []DefinitionSummary
	switch defType {
	case DefinitionTypeComponent:
		var l v1beta1.ComponentDefinitionList
		if err := cli.List(ctx, &l, opts...); err != nil {
			return nil, err
		}
		for i := range l.Items {
			it := &l.Items[i]
			out = append(out, summarize(defType, &it.ObjectMeta, it.Spec.Version, it.Status.LatestRevision))
		}
	case DefinitionTypeTrait:
		var l v1beta1.TraitDefinitionList
		if err := cli.List(ctx, &l, opts...); err != nil {
			return nil, err
		}
		for i := range l.Items {
			it := &l.Items[i]
			out = append(out, summarize(defType, &it.ObjectMeta, it.Spec.Version, it.Status.LatestRevision))
		}
	case DefinitionTypePolicy:
		var l v1beta1.PolicyDefinitionList
		if err := cli.List(ctx, &l, opts...); err != nil {
			return nil, err
		}
		for i := range l.Items {
			it := &l.Items[i]
			out = append(out, summarize(defType, &it.ObjectMeta, it.Spec.Version, it.Status.LatestRevision))
		}
	case DefinitionTypeWorkflowStep:
		var l v1beta1.WorkflowStepDefinitionList
		if err := cli.List(ctx, &l, opts...); err != nil {
			return nil, err
		}
		for i := range l.Items {
			it := &l.Items[i]
			out = append(out, summarize(defType, &it.ObjectMeta, it.Spec.Version, it.Status.LatestRevision))
		}
	}
	return out, nil
}

// findDefinition looks a definition up by name, trying the requested
// namespace then the system namespace, and each type in order when no type
// was given. It returns the summary and the CUE template.
func (d *deps) findDefinition(ctx context.Context, cli client.Client, in DescribeDefinitionInput) (DefinitionSummary, string, error) {
	namespaces := []string{d.opts.SystemNamespace}
	if in.Namespace != "" && in.Namespace != d.opts.SystemNamespace {
		namespaces = []string{in.Namespace, d.opts.SystemNamespace}
	}
	typesToTry := definitionTypes
	if in.Type != "" {
		typesToTry = []string{in.Type}
	}
	for _, ns := range namespaces {
		for _, t := range typesToTry {
			summary, tpl, err := getDefinition(ctx, cli, t, ns, in.Name)
			if err == nil {
				return summary, tpl, nil
			}
			if !apierrors.IsNotFound(err) {
				return DefinitionSummary{}, "", fmt.Errorf("get %s %q in %q: %w", t, in.Name, ns, err)
			}
		}
	}
	return DefinitionSummary{}, "", fmt.Errorf("definition %q not found in %s", in.Name, strings.Join(namespaces, ", "))
}

func getDefinition(ctx context.Context, cli client.Client, defType, ns, name string) (DefinitionSummary, string, error) {
	key := client.ObjectKey{Namespace: ns, Name: name}
	switch defType {
	case DefinitionTypeComponent:
		var o v1beta1.ComponentDefinition
		if err := cli.Get(ctx, key, &o); err != nil {
			return DefinitionSummary{}, "", err
		}
		return summarize(defType, &o.ObjectMeta, o.Spec.Version, o.Status.LatestRevision), cueTemplate(o.Spec.Schematic), nil
	case DefinitionTypeTrait:
		var o v1beta1.TraitDefinition
		if err := cli.Get(ctx, key, &o); err != nil {
			return DefinitionSummary{}, "", err
		}
		return summarize(defType, &o.ObjectMeta, o.Spec.Version, o.Status.LatestRevision), cueTemplate(o.Spec.Schematic), nil
	case DefinitionTypePolicy:
		var o v1beta1.PolicyDefinition
		if err := cli.Get(ctx, key, &o); err != nil {
			return DefinitionSummary{}, "", err
		}
		return summarize(defType, &o.ObjectMeta, o.Spec.Version, o.Status.LatestRevision), cueTemplate(o.Spec.Schematic), nil
	case DefinitionTypeWorkflowStep:
		var o v1beta1.WorkflowStepDefinition
		if err := cli.Get(ctx, key, &o); err != nil {
			return DefinitionSummary{}, "", err
		}
		return summarize(defType, &o.ObjectMeta, o.Spec.Version, o.Status.LatestRevision), cueTemplate(o.Spec.Schematic), nil
	}
	return DefinitionSummary{}, "", fmt.Errorf("unknown definition type %q", defType)
}

func summarize(defType string, meta interface {
	GetName() string
	GetNamespace() string
	GetAnnotations() map[string]string
}, ver string, rev *oamcommon.Revision) DefinitionSummary {
	s := DefinitionSummary{
		Name:        meta.GetName(),
		Type:        defType,
		Namespace:   meta.GetNamespace(),
		Description: meta.GetAnnotations()[types.AnnoDefinitionDescription],
		Version:     ver,
	}
	if rev != nil {
		s.LatestRevision = rev.Name
	}
	return s
}

func cueTemplate(s *oamcommon.Schematic) string {
	if s == nil || s.CUE == nil {
		return ""
	}
	return s.CUE.Template
}

// parameterSchema turns a definition's CUE template into the JSON Schema of
// its parameter block. It uses the same generator the controller uses to
// publish definition schema ConfigMaps, so an agent sees exactly what
// VelaUX and vela show see.
func parameterSchema(ctx context.Context, template string) (map[string]any, []string, error) {
	if strings.TrimSpace(template) == "" {
		return map[string]any{}, []string{}, nil
	}
	sch, err := schema.ParsePropertiesToSchema(ctx, template)
	if err != nil {
		return nil, nil, err
	}
	raw, err := json.Marshal(sch)
	if err != nil {
		return nil, nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, nil, err
	}
	return out, mustSupply(sch), nil
}

// mustSupply lists the top-level parameters a user has to provide. CUE marks
// every non-optional field required, including fields with a default, so the
// schema's own required list over-reports; a field with a default is dropped.
func mustSupply(sch *openapi3.Schema) []string {
	required := []string{}
	for _, name := range sch.Required {
		if prop, ok := sch.Properties[name]; ok && prop.Value != nil && prop.Value.Default != nil {
			continue
		}
		required = append(required, name)
	}
	return required
}

func validDefinitionType(t string) bool {
	for _, d := range definitionTypes {
		if d == t {
			return true
		}
	}
	return false
}

func nsLabel(ns string) string {
	if ns == "" {
		return "all namespaces"
	}
	return ns
}
