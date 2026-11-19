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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/cue/script"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/sources"
)

// MigrationOutcome is what happened to one legacy object during a migration run.
type MigrationOutcome string

const (
	// MigrationAdopted means a CR now stands for the legacy object, which it owns.
	MigrationAdopted MigrationOutcome = "adopted"
	// MigrationSkipped means the object was already migrated or opted out.
	MigrationSkipped MigrationOutcome = "skipped"
	// MigrationConflict means a CR of the target name exists that migration did not
	// create; nothing was written.
	MigrationConflict MigrationOutcome = "conflict"
	// MigrationFailed means a write or lookup for this object failed; the run went on.
	MigrationFailed MigrationOutcome = "failed"
)

// Event reasons recorded on legacy objects while they are migrated.
const (
	EventMigrationAdopted  = "MigrationAdopted"
	EventMigrationSkipped  = "MigrationSkipped"
	EventMigrationConflict = "MigrationConflict"
	EventMigrationFailed   = "MigrationFailed"
)

// migrationFieldOwner is the field manager of every write migration makes, so a GitOps tool
// that applies the legacy objects server-side sees the owner references and annotations
// migration adds owned by another manager, not as drift in its own fields.
var migrationFieldOwner = client.FieldOwner("kubevela-config-migration")

const (
	reasonAlreadyMigrated   = "already migrated"
	reasonOptedOut          = "opted out"
	reasonAdoptionCompleted = "adoption completed"
	reasonSourceCacheEntry  = "source cache entry managed by the controller"
	migrateOptOutValue      = "false"
	propertiesSecretSuffix  = "-properties"
	kindConfigMap           = "ConfigMap"
	kindSecret              = "Secret"
)

// MigrationResult is one row of a migration report: the legacy source object and
// what happened to it.
type MigrationResult struct {
	// Kind, Namespace and Name identify the legacy object. Name is the template name
	// for a template ConfigMap, not the ConfigMap name.
	Kind, Namespace, Name string
	Outcome               MigrationOutcome
	// Reason is one short line for the table and the Event message.
	Reason string
	// Target is the CR written for an adopted object, or the one that would be
	// written on a dry run.
	Target client.Object
}

// MigrationReport collects the results of one run.
type MigrationReport struct {
	Results []MigrationResult
}

func (r *MigrationReport) add(res MigrationResult) {
	r.Results = append(r.Results, res)
}

// Count returns how many results have the outcome.
func (r *MigrationReport) Count(outcome MigrationOutcome) int {
	n := 0
	for _, res := range r.Results {
		if res.Outcome == outcome {
			n++
		}
	}
	return n
}

// HasConflict reports whether any result is a conflict.
func (r *MigrationReport) HasConflict() bool { return r.Count(MigrationConflict) > 0 }

// HasFailure reports whether any result is a failure.
func (r *MigrationReport) HasFailure() bool { return r.Count(MigrationFailed) > 0 }

// MigrationOptions tune a run.
type MigrationOptions struct {
	// DryRun computes the report and fills Target without writing or recording.
	DryRun bool
	// Recorder receives Events on the legacy objects; nil records nothing.
	Recorder record.EventRecorder
}

// Migrator adopts legacy config templates (config-template-* ConfigMaps) and
// configs (velacore-config Secrets) into ConfigTemplate and Config CRs. The legacy
// objects are never deleted. A config Secret becomes owned by its Config, which
// then renders into it as it would into its own Secret. A template ConfigMap is
// only annotated with the ConfigTemplate's UID: templates are shared across
// namespaces and the ConfigMap must outlive the CR for a downgrade. Both carry
// the annotation so a second run skips them.
type Migrator struct {
	cli client.Client
}

// NewMigrator returns a Migrator writing through cli.
func NewMigrator(cli client.Client) *Migrator {
	return &Migrator{cli: cli}
}

// MigrateTemplate migrates the legacy template name in ns and every legacy config
// that references it, in any namespace. The error return is for a missing template
// or a broken client; per-object problems are Failed results.
func (m *Migrator) MigrateTemplate(ctx context.Context, ns, name string, opts MigrationOptions) (MigrationReport, error) {
	var report MigrationReport
	var cm corev1.ConfigMap
	if err := m.cli.Get(ctx, client.ObjectKey{Namespace: ns, Name: TemplateConfigMapNamePrefix + name}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return report, fmt.Errorf("the config template %s not found", name)
		}
		return report, err
	}
	if cm.Labels[types.LabelConfigCatalog] != types.VelaCoreConfig {
		return report, fmt.Errorf("found a configmap but is not a config template")
	}
	if err := m.migrateTemplateAndConfigs(ctx, &cm, opts, &report, map[client.ObjectKey]bool{}); err != nil {
		return report, err
	}
	return report, nil
}

// MigrateConfig migrates one legacy config Secret and nothing else. Its template may
// be a CR or still a legacy ConfigMap; a Config resolves either. The error return is
// for a missing or non-config Secret or a broken client.
func (m *Migrator) MigrateConfig(ctx context.Context, ns, name string, opts MigrationOptions) (MigrationReport, error) {
	var report MigrationReport
	var secret corev1.Secret
	if err := m.cli.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return report, fmt.Errorf("the config %s not found", name)
		}
		return report, err
	}
	if secret.Labels[types.LabelConfigCatalog] != types.VelaCoreConfig {
		return report, fmt.Errorf("found a secret but is not a config")
	}
	m.migrateConfigSecret(ctx, &secret, opts, &report)
	return report, nil
}

// MigrateConfigs migrates every legacy config Secret in ns, with or without a
// template. Secrets a Config CR materialised itself are not legacy and are left out.
func (m *Migrator) MigrateConfigs(ctx context.Context, ns string, opts MigrationOptions) (MigrationReport, error) {
	var report MigrationReport
	secrets, err := m.listLegacyConfigs(ctx, ns)
	if err != nil {
		return report, err
	}
	m.migrateConfigList(ctx, secrets, opts, &report, map[client.ObjectKey]bool{})
	return report, nil
}

// MigrateNamespace migrates every legacy template in ns, each with its configs
// wherever they live, then every legacy config left in ns: template-less ones and
// those whose template lives in another namespace. The configs come from one
// cluster-wide list grouped by template; when that list is refused, ns alone is
// listed and one skipped row says what was not covered.
func (m *Migrator) MigrateNamespace(ctx context.Context, ns string, opts MigrationOptions) (MigrationReport, error) {
	return m.migrateTemplatesAndConfigs(ctx, ns, opts)
}

// MigrateAll migrates every legacy template in the cluster, each with its configs
// wherever they live, then every legacy config left anywhere. Templates are visited
// in namespace then name order. One ConfigMap list and one Secret list serve the
// whole run, however many templates there are. The error return is for a refused
// list only; per-object problems are rows.
func (m *Migrator) MigrateAll(ctx context.Context, opts MigrationOptions) (MigrationReport, error) {
	return m.migrateTemplatesAndConfigs(ctx, "", opts)
}

// migrateTemplatesAndConfigs is MigrateNamespace for one namespace and MigrateAll
// for ns == "".
func (m *Migrator) migrateTemplatesAndConfigs(ctx context.Context, ns string, opts MigrationOptions) (MigrationReport, error) {
	var report MigrationReport
	cms, err := m.listLegacyTemplates(ctx, ns)
	if err != nil {
		return report, err
	}
	groups, err := m.listLegacyConfigGroups(ctx, ns)
	if err != nil {
		return report, err
	}
	seen := map[client.ObjectKey]bool{}
	for i := range cms {
		name := strings.TrimPrefix(cms[i].Name, TemplateConfigMapNamePrefix)
		bound := groups.of(cms[i].Namespace, name)
		m.migrateTemplateConfigMap(ctx, &cms[i], bound, opts, &report)
		m.migrateConfigList(ctx, bound, opts, &report, seen)
	}
	if groups.unlisted != nil {
		report.add(unlistedNamespacesRow(groups.unlisted))
	}
	m.migrateConfigList(ctx, groups.remaining(ns), opts, &report, seen)
	return report, nil
}

// migrateTemplateAndConfigs migrates one template ConfigMap, then the configs that
// reference it. Configs already reported in this run (seen) are skipped. The configs
// are listed before anything is written, so a failed list leaves the cluster as it was.
func (m *Migrator) migrateTemplateAndConfigs(ctx context.Context, cm *corev1.ConfigMap, opts MigrationOptions, report *MigrationReport, seen map[client.ObjectKey]bool) error {
	name := strings.TrimPrefix(cm.Name, TemplateConfigMapNamePrefix)
	secrets, unlisted, err := m.listConfigsOfTemplate(ctx, cm.Namespace, name)
	if err != nil {
		return err
	}
	m.migrateTemplateConfigMap(ctx, cm, secrets, opts, report)
	m.migrateConfigList(ctx, secrets, opts, report, seen)
	if unlisted != nil {
		report.add(unlistedNamespacesRow(unlisted))
	}
	return nil
}

// unlistedNamespacesRow is the one row a run adds when the cluster-wide Secret
// list was refused and only one namespace was covered. It is skipped, not failed:
// everything the caller could reach was handled, and a namespace-scoped user is
// not wrong to be namespace-scoped. The reason names the namespaces it could not
// see so an operator with wider rights knows to run it again.
func unlistedNamespacesRow(err error) MigrationResult {
	return MigrationResult{
		Kind: kindSecret, Namespace: "*", Name: "*", Outcome: MigrationSkipped,
		Reason: fmt.Sprintf("configs in other namespaces were not listed: %v", err),
	}
}

// listLegacyTemplates lists the legacy template ConfigMaps in ns, or everywhere
// for ns == "", in namespace then name order.
func (m *Migrator) listLegacyTemplates(ctx context.Context, ns string) ([]corev1.ConfigMap, error) {
	var list corev1.ConfigMapList
	if err := m.cli.List(ctx, &list, client.InNamespace(ns), client.MatchingLabels{types.LabelConfigCatalog: types.VelaCoreConfig}); err != nil {
		return nil, err
	}
	var cms []corev1.ConfigMap
	for _, cm := range list.Items {
		if strings.HasPrefix(cm.Name, TemplateConfigMapNamePrefix) {
			cms = append(cms, cm)
		}
	}
	sort.Slice(cms, func(i, j int) bool {
		if cms[i].Namespace != cms[j].Namespace {
			return cms[i].Namespace < cms[j].Namespace
		}
		return cms[i].Name < cms[j].Name
	})
	return cms, nil
}

// legacyConfigGroups is one listing of legacy config Secrets, grouped by the
// template they were made from. Template-less configs sit under the empty key.
type legacyConfigGroups struct {
	byTemplate map[NamespacedName][]corev1.Secret
	// unlisted is the error of a refused cluster-wide list, when the listing then
	// covered one namespace only.
	unlisted error
}

// listLegacyConfigGroups lists the legacy config Secrets across the cluster in one
// call. When the call is forbidden and fallbackNS is set, that namespace is listed
// instead and the refusal is kept in unlisted for the report. Any other error is
// returned: a timeout or a server error is not a visibility limit, and reporting it
// as one would let a transient failure end a run with exit 0.
func (m *Migrator) listLegacyConfigGroups(ctx context.Context, fallbackNS string) (legacyConfigGroups, error) {
	groups := legacyConfigGroups{byTemplate: map[NamespacedName][]corev1.Secret{}}
	selector := client.MatchingLabels{types.LabelConfigCatalog: types.VelaCoreConfig}
	var list corev1.SecretList
	if err := m.cli.List(ctx, &list, selector); err != nil {
		if fallbackNS == "" || !apierrors.IsForbidden(err) {
			return groups, err
		}
		groups.unlisted = err
		list = corev1.SecretList{}
		if err := m.cli.List(ctx, &list, selector, client.InNamespace(fallbackNS)); err != nil {
			return groups, err
		}
	}
	for i := range list.Items {
		secret := &list.Items[i]
		if !isLegacyConfig(secret) {
			continue
		}
		key := NamespacedName{Name: secret.Labels[types.LabelConfigType]}
		if key.Name != "" {
			key.Namespace = templateNamespaceOf(secret)
		}
		groups.byTemplate[key] = append(groups.byTemplate[key], *secret)
	}
	for key := range groups.byTemplate {
		sortSecrets(groups.byTemplate[key])
	}
	return groups, nil
}

// of returns the configs made from template name in templateNS, in namespace then
// name order.
func (g legacyConfigGroups) of(templateNS, name string) []corev1.Secret {
	return g.byTemplate[NamespacedName{Namespace: templateNS, Name: name}]
}

// remaining returns every listed config in ns, or everywhere for ns == "", in
// namespace then name order; the caller's seen set drops the ones already done.
func (g legacyConfigGroups) remaining(ns string) []corev1.Secret {
	var secrets []corev1.Secret
	for _, group := range g.byTemplate {
		for _, secret := range group {
			if ns == "" || secret.Namespace == ns {
				secrets = append(secrets, secret)
			}
		}
	}
	sortSecrets(secrets)
	return secrets
}

func (m *Migrator) migrateConfigList(ctx context.Context, secrets []corev1.Secret, opts MigrationOptions, report *MigrationReport, seen map[client.ObjectKey]bool) {
	for i := range secrets {
		key := client.ObjectKeyFromObject(&secrets[i])
		if seen[key] {
			continue
		}
		seen[key] = true
		m.migrateConfigSecret(ctx, &secrets[i], opts, report)
	}
}

// listConfigsOfTemplate finds the legacy configs made from template name in
// templateNS, across all namespaces. A Secret without a template-namespace
// annotation counts as made from vela-system, the pre-CRD default. When the
// cluster-wide list is forbidden the namespace of the template is listed instead
// and the refusal comes back as unlisted, so the caller can report that configs
// elsewhere may remain. Any other list error is returned as err: it is a failure
// of this run, not a limit of the caller's rights.
func (m *Migrator) listConfigsOfTemplate(ctx context.Context, templateNS, name string) (secrets []corev1.Secret, unlisted error, err error) {
	selector := client.MatchingLabels{types.LabelConfigCatalog: types.VelaCoreConfig, types.LabelConfigType: name}
	var list corev1.SecretList
	if err := m.cli.List(ctx, &list, selector); err != nil {
		if !apierrors.IsForbidden(err) {
			return nil, nil, err
		}
		unlisted = err
		list = corev1.SecretList{}
		if err := m.cli.List(ctx, &list, selector, client.InNamespace(templateNS)); err != nil {
			return nil, nil, err
		}
	}
	for _, secret := range list.Items {
		if !isLegacyConfig(&secret) || templateNamespaceOf(&secret) != templateNS {
			continue
		}
		secrets = append(secrets, secret)
	}
	sortSecrets(secrets)
	return secrets, unlisted, nil
}

// listLegacyConfigs lists the legacy config Secrets in ns.
func (m *Migrator) listLegacyConfigs(ctx context.Context, ns string) ([]corev1.Secret, error) {
	var list corev1.SecretList
	if err := m.cli.List(ctx, &list, client.InNamespace(ns), client.MatchingLabels{types.LabelConfigCatalog: types.VelaCoreConfig}); err != nil {
		return nil, err
	}
	var secrets []corev1.Secret
	for _, secret := range list.Items {
		if isLegacyConfig(&secret) {
			secrets = append(secrets, secret)
		}
	}
	sortSecrets(secrets)
	return secrets, nil
}

func sortSecrets(secrets []corev1.Secret) {
	sort.Slice(secrets, func(i, j int) bool {
		if secrets[i].Namespace != secrets[j].Namespace {
			return secrets[i].Namespace < secrets[j].Namespace
		}
		return secrets[i].Name < secrets[j].Name
	})
}

// isLegacyConfig tells a legacy config Secret, adopted or not, from two kinds of
// Secret that carry the same catalog label and are not for migration: the Secret a
// Config CR materialises for itself (owned by a Config, no migrated-to annotation),
// and a source cache entry, which the application controller declares with its own
// Config and owns itself; one caught between those two writes must be left to it.
func isLegacyConfig(secret *corev1.Secret) bool {
	if secret.Labels[types.LabelConfigCatalog] != types.VelaCoreConfig {
		return false
	}
	if sources.IsSourceCacheEntry(secret) {
		return false
	}
	if isConfigCRDOwned(secret) && secret.Annotations[types.AnnotationConfigMigratedTo] == "" {
		return false
	}
	return true
}

func templateNamespaceOf(secret *corev1.Secret) string {
	if ns := secret.Annotations[types.AnnotationConfigTemplateNamespace]; ns != "" {
		return ns
	}
	return types.DefaultKubeVelaNS
}

func optedOut(obj metav1.Object) bool {
	return obj.GetAnnotations()[types.AnnotationConfigMigrate] == migrateOptOutValue
}

// CreatedByMigration reports an object the migration library created: a Config or
// ConfigTemplate standing for a legacy object. The Config controller and the workflow
// step read it to tell such a Config from one a user wrote by hand.
func CreatedByMigration(obj metav1.Object) bool {
	return obj.GetLabels()[types.LabelConfigCreatedBy] == types.ConfigCreatedByMigration
}

func withMigrationLabel(labels map[string]string) map[string]string {
	if labels == nil {
		labels = map[string]string{}
	}
	labels[types.LabelConfigCreatedBy] = types.ConfigCreatedByMigration
	return labels
}

// migrateTemplateConfigMap adopts one legacy template ConfigMap into a
// ConfigTemplate CR of the same name in the same namespace.
//
// bound is the template's configs as listed for this run. The ones in other namespaces are
// recorded on the CR (AnnotationGrandfatheredConfigs): 1.11 let them use the template, and they
// keep doing so without the grant the access table now asks for. A template whose nacos block
// names an endpoint outside its namespace and vela-system is skipped: that endpoint was never
// reviewed for anyone, and the controller would read it with its own rights.
func (m *Migrator) migrateTemplateConfigMap(ctx context.Context, cm *corev1.ConfigMap, bound []corev1.Secret, opts MigrationOptions, report *MigrationReport) {
	name := strings.TrimPrefix(cm.Name, TemplateConfigMapNamePrefix)
	res := MigrationResult{Kind: kindConfigMap, Namespace: cm.Namespace, Name: name}
	note := gitOpsNote(cm, kindConfigMap, configv1alpha1.ConfigTemplateKind)
	finish := func(outcome MigrationOutcome, reason string) {
		if outcome == MigrationAdopted && note != "" {
			reason += "; " + note
		}
		res.Outcome, res.Reason = outcome, reason
		report.add(res)
		m.recordOutcome(opts, cm, res)
		if outcome == MigrationAdopted && note != "" {
			m.event(opts, res.Target, corev1.EventTypeNormal, EventMigratedGitOpsManaged, note)
		}
	}

	if optedOut(cm) {
		finish(MigrationSkipped, reasonOptedOut)
		return
	}

	existing := &configv1alpha1.ConfigTemplate{}
	switch err := m.cli.Get(ctx, client.ObjectKey{Namespace: cm.Namespace, Name: name}, existing); {
	case err == nil:
		if !CreatedByMigration(existing) {
			finish(MigrationConflict, fmt.Sprintf("ConfigTemplate %s exists and was not created by migration", name))
			return
		}
		res.Target = existing
		if cm.Annotations[types.AnnotationConfigMigratedTo] == string(existing.UID) {
			finish(MigrationSkipped, reasonAlreadyMigrated)
			return
		}
		// a CR from an earlier run whose ConfigMap annotation never landed
		if !opts.DryRun {
			if err := m.annotateMigratedTo(ctx, cm, existing.UID); err != nil {
				finish(MigrationFailed, err.Error())
				return
			}
		}
		finish(MigrationAdopted, reasonAdoptionCompleted)
		return
	case apierrors.IsNotFound(err):
	default:
		finish(MigrationFailed, err.Error())
		return
	}

	tmpl, err := convertConfigMap2Template(*cm)
	if err != nil {
		finish(MigrationFailed, err.Error())
		return
	}
	if nacos := expandedWriterOf(ctx, script.CUE(cm.Data[SaveTemplateKey])).Nacos; nacos != nil && !EndpointNamespaceFree(cm.Namespace, nacos.Endpoint.Namespace) {
		finish(MigrationSkipped, fmt.Sprintf("template %s/%s names the nacos endpoint %s/%s in another namespace, which migration cannot review for anyone; apply it with vela config-template apply as a user who may read that Secret",
			cm.Namespace, name, nacos.Endpoint.Namespace, nacos.Endpoint.Name))
		return
	}
	tmpl.Labels = withMigrationLabel(tmpl.Labels)
	spec := configTemplateSpecFromTemplate(tmpl)
	spec.Scope = LegacyTemplateScope(cm.Namespace) // by its namespace, not its label

	ct := &configv1alpha1.ConfigTemplate{
		TypeMeta:   metav1.TypeMeta{APIVersion: configv1alpha1.SchemeGroupVersion.String(), Kind: configv1alpha1.ConfigTemplateKind},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cm.Namespace},
		Spec:       spec,
	}
	applyTemplateLabels(ct, tmpl.Labels)
	if spec.Scope != configv1alpha1.ConfigTemplateScopeSystem {
		var grandfathered []string
		for _, secret := range bound {
			if secret.Namespace != cm.Namespace {
				grandfathered = append(grandfathered, secret.Namespace+"/"+secret.Name)
			}
		}
		if len(grandfathered) > 0 {
			ct.Annotations = map[string]string{AnnotationGrandfatheredConfigs: strings.Join(grandfathered, ",")}
		}
	}
	res.Target = ct
	reason := fmt.Sprintf("ConfigTemplate %s created", name)
	if opts.DryRun {
		finish(MigrationAdopted, reason)
		return
	}
	if err := m.cli.Create(ctx, ct, migrationFieldOwner); err != nil {
		finish(MigrationFailed, err.Error())
		return
	}
	if err := m.annotateMigratedTo(ctx, cm, ct.UID); err != nil {
		finish(MigrationFailed, fmt.Sprintf("ConfigTemplate %s created but the ConfigMap could not be annotated: %v", name, err))
		return
	}
	finish(MigrationAdopted, reason)
}

// migrateConfigSecret adopts one legacy config Secret into a Config CR of the same
// name in the same namespace, which then owns the Secret, its output objects and
// its distribution Application.
func (m *Migrator) migrateConfigSecret(ctx context.Context, secret *corev1.Secret, opts MigrationOptions, report *MigrationReport) {
	res := MigrationResult{Kind: kindSecret, Namespace: secret.Namespace, Name: secret.Name}
	note := gitOpsNote(secret, kindSecret, configv1alpha1.ConfigKind)
	finish := func(outcome MigrationOutcome, reason string) {
		if outcome == MigrationAdopted && note != "" {
			reason += "; " + note
		}
		res.Outcome, res.Reason = outcome, reason
		report.add(res)
		m.recordOutcome(opts, secret, res)
		if outcome == MigrationAdopted && note != "" {
			m.event(opts, res.Target, corev1.EventTypeNormal, EventMigratedGitOpsManaged, note)
		}
	}

	// What is not a legacy config is settled before any Config lookup: the Secret a
	// Config CR wrote for itself has a Config of its name, and that is no conflict.
	if optedOut(secret) {
		finish(MigrationSkipped, reasonOptedOut)
		return
	}
	if sources.IsSourceCacheEntry(secret) {
		finish(MigrationSkipped, reasonSourceCacheEntry)
		return
	}
	if writer := configWriterOf(secret); writer != "" {
		finish(MigrationSkipped, fmt.Sprintf("not a legacy config: written by Config %s", writer))
		return
	}

	existing := &configv1alpha1.Config{}
	switch err := m.cli.Get(ctx, client.ObjectKeyFromObject(secret), existing); {
	case err == nil:
		if !CreatedByMigration(existing) {
			finish(MigrationConflict, fmt.Sprintf("Config %s exists and was not created by migration", secret.Name))
			return
		}
		res.Target = existing
		if secret.Annotations[types.AnnotationConfigMigratedTo] == string(existing.UID) && metav1.IsControlledBy(secret, existing) {
			finish(MigrationSkipped, reasonAlreadyMigrated)
			return
		}
		// a Config from an earlier run whose owner patches never finished
		if !opts.DryRun {
			refs, err := ObjectReferences(secret)
			if err != nil {
				finish(MigrationFailed, err.Error())
				return
			}
			if err := m.adopt(ctx, existing, secret, refs); err != nil {
				finish(MigrationFailed, err.Error())
				return
			}
		}
		finish(MigrationAdopted, reasonAdoptionCompleted)
		return
	case apierrors.IsNotFound(err):
	default:
		finish(MigrationFailed, err.Error())
		return
	}

	refs, err := ObjectReferences(secret)
	if err != nil {
		finish(MigrationFailed, err.Error())
		return
	}
	if outside := outputsOutside(refs, secret.Namespace); len(outside) > 0 {
		finish(MigrationConflict, fmt.Sprintf("%d output object(s) outside %s would be recreated there by the controller: %s",
			len(outside), secret.Namespace, strings.Join(outside, ", ")))
		return
	}
	cfg, propsSecret, block, err := m.buildConfig(ctx, secret)
	if err != nil {
		finish(MigrationFailed, err.Error())
		return
	}
	if block != nil {
		finish(block.outcome, block.reason)
		return
	}
	res.Target = cfg
	if propsSecret != nil {
		foreign, err := m.propertiesSecretForeign(ctx, propsSecret)
		if err != nil {
			finish(MigrationFailed, fmt.Sprintf("properties Secret %s: %v", propsSecret.Name, err))
			return
		}
		if foreign {
			finish(MigrationConflict, fmt.Sprintf("Secret %s exists and was not created by migration", propsSecret.Name))
			return
		}
	}
	reason := fmt.Sprintf("Config %s created", secret.Name)
	if opts.DryRun {
		finish(MigrationAdopted, reason)
		return
	}
	if propsSecret != nil {
		if err := m.cli.Create(ctx, propsSecret, migrationFieldOwner); err != nil && !apierrors.IsAlreadyExists(err) {
			finish(MigrationFailed, fmt.Sprintf("properties Secret %s: %v", propsSecret.Name, err))
			return
		}
	}
	// The controller adopts the Secret, and the outputs it records, for the Config named by
	// this mark as soon as it sees the Config; the owner reference set in adopt needs the
	// Config's UID, which exists only after the Create. A reconcile between the two used to
	// find the Secret unowned and report Error until the next one. The mark takes write
	// access to the Secret, which a Config author alone does not have, so a hand-made Config
	// cannot claim a Secret by name.
	if err := m.markAdoptBy(ctx, secret, cfg.Name); err != nil {
		finish(MigrationFailed, fmt.Sprintf("Secret %s: %v", secret.Name, err))
		return
	}
	if err := m.createConfig(ctx, cfg); err != nil {
		// A mark with no Config behind it would let a later hand-made Config of the name
		// collect the Secret; take it back, and say so if that fails too.
		reason := err.Error()
		if uerr := m.unmarkAdoptBy(ctx, secret); uerr != nil {
			reason = fmt.Sprintf("%s (and the adopt-by mark on the Secret could not be removed: %v)", reason, uerr)
		}
		finish(MigrationFailed, reason)
		return
	}
	if err := m.adopt(ctx, cfg, secret, refs); err != nil {
		finish(MigrationFailed, fmt.Sprintf("Config %s created but adoption did not finish: %v", secret.Name, err))
		return
	}
	finish(MigrationAdopted, reason)
}

// pendingBackoff paces createConfig's retries: about forty seconds in all, long enough for
// the ConfigTemplate controller to reconcile a template this run has just created.
var pendingBackoff = wait.Backoff{Duration: time.Second, Factor: 1.6, Steps: 8}

// createConfig creates cfg, retrying while the Config webhook answers that its template is not
// available yet: a run creates a template's CR, then its configs, and one in another namespace
// is admitted only once the controller has made that CR usable (PendingTemplateError).
func (m *Migrator) createConfig(ctx context.Context, cfg *configv1alpha1.Config) error {
	var err error
	backoff := pendingBackoff
	for {
		if err = m.cli.Create(ctx, cfg, migrationFieldOwner); err == nil || !strings.Contains(err.Error(), pendingTemplatePhrase) || backoff.Steps <= 1 {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(backoff.Step()):
		}
	}
}

// configWriterOf returns the name of the Config that wrote secret for itself: a
// controller owner of kind Config with no migrated-to annotation on the Secret,
// which an adopted legacy Secret always carries. "" means a legacy config.
func configWriterOf(secret *corev1.Secret) string {
	if secret.Annotations[types.AnnotationConfigMigratedTo] != "" {
		return ""
	}
	for _, ref := range secret.OwnerReferences {
		if IsConfigOwnerRef(ref) {
			return ref.Name
		}
	}
	return ""
}

// IsConfigOwnerRef reports an owner reference to a config.oam.dev Config, the shape
// adoption writes on a legacy Secret and its outputs.
func IsConfigOwnerRef(ref metav1.OwnerReference) bool {
	return ref.Kind == configv1alpha1.ConfigKind && ref.APIVersion == configv1alpha1.SchemeGroupVersion.String()
}

// templateBlock says a config cannot be adopted in this run because of the state
// of its template; the row takes this outcome and reason instead of a Config.
type templateBlock struct {
	outcome MigrationOutcome
	reason  string
}

// buildConfig maps a legacy config Secret onto the Config CR that adopts it. The
// input goes into a companion Secret the Config references when the template or
// the Secret's own annotation says sensitive; it never enters the CR then. The
// template must exist as a CR or a legacy ConfigMap, otherwise the Config could
// never become Available, and it must not block (see templateLookup.block).
func (m *Migrator) buildConfig(ctx context.Context, secret *corev1.Secret) (*configv1alpha1.Config, *corev1.Secret, *templateBlock, error) {
	legacy, err := convertSecret2Config(secret)
	if err != nil {
		return nil, nil, nil, err
	}
	cfg := &configv1alpha1.Config{
		TypeMeta: metav1.TypeMeta{APIVersion: configv1alpha1.SchemeGroupVersion.String(), Kind: configv1alpha1.ConfigKind},
		ObjectMeta: metav1.ObjectMeta{
			Name: secret.Name, Namespace: secret.Namespace,
			Labels: withMigrationLabel(nil),
		},
		Spec: configv1alpha1.ConfigSpec{Alias: legacy.Alias, Description: legacy.Description},
	}
	// the owner's leave to serve as a Nacos endpoint moves to the Config, which the
	// controller carries back to the Secret
	if NacosEndpointMarked(secret) {
		cfg.Annotations = map[string]string{types.AnnotationConfigNacosEndpoint: NacosEndpointAnnotationValue}
	}
	raw := secret.Data[SaveInputPropertiesKey]
	if string(raw) == "null" {
		raw = nil
	}
	templateName := secret.Labels[types.LabelConfigType]
	sensitive := SecretSensitive(secret)
	if templateName != "" {
		templateNS := templateNamespaceOf(secret)
		cfg.Spec.TemplateRef = &configv1alpha1.ConfigTemplateReference{Name: templateName, Namespace: templateNS}
		tpl, err := m.lookupTemplate(ctx, templateNS, templateName)
		if err != nil {
			return nil, nil, nil, err
		}
		if tpl.cr == nil && tpl.legacy == nil {
			return nil, nil, nil, fmt.Errorf("template %s/%s not found", templateNS, templateName)
		}
		if block := tpl.block(templateNS, templateName); block != nil {
			return nil, nil, block, nil
		}
		if block, err := m.accessBlock(ctx, tpl.scope(templateName), secret); block != nil || err != nil {
			return nil, nil, block, err
		}
		sensitive = sensitive || tpl.sensitive()
	}
	if len(raw) == 0 {
		return cfg, nil, nil, nil
	}
	if !sensitive {
		cfg.Spec.Properties = &runtime.RawExtension{Raw: raw}
		return cfg, nil, nil, nil
	}
	propsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secret.Name + propertiesSecretSuffix, Namespace: secret.Namespace,
			Labels:      withMigrationLabel(nil),
			Annotations: map[string]string{types.AnnotationConfigPropertiesOf: secret.Name},
		},
		Data: map[string][]byte{DefaultPropertiesSecretKey: raw},
	}
	cfg.Spec.PropertiesFrom = &configv1alpha1.PropertiesReference{
		SecretRef: configv1alpha1.SecretKeySelector{Name: propsSecret.Name},
	}
	return cfg, propsSecret, nil, nil
}

// NacosEndpointMarked reports whether a Config CR or a config Secret carries the owner's
// leave to serve as a Nacos endpoint while sensitive.
func NacosEndpointMarked(obj metav1.Object) bool {
	return obj.GetAnnotations()[types.AnnotationConfigNacosEndpoint] == NacosEndpointAnnotationValue
}

// SecretSensitive reports whether a config Secret is marked sensitive. For a legacy
// Secret it is the Secret's own word that it was made from a sensitive template; the
// template may have been flipped since, and the Secret's word keeps its input out of
// the CR either way.
func SecretSensitive(secret *corev1.Secret) bool {
	return secret.Annotations[types.AnnotationConfigSensitive] == SensitiveAnnotationValue
}

// propertiesSecretForeign reports whether a Secret already holds the companion
// name and is not the one an earlier, interrupted migration run wrote: only a
// Secret carrying the migration label with the same properties data is reused.
// Anything else would make the Config render from data that is not the legacy
// input, so it is a conflict.
func (m *Migrator) propertiesSecretForeign(ctx context.Context, want *corev1.Secret) (bool, error) {
	var existing corev1.Secret
	switch err := m.cli.Get(ctx, client.ObjectKeyFromObject(want), &existing); {
	case apierrors.IsNotFound(err):
		return false, nil
	case err != nil:
		return false, err
	}
	return !CreatedByMigration(&existing) || !bytes.Equal(existing.Data[DefaultPropertiesSecretKey], want.Data[DefaultPropertiesSecretKey]), nil
}

// templateLookup is a config's template as it exists now: a CR, a legacy
// ConfigMap, both, or neither.
type templateLookup struct {
	cr     *configv1alpha1.ConfigTemplate
	legacy *corev1.ConfigMap
}

// scope is the template as AccessToTemplate reads it: the CR when there is one, else the
// legacy ConfigMap (callers have checked that one of them exists).
func (t templateLookup) scope(name string) *ResolvedTemplate {
	if t.cr != nil {
		return templateOfCR(t.cr)
	}
	return templateOfConfigMap(name, t.legacy)
}

// accessBlock is the access table of issue #7410 section 6 for the Config migration would
// write, used from the legacy Secret's namespace. A binding the template recorded when it was
// adopted needs no review; nor does one in the template's namespace or to a system template.
// Otherwise the run needs a user to ask: the create-config step adopting as the Application's
// user is reviewed for that user; the startup sweep and a run with no user skip it, since the
// template namespace on a legacy Secret is only what its writer said it was.
func (m *Migrator) accessBlock(ctx context.Context, tmpl *ResolvedTemplate, secret *corev1.Secret) (*templateBlock, error) {
	access := AccessToTemplate(tmpl, secret.Namespace)
	if access.Review == nil || tmpl.Grandfathers(client.ObjectKeyFromObject(secret)) {
		return nil, nil
	}
	user, named := auth.NamedUserFrom(ctx)
	if !named {
		return &templateBlock{MigrationSkipped, access.Reason + "; the binding was not recorded when the template was migrated, and this run has no user to ask"}, nil
	}
	allowed, err := auth.UserCan(auth.ContextClearUserInfo(ctx), m.cli, auth.UserInfoOf(user), *access.Review)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return &templateBlock{MigrationSkipped, fmt.Sprintf("%s; %s may not", access.Reason, user.GetName())}, nil
	}
	return nil, nil
}

// lookupTemplate fetches both forms of the template name in ns, with the controller's own
// identity, as the Config controller reads a template to render. Who may bind a config to
// it is accessBlock's question, asked for the run's user.
func (m *Migrator) lookupTemplate(ctx context.Context, ns, name string) (templateLookup, error) {
	ctx = auth.ContextClearUserInfo(ctx)
	var tpl templateLookup
	var ct configv1alpha1.ConfigTemplate
	switch err := m.cli.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &ct); {
	case err == nil:
		tpl.cr = &ct
	case apierrors.IsNotFound(err) || IsCRDTypeMissing(err):
	default:
		return tpl, err
	}
	var cm corev1.ConfigMap
	switch err := m.cli.Get(ctx, client.ObjectKey{Namespace: ns, Name: TemplateConfigMapNamePrefix + name}, &cm); {
	case err == nil:
		tpl.legacy = &cm
	case apierrors.IsNotFound(err):
	default:
		return tpl, err
	}
	return tpl, nil
}

// block reports why the template keeps its configs from being adopted in this run.
// A CR that migration did not create, while the legacy ConfigMap is still there,
// is a conflict: the configs were made from the ConfigMap, and binding them to a
// foreign CR would have the controller re-render their Secrets from its CUE. The
// ConfigMap's own migrated-to annotation counts as proof of adoption too, so a CR
// that lost the migration label (relabelled by hand or by a controller) still
// stands for its ConfigMap. A legacy ConfigMap that opted out keeps its configs
// legacy with it. A CR alone means the operator removed the ConfigMap and wants
// the rebinding; a ConfigMap alone is the config-first path, where the template
// follows later.
func (t templateLookup) block(ns, name string) *templateBlock {
	if t.cr != nil && t.legacy != nil && !CreatedByMigration(t.cr) && !t.adoptedInto() {
		return &templateBlock{MigrationConflict, fmt.Sprintf("template %s/%s: ConfigTemplate exists and was not created by migration", ns, name)}
	}
	if t.cr == nil && t.legacy != nil && optedOut(t.legacy) {
		return &templateBlock{MigrationSkipped, fmt.Sprintf("template %s/%s opted out", ns, name)}
	}
	return nil
}

// adoptedInto reports whether the legacy ConfigMap records the CR as the one that
// adopted it.
func (t templateLookup) adoptedInto() bool {
	return t.cr != nil && t.legacy != nil && t.legacy.Annotations[types.AnnotationConfigMigratedTo] == string(t.cr.UID)
}

// sensitive is the template's own flag, the CR's when there is one.
func (t templateLookup) sensitive() bool {
	if t.cr != nil {
		return t.cr.Spec.Sensitive
	}
	return t.legacy != nil && t.legacy.Annotations[types.AnnotationConfigSensitive] == SensitiveAnnotationValue
}

// ObjectReferences lists the output objects a config Secret records under
// SaveObjectReferenceKey; nil when it records none.
func ObjectReferences(secret *corev1.Secret) ([]corev1.ObjectReference, error) {
	content, ok := secret.Data[SaveObjectReferenceKey]
	if !ok || len(content) == 0 {
		return nil, nil
	}
	var refs []corev1.ObjectReference
	if err := json.Unmarshal(content, &refs); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", SaveObjectReferenceKey, err)
	}
	return refs, nil
}

// outputsOutside names the output objects that do not live in the Secret's
// namespace, cluster-scoped ones included. The Config controller renders every
// output into the Config's namespace, so adopting such a config would make it
// recreate the object there, repoint objects-reference at the copy and orphan the
// original. Migration reports these as a conflict for the operator to settle.
func outputsOutside(refs []corev1.ObjectReference, namespace string) []string {
	var names []string
	for _, ref := range refs {
		if ref.Namespace == namespace {
			continue
		}
		names = append(names, fmt.Sprintf("%s %s/%s", ref.Kind, ref.Namespace, ref.Name))
	}
	return names
}

// adopt makes cfg the controller owner of everything the legacy config left
// behind, patching in this order: output objects, the distribution Application,
// the companion properties Secret, and the config Secret last. Outputs outside the
// namespace are a conflict before this point; the guard here is for a Config left
// by an earlier run that still records one, which is never owned. The
// Config reconciler refuses output objects it does not own and only a Secret
// update re-enqueues it, so the Secret patch has to be the final trigger. The
// Secret also receives the migrated-to annotation. Every step is idempotent, so a
// resumed run finishes whatever an earlier one left.
func (m *Migrator) adopt(ctx context.Context, cfg *configv1alpha1.Config, secret *corev1.Secret, refs []corev1.ObjectReference) error {
	for _, ref := range refs {
		if ref.Namespace != secret.Namespace {
			continue
		}
		obj := convertObjectReference2Unstructured(ref)
		if err := m.own(ctx, cfg, obj, false); err != nil {
			if apierrors.IsNotFound(err) || IsCRDTypeMissing(err) {
				continue
			}
			return fmt.Errorf("output object %s %s: %w", ref.Kind, ref.Name, err)
		}
	}
	distribution := DefaultDistributionName(cfg.Name)
	switch err := m.cli.Get(ctx, client.ObjectKey{Namespace: cfg.Namespace, Name: distribution}, &v1beta1.Application{}); {
	case err == nil:
		if err := SetDistributionOwner(ctx, m.cli, cfg.Namespace, cfg.Name, distribution, migrationFieldOwner); err != nil {
			return err
		}
	case apierrors.IsNotFound(err):
	default:
		return err
	}
	if cfg.Spec.PropertiesFrom != nil {
		props := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: cfg.Namespace, Name: cfg.Spec.PropertiesFrom.SecretRef.Name}}
		if err := m.own(ctx, cfg, props, false); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("properties Secret %s: %w", props.Name, err)
		}
	}
	return m.own(ctx, cfg, secret, true)
}

// own reloads obj and patches cfg in as its controller owner, plus the migrated-to
// annotation when asked. Nothing else on the object changes.
func (m *Migrator) own(ctx context.Context, cfg *configv1alpha1.Config, obj client.Object, annotate bool) error {
	if err := m.cli.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		return err
	}
	orig := obj.DeepCopyObject().(client.Object)
	changed := false
	if !metav1.IsControlledBy(obj, cfg) {
		dropStaleConfigOwner(obj, cfg)
		if err := controllerutil.SetControllerReference(cfg, obj, m.cli.Scheme()); err != nil {
			return err
		}
		changed = true
	}
	if annotate && obj.GetAnnotations()[types.AnnotationConfigMigratedTo] != string(cfg.UID) {
		setAnnotation(obj, types.AnnotationConfigMigratedTo, string(cfg.UID))
		changed = true
	}
	if annotations := obj.GetAnnotations(); annotations[types.AnnotationConfigAdoptBy] != "" {
		delete(annotations, types.AnnotationConfigAdoptBy)
		obj.SetAnnotations(annotations)
		changed = true
	}
	if dropApplicationMarks(obj) {
		changed = true
	}
	if !changed {
		return nil
	}
	return m.cli.Patch(ctx, obj, client.MergeFrom(orig), migrationFieldOwner)
}

// markAdoptBy writes the mark the Config controller adopts the Secret on (see
// migrateConfigSecret).
func (m *Migrator) markAdoptBy(ctx context.Context, secret *corev1.Secret, configName string) error {
	orig := secret.DeepCopy()
	setAnnotation(secret, types.AnnotationConfigAdoptBy, configName)
	return m.cli.Patch(ctx, secret, client.MergeFrom(orig), migrationFieldOwner)
}

// unmarkAdoptBy takes the mark back when no Config followed it.
func (m *Migrator) unmarkAdoptBy(ctx context.Context, secret *corev1.Secret) error {
	if err := m.cli.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
		return client.IgnoreNotFound(err)
	}
	if secret.Annotations[types.AnnotationConfigAdoptBy] == "" {
		return nil
	}
	orig := secret.DeepCopy()
	delete(secret.Annotations, types.AnnotationConfigAdoptBy)
	return m.cli.Patch(ctx, secret, client.MergeFrom(orig), migrationFieldOwner)
}

// applicationMarkKeys are the labels an Application's dispatcher leaves on what it applies:
// the app pair the generator merges in and the owner triple the resource keeper stamps.
var applicationMarkKeys = []string{oam.LabelAppName, oam.LabelAppNamespace, oam.LabelOwnerKind, oam.LabelOwnerName, oam.LabelOwnerNamespace}

// dropApplicationMarks releases an adopted object from the Application that dispatched it,
// if one did (a config a workflow step wrote on an older controller). The Config owns it
// now; left on, the marks would have the Application's garbage collection delete it once
// the run that dispatched it is retired, and state-keep re-apply the stale copy. Reports
// whether anything changed.
func dropApplicationMarks(obj metav1.Object) bool {
	labels := obj.GetLabels()
	changed := false
	for _, key := range applicationMarkKeys {
		if _, ok := labels[key]; ok {
			delete(labels, key)
			changed = true
		}
	}
	if changed {
		obj.SetLabels(labels)
	}
	return changed
}

// dropStaleConfigOwner removes a controller reference to a Config of the same
// name as cfg but another UID: the Config was deleted and recreated while the
// legacy object outlived it, so the new one takes over.
func dropStaleConfigOwner(obj metav1.Object, cfg *configv1alpha1.Config) {
	refs := obj.GetOwnerReferences()
	kept := refs[:0]
	for _, ref := range refs {
		stale := IsConfigOwnerRef(ref) && ref.Name == cfg.Name && ref.UID != cfg.UID
		if !stale {
			kept = append(kept, ref)
		}
	}
	obj.SetOwnerReferences(kept)
}

func (m *Migrator) annotateMigratedTo(ctx context.Context, obj client.Object, uid k8stypes.UID) error {
	orig := obj.DeepCopyObject().(client.Object)
	setAnnotation(obj, types.AnnotationConfigMigratedTo, string(uid))
	return m.cli.Patch(ctx, obj, client.MergeFrom(orig), migrationFieldOwner)
}

func setAnnotation(obj metav1.Object, key, value string) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[key] = value
	obj.SetAnnotations(annotations)
}

// recordOutcome turns a result into an Event on the legacy object. Rerunning a
// migration is routine, so "already migrated" records nothing.
func (m *Migrator) recordOutcome(opts MigrationOptions, obj runtime.Object, res MigrationResult) {
	switch res.Outcome {
	case MigrationAdopted:
		m.event(opts, obj, corev1.EventTypeNormal, EventMigrationAdopted, res.Reason)
	case MigrationSkipped:
		if res.Reason != reasonAlreadyMigrated {
			m.event(opts, obj, corev1.EventTypeNormal, EventMigrationSkipped, res.Reason)
		}
	case MigrationConflict:
		m.event(opts, obj, corev1.EventTypeWarning, EventMigrationConflict, res.Reason)
	case MigrationFailed:
		m.event(opts, obj, corev1.EventTypeWarning, EventMigrationFailed, res.Reason)
	}
}

func (m *Migrator) event(opts MigrationOptions, obj runtime.Object, eventType, reason, message string) {
	if opts.Recorder == nil || opts.DryRun {
		return
	}
	opts.Recorder.Event(obj, eventType, reason, message)
}
