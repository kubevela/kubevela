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

package configtemplate

import (
	"context"
	"fmt"
	"net/http"

	"cuelang.org/go/cue"
	admissionv1 "k8s.io/api/admission/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/auth"
	legacyconfig "github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/config/writer"
	"github.com/oam-dev/kubevela/pkg/cue/script"
	"github.com/oam-dev/kubevela/pkg/utils"
)

var configTemplateGVR = configv1alpha1.ConfigTemplateGVR

// ValidatingHandler validates ConfigTemplate resources.
type ValidatingHandler struct {
	Decoder admission.Decoder
	// Client runs the SubjectAccessReview the endpoint rule needs.
	Client client.Client
	// ControllerUser is the controller's own identity. Its requests (the startup sweep's
	// adoption of legacy templates) have no user behind them to ask about an endpoint.
	ControllerUser string
}

var _ admission.Handler = &ValidatingHandler{}

func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Resource.String() != configTemplateGVR.String() {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("expect resource to be %s", configTemplateGVR))
	}
	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
		return admission.ValidationResponse(true, "")
	}

	obj := &configv1alpha1.ConfigTemplate{}
	if err := h.Decoder.Decode(req, obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	// An object being deleted only loses finalizers: refusing that would leave it Terminating.
	if obj.DeletionTimestamp != nil {
		return admission.ValidationResponse(true, "")
	}
	// The stored object, on an update. One that cannot be read leaves old nil, and the
	// update is judged as strictly as a create.
	var old *configv1alpha1.ConfigTemplate
	if req.Operation == admissionv1.Update && len(req.OldObject.Raw) > 0 {
		old = &configv1alpha1.ConfigTemplate{}
		if err := h.Decoder.DecodeRaw(req.OldObject, old); err != nil {
			old = nil
		}
	}

	// A system template lives in vela-system (issue #7410 section 6). CEL cannot say so, it
	// cannot read metadata.namespace; this is the one admission-time place. The CLI refuses
	// the same template before it writes, with the same words, and the ConfigTemplate
	// controller marks one that got through with the webhooks off. A template already
	// misplaced may still be edited, relabelled or fixed: its controller reports it, and every
	// reader treats it as namespace-scoped.
	if legacyconfig.SystemScopeMisplaced(obj.Namespace, obj.Spec.Scope) && (old == nil || !legacyconfig.SystemScopeMisplaced(old.Namespace, old.Spec.Scope)) {
		return admission.Denied(legacyconfig.SystemScopePlacementError(obj.Namespace, obj.Name).Error())
	}

	// One parse of the template serves every check on its nacos block. A template that
	// does not compile is admitted, as before: the ConfigTemplate controller reports that
	// through status.phase=Error.
	v, err := script.CUE(obj.Spec.Template).ParseToValueWithCueX(ctx)
	if err != nil {
		return admission.ValidationResponse(true, "")
	}
	template := v.LookupPath(cue.ParsePath("template"))

	// The shape of the nacos block is admission's question, so the mistake shows at apply
	// time rather than on the first Config made from the template. Whether the endpoint
	// config exists is reconcile-time, reported through the Config's NacosSynced condition.
	// The Config reconciler repeats this check (writer.ValidateTemplate), so a cluster
	// running without the webhooks gets the message on the Config instead.
	if err := writer.ValidateExpandedWriter(template); err != nil {
		return admission.Denied(err.Error())
	}

	// The endpoint rule judges template text, so it runs when the text is new: on create, or
	// on an update that changes it. A template admitted before the rule is not re-validated by
	// a label edit; the Config reconciler checks what such a template renders.
	if old != nil && old.Spec.Template == obj.Spec.Template {
		return admission.ValidationResponse(true, "")
	}
	if err := writer.ValidateEndpointNamespace(template); err != nil {
		return admission.Denied(err.Error())
	}
	// The nacos block names the endpoint Secret the controller publishes with, read with the
	// controller's permissions. One in the template's own namespace or in vela-system (the
	// default, the shared endpoints) is free; one anywhere else only if the requesting user may
	// read that Secret. The controller's own request has no user to ask, so for it a foreign
	// endpoint is a refusal.
	endpoint := foreignNacosEndpoint(template, obj.Namespace)
	if endpoint == nil {
		return admission.ValidationResponse(true, "")
	}
	denied := fmt.Sprintf("config template %s/%s names the nacos endpoint %s/%s in another namespace, which %s may not read; grant get on that Secret, or name an endpoint in %s or in %s",
		obj.Namespace, obj.Name, endpoint.Namespace, endpoint.Name, req.UserInfo.Username, obj.Namespace, types.DefaultKubeVelaNS)
	if h.ControllerUser != "" && req.UserInfo.Username == h.ControllerUser {
		return admission.Denied(denied + "; the controller writes with no user to ask, so apply this template as a user who may")
	}
	allowed, err := auth.UserCan(ctx, h.Client, req.UserInfo, authorizationv1.ResourceAttributes{
		Verb: "get", Group: "", Version: "v1", Resource: "secrets", Namespace: endpoint.Namespace, Name: endpoint.Name,
	})
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if !allowed {
		return admission.Denied(denied)
	}
	return admission.ValidationResponse(true, "")
}

// foreignNacosEndpoint is the endpoint the template's nacos block names when it lies outside
// both the template's namespace and vela-system (ParseExpandedWriterConfig applies the
// vela-system default); nil without a block, and nil for an endpoint that is free to use.
func foreignNacosEndpoint(template cue.Value, namespace string) *writer.ConfigRef {
	cfg := writer.ParseExpandedWriterConfig(template)
	if cfg.Nacos == nil || cfg.Nacos.Endpoint.Name == "" {
		return nil
	}
	if endpoint := cfg.Nacos.Endpoint; !legacyconfig.EndpointNamespaceFree(namespace, endpoint.Namespace) {
		return &endpoint
	}
	return nil
}

// RegisterValidatingHandler registers ConfigTemplate validation to the webhook server.
func RegisterValidatingHandler(mgr manager.Manager) {
	server := mgr.GetWebhookServer()
	handler := &ValidatingHandler{
		Decoder: admission.NewDecoder(mgr.GetScheme()),
		Client:  mgr.GetClient(),
	}
	if userInfo := utils.GetUserInfoFromConfig(mgr.GetConfig()); userInfo != nil {
		handler.ControllerUser = userInfo.Username
	}
	server.Register("/validating-config-oam-dev-v1alpha1-configtemplates", &webhook.Admission{Handler: handler})
}
