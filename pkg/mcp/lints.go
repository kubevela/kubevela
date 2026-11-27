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
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// How a CUE health policy reads an auxiliary output: by name with dot or
// bracket notation, or the whole map, as in a comprehension or a default
// expression, which may select any output.
var (
	outputByDot     = regexp.MustCompile(`context\.outputs\.([A-Za-z_][A-Za-z0-9_]*)`)
	outputByBracket = regexp.MustCompile(`context\.outputs\["((?:[^"\\]|\\.)*)"\]`)
	outputsWhole    = regexp.MustCompile(`context\.outputs(?:[^.\["A-Za-z0-9_]|$)`)
)

// outputsRead returns the auxiliary outputs a health policy names, and
// whether it reads the whole outputs map, which may reach any of them.
func outputsRead(policy string) (names map[string]bool, whole bool) {
	names = map[string]bool{}
	for _, m := range outputByDot.FindAllStringSubmatch(policy, -1) {
		names[m[1]] = true
	}
	for _, m := range outputByBracket.FindAllStringSubmatch(policy, -1) {
		names[strings.ReplaceAll(m[1], `\"`, `"`)] = true
	}
	return names, outputsWhole.MatchString(policy)
}

// healthCoverageFindings reports a component definition whose health rule
// leaves auxiliary outputs the template emits unjudged. Such a component is
// healthy as soon as the outputs it does read are, before its members and
// the address a consumer needs exist, and every component that depends on
// it is released at that moment. The check is static: it reads the
// template's output paths and the names the policy text reads; a policy
// that reads the whole outputs map is taken to cover them all.
func healthCoverageFindings(defType string, produces []ProducedResource, health HealthRules, evidence string) []Finding {
	if defType != DefinitionTypeComponent {
		return nil
	}
	var aux []string
	kinds := map[string]string{}
	for _, p := range produces {
		if !strings.HasPrefix(p.Path, "outputs.") {
			continue
		}
		name := strings.TrimPrefix(p.Path, "outputs.")
		aux = append(aux, name)
		kinds[name] = p.Kind
	}
	if len(aux) == 0 {
		return nil
	}
	describe := func(names []string) string {
		parts := make([]string, 0, len(names))
		for _, n := range names {
			if kinds[n] != "" {
				n += " (" + kinds[n] + ")"
			}
			parts = append(parts, n)
		}
		return strings.Join(parts, ", ")
	}
	policy := strings.TrimSpace(health.HealthPolicy)
	if policy == "" {
		return []Finding{{
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("the definition declares no health policy, so health falls back to the built-in check on the primary resource; its %d auxiliary outputs (%s) are not judged, and a component that depends on this one is released before they exist", len(aux), describe(aux)),
			Evidence: evidence,
		}}
	}
	read, whole := outputsRead(policy)
	if whole {
		return nil
	}
	var missing []string
	for _, n := range aux {
		if !read[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	what := "reads context.output only"
	if len(read) > 0 {
		what = fmt.Sprintf("leaves %d of its %d auxiliary outputs unread", len(missing), len(aux))
	}
	return []Finding{{
		Severity: SeverityWarning,
		Message:  fmt.Sprintf("the health policy %s, so the component is healthy before %s is; a component that depends on this one is released at that moment and can start before those exist", what, describe(missing)),
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
	// namespace/kind/name -> component; an exporter's target lives in the
	// namespace it names, else the exporter's own
	producers := map[string]string{}
	key := func(ns, kind, name string) string { return ns + "\x00" + kind + "\x00" + name }
	for i := range resources {
		r := &resources[i]
		obj := objects[resourceKey(r.APIVersion, r.Kind, r.Namespace, r.Name)]
		switch {
		case r.Kind == secretKind || r.Kind == "ConfigMap":
			producers[key(r.Namespace, r.Kind, r.Name)] = r.Component
		case obj != nil:
			var e *ExportDetail
			if r.Kind == fieldExportKind {
				e = fieldExportOf(obj)
			} else if groupOf(r.APIVersion) != "" && !builtinGroups[groupOf(r.APIVersion)] {
				e = connectionSecretOf(&ResourceDiagnosis{APIVersion: r.APIVersion, Kind: r.Kind, Name: r.Name}, obj)
			}
			if e != nil {
				ns := e.TargetNamespace
				if ns == "" {
					ns = r.Namespace
				}
				producers[key(ns, e.TargetKind, e.TargetName)] = r.Component
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
			producer, ok := producers[key(r.Namespace, ref.kind, ref.name)]
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
