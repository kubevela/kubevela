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
	"errors"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	"github.com/oam-dev/kubevela/pkg/cue/script"
)

// ErrMutuallyExclusiveProperties is returned when both spec.properties and
// spec.propertiesFrom are set on a Config.
var ErrMutuallyExclusiveProperties = errors.New("spec.properties and spec.propertiesFrom are mutually exclusive")

// defaultPropertiesSecretKey is the key read from spec.propertiesFrom.secretRef when
// no key is specified.
const defaultPropertiesSecretKey = "properties"

// resolvedTemplate is the template resolved for a Config, whether sourced from a
// ConfigTemplate CRD or a legacy config-template-* ConfigMap.
type resolvedTemplate struct {
	name      string
	namespace string
	cue       script.CUE
	scope     string
	sensitive bool
}

// Reconciler reconciles a Config object. It resolves the referenced template
// (ConfigTemplate CRD first, falling back to a legacy config-template-* ConfigMap)
// and properties (inline or from a Secret), evaluates the CUE template, and
// materializes the result as a Secret owned by the Config.
type Reconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	record               event.Recorder
	concurrentReconciles int
}

// Reconcile is the main logic for the Config controller.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	klog.InfoS("Reconciling Config", "name", req.Name, "namespace", req.Namespace)

	var cfg configv1alpha1.Config
	if err := r.Get(ctx, req.NamespacedName, &cfg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// no finalizer: the materialized Secret is owned by the Config and is garbage
	// collected natively by Kubernetes when the Config is deleted.
	if cfg.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	if cfg.Spec.Properties != nil && cfg.Spec.PropertiesFrom != nil {
		return r.markError(ctx, &cfg, ErrMutuallyExclusiveProperties)
	}

	var tmpl *resolvedTemplate
	if cfg.Spec.TemplateRef != nil {
		resolved, waiting, err := r.resolveTemplate(ctx, cfg.Spec.TemplateRef)
		if err != nil {
			return r.markError(ctx, &cfg, err)
		}
		if waiting {
			// the ConfigTemplate exists but hasn't finished reconciling its schema yet;
			// the Watches on ConfigTemplate below re-triggers us once it becomes Available.
			return r.markError(ctx, &cfg, fmt.Errorf("config template %s/%s is not Available yet", cfg.Spec.TemplateRef.Namespace, cfg.Spec.TemplateRef.Name))
		}
		tmpl = resolved
	}

	props, err := r.resolveProperties(ctx, &cfg)
	if err != nil {
		return r.markError(ctx, &cfg, err)
	}

	secret, err := r.renderSecret(ctx, &cfg, tmpl, props)
	if err != nil {
		return r.markError(ctx, &cfg, err)
	}

	if err := controllerutil.SetControllerReference(&cfg, secret, r.Scheme); err != nil {
		return r.markError(ctx, &cfg, err)
	}

	if err := r.applySecret(ctx, &cfg, secret); err != nil {
		return r.markError(ctx, &cfg, err)
	}

	cfg.Status.Phase = configv1alpha1.ConfigPhaseAvailable
	cfg.Status.SecretRef = &corev1.LocalObjectReference{Name: secret.Name}
	cfg.Status.SetConditions(condition.ReconcileSuccess())
	return ctrl.Result{}, r.UpdateStatus(ctx, &cfg)
}

// resolveTemplate looks up the referenced template, CRD first, falling back to a
// legacy config-template-<name> ConfigMap. waiting is true if a ConfigTemplate CRD
// was found but has not finished reconciling its schema.
func (r *Reconciler) resolveTemplate(ctx context.Context, ref *configv1alpha1.ConfigTemplateReference) (*resolvedTemplate, bool, error) {
	ns := ref.Namespace
	if ns == "" {
		ns = apitypes.DefaultKubeVelaNS
	}

	var ct configv1alpha1.ConfigTemplate
	err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: ref.Name}, &ct)
	switch {
	case err == nil:
		if ct.Status.Phase != configv1alpha1.ConfigTemplatePhaseAvailable {
			return nil, true, nil
		}
		return &resolvedTemplate{
			name:      ct.Name,
			namespace: ns,
			cue:       script.CUE(ct.Spec.Template),
			scope:     string(ct.Spec.Scope),
			sensitive: ct.Spec.Sensitive,
		}, false, nil
	case apierrors.IsNotFound(err):
		// fall back to the legacy ConfigMap convention
	default:
		return nil, false, err
	}

	var cm corev1.ConfigMap
	cmName := legacyconfig.TemplateConfigMapNamePrefix + ref.Name
	if err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: cmName}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, false, legacyconfig.ErrTemplateNotFound
		}
		return nil, false, err
	}
	return &resolvedTemplate{
		name:      ref.Name,
		namespace: ns,
		cue:       script.CUE(cm.Data[legacyconfig.SaveTemplateKey]),
		scope:     cm.Labels[apitypes.LabelConfigScope],
		sensitive: cm.Annotations[apitypes.AnnotationConfigSensitive] == "true",
	}, false, nil
}

// resolveProperties reads the Config's properties, either inline or from the
// referenced Secret.
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

// renderSecret evaluates the resolved template (if any) against the properties and
// builds the output Secret to materialize.
func (r *Reconciler) renderSecret(ctx context.Context, cfg *configv1alpha1.Config, tmpl *resolvedTemplate, props map[string]interface{}) (*corev1.Secret, error) {
	secret := &corev1.Secret{}

	if tmpl != nil {
		contextValue := icontext.ConfigRenderContext{Name: cfg.Name, Namespace: cfg.Namespace}
		val, err := tmpl.cue.RunAndOutputWithCueX(ctx, contextValue, props)
		if err != nil && !velacue.IsFieldNotExist(err) {
			return nil, fmt.Errorf("failed to render config template: %w", err)
		}

		validReturns := val.LookupPath(cue.ParsePath(legacyconfig.TemplateValidationReturns))
		if validReturns.Exists() {
			var validation legacyconfig.Validation
			if err := validReturns.Decode(&validation); err != nil {
				return nil, fmt.Errorf("template.validation.$returns format must be a validation object: %w", err)
			}
			if len(validation.Message) > 0 {
				return nil, &validation
			}
		}

		output := val.LookupPath(cue.ParsePath(legacyconfig.TemplateOutput))
		if output.Exists() {
			if err := output.Decode(secret); err != nil {
				return nil, fmt.Errorf("template.output format must be a secret: %w", err)
			}
		}

		if secret.Labels == nil {
			secret.Labels = map[string]string{}
		}
		secret.Labels[apitypes.LabelConfigCatalog] = apitypes.VelaCoreConfig
		secret.Labels[apitypes.LabelConfigType] = tmpl.name
		secret.Labels[apitypes.LabelConfigScope] = tmpl.scope

		if secret.Annotations == nil {
			secret.Annotations = map[string]string{}
		}
		secret.Annotations[apitypes.AnnotationConfigSensitive] = fmt.Sprintf("%t", tmpl.sensitive)
		secret.Annotations[apitypes.AnnotationConfigTemplateNamespace] = tmpl.namespace
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
		return nil, fmt.Errorf("failed to encode properties: %w", err)
	}
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	// keep the input properties readable under the same key the legacy
	// Factory.ReadConfig/ListConfigs use, so CRD-materialized Secrets remain
	// discoverable through the existing read paths.
	secret.Data[legacyconfig.SaveInputPropertiesKey] = propsJSON

	return secret, nil
}

// applySecret creates or updates the materialized output Secret. It refuses to
// adopt a pre-existing Secret that isn't already controlled by this Config, so a
// user who can only create Configs can't use the manager's broader Secret RBAC to
// overwrite an unrelated Secret via a name collision.
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

// markError records a reconcile failure on the Config status. Spec-level errors
// (bad template ref, mutually exclusive properties, invalid properties) are not
// propagated to the controller so they don't hot-loop; only the status update
// itself can trigger a requeue via the returned error.
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

// findConfigsForTemplate re-triggers Configs that reference a ConfigTemplate whose
// status just changed (e.g. it just became Available).
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

// findConfigsForSecret re-triggers Configs whose spec.propertiesFrom references a
// Secret that just changed, so creating/rotating the source Secret doesn't require
// an unrelated Config spec change to pick up.
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

// findConfigsForLegacyTemplateConfigMap re-triggers Configs whose templateRef
// resolves to a legacy config-template-<name> ConfigMap that just changed, so
// creating or fixing the ConfigMap doesn't require an unrelated Config spec change
// to pick up.
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
	}
	return r.SetupWithManager(mgr)
}
