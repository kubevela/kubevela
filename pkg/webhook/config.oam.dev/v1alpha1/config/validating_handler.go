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
	"net/http"

	"cuelang.org/go/cue"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	legacyconfig "github.com/oam-dev/kubevela/pkg/config"
	icontext "github.com/oam-dev/kubevela/pkg/config/context"
	velacue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/cue/script"
)

var configGVR = configv1alpha1.ConfigGVR

const defaultPropertiesSecretKey = "properties"

// ValidatingHandler validates Config resources.
type ValidatingHandler struct {
	Decoder admission.Decoder
	Client  client.Client
}

var _ admission.Handler = &ValidatingHandler{}

// Handle validates spec.properties/spec.propertiesFrom mutual exclusivity and, when
// the referenced template can be resolved, that the properties match its CUE schema
// and satisfy any template.validation.$returns check. If the template can't be
// resolved yet (not found, or a ConfigTemplate CRD not yet Available), validation is
// skipped here and left to the Config controller, which re-reconciles once the
// template becomes available.
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

	if obj.Spec.Properties != nil && obj.Spec.PropertiesFrom != nil {
		return admission.Denied(fmt.Sprintf("spec.properties and spec.propertiesFrom are mutually exclusive (requestUID=%s)", req.UID))
	}

	if obj.Spec.TemplateRef == nil {
		return admission.ValidationResponse(true, "")
	}

	cueScript, resolved, err := h.resolveTemplate(ctx, obj.Spec.TemplateRef)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if !resolved {
		return admission.ValidationResponse(true, "")
	}

	props, err := h.resolveProperties(ctx, obj)
	if err != nil {
		return admission.Denied(fmt.Sprintf("%s (requestUID=%s)", err.Error(), req.UID))
	}

	if err := cueScript.ValidatePropertiesWithCueX(ctx, props); err != nil {
		return admission.Denied(fmt.Sprintf("properties do not match template schema: %s (requestUID=%s)", err.Error(), req.UID))
	}

	contextValue := icontext.ConfigRenderContext{Name: obj.Name, Namespace: obj.Namespace}
	val, err := cueScript.RunAndOutputWithCueX(ctx, contextValue, props)
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

	return admission.ValidationResponse(true, "")
}

// resolveTemplate looks up the referenced template, CRD first, falling back to the
// legacy config-template-<name> ConfigMap, the same way the Config controller does.
func (h *ValidatingHandler) resolveTemplate(ctx context.Context, ref *configv1alpha1.ConfigTemplateReference) (script.CUE, bool, error) {
	ns := ref.Namespace
	if ns == "" {
		ns = apitypes.DefaultKubeVelaNS
	}

	var ct configv1alpha1.ConfigTemplate
	switch err := h.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: ref.Name}, &ct); {
	case err == nil:
		if ct.Status.Phase != configv1alpha1.ConfigTemplatePhaseAvailable {
			return "", false, nil
		}
		return script.CUE(ct.Spec.Template), true, nil
	case apierrors.IsNotFound(err):
	default:
		return "", false, err
	}

	var cm corev1.ConfigMap
	cmName := legacyconfig.TemplateConfigMapNamePrefix + ref.Name
	switch err := h.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: cmName}, &cm); {
	case err == nil:
		return script.CUE(cm.Data[legacyconfig.SaveTemplateKey]), true, nil
	case apierrors.IsNotFound(err):
		return "", false, nil
	default:
		return "", false, err
	}
}

// resolveProperties reads the Config's properties, either inline or from the
// referenced Secret, the same way the Config controller does.
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
		raw := secret.Data[key]
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
