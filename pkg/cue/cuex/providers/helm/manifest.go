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

package helm

import (
	stderrors "errors"
	"io"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/kubevela/pkg/util/singleton"
)

// parseManifestResources parses a Helm release manifest string into a slice of
// resource maps, skipping test hooks when requested and ordering CRDs first.
// Resources whose `metadata.namespace` is empty get defaulted to
// releaseNamespace unless their kind is cluster-scoped. Upstream Helm charts
// commonly omit metadata.namespace and rely on the helm install --namespace
// flag for placement; KubeVela's resource tracker re-applies these outputs
// independently and would otherwise default them to vela-system, creating
// shadow copies and tripping helm's ownership annotation guard on the next
// release. Defaulting at parse time keeps every output keyed to the correct
// namespace from the start.
func (p *Provider) parseManifestResources(manifestStr string, options *RenderOptionsParams, releaseNamespace string) ([]map[string]interface{}, error) {
	skipTests := true
	if options != nil && options.SkipTests != nil {
		skipTests = *options.SkipTests
	}

	resources := []map[string]interface{}{}
	decoder := kyaml.NewYAMLOrJSONDecoder(strings.NewReader(manifestStr), 4096)

	for {
		resource := &unstructured.Unstructured{}
		if err := decoder.Decode(&resource); err != nil {
			if stderrors.Is(err, io.EOF) {
				break
			}
			return nil, errors.Wrap(err, "failed to decode manifest")
		}

		// Skip empty resources
		if resource == nil || len(resource.Object) == 0 {
			continue
		}

		// Skip test resources if requested
		if skipTests && isTestResource(resource) {
			continue
		}

		// Default the namespace for namespaced resources whose template
		// omitted metadata.namespace. Cluster-scoped kinds (CRDs,
		// ClusterRoles, Namespaces, ...) are left as-is so the API server
		// does not reject them.
		if releaseNamespace != "" && resource.GetNamespace() == "" && !isClusterScopedGVK(resource.GroupVersionKind()) {
			resource.SetNamespace(releaseNamespace)
		}

		cleanedResource := cleanResource(resource.Object)
		resources = append(resources, cleanedResource)
	}

	// Order resources: CRDs first, then namespaces, then other resources
	return orderResources(resources), nil
}

// isClusterScopedGVK reports whether the given GroupVersionKind denotes a
// Kubernetes resource that lives at the cluster scope (no namespace).
//
// Resolution order:
//
//  1. Ask the cluster's RESTMapper. This sees built-in kinds AND third-party
//     CRDs (cert-manager's ClusterIssuer, Knative's ClusterIngress, etc.),
//     so the namespace-default logic doesn't mis-namespace custom
//     cluster-scoped resources.
//  2. If the RESTMapper is unavailable, or doesn't know the GVK (e.g.,
//     because the chart manifest itself defines a CRD whose kind hasn't
//     been registered with the API server yet), fall back to a static
//     allowlist of well-known cluster-scoped kinds.
//
// The fallback is intentionally conservative: an unrecognized kind is
// treated as namespaced, so a new namespaced custom resource gets the
// safe default (release namespace) rather than landing in vela-system.
func isClusterScopedGVK(gvk schema.GroupVersionKind) bool {
	if mapper := singleton.RESTMapper.Get(); mapper != nil {
		if mapping, mErr := mapper.RESTMapping(gvk.GroupKind(), gvk.Version); mErr == nil && mapping != nil {
			return mapping.Scope.Name() == meta.RESTScopeNameRoot
		}
	}
	return isClusterScopedKindStaticFallback(gvk.Kind)
}

// isClusterScopedKindStaticFallback returns true for the well-known set of
// built-in cluster-scoped kinds. Used only when the RESTMapper cannot answer
// authoritatively. New entries should be limited to stable upstream APIs;
// for third-party CRDs the RESTMapper path is the source of truth.
func isClusterScopedKindStaticFallback(kind string) bool {
	switch kind {
	case "CustomResourceDefinition",
		"Namespace",
		"ClusterRole",
		"ClusterRoleBinding",
		"PersistentVolume",
		"StorageClass",
		"VolumeAttachment",
		"CSIDriver",
		"CSINode",
		"PriorityClass",
		"RuntimeClass",
		"IngressClass",
		"MutatingWebhookConfiguration",
		"ValidatingWebhookConfiguration",
		"APIService",
		"FlowSchema",
		"PriorityLevelConfiguration",
		"Node",
		"ComponentStatus":
		return true
	}
	return false
}

// isTestResource checks if a resource is a test resource
func isTestResource(resource *unstructured.Unstructured) bool {
	annotations := resource.GetAnnotations()
	if annotations != nil {
		if hookType, exists := annotations["helm.sh/hook"]; exists {
			return strings.Contains(hookType, "test")
		}
	}
	return false
}

// cleanResource removes any nil values from a resource map
func cleanResource(resource map[string]interface{}) map[string]interface{} {
	cleaned := make(map[string]interface{})
	for k, v := range resource {
		if v != nil {
			switch val := v.(type) {
			case map[string]interface{}:
				// Recursively clean nested maps
				cleanedMap := cleanResource(val)
				if len(cleanedMap) > 0 {
					cleaned[k] = cleanedMap
				}
			case []interface{}:
				// Clean arrays
				cleanedArray := make([]interface{}, 0)
				for _, item := range val {
					if item != nil {
						if m, ok := item.(map[string]interface{}); ok {
							cleanedArray = append(cleanedArray, cleanResource(m))
						} else {
							cleanedArray = append(cleanedArray, item)
						}
					}
				}
				if len(cleanedArray) > 0 {
					cleaned[k] = cleanedArray
				}
			default:
				// Keep non-nil values
				cleaned[k] = v
			}
		}
	}
	return cleaned
}

// orderResources orders resources with CRDs first, then namespaces, then others
func orderResources(resources []map[string]interface{}) []map[string]interface{} {
	var crds, namespaces, others []map[string]interface{}

	for _, r := range resources {
		kind, _, _ := unstructured.NestedString(r, "kind")
		switch kind {
		case "CustomResourceDefinition":
			crds = append(crds, r)
		case "Namespace":
			namespaces = append(namespaces, r)
		default:
			others = append(others, r)
		}
	}

	// Combine in order
	result := make([]map[string]interface{}, 0, len(resources))
	result = append(result, crds...)
	result = append(result, namespaces...)
	result = append(result, others...)

	return result
}
