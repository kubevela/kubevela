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
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// HTTPS https protocol name
	HTTPS = "https"
	// HTTP http protocol name
	HTTP = "http"
	// Mysql mysql protocol name
	Mysql = "mysql"
	// Redis redis protocol name
	Redis = "redis"
)

// ServiceEndpoint record the access endpoints of the application services
type ServiceEndpoint struct {
	Endpoint  Endpoint               `json:"endpoint"`
	Ref       corev1.ObjectReference `json:"ref"`
	Cluster   string                 `json:"cluster"`
	Component string                 `json:"component"`
}

// String return endpoint URL
func (s *ServiceEndpoint) String() string {
	if s.Endpoint.Host == "" && s.Endpoint.Port == 0 {
		return "-"
	}
	protocol := strings.ToLower(string(s.Endpoint.Protocol))
	if s.Endpoint.AppProtocol != nil && *s.Endpoint.AppProtocol != "" {
		protocol = *s.Endpoint.AppProtocol
	}
	path := s.Endpoint.Path
	if s.Endpoint.Path == "/" {
		path = ""
	}
	if (protocol == HTTPS && s.Endpoint.Port == 443) || (protocol == HTTP && s.Endpoint.Port == 80) {
		return fmt.Sprintf("%s://%s%s", protocol, s.Endpoint.Host, path)
	}
	if protocol == "tcp" {
		return fmt.Sprintf("%s:%d%s", s.Endpoint.Host, s.Endpoint.Port, path)
	}
	if s.Endpoint.Port == 0 {
		return fmt.Sprintf("%s://%s%s", protocol, s.Endpoint.Host, path)
	}
	return fmt.Sprintf("%s://%s:%d%s", protocol, s.Endpoint.Host, s.Endpoint.Port, path)
}

// Endpoint create by ingress or service
type Endpoint struct {
	// The protocol for this endpoint. Supports "TCP", "UDP", and "SCTP".
	// Default is TCP.
	// +default="TCP"
	// +optional
	Protocol corev1.Protocol `json:"protocol,omitempty"`

	// The protocol for this endpoint.
	// Un-prefixed names are reserved for IANA standard service names (as per
	// RFC-6335 and http://www.iana.org/assignments/service-names).
	// +optional
	AppProtocol *string `json:"appProtocol,omitempty"`

	// the host for the endpoint, it could be IP or domain
	Host string `json:"host"`

	// the port for the endpoint
	// Default is 80.
	Port int `json:"port"`

	// +optional
	// the name of the port
	PortName string `json:"portName,omitempty"`

	// the path for the endpoint
	Path string `json:"path,omitempty"`

	// Inner means the endpoint is only accessible within the cluster.
	Inner bool `json:"inner,omitempty"`
}

// AppliedResource resource metadata
type AppliedResource struct {
	Cluster         string            `json:"cluster"`
	Component       string            `json:"component"`
	Trait           string            `json:"trait"`
	Kind            string            `json:"kind"`
	Namespace       string            `json:"namespace,omitempty"`
	Name            string            `json:"name,omitempty"`
	UID             types.UID         `json:"uid,omitempty"`
	APIVersion      string            `json:"apiVersion,omitempty"`
	ResourceVersion string            `json:"resourceVersion,omitempty"`
	DeployVersion   string            `json:"deployVersion,omitempty"`
	PublishVersion  string            `json:"publishVersion,omitempty"`
	Revision        string            `json:"revision,omitempty"`
	Latest          bool              `json:"latest"`
	ResourceTree    *ResourceTreeNode `json:"resourceTree,omitempty"`
}

// ResourceTreeNode is the tree node of every resource
type ResourceTreeNode struct {
	Cluster           string                    `json:"cluster"`
	APIVersion        string                    `json:"apiVersion,omitempty"`
	Kind              string                    `json:"kind"`
	Namespace         string                    `json:"namespace,omitempty"`
	Name              string                    `json:"name,omitempty"`
	UID               types.UID                 `json:"uid,omitempty"`
	HealthStatus      HealthStatus              `json:"healthStatus,omitempty"`
	DeletionTimestamp time.Time                 `json:"deletionTimestamp,omitempty"`
	CreationTimestamp time.Time                 `json:"creationTimestamp,omitempty"`
	LeafNodes         []*ResourceTreeNode       `json:"leafNodes,omitempty"`
	AdditionalInfo    map[string]interface{}    `json:"additionalInfo,omitempty"`
	Object            unstructured.Unstructured `json:"-"`
}

// GroupVersionKind returns the stored group, version, and kind from AppliedResource
func (obj *AppliedResource) GroupVersionKind() schema.GroupVersionKind {
	return schema.FromAPIVersionAndKind(obj.APIVersion, obj.Kind)
}

// GroupVersionKind returns the stored group, version, and kind from ResourceTreeNode
func (rtn *ResourceTreeNode) GroupVersionKind() schema.GroupVersionKind {
	return schema.FromAPIVersionAndKind(rtn.APIVersion, rtn.Kind)
}

// ResourceItem the resource base info struct
type ResourceItem struct {
	Cluster        string                    `json:"cluster"`
	Workload       Workload                  `json:"workload"`
	Component      string                    `json:"component"`
	Object         unstructured.Unstructured `json:"object"`
	PublishVersion string                    `json:"publishVersion"`
	DeployVersion  string                    `json:"deployVersion"`
}

// Workload workload resource base info
type Workload struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}

// PodBase the struct of pod list
type PodBase struct {
	Cluster   string   `json:"cluster"`
	Workload  Workload `json:"workload"`
	Component string   `json:"component"`
	Metadata  struct {
		CreationTime string `json:"creationTime"`
		Name         string `json:"name"`
		Namespace    string `json:"namespace"`
		Version      struct {
			PublishVersion string `json:"publishVersion"`
			DeployVersion  string `json:"deployVersion"`
		} `json:"version"`
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
	Status struct {
		HostIP   string `json:"hostIP"`
		NodeName string `json:"nodeName"`
		Phase    string `json:"phase"`
		PodIP    string `json:"podIP"`
	} `json:"status"`
}
