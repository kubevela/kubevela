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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// healthCoverageFindings reports a component definition whose health rule
// judges only the primary output although the template emits auxiliary
// outputs. Such a component is healthy as soon as its primary resource is,
// before its members and the address a consumer needs exist, and every
// component that depends on it is released at that moment. The check is
// static: it reads the template's output paths and the health policy text.
func healthCoverageFindings(defType string, produces []ProducedResource, health HealthRules, evidence string) []Finding {
	if defType != DefinitionTypeComponent {
		return nil
	}
	var aux []string
	for _, p := range produces {
		if !strings.HasPrefix(p.Path, "outputs.") {
			continue
		}
		name := strings.TrimPrefix(p.Path, "outputs.")
		if p.Kind != "" {
			name += " (" + p.Kind + ")"
		}
		aux = append(aux, name)
	}
	if len(aux) == 0 {
		return nil
	}
	policy := strings.TrimSpace(health.HealthPolicy)
	switch {
	case policy == "":
		return []Finding{{
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("the definition declares no health policy, so health falls back to the built-in check on the primary resource; its %d auxiliary outputs (%s) are not judged, and a component that depends on this one is released before they exist", len(aux), strings.Join(aux, ", ")),
			Evidence: evidence,
		}}
	case strings.Contains(policy, "context.outputs"):
		return nil
	}
	return []Finding{{
		Severity: SeverityWarning,
		Message:  fmt.Sprintf("the health policy reads context.output only, so the component is healthy as soon as its primary resource is, before its %d auxiliary outputs (%s) are; a component that depends on this one is released at that moment and can start before those exist", len(aux), strings.Join(aux, ", ")),
		Observed: text(policy),
		Evidence: evidence,
	}}
}

// dependencyFindings reports a rendered workload that reads a Secret or
// ConfigMap another component of the same Application produces, directly or
// through an exporter, without declaring dependsOn on that component. The
// default workflow applies components in parallel, so such a consumer can
// start before the object or its keys exist. It runs on the rendered
// objects before redaction, which hides every secretKeyRef, and before any
// cluster is touched; objects is the render's identity-keyed map.
func dependencyFindings(app *v1beta1.Application, resources []RenderedResource, objects map[string]*unstructured.Unstructured) []Finding {
	producers := map[string]string{} // kind/name -> component
	for i := range resources {
		r := &resources[i]
		obj := objects[resourceKey(r.APIVersion, r.Kind, r.Namespace, r.Name)]
		switch {
		case r.Kind == secretKind || r.Kind == "ConfigMap":
			producers[r.Kind+"/"+r.Name] = r.Component
		case obj != nil:
			var e *ExportDetail
			if r.Kind == fieldExportKind {
				e = fieldExportOf(obj)
			} else if groupOf(r.APIVersion) != "" && !builtinGroups[groupOf(r.APIVersion)] {
				e = connectionSecretOf(&ResourceDiagnosis{Kind: r.Kind, Name: r.Name}, obj)
			}
			if e != nil {
				producers[e.TargetKind+"/"+e.TargetName] = r.Component
			}
		}
	}
	if len(producers) == 0 {
		return nil
	}
	declared := map[string]map[string]bool{}
	for _, c := range app.Spec.Components {
		declared[c.Name] = map[string]bool{}
		for _, dep := range c.DependsOn {
			declared[c.Name][dep] = true
		}
	}
	var findings []Finding
	seen := map[string]bool{}
	for i := range resources {
		r := &resources[i]
		obj := objects[resourceKey(r.APIVersion, r.Kind, r.Namespace, r.Name)]
		if !workloadKinds[r.Kind] || obj == nil {
			continue
		}
		for _, ref := range consumedRefs(obj) {
			producer, ok := producers[ref.kind+"/"+ref.name]
			if ref.optional || !ok || producer == r.Component || declared[r.Component][producer] {
				continue
			}
			key := r.Component + "\x00" + producer
			if seen[key] {
				continue
			}
			seen[key] = true
			what := fmt.Sprintf("%s %q", ref.kind, ref.name)
			if ref.key != "" {
				what += fmt.Sprintf(" key %q", ref.key)
			}
			findings = append(findings, Finding{
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("component %q: %s %s reads %s that component %q produces, and declares no dependsOn on it; the default workflow applies components in parallel, so the consumer can start before the object or its keys exist", r.Component, r.Kind, r.Name, what, producer),
				Evidence: fmt.Sprintf("rendered %s %s of component %q", r.Kind, r.Name, r.Component),
			})
		}
	}
	return findings
}
