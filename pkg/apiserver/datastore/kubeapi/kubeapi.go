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

package kubeapi

import (
	"context"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
)

type kubeapi struct {
	// kubeclient client.Client
}

// New new kubeapi datastore instance
func New(ctx context.Context, cfg datastore.Config) (datastore.DataStore, error) {
	return &kubeapi{}, nil
}

// Add add data model
func (m *kubeapi) Add(ctx context.Context, kind string, entity interface{}) error {
	return nil
}

// Get get data model
func (m *kubeapi) Get(ctx context.Context, kind, name string, decodeTo interface{}) error {
	return nil
}

// Put update data model
func (m *kubeapi) Put(ctx context.Context, kind, name string, entity interface{}) error {
	return nil
}

// Find find data model
func (m *kubeapi) Find(ctx context.Context, kind string) (datastore.Iterator, error) {
	return nil, nil
}

// FindOne find one data model
func (m *kubeapi) FindOne(ctx context.Context, kind, name string) (datastore.Iterator, error) {
	return nil, nil
}

// IsExist determine whether data exists.
func (m *kubeapi) IsExist(ctx context.Context, kind, name string) (bool, error) {
	return true, nil
}

// Delete delete data
func (m *kubeapi) Delete(ctx context.Context, kind, name string) error {
	return nil
}
