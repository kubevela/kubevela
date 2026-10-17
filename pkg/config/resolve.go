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

package config

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"cuelang.org/go/cue"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/config/writer"
	"github.com/oam-dev/kubevela/pkg/cue/script"
)

// ResolvedTemplate is a template resolved for a Config, from a ConfigTemplate CRD or a
// legacy config-template-* ConfigMap. Shared by the Config controller and validating
// webhook so both resolvers agree on what a template looks like, Sensitive included.
type ResolvedTemplate struct {
	Name      string
	Namespace string
	CUE       script.CUE
	// Scope is the stored value: spec.scope of a ConfigTemplate, the config.oam.dev/scope
	// label of a legacy ConfigMap (free text there). EffectiveScope is what the rules read.
	Scope     string
	Sensitive bool
	// Legacy marks a template that is still a config-template-* ConfigMap.
	Legacy bool
	// Grandfathered lists, as namespace/name, the configs in other namespaces that were
	// bound to the template when migration adopted it (AnnotationGrandfatheredConfigs).
	Grandfathered []string
	// ExpandedWriter is the template's nacos block, read from the CUE on both paths so
	// the controller and the webhook agree on whether a template writes to Nacos. Nil
	// Nacos when the template has no block or its CUE does not compile; the reconciler's
	// own parse reports the latter.
	ExpandedWriter writer.ExpandedWriterConfig
}

// EffectiveScope is the scope every reader applies, whatever the stored value says. A
// system template lives in vela-system: a ConfigTemplate elsewhere that claims scope system
// (it reached the cluster with the webhooks off; its controller reports Error) reads as
// namespace, so it grants nothing beyond its own namespace. A legacy ConfigMap in vela-system
// is system whatever its label (project, global, env and friends were never enforced and
// those templates have always been used from every namespace); elsewhere it is namespace.
func (t *ResolvedTemplate) EffectiveScope() configv1alpha1.ConfigTemplateScope {
	if t.Legacy {
		return LegacyTemplateScope(t.Namespace)
	}
	if t.Namespace == apitypes.DefaultKubeVelaNS && t.Scope == string(configv1alpha1.ConfigTemplateScopeSystem) {
		return configv1alpha1.ConfigTemplateScopeSystem
	}
	return configv1alpha1.ConfigTemplateScopeNamespace
}

// LegacyTemplateScope is the scope of a config-template-* ConfigMap, by its namespace alone:
// system in vela-system, namespace elsewhere. It is what the resolver reads for one, and what
// migration writes onto the ConfigTemplate it becomes, so adoption changes nothing a Config can
// observe. The ConfigMap's scope label (system, project, global, env, written by older tools)
// never restricted anything and is not consulted.
func LegacyTemplateScope(namespace string) configv1alpha1.ConfigTemplateScope {
	if namespace == apitypes.DefaultKubeVelaNS {
		return configv1alpha1.ConfigTemplateScopeSystem
	}
	return configv1alpha1.ConfigTemplateScopeNamespace
}

// SystemScopeMisplaced reports a ConfigTemplate that claims scope system outside vela-system,
// the one placement the rules forbid.
func SystemScopeMisplaced(namespace string, scope configv1alpha1.ConfigTemplateScope) bool {
	return scope == configv1alpha1.ConfigTemplateScopeSystem && namespace != apitypes.DefaultKubeVelaNS
}

// SystemScopePlacementError is the message every layer gives for SystemScopeMisplaced: the
// CLI before it writes, the webhook at admission, the ConfigTemplate controller on its status.
func SystemScopePlacementError(namespace, name string) error {
	return fmt.Errorf("config template %s/%s: scope system is only valid in %s; a system template lives in %s and is usable from any namespace, a namespace-scoped one lives with the configs that use it",
		namespace, name, apitypes.DefaultKubeVelaNS, apitypes.DefaultKubeVelaNS)
}

// TemplateAccess is the answer to "may a Config in fromNamespace use this template". A nil
// Review is yes without a user: the same namespace, or a system template. Otherwise the answer
// is the requesting user's, through the review Review names (get on the ConfigTemplate, the
// object a user is granted, whichever backend holds the template today), with Reason as the
// denial.
// A config the template grandfathers (ResolvedTemplate.Grandfathers) needs no review either;
// callers check that themselves, since it depends on the config's name. A write with no user to
// ask (the controller's own) gets only the structural answers: a review is a refusal.
type TemplateAccess struct {
	Review *authorizationv1.ResourceAttributes
	Reason string
}

// AccessToTemplate applies the table of issue #7410 section 6.
func AccessToTemplate(tmpl *ResolvedTemplate, fromNamespace string) TemplateAccess {
	if tmpl.Namespace == fromNamespace || tmpl.EffectiveScope() == configv1alpha1.ConfigTemplateScopeSystem {
		return TemplateAccess{}
	}
	return TemplateAccess{
		Review: &authorizationv1.ResourceAttributes{
			Verb:      "get",
			Group:     configv1alpha1.Group,
			Version:   configv1alpha1.Version,
			Resource:  "configtemplates",
			Namespace: tmpl.Namespace,
			Name:      tmpl.Name,
		},
		Reason: fmt.Sprintf("config template %s/%s is namespace-scoped: a config in %s may use it only if its author can get it (grant get on configtemplates.%s %s in namespace %s)",
			tmpl.Namespace, tmpl.Name, fromNamespace, configv1alpha1.Group, tmpl.Name, tmpl.Namespace),
	}
}

// PendingTemplateError is the answer for a Config whose template is not usable yet (pending
// is what ResolveConfigTemplate returned with waiting, nil when the template does not exist
// at all). It is nil, and the Config waits for the template as before, in the Config's own
// namespace, and for a system template in vela-system: its scope is known from its spec and
// grants everyone. Otherwise it is a refusal: the template's scope decides who may use it,
// and with templateRef immutable the create is the only time anyone asks, so the Config
// waits until the template is there to be asked about.
func PendingTemplateError(pending *ResolvedTemplate, templateNamespace, name, configNamespace string) error {
	if templateNamespace == configNamespace {
		return nil
	}
	if pending != nil && pending.EffectiveScope() == configv1alpha1.ConfigTemplateScopeSystem {
		return nil
	}
	return fmt.Errorf("config template %s/%s %s: a config in %s may reference a template in another namespace once it is, when its scope decides who may use it",
		templateNamespace, name, pendingTemplatePhrase, configNamespace)
}

// pendingTemplatePhrase is how PendingTemplateError's refusal reads, which migration's
// retry recognises in the webhook's answer.
const pendingTemplatePhrase = "is not available yet"

// AnnotationGrandfatheredConfigs lists, on a ConfigTemplate that migration adopted, the
// configs in other namespaces that were bound to the legacy template at that moment, as
// comma-separated namespace/name. 1.11 enforced no scope, so those bindings were allowed;
// they keep working without the grant the access table would now ask for. A binding made
// after the adoption is not on the list, and the ConfigTemplate webhook refuses an update that
// adds to it: the list is written once, when the template is created.
const AnnotationGrandfatheredConfigs = "config.oam.dev/grandfathered-configs"

// Grandfathers reports whether the config at key was bound to the template when migration
// adopted it.
func (t *ResolvedTemplate) Grandfathers(key client.ObjectKey) bool {
	return slices.Contains(t.Grandfathered, key.Namespace+"/"+key.Name)
}

// grandfatheredOf reads AnnotationGrandfatheredConfigs.
func grandfatheredOf(annotations map[string]string) []string {
	return GrandfatheredOf(annotations)
}

// GrandfatheredOf reads AnnotationGrandfatheredConfigs from annotations.
func GrandfatheredOf(annotations map[string]string) []string {
	var out []string
	for _, entry := range strings.Split(annotations[AnnotationGrandfatheredConfigs], ",") {
		if entry = strings.TrimSpace(entry); entry != "" {
			out = append(out, entry)
		}
	}
	return out
}

// EndpointNamespaceFree reports a nacos endpoint namespace a template may name with no review:
// its own namespace, or vela-system, where the shared endpoint configs live.
func EndpointNamespaceFree(templateNamespace, endpointNamespace string) bool {
	return endpointNamespace == templateNamespace || endpointNamespace == apitypes.DefaultKubeVelaNS
}

// RenderedEndpointError is the reconciler's check of the endpoint a Config renders, which the
// controller then reads with its own rights. A free namespace is fine. Elsewhere, only the
// endpoint the ConfigTemplate's text names: admission reviewed that one for the template's
// author, and admission requires it to be a plain literal, so a Config cannot move it. A legacy
// ConfigMap template was never reviewed, and has no foreign endpoint at all.
func RenderedEndpointError(tmpl *ResolvedTemplate, endpoint writer.ConfigRef) error {
	if EndpointNamespaceFree(tmpl.Namespace, endpoint.Namespace) {
		return nil
	}
	if !tmpl.Legacy && tmpl.ExpandedWriter.Nacos != nil && tmpl.ExpandedWriter.Nacos.Endpoint == endpoint {
		return nil
	}
	return fmt.Errorf("config template %s/%s: the nacos endpoint %s/%s is outside %s and %s, and is not the one the template names: an endpoint in another namespace must be written in a ConfigTemplate's text, where admission reviews it",
		tmpl.Namespace, tmpl.Name, endpoint.Namespace, endpoint.Name, tmpl.Namespace, apitypes.DefaultKubeVelaNS)
}

// TemplateRefNamespace is the namespace a templateRef names: vela-system when it names none.
func TemplateRefNamespace(ref *configv1alpha1.ConfigTemplateReference) string {
	if ref.Namespace == "" {
		return apitypes.DefaultKubeVelaNS
	}
	return ref.Namespace
}

// ResolveConfigTemplate looks up the referenced template, CRD first, falling back to a
// legacy ConfigMap. waiting is true if the ConfigTemplate exists but is not usable yet
// (templateUsable: the controller has not extracted its schema, or reported a parse
// Error); tmpl then carries what TemplateScopeOf does and no CUE, so a caller can still
// tell a system template in vela-system from the rest. err is ErrTemplateNotFound if
// neither the CRD nor the legacy ConfigMap exist.
func ResolveConfigTemplate(ctx context.Context, cli client.Client, ref *configv1alpha1.ConfigTemplateReference) (tmpl *ResolvedTemplate, waiting bool, err error) {
	ct, cm, err := getTemplate(ctx, cli, ref)
	if err != nil {
		return nil, false, err
	}
	if ct != nil {
		tmpl = templateOfCR(ct)
		if !templateUsable(ct) {
			return tmpl, true, nil
		}
		tmpl.CUE = script.CUE(ct.Spec.Template)
	} else {
		tmpl = templateOfConfigMap(ref.Name, cm)
		tmpl.CUE = script.CUE(cm.Data[SaveTemplateKey])
	}
	tmpl.ExpandedWriter = expandedWriterOf(ctx, tmpl.CUE)
	return tmpl, false, nil
}

// TemplateScopeOf is ResolveConfigTemplate without the CUE: name, namespace, stored scope,
// sensitivity, the legacy mark and the grandfathered bindings, from one read and no compile.
// Enough for EffectiveScope and AccessToTemplate.
func TemplateScopeOf(ctx context.Context, cli client.Client, ref *configv1alpha1.ConfigTemplateReference) (tmpl *ResolvedTemplate, waiting bool, err error) {
	ct, cm, err := getTemplate(ctx, cli, ref)
	switch {
	case err != nil:
		return nil, false, err
	case ct != nil:
		return templateOfCR(ct), !templateUsable(ct), nil
	default:
		return templateOfConfigMap(ref.Name, cm), false, nil
	}
}

// getTemplate reads the CR, or failing that the legacy ConfigMap. A cluster without the
// ConfigTemplate kind reads the ConfigMap, as LoadTemplate does.
func getTemplate(ctx context.Context, cli client.Client, ref *configv1alpha1.ConfigTemplateReference) (*configv1alpha1.ConfigTemplate, *corev1.ConfigMap, error) {
	ns := TemplateRefNamespace(ref)
	var ct configv1alpha1.ConfigTemplate
	switch err := cli.Get(ctx, client.ObjectKey{Namespace: ns, Name: ref.Name}, &ct); {
	case err == nil:
		return &ct, nil, nil
	case apierrors.IsNotFound(err), IsCRDTypeMissing(err):
	default:
		return nil, nil, err
	}
	var cm corev1.ConfigMap
	if err := cli.Get(ctx, client.ObjectKey{Namespace: ns, Name: TemplateConfigMapNamePrefix + ref.Name}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil, ErrTemplateNotFound
		}
		return nil, nil, err
	}
	return nil, &cm, nil
}

func templateOfCR(ct *configv1alpha1.ConfigTemplate) *ResolvedTemplate {
	return &ResolvedTemplate{
		Name:          ct.Name,
		Namespace:     ct.Namespace,
		Scope:         string(ct.Spec.Scope),
		Sensitive:     ct.Spec.Sensitive,
		Grandfathered: grandfatheredOf(ct.Annotations),
	}
}

func templateOfConfigMap(name string, cm *corev1.ConfigMap) *ResolvedTemplate {
	return &ResolvedTemplate{
		Name:      name,
		Namespace: cm.Namespace,
		Scope:     cm.Labels[apitypes.LabelConfigScope],
		Sensitive: cm.Annotations[apitypes.AnnotationConfigSensitive] == sensitiveAnnotationValue,
		Legacy:    true,
	}
}

// templateUsable is true once the controller has extracted the template's schema: Available,
// or Error for placement only (a system template outside vela-system keeps its schema; it
// reads as namespace-scoped through EffectiveScope). A parse Error has no schema, so it is not
// usable; a template the controller has not reached yet is not either.
func templateUsable(ct *configv1alpha1.ConfigTemplate) bool {
	if ct.Status.Phase == configv1alpha1.ConfigTemplatePhaseAvailable {
		return true
	}
	return ct.Status.Phase == configv1alpha1.ConfigTemplatePhaseError &&
		SystemScopeMisplaced(ct.Namespace, ct.Spec.Scope) && ct.Status.Schema != nil
}

// expandedWriterOf reads the nacos block of a template's CUE. The legacy ConfigMap also
// carries it as yaml under the expanded-writer key; parsing the CUE on both paths keeps
// one code path and one set of tests. The parse is the CueX one the Secret render uses,
// with provider functions disabled, so a template that imports a vela package keeps its
// block; the plain compiler would reject the import and the block would go missing
// without a word. A template that does not compile at all has no block: the
// ConfigTemplate controller reports that.
func expandedWriterOf(ctx context.Context, cueScript script.CUE) writer.ExpandedWriterConfig {
	v, err := cueScript.ParseToValueWithCueX(ctx)
	if err != nil {
		return writer.ExpandedWriterConfig{}
	}
	return writer.ParseExpandedWriterConfig(v.LookupPath(cue.ParsePath("template")))
}
