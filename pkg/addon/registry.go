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

package addon

import (
	"context"
	"encoding/json"
	"fmt"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	velatypes "github.com/oam-dev/kubevela/apis/types"
)

const registryConfigMapName = "vela-addon-registry"
const registriesKey = "registries"

// Registry represent a addon registry model
type Registry struct {
	Name string `json:"name"`

	Helm  *HelmSource       `json:"helm,omitempty"`
	Git   *GitAddonSource   `json:"git,omitempty"`
	OSS   *OSSAddonSource   `json:"oss,omitempty"`
	Gitee *GiteeAddonSource `json:"gitee,omitempty"`
}

// RegistryDataStore CRUD addon registry data in configmap
type RegistryDataStore interface {
	ListRegistries(context.Context) ([]Registry, error)
	AddRegistry(context.Context, Registry) error
	DeleteRegistry(context.Context, string) error
	UpdateRegistry(context.Context, Registry) error
	GetRegistry(context.Context, string) (Registry, error)
}

// NewRegistryDataStore get RegistryDataStore operation interface
func NewRegistryDataStore(cli client.Client) RegistryDataStore {
	return registryImpl{cli}
}

type registryImpl struct {
	client client.Client
}

func (r registryImpl) ListRegistries(ctx context.Context) ([]Registry, error) {
	cm := &v1.ConfigMap{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: velatypes.DefaultKubeVelaNS, Name: registryConfigMapName}, cm); err != nil {
		return nil, err
	}
	if _, ok := cm.Data[registriesKey]; !ok {
		return nil, NewAddonError("Error addon registry configmap registry-key not exist")
	}
	registries := map[string]Registry{}
	if err := json.Unmarshal([]byte(cm.Data[registriesKey]), &registries); err != nil {
		return nil, err
	}
	var res []Registry
	for _, registry := range registries {
		res = append(res, registry)
	}
	return res, nil
}

func (r registryImpl) AddRegistry(ctx context.Context, registry Registry) error {
	cm := &v1.ConfigMap{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: velatypes.DefaultKubeVelaNS, Name: registryConfigMapName}, cm); err != nil {
		if apierrors.IsNotFound(err) {
			b, err := json.Marshal(map[string]Registry{
				registry.Name: registry,
			})
			if err != nil {
				return err
			}
			cm = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      registryConfigMapName,
					Namespace: velatypes.DefaultKubeVelaNS,
				},
				Data: map[string]string{
					registriesKey: string(b),
				},
			}
			if err := r.client.Create(ctx, cm); err != nil {
				return err
			}
			return nil
		}
		return err
	}
	registries := map[string]Registry{}
	if err := json.Unmarshal([]byte(cm.Data[registriesKey]), &registries); err != nil {
		return err
	}
	registries[registry.Name] = registry
	b, err := json.Marshal(registries)
	if err != nil {
		return err
	}
	cm.Data = map[string]string{
		registriesKey: string(b),
	}
	if err := r.client.Update(ctx, cm); err != nil {
		return err
	}
	return nil
}

func (r registryImpl) DeleteRegistry(ctx context.Context, name string) error {
	cm := &v1.ConfigMap{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: velatypes.DefaultKubeVelaNS, Name: registryConfigMapName}, cm); err != nil {
		return err
	}
	registries := map[string]Registry{}
	if err := json.Unmarshal([]byte(cm.Data[registriesKey]), &registries); err != nil {
		return err
	}
	delete(registries, name)
	b, err := json.Marshal(registries)
	if err != nil {
		return err
	}
	cm.Data = map[string]string{
		registriesKey: string(b),
	}
	if err := r.client.Update(ctx, cm); err != nil {
		return err
	}
	return nil
}

func (r registryImpl) UpdateRegistry(ctx context.Context, registry Registry) error {
	cm := &v1.ConfigMap{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: velatypes.DefaultKubeVelaNS, Name: registryConfigMapName}, cm); err != nil {
		return err
	}
	registries := map[string]Registry{}
	if err := json.Unmarshal([]byte(cm.Data[registriesKey]), &registries); err != nil {
		return err
	}
	if _, ok := registries[registry.Name]; !ok {
		return fmt.Errorf("addon registry %s not exist", registry.Name)
	}
	registries[registry.Name] = registry
	b, err := json.Marshal(registries)
	if err != nil {
		return err
	}
	cm.Data = map[string]string{
		registriesKey: string(b),
	}
	if err := r.client.Update(ctx, cm); err != nil {
		return err
	}
	return nil
}

func (r registryImpl) GetRegistry(ctx context.Context, name string) (Registry, error) {
	var res Registry
	cm := &v1.ConfigMap{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: velatypes.DefaultKubeVelaNS, Name: registryConfigMapName}, cm); err != nil {
		return res, err
	}
	registries := map[string]Registry{}
	if err := json.Unmarshal([]byte(cm.Data[registriesKey]), &registries); err != nil {
		return res, err
	}
	var notExist bool
	if res, notExist = registries[name]; !notExist {
		return res, fmt.Errorf("registry name %s not found", name)
	}
	return res, nil
}
