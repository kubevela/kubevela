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

// Package test copied and modified from https://github.com/kubevela/pkg/blob/main/cue/cuex/providers/kube/kube.go.
package test

import (
	"context"
	_ "embed"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ResourceVars .
type ResourceVars struct {
	// +usage=The cluster to use
	Cluster string `json:"cluster"`
	// +usage=The resource to get or apply
	Resource *unstructured.Unstructured `json:"resource"`
	// +usage=The options to get or apply
	Options ApplyOptions `json:"options"`
}

// ApplyOptions .
type ApplyOptions struct {
	// +usage=The strategy of the resource
	ThreeWayMergePatch ThreeWayMergePatchOptions `json:"threeWayMergePatch"`
}

// ThreeWayMergePatchOptions .
type ThreeWayMergePatchOptions struct {
	// +usage=The strategy to get or apply the resource
	Enabled bool `json:"enabled" cue:"default:true"`
	// +usage=The annotation prefix to use for the three way merge patch
	AnnotationPrefix string `json:"annotationPrefix" cue:"default:resource"`
}

// ResourceParams is the params for resource
type ResourceParams providers.Params[ResourceVars]

// ResourceReturns is the returns for resource
type ResourceReturns providers.Returns[*unstructured.Unstructured]

// Apply .
func Apply(_ context.Context, _ *ResourceParams) (*ResourceReturns, error) {
	return nil, nil
}

// Get .
func Get(_ context.Context, _ *ResourceParams) (*ResourceReturns, error) {
	return nil, nil
}

// ListFilter filter for list resources
type ListFilter struct {
	// +usage=The namespace to list the resources
	Namespace string `json:"namespace,omitempty"`
	// +usage=The label selector to filter the resources
	MatchingLabels map[string]string `json:"matchingLabels,omitempty"`
}

// ListVars is the vars for list
type ListVars struct {
	// +usage=The cluster to use
	Cluster string `json:"cluster"`
	// +usage=The filter to list the resources
	Filter *ListFilter `json:"filter,omitempty"`
	// +usage=The resource to list
	Resource *unstructured.Unstructured `json:"resource"`
}

// ListParams is the params for list
type ListParams providers.Params[ListVars]

// ListReturns is the returns for list
type ListReturns providers.Returns[*unstructured.UnstructuredList]

// List .
func List(_ context.Context, _ *ListParams) (*ListReturns, error) {
	return nil, nil
}

// PatchVars is the vars for patch
type PatchVars struct {
	// +usage=The cluster to use
	Cluster string `json:"cluster"`
	// +usage=The resource to patch
	Resource *unstructured.Unstructured `json:"resource"`
	// +usage=The patch to be applied to the resource with kubernetes patch
	Patch Patcher `json:"patch"`
}

// Patcher is the patcher
type Patcher struct {
	// +usage=The type of patch being provided
	Type string `json:"type" cue:"enum:merge,json,strategic;default:merge"`
	Data any    `json:"data"`
}

// PatchParams is the params for patch
type PatchParams providers.Params[PatchVars]

// Patch patches a kubernetes resource with patch strategy
func Patch(_ context.Context, _ *PatchParams) (*ResourceReturns, error) {
	return nil, nil
}

// ProviderName .
const ProviderName = "kube"

// Package .
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, "", map[string]cuexruntime.ProviderFn{
	"apply": cuexruntime.GenericProviderFn[ResourceParams, ResourceReturns](Apply),
	"get":   cuexruntime.GenericProviderFn[ResourceParams, ResourceReturns](Get),
	"list":  cuexruntime.GenericProviderFn[ListParams, ListReturns](List),
	"patch": cuexruntime.GenericProviderFn[PatchParams, ResourceReturns](Patch),
}))
