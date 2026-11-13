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

package application

import (
	"context"

	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// fromController reports a request made with vela-core's own identity. That proves the object
// came through dispatch, not on whose behalf: the controller writes for every tenant, so a
// decision that trusts such a write also needs dispatchingNamespace.
func (h *ValidatingHandler) fromController(req admission.Request) bool {
	return h.controllerUsername != "" && req.UserInfo.Username == h.controllerUsername
}

// dispatchingNamespace is the namespace of the Application that dispatched obj, from the labels
// the controller stamps on everything it dispatches (overwriting any the manifest carried), or ""
// when obj records none.
func dispatchingNamespace(obj metav1.Object) string {
	labels := obj.GetLabels()
	if labels[oam.LabelAppName] == "" {
		return ""
	}
	return labels[oam.LabelAppNamespace]
}

// dispatcherAuthor is who wrote the Application that dispatched obj, as the identity recorded on
// it. It is trusted only when the mutating webhook stamps that identity from the request, that is
// with AuthenticateApplication on and the username stamped (--authentication-with-user); otherwise
// the recorded username is the Application author's own claim, and false says permissions cannot
// be checked for it.
func (h *ValidatingHandler) dispatcherAuthor(ctx context.Context, obj metav1.Object) (authenticationv1.UserInfo, bool, error) {
	labels := obj.GetLabels()
	name, namespace := labels[oam.LabelAppName], labels[oam.LabelAppNamespace]
	if name == "" || namespace == "" ||
		!utilfeature.DefaultMutableFeatureGate.Enabled(features.AuthenticateApplication) || !auth.AuthenticationWithUser {
		return authenticationv1.UserInfo{}, false, nil
	}
	app := &v1beta1.Application{}
	if err := h.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, app); err != nil {
		if errors.IsNotFound(err) {
			return authenticationv1.UserInfo{}, false, nil
		}
		return authenticationv1.UserInfo{}, false, err
	}
	author := auth.GetUserInfoInAnnotation(&app.ObjectMeta)
	return authenticationv1.UserInfo{Username: author.GetName(), Groups: author.GetGroups()}, author.GetName() != "", nil
}
