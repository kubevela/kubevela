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

// Package nsrestrict reads a definition's restrictions: which namespaces'
// Applications may use it.
//
// Restrictions come from spec.restrictions, or from the
// definition.oam.dev/restrict-namespaces annotation where the spec is owned
// elsewhere, as it is for a builtin the chart installs. The annotation holds
// namespace names and globs; a label selector is spec-only. Neither can be a
// label, because label values reject "*" and ",".
//
// ComponentDefinition, TraitDefinition, PolicyDefinition, WorkflowStepDefinition
// and SourceDefinition are enforced. WorkloadDefinition is readable here but no
// caller looks one up, so restrict the ComponentDefinition instead.
package nsrestrict

import (
	"fmt"
	"path"
	"reflect"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// Of returns the restrictions a definition declares. A non-empty
// spec.restrictions wins over the annotation. nil means unrestricted.
func Of(obj client.Object) *common.DefinitionRestrictions {
	if isNil(obj) {
		return nil
	}
	if u, ok := obj.(*unstructured.Unstructured); ok {
		return OfUnstructured(*u)
	}
	if r, _ := fromSpec(obj); !isEmpty(r) {
		return r
	}
	return fromAnnotation(obj.GetAnnotations())
}

// NeedsNamespaceLabels reports whether Allows needs the target namespace's labels
// to decide, so a caller can skip fetching the Namespace otherwise. A name glob
// that already admits ns settles it.
func NeedsNamespaceLabels(obj client.Object, ns string) bool {
	r := Of(obj)
	if r == nil || r.NamespaceSelector == nil {
		return false
	}
	return !matchesName(r.Namespaces, ns)
}

// Allows reports whether a namespace satisfies the restrictions. The name globs
// and the selector are alternatives: either matching is enough. No restrictions
// allows every namespace.
//
// Pass nil nsLabels when the namespace could not be read. A selector then matches
// nothing.
func Allows(r *common.DefinitionRestrictions, ns string, nsLabels map[string]string) bool {
	if isEmpty(r) {
		return true
	}
	if matchesName(r.Namespaces, ns) {
		return true
	}
	return matchesSelector(r.NamespaceSelector, nsLabels)
}

// OfUnstructured returns the restrictions a definition declares, read from
// unstructured data. It is for callers that list definitions generically, such
// as the CLI, where the typed object is not to hand.
func OfUnstructured(obj unstructured.Unstructured) *common.DefinitionRestrictions {
	raw, found, err := unstructured.NestedMap(obj.Object, "spec", restrictionsField)
	if err != nil {
		// Present but unreadable. Something restricts this definition, so allow
		// nothing rather than read it as unrestricted.
		return denyAll()
	}
	if found {
		r := &common.DefinitionRestrictions{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, r); err != nil {
			return denyAll()
		}
		if !isEmpty(r) {
			return r
		}
	}
	return fromAnnotation(obj.GetAnnotations())
}

// denyAll is what an unreadable spec.restrictions block resolves to: a selector
// no namespace satisfies.
func denyAll() *common.DefinitionRestrictions {
	return &common.DefinitionRestrictions{
		NamespaceSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      "definition.oam.dev/unreadable-restrictions",
				Operator: metav1.LabelSelectorOpExists,
			}},
		},
	}
}

// KindOf names the definition's kind, for messages. A typed object loses its
// TypeMeta through a client Get, so GetObjectKind is empty by then.
func KindOf(obj client.Object) string {
	if isNil(obj) {
		return ""
	}
	_, kind := fromSpec(obj)
	return kind
}

// Check reports whether an Application in namespace ns may use the definition.
//
// The error says that the definition is restricted, never what it is restricted
// to. Whoever hits it cannot use the definition, so naming the allowed namespaces
// or labels would tell them how the cluster is carved up. Describe gives that
// detail to the operator, through the log.
func Check(obj client.Object, ns string, nsLabels map[string]string) error {
	if isNil(obj) {
		return nil
	}
	if Allows(Of(obj), ns, nsLabels) {
		return nil
	}
	_, kind := fromSpec(obj)
	return fmt.Errorf("%s %q is restricted and cannot be used from namespace %q",
		kind, obj.GetName(), ns)
}

// Describe renders a definition's restrictions for an operator-facing log line.
// It is not for the error returned to whoever submitted the Application.
func Describe(obj client.Object) string {
	return describe(Of(obj))
}

// Validate reports restrictions that cannot be applied: an empty entry, a glob
// path.Match rejects, or a selector that does not compile. The definition
// webhooks call it so a typo is caught on write.
func Validate(r *common.DefinitionRestrictions) error {
	if r == nil {
		return nil
	}
	for i, p := range r.Namespaces {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			return fmt.Errorf("namespaces[%d] is empty", i)
		}
		// Namespace names carry no whitespace, so a padded pattern matches nothing.
		// The annotation channel trims, so reject here rather than have one typo
		// behave two ways.
		if trimmed != p {
			return fmt.Errorf("namespaces[%d] %q has leading or trailing whitespace", i, p)
		}
		if _, err := path.Match(p, ""); err != nil {
			return fmt.Errorf("namespaces[%d] %q is not a valid pattern: %w", i, p, err)
		}
	}
	if r.NamespaceSelector != nil {
		if _, err := metav1.LabelSelectorAsSelector(r.NamespaceSelector); err != nil {
			return fmt.Errorf("namespaceSelector is not a valid label selector: %w", err)
		}
	}
	return nil
}

// ValidateObject validates the spec block and the annotation, not just whichever
// Of would return. An invalid annotation shadowed by a spec block becomes live as
// soon as that block is cleared.
func ValidateObject(obj client.Object) error {
	if isNil(obj) {
		return nil
	}
	spec, _ := fromSpec(obj)
	if err := Validate(spec); err != nil {
		return fmt.Errorf("spec.restrictions: %w", err)
	}
	if err := Validate(fromAnnotation(obj.GetAnnotations())); err != nil {
		return fmt.Errorf("%s: %w", oam.AnnotationRestrictNamespaces, err)
	}
	return nil
}

func matchesName(patterns []string, ns string) bool {
	for _, p := range patterns {
		// An unparseable pattern matches nothing.
		if matched, err := path.Match(p, ns); err == nil && matched {
			return true
		}
	}
	return false
}

func matchesSelector(sel *metav1.LabelSelector, nsLabels map[string]string) bool {
	if sel == nil || nsLabels == nil {
		return false
	}
	selector, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return false
	}
	return selector.Matches(labels.Set(nsLabels))
}

// describe renders the restrictions for an error message.
func describe(r *common.DefinitionRestrictions) string {
	var parts []string
	if r != nil && len(r.Namespaces) > 0 {
		parts = append(parts, fmt.Sprintf("namespaces [%s]", strings.Join(r.Namespaces, " ")))
	}
	if r != nil && r.NamespaceSelector != nil {
		if sel, err := metav1.LabelSelectorAsSelector(r.NamespaceSelector); err == nil {
			parts = append(parts, fmt.Sprintf("namespaces matching %q", sel.String()))
		} else {
			// A selector that will not compile matches nothing. Print it raw so the
			// message still names what has to be fixed.
			parts = append(parts, fmt.Sprintf("namespaces matching an unusable selector %+v (%v)",
				*r.NamespaceSelector, err))
		}
	}
	if len(parts) == 0 {
		return "no namespaces"
	}
	return strings.Join(parts, " or ")
}

func isEmpty(r *common.DefinitionRestrictions) bool {
	return r == nil || (len(r.Namespaces) == 0 && r.NamespaceSelector == nil)
}

// fromSpec returns the restrictions in the definition's spec, with its kind for
// error messages. A non-definition yields no restrictions and its Go type name.
func fromSpec(obj client.Object) (*common.DefinitionRestrictions, string) {
	switch def := obj.(type) {
	case *v1beta1.ComponentDefinition:
		return def.Spec.Restrictions, "ComponentDefinition"
	case *v1beta1.TraitDefinition:
		return def.Spec.Restrictions, "TraitDefinition"
	case *v1beta1.PolicyDefinition:
		return def.Spec.Restrictions, "PolicyDefinition"
	case *v1beta1.WorkflowStepDefinition:
		return def.Spec.Restrictions, "WorkflowStepDefinition"
	case *v1beta1.SourceDefinition:
		return def.Spec.Restrictions, "SourceDefinition"
	case *unstructured.Unstructured:
		return OfUnstructured(*def), def.GetKind()
	case *v1beta1.WorkloadDefinition:
		// WorkloadDefinition's CRD is frozen against regeneration
		// (hack/crd/dispatch/dispatch.go), so the API server prunes any spec field
		// added to it. Annotation only.
		return nil, "WorkloadDefinition"
	default:
		// Every definition kind is handled above; this names whatever else arrived.
		t := reflect.TypeOf(obj)
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		return nil, t.Name()
	}
}

// fromAnnotation builds restrictions from the annotation, which holds namespace
// names and globs only.
// restrictionsField is the spec field restrictions live in.
const restrictionsField = "restrictions"

func fromAnnotation(annotations map[string]string) *common.DefinitionRestrictions {
	r := &common.DefinitionRestrictions{}
	for _, p := range strings.Split(annotations[oam.AnnotationRestrictNamespaces], ",") {
		if p = strings.TrimSpace(p); p != "" {
			r.Namespaces = append(r.Namespaces, p)
		}
	}
	if isEmpty(r) {
		return nil
	}
	return r
}

// isNil reports a typed nil pointer, which a client.Object holding one does not
// compare equal to nil.
func isNil(obj client.Object) bool {
	if obj == nil {
		return true
	}
	v := reflect.ValueOf(obj)
	return v.Kind() == reflect.Ptr && v.IsNil()
}
