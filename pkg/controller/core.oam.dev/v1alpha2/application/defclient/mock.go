/*
Copyright 2020 The KubeVela Authors.

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

package defclient

import (
	"encoding/json"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kyaml "sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
)

var _ DefinitionClient = &MockClient{}

// MockClient simulate the behavior of client
type MockClient struct {
	wds  []*v1alpha2.WorkloadDefinition
	tds  []*v1alpha2.TraitDefinition
	gvks map[string]schema.GroupVersionKind
}

// GetWorkloadDefinition  Get WorkloadDefinition
func (mock *MockClient) GetWorkloadDefinition(name string) (*v1alpha2.WorkloadDefinition, error) {
	for _, wd := range mock.wds {
		if wd.Name == name {
			return wd, nil
		}
	}
	return nil, kerrors.NewNotFound(schema.GroupResource{
		Group:    v1alpha2.Group,
		Resource: "WorkloadDefinition",
	}, name)
}

// GetTraitDefinition Get TraitDefinition
func (mock *MockClient) GetTraitDefinition(name string) (*v1alpha2.TraitDefinition, error) {
	for _, td := range mock.tds {
		if td.Name == name {
			return td, nil
		}
	}
	return nil, kerrors.NewNotFound(schema.GroupResource{
		Group:    v1alpha2.Group,
		Resource: "TraitDefinition",
	}, name)
}

// GetScopeGVK return gvk
func (mock *MockClient) GetScopeGVK(name string) (schema.GroupVersionKind, error) {
	return mock.gvks[name], nil
}

// AddGVK  add gvk to Mock Manager
func (mock *MockClient) AddGVK(name string, gvk schema.GroupVersionKind) error {
	if mock.gvks == nil {
		mock.gvks = make(map[string]schema.GroupVersionKind)
	}
	mock.gvks[name] = gvk
	return nil
}

// AddWD  add workload definition to Mock Manager
func (mock *MockClient) AddWD(s string) error {
	wd := &v1alpha2.WorkloadDefinition{}
	_body, err := kyaml.YAMLToJSON([]byte(s))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(_body, wd); err != nil {
		return err
	}

	if mock.wds == nil {
		mock.wds = []*v1alpha2.WorkloadDefinition{}
	}
	mock.wds = append(mock.wds, wd)
	return nil
}

// AddTD add trait definition to Mock Manager
func (mock *MockClient) AddTD(s string) error {
	td := &v1alpha2.TraitDefinition{}
	_body, err := kyaml.YAMLToJSON([]byte(s))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(_body, td); err != nil {
		return err
	}
	if mock.tds == nil {
		mock.tds = []*v1alpha2.TraitDefinition{}
	}
	mock.tds = append(mock.tds, td)
	return nil
}
