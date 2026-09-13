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

	"cuelang.org/go/cue"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
	velacue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

const defaultPropertiesSecretKey = "properties"

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

	props, err := r.resolveProperties(ctx, &cfg)
	if err != nil {
		return r.markError(ctx, &cfg, err)
	}

	secret, outputs, err := r.renderSecret(ctx, &cfg, tmpl, props)
	if err != nil {
		return r.markError(ctx, &cfg, err)
	}

	if err := controllerutil.SetControllerReference(&cfg, secret, r.Scheme); err != nil {
		return r.markError(ctx, &cfg, err)
	}

	if err := r.applySecret(ctx, &cfg, secret); err != nil {
		return r.markError(ctx, &cfg, err)
	}

	if err := r.applyOutputs(ctx, &cfg, outputs); err != nil {
		return r.markError(ctx, &cfg, err)
	}

	cfg.Status.Phase = configv1alpha1.ConfigPhaseAvailable
	cfg.Status.SecretRef = &corev1.LocalObjectReference{Name: secret.Name}
	cfg.Status.SetConditions(condition.ReconcileSuccess())
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
			key = defaultPropertiesSecretKey
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

func (r *Reconciler) renderSecret(ctx context.Context, cfg *configv1alpha1.Config, tmpl *legacyconfig.ResolvedTemplate, props map[string]interface{}) (*corev1.Secret, []*unstructured.Unstructured, error) {
	secret := &corev1.Secret{}
	var outputs []*unstructured.Unstructured

	if tmpl != nil {
		contextValue := icontext.ConfigRenderContext{Name: cfg.Name, Namespace: cfg.Namespace}
		val, err := tmpl.CUE.RunAndOutputWithCueX(ctx, contextValue, props)
		if err != nil && !velacue.IsFieldNotExist(err) {
			return nil, nil, fmt.Errorf("failed to render config template: %w", err)
		}

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
// unrelated Secret via a name collision.
func (r *Reconciler) applySecret(ctx context.Context, cfg *configv1alpha1.Config, secret *corev1.Secret) error {
	existing := &corev1.Secret{ObjectMeta: secret.ObjectMeta}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, existing, func() error {
		if existing.UID != "" && !metav1.IsControlledBy(existing, cfg) {
			return fmt.Errorf("secret %s/%s already exists and is not owned by this Config", existing.Namespace, existing.Name)
		}
		existing.Labels = secret.Labels
		existing.Annotations = secret.Annotations
		existing.Data = secret.Data
		existing.StringData = secret.StringData
		existing.Type = secret.Type
		existing.OwnerReferences = secret.OwnerReferences
		return nil
	})
	return err
}

// applyOutputs applies the template.outputs objects, owned by the Config for GC on delete.
func (r *Reconciler) applyOutputs(ctx context.Context, cfg *configv1alpha1.Config, outputs []*unstructured.Unstructured) error {
	for _, obj := range outputs {
		if err := controllerutil.SetControllerReference(cfg, obj, r.Scheme); err != nil {
			return fmt.Errorf("failed to set owner reference on output object %s %s: %w", obj.GetKind(), obj.GetName(), err)
		}
		mustBeOwnedByConfig := apply.MakeCustomApplyOption(func(existing, _ client.Object) error {
			if existing != nil && existing.GetUID() != "" && !metav1.IsControlledBy(existing, cfg) {
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

// markError records a reconcile failure on the status without propagating the
// error to the controller, so spec-level errors don't hot-loop.
func (r *Reconciler) markError(ctx context.Context, cfg *configv1alpha1.Config, err error) (ctrl.Result, error) {
	cfg.Status.Phase = configv1alpha1.ConfigPhaseError
	cfg.Status.SetConditions(condition.ReconcileError(err))
	if uerr := r.UpdateStatus(ctx, cfg); uerr != nil {
		return ctrl.Result{}, uerr
	}
	return ctrl.Result{}, nil
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

// findConfigsForSecret re-triggers Configs when their spec.propertiesFrom Secret changes.
func (r *Reconciler) findConfigsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return nil
	}
	var list configv1alpha1.ConfigList
	if err := r.List(ctx, &list, client.InNamespace(secret.Namespace)); err != nil {
		klog.ErrorS(err, "failed to list Configs for Secret watch", "secret", klog.KObj(secret))
		return nil
	}
	var requests []reconcile.Request
	for i := range list.Items {
		cfg := &list.Items[i]
		ref := cfg.Spec.PropertiesFrom
		if ref == nil || ref.SecretRef.Name != secret.Name {
			continue
		}
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cfg)})
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
