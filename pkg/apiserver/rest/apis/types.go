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

package apis

import "github.com/oam-dev/kubevela/pkg/apiserver/proto/model"

// Action action type
type Action string

// ClusterType cluster type
type ClusterType struct {
	Name       string `json:"name"`
	Desc       string `json:"desc,omitempty"`
	UpdateAt   int64  `json:"updateAt,omitempty"`
	Kubeconfig string `json:"kubeconfig"`
}

// ClusterMeta cluster meta
type ClusterMeta struct {
	Cluster *model.Cluster `json:"cluster"`
}

// ClustersMeta cluster list meta
type ClustersMeta struct {
	Clusters []string `json:"clusters"`
}

// ClusterRequest cluster request
type ClusterRequest struct {
	ClusterType
	Method Action `json:"method"`
}

// ClusterVelaStatus status for whether install KubeVela
type ClusterVelaStatus struct {
	Installed bool `json:"installed"`
}
