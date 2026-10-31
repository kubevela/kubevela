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
	"regexp"
	"sort"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	querytypes "github.com/oam-dev/kubevela/pkg/utils/types"
)

const (
	// windowLead is how far before the apply time the window starts, so a
	// query also covers the minutes leading up to the change.
	windowLead = 5 * time.Minute
	// windowFallback is the window used when the apply time is unknown.
	windowFallback = time.Hour
	// maxHandlePods bounds the pods listed in handles.
	maxHandlePods = 50
)

// promLabelInvalid matches characters Prometheus and Loki label names do not
// allow; the usual Kubernetes discovery relabelling replaces them with "_".
var promLabelInvalid = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// PodHandle names one pod an Application runs.
type PodHandle struct {
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Component string `json:"component,omitempty"`
}

// TimeWindow is the span downstream queries should cover.
type TimeWindow struct {
	From time.Time `json:"from" jsonschema:"Start of the window, UTC: shortly before the running revision was applied."`
	To   time.Time `json:"to" jsonschema:"End of the window, UTC: when this answer was read."`
	// Basis says what From was computed from.
	Basis string `json:"basis" jsonschema:"What the window start was computed from: workflow start, revision creation, or a fallback when neither is known."`
}

// Handles are the exact identifiers an agent passes to other tools, such
// as an observability or Kubernetes MCP server, so every downstream query
// is about the same objects and the same time span. Every value is read
// from the cluster; none is inferred.
type Handles struct {
	Clusters  []string `json:"clusters" jsonschema:"Clusters where the Application's resources run; local is the control plane."`
	Namespace string   `json:"namespace"`
	// Application is the Application's name.
	Application string `json:"application"`
	Revision    string `json:"revision,omitempty" jsonschema:"ApplicationRevision that is running now."`
	// AppliedAt is when the running revision was applied.
	AppliedAt *time.Time  `json:"appliedAt,omitempty" jsonschema:"When the running revision was applied, UTC. Compare it with when a symptom started."`
	Pods      []PodHandle `json:"pods" jsonschema:"Pods the Application runs, by cluster and namespace."`
	// Labels are the labels KubeVela stamps on every pod it renders.
	Labels map[string]string `json:"labels" jsonschema:"Labels KubeVela stamps on every pod of this Application, as they appear in Kubernetes."`
	// LabelSelector is Labels in Kubernetes selector syntax.
	LabelSelector string `json:"labelSelector" jsonschema:"The labels as a Kubernetes label selector, for kubectl or a Kubernetes MCP server."`
	// ComponentLabels select the pods of one component.
	ComponentLabels map[string]map[string]string `json:"componentLabels,omitempty" jsonschema:"Labels that select one component's pods, keyed by component."`
	// MetricLabels are Labels with the name sanitising Prometheus and Loki
	// discovery usually applies. Verify with one probe query, since a
	// monitoring stack can relabel differently.
	MetricLabels map[string]string `json:"metricLabels" jsonschema:"The labels as Prometheus and Loki usually name them after Kubernetes discovery (dots and slashes become underscores). Verify with one probe query before relying on them; the observability server appendix gives the mapping for each stack."`
	Window       TimeWindow        `json:"window" jsonschema:"Time span to query. Use it verbatim in every downstream query."`
}

// buildHandles assembles handles from objects explain_application already
// read, plus the running revision for its apply time.
func (d *deps) buildHandles(ctx context.Context, cli client.Client, app *v1beta1.Application, applied []querytypes.AppliedResource, now time.Time) (Handles, []string, []string) {
	h := Handles{
		Namespace:       app.Namespace,
		Application:     app.Name,
		Labels:          map[string]string{oam.LabelAppName: app.Name},
		ComponentLabels: map[string]map[string]string{},
		Pods:            []PodHandle{},
	}
	var evidence, unknowns []string

	clusters := map[string]bool{}
	for i := range applied {
		clusters[clusterName(applied[i].Cluster)] = true
	}
	for c := range clusters {
		h.Clusters = append(h.Clusters, c)
	}
	sort.Strings(h.Clusters)
	if len(h.Clusters) == 0 {
		h.Clusters = []string{localCluster}
	}

	for _, p := range collectPods(applied) {
		if len(h.Pods) == maxHandlePods {
			unknowns = append(unknowns, fmt.Sprintf("more than %d pods; handles list the first %d", maxHandlePods, maxHandlePods))
			break
		}
		h.Pods = append(h.Pods, PodHandle{Cluster: p.cluster, Namespace: p.namespace, Name: p.name, Component: p.component})
	}
	for _, c := range app.Spec.Components {
		h.ComponentLabels[c.Name] = map[string]string{oam.LabelAppName: app.Name, oam.LabelAppComponent: c.Name}
	}
	h.LabelSelector = labelSelector(h.Labels)
	h.MetricLabels = metricLabels(h.Labels)

	if app.Status.LatestRevision != nil {
		h.Revision = app.Status.LatestRevision.Name
	}
	applyTime, basis := d.appliedAt(ctx, cli, app)
	switch {
	case applyTime != nil:
		h.AppliedAt = applyTime
		h.Window = TimeWindow{From: applyTime.Add(-windowLead), To: now, Basis: basis}
		evidence = append(evidence, fmt.Sprintf("apply time from %s", basis))
	default:
		h.Window = TimeWindow{From: now.Add(-windowFallback), To: now, Basis: "the last hour, because the apply time could not be read"}
		unknowns = append(unknowns, "when the running revision was applied; the window is the last hour")
	}
	return h, evidence, unknowns
}

// appliedAt is when the running revision was applied: the workflow's start
// for that revision, else the revision object's creation time.
func (d *deps) appliedAt(ctx context.Context, cli client.Client, app *v1beta1.Application) (*time.Time, string) {
	if wf := app.Status.Workflow; wf != nil && !wf.StartTime.IsZero() &&
		(app.Status.LatestRevision == nil || wf.AppRevision == "" || wf.AppRevision == app.Status.LatestRevision.Name) {
		return timePtr(wf.StartTime.Time), "the workflow start of revision " + wf.AppRevision
	}
	if app.Status.LatestRevision == nil {
		return nil, ""
	}
	rev := &v1beta1.ApplicationRevision{}
	if err := cli.Get(ctx, client.ObjectKey{Namespace: app.Namespace, Name: app.Status.LatestRevision.Name}, rev); err != nil {
		return nil, ""
	}
	if rev.CreationTimestamp.IsZero() {
		return nil, ""
	}
	return timePtr(rev.CreationTimestamp.Time), "the creation of ApplicationRevision " + rev.Name
}

func labelSelector(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+labels[k])
	}
	return strings.Join(parts, ",")
}

func metricLabels(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels))
	for k, v := range labels {
		out[promLabelInvalid.ReplaceAllString(k, "_")] = v
	}
	return out
}
