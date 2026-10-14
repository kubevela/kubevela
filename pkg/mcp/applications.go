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
	"strings"
	"time"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// ApplicationRef names an Application. Namespace is required so a tool
// never guesses; agents that omit it get a clear error, not a default.
type ApplicationRef struct {
	Name      string `json:"name" jsonschema:"Application name."`
	Namespace string `json:"namespace" jsonschema:"Application namespace."`
}

func (r ApplicationRef) validate() error {
	if r.Name == "" {
		return fmt.Errorf("name is required")
	}
	if r.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	return nil
}

// ComponentRef is a component's name and definition type.
type ComponentRef struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// WorkflowSummary is the workflow state an operator checks first.
type WorkflowSummary struct {
	Mode       string `json:"mode,omitempty"`
	Phase      string `json:"phase,omitempty"`
	Suspended  bool   `json:"suspended"`
	Terminated bool   `json:"terminated"`
	Finished   bool   `json:"finished"`
	Message    string `json:"message,omitempty"`
}

// ApplicationSummary is one row of list_applications and the head of
// get_application.
type ApplicationSummary struct {
	Name       string           `json:"name"`
	Namespace  string           `json:"namespace"`
	Phase      string           `json:"phase"`
	Healthy    bool             `json:"healthy"`
	Components []ComponentRef   `json:"components"`
	Workflow   *WorkflowSummary `json:"workflow,omitempty"`
	Revision   string           `json:"revision,omitempty"`
	// StatusStale is set when the controller has not yet observed the
	// latest spec, so status describes an older generation.
	StatusStale bool      `json:"statusStale"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ListApplicationsInput filters list_applications.
type ListApplicationsInput struct {
	Namespace string `json:"namespace,omitempty" jsonschema:"Namespace to list. Empty lists every namespace the server allows."`
}

// ListApplicationsOutput is the structured result of list_applications.
type ListApplicationsOutput struct {
	Applications []ApplicationSummary `json:"applications"`
	Meta         Meta                 `json:"meta"`
}

// GetApplicationInput names the Application to read.
type GetApplicationInput = ApplicationRef

// ComponentSpec is a component as declared, with secret-like values removed.
type ComponentSpec struct {
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
	Traits     []TraitSpec    `json:"traits,omitempty"`
	DependsOn  []string       `json:"dependsOn,omitempty"`
}

// TraitSpec is a trait as declared, with secret-like values removed.
type TraitSpec struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
}

// PolicySpec is a policy as declared, with secret-like values removed.
type PolicySpec struct {
	Name       string         `json:"name,omitempty"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
}

// SubStepSpec is a step inside a step group as declared. Sub-steps do not
// nest, so this type has no children; MCP output schemas cannot be
// recursive.
type SubStepSpec struct {
	Name      string   `json:"name,omitempty"`
	Type      string   `json:"type"`
	If        string   `json:"if,omitempty"`
	Timeout   string   `json:"timeout,omitempty"`
	DependsOn []string `json:"dependsOn,omitempty"`
}

// StepSpec is a workflow step as declared. Properties are omitted; the
// step definition's schema says what they mean and they often carry
// credentials for external systems.
type StepSpec struct {
	SubStepSpec
	SubSteps []SubStepSpec `json:"subSteps,omitempty"`
}

// WorkflowSpec is the declared workflow.
type WorkflowSpec struct {
	Ref   string     `json:"ref,omitempty"`
	Mode  string     `json:"mode,omitempty"`
	Steps []StepSpec `json:"steps"`
}

// ApplicationSpecView is the declared Application after redaction.
type ApplicationSpecView struct {
	Components []ComponentSpec `json:"components"`
	Policies   []PolicySpec    `json:"policies,omitempty"`
	Workflow   *WorkflowSpec   `json:"workflow,omitempty"`
}

// TraitStatus is the controller's view of one trait on one component.
type TraitStatus struct {
	Type    string `json:"type"`
	Healthy bool   `json:"healthy"`
	Pending bool   `json:"pending,omitempty"`
	Message string `json:"message,omitempty"`
}

// ServiceStatus is the controller's view of one component in one place.
type ServiceStatus struct {
	Name      string            `json:"name"`
	Cluster   string            `json:"cluster,omitempty"`
	Namespace string            `json:"namespace,omitempty"`
	Env       string            `json:"env,omitempty"`
	Workload  string            `json:"workload,omitempty"`
	Healthy   bool              `json:"healthy"`
	Message   string            `json:"message,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
	Traits    []TraitStatus     `json:"traits,omitempty"`
}

// Condition is a status condition reduced to what a reader acts on.
type Condition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// AppliedResourceRef is one object the workflow applied, as recorded in
// status. It is a pointer for get_managed_resource, not a live read.
type AppliedResourceRef struct {
	Cluster    string `json:"cluster"`
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
	Creator    string `json:"creator,omitempty"`
}

// ApplicationStatusView is status as the controller last wrote it.
type ApplicationStatusView struct {
	Phase              string               `json:"phase"`
	ObservedGeneration int64                `json:"observedGeneration"`
	Generation         int64                `json:"generation"`
	Services           []ServiceStatus      `json:"services"`
	Conditions         []Condition          `json:"conditions,omitempty"`
	AppliedResources   []AppliedResourceRef `json:"appliedResources,omitempty"`
	Workflow           *WorkflowDetail      `json:"workflow,omitempty"`
}

// GetApplicationOutput is the structured result of get_application.
type GetApplicationOutput struct {
	ApplicationSummary
	Labels      map[string]string     `json:"labels,omitempty"`
	Annotations map[string]string     `json:"annotations,omitempty"`
	Spec        ApplicationSpecView   `json:"spec"`
	Status      ApplicationStatusView `json:"status"`
	// Evidence lists the objects this answer was derived from.
	Evidence []string `json:"evidence"`
	Meta     Meta     `json:"meta"`
}

// SubStepStatus is one sub-step as last recorded.
type SubStepStatus struct {
	ID               string     `json:"id,omitempty"`
	Name             string     `json:"name,omitempty"`
	Type             string     `json:"type,omitempty"`
	Phase            string     `json:"phase,omitempty"`
	Reason           string     `json:"reason,omitempty"`
	Message          string     `json:"message,omitempty"`
	FirstExecuteTime *time.Time `json:"firstExecuteTime,omitempty"`
	LastExecuteTime  *time.Time `json:"lastExecuteTime,omitempty"`
}

// StepStatus is one workflow step as last recorded, with its sub-steps.
type StepStatus struct {
	SubStepStatus
	SubSteps []SubStepStatus `json:"subSteps,omitempty"`
}

// WorkflowDetail is the full workflow status with the next operator action
// spelled out, so an agent reports it rather than inventing one.
type WorkflowDetail struct {
	WorkflowSummary
	AppRevision  string       `json:"appRevision,omitempty"`
	SuspendState string       `json:"suspendState,omitempty"`
	StartTime    *time.Time   `json:"startTime,omitempty"`
	EndTime      *time.Time   `json:"endTime,omitempty"`
	Steps        []StepStatus `json:"steps"`
	// SuspendedSteps names the steps waiting for a human.
	SuspendedSteps []string `json:"suspendedSteps,omitempty"`
	// FailedSteps names the steps that failed.
	FailedSteps []string `json:"failedSteps,omitempty"`
	// NextAction is the vela command that moves the workflow forward, or
	// empty when nothing is waiting on an operator.
	NextAction string `json:"nextAction,omitempty"`
}

// GetWorkflowInput names the Application whose workflow to read.
type GetWorkflowInput = ApplicationRef

// GetWorkflowOutput is the structured result of get_workflow.
type GetWorkflowOutput struct {
	Application string        `json:"application"`
	Namespace   string        `json:"namespace"`
	Spec        *WorkflowSpec `json:"spec,omitempty"`
	Status      *WorkflowDetail
	Evidence    []string `json:"evidence"`
	Meta        Meta     `json:"meta"`
}

func (d *deps) listApplications(ctx context.Context, _ *sdk.CallToolRequest, in ListApplicationsInput) (*sdk.CallToolResult, ListApplicationsOutput, error) {
	out := ListApplicationsOutput{Applications: []ApplicationSummary{}}
	cli := d.f.Client()
	if cli == nil {
		return nil, out, errNoConfig
	}
	namespaces, err := d.applicationNamespaces(in.Namespace)
	if err != nil {
		return nil, out, err
	}
	paged := false
	for _, ns := range namespaces {
		var list v1beta1.ApplicationList
		opts := []client.ListOption{client.Limit(int64(maxListItems) + 1)}
		if ns != "" {
			opts = append(opts, client.InNamespace(ns))
		}
		if err := cli.List(ctx, &list, opts...); err != nil {
			return nil, out, fmt.Errorf("list applications in %s: %w", nsLabel(ns), err)
		}
		paged = paged || list.Continue != ""
		for i := range list.Items {
			out.Applications = append(out.Applications, summarizeApplication(&list.Items[i]))
		}
	}
	sort.SliceStable(out.Applications, func(i, j int) bool {
		a, b := out.Applications[i], out.Applications[j]
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		return a.Name < b.Name
	})
	out.Meta = d.meta(ctx)
	if paged || len(out.Applications) > maxListItems {
		if len(out.Applications) > maxListItems {
			out.Applications = out.Applications[:maxListItems]
		}
		out.Meta.Truncated = true
		out.Meta.Warnings = append(out.Meta.Warnings, fmt.Sprintf("more than %d applications; narrow by namespace", maxListItems))
	}
	return nil, out, nil
}

func (d *deps) getApplication(ctx context.Context, _ *sdk.CallToolRequest, in GetApplicationInput) (*sdk.CallToolResult, GetApplicationOutput, error) {
	var out GetApplicationOutput
	app, err := d.loadApplication(ctx, in)
	if err != nil {
		return nil, out, err
	}
	out.ApplicationSummary = summarizeApplication(app)
	out.Labels = redactStringMap(app.Labels)
	out.Annotations = redactStringMap(app.Annotations)
	out.Spec = viewApplicationSpec(app)
	out.Status = viewApplicationStatus(app)
	out.Evidence = []string{applicationEvidence(app)}
	out.Meta = d.meta(ctx)
	return nil, out, nil
}

func (d *deps) getWorkflow(ctx context.Context, _ *sdk.CallToolRequest, in GetWorkflowInput) (*sdk.CallToolResult, GetWorkflowOutput, error) {
	var out GetWorkflowOutput
	app, err := d.loadApplication(ctx, in)
	if err != nil {
		return nil, out, err
	}
	out.Application, out.Namespace = app.Name, app.Namespace
	if app.Spec.Workflow != nil {
		spec := viewWorkflowSpec(app.Spec.Workflow)
		out.Spec = &spec
	}
	out.Status = viewWorkflowStatus(app)
	out.Evidence = []string{applicationEvidence(app)}
	out.Meta = d.meta(ctx)
	return nil, out, nil
}

// loadApplication reads one Application after checking the namespace is
// allowed. The policy check comes first so a restricted server does not
// reveal whether an application exists elsewhere.
func (d *deps) loadApplication(ctx context.Context, ref ApplicationRef) (*v1beta1.Application, error) {
	if err := ref.validate(); err != nil {
		return nil, err
	}
	if err := d.namespaceAllowed(ref.Namespace); err != nil {
		return nil, err
	}
	cli := d.f.Client()
	if cli == nil {
		return nil, errNoConfig
	}
	app := &v1beta1.Application{}
	if err := cli.Get(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, app); err != nil {
		return nil, fmt.Errorf("get application %s/%s: %w", ref.Namespace, ref.Name, err)
	}
	return app, nil
}

// namespaceAllowed enforces Options.Namespaces. Without a restriction every
// namespace RBAC permits is allowed.
func (d *deps) namespaceAllowed(ns string) error {
	if len(d.opts.Namespaces) == 0 {
		return nil
	}
	for _, allowed := range d.opts.Namespaces {
		if allowed == ns {
			return nil
		}
	}
	return fmt.Errorf("namespace %q is outside this server's allowed namespaces (%s)", ns, strings.Join(d.opts.Namespaces, ", "))
}

// applicationNamespaces resolves which namespaces an application list
// covers: the requested one if allowed, else the allowed set, else all.
func (d *deps) applicationNamespaces(requested string) ([]string, error) {
	if requested != "" {
		if err := d.namespaceAllowed(requested); err != nil {
			return nil, err
		}
		return []string{requested}, nil
	}
	if len(d.opts.Namespaces) == 0 {
		return []string{""}, nil
	}
	return append([]string{}, d.opts.Namespaces...), nil
}

func summarizeApplication(app *v1beta1.Application) ApplicationSummary {
	s := ApplicationSummary{
		Name:        app.Name,
		Namespace:   app.Namespace,
		Phase:       string(app.Status.Phase),
		Healthy:     applicationHealthy(app),
		Components:  make([]ComponentRef, 0, len(app.Spec.Components)),
		StatusStale: app.Status.ObservedGeneration != 0 && app.Status.ObservedGeneration < app.Generation,
		CreatedAt:   app.CreationTimestamp.Time.UTC(),
	}
	for _, c := range app.Spec.Components {
		s.Components = append(s.Components, ComponentRef{Name: c.Name, Type: c.Type})
	}
	if app.Status.LatestRevision != nil {
		s.Revision = app.Status.LatestRevision.Name
	}
	if wf := app.Status.Workflow; wf != nil {
		sum := summarizeWorkflow(wf)
		s.Workflow = &sum
	}
	return s
}

func summarizeWorkflow(wf *oamcommon.WorkflowStatus) WorkflowSummary {
	return WorkflowSummary{
		Mode:       wf.Mode,
		Phase:      string(wf.Phase),
		Suspended:  wf.Suspend,
		Terminated: wf.Terminated,
		Finished:   wf.Finished,
		Message:    text(wf.Message),
	}
}

// applicationHealthy mirrors the rule vela status prints: a terminal or
// unhealthy phase, a failed step, missing service status, or any unhealthy
// service or trait means not healthy.
func applicationHealthy(app *v1beta1.Application) bool {
	switch app.Status.Phase {
	case oamcommon.ApplicationWorkflowFailed,
		oamcommon.ApplicationWorkflowTerminated,
		oamcommon.ApplicationUnhealthy,
		oamcommon.ApplicationDeleting:
		return false
	default:
	}
	if wf := app.Status.Workflow; wf != nil {
		for i := range wf.Steps {
			if wf.Steps[i].Phase == workflowv1alpha1.WorkflowStepPhaseFailed {
				return false
			}
			for _, sub := range wf.Steps[i].SubStepsStatus {
				if sub.Phase == workflowv1alpha1.WorkflowStepPhaseFailed {
					return false
				}
			}
		}
	}
	if len(app.Status.Services) == 0 {
		return false
	}
	for _, s := range app.Status.Services {
		if !s.Healthy {
			return false
		}
		for _, t := range s.Traits {
			if !t.Healthy {
				return false
			}
		}
	}
	return true
}

func viewApplicationSpec(app *v1beta1.Application) ApplicationSpecView {
	view := ApplicationSpecView{Components: make([]ComponentSpec, 0, len(app.Spec.Components))}
	for _, c := range app.Spec.Components {
		cs := ComponentSpec{
			Name:       c.Name,
			Type:       c.Type,
			Properties: rawToRedactedMap(c.Properties),
			DependsOn:  c.DependsOn,
		}
		for _, t := range c.Traits {
			cs.Traits = append(cs.Traits, TraitSpec{Type: t.Type, Properties: rawToRedactedMap(t.Properties)})
		}
		view.Components = append(view.Components, cs)
	}
	for _, p := range app.Spec.Policies {
		view.Policies = append(view.Policies, PolicySpec{Name: p.Name, Type: p.Type, Properties: rawToRedactedMap(p.Properties)})
	}
	if app.Spec.Workflow != nil {
		wf := viewWorkflowSpec(app.Spec.Workflow)
		view.Workflow = &wf
	}
	return view
}

func viewWorkflowSpec(wf *v1beta1.Workflow) WorkflowSpec {
	spec := WorkflowSpec{Ref: wf.Ref, Steps: make([]StepSpec, 0, len(wf.Steps))}
	if wf.Mode != nil {
		spec.Mode = string(wf.Mode.Steps)
	}
	for _, s := range wf.Steps {
		step := StepSpec{SubStepSpec: SubStepSpec{Name: s.Name, Type: s.Type, If: s.If, Timeout: s.Timeout, DependsOn: s.DependsOn}}
		for _, sub := range s.SubSteps {
			step.SubSteps = append(step.SubSteps, SubStepSpec{Name: sub.Name, Type: sub.Type, If: sub.If, Timeout: sub.Timeout, DependsOn: sub.DependsOn})
		}
		spec.Steps = append(spec.Steps, step)
	}
	return spec
}

func viewApplicationStatus(app *v1beta1.Application) ApplicationStatusView {
	st := ApplicationStatusView{
		Phase:              string(app.Status.Phase),
		ObservedGeneration: app.Status.ObservedGeneration,
		Generation:         app.Generation,
		Services:           make([]ServiceStatus, 0, len(app.Status.Services)),
		Workflow:           viewWorkflowStatus(app),
	}
	for _, s := range app.Status.Services {
		ss := ServiceStatus{
			Name:      s.Name,
			Cluster:   s.Cluster,
			Namespace: s.Namespace,
			Env:       s.Env,
			Healthy:   s.Healthy,
			Message:   text(s.Message),
			Details:   redactStringMap(s.Details),
		}
		if s.WorkloadDefinition.Kind != "" {
			ss.Workload = s.WorkloadDefinition.APIVersion + "/" + s.WorkloadDefinition.Kind
		}
		for _, t := range s.Traits {
			ss.Traits = append(ss.Traits, TraitStatus{Type: t.Type, Healthy: t.Healthy, Pending: t.Pending, Message: text(t.Message)})
		}
		st.Services = append(st.Services, ss)
	}
	for _, c := range app.Status.Conditions {
		st.Conditions = append(st.Conditions, Condition{
			Type:    string(c.Type),
			Status:  string(c.Status),
			Reason:  string(c.Reason),
			Message: text(c.Message),
		})
	}
	for _, r := range app.Status.AppliedResources {
		st.AppliedResources = append(st.AppliedResources, AppliedResourceRef{
			Cluster:    clusterName(r.Cluster),
			APIVersion: r.APIVersion,
			Kind:       r.Kind,
			Namespace:  r.Namespace,
			Name:       r.Name,
			Creator:    r.Creator,
		})
	}
	return st
}

func viewWorkflowStatus(app *v1beta1.Application) *WorkflowDetail {
	wf := app.Status.Workflow
	if wf == nil {
		return nil
	}
	detail := &WorkflowDetail{
		WorkflowSummary: summarizeWorkflow(wf),
		AppRevision:     wf.AppRevision,
		SuspendState:    wf.SuspendState,
		StartTime:       timePtr(wf.StartTime.Time),
		EndTime:         timePtr(wf.EndTime.Time),
		Steps:           make([]StepStatus, 0, len(wf.Steps)),
	}
	for _, s := range wf.Steps {
		step := StepStatus{SubStepStatus: viewStepStatus(s.StepStatus)}
		for _, sub := range s.SubStepsStatus {
			step.SubSteps = append(step.SubSteps, viewStepStatus(sub))
		}
		detail.Steps = append(detail.Steps, step)
		noteStep(detail, s.StepStatus)
		for _, sub := range s.SubStepsStatus {
			noteStep(detail, sub)
		}
	}
	switch {
	case wf.Suspend:
		detail.NextAction = fmt.Sprintf("vela workflow resume %s -n %s", app.Name, app.Namespace)
	case wf.Terminated || len(detail.FailedSteps) > 0:
		detail.NextAction = fmt.Sprintf("vela workflow restart %s -n %s", app.Name, app.Namespace)
	}
	return detail
}

func noteStep(detail *WorkflowDetail, s workflowv1alpha1.StepStatus) {
	switch s.Phase {
	case workflowv1alpha1.WorkflowStepPhaseSuspending:
		detail.SuspendedSteps = append(detail.SuspendedSteps, s.Name)
	case workflowv1alpha1.WorkflowStepPhaseFailed:
		detail.FailedSteps = append(detail.FailedSteps, s.Name)
	default:
	}
}

func viewStepStatus(s workflowv1alpha1.StepStatus) SubStepStatus {
	return SubStepStatus{
		ID:               s.ID,
		Name:             s.Name,
		Type:             s.Type,
		Phase:            string(s.Phase),
		Reason:           text(s.Reason),
		Message:          text(s.Message),
		FirstExecuteTime: timePtr(s.FirstExecuteTime.Time),
		LastExecuteTime:  timePtr(s.LastExecuteTime.Time),
	}
}

func applicationEvidence(app *v1beta1.Application) string {
	return fmt.Sprintf("Application %s/%s generation %d resourceVersion %s", app.Namespace, app.Name, app.Generation, app.ResourceVersion)
}

func clusterName(c string) string {
	if c == "" {
		return localCluster
	}
	return c
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}
