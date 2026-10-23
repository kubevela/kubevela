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
	"fmt"
	"sort"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const (
	maxRevisionsListed = 10
	maxConsumersListed = 20
	// maxConsumerScan bounds the Applications read per namespace when
	// counting consumers; the count says when it hit the bound.
	maxConsumerScan = 2000
)

// ExplainDefinitionInput names the definition to explain.
type ExplainDefinitionInput = DescribeDefinitionInput

// TraitBehaviour is what a trait definition declares about how it applies.
type TraitBehaviour struct {
	AppliesToWorkloads []string `json:"appliesToWorkloads,omitempty"`
	ConflictsWith      []string `json:"conflictsWith,omitempty"`
	PodDisruptive      bool     `json:"podDisruptive"`
	ManageWorkload     bool     `json:"manageWorkload,omitempty"`
	ControlPlaneOnly   bool     `json:"controlPlaneOnly,omitempty"`
	Stage              string   `json:"stage,omitempty"`
	WorkloadRefPath    string   `json:"workloadRefPath,omitempty"`
}

// WorkloadInfo is the workload a component definition declares it produces.
type WorkloadInfo struct {
	Type       string `json:"type,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

// HealthRules are the CUE expressions the controller evaluates for health
// and custom status. They are returned verbatim; an agent reads them, it
// does not evaluate them.
type HealthRules struct {
	HealthPolicy string `json:"healthPolicy,omitempty"`
	CustomStatus string `json:"customStatus,omitempty"`
	Details      string `json:"details,omitempty"`
}

// RevisionSummary is one DefinitionRevision.
type RevisionSummary struct {
	Name      string     `json:"name"`
	Revision  int64      `json:"revision"`
	Hash      string     `json:"hash,omitempty"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
}

// ConsumerSummary counts the Applications that use a definition.
type ConsumerSummary struct {
	Count int `json:"count"`
	// Applications lists up to maxConsumersListed consumers as namespace/name.
	Applications []string `json:"applications"`
	// NamespacesScanned says what the count covers: the allowed namespaces,
	// or "all" when the server has no restriction.
	NamespacesScanned []string `json:"namespacesScanned"`
	Error             string   `json:"error,omitempty"`
}

// ExplainDefinitionOutput is the structured result of explain_definition.
type ExplainDefinitionOutput struct {
	DefinitionSummary
	Alias      string `json:"alias,omitempty"`
	Deprecated bool   `json:"deprecated,omitempty"`
	Hidden     bool   `json:"hidden,omitempty"`
	Extends    string `json:"extends,omitempty"`
	Abstract   bool   `json:"abstract,omitempty"`
	// Workload is set for component definitions.
	Workload *WorkloadInfo `json:"workload,omitempty"`
	// Trait is set for trait definitions.
	Trait *TraitBehaviour `json:"trait,omitempty"`
	// RestrictedToNamespaces lists where the definition may be used when
	// the platform restricts it.
	RestrictedToNamespaces []string           `json:"restrictedToNamespaces,omitempty"`
	Produces               []ProducedResource `json:"produces" jsonschema:"Kinds the template emits, read from the template without rendering."`
	ParameterUsage         []ParameterUsage   `json:"parameterUsage" jsonschema:"Which template fields each parameter flows into."`
	Parameters             map[string]any     `json:"parameters" jsonschema:"JSON Schema of the parameter block."`
	Required               []string           `json:"required" jsonschema:"Top-level parameters with no default, which must be supplied."`
	Health                 HealthRules        `json:"health" jsonschema:"Health and custom status rules, verbatim CUE."`
	Revisions              []RevisionSummary  `json:"revisions" jsonschema:"DefinitionRevisions, newest first."`
	Consumers              ConsumerSummary    `json:"consumers" jsonschema:"Applications that use this definition, within the namespaces the server may read."`
	// InspectionError is set when the template could not be parsed; the
	// schema and metadata above are still valid.
	InspectionError string   `json:"inspectionError,omitempty" jsonschema:"Set when the template could not be parsed; the schema and metadata are still valid."`
	Evidence        []string `json:"evidence" jsonschema:"Objects this answer was derived from."`
	Meta            Meta     `json:"meta"`
}

// definitionFacts is the type-independent view of one definition object.
type definitionFacts struct {
	summary     DefinitionSummary
	template    string
	labels      map[string]string
	annotations map[string]string
	extends     string
	abstract    bool
	workload    *WorkloadInfo
	trait       *TraitBehaviour
	restricted  []string
	health      HealthRules
	revLabel    string
	resourceVer string
}

func (d *deps) explainDefinition(ctx context.Context, _ *sdk.CallToolRequest, in ExplainDefinitionInput) (*sdk.CallToolResult, ExplainDefinitionOutput, error) {
	out := ExplainDefinitionOutput{Produces: []ProducedResource{}, ParameterUsage: []ParameterUsage{}, Revisions: []RevisionSummary{}}
	if in.Name == "" {
		return nil, out, fmt.Errorf("name is required")
	}
	if in.Type != "" && !validDefinitionType(in.Type) {
		return nil, out, fmt.Errorf("unknown definition type %q", in.Type)
	}
	if err := d.definitionNamespaceAllowed(in.Namespace); err != nil {
		return nil, out, err
	}
	cli := d.f.Client()
	if cli == nil {
		return nil, out, errNoConfig
	}
	facts, err := d.findDefinitionFacts(ctx, cli, in)
	if err != nil {
		return nil, out, err
	}
	out.DefinitionSummary = facts.summary
	out.Alias = text(facts.annotations[types.AnnoDefinitionAlias])
	out.Deprecated = facts.labels[types.LabelDefinitionDeprecated] == "true"
	out.Hidden = facts.labels[types.LabelDefinitionHidden] == "true"
	out.Extends = facts.extends
	out.Abstract = facts.abstract
	out.Workload = facts.workload
	out.Trait = facts.trait
	out.RestrictedToNamespaces = facts.restricted
	out.Health = facts.health
	out.Evidence = []string{fmt.Sprintf("%s %s/%s resourceVersion %s", definitionKind(facts.summary.Type), facts.summary.Namespace, facts.summary.Name, facts.resourceVer)}

	inspected := inspectTemplate(facts.template)
	if inspected.Produces != nil {
		out.Produces = inspected.Produces
	}
	if inspected.Usage != nil {
		out.ParameterUsage = inspected.Usage
	}
	out.InspectionError = inspected.Err

	out.Parameters, out.Required, err = parameterSchema(ctx, facts.template)
	if err != nil {
		return nil, out, fmt.Errorf("parameter schema for %s %q: %w", facts.summary.Type, facts.summary.Name, err)
	}

	out.Revisions, err = listDefinitionRevisions(ctx, cli, facts)
	if err != nil {
		out.Meta.Warnings = append(out.Meta.Warnings, "revision history unavailable: "+redactLine(err.Error()))
	} else {
		out.Evidence = append(out.Evidence, fmt.Sprintf("%d DefinitionRevision objects labelled %s=%s", len(out.Revisions), facts.revLabel, facts.summary.Name))
	}
	out.Consumers = d.countConsumers(ctx, cli, facts.summary.Type, facts.summary.Name)

	warnings := out.Meta.Warnings
	out.Meta = d.meta(ctx)
	out.Meta.Warnings = append(out.Meta.Warnings, warnings...)
	return nil, out, nil
}

// findDefinitionFacts mirrors findDefinition's lookup order but keeps the
// whole object so type-specific facts can be read.
func (d *deps) findDefinitionFacts(ctx context.Context, cli client.Client, in ExplainDefinitionInput) (*definitionFacts, error) {
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
			facts, err := loadDefinitionFacts(ctx, cli, t, ns, in.Name)
			if err == nil {
				return facts, nil
			}
			if !apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("get %s %q in %q: %w", t, in.Name, ns, err)
			}
		}
	}
	return nil, fmt.Errorf("definition %q not found in %v", in.Name, namespaces)
}

func loadDefinitionFacts(ctx context.Context, cli client.Client, defType, ns, name string) (*definitionFacts, error) {
	key := client.ObjectKey{Namespace: ns, Name: name}
	switch defType {
	case DefinitionTypeComponent:
		var o v1beta1.ComponentDefinition
		if err := cli.Get(ctx, key, &o); err != nil {
			return nil, err
		}
		f := &definitionFacts{
			summary:     summarize(defType, &o.ObjectMeta, o.Spec.Version, o.Status.LatestRevision),
			template:    cueTemplate(o.Spec.Schematic),
			labels:      o.Labels,
			annotations: o.Annotations,
			extends:     o.Spec.Extends,
			abstract:    o.Spec.Abstract,
			health:      healthRules(o.Spec.Status),
			revLabel:    oam.LabelComponentDefinitionName,
			resourceVer: o.ResourceVersion,
		}
		if o.Spec.Workload.Type != "" || o.Spec.Workload.Definition.Kind != "" {
			f.workload = &WorkloadInfo{Type: o.Spec.Workload.Type, APIVersion: o.Spec.Workload.Definition.APIVersion, Kind: o.Spec.Workload.Definition.Kind}
		}
		return f, nil
	case DefinitionTypeTrait:
		var o v1beta1.TraitDefinition
		if err := cli.Get(ctx, key, &o); err != nil {
			return nil, err
		}
		f := &definitionFacts{
			summary:     summarize(defType, &o.ObjectMeta, o.Spec.Version, o.Status.LatestRevision),
			template:    cueTemplate(o.Spec.Schematic),
			labels:      o.Labels,
			annotations: o.Annotations,
			extends:     o.Spec.Extends,
			abstract:    o.Spec.Abstract,
			health:      healthRules(o.Spec.Status),
			revLabel:    oam.LabelTraitDefinitionName,
			resourceVer: o.ResourceVersion,
			trait: &TraitBehaviour{
				AppliesToWorkloads: o.Spec.AppliesToWorkloads,
				ConflictsWith:      o.Spec.ConflictsWith,
				PodDisruptive:      o.Spec.PodDisruptive,
				ManageWorkload:     o.Spec.ManageWorkload,
				ControlPlaneOnly:   o.Spec.ControlPlaneOnly,
				Stage:              string(o.Spec.Stage),
				WorkloadRefPath:    o.Spec.WorkloadRefPath,
			},
		}
		if o.Spec.Restrictions != nil {
			f.restricted = o.Spec.Restrictions.Namespaces
		}
		return f, nil
	case DefinitionTypePolicy:
		var o v1beta1.PolicyDefinition
		if err := cli.Get(ctx, key, &o); err != nil {
			return nil, err
		}
		return &definitionFacts{
			summary:     summarize(defType, &o.ObjectMeta, o.Spec.Version, o.Status.LatestRevision),
			template:    cueTemplate(o.Spec.Schematic),
			labels:      o.Labels,
			annotations: o.Annotations,
			revLabel:    oam.LabelPolicyDefinitionName,
			resourceVer: o.ResourceVersion,
		}, nil
	case DefinitionTypeWorkflowStep:
		var o v1beta1.WorkflowStepDefinition
		if err := cli.Get(ctx, key, &o); err != nil {
			return nil, err
		}
		return &definitionFacts{
			summary:     summarize(defType, &o.ObjectMeta, o.Spec.Version, o.Status.LatestRevision),
			template:    cueTemplate(o.Spec.Schematic),
			labels:      o.Labels,
			annotations: o.Annotations,
			revLabel:    oam.LabelWorkflowStepDefinitionName,
			resourceVer: o.ResourceVersion,
		}, nil
	}
	return nil, fmt.Errorf("unknown definition type %q", defType)
}

func healthRules(s *oamcommon.Status) HealthRules {
	if s == nil {
		return HealthRules{}
	}
	return HealthRules{HealthPolicy: redactLine(s.HealthPolicy), CustomStatus: redactLine(s.CustomStatus), Details: redactLine(s.Details)}
}

// listDefinitionRevisions returns the newest revisions first, capped.
func listDefinitionRevisions(ctx context.Context, cli client.Client, facts *definitionFacts) ([]RevisionSummary, error) {
	var list v1beta1.DefinitionRevisionList
	if err := cli.List(ctx, &list, client.InNamespace(facts.summary.Namespace), client.MatchingLabels{facts.revLabel: facts.summary.Name}); err != nil {
		return nil, err
	}
	revs := make([]RevisionSummary, 0, len(list.Items))
	for i := range list.Items {
		r := &list.Items[i]
		revs = append(revs, RevisionSummary{Name: r.Name, Revision: r.Spec.Revision, Hash: r.Spec.RevisionHash, CreatedAt: timePtr(r.CreationTimestamp.Time)})
	}
	sort.Slice(revs, func(i, j int) bool { return revs[i].Revision > revs[j].Revision })
	if len(revs) > maxRevisionsListed {
		revs = revs[:maxRevisionsListed]
	}
	return revs, nil
}

// countConsumers scans Applications in the allowed namespaces for uses of
// the definition. A list failure is reported in the summary, not as a tool
// error, because the rest of the explanation does not depend on it.
func (d *deps) countConsumers(ctx context.Context, cli client.Client, defType, name string) ConsumerSummary {
	sum := ConsumerSummary{Applications: []string{}, NamespacesScanned: []string{"all"}}
	namespaces := []string{""}
	if len(d.opts.Namespaces) > 0 {
		namespaces = append([]string{}, d.opts.Namespaces...)
		sum.NamespacesScanned = namespaces
	}
	for _, ns := range namespaces {
		var list v1beta1.ApplicationList
		opts := []client.ListOption{client.Limit(int64(maxConsumerScan))}
		if ns != "" {
			opts = append(opts, client.InNamespace(ns))
		}
		if err := cli.List(ctx, &list, opts...); err != nil {
			sum.Error = redactLine(fmt.Sprintf("list applications in %s: %v", nsLabel(ns), err))
			return sum
		}
		if list.Continue != "" {
			sum.Error = fmt.Sprintf("more than %d applications in %s; the count covers the first %d only", maxConsumerScan, nsLabel(ns), maxConsumerScan)
		}
		for i := range list.Items {
			app := &list.Items[i]
			if usesDefinition(app, defType, name) {
				sum.Count++
				if len(sum.Applications) < maxConsumersListed {
					sum.Applications = append(sum.Applications, app.Namespace+"/"+app.Name)
				}
			}
		}
	}
	return sum
}

func usesDefinition(app *v1beta1.Application, defType, name string) bool {
	switch defType {
	case DefinitionTypeComponent:
		for _, c := range app.Spec.Components {
			if c.Type == name {
				return true
			}
		}
	case DefinitionTypeTrait:
		for _, c := range app.Spec.Components {
			for _, t := range c.Traits {
				if t.Type == name {
					return true
				}
			}
		}
	case DefinitionTypePolicy:
		for _, p := range app.Spec.Policies {
			if p.Type == name {
				return true
			}
		}
	case DefinitionTypeWorkflowStep:
		if app.Spec.Workflow == nil {
			return false
		}
		for _, s := range app.Spec.Workflow.Steps {
			if s.Type == name {
				return true
			}
			for _, sub := range s.SubSteps {
				if sub.Type == name {
					return true
				}
			}
		}
	}
	return false
}

func definitionKind(defType string) string {
	switch defType {
	case DefinitionTypeComponent:
		return "ComponentDefinition"
	case DefinitionTypeTrait:
		return "TraitDefinition"
	case DefinitionTypePolicy:
		return "PolicyDefinition"
	case DefinitionTypeWorkflowStep:
		return "WorkflowStepDefinition"
	}
	return defType
}
