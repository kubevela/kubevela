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

// Package apis contains typed structs from fluxcd/helm-controller and fluxcd/source-controller.
// Because we cannot solve dependency inconsistencies between KubeVela and fluxcd/gotk,
// so we pick up those APIs used in KubeVela to install helm resources.
package apis

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// HelmRepositoryKind is the kind name of fluxcd/helmrepository
	HelmRepositoryKind = "HelmRepository"
)

// HelmSpec includes information to install a Helm chart
type HelmSpec struct {
	HelmReleaseSpec    `json:"release"`
	HelmRepositorySpec `json:"repository"`
}

var (
	// HelmReleaseGVK refers to GVK of fluxcd/helmrelease
	HelmReleaseGVK = schema.GroupVersionKind{
		Group:   "helm.toolkit.fluxcd.io",
		Version: "v2beta1",
		Kind:    "HelmRelease",
	}

	// HelmRepositoryGVK refers to GVK of fluxcd/helmrepository
	HelmRepositoryGVK = schema.GroupVersionKind{
		Group:   "source.toolkit.fluxcd.io",
		Version: "v1beta1",
		Kind:    "HelmRepository",
	}

	// HelmChartNamePath indicates the field to path in HelmRelease to get the chart name
	HelmChartNamePath = []string{"spec", "chart", "spec", "chart"}
)
