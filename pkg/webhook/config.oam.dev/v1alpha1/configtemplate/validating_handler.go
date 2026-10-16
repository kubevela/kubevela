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
)

var configTemplateGVR = configv1alpha1.ConfigTemplateGVR

// ValidatingHandler validates ConfigTemplate resources.
type ValidatingHandler struct {
	Decoder admission.Decoder
	// Client runs the SubjectAccessReview the endpoint rule needs.
	Client client.Client
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

	// A system template lives in vela-system (issue #7410 section 6). CEL cannot say so, it
	// cannot read metadata.namespace; this is the one admission-time place. The CLI refuses
	// the same template before it writes, with the same words, and the ConfigTemplate
	// controller marks one that got through with the webhooks off.
	if legacyconfig.SystemScopeMisplaced(obj.Namespace, obj.Spec.Scope) {
		return admission.Denied(legacyconfig.SystemScopePlacementError(obj.Namespace, obj.Name).Error())
	}

	// One parse of the template serves both checks on its nacos block. A template that
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

	// The nacos block names the endpoint Secret the controller publishes with, read with the
	// controller's permissions. One in the template's own namespace or in vela-system (the
	// default, the shared endpoints) is free; one anywhere else only if the requesting user
	// could read that Secret themselves (Brian, #7457). Admission is the only place with a
	// user; a template admitted before this rule is not re-validated, so an update is reviewed
	// only when it changes the template text, as the user making that edit (a label edit is
	// not a new claim on the endpoint).
	if endpoint := foreignNacosEndpoint(template, obj.Namespace); endpoint != nil && h.templateTextChanged(req, obj) {
		allowed, err := auth.UserCan(ctx, h.Client, req.UserInfo, authorizationv1.ResourceAttributes{
			Verb: "get", Group: "", Version: "v1", Resource: "secrets", Namespace: endpoint.Namespace, Name: endpoint.Name,
		})
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		if !allowed {
			return admission.Denied(fmt.Sprintf("config template %s/%s names the nacos endpoint %s/%s in another namespace, which %s may not read; grant get on that Secret, or name an endpoint in %s or in %s",
				obj.Namespace, obj.Name, endpoint.Namespace, endpoint.Name, req.UserInfo.Username, obj.Namespace, types.DefaultKubeVelaNS))
		}
	}
	return admission.ValidationResponse(true, "")
}

// templateTextChanged is true on create, and on an update whose spec.template differs from the
// stored one. An update whose old object cannot be read counts as changed: the review runs
// rather than being skipped on a decode failure.
func (h *ValidatingHandler) templateTextChanged(req admission.Request, obj *configv1alpha1.ConfigTemplate) bool {
	if req.Operation != admissionv1.Update {
		return true
	}
	old := &configv1alpha1.ConfigTemplate{}
	if err := h.Decoder.DecodeRaw(req.OldObject, old); err != nil {
		return true
	}
	return old.Spec.Template != obj.Spec.Template
}

// foreignNacosEndpoint is the endpoint the template's nacos block names when it lies outside
// both the template's namespace and vela-system (ParseExpandedWriterConfig applies the
// vela-system default); nil without a block, and nil for an endpoint that is free to use.
func foreignNacosEndpoint(template cue.Value, namespace string) *writer.ConfigRef {
	cfg := writer.ParseExpandedWriterConfig(template)
	if cfg.Nacos == nil || cfg.Nacos.Endpoint.Name == "" {
		return nil
	}
	if endpoint := cfg.Nacos.Endpoint; endpoint.Namespace != namespace && endpoint.Namespace != types.DefaultKubeVelaNS {
		return &endpoint
	}
	return nil
}

// RegisterValidatingHandler registers ConfigTemplate validation to the webhook server.
func RegisterValidatingHandler(mgr manager.Manager) {
	server := mgr.GetWebhookServer()
	server.Register("/validating-config-oam-dev-v1alpha1-configtemplates", &webhook.Admission{Handler: &ValidatingHandler{
		Decoder: admission.NewDecoder(mgr.GetScheme()),
		Client:  mgr.GetClient(),
	}})
}
