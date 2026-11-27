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

	// exportWritesKey marks an exporter that copies one value under one key
	// of the target; exportWritesSet one that writes a set of keys it does
	// not name, the way a connection Secret is written. The shape is stated
	// rather than inferred from an empty key, so an exporter whose key fell
	// back to its controller's default is never mistaken for a set writer.
	exportWritesKey = "key"
	exportWritesSet = "set"
)

// ExportDetail describes a resource whose job is to hand a value from a
// provisioned resource to workloads through a Secret or ConfigMap: an ACK
// FieldExport, or any custom resource that writes a connection Secret the
// way Crossplane managed resources and Terraform Configurations do. The
// target's content is never read; whether the value landed is taken from
// the exporter's own conditions, and is unknown when it writes none or
// when its family's conditions are missing so its readiness is unknown.
type ExportDetail struct {
	SourceGroup     string `json:"sourceGroup,omitempty" jsonschema:"API group of the resource the value is read from."`
	SourceKind      string `json:"sourceKind,omitempty" jsonschema:"Kind of the resource the value is read from."`
	SourceName      string `json:"sourceName,omitempty" jsonschema:"Name of the resource the value is read from, in the exporter's namespace."`
	SourcePath      string `json:"sourcePath,omitempty" jsonschema:"Field path copied, as the exporter spells it; empty when the exporter writes a whole set of connection details."`
	TargetKind      string `json:"targetKind" jsonschema:"Secret or ConfigMap."`
	TargetName      string `json:"targetName" jsonschema:"Name of the Secret or ConfigMap written."`
	TargetNamespace string `json:"targetNamespace,omitempty" jsonschema:"Namespace of the target when the exporter names one; otherwise the exporter's own namespace."`
	TargetKey       string `json:"targetKey,omitempty" jsonschema:"Key written in the target when writes is key. An exporter that names no key is reported under the key its controller writes by default, such as the exporter's own namespace.name for an ACK FieldExport. Empty when the exporter writes a set of keys, or when its default key is not known here."`
	Writes          string `json:"writes" jsonschema:"key when the exporter copies one value under targetKey; set when it writes several keys it does not name, such as a connection Secret."`
	// Written is nil when the exporter writes no conditions, so its
	// verdict came from the resource tree, which cannot say, and when its
	// readiness is unknown because the family's conditions are missing.
	Written *bool `json:"written,omitempty" jsonschema:"True when the exporter's own conditions say the target has been written, false when they say it has not. Absent when the resource writes no conditions and was judged from the resource tree, which cannot tell, and when its readiness is unknown because the conditions its family reads are missing. The target is never read."`
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
// A FieldExport copies one value, so it always reports a key: the one its
// spec names, else the one the ACK runtime writes when spec.to.key is empty
// or blank, "<namespace>.<name>" of the FieldExport itself. Each single-key
// exporter family states its own default here, beside its reader, so that an
// exporter that left the key to its controller is never read as a set writer.
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
	e := &ExportDetail{TargetKind: toKind, TargetName: toName, Writes: exportWritesKey}
	e.TargetNamespace, _, _ = unstructured.NestedString(obj.Object, "spec", "to", "namespace")
	e.TargetKey, _, _ = unstructured.NestedString(obj.Object, "spec", "to", "key")
	if strings.TrimSpace(e.TargetKey) == "" {
		e.TargetKey = obj.GetNamespace() + "." + obj.GetName()
	}
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
		e := &ExportDetail{SourceGroup: groupOf(diag.APIVersion), SourceKind: diag.Kind, SourceName: diag.Name, TargetKind: secretKind, TargetName: name, Writes: exportWritesSet}
		e.TargetNamespace, _, _ = unstructured.NestedString(obj.Object, "spec", field, "namespace")
		return e
	}
	return nil
}

// ackService names the ACK service whose controller reconciles a resource:
// the prefix of its API group. The common-group kinds are reconciled by the
// controller of the service their spec points at: a FieldExport's source
// resource group, an AdoptedResource's target group. For any other
// common-group kind, or one that names no group, there is no service to
// derive, and the result is empty rather than an invented name.
func ackService(diag *ResourceDiagnosis, obj *unstructured.Unstructured) string {
	group := groupOf(diag.APIVersion)
	if group == ackCommonGroup {
		switch diag.Kind {
		case fieldExportKind:
			group, _, _ = unstructured.NestedString(obj.Object, "spec", "from", "resource", "group")
		case "AdoptedResource":
			group, _, _ = unstructured.NestedString(obj.Object, "spec", "kubernetes", "group")
		default:
			return ""
		}
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
	switch {
	case diag.Provider == providerKubernetes:
		// settled from the tree, which cannot say
	case diag.State == stateUnknown:
		// the family's conditions are missing, so its verdict is not known
		state = "whether it was written is unknown, because the resource's readiness is unknown"
	default:
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

// covers says whether this export writes what a consumer reads: everything
// an exporter writes is part of a whole-object read (an empty key); a set
// writer writes every key; a single-key exporter writes only its key, and
// one whose key is not known here is matched to whole-object reads alone, so
// that it is never blamed for a key it may not write.
func (e *ExportDetail) covers(key string) bool {
	switch {
	case key == "", e.Writes == exportWritesSet:
		return true
	default:
		return e.TargetKey != "" && e.TargetKey == key
	}
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
