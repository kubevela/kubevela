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

// Package addon holds the admission webhook for the addon-as-component feature.
//
// It is deliberately separate from the shared Application webhook. The addon check
// is critical when the feature is in use, so it needs a failurePolicy that can be
// reasoned about on its own, and it must not add work to the admission path of
// Applications that have no addon component. Its ValidatingWebhookConfiguration
// entry carries a matchConditions expression so the API server only forwards
// Applications that actually contain a type: addon component, and the entry is
// installed only when the EnableAddonComponent feature gate is on.
package addon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/logging"
)

// ValidationWebhookPath is the route this handler is served on. It must match the
// clientConfig.service.path of the validating.core.oam.dev.v1beta1.addoncomponents
// entry in charts/vela-core/templates/admission-webhooks/validatingWebhookConfiguration.yaml.
const ValidationWebhookPath = "/validating-core-oam-dev-v1beta1-addon-components"

// ComponentType is the ComponentDefinition name used by the addon-as-component
// feature: an Application component of this type installs an addon.
const ComponentType = "addon"

var _ admission.Handler = &ValidatingHandler{}

// ValidatingHandler validates the addon-specific parts of an Application.
type ValidatingHandler struct {
	// Decoder decodes objects
	Decoder admission.Decoder
	// compatChecker is an injectable seam for the compatibility check. It returns
	// nil to allow and a non-nil *field.Error (carrying the mismatch detail) to
	// deny. When nil, defaultCompatChecker is used. Unit tests inject a fake to
	// stay hermetic, with no live registry.
	compatChecker func(ctx context.Context, addon, version, registry string) *field.Error
}

// componentProperties is the subset of a type: addon component's properties
// relevant to admission-time compatibility validation.
type componentProperties struct {
	Addon               string `json:"addon"`
	Version             string `json:"version"`
	Registry            string `json:"registry"`
	SkipVersionValidate bool   `json:"skipVersionValidate"`
}

// Handle validates the addon components of an Application.
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	startTime := time.Now()
	ctx = logging.WithRequestID(ctx, string(req.UID))
	logger := logging.NewHandlerLogger(ctx, req, "AddonComponentValidator")

	switch req.Operation {
	case admissionv1.Create, admissionv1.Update:
	default:
		return admission.ValidationResponse(true, "")
	}

	app := &v1beta1.Application{}
	if err := h.Decoder.Decode(req, app); err != nil {
		logger.WithStep("decode").WithError(err).Error(err, "Unable to decode admission request payload into Application object")
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode: %w (requestUID=%s)", err, req.UID))
	}

	if !app.ObjectMeta.DeletionTimestamp.IsZero() {
		return admission.ValidationResponse(true, "")
	}

	if errs := h.ValidateComponents(ctx, app); len(errs) > 0 {
		err := errs.ToAggregate()
		logger.WithStep("validate").WithError(err).Error(err, "Addon component validation failed",
			"errorCount", len(errs), "applicationName", app.Name)
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("%w (requestUID=%s)", err, req.UID))
	}

	logger.WithStep("complete").WithSuccess(true, startTime).Info("Addon component validation completed successfully",
		"applicationName", req.Name, "operation", req.Operation, "namespace", req.Namespace)
	return admission.ValidationResponse(true, "")
}

// ValidateComponents rejects type: addon components whose addon is incompatible
// with the current environment, meaning its vela/kubernetes SystemRequirements are
// not satisfied.
//
// It FAILS OPEN: any registry or resolve error (registry down, addon not found,
// timeout, discovery-client build failure) results in no denial and is only logged,
// so a registry outage never blocks Application applies. Admission is denied only
// on a concrete compatibility mismatch. The render-time check in pkg/addon/service
// remains the backstop for the fail-open case and for paths that bypass the webhook.
func (h *ValidatingHandler) ValidateComponents(ctx context.Context, app *v1beta1.Application) field.ErrorList {
	check := h.compatChecker
	if check == nil {
		check = h.defaultCompatChecker
	}

	var errs field.ErrorList
	for i, comp := range app.Spec.Components {
		if comp.Type != ComponentType {
			continue
		}

		props := componentProperties{}
		if comp.Properties != nil && len(comp.Properties.Raw) > 0 {
			if err := json.Unmarshal(comp.Properties.Raw, &props); err != nil {
				// Malformed properties are surfaced by CUE schema validation; skip the
				// compatibility check rather than deny for a decode error.
				klog.V(4).Infof("skip addon compatibility check for component %q: cannot decode properties: %v", comp.Name, err)
				continue
			}
		}

		if props.SkipVersionValidate {
			continue
		}

		addonName := props.Addon
		if addonName == "" {
			// Matches the ComponentDefinition default: the addon name falls back to
			// the component name when the addon field is empty.
			addonName = comp.Name
		}

		if fe := check(ctx, addonName, props.Version, props.Registry); fe != nil {
			errs = append(errs, field.Invalid(
				field.NewPath("spec", "components").Index(i).Child("properties"),
				comp.Name, fe.Detail))
		}
	}
	return errs
}

// RegisterValidatingHandler registers the addon component validator on the webhook
// server. Called from pkg/webhook/core.oam.dev only when the EnableAddonComponent
// feature gate is on, so with the gate off no addon code runs at admission at all.
func RegisterValidatingHandler(mgr manager.Manager) {
	server := mgr.GetWebhookServer()
	server.Register(ValidationWebhookPath, &webhook.Admission{Handler: &ValidatingHandler{
		Decoder: admission.NewDecoder(mgr.GetScheme()),
	}})
}
