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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

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
