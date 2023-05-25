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

package mock

import (
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// NewClient new client with given mappings
func NewClient(c client.Client, mappings map[schema.GroupVersionResource][]schema.GroupVersionKind) client.Client {
	if c == nil {
		c = fake.NewClientBuilder().Build()
	}
	return &Client{Client: c, mappings: mappings}
}

// Client fake client
type Client struct {
	client.Client
	mappings map[schema.GroupVersionResource][]schema.GroupVersionKind
}

// RESTMapper override default mapper
func (in *Client) RESTMapper() meta.RESTMapper {
	mapper := in.Client.RESTMapper()
	if mapper == nil {
		mapper = fake.NewClientBuilder().Build().RESTMapper()
	}
	return &RESTMapper{RESTMapper: mapper, mappings: in.mappings}
}

// RESTMapper test mapper
type RESTMapper struct {
	meta.RESTMapper
	mappings map[schema.GroupVersionResource][]schema.GroupVersionKind
}

// KindsFor get kinds
func (in *RESTMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	if gvks, found := in.mappings[resource]; found {
		return gvks, nil
	}
	return in.RESTMapper.KindsFor(resource)
}

// RESTMapping get mapping
func (in *RESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	version := "v1"
	if len(versions) > 0 {
		version = versions[0]
	}
	return &meta.RESTMapping{
		Resource:         schema.GroupVersionResource{Group: gk.Group, Version: versions[0], Resource: strings.ToLower(gk.Kind) + "s"},
		GroupVersionKind: gk.WithVersion(version),
		Scope:            scope(meta.RESTScopeNameNamespace),
	}, nil
}

type scope meta.RESTScopeName

func (in scope) Name() meta.RESTScopeName {
	return meta.RESTScopeName(in)
}
