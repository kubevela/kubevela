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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
)

type kubeapi struct {
	kubeclient client.Client
	namespace  string
}

// New new kubeapi datastore instance
// Data is stored using ConfigMap.
func New(ctx context.Context, cfg datastore.Config) (datastore.DataStore, error) {
	kubeClient, err := clients.GetKubeClient()
	if err != nil {
		return nil, err
	}
	if cfg.Database == "" {
		cfg.Database = "kubevela_store"
	}
	var namespace corev1.Namespace
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: cfg.Database}, &namespace); apierrors.IsNotFound(err) {
		if err := kubeClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:        cfg.Database,
				Annotations: map[string]string{"description": "For kubevela apiserver metadata storage."},
			}}); err != nil {
			return nil, fmt.Errorf("create namesapce failure %w", err)
		}
	}
	return &kubeapi{
		kubeclient: kubeClient,
		namespace:  cfg.Database,
	}, nil
}

func generateName(entity datastore.Entity) string {
	name := fmt.Sprintf("veladatabase-%s-%s", entity.TableName(), entity.PrimaryKey())
	return strings.ReplaceAll(name, "_", "-")
}

func (m *kubeapi) generateConfigMap(entity datastore.Entity) *corev1.ConfigMap {
	data, _ := json.Marshal(entity)
	labels := entity.Index()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["table"] = entity.TableName()
	labels["primaryKey"] = entity.PrimaryKey()
	var configMap = corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateName(entity),
			Namespace: m.namespace,
			Labels:    labels,
		},
		BinaryData: map[string][]byte{
			"data": data,
		},
	}
	return &configMap
}

// Add add data model
func (m *kubeapi) Add(ctx context.Context, entity datastore.Entity) error {
	if entity.PrimaryKey() == "" {
		return datastore.ErrPrimaryEmpty
	}
	if entity.TableName() == "" {
		return datastore.ErrTableNameEmpty
	}
	entity.SetCreateTime(time.Now())
	entity.SetUpdateTime(time.Now())
	configMap := m.generateConfigMap(entity)
	if err := m.kubeclient.Create(ctx, configMap); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return datastore.ErrRecordExist
		}
		return datastore.NewDBError(err)
	}
	return nil
}

// BatchAdd batch add entity, this operation has some atomicity.
func (m *kubeapi) BatchAdd(ctx context.Context, entitys []datastore.Entity) error {
	donotRollback := make(map[string]int)
	for i, saveEntity := range entitys {
		if err := m.Add(ctx, saveEntity); err != nil {
			if errors.Is(err, datastore.ErrRecordExist) {
				donotRollback[saveEntity.PrimaryKey()] = 1
			}
			for _, deleteEntity := range entitys[:i] {
				if _, exit := donotRollback[deleteEntity.PrimaryKey()]; !exit {
					if err := m.Delete(ctx, deleteEntity); err != nil {
						if !errors.Is(err, datastore.ErrRecordNotExist) {
							log.Logger.Errorf("rollback delete component failure %w", err)
						}
					}
				}
			}
			return datastore.NewDBError(fmt.Errorf("save components occur error, %w", err))
		}
	}
	return nil
}

// Get get data model
func (m *kubeapi) Get(ctx context.Context, entity datastore.Entity) error {
	if entity.PrimaryKey() == "" {
		return datastore.ErrPrimaryEmpty
	}
	if entity.TableName() == "" {
		return datastore.ErrTableNameEmpty
	}
	var configMap corev1.ConfigMap
	if err := m.kubeclient.Get(ctx, types.NamespacedName{Namespace: m.namespace, Name: generateName(entity)}, &configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return datastore.ErrRecordNotExist
		}
		return datastore.NewDBError(err)
	}
	if err := json.Unmarshal(configMap.BinaryData["data"], entity); err != nil {
		return datastore.NewDBError(err)
	}
	return nil
}

// Put update data model
func (m *kubeapi) Put(ctx context.Context, entity datastore.Entity) error {
	if entity.PrimaryKey() == "" {
		return datastore.ErrPrimaryEmpty
	}
	if entity.TableName() == "" {
		return datastore.ErrTableNameEmpty
	}
	entity.SetUpdateTime(time.Now())
	var configMap corev1.ConfigMap
	if err := m.kubeclient.Get(ctx, types.NamespacedName{Namespace: m.namespace, Name: generateName(entity)}, &configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return datastore.ErrRecordNotExist
		}
		return datastore.NewDBError(err)
	}
	data, err := json.Marshal(entity)
	if err != nil {
		return datastore.NewDBError(err)
	}
	configMap.BinaryData["data"] = data
	if err := m.kubeclient.Update(ctx, &configMap); err != nil {
		return datastore.NewDBError(err)
	}
	return nil
}

// IsExist determine whether data exists.
func (m *kubeapi) IsExist(ctx context.Context, entity datastore.Entity) (bool, error) {
	if entity.PrimaryKey() == "" {
		return false, datastore.ErrPrimaryEmpty
	}
	if entity.TableName() == "" {
		return false, datastore.ErrTableNameEmpty
	}
	var configMap corev1.ConfigMap
	if err := m.kubeclient.Get(ctx, types.NamespacedName{Namespace: m.namespace, Name: generateName(entity)}, &configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, datastore.NewDBError(err)
	}
	return true, nil
}

// Delete delete data
func (m *kubeapi) Delete(ctx context.Context, entity datastore.Entity) error {
	if entity.PrimaryKey() == "" {
		return datastore.ErrPrimaryEmpty
	}
	if entity.TableName() == "" {
		return datastore.ErrTableNameEmpty
	}
	if err := m.kubeclient.Delete(ctx, m.generateConfigMap(entity)); err != nil {
		if apierrors.IsNotFound(err) {
			return datastore.ErrRecordNotExist
		}
		return datastore.NewDBError(err)
	}
	return nil
}

// TableName() can't return zero value.
func (m *kubeapi) List(ctx context.Context, entity datastore.Entity, op *datastore.ListOptions) ([]datastore.Entity, error) {
	if entity.TableName() == "" {
		return nil, datastore.ErrTableNameEmpty
	}

	selector, err := labels.Parse(fmt.Sprintf("table=%s", entity.TableName()))
	if err != nil {
		return nil, datastore.NewDBError(err)
	}
	for k, v := range entity.Index() {
		rq, err := labels.NewRequirement(k, selection.Equals, []string{v})
		if err != nil {
			return nil, datastore.ErrIndexInvalid
		}
		selector = selector.Add(*rq)
	}
	options := &client.ListOptions{
		LabelSelector: selector,
	}
	var skip, limit int64
	if op != nil && op.PageSize > 0 && op.Page > 0 {
		skip = int64(op.PageSize * (op.Page - 1))
		limit = int64(op.PageSize * op.Page)
		if skip < 0 {
			skip = 0
		}
		if limit < 0 {
			limit = skip
		}
		options.Limit = limit
	}
	var configMaps corev1.ConfigMapList
	if err := m.kubeclient.List(ctx, &configMaps, options); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, datastore.NewDBError(err)
	}
	items := configMaps.Items
	if op != nil && op.PageSize > 0 && op.Page > 0 {
		if len(configMaps.Items) > int(limit) {
			items = configMaps.Items[skip:limit]
		} else {
			items = configMaps.Items[skip:]
		}
	}
	var list []datastore.Entity
	for _, item := range items {
		ent, err := datastore.NewEntity(entity)
		if err != nil {
			return nil, datastore.NewDBError(err)
		}
		if err := json.Unmarshal(item.BinaryData["data"], ent); err != nil {
			return nil, datastore.NewDBError(err)
		}
		list = append(list, ent)
	}
	return list, nil
}
