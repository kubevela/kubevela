/*
Copyright 2021 The KubeVela Authors.

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

package resourcekeeper

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var (
	// AllowCrossNamespaceResource indicates whether application can apply resources into other namespaces
	AllowCrossNamespaceResource = true
	// AllowResourceTypes if not empty, application can only apply resources with specified types
	AllowResourceTypes = ""
)

// AdmissionCheck check whether resources dispatch/deletion is admitted
func (h *resourceKeeper) AdmissionCheck(ctx context.Context, manifests []*unstructured.Unstructured) error {
	for _, handler := range []ResourceAdmissionHandler{
		&NamespaceAdmissionHandler{app: h.app},
		&ResourceTypeAdmissionHandler{},
	} {
		if err := handler.Validate(ctx, manifests); err != nil {
			return err
		}
	}
	return nil
}

// ResourceAdmissionHandler defines the handler to validate the admission of resource operation
type ResourceAdmissionHandler interface {
	Validate(ctx context.Context, manifests []*unstructured.Unstructured) error
}

// NamespaceAdmissionHandler defines the handler to validate if the resource namespace is valid to be dispatch/delete
type NamespaceAdmissionHandler struct {
	app *v1beta1.Application
}

// Validate check if cross namespace is available
func (h *NamespaceAdmissionHandler) Validate(ctx context.Context, manifests []*unstructured.Unstructured) error {
	if !AllowCrossNamespaceResource {
		for _, manifest := range manifests {
			if manifest.GetNamespace() != h.app.GetNamespace() {
				return errors.Errorf("forbidden resource: %s %s/%s is outside the namespace of application", manifest.GetKind(), manifest.GetNamespace(), manifest.GetName())
			}
		}
	}
	return nil
}

// ResourceTypeAdmissionHandler defines the handler to validate if the resource type is valid to be dispatch/delete
type ResourceTypeAdmissionHandler struct {
	initialized     bool
	isWhiteList     bool
	resourceTypeMap map[string]struct{}
}

// Validate check if resource type is valid
func (h *ResourceTypeAdmissionHandler) Validate(ctx context.Context, manifests []*unstructured.Unstructured) error {
	if AllowResourceTypes != "" {
		if !h.initialized {
			h.initialized = true
			h.resourceTypeMap = make(map[string]struct{})
			if strings.HasPrefix(AllowResourceTypes, "whitelist:") {
				for _, t := range strings.Split(strings.TrimPrefix(AllowResourceTypes, "whitelist:"), ",") {
					h.isWhiteList = true
					h.resourceTypeMap[t] = struct{}{}
				}
			}
			if strings.HasPrefix(AllowResourceTypes, "blacklist:") {
				for _, t := range strings.Split(strings.TrimPrefix(AllowResourceTypes, "blacklist:"), ",") {
					h.isWhiteList = false
					h.resourceTypeMap[t] = struct{}{}
				}
			}
		}
		for _, manifest := range manifests {
			gvk := manifest.GetObjectKind().GroupVersionKind()
			resourceType := fmt.Sprintf("%s.%s", manifest.GetKind(), gvk.Version)
			if gvk.Group != "" {
				resourceType += "." + gvk.Group
			}
			_, found := h.resourceTypeMap[resourceType]
			if h.isWhiteList != found {
				return errors.Errorf("forbidden resource: type (%s) of resource %s/%s is not allowed", manifest.GetKind(), manifest.GetNamespace(), manifest.GetName())
			}
		}
	}
	return nil
}
