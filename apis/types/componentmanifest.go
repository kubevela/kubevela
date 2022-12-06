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

package types

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ComponentManifest contains resources rendered from an application component.
type ComponentManifest struct {
	Name             string
	Namespace        string
	RevisionName     string
	RevisionHash     string
	ExternalRevision string
	// StandardWorkload contains K8s resource generated from "output" block of ComponentDefinition
	StandardWorkload *unstructured.Unstructured
	// Traits contains both resources generated from "outputs" block of ComponentDefinition and resources generated from TraitDefinition
	Traits []*unstructured.Unstructured
	Scopes []*corev1.ObjectReference

	// PackagedWorkloadResources contain all the workload related resources. It could be a Helm
	// Release, Git Repo or anything that can package and run a workload.
	PackagedWorkloadResources []*unstructured.Unstructured
	PackagedTraitResources    map[string][]*unstructured.Unstructured
}
