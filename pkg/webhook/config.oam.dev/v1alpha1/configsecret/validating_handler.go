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

// Package configsecret guards the Secret of a config adopted by a Config.
package configsecret

import (
	"context"
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/types"
)

// ValidatingHandler refuses an update that takes a config Secret back from the Config that
// adopted it. Clients older than 1.12 write the legacy Secret whole, without the migration
// mark or the Config's owner reference, so the Config would go on holding the old value.
type ValidatingHandler struct {
	Decoder admission.Decoder
}

var _ admission.Handler = &ValidatingHandler{}

// Handle allows every write but an update dropping the migrated-to annotation, or the catalog
// label the webhook selects on, which would let a later update drop the annotation unseen. The
// garbage collector orphaning the Secret drops only the owner reference, and is allowed.
func (h *ValidatingHandler) Handle(_ context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Update {
		return admission.Allowed("")
	}
	oldObj, newObj := &corev1.Secret{}, &corev1.Secret{}
	if err := h.Decoder.DecodeRaw(req.OldObject, oldObj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if err := h.Decoder.DecodeRaw(req.Object, newObj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	keepsMark := newObj.Annotations[types.AnnotationConfigMigratedTo] != "" &&
		newObj.Labels[types.LabelConfigCatalog] == oldObj.Labels[types.LabelConfigCatalog]
	if oldObj.Annotations[types.AnnotationConfigMigratedTo] == "" || keepsMark {
		return admission.Allowed("")
	}
	holder := oldObj.Name
	for _, ref := range oldObj.OwnerReferences {
		if ref.Kind == "Config" && ref.Controller != nil && *ref.Controller {
			holder = ref.Name
		}
	}
	return admission.Denied(fmt.Sprintf("config %s/%s is held by the Config %s/%s since its migration; "+
		"edit it with the vela CLI or VelaUX from 1.12 on, or edit the Config. "+
		"An older client rewrites the Secret and leaves the Config behind",
		oldObj.Namespace, oldObj.Name, oldObj.Namespace, holder))
}

// RegisterValidatingHandler registers the handler with the manager's webhook server.
func RegisterValidatingHandler(mgr manager.Manager) {
	mgr.GetWebhookServer().Register("/validating-config-oam-dev-v1alpha1-configsecrets",
		&webhook.Admission{Handler: &ValidatingHandler{Decoder: admission.NewDecoder(mgr.GetScheme())}})
}
