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
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// Of returns the restrictions a definition declares, combining the two channels.
// nil means unrestricted.
func Of(obj client.Object) *common.DefinitionRestrictions {
	if isNil(obj) {
		return nil
	}
	if u, ok := obj.(*unstructured.Unstructured); ok {
		return OfUnstructured(*u)
	}
	r, _ := fromSpec(obj)
	return effective(r, obj.GetAnnotations())
}

// effective combines the two channels. The spec settles the namespaces where it
// names any, the annotation otherwise. A quota is spec-only, so adding one never
// widens who may use the definition.
func effective(spec *common.DefinitionRestrictions, annotations map[string]string) *common.DefinitionRestrictions {
	if restrictsNamespaces(spec) {
		return spec // already carries its own quota
	}
	ann := fromAnnotation(annotations)
	if ann == nil {
		if isEmpty(spec) {
			return nil
		}
		return spec // quota only
	}
	if spec == nil || len(spec.Quota) == 0 {
		return ann
	}
	// Namespaces from the annotation, quota from the spec.
	return &common.DefinitionRestrictions{Namespaces: ann.Namespaces, Quota: spec.Quota}
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
	// A block carrying only a quota restricts no namespace.
	if !restrictsNamespaces(r) {
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
	var spec *common.DefinitionRestrictions
	if found {
		spec = &common.DefinitionRestrictions{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, spec); err != nil {
			return denyAll()
		}
	}
	return effective(spec, obj.GetAnnotations())
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

// isSystemNamespace reports a namespace no quota applies to. Addons install their
// own Applications using these definitions, so a quota there breaks installation
// rather than capping a tenant.
//
// Both names are checked because they are configured by different paths and can
// disagree. oam.SystemDefinitionNamespace follows --system-definition-namespace,
// which the chart sets to the release namespace. types.DefaultKubeVelaNS is where
// addon Applications are placed, and only the CLI binds it from the environment;
// in the controller it keeps its compiled default. Exempting one would leave the
// other quota'd on any install outside vela-system.
func isSystemNamespace(ns string) bool {
	return ns == types.DefaultKubeVelaNS || ns == oam.SystemDefinitionNamespace
}

// QuotaExempt reports whether a namespace has opted out of every quota, from its
// own annotations. Only "true" counts, so a typo leaves the quota in force.
func QuotaExempt(nsAnnotations map[string]string) bool {
	return nsAnnotations[oam.AnnotationQuotaExempt] == "true"
}

// QuotaFor returns the quota entry governing namespace ns, or nil when none does.
// The first matching entry wins, so an entry with no matcher reads as the default
// and belongs last.
//
// nsLabels may be nil when the namespace could not be read; an entry selecting on
// labels then does not match, exactly as Allows treats it.
func QuotaFor(obj client.Object, ns string, nsLabels map[string]string) *common.NamespaceQuota {
	if isSystemNamespace(ns) {
		return nil
	}
	r := Of(obj)
	if r == nil {
		return nil
	}
	for i := range r.Quota {
		q := &r.Quota[i]
		if matchesName(q.Namespaces, ns) || matchesSelector(q.NamespaceSelector, nsLabels) {
			return q
		}
		if len(q.Namespaces) == 0 && q.NamespaceSelector == nil {
			return q
		}
	}
	return nil
}

// QuotaNeedsNamespaceLabels reports whether picking the quota entry for ns requires
// the namespace's labels, so a caller can skip fetching the Namespace otherwise.
func QuotaNeedsNamespaceLabels(obj client.Object, ns string) bool {
	if isSystemNamespace(ns) {
		return false
	}
	r := Of(obj)
	if r == nil {
		return false
	}
	for i := range r.Quota {
		q := &r.Quota[i]
		if matchesName(q.Namespaces, ns) {
			return false // an earlier entry already decides it
		}
		if q.NamespaceSelector != nil {
			return true
		}
		if len(q.Namespaces) == 0 {
			return false // the no-matcher default decides it
		}
	}
	return false
}

// ExceedsQuota reports whether a total breaches the entry's ceiling, and whether it
// has reached the point the entry wants flagged.
func ExceedsQuota(q *common.NamespaceQuota, total int) (refuse bool, warn bool) {
	if q == nil {
		return false, false
	}
	if q.Limit != nil && total > int(*q.Limit) {
		return true, false
	}
	if q.Warn != nil && total >= int(*q.Warn) {
		return false, true
	}
	return false, false
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
	for i := range r.Quota {
		q := &r.Quota[i]
		if err := validateQuota(q); err != nil {
			return fmt.Errorf("quota[%d]: %w", i, err)
		}
		// The first matching entry wins, so an entry with no matcher takes every
		// namespace and leaves anything after it unreachable. Only the last entry
		// may be the default.
		if len(q.Namespaces) == 0 && q.NamespaceSelector == nil && i != len(r.Quota)-1 {
			return fmt.Errorf("quota[%d] matches every namespace, so quota[%d] onwards could never apply", i, i+1)
		}
	}
	return nil
}

func validateQuota(q *common.NamespaceQuota) error {
	for j, p := range q.Namespaces {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			return fmt.Errorf("namespaces[%d] is empty", j)
		}
		if trimmed != p {
			return fmt.Errorf("namespaces[%d] %q has leading or trailing whitespace", j, p)
		}
		if _, err := path.Match(p, ""); err != nil {
			return fmt.Errorf("namespaces[%d] %q is not a valid pattern: %w", j, p, err)
		}
	}
	if q.NamespaceSelector != nil {
		if _, err := metav1.LabelSelectorAsSelector(q.NamespaceSelector); err != nil {
			return fmt.Errorf("namespaceSelector is not a valid label selector: %w", err)
		}
	}
	if q.Warn == nil && q.Limit == nil {
		return fmt.Errorf("sets neither warn nor limit, so it does nothing")
	}
	// A ceiling below the warning never warns before it refuses, which is a typo
	// rather than a policy.
	if q.Warn != nil && q.Limit != nil && *q.Warn > *q.Limit {
		return fmt.Errorf("warn %d is above the limit %d, so it would never warn", *q.Warn, *q.Limit)
	}
	return nil
}

// validateQuotaKind rejects a quota on a kind nothing counts. A namespace
// accumulates components and the traits on them; a policy, workflow step or source
// is part of how one Application is put together, so capping them per namespace
// would say nothing. Rejected on write rather than ignored at admission, because a
// quota that silently does nothing is worse than one that will not apply.
func validateQuotaKind(r *common.DefinitionRestrictions, kind string) error {
	if r == nil || len(r.Quota) == 0 {
		return nil
	}
	switch kind {
	case "ComponentDefinition", "TraitDefinition":
		return nil
	default:
		return fmt.Errorf("quota is not supported on %s, only on ComponentDefinition and TraitDefinition", kind)
	}
}

// ValidateObject validates the spec block and the annotation, not just whichever
// Of would return. An invalid annotation shadowed by a spec block becomes live as
// soon as that block is cleared.
func ValidateObject(obj client.Object) error {
	if isNil(obj) {
		return nil
	}
	spec, kind := fromSpec(obj)
	if err := Validate(spec); err != nil {
		return fmt.Errorf("spec.restrictions: %w", err)
	}
	if err := validateQuotaKind(spec, kind); err != nil {
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

// isEmpty reports a block that declares nothing at all, so the annotation channel
// should be consulted instead. A quota counts: a definition may cap usage without
// restricting which namespaces may use it.
func isEmpty(r *common.DefinitionRestrictions) bool {
	return r == nil || (!restrictsNamespaces(r) && len(r.Quota) == 0)
}

// restrictsNamespaces reports whether the block limits which namespaces may use the
// definition at all, as opposed to only capping how much they may use.
func restrictsNamespaces(r *common.DefinitionRestrictions) bool {
	return r != nil && (len(r.Namespaces) > 0 || r.NamespaceSelector != nil)
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
