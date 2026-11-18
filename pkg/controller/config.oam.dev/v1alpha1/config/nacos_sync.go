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
	"errors"
	"fmt"

	"cuelang.org/go/cue"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	legacyconfig "github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/config/writer"
)

// The controller publishes the template's nacos block for each Config, which the CLI
// did synchronously at create time before. The rules, from the issue: write only when
// the rendered content changes; read before publishing so content Nacos already holds
// is never published again (a config the CLI published before the upgrade, or a
// reconcile that runs on a cache that has not seen the controller's own status write
// yet); every Nacos failure is retried with the controller's backoff and none is
// terminal; the Config's phase stays Available because its Secret and outputs are in
// place; nothing is ever deleted from Nacos.

// Sentinel errors for the endpoint config, so a failure can be reported under the right
// reason whether it surfaced from the read or from the publish.
var (
	errEndpointNotFound   = errors.New("nacos endpoint config not found")
	errEndpointUnreadable = errors.New("nacos endpoint config unreadable")
)

// errEndpointUnmarked is the cause endpointUnreadable wraps for a sensitive endpoint config
// its owner has not marked for use.
var errEndpointUnmarked = fmt.Errorf("sensitive and not marked %s: %q (annotate its Config, or the legacy config Secret when no Config controls it)", apitypes.AnnotationConfigNacosEndpoint, legacyconfig.NacosEndpointAnnotationValue)

// endpointUnreadable is the one shape of "the endpoint config exists but cannot be
// used": malformed, refused by the API, sensitive and not marked for use, or no client can
// be built from it.
func endpointUnreadable(namespace, name string, err error) error {
	return fmt.Errorf("%w: secret %s/%s: %w", errEndpointUnreadable, namespace, name, err)
}

// validateExpandedWriter repeats the ConfigTemplate webhook's structural check of the
// nacos block (the same writer.ValidateTemplate), so a broken template is never silently
// ignored when the webhooks are off. A failure is a template error: the Config goes to
// Error with the message.
func validateExpandedWriter(ctx context.Context, tmpl *legacyconfig.ResolvedTemplate) error {
	if tmpl == nil || tmpl.ExpandedWriter.Nacos == nil {
		return nil
	}
	if err := writer.ValidateTemplate(ctx, tmpl.CUE); err != nil {
		return fmt.Errorf("config template %s/%s: %w", tmpl.Namespace, tmpl.Name, err)
	}
	return nil
}

// renderExpandedWriter reads the nacos block out of the rendered template value the
// Secret render produced, so the Secret, the outputs and the block come from one
// compile. nil when the template has no block. A render failure is a template error, and
// so is an endpoint the template may not name (config.RenderedEndpointError).
func renderExpandedWriter(tmpl *legacyconfig.ResolvedTemplate, rendered cue.Value) (*writer.NacosData, error) {
	if tmpl == nil || tmpl.ExpandedWriter.Nacos == nil {
		return nil, nil
	}
	data, err := writer.RenderForExpandedWriter(tmpl.ExpandedWriter, rendered)
	if err != nil {
		return nil, fmt.Errorf("failed to render the nacos block of config template %s/%s: %w", tmpl.Namespace, tmpl.Name, err)
	}
	// the controller reads the endpoint with its own rights: only one the template may name
	if data.Nacos != nil {
		if err := legacyconfig.RenderedEndpointError(tmpl, data.Nacos.Endpoint); err != nil {
			return nil, err
		}
	}
	return data.Nacos, nil
}

// syncNacos brings Nacos in line with the rendered block and records the outcome on the
// status. It returns an error only for a Nacos-side failure, after setting the
// NacosSynced condition to False with the reason; the caller persists the status and
// returns the error so controller-runtime requeues with backoff.
//
// The endpoint config is read on every pass, from the cache, before the fast path: its
// digest is part of what "current" means, so a Config synced through one server is
// re-synced (read first, published only if missing) when the template's endpoint is
// pointed at another, and a Config whose endpoint disappears says so on the condition
// instead of staying True against a server nothing can reach any more.
func (r *Reconciler) syncNacos(ctx context.Context, cfg *configv1alpha1.Config, data *writer.NacosData) error {
	if data == nil {
		cfg.Status.ExpandedWriter = nil
		dropCondition(&cfg.Status, configv1alpha1.ConditionNacosSynced)
		return nil
	}
	endpointConfig, err := r.readEndpointConfig(ctx, data.Endpoint.Namespace, data.Endpoint.Name)
	if err != nil {
		return r.nacosFailed(cfg, data, configv1alpha1.ReasonNacosReadFailed, err)
	}
	hash := writer.ContentHash(data)
	endpointHash, err := writer.EndpointHash(endpointConfig)
	if err != nil {
		return r.nacosFailed(cfg, data, configv1alpha1.ReasonNacosReadFailed, endpointUnreadable(data.Endpoint.Namespace, data.Endpoint.Name, err))
	}
	var current *configv1alpha1.NacosSyncStatus
	if cfg.Status.ExpandedWriter != nil {
		current = cfg.Status.ExpandedWriter.Nacos
	}
	synced := cfg.Status.GetCondition(configv1alpha1.ConditionNacosSynced)
	if current != nil && current.ContentHash == hash && current.EndpointHash == endpointHash && synced.Status == corev1.ConditionTrue {
		return nil
	}

	// One client for the read and the publish: the SDK re-initialises its global logger
	// on every construction, so the controller builds at most one per sync. With the
	// client set, the read and the publish never consult an endpoint reader.
	cli, release, err := data.NewClient(endpointConfig)
	if err != nil {
		return r.nacosFailed(cfg, data, configv1alpha1.ReasonNacosReadFailed, endpointUnreadable(data.Endpoint.Namespace, data.Endpoint.Name, err))
	}
	defer release()
	data.Client = cli

	// Status says the content is not current (or says nothing yet, or was confirmed
	// through another endpoint). Ask Nacos before publishing: one read per real change,
	// and no duplicate history entry when the status seen here is older than what the
	// controller last wrote.
	existing, err := writer.Read(ctx, data, nil)
	if err != nil {
		return r.nacosFailed(cfg, data, configv1alpha1.ReasonNacosReadFailed, err)
	}
	if len(existing) > 0 && writer.SameContent(data.Format, existing, data.Content) {
		klog.InfoS("nacos content already current, nothing published", "config", klog.KObj(cfg), "dataId", data.Metadata.DataID, "group", data.Metadata.Group)
		r.nacosSynced(cfg, data, hash, endpointHash, configv1alpha1.ReasonNacosAlreadyCurrent)
		return nil
	}

	if errs := writer.Write(ctx, &writer.ExpandedWriterData{Nacos: data}, nil); len(errs) > 0 {
		return r.nacosFailed(cfg, data, configv1alpha1.ReasonNacosPublishFailed, errs[0])
	}
	klog.InfoS("published the config content to nacos", "config", klog.KObj(cfg), "dataId", data.Metadata.DataID, "group", data.Metadata.Group)
	r.nacosSynced(cfg, data, hash, endpointHash, configv1alpha1.ReasonNacosPublished)
	return nil
}

// nacosSynced records a successful sync: the target, the two hashes and the time, and
// the condition True with the reason.
func (r *Reconciler) nacosSynced(cfg *configv1alpha1.Config, data *writer.NacosData, hash, endpointHash string, reason condition.ConditionReason) {
	now := metav1.Now()
	cfg.Status.ExpandedWriter = &configv1alpha1.ExpandedWriterStatus{Nacos: &configv1alpha1.NacosSyncStatus{
		DataID:       data.Metadata.DataID,
		Group:        data.Metadata.Group,
		ContentHash:  hash,
		Endpoint:     endpointReference(data),
		EndpointHash: endpointHash,
		LastSyncTime: &now,
	}}
	cfg.Status.SetConditions(condition.Condition{
		Type:               configv1alpha1.ConditionNacosSynced,
		Status:             corev1.ConditionTrue,
		LastTransitionTime: now,
		Reason:             reason,
	})
}

// nacosFailed records a failed attempt on the condition and returns the error for the
// requeue. The hashes of the last successful sync, if any, stay as they were; the target
// and the endpoint the template names are recorded whatever the outcome, so the
// endpoint Secret watch finds this Config by name while it waits. An endpoint problem
// takes its own reason whichever step surfaced it.
func (r *Reconciler) nacosFailed(cfg *configv1alpha1.Config, data *writer.NacosData, reason condition.ConditionReason, err error) error {
	switch {
	case errors.Is(err, errEndpointNotFound):
		reason = configv1alpha1.ReasonNacosEndpointNotFound
	case errors.Is(err, errEndpointUnreadable):
		reason = configv1alpha1.ReasonNacosEndpointUnreadable
	}
	if cfg.Status.ExpandedWriter == nil || cfg.Status.ExpandedWriter.Nacos == nil {
		cfg.Status.ExpandedWriter = &configv1alpha1.ExpandedWriterStatus{Nacos: &configv1alpha1.NacosSyncStatus{}}
	}
	status := cfg.Status.ExpandedWriter.Nacos
	status.DataID = data.Metadata.DataID
	status.Group = data.Metadata.Group
	status.Endpoint = endpointReference(data)
	cfg.Status.SetConditions(condition.Condition{
		Type:               configv1alpha1.ConditionNacosSynced,
		Status:             corev1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            err.Error(),
	})
	return err
}

func endpointReference(data *writer.NacosData) *configv1alpha1.NacosEndpointReference {
	return &configv1alpha1.NacosEndpointReference{Name: data.Endpoint.Name, Namespace: data.Endpoint.Namespace}
}

// readEndpointConfig is the controller's reader for the endpoint config the template
// names: the Secret endpoint.namespace/endpoint.name and its input-properties, the same
// key for a legacy Secret and for the Secret a Config CR materialises. The template rules
// (config.RenderedEndpointError) let any template name an endpoint in its own namespace
// or in vela-system, and the controller reads it with its own rights; so a sensitive
// config is used only with its owner's leave, the AnnotationConfigNacosEndpoint
// annotation (on a Config CR, carried to its Secret). With it the config is read whatever
// its sensitive mark: that mark guards what users read back, and the controller reads the
// credentials to use them. A config that is not sensitive needs no annotation.
func (r *Reconciler) readEndpointConfig(ctx context.Context, namespace, name string) (map[string]interface{}, error) {
	var secret corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: secret %s/%s", errEndpointNotFound, namespace, name)
		}
		return nil, endpointUnreadable(namespace, name, err)
	}
	if legacyconfig.SecretSensitive(&secret) && !legacyconfig.NacosEndpointMarked(&secret) {
		return nil, endpointUnreadable(namespace, name, errEndpointUnmarked)
	}
	properties, err := legacyconfig.SecretInputProperties(&secret)
	if err != nil {
		return nil, endpointUnreadable(namespace, name, err)
	}
	return properties, nil
}

// nacosEndpointIndex indexes Configs by the endpoint config their status names
// (namespace/name), so the endpoint Secret watch finds exactly the Configs that use it.
// The reference is written on every pass, success or failure, so a Config waiting on a
// missing endpoint is indexed too; a Config whose status predates the field is indexed
// at its next reconcile, which every Config gets when the controller starts.
const nacosEndpointIndex = "status.expandedWriter.nacos.endpoint"

func nacosEndpointIndexValue(obj client.Object) []string {
	cfg, ok := obj.(*configv1alpha1.Config)
	if !ok || cfg.Status.ExpandedWriter == nil || cfg.Status.ExpandedWriter.Nacos == nil || cfg.Status.ExpandedWriter.Nacos.Endpoint == nil {
		return nil
	}
	ep := cfg.Status.ExpandedWriter.Nacos.Endpoint
	return []string{ep.Namespace + "/" + ep.Name}
}

// findConfigsForEndpoint re-triggers the Configs whose status names the config Secret
// that changed, appeared or went: EndpointNotFound clears as soon as the endpoint exists
// rather than on backoff, a changed endpoint re-syncs through its digest, and a deleted
// one is reported. Only velacore-config Secrets can be endpoint configs, so other Secret
// events never reach the index.
func (r *Reconciler) findConfigsForEndpoint(ctx context.Context, secret *corev1.Secret) []reconcile.Request {
	if secret.Labels[apitypes.LabelConfigCatalog] != apitypes.VelaCoreConfig {
		return nil
	}
	var list configv1alpha1.ConfigList
	if err := r.List(ctx, &list, client.MatchingFields{nacosEndpointIndex: secret.Namespace + "/" + secret.Name}); err != nil {
		klog.ErrorS(err, "failed to list Configs for endpoint Secret watch", "secret", klog.KObj(secret))
		return nil
	}
	requests := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
	}
	return requests
}

// dropCondition removes a condition type from the status; ConditionedStatus has no
// remover of its own.
func dropCondition(status *configv1alpha1.ConfigStatus, t condition.ConditionType) {
	kept := status.Conditions[:0]
	for _, c := range status.Conditions {
		if c.Type != t {
			kept = append(kept, c)
		}
	}
	status.Conditions = kept
}
