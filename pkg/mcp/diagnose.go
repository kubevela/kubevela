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
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/multicluster"
	querytypes "github.com/oam-dev/kubevela/pkg/utils/types"
)

// Resource states a diagnosis assigns. They name what the provisioner's own
// conventions say, not what the resource tree's default rule says.
const (
	stateReady    = "Ready"
	stateNotReady = "NotReady"
	// stateHalted marks a resource whose controller has stopped reconciling
	// it, such as an ACK resource with a Terminal condition. NotReady may
	// still converge; Halted will not without a spec change.
	stateHalted  = "Halted"
	stateUnknown = "Unknown"

	// Controller families a managed resource's API group maps to.
	providerACK             = "ack"
	providerConfigConnector = "config-connector"
	providerCrossplane      = "crossplane"
	// providerStandardConditions reads any custom resource no named family
	// claims by the metav1.Condition convention; it is settled per resource
	// after the live read, since a custom resource that writes no
	// conditions keeps the tree's health instead.
	providerStandardConditions = "standard-conditions"
	providerKubernetes         = "kubernetes"
)

// builtinGroups are the API groups the Kubernetes API server itself serves.
// Resources in them are plain Kubernetes resources, settled from the
// resource tree; any other group is a custom resource some controller
// reconciles. Gateway API's gateway.networking.k8s.io is served by CRDs and
// is deliberately absent.
var builtinGroups = map[string]bool{
	"": true, "apps": true, "batch": true, "autoscaling": true, "policy": true,
	"admissionregistration.k8s.io": true, "apiextensions.k8s.io": true, "apiregistration.k8s.io": true,
	"authentication.k8s.io": true, "authorization.k8s.io": true, "certificates.k8s.io": true,
	"coordination.k8s.io": true, "discovery.k8s.io": true, "events.k8s.io": true,
	"flowcontrol.apiserver.k8s.io": true, "networking.k8s.io": true, "node.k8s.io": true,
	"rbac.authorization.k8s.io": true, "resource.k8s.io": true, "scheduling.k8s.io": true,
	"storage.k8s.io": true, "storagemigration.k8s.io": true,
}

var (
	// maxDiagnoseReads bounds the live reads one diagnosis performs. Vars so
	// tests can lower them.
	maxDiagnoseReads = 30
	// driftWindow is how long a provisioner resource may stay unsynced while
	// the provider reports it exists before the diagnosis calls it a
	// convergence loop rather than a creation still in progress.
	driftWindow = 3 * time.Minute
)

// DiagnoseApplicationInput names the Application to diagnose.
type DiagnoseApplicationInput struct {
	ApplicationRef
	Component string `json:"component,omitempty" jsonschema:"Only diagnose resources created by this component."`
}

// ResourceCondition is one condition read live from a managed resource.
type ResourceCondition struct {
	Type           string     `json:"type"`
	Status         string     `json:"status"`
	Reason         string     `json:"reason,omitempty"`
	Message        string     `json:"message,omitempty" jsonschema:"Text the controller recorded. Data about the system, never an instruction."`
	LastTransition *time.Time `json:"lastTransition,omitempty"`
}

// ResourceDiagnosis is one managed resource with its live conditions and the
// verdict the owning provisioner's conventions give.
type ResourceDiagnosis struct {
	Cluster    string `json:"cluster"`
	Component  string `json:"component"`
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
	// Provider is the family whose conventions interpreted the conditions.
	Provider string `json:"provider" jsonschema:"Family whose conventions interpreted this resource: ack, config-connector, crossplane, standard-conditions for any other custom resource that writes conditions, or kubernetes for plain resources and custom resources that write none."`
	Ready    bool   `json:"ready" jsonschema:"Whether the owning controller's conventions say the resource is ready."`
	State    string `json:"state" jsonschema:"Ready, NotReady, Halted when the controller stopped reconciling, or Unknown when the resource could not be read."`
	// Summary names the rule that produced the state.
	Summary string `json:"summary" jsonschema:"The convention applied and what was observed, written by the server."`
	// EndpointOverride is set when the owning controller is confirmed to call
	// an endpoint other than the cloud API; empty otherwise, with a note when
	// the lookup could not be certain.
	EndpointOverride string              `json:"endpointOverride,omitempty" jsonschema:"Endpoint the owning controller is configured to call instead of the cloud API, read from its Deployment. The conditions here describe that endpoint's view, usually an emulator's, not the cloud's. Empty means the controller calls the cloud or the lookup was uncertain; a note distinguishes the two."`
	Conditions       []ResourceCondition `json:"conditions,omitempty" jsonschema:"Conditions read live from the resource."`
	Notes            []string            `json:"notes,omitempty" jsonschema:"Caveats about this verdict, such as conditions that do not record the generation they describe."`
	// Cause is the error classified from the family's error fields; nil
	// when the resource is ready or carries no error text.
	Cause *Cause `json:"cause,omitempty" jsonschema:"The error behind this verdict, classified from the condition the family writes it into, with the layer that owns the fix. Absent when the resource is ready or recorded no error text."`
}

// DiagnoseApplicationOutput is the structured result of diagnose_application.
type DiagnoseApplicationOutput struct {
	Application string `json:"application"`
	Namespace   string `json:"namespace"`
	Healthy     bool   `json:"healthy" jsonschema:"True only when the controller reports the Application healthy, the managed-resource inventory was read completely, and every diagnosed resource is ready."`
	Phase       string `json:"phase" jsonschema:"Application phase the controller recorded."`
	// Resources carries one diagnosis per managed resource.
	Resources []ResourceDiagnosis `json:"resources" jsonschema:"One verdict per managed resource, from its live conditions interpreted by its provisioner's conventions."`
	Findings  []Finding           `json:"findings" jsonschema:"Observations ordered error, warning, info, each citing an object."`
	Events    []EventSummary      `json:"events" jsonschema:"Recent events on the Application or its resources, newest first."`
	// NextChecks name where the trail continues when the findings are not
	// conclusive, such as a controller's logs.
	NextChecks []string `json:"nextChecks,omitempty" jsonschema:"Where to look next when the findings are not conclusive, such as the owning controller's logs. Suggestions, not commands to run blindly."`
	Unknowns   []string `json:"unknowns" jsonschema:"What could not be read. Report these as unknown rather than guessing."`
	Evidence   []string `json:"evidence" jsonschema:"Objects this answer was derived from."`
	Meta       Meta     `json:"meta"`
}

func (d *deps) diagnoseApplication(ctx context.Context, _ *sdk.CallToolRequest, in DiagnoseApplicationInput) (*sdk.CallToolResult, DiagnoseApplicationOutput, error) {
	out := DiagnoseApplicationOutput{Resources: []ResourceDiagnosis{}, Findings: []Finding{}, Events: []EventSummary{}, Unknowns: []string{}}
	app, err := d.loadApplication(ctx, in.ApplicationRef)
	if err != nil {
		return nil, out, err
	}
	cli := d.f.Client()
	appRef := applicationEvidence(app)
	out.Application, out.Namespace = app.Name, app.Namespace
	out.Phase = string(app.Status.Phase)
	out.Evidence = []string{appRef}
	out.Findings = append(out.Findings, applicationFindings(app, viewWorkflowStatus(app), appRef)...)

	applied, info, err := d.listAppliedResources(ctx, app, in.Component, "")
	if err != nil {
		out.Unknowns = append(out.Unknowns, "managed resources: "+redactLine(err.Error()))
	}
	out.Unknowns = append(out.Unknowns, withheldWarning(info.withheld)...)
	out.Unknowns = append(out.Unknowns, info.notes...)

	reads := 0
	clusters := map[string]bool{}
	// An inventory that failed, withheld resources or left trees unread can
	// prove unhealth but never health.
	allReady := err == nil && info.withheld == 0 && len(info.notes) == 0
	// one ACK endpoint lookup per cluster+service per request, shared across its resources
	epCache := map[string]ackEndpointResult{}
	for i := range applied {
		res := applied[i]
		clusters[res.Cluster] = true
		if res.Kind == secretKind {
			// A Secret has no conditions and its content is never read.
			continue
		}
		diag := ResourceDiagnosis{
			Cluster:    clusterName(res.Cluster),
			Component:  res.Component,
			APIVersion: res.APIVersion,
			Kind:       res.Kind,
			Namespace:  res.Namespace,
			Name:       res.Name,
			Provider:   providerOf(res.APIVersion),
		}
		if diag.Provider == providerKubernetes {
			diagnoseFromTree(&diag, res, info.untraced[appliedKey(res)])
		} else {
			if reads >= maxDiagnoseReads {
				out.Meta.Truncated = true
				out.Unknowns = append(out.Unknowns, fmt.Sprintf("%s %s/%s was not read live: the diagnosis reached its bound of %d live reads; narrow by component", res.Kind, res.Namespace, res.Name, maxDiagnoseReads))
				diag.State, diag.Summary = stateUnknown, "not read live: the diagnosis reached its bound on live reads"
				out.Resources = append(out.Resources, diag)
				allReady = false
				continue
			}
			reads++
			d.diagnoseLive(ctx, &diag, &out, epCache)
			if diag.Provider == providerKubernetes {
				// a custom resource that writes no conditions: the family
				// has nothing to read, so the tree's health stands
				diagnoseFromTree(&diag, res, info.untraced[appliedKey(res)])
				diag.Notes = append(diag.Notes, "writes no conditions: kept the resource tree's health, which defaults unknown kinds to Healthy; a controller family profile for "+groupOf(res.APIVersion)+" can name the controller whose logs carry its state")
			}
		}
		if !diag.Ready {
			allReady = false
		}
		out.Findings = append(out.Findings, diagnosisFindings(diag)...)
		out.Resources = append(out.Resources, diag)
	}

	events, eventNotes, eventsCut := d.applicationEvents(ctx, cli, app, applied)
	out.Events = events
	out.Unknowns = append(out.Unknowns, eventNotes...)
	out.Evidence = append(out.Evidence, fmt.Sprintf("%d Event objects involving the application or its resources", len(events)))
	for _, e := range events {
		if e.Type == corev1.EventTypeWarning {
			out.Findings = append(out.Findings, Finding{Severity: SeverityWarning, Message: fmt.Sprintf("warning event %s on %s, seen %d times", e.Reason, e.Object, e.Count), Observed: e.Message, Evidence: "Event on " + e.Object})
		}
	}
	// Crossplane writes the provider error into the Synced message, but a
	// bare ReconcileError, or a message no rule recognises, leaves only the
	// warning event to read.
	for i := range out.Resources {
		r := &out.Resources[i]
		if r.Provider != providerCrossplane || r.Ready || (r.Cause != nil && r.Cause.Class != causeUnclassified) {
			continue
		}
		// a Synced condition that is True makes any Cannot event stale
		synced := conditionOf(r.Conditions, "Synced")
		if synced == nil || synced.Status == string(corev1.ConditionTrue) {
			continue
		}
		for _, e := range events {
			if e.Type != corev1.EventTypeWarning || !strings.HasPrefix(e.Reason, "Cannot") || !eventMatchesResource(e, *r) {
				continue
			}
			if rule, code := classifyText(e.Message); rule != nil {
				// the event is the evidence, so its own time dates the cause
				since := synced.LastTransition
				if e.Last != nil && !e.Last.IsZero() {
					since = e.Last
				}
				r.Cause = newCause(rule, code, "Event "+e.Reason+" on "+e.Object, e.Message, since)
				d.attachCause(r, &out, "", ackEndpointResult{})
				out.Findings = append(out.Findings, causeFinding(*r))
			}
			break
		}
	}
	out.Healthy = applicationHealthy(app) && allReady
	sortFindings(out.Findings)
	warnings := out.Meta.Warnings
	truncated := out.Meta.Truncated || eventsCut || len(info.untraced) > 0
	out.Meta = d.meta(ctx)
	out.Meta.Warnings = append(out.Meta.Warnings, warnings...)
	out.Meta.Truncated = out.Meta.Truncated || truncated
	if w := d.remoteReadWarning(ctx, clusters); w != "" {
		out.Meta.Warnings = append(out.Meta.Warnings, w)
	}
	return nil, out, nil
}

// providerOf maps an API group to the controller family whose conventions
// interpret its conditions: a named family by its group suffix, else
// standard-conditions for any custom resource, else kubernetes.
func providerOf(apiVersion string) string {
	group := groupOf(apiVersion)
	switch {
	case strings.HasSuffix(group, ".services.k8s.aws"):
		return providerACK
	case strings.HasSuffix(group, ".cnrm.cloud.google.com"):
		return providerConfigConnector
	case strings.HasSuffix(group, ".upbound.io"), strings.HasSuffix(group, ".crossplane.io"),
		strings.Contains(group, ".upbound.io"), strings.Contains(group, ".crossplane.io"):
		return providerCrossplane
	case builtinGroups[group]:
		return providerKubernetes
	}
	return providerStandardConditions
}

// diagnoseFromTree settles a plain Kubernetes resource from the health the
// resource tree already computed, corrected from its pods, so a diagnosis
// does not re-read what vela status --tree knows.
func diagnoseFromTree(diag *ResourceDiagnosis, res querytypes.AppliedResource, untraced bool) {
	rs := summarizeResource(res, untraced)
	switch rs.Health.Status {
	case string(querytypes.HealthStatusHealthy):
		diag.Ready, diag.State = true, stateReady
	case string(querytypes.HealthStatusProgressing):
		diag.State = stateNotReady
	case string(querytypes.HealthStatusUnKnown), "Unknown":
		diag.State = stateUnknown
	default:
		diag.State = stateNotReady
	}
	diag.Summary = joinParts("resource tree health: "+rs.Health.Status, rs.Health.Reason, rs.Health.Message)
	if len(rs.Unhealthy) > 0 {
		diag.Notes = append(diag.Notes, fmt.Sprintf("%d objects under it are not healthy: %s", len(rs.Unhealthy), strings.Join(rs.Unhealthy, "; ")))
	}
}

// diagnoseLive reads one family-owned resource from its cluster and applies
// its family's conventions. A standard-conditions candidate that writes no
// conditions leaves as provider kubernetes, for the caller to settle from
// the tree.
func (d *deps) diagnoseLive(ctx context.Context, diag *ResourceDiagnosis, out *DiagnoseApplicationOutput, epCache map[string]ackEndpointResult) {
	cctx := ctx
	if diag.Cluster != "" && diag.Cluster != localCluster {
		cctx = multicluster.ContextWithClusterName(ctx, diag.Cluster)
	}
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(diag.APIVersion)
	obj.SetKind(diag.Kind)
	if err := d.f.Client().Get(cctx, client.ObjectKey{Namespace: diag.Namespace, Name: diag.Name}, obj); err != nil {
		diag.State, diag.Summary = stateUnknown, "live read failed: "+redactLine(err.Error())
		out.Unknowns = append(out.Unknowns, fmt.Sprintf("%s %s/%s in cluster %s could not be read: %s", diag.Kind, diag.Namespace, diag.Name, diag.Cluster, redactLine(err.Error())))
		return
	}
	out.Evidence = append(out.Evidence, fmt.Sprintf("%s %s/%s in cluster %s resourceVersion %s", diag.Kind, diag.Namespace, diag.Name, diag.Cluster, obj.GetResourceVersion()))
	diag.Conditions = objectConditions(obj)
	if diag.Provider == providerStandardConditions && len(diag.Conditions) == 0 {
		diag.Provider = providerKubernetes
		return
	}
	service := strings.TrimSuffix(groupOf(diag.APIVersion), ".services.k8s.aws")
	var ep ackEndpointResult
	if diag.Provider == providerACK {
		// key by cluster and service: the lookup is cluster-scoped through
		// cctx, so the same service in another cluster must not reuse it
		epKey := diag.Cluster + "\x00" + service
		var ok bool
		if ep, ok = epCache[epKey]; !ok {
			ep = ackControllerEndpoint(cctx, d.f.Client(), service)
			epCache[epKey] = ep
		}
		switch ep.status {
		case ackEndpointStatusOverride:
			diag.EndpointOverride = ep.url
			out.Evidence = append(out.Evidence, "endpoint provenance: "+ep.evidence+" — this diagnosis describes that endpoint, not AWS")
		case ackEndpointStatusUnknown:
			diag.Notes = append(diag.Notes, "endpoint provenance unknown: "+ep.evidence+"; these conditions may describe an emulator rather than AWS")
		}
	}
	switch diag.Provider {
	case providerACK:
		diagnoseACK(diag, obj, out)
	case providerConfigConnector:
		diagnoseConfigConnector(diag, out)
	case providerCrossplane:
		diagnoseCrossplane(diag)
	case providerStandardConditions:
		diagnoseStandardConditions(diag, out)
	}
	diag.Cause = classifyCause(diag)
	if diag.Cause != nil {
		d.attachCause(diag, out, service, ep)
	}
}

// attachCause adds the sweep pointer and the controller handle a classified
// cause carries. The sweep is never run here: one Application cannot show
// account scope, and a bounded per-Application read stays bounded.
func (d *deps) attachCause(diag *ResourceDiagnosis, out *DiagnoseApplicationOutput, service string, ep ackEndpointResult) {
	if diag.Cause.Class == causeUnclassified {
		return
	}
	addSweepPointer(diag, out)
	handle, note := controllerHandle(diag, service, ep, time.Now().UTC())
	diag.Cause.Controller = handle
	if note == "" {
		return
	}
	if diag.Provider == providerACK {
		// a missing or ambiguous Deployment already left a provenance note;
		// say once that it also leaves no handle
		for i, n := range diag.Notes {
			if strings.HasPrefix(n, "endpoint provenance unknown") {
				diag.Notes[i] = n + "; no controller handle"
				return
			}
		}
	}
	diag.Notes = append(diag.Notes, note)
}

// addSweepPointer names diagnose_controllers for a class that is usually
// account-wide, once per provider, family and class; per family and API
// group when the family has no provider.
func addSweepPointer(diag *ResourceDiagnosis, out *DiagnoseApplicationOutput) {
	c := diag.Cause
	if !accountLevelClass(c.Class) {
		return
	}
	target := "family " + diag.Provider
	if cloud := cloudOf(diag.APIVersion); cloud != "" {
		target = "provider " + cloud + ", " + target
	} else {
		target += ", group " + groupOf(diag.APIVersion)
	}
	msg := fmt.Sprintf("confirm the scope with diagnose_controllers when this server offers it: %s, window 1h — a %s cause is usually account-wide and one Application cannot show that", target, c.Class)
	for _, n := range out.NextChecks {
		if n == msg {
			return
		}
	}
	out.NextChecks = append(out.NextChecks, msg)
}

// controllerHandle returns the owning controller as a handle, with a window
// from the cause's condition transition, or a note saying why there is none.
func controllerHandle(diag *ResourceDiagnosis, service string, ep ackEndpointResult, now time.Time) (*ControllerHandle, string) {
	window := TimeWindow{From: now.Add(-time.Hour), To: now, Basis: "fallback: one hour before this read"}
	if diag.Cause.Since != nil {
		window = TimeWindow{From: *diag.Cause.Since, To: now, Basis: "condition transition"}
	}
	switch diag.Provider {
	case providerACK:
		if ep.namespace == "" {
			return nil, "no controller handle: the ACK " + service + " controller Deployment could not be identified"
		}
		return &ControllerHandle{Cluster: diag.Cluster, Namespace: ep.namespace, Name: ep.name, LabelSelector: "app.kubernetes.io/name=" + service + "-chart", Family: providerACK, APIGroup: groupOf(diag.APIVersion), Window: window}, ""
	case providerConfigConnector:
		return nil, "no controller handle: this server does not look up the Config Connector manager; it runs as StatefulSet cnrm-controller-manager in cnrm-system (or per-namespace in namespaced mode)"
	case providerStandardConditions:
		return nil, "no controller handle: the standard-conditions family names no controller; a profile for " + groupOf(diag.APIVersion) + " can name it"
	default:
		return nil, "no controller handle: the Crossplane provider pod for " + groupOf(diag.APIVersion) + " is not identified by this server; list pods with label pkg.crossplane.io/provider in the crossplane-system namespace"
	}
}

// eventMatchesResource reports whether an event is about the resource: the
// same object in the same namespace and cluster.
func eventMatchesResource(e EventSummary, r ResourceDiagnosis) bool {
	return e.Cluster == r.Cluster && e.Namespace == r.Namespace && e.Object == r.Kind+"/"+r.Name
}

// cloudOf maps an API group to its cloud through the family tables, for the
// diagnose_controllers pointer. Empty when no family owns the group.
func cloudOf(apiVersion string) string {
	group := groupOf(apiVersion)
	for _, provider := range providerOrder {
		for _, fam := range providerFamilies[provider] {
			for _, suffix := range fam.suffixes {
				if strings.HasSuffix(group, suffix) {
					return provider
				}
			}
		}
	}
	return ""
}

// diagnoseACK applies the ACK conventions: ready means ACK.ResourceSynced is
// True and ACK.Terminal is absent or not True. Terminal halts reconciliation
// while Synced keeps its last value, so Synced alone can report a dead
// resource as fine. A resource the provider says exists that stays unsynced
// past the drift window is the signature of a field the service computes
// differently from the spec, which keeps the controller's update loop running.
func diagnoseACK(diag *ResourceDiagnosis, obj *unstructured.Unstructured, out *DiagnoseApplicationOutput) {
	synced := conditionOf(diag.Conditions, "ACK.ResourceSynced")
	terminal := conditionOf(diag.Conditions, "ACK.Terminal")
	refs := conditionOf(diag.Conditions, "ACK.ReferencesResolved")
	arn, _, _ := unstructured.NestedString(obj.Object, "status", "ackResourceMetadata", "arn")
	service := strings.TrimSuffix(groupOf(diag.APIVersion), ".services.k8s.aws")
	switch {
	case terminal != nil && terminal.Status == string(corev1.ConditionTrue):
		diag.State = stateHalted
		diag.Summary = "ACK.Terminal is True: the controller stopped reconciling this resource and will not retry without a spec change"
		out.NextChecks = append(out.NextChecks, fmt.Sprintf("fix the cause in the %s's spec; the Terminal message names it", diag.Kind))
	case refs != nil && refs.Status != string(corev1.ConditionTrue):
		// An unresolved reference halts reconciliation, and ResourceSynced
		// keeps whatever value it had before, so it is checked first.
		diag.State = stateNotReady
		if refs.Status == string(corev1.ConditionFalse) {
			diag.Summary = "ACK.ReferencesResolved is False: reference resolution failed and the controller will not reconcile this resource until it succeeds; the condition message names the blocker"
		} else {
			diag.Summary = "ACK.ReferencesResolved is " + refs.Status + ": reference resolution has not completed; the condition message says what it is waiting on"
		}
		if synced != nil && synced.Status == string(corev1.ConditionTrue) {
			diag.Notes = append(diag.Notes, "ACK.ResourceSynced still reads True from before the reference became unresolved; it is stale while reconciliation waits on the reference")
		}
	case synced == nil:
		diag.State = stateUnknown
		diag.Summary = "no ACK.ResourceSynced condition yet: the controller has not reconciled this resource; check that the ACK " + service + " controller is running"
	case synced.Status == string(corev1.ConditionTrue):
		diag.Ready, diag.State = true, stateReady
		diag.Summary = "ACK.ResourceSynced is True and no Terminal condition is set"
		if obj.GetGeneration() > 1 {
			diag.Notes = append(diag.Notes, "ACK conditions do not record the generation they describe; after a spec change a True can be stale until the condition is seen to move")
		}
	default:
		diag.State = stateNotReady
		diag.Summary = "ACK.ResourceSynced is " + synced.Status
		if arn != "" && synced.LastTransition != nil && time.Since(*synced.LastTransition) > driftWindow {
			diag.Summary += fmt.Sprintf("; the provider reports the resource exists (%s) but the controller has not converged for %s — often a spec field the service computes differently keeping the update loop running, though a persistent access or validation error looks the same; the condition message and the controller's logs name the actual error", arn, time.Since(*synced.LastTransition).Round(time.Second))
			out.NextChecks = append(out.NextChecks, fmt.Sprintf("the ACK %s controller's logs record the desired-vs-latest diff for %s %s/%s on every reconcile; the drifting field is named there", service, diag.Kind, diag.Namespace, diag.Name))
		}
	}
}

// diagnoseConfigConnector applies the Config Connector convention: ready
// means the Ready condition is True with reason UpToDate.
func diagnoseConfigConnector(diag *ResourceDiagnosis, out *DiagnoseApplicationOutput) {
	ready := conditionOf(diag.Conditions, "Ready")
	switch {
	case ready == nil:
		diag.State = stateUnknown
		diag.Summary = "no Ready condition yet: Config Connector has not reconciled this resource; check that cnrm-controller-manager is running in cnrm-system"
	case ready.Status == string(corev1.ConditionTrue) && ready.Reason == "UpToDate":
		diag.Ready, diag.State = true, stateReady
		diag.Summary = "Ready is True with reason UpToDate"
	case ready.Status == string(corev1.ConditionTrue):
		diag.State = stateNotReady
		diag.Summary = "Ready is True but its reason is " + ready.Reason + ", not UpToDate: the resource exists but its latest spec has not been actuated"
	default:
		diag.State = stateNotReady
		diag.Summary = "Ready is " + ready.Status + " with reason " + ready.Reason
		out.NextChecks = append(out.NextChecks, fmt.Sprintf("cnrm-controller-manager logs in cnrm-system carry the API error behind reason %s for %s %s/%s", ready.Reason, diag.Kind, diag.Namespace, diag.Name))
	}
}

// diagnoseCrossplane applies the Crossplane convention: ready means both the
// Ready and Synced conditions are True.
func diagnoseCrossplane(diag *ResourceDiagnosis) {
	ready := conditionOf(diag.Conditions, "Ready")
	synced := conditionOf(diag.Conditions, "Synced")
	switch {
	case ready == nil || synced == nil:
		missing := "Ready or Synced condition"
		switch {
		case ready != nil:
			missing = "Synced condition"
		case synced != nil:
			missing = "Ready condition"
		}
		diag.State = stateUnknown
		diag.Summary = "no " + missing + " yet: the provider has not fully reconciled this resource; check that its Crossplane provider is installed and healthy"
	case synced != nil && synced.Status != string(corev1.ConditionTrue):
		diag.State = stateNotReady
		diag.Summary = "Synced is " + synced.Status + ": the provider could not apply the spec to the external system"
	case ready != nil && ready.Status != string(corev1.ConditionTrue):
		diag.State = stateNotReady
		diag.Summary = "Synced holds but Ready is " + ready.Status + ": the external resource is not available yet"
	default:
		diag.Ready, diag.State = true, stateReady
		diag.Summary = "Ready and Synced are both True"
	}
}

// diagnoseStandardConditions applies the metav1.Condition convention: ready
// means a Ready condition that is True; with no Ready condition the
// readiness is unknown, never healthy, and the resource is listed under
// unknowns. Whether Accepted or Programmed should count as ready for the
// kinds that write those instead is an open question in the KEP.
func diagnoseStandardConditions(diag *ResourceDiagnosis, out *DiagnoseApplicationOutput) {
	ready := conditionOf(diag.Conditions, "Ready")
	switch {
	case ready == nil:
		types := make([]string, 0, len(diag.Conditions))
		for _, c := range diag.Conditions {
			types = append(types, c.Type)
		}
		diag.State = stateUnknown
		diag.Summary = "no Ready condition: the standard-conditions convention cannot say whether this resource is ready; its controller writes " + strings.Join(types, ", ")
		out.Unknowns = append(out.Unknowns, fmt.Sprintf("%s %s/%s in cluster %s: readiness unknown, it writes no Ready condition (%s); a controller family profile for %s can name the condition that means ready", diag.Kind, diag.Namespace, diag.Name, diag.Cluster, strings.Join(types, ", "), groupOf(diag.APIVersion)))
	case ready.Status == string(corev1.ConditionTrue):
		diag.Ready, diag.State = true, stateReady
		diag.Summary = "Ready is True"
	default:
		diag.State = stateNotReady
		diag.Summary = "Ready is " + ready.Status
		if ready.Reason != "" {
			diag.Summary += "; reason " + ready.Reason
		}
	}
}

// diagnosisFindings turns one resource's verdict into findings.
func diagnosisFindings(diag ResourceDiagnosis) []Finding {
	if diag.Ready {
		return nil
	}
	ref := fmt.Sprintf("%s %s/%s in cluster %s", diag.Kind, diag.Namespace, diag.Name, diag.Cluster)
	severity := SeverityWarning
	if diag.State == stateHalted {
		severity = SeverityError
	}
	if diag.State == stateNotReady && diag.Provider != providerKubernetes {
		severity = SeverityError
	}
	observed := ""
	for _, c := range diag.Conditions {
		if c.Status != string(corev1.ConditionTrue) || c.Message != "" {
			observed = joinParts(observed, c.Type+"="+c.Status, c.Reason, c.Message)
		}
	}
	findings := []Finding{{
		Severity: severity,
		Message:  fmt.Sprintf("component %q: %s %s is %s (%s): %s", diag.Component, diag.Kind, diag.Name, diag.State, diag.Provider, diag.Summary),
		Observed: text(observed),
		Evidence: ref,
	}}
	if diag.Cause != nil && diag.Cause.Class != causeUnclassified {
		findings = append(findings, causeFinding(diag))
	}
	return findings
}

// causeFinding states the classified cause as its own finding so the chain
// reads without the structured block.
func causeFinding(diag ResourceDiagnosis) Finding {
	c := diag.Cause
	return Finding{
		Severity: SeverityError,
		Message:  fmt.Sprintf("component %q: %s %s cause %s (%s) — owner: %s; scope: %s", diag.Component, diag.Kind, diag.Name, c.Class, c.Code, c.Owner, c.Scope),
		Observed: text(c.Evidence),
		Evidence: fmt.Sprintf("%s %s/%s in cluster %s", diag.Kind, diag.Namespace, diag.Name, diag.Cluster),
	}
}

// objectConditions reads status.conditions from an unstructured object.
func objectConditions(obj *unstructured.Unstructured) []ResourceCondition {
	raw, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found || err != nil {
		return nil
	}
	out := make([]ResourceCondition, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		c := ResourceCondition{
			Type:    stringAt(m, "type"),
			Status:  stringAt(m, "status"),
			Reason:  text(stringAt(m, "reason")),
			Message: text(stringAt(m, "message")),
		}
		if ts := stringAt(m, "lastTransitionTime"); ts != "" {
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				c.LastTransition = timePtr(t)
			}
		}
		out = append(out, c)
	}
	return out
}

// conditionOf returns the condition of the given type, or nil.
func conditionOf(conds []ResourceCondition, condType string) *ResourceCondition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}
	return nil
}

func stringAt(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func groupOf(apiVersion string) string {
	if i := strings.Index(apiVersion, "/"); i >= 0 {
		return apiVersion[:i]
	}
	return ""
}
