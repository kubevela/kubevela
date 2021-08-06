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
