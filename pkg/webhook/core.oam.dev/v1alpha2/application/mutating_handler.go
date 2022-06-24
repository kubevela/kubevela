/*
Copyright 2022 The KubeVela Authors.

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

package application

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils"
)

// MutatingHandler adding user info to application annotations
type MutatingHandler struct {
	skipUsers []string
	Decoder   *admission.Decoder
}

var _ admission.Handler = &MutatingHandler{}

// Handle mutate application
func (h *MutatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.AuthenticateApplication) {
		return admission.Patched("")
	}

	if slices.Contains(h.skipUsers, req.UserInfo.Username) {
		return admission.Patched("")
	}

	app := &v1beta1.Application{}
	if err := h.Decoder.Decode(req, app); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if metav1.HasAnnotation(app.ObjectMeta, oam.AnnotationApplicationServiceAccountName) {
		return admission.Errored(http.StatusBadRequest, errors.New("service-account annotation is not permitted when authentication enabled"))
	}
	klog.Infof("[ApplicationMutatingHandler] Setting UserInfo into Application, UserInfo: %v, Application: %s/%s", req.UserInfo, app.GetNamespace(), app.GetName())
	auth.SetUserInfoInAnnotation(&app.ObjectMeta, req.UserInfo)

	bs, err := json.Marshal(app)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, bs)
}

var _ admission.DecoderInjector = &MutatingHandler{}

// InjectDecoder .
func (h *MutatingHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}

// RegisterMutatingHandler will register component mutation handler to the webhook
func RegisterMutatingHandler(mgr manager.Manager) {
	server := mgr.GetWebhookServer()
	handler := &MutatingHandler{}
	if userInfo := utils.GetUserInfoFromConfig(mgr.GetConfig()); userInfo != nil {
		klog.Infof("[ApplicationMutatingHandler] add skip user %s", userInfo.Username)
		handler.skipUsers = []string{userInfo.Username}
	}
	server.Register("/mutating-core-oam-dev-v1beta1-applications", &webhook.Admission{Handler: handler})
}
