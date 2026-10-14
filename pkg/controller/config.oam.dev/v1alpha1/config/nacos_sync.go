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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	legacyconfig "github.com/oam-dev/kubevela/pkg/config"
	icontext "github.com/oam-dev/kubevela/pkg/config/context"
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

// renderExpandedWriter renders the nacos block with the same context the Secret render
// used. nil when the template has no block. A render failure is a template error.
func renderExpandedWriter(cfg *configv1alpha1.Config, tmpl *legacyconfig.ResolvedTemplate, props map[string]interface{}) (*writer.NacosData, error) {
	if tmpl == nil || tmpl.ExpandedWriter.Nacos == nil {
		return nil, nil
	}
	renderCtx := icontext.ConfigRenderContext{Name: cfg.Name, Namespace: cfg.Namespace}
	data, err := writer.RenderForExpandedWriter(tmpl.ExpandedWriter, tmpl.CUE, renderCtx, props)
	if err != nil {
		return nil, fmt.Errorf("failed to render the nacos block of config template %s/%s: %w", tmpl.Namespace, tmpl.Name, err)
	}
	return data.Nacos, nil
}

// syncNacos brings Nacos in line with the rendered block and records the outcome on the
// status. It returns an error only for a Nacos-side failure, after setting the
// NacosSynced condition to False with the reason; the caller persists the status and
// returns the error so controller-runtime requeues with backoff.
func (r *Reconciler) syncNacos(ctx context.Context, cfg *configv1alpha1.Config, data *writer.NacosData) error {
	if data == nil {
		cfg.Status.ExpandedWriter = nil
		dropCondition(&cfg.Status, configv1alpha1.ConditionNacosSynced)
		return nil
	}
	hash := writer.ContentHash(data)
	var current *configv1alpha1.NacosSyncStatus
	if cfg.Status.ExpandedWriter != nil {
		current = cfg.Status.ExpandedWriter.Nacos
	}
	synced := cfg.Status.GetCondition(configv1alpha1.ConditionNacosSynced)
	if current != nil && current.ContentHash == hash && synced.Status == corev1.ConditionTrue {
		return nil
	}

	// Status says the content is not current (or says nothing yet). Ask Nacos before
	// publishing: one read per real change, and no duplicate history entry when the
	// status seen here is older than what the controller last wrote.
	existing, err := writer.Read(ctx, data, r.readEndpointConfig)
	if err != nil {
		return r.nacosFailed(cfg, configv1alpha1.ReasonNacosReadFailed, err)
	}
	if len(existing) > 0 && writer.SameContent(data.Format, existing, data.Content) {
		klog.InfoS("nacos content already current, nothing published", "config", klog.KObj(cfg), "dataId", data.Metadata.DataID, "group", data.Metadata.Group)
		r.nacosSynced(cfg, data, hash, configv1alpha1.ReasonNacosAlreadyCurrent)
		return nil
	}

	if errs := writer.Write(ctx, &writer.ExpandedWriterData{Nacos: data}, r.readEndpointConfig); len(errs) > 0 {
		return r.nacosFailed(cfg, configv1alpha1.ReasonNacosPublishFailed, errs[0])
	}
	klog.InfoS("published the config content to nacos", "config", klog.KObj(cfg), "dataId", data.Metadata.DataID, "group", data.Metadata.Group)
	r.nacosSynced(cfg, data, hash, configv1alpha1.ReasonNacosPublished)
	return nil
}

// nacosSynced records a successful sync: the target, the hash and the time, and the
// condition True with the reason.
func (r *Reconciler) nacosSynced(cfg *configv1alpha1.Config, data *writer.NacosData, hash string, reason condition.ConditionReason) {
	now := metav1.Now()
	cfg.Status.ExpandedWriter = &configv1alpha1.ExpandedWriterStatus{Nacos: &configv1alpha1.NacosSyncStatus{
		DataID:       data.Metadata.DataID,
		Group:        data.Metadata.Group,
		ContentHash:  hash,
		LastSyncTime: &now,
	}}
	cfg.Status.SetConditions(condition.Condition{
		Type:               configv1alpha1.ConditionNacosSynced,
		Status:             corev1.ConditionTrue,
		LastTransitionTime: now,
		Reason:             reason,
	})
}

// nacosFailed records a failed attempt on the condition, leaving the last successful
// sync in status as it was, and returns the error for the requeue. An endpoint problem
// takes its own reason whichever step surfaced it.
func (r *Reconciler) nacosFailed(cfg *configv1alpha1.Config, reason condition.ConditionReason, err error) error {
	switch {
	case errors.Is(err, errEndpointNotFound):
		reason = configv1alpha1.ReasonNacosEndpointNotFound
	case errors.Is(err, errEndpointUnreadable):
		reason = configv1alpha1.ReasonNacosEndpointUnreadable
	}
	cfg.Status.SetConditions(condition.Condition{
		Type:               configv1alpha1.ConditionNacosSynced,
		Status:             corev1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            err.Error(),
	})
	return err
}

// readEndpointConfig is the controller's reader for the endpoint config the template
// names: the Secret endpoint.namespace/endpoint.name and its input-properties, the same
// key for a legacy Secret and for the Secret a Config CR materialises. A sensitive
// endpoint config is refused, as the legacy ReadConfig refuses it for the CLI.
func (r *Reconciler) readEndpointConfig(ctx context.Context, namespace, name string) (map[string]interface{}, error) {
	var secret corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: secret %s/%s", errEndpointNotFound, namespace, name)
		}
		return nil, fmt.Errorf("%w: secret %s/%s: %w", errEndpointUnreadable, namespace, name, err)
	}
	if secret.Annotations[apitypes.AnnotationConfigSensitive] == "true" {
		return nil, fmt.Errorf("%w: secret %s/%s: %w", errEndpointUnreadable, namespace, name, legacyconfig.ErrSensitiveConfig)
	}
	raw, ok := secret.Data[legacyconfig.SaveInputPropertiesKey]
	if !ok || len(raw) == 0 {
		return nil, fmt.Errorf("%w: secret %s/%s has no %s", errEndpointUnreadable, namespace, name, legacyconfig.SaveInputPropertiesKey)
	}
	properties := map[string]interface{}{}
	if err := json.Unmarshal(raw, &properties); err != nil {
		return nil, fmt.Errorf("%w: secret %s/%s: %s is not json: %w", errEndpointUnreadable, namespace, name, legacyconfig.SaveInputPropertiesKey, err)
	}
	return properties, nil
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
