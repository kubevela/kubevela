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
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	fieldExportKind = "FieldExport"
	// ackCommonGroup is the API group of ACK's kinds that every service
	// controller serves, FieldExport among them.
	ackCommonGroup = "services.k8s.aws"
)

// ExportDetail describes a resource whose job is to hand a value from a
// provisioned resource to workloads through a Secret or ConfigMap: an ACK
// FieldExport, or any custom resource that writes a connection Secret the
// way Crossplane managed resources and Terraform Configurations do. The
// target's content is never read; whether the value landed is taken from
// the exporter's own conditions, and is unknown when it writes none.
type ExportDetail struct {
	SourceGroup     string `json:"sourceGroup,omitempty" jsonschema:"API group of the resource the value is read from."`
	SourceKind      string `json:"sourceKind,omitempty" jsonschema:"Kind of the resource the value is read from."`
	SourceName      string `json:"sourceName,omitempty" jsonschema:"Name of the resource the value is read from, in the exporter's namespace."`
	SourcePath      string `json:"sourcePath,omitempty" jsonschema:"Field path copied, as the exporter spells it; empty when the exporter writes a whole set of connection details."`
	TargetKind      string `json:"targetKind" jsonschema:"Secret or ConfigMap."`
	TargetName      string `json:"targetName" jsonschema:"Name of the Secret or ConfigMap written."`
	TargetNamespace string `json:"targetNamespace,omitempty" jsonschema:"Namespace of the target when the exporter names one; otherwise the exporter's own namespace."`
	TargetKey       string `json:"targetKey,omitempty" jsonschema:"Key written in the target; empty when the exporter writes several keys, such as a connection Secret."`
	// Written is nil when the exporter writes no conditions, so its
	// verdict came from the resource tree, which cannot say.
	Written *bool `json:"written,omitempty" jsonschema:"True when the exporter's own conditions say the target has been written, false when they say it has not. Absent when the resource writes no conditions and was judged from the resource tree, which cannot tell. The target is never read."`
}

// exportOf reads what a resource exports for workloads, or nil when it is not
// an exporter. The connection-Secret field is read on every custom resource,
// not only Crossplane's, because the Terraform controller's Configuration
// and provisioners not yet known here spell it the same way.
func exportOf(diag *ResourceDiagnosis, obj *unstructured.Unstructured) *ExportDetail {
	if diag.Provider == providerACK && diag.Kind == fieldExportKind {
		return fieldExportOf(obj)
	}
	if diag.Provider == providerKubernetes {
		return nil
	}
	return connectionSecretOf(diag, obj)
}

// fieldExportOf reads an ACK FieldExport's source and target from its spec.
func fieldExportOf(obj *unstructured.Unstructured) *ExportDetail {
	toKind, _, _ := unstructured.NestedString(obj.Object, "spec", "to", "kind")
	toName, _, _ := unstructured.NestedString(obj.Object, "spec", "to", "name")
	if toName == "" {
		return nil
	}
	switch strings.ToLower(toKind) {
	case "secret":
		toKind = secretKind
	case "configmap":
		toKind = "ConfigMap"
	}
	e := &ExportDetail{TargetKind: toKind, TargetName: toName}
	e.TargetNamespace, _, _ = unstructured.NestedString(obj.Object, "spec", "to", "namespace")
	e.TargetKey, _, _ = unstructured.NestedString(obj.Object, "spec", "to", "key")
	e.SourcePath, _, _ = unstructured.NestedString(obj.Object, "spec", "from", "path")
	e.SourceGroup, _, _ = unstructured.NestedString(obj.Object, "spec", "from", "resource", "group")
	e.SourceKind, _, _ = unstructured.NestedString(obj.Object, "spec", "from", "resource", "kind")
	e.SourceName, _, _ = unstructured.NestedString(obj.Object, "spec", "from", "resource", "name")
	return e
}

// connectionSecretOf reads the connection Secret a custom resource writes:
// writeConnectionSecretToRef on Crossplane managed resources and Terraform
// Configurations, publishConnectionDetailsTo on Crossplane composites. The
// resource is its own source.
func connectionSecretOf(diag *ResourceDiagnosis, obj *unstructured.Unstructured) *ExportDetail {
	for _, field := range []string{"writeConnectionSecretToRef", "publishConnectionDetailsTo"} {
		name, found, _ := unstructured.NestedString(obj.Object, "spec", field, "name")
		if !found || name == "" {
			continue
		}
		e := &ExportDetail{SourceGroup: groupOf(diag.APIVersion), SourceKind: diag.Kind, SourceName: diag.Name, TargetKind: secretKind, TargetName: name}
		e.TargetNamespace, _, _ = unstructured.NestedString(obj.Object, "spec", field, "namespace")
		return e
	}
	return nil
}

// ackService names the ACK service whose controller reconciles a resource:
// the prefix of its API group. A FieldExport lives in the common group and
// is reconciled by the controller of its source resource's service, read
// from its spec. For any other common-group kind, or a FieldExport that
// names no source group, there is no service to derive, and the result is
// empty rather than an invented name.
func ackService(diag *ResourceDiagnosis, obj *unstructured.Unstructured) string {
	group := groupOf(diag.APIVersion)
	if group == ackCommonGroup {
		if diag.Kind != fieldExportKind {
			return ""
		}
		group, _, _ = unstructured.NestedString(obj.Object, "spec", "from", "resource", "group")
		if group == "" || group == ackCommonGroup {
			return ""
		}
	}
	return strings.TrimSuffix(group, "."+ackCommonGroup)
}

// describeExport settles whether the target has been written from the
// exporter's own verdict and says so in the summary. A resource judged from
// the resource tree, which defaults unknown kinds to Healthy, gets no
// verdict: the tree cannot say whether anything was written.
func describeExport(diag *ResourceDiagnosis) {
	e := diag.Export
	source := ""
	if e.SourcePath != "" {
		source = fmt.Sprintf("%s of %s %s ", e.SourcePath, e.SourceKind, e.SourceName)
	}
	state := "whether it was written is unknown, because the resource writes no conditions"
	if diag.Provider != providerKubernetes {
		written := diag.Ready
		e.Written = &written
		state = "not written yet"
		if written {
			state = "written"
		}
	}
	diag.Summary = joinParts(diag.Summary, fmt.Sprintf("exports %sinto %s: %s", source, exportTarget(e), state))
}

func exportTarget(e *ExportDetail) string {
	target := e.TargetKind + " " + e.TargetName
	if e.TargetKey != "" {
		target += " key " + e.TargetKey
	}
	return target
}

// linkExports ties each export its exporter says is unwritten to the state
// of its source resource, which is where a workload's missing key usually
// comes from. It runs after every resource has its verdict, because the
// source may follow the exporter in the inventory. component is the
// component the diagnosis was narrowed to, empty for the whole Application.
func linkExports(out *DiagnoseApplicationOutput, component string) {
	for i := range out.Resources {
		r := &out.Resources[i]
		if r.Export == nil || r.Export.Written == nil || *r.Export.Written {
			continue
		}
		e := r.Export
		ref := fmt.Sprintf("%s %s/%s in cluster %s", r.Kind, r.Namespace, r.Name, r.Cluster)
		if e.SourceKind == r.Kind && e.SourceName == r.Name {
			// a resource that is its own source: the Secret appears with
			// readiness, and the resource's own finding already says why it
			// is not ready
			r.Notes = append(r.Notes, fmt.Sprintf("connection %s %s is written when this resource is Ready", e.TargetKind, e.TargetName))
			continue
		}
		src := sourceOf(out.Resources, r)
		switch {
		case src == nil && component != "":
			r.Notes = append(r.Notes, fmt.Sprintf("source %s %s was not read: the diagnosis was narrowed to component %q; diagnose the whole Application to see it", e.SourceKind, e.SourceName, component))
		case src == nil:
			out.Findings = append(out.Findings, Finding{
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("component %q: %s %s has not written %s; its source %s %s was not among the diagnosed resources, so its state was not read", r.Component, r.Kind, r.Name, exportTarget(e), e.SourceKind, e.SourceName),
				Evidence: ref,
			})
		case src.State == stateNotReady || src.State == stateHalted:
			r.Notes = append(r.Notes, fmt.Sprintf("source %s %s is %s; the export cannot complete until the source field exists", src.Kind, src.Name, src.State))
			out.Findings = append(out.Findings, Finding{
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("component %q: %s %s has not written %s because its source %s %s is %s; a workload that reads that key starts without it until the source is ready", r.Component, r.Kind, r.Name, exportTarget(e), src.Kind, src.Name, src.State),
				Observed: text(src.Summary),
				Evidence: ref,
			})
		case src.Ready:
			r.Notes = append(r.Notes, fmt.Sprintf("source %s %s is Ready; the exporter's own conditions name the problem, often a source path that is unset and never converges", src.Kind, src.Name))
		default:
			r.Notes = append(r.Notes, fmt.Sprintf("source %s %s is %s, so whether it is the reason the export is unwritten could not be read", src.Kind, src.Name, src.State))
		}
	}
}

// sourceOf finds the diagnosed resource an export reads from: same cluster
// and namespace, the kind and name the exporter names, and the API group
// when the exporter names one.
func sourceOf(resources []ResourceDiagnosis, r *ResourceDiagnosis) *ResourceDiagnosis {
	e := r.Export
	for i := range resources {
		s := &resources[i]
		if s.Cluster != r.Cluster || s.Namespace != r.Namespace || s.Kind != e.SourceKind || s.Name != e.SourceName {
			continue
		}
		if e.SourceGroup != "" && groupOf(s.APIVersion) != e.SourceGroup {
			continue
		}
		return s
	}
	return nil
}
