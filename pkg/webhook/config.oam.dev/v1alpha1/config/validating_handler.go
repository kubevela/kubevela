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
	"net/http"
	"reflect"

	"cuelang.org/go/cue"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	legacyconfig "github.com/oam-dev/kubevela/pkg/config"
	icontext "github.com/oam-dev/kubevela/pkg/config/context"
	velacue "github.com/oam-dev/kubevela/pkg/cue"
)

var configGVR = configv1alpha1.ConfigGVR

const defaultPropertiesSecretKey = "properties"

// ValidatingHandler validates Config resources.
type ValidatingHandler struct {
	Decoder admission.Decoder
	Client  client.Client
}

var _ admission.Handler = &ValidatingHandler{}

func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Resource.String() != configGVR.String() {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("expect resource to be %s", configGVR))
	}
	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
		return admission.ValidationResponse(true, "")
	}

	obj := &configv1alpha1.Config{}
	if err := h.Decoder.Decode(req, obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// spec.templateRef is immutable, matching the legacy factory's ErrChangeTemplate:
	// delete and recreate to move a config to a different template. Compares the whole
	// reference (not just .Name) so a namespace change, or adding/removing the field
	// entirely, is caught too - all three change which template is in effect.
	if req.Operation == admissionv1.Update {
		oldObj := &configv1alpha1.Config{}
		if err := h.Decoder.DecodeRaw(req.OldObject, oldObj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if !reflect.DeepEqual(obj.Spec.TemplateRef, oldObj.Spec.TemplateRef) {
			return admission.Denied(fmt.Sprintf(
				"%s: delete and recreate the config to use a different template (requestUID=%s)",
				legacyconfig.ErrChangeTemplate, req.UID))
		}
	}

	if obj.Spec.Properties != nil && obj.Spec.PropertiesFrom != nil {
		return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", legacyconfig.ErrMutuallyExclusiveProperties, req.UID))
	}

	if obj.Spec.TemplateRef == nil {
		return admission.ValidationResponse(true, "")
	}

	tmpl, resolved, err := h.resolveTemplate(ctx, obj.Spec.TemplateRef)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if !resolved {
		return admission.ValidationResponse(true, "")
	}

	if tmpl.Sensitive && obj.Spec.Properties != nil {
		return admission.Denied(fmt.Sprintf(
			"config template %s/%s is sensitive: spec.properties must not be set, use spec.propertiesFrom instead (requestUID=%s)",
			tmpl.Namespace, tmpl.Name, req.UID))
	}

	props, err := h.resolveProperties(ctx, obj)
	if err != nil {
		return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
	}

	if err := tmpl.CUE.ValidatePropertiesWithCueX(ctx, props); err != nil {
		return admission.Denied(fmt.Sprintf("properties do not match template schema: %s (requestUID=%s)", err.Error(), req.UID))
	}

	contextValue := icontext.ConfigRenderContext{Name: obj.Name, Namespace: obj.Namespace}
	val, err := tmpl.CUE.RunAndOutputWithCueX(ctx, contextValue, props)
	if err != nil && !velacue.IsFieldNotExist(err) {
		return admission.Denied(fmt.Sprintf("failed to render config template: %s (requestUID=%s)", err.Error(), req.UID))
	}
	if validReturns := val.LookupPath(cue.ParsePath(legacyconfig.TemplateValidationReturns)); validReturns.Exists() {
		var validation legacyconfig.Validation
		if err := validReturns.Decode(&validation); err != nil {
			return admission.Denied(fmt.Sprintf("template.validation.$returns format must be a validation object: %s (requestUID=%s)", err.Error(), req.UID))
		}
		if len(validation.Message) > 0 {
			return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", validation.Message, req.UID))
		}
	}
	if output := val.LookupPath(cue.ParsePath(legacyconfig.TemplateOutput)); output.Exists() {
		var secret corev1.Secret
		if err := output.Decode(&secret); err != nil {
			return admission.Denied(fmt.Sprintf("template.output format must be a secret: %s (requestUID=%s)", err.Error(), req.UID))
		}
	}

	return admission.ValidationResponse(true, "")
}

// resolveTemplate mirrors the Config controller's CRD-first, ConfigMap-fallback lookup.
// resolved is false (with no error) both when the template doesn't exist yet and when a
// ConfigTemplate CRD exists but hasn't reached Available - either way admission defers
// to the controller, which will surface a clear error on the Config once it reconciles.
func (h *ValidatingHandler) resolveTemplate(ctx context.Context, ref *configv1alpha1.ConfigTemplateReference) (*legacyconfig.ResolvedTemplate, bool, error) {
	tmpl, waiting, err := legacyconfig.ResolveConfigTemplate(ctx, h.Client, ref)
	switch {
	case errors.Is(err, legacyconfig.ErrTemplateNotFound):
		return nil, false, nil
	case err != nil:
		return nil, false, err
	case waiting:
		return nil, false, nil
	default:
		return tmpl, true, nil
	}
}

// resolveProperties mirrors the Config controller's inline/Secret property resolution.
func (h *ValidatingHandler) resolveProperties(ctx context.Context, cfg *configv1alpha1.Config) (map[string]interface{}, error) {
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
		if err := h.Client.Get(ctx, client.ObjectKey{Namespace: cfg.Namespace, Name: cfg.Spec.PropertiesFrom.SecretRef.Name}, &secret); err != nil {
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

// RegisterValidatingHandler registers Config validation to the webhook server.
func RegisterValidatingHandler(mgr manager.Manager) {
	server := mgr.GetWebhookServer()
	server.Register("/validating-config-oam-dev-v1alpha1-configs", &webhook.Admission{Handler: &ValidatingHandler{
		Client:  mgr.GetClient(),
		Decoder: admission.NewDecoder(mgr.GetScheme()),
	}})
}
