/*
Copyright 2023 The KubeVela Authors.

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

package definition

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/kubevela/kubevela/apis/core.oam.dev/v1beta1"
)

// VersionValidator validates definition versions for immutability
type VersionValidator struct {
	Client  client.Client
	Decoder *admission.Decoder
}

// Handle implements admission.Handler interface
func (v *VersionValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	// Only handle UPDATE operations
	if req.Operation != admissionv1.Update {
		return admission.Allowed("")
	}

	// Get the old object
	oldObj := &unstructured.Unstructured{}
	if err := json.Unmarshal(req.OldObject.Raw, oldObj); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("unmarshal old object failed: %w", err))
	}

	// Get the new object
	newObj := &unstructured.Unstructured{}
	if err := json.Unmarshal(req.Object.Raw, newObj); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("unmarshal new object failed: %w", err))
	}

	// Check if this is a definition with version
	oldVersion, oldFound, err := unstructured.NestedString(oldObj.Object, "spec", "version")
	if err != nil || !oldFound || oldVersion == "" {
		// No version to check, allow the update
		return admission.Allowed("")
	}

	newVersion, newFound, err := unstructured.NestedString(newObj.Object, "spec", "version")
	if err != nil || !newFound {
		// New object doesn't have version field, strange but allow
		return admission.Allowed("")
	}

	// If versions match, block the update
	if oldVersion == newVersion {
		return admission.Denied(fmt.Sprintf("Definition with version %s is immutable and cannot be updated. Please create a new version instead.", oldVersion))
	}

	// Versions are different, allow the update
	return admission.Allowed("")
}

// InjectClient injects the client
func (v *VersionValidator) InjectClient(c client.Client) error {
	v.Client = c
	return nil
}

// InjectDecoder injects the decoder
func (v *VersionValidator) InjectDecoder(d *admission.Decoder) error {
	v.Decoder = d
	return nil
}

// RegisterValidatingWebhook registers the validating webhook
func RegisterValidatingWebhook(mgr client.Manager) error {
	server := mgr.GetWebhookServer()
	server.Register("/validate-definition-version", &admission.Webhook{
		Handler: &VersionValidator{
			Client: mgr.GetClient(),
		},
	})
	return nil
}
