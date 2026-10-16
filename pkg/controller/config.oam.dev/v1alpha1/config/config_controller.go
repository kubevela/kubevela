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
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrlHandler "sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	legacyconfig "github.com/oam-dev/kubevela/pkg/config"
	icontext "github.com/oam-dev/kubevela/pkg/config/context"
	oamctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// Reconciler reconciles a Config object.
type Reconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	record               event.Recorder
	concurrentReconciles int
	applicator           apply.Applicator
}

// Reconcile is the main logic for the Config controller.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	klog.InfoS("Reconciling Config", "name", req.Name, "namespace", req.Namespace)

	var cfg configv1alpha1.Config
	if err := r.Get(ctx, req.NamespacedName, &cfg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// every status write below describes this spec
	cfg.Status.ObservedGeneration = cfg.Generation

	// the materialized Secret is owned by the Config, so no finalizer is needed
	if cfg.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	if cfg.Spec.Properties != nil && cfg.Spec.PropertiesFrom != nil {
		return r.markError(ctx, &cfg, legacyconfig.ErrMutuallyExclusiveProperties)
	}

	var tmpl *legacyconfig.ResolvedTemplate
	if cfg.Spec.TemplateRef != nil {
		resolved, waiting, err := r.resolveTemplate(ctx, cfg.Spec.TemplateRef)
		if err != nil {
			return r.markError(ctx, &cfg, err)
		}
		if waiting {
			return r.markError(ctx, &cfg, fmt.Errorf("config template %s/%s is not Available yet", cfg.Spec.TemplateRef.Namespace, cfg.Spec.TemplateRef.Name))
		}
		tmpl = resolved
	}

	// defense in depth: the validating webhook already blocks this, but a Config applied
	// before its sensitive template first becomes Available can slip past admission.
	if tmpl != nil && tmpl.Sensitive && cfg.Spec.Properties != nil {
		return r.markError(ctx, &cfg, fmt.Errorf("config template %s/%s is sensitive: spec.properties must not be set, use spec.propertiesFrom instead", tmpl.Namespace, tmpl.Name))
	}

	// the webhook's check of the template's nacos block, repeated here for a template
	// that reached the cluster with the webhooks off
	if err := validateExpandedWriter(ctx, tmpl); err != nil {
		return r.markError(ctx, &cfg, err)
	}

	props, err := r.resolveProperties(ctx, &cfg)
	if err != nil {
		return r.markError(ctx, &cfg, err)
	}

	// one compile serves the Secret, the outputs and the nacos block
	rendered, err := renderTemplate(ctx, &cfg, tmpl, props)
	if err != nil {
		return r.markError(ctx, &cfg, err)
	}
	secret, outputs, err := r.renderSecret(&cfg, tmpl, rendered, props)
	if err != nil {
		return r.markError(ctx, &cfg, err)
	}

	nacosData, err := renderExpandedWriter(tmpl, rendered)
	if err != nil {
		return r.markError(ctx, &cfg, err)
	}

	if err := controllerutil.SetControllerReference(&cfg, secret, r.Scheme); err != nil {
		return r.markError(ctx, &cfg, err)
	}

	legacyOutputs, err := r.applySecret(ctx, &cfg, secret)
	if err != nil {
		return r.markError(ctx, &cfg, err)
	}

	if err := r.applyOutputs(ctx, &cfg, outputs, legacyOutputs); err != nil {
		return r.markError(ctx, &cfg, err)
	}

	cfg.Status.Phase = configv1alpha1.ConfigPhaseAvailable
	cfg.Status.SecretRef = &corev1.LocalObjectReference{Name: secret.Name}
	cfg.Status.SetConditions(condition.ReconcileSuccess())

	// The Secret and the outputs are in place, so the phase is Available whatever Nacos
	// says; a Nacos failure lives on the NacosSynced condition and is retried with the
	// controller's backoff.
	if err := r.syncNacos(ctx, &cfg, nacosData); err != nil {
		if uerr := r.UpdateStatus(ctx, &cfg); uerr != nil {
			return ctrl.Result{}, uerr
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, r.UpdateStatus(ctx, &cfg)
}

// resolveTemplate looks up the referenced template, CRD first, falling back to a
// legacy ConfigMap. waiting is true if the ConfigTemplate CRD exists but its schema
// hasn't reconciled yet.
func (r *Reconciler) resolveTemplate(ctx context.Context, ref *configv1alpha1.ConfigTemplateReference) (*legacyconfig.ResolvedTemplate, bool, error) {
	return legacyconfig.ResolveConfigTemplate(ctx, r.Client, ref)
}

func (r *Reconciler) resolveProperties(ctx context.Context, cfg *configv1alpha1.Config) (map[string]interface{}, error) {
	props := map[string]interface{}{}
	switch {
	case cfg.Spec.Properties != nil:
		if len(cfg.Spec.Properties.Raw) > 0 {
			if err := json.Unmarshal(cfg.Spec.Properties.Raw, &props); err != nil {
				return nil, fmt.Errorf("failed to decode spec.properties: %w", err)
			}
		}
	case cfg.Spec.PropertiesFrom != nil:
		key := cfg.Spec.PropertiesFrom.SecretRef.Key
		if key == "" {
			key = legacyconfig.DefaultPropertiesSecretKey
		}
		var secret corev1.Secret
		if err := r.Get(ctx, client.ObjectKey{Namespace: cfg.Namespace, Name: cfg.Spec.PropertiesFrom.SecretRef.Name}, &secret); err != nil {
			return nil, fmt.Errorf("failed to load spec.propertiesFrom secret: %w", err)
		}
		raw, ok := secret.Data[key]
		if !ok {
			return nil, fmt.Errorf("secret %s/%s has no key %q", secret.Namespace, secret.Name, key)
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &props); err != nil {
				return nil, fmt.Errorf("failed to decode properties from secret key %s: %w", key, err)
			}
		}
	}
	return props, nil
}

// renderTemplate compiles the Config's template once, for the Secret, the outputs and
// the nacos block (legacyconfig.RenderTemplate holds the one rule on partial renders).
// A Config without a template has nothing to render.
func renderTemplate(ctx context.Context, cfg *configv1alpha1.Config, tmpl *legacyconfig.ResolvedTemplate, props map[string]interface{}) (cue.Value, error) {
	if tmpl == nil {
		return cue.Value{}, nil
	}
	contextValue := icontext.ConfigRenderContext{Name: cfg.Name, Namespace: cfg.Namespace}
	val, err := legacyconfig.RenderTemplate(ctx, tmpl.CUE, tmpl.ExpandedWriter, contextValue, props)
	if err != nil {
		return cue.Value{}, fmt.Errorf("failed to render config template: %w", err)
	}
	return val, nil
}

// renderSecret builds the Secret and the outputs from the rendered template value.
func (r *Reconciler) renderSecret(cfg *configv1alpha1.Config, tmpl *legacyconfig.ResolvedTemplate, val cue.Value, props map[string]interface{}) (*corev1.Secret, []*unstructured.Unstructured, error) {
	secret := &corev1.Secret{}
	var outputs []*unstructured.Unstructured

	if tmpl != nil {
		validReturns := val.LookupPath(cue.ParsePath(legacyconfig.TemplateValidationReturns))
		if validReturns.Exists() {
			var validation legacyconfig.Validation
			if err := validReturns.Decode(&validation); err != nil {
				return nil, nil, fmt.Errorf("template.validation.$returns format must be a validation object: %w", err)
			}
			if len(validation.Message) > 0 {
				return nil, nil, &validation
			}
		}

		output := val.LookupPath(cue.ParsePath(legacyconfig.TemplateOutput))
		if output.Exists() {
			if err := output.Decode(secret); err != nil {
				return nil, nil, fmt.Errorf("template.output format must be a secret: %w", err)
			}
		}
		if secret.Type == "" {
			// matches the legacy Factory.ParseConfig fallback
			secret.Type = corev1.SecretType(fmt.Sprintf("%s/%s", "", tmpl.Name))
		}

		var err error
		outputs, err = renderOutputs(val, cfg.Namespace)
		if err != nil {
			return nil, nil, err
		}

		if secret.Labels == nil {
			secret.Labels = map[string]string{}
		}
		secret.Labels[apitypes.LabelConfigCatalog] = apitypes.VelaCoreConfig
		secret.Labels[apitypes.LabelConfigType] = tmpl.Name
		secret.Labels[apitypes.LabelConfigScope] = tmpl.Scope

		if secret.Annotations == nil {
			secret.Annotations = map[string]string{}
		}
		secret.Annotations[apitypes.AnnotationConfigSensitive] = fmt.Sprintf("%t", tmpl.Sensitive)
		secret.Annotations[apitypes.AnnotationConfigTemplateNamespace] = tmpl.Namespace
	} else {
		secret.Labels = map[string]string{
			apitypes.LabelConfigCatalog: apitypes.VelaCoreConfig,
		}
		secret.Annotations = map[string]string{}
	}

	if secret.Name == "" {
		secret.Name = cfg.Name
	}
	secret.Namespace = cfg.Namespace
	secret.Annotations[apitypes.AnnotationConfigAlias] = cfg.Spec.Alias
	secret.Annotations[apitypes.AnnotationConfigDescription] = cfg.Spec.Description

	propsJSON, err := json.Marshal(props)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode properties: %w", err)
	}
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	// matches the key the legacy Factory.ReadConfig/ListConfigs read
	secret.Data[legacyconfig.SaveInputPropertiesKey] = propsJSON

	if len(outputs) > 0 {
		refs := make([]corev1.ObjectReference, 0, len(outputs))
		for _, obj := range outputs {
			refs = append(refs, corev1.ObjectReference{
				Kind:       obj.GetKind(),
				Namespace:  obj.GetNamespace(),
				Name:       obj.GetName(),
				APIVersion: obj.GetAPIVersion(),
			})
		}
		refsJSON, err := json.Marshal(refs)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to encode object references: %w", err)
		}
		// matches the key the legacy Factory.ParseConfig writes, so tooling that
		// discovers a config's companion objects through the Secret works for both.
		secret.Data[legacyconfig.SaveObjectReferenceKey] = refsJSON
	}

	return secret, outputs, nil
}

// renderOutputs decodes template.outputs (name -> arbitrary object) into unstructured
// objects, forcing each into the Config's own namespace to prevent cross-namespace writes.
func renderOutputs(val cue.Value, namespace string) ([]*unstructured.Unstructured, error) {
	outputsVal := val.LookupPath(cue.ParsePath(legacyconfig.TemplateOutputs))
	if !outputsVal.Exists() {
		return nil, nil
	}
	var objects map[string]interface{}
	if err := outputsVal.Decode(&objects); err != nil {
		return nil, fmt.Errorf("template.outputs format must be a map of objects: %w", err)
	}

	names := make([]string, 0, len(objects))
	for name := range objects {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]*unstructured.Unstructured, 0, len(names))
	for _, name := range names {
		obj, ok := objects[name].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("template.outputs.%s must be an object", name)
		}
		u := &unstructured.Unstructured{Object: obj}
		if u.GetAPIVersion() == "" || u.GetKind() == "" {
			return nil, fmt.Errorf("template.outputs.%s must set apiVersion and kind", name)
		}
		if u.GetName() == "" {
			return nil, fmt.Errorf("template.outputs.%s must set metadata.name", name)
		}
		u.SetNamespace(namespace)
		result = append(result, u)
	}
	return result, nil
}

// applySecret creates or updates the materialized output Secret. It refuses to
// adopt a pre-existing Secret not already controlled by this Config, so a user who
// can only create Configs can't use the manager's Secret RBAC to overwrite an
// unrelated Secret via a name collision. The one exception is a legacy config Secret
// a migration-created Config stands for (adoptableLegacySecret). For such a Config the
// output objects that Secret recorded are returned, so applyOutputs may adopt them too.
func (r *Reconciler) applySecret(ctx context.Context, cfg *configv1alpha1.Config, secret *corev1.Secret) ([]corev1.ObjectReference, error) {
	existing := &corev1.Secret{ObjectMeta: secret.ObjectMeta}
	var legacyOutputs []corev1.ObjectReference
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, existing, func() error {
		if existing.UID != "" && !metav1.IsControlledBy(existing, cfg) && !adoptableLegacySecret(existing, cfg) {
			return fmt.Errorf("secret %s/%s already exists and is not owned by this Config", existing.Namespace, existing.Name)
		}
		if existing.UID != "" && legacyconfig.CreatedByMigration(cfg) && existing.Annotations[apitypes.AnnotationConfigAdoptBy] == cfg.Name {
			// What the legacy config rendered, read before the render below replaces it.
			// A list that does not parse adopts nothing; the outputs then get the refusal.
			if refs, err := legacyconfig.ObjectReferences(existing); err == nil {
				legacyOutputs = refs
			}
		}
		// Merge, not replace. The Secret is this Config's to reconcile, but not
		// everything on it is this Config's to own: an operator labels it for
		// cost allocation, another controller marks it, KubeVela's own source
		// cache records what the entry is for. Replacing wholesale destroyed all
		// of it within seconds of being written, with nothing to say why.
		//
		// Keys this Config sets - its own, plus whatever the template's output
		// declares - are reconciled to the desired value. A key it dropped since
		// the last pass is left behind rather than removed; erring towards
		// keeping is the safe direction when the alternative deletes somebody
		// else's metadata.
		existing.Labels = mergeMeta(existing.Labels, secret.Labels)
		existing.Annotations = mergeMeta(existing.Annotations, secret.Annotations)
		delete(existing.Annotations, apitypes.AnnotationConfigAdoptBy) // the mark has done its work
		existing.Data = secret.Data
		existing.StringData = secret.StringData
		existing.Type = secret.Type
		existing.OwnerReferences = secret.OwnerReferences
		return nil
	})
	return legacyOutputs, err
}

// adoptableLegacySecret reports a legacy config Secret this Config may take: one the
// migration library marked for it (AnnotationConfigAdoptBy names this Config) that nothing
// controls, or that a Config of this name and another UID did (deleted and recreated). The
// library creates the Config and then sets the Secret's owner reference; a reconcile between
// the two found the Secret unowned and reported Error until the next one, so the workflow
// step waiting on the Config, or a user watching it, saw an Error that was never final. The
// controller sets that owner reference itself now. The mark is the proof: writing it takes
// write access to the Secret, which a Config author alone does not have, so a hand-made
// Config cannot claim a Secret by name, with or without the migration label. The Config
// must carry that label as well: the mark goes on before the Config exists, and a Config of
// the name created by hand in that gap must not collect it. Any other Config, and any other
// Secret, still gets the refusal.
func adoptableLegacySecret(existing *corev1.Secret, cfg *configv1alpha1.Config) bool {
	if !legacyconfig.CreatedByMigration(cfg) || existing.Annotations[apitypes.AnnotationConfigAdoptBy] != cfg.Name || existing.Labels[apitypes.LabelConfigCatalog] != apitypes.VelaCoreConfig {
		return false
	}
	owner := metav1.GetControllerOf(existing)
	return owner == nil || (legacyconfig.IsConfigOwnerRef(*owner) && owner.Name == cfg.Name)
}

// adoptableLegacyOutput reports an output object a migration-created Config may take: one
// nothing controls that the legacy Secret it stands for recorded as its own output.
func adoptableLegacyOutput(existing, desired client.Object, legacyOutputs []corev1.ObjectReference) bool {
	if metav1.GetControllerOf(existing) != nil {
		return false
	}
	gvk := desired.GetObjectKind().GroupVersionKind()
	for _, ref := range legacyOutputs {
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err == nil && gv.Group == gvk.Group && ref.Kind == gvk.Kind && ref.Namespace == desired.GetNamespace() && ref.Name == desired.GetName() {
			return true
		}
	}
	return false
}

// mergeMeta layers desired keys over existing ones, leaving keys the caller does
// not set untouched.
func mergeMeta(existing, desired map[string]string) map[string]string {
	if len(desired) == 0 {
		return existing
	}
	if existing == nil {
		existing = map[string]string{}
	}
	for key, value := range desired {
		existing[key] = value
	}
	return existing
}

// applyOutputs applies the template.outputs objects, owned by the Config for GC on delete.
func (r *Reconciler) applyOutputs(ctx context.Context, cfg *configv1alpha1.Config, outputs []*unstructured.Unstructured, legacyOutputs []corev1.ObjectReference) error {
	for _, obj := range outputs {
		if err := controllerutil.SetControllerReference(cfg, obj, r.Scheme); err != nil {
			return fmt.Errorf("failed to set owner reference on output object %s %s: %w", obj.GetKind(), obj.GetName(), err)
		}
		mustBeOwnedByConfig := apply.MakeCustomApplyOption(func(existing, _ client.Object) error {
			if existing != nil && existing.GetUID() != "" && !metav1.IsControlledBy(existing, cfg) && !adoptableLegacyOutput(existing, obj, legacyOutputs) {
				return fmt.Errorf("object %s %s/%s already exists and is not owned by this Config", obj.GetKind(), obj.GetNamespace(), obj.GetName())
			}
			return nil
		})
		if err := r.applicator.Apply(ctx, obj, apply.DisableUpdateAnnotation(), apply.Quiet(), mustBeOwnedByConfig); err != nil {
			return fmt.Errorf("failed to apply output object %s %s/%s: %w", obj.GetKind(), obj.GetNamespace(), obj.GetName(), err)
		}
	}
	return nil
}

// errorRetryInterval is how long a Config stays in Error before it is rendered again.
// Some of its errors are about things outside it that nothing watches: an output object of
// the same name that someone else owns, of a kind the controller has no watch for. (A
// foreign Secret of the Config's name is watched, see findConfigsForSecret.)
// Without a retry the Config sat in Error until the informer resync, and a workflow step
// restarted after the obstacle was cleared read that stale Error and failed again. One
// Get and a render every half minute is the price. The suite shortens it.
var errorRetryInterval = 30 * time.Second

// markError records a reconcile failure on the status without propagating the
// error to the controller, so spec-level errors don't hot-loop; the Config is rendered
// again after errorRetryInterval. A write conflict is not the Config's error: someone
// else wrote the object between this reconcile's cached read and its write (the
// migration library's owner-reference patch on the Secret it adopts lands this way), so
// the Config is left as it is and read again shortly. Reporting it showed an Error phase
// that was never final.
func (r *Reconciler) markError(ctx context.Context, cfg *configv1alpha1.Config, err error) (ctrl.Result, error) {
	if apierrors.IsConflict(err) {
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}
	cfg.Status.Phase = configv1alpha1.ConfigPhaseError
	cfg.Status.SetConditions(condition.ReconcileError(err))
	if uerr := r.UpdateStatus(ctx, cfg); uerr != nil {
		return ctrl.Result{}, uerr
	}
	return ctrl.Result{RequeueAfter: errorRetryInterval}, nil
}

// UpdateStatus updates Config's Status with retry.RetryOnConflict
func (r *Reconciler) UpdateStatus(ctx context.Context, cfg *configv1alpha1.Config, opts ...client.SubResourceUpdateOption) error {
	status := cfg.DeepCopy().Status
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = r.Get(ctx, client.ObjectKey{Namespace: cfg.Namespace, Name: cfg.Name}, cfg); err != nil {
			return
		}
		cfg.Status = status
		return r.Status().Update(ctx, cfg, opts...)
	})
}

// findConfigsForTemplate re-triggers Configs when their ConfigTemplate's status changes.
func (r *Reconciler) findConfigsForTemplate(ctx context.Context, obj client.Object) []reconcile.Request {
	ct, ok := obj.(*configv1alpha1.ConfigTemplate)
	if !ok {
		return nil
	}
	var list configv1alpha1.ConfigList
	if err := r.List(ctx, &list); err != nil {
		klog.ErrorS(err, "failed to list Configs for ConfigTemplate watch", "configTemplate", klog.KObj(ct))
		return nil
	}
	var requests []reconcile.Request
	for i := range list.Items {
		cfg := &list.Items[i]
		ref := cfg.Spec.TemplateRef
		if ref == nil || ref.Name != ct.Name {
			continue
		}
		ns := ref.Namespace
		if ns == "" {
			ns = apitypes.DefaultKubeVelaNS
		}
		if ns != ct.Namespace {
			continue
		}
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cfg)})
	}
	return requests
}

// findConfigsForSecret re-triggers Configs when their spec.propertiesFrom Secret changes,
// and the Configs with a nacos block when a Nacos endpoint config Secret does.
func (r *Reconciler) findConfigsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return nil
	}
	requests := r.findConfigsForEndpoint(ctx, secret)
	var list configv1alpha1.ConfigList
	if err := r.List(ctx, &list, client.InNamespace(secret.Namespace)); err != nil {
		klog.ErrorS(err, "failed to list Configs for Secret watch", "secret", klog.KObj(secret))
		return requests
	}
	for i := range list.Items {
		cfg := &list.Items[i]
		// The Config reads it as properties, or it is the Config's own output Secret: one of
		// that name that someone else owns blocks the render, and its deletion is the event
		// the Config is waiting for.
		if ref := cfg.Spec.PropertiesFrom; (ref != nil && ref.SecretRef.Name == secret.Name) || cfg.Name == secret.Name {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cfg)})
		}
	}
	return requests
}

// findConfigsForLegacyTemplateConfigMap re-triggers Configs when their legacy template ConfigMap changes.
func (r *Reconciler) findConfigsForLegacyTemplateConfigMap(ctx context.Context, obj client.Object) []reconcile.Request {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok || !strings.HasPrefix(cm.Name, legacyconfig.TemplateConfigMapNamePrefix) {
		return nil
	}
	name := strings.TrimPrefix(cm.Name, legacyconfig.TemplateConfigMapNamePrefix)
	var list configv1alpha1.ConfigList
	if err := r.List(ctx, &list); err != nil {
		klog.ErrorS(err, "failed to list Configs for legacy template ConfigMap watch", "configMap", klog.KObj(cm))
		return nil
	}
	var requests []reconcile.Request
	for i := range list.Items {
		cfg := &list.Items[i]
		ref := cfg.Spec.TemplateRef
		if ref == nil || ref.Name != name {
			continue
		}
		ns := ref.Namespace
		if ns == "" {
			ns = apitypes.DefaultKubeVelaNS
		}
		if ns != cm.Namespace {
			continue
		}
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cfg)})
	}
	return requests
}

// SetupWithManager will setup with event recorder
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("Config")).
		WithAnnotations("controller", "Config")
	if r.applicator == nil {
		r.applicator = apply.NewAPIApplicator(mgr.GetClient())
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &configv1alpha1.Config{}, nacosEndpointIndex, nacosEndpointIndexValue); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.concurrentReconciles,
		}).
		For(&configv1alpha1.Config{}).
		Owns(&corev1.Secret{}).
		Watches(
			&configv1alpha1.ConfigTemplate{},
			ctrlHandler.EnqueueRequestsFromMapFunc(r.findConfigsForTemplate)).
		Watches(
			&corev1.Secret{},
			ctrlHandler.EnqueueRequestsFromMapFunc(r.findConfigsForSecret)).
		Watches(
			&corev1.ConfigMap{},
			ctrlHandler.EnqueueRequestsFromMapFunc(r.findConfigsForLegacyTemplateConfigMap)).
		Complete(r)
}

// Setup adds a controller that reconciles Config.
func Setup(mgr ctrl.Manager, args oamctrl.Args) error {
	r := Reconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		concurrentReconciles: args.ConcurrentReconciles,
		applicator:           apply.NewAPIApplicator(mgr.GetClient()),
	}
	return r.SetupWithManager(mgr)
}
