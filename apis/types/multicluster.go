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
	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	"github.com/oam-dev/cluster-gateway/pkg/config"
)

const (
	// ClusterLocalName the name for the hub cluster
	ClusterLocalName = "local"

	// CredentialTypeInternal identifies the virtual cluster from internal kubevela system
	CredentialTypeInternal v1alpha1.CredentialType = "Internal"
	// CredentialTypeOCMManagedCluster identifies the virtual cluster from ocm
	CredentialTypeOCMManagedCluster v1alpha1.CredentialType = "ManagedCluster"
	// ClusterBlankEndpoint identifies the endpoint of a cluster as blank (not available)
	ClusterBlankEndpoint = "-"

	// ClustersArg indicates the argument for specific clusters to install addon
	ClustersArg = "clusters"
)

var (
	// AnnotationClusterVersion the annotation key for cluster version
	AnnotationClusterVersion = config.MetaApiGroupName + "/cluster-version"
)

// ClusterVersion defines the Version info of managed clusters.
type ClusterVersion struct {
	Major      string `json:"major"`
	Minor      string `json:"minor"`
	GitVersion string `json:"gitVersion,omitempty"`
	Platform   string `json:"platform,omitempty"`
}

// ControlPlaneClusterVersion will be the default value of cluster info if managed cluster version get error, it will have value when vela-core started.
var ControlPlaneClusterVersion ClusterVersion
