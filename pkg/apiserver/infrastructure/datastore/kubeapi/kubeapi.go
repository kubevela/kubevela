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
	"sort"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
)

type kubeapi struct {
	kubeClient client.Client
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
				Annotations: map[string]string{"description": "For KubeVela API Server metadata storage."},
			}}); err != nil {
			return nil, fmt.Errorf("create namespace failure %w", err)
		}
	}
	migrate(cfg.Database)
	return &kubeapi{
		kubeClient: kubeClient,
		namespace:  cfg.Database,
	}, nil
}

func generateName(entity datastore.Entity) string {
	// record the old ways here, it'll be migrated
	// name := fmt.Sprintf("veladatabase-%s-%s", entity.TableName(), entity.PrimaryKey())
	name := fmt.Sprintf("%s-%s", entity.ShortTableName(), entity.PrimaryKey())
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
	if err := m.kubeClient.Create(ctx, configMap); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return datastore.ErrRecordExist
		}
		return datastore.NewDBError(err)
	}
	return nil
}

// BatchAdd batch add entity, this operation has some atomicity.
func (m *kubeapi) BatchAdd(ctx context.Context, entities []datastore.Entity) error {
	notRollback := make(map[string]int)
	for i, saveEntity := range entities {
		if err := m.Add(ctx, saveEntity); err != nil {
			if errors.Is(err, datastore.ErrRecordExist) {
				notRollback[saveEntity.PrimaryKey()] = 1
			}
			for _, deleteEntity := range entities[:i] {
				if _, exit := notRollback[deleteEntity.PrimaryKey()]; !exit {
					if err := m.Delete(ctx, deleteEntity); err != nil {
						if !errors.Is(err, datastore.ErrRecordNotExist) {
							log.Logger.Errorf("rollback delete entity failure %w", err)
						}
					}
				}
			}
			return datastore.NewDBError(fmt.Errorf("save entities occur error, %w", err))
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
	if err := m.kubeClient.Get(ctx, types.NamespacedName{Namespace: m.namespace, Name: generateName(entity)}, &configMap); err != nil {
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
	// update labels
	labels := entity.Index()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["table"] = entity.TableName()
	labels["primaryKey"] = entity.PrimaryKey()
	entity.SetUpdateTime(time.Now())
	var configMap corev1.ConfigMap
	if err := m.kubeClient.Get(ctx, types.NamespacedName{Namespace: m.namespace, Name: generateName(entity)}, &configMap); err != nil {
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
	configMap.Labels = labels
	if err := m.kubeClient.Update(ctx, &configMap); err != nil {
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
	if err := m.kubeClient.Get(ctx, types.NamespacedName{Namespace: m.namespace, Name: generateName(entity)}, &configMap); err != nil {
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
	if err := m.kubeClient.Delete(ctx, m.generateConfigMap(entity)); err != nil {
		if apierrors.IsNotFound(err) {
			return datastore.ErrRecordNotExist
		}
		return datastore.NewDBError(err)
	}
	return nil
}

type bySortOptionConfigMap struct {
	items   []corev1.ConfigMap
	objects []map[string]interface{}
	sortBy  []datastore.SortOption
}

func newBySortOptionConfigMap(items []corev1.ConfigMap, sortBy []datastore.SortOption) bySortOptionConfigMap {
	s := bySortOptionConfigMap{
		items:   items,
		objects: make([]map[string]interface{}, len(items)),
		sortBy:  sortBy,
	}
	for i, item := range items {
		m := map[string]interface{}{}
		data := item.BinaryData["data"]
		for _, op := range sortBy {
			res := gjson.Get(string(data), op.Key)
			switch res.Type {
			case gjson.Number:
				m[op.Key] = res.Num
			default:
				if !res.Time().IsZero() {
					m[op.Key] = res.Time()
				} else {
					m[op.Key] = res.Raw
				}
			}
		}
		s.objects[i] = m
	}
	return s
}

func (b bySortOptionConfigMap) Len() int {
	return len(b.items)
}

func (b bySortOptionConfigMap) Swap(i, j int) {
	b.items[i], b.items[j] = b.items[j], b.items[i]
	b.objects[i], b.objects[j] = b.objects[j], b.objects[i]
}

func (b bySortOptionConfigMap) Less(i, j int) bool {
	for _, op := range b.sortBy {
		x := b.objects[i][op.Key]
		y := b.objects[j][op.Key]
		_x, xok := x.(time.Time)
		_y, yok := y.(time.Time)
		var xScore, yScore float64
		if xok && yok {
			xScore = float64(_x.UnixNano())
			yScore = float64(_y.UnixNano())
		}
		if !xok && !yok {
			_x, xok := x.(float64)
			_y, yok := y.(float64)
			if xok && yok {
				xScore = _x
				yScore = _y
			}
		}
		if xScore == yScore {
			continue
		}
		if op.Order == datastore.SortOrderAscending {
			return xScore < yScore
		}
		return xScore > yScore
	}
	return true
}

func _sortConfigMapBySortOptions(items []corev1.ConfigMap, sortOptions []datastore.SortOption) []corev1.ConfigMap {
	so := newBySortOptionConfigMap(items, sortOptions)
	sort.Sort(so)
	return so.items
}

func _filterConfigMapByFuzzyQueryOptions(items []corev1.ConfigMap, queries []datastore.FuzzyQueryOption) []corev1.ConfigMap {
	var _items []corev1.ConfigMap
	for _, item := range items {
		data := string(item.BinaryData["data"])
		valid := true
		for _, query := range queries {
			res := gjson.Get(data, query.Key)
			if res.Type != gjson.String || !strings.Contains(res.Str, query.Query) {
				valid = false
				break
			}
		}
		if valid {
			_items = append(_items, item)
		}
	}
	return _items
}

// List will list all database records by select labels according to table name
func (m *kubeapi) List(ctx context.Context, entity datastore.Entity, op *datastore.ListOptions) ([]datastore.Entity, error) {
	if entity.TableName() == "" {
		return nil, datastore.ErrTableNameEmpty
	}

	selector, err := labels.Parse(fmt.Sprintf("table=%s", entity.TableName()))
	if err != nil {
		return nil, datastore.NewDBError(err)
	}

	rq, _ := labels.NewRequirement(MigrateKey, selection.DoesNotExist, []string{"ok"})
	selector = selector.Add(*rq)

	for k, v := range entity.Index() {
		rq, err := labels.NewRequirement(k, selection.Equals, []string{v})
		if err != nil {
			return nil, datastore.ErrIndexInvalid
		}
		selector = selector.Add(*rq)
	}
	if op != nil {
		for _, inFilter := range op.In {
			rq, err := labels.NewRequirement(inFilter.Key, selection.In, inFilter.Values)
			if err != nil {
				log.Logger.Errorf("new list requirement failure %s", err.Error())
				return nil, datastore.ErrIndexInvalid
			}
			selector = selector.Add(*rq)
		}
		for _, notFilter := range op.IsNotExist {
			rq, err := labels.NewRequirement(notFilter.Key, selection.DoesNotExist, []string{})
			if err != nil {
				log.Logger.Errorf("new list requirement failure %s", err.Error())
				return nil, datastore.ErrIndexInvalid
			}
			selector = selector.Add(*rq)
		}
	}
	options := &client.ListOptions{
		LabelSelector: selector,
		Namespace:     m.namespace,
	}
	var skip, limit int
	if op != nil && op.PageSize > 0 && op.Page > 0 {
		skip = op.PageSize * (op.Page - 1)
		limit = op.PageSize
		if skip < 0 {
			skip = 0
		}
	}
	var configMaps corev1.ConfigMapList
	if err := m.kubeClient.List(ctx, &configMaps, options); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, datastore.NewDBError(err)
	}
	items := configMaps.Items
	if op != nil && len(op.Queries) > 0 {
		items = _filterConfigMapByFuzzyQueryOptions(items, op.Queries)
	}
	if op != nil && len(op.SortBy) > 0 {
		items = _sortConfigMapBySortOptions(items, op.SortBy)
	}
	if op != nil && op.PageSize > 0 && op.Page > 0 {
		if skip >= len(items) {
			items = []corev1.ConfigMap{}
		} else {
			items = items[skip:]
		}
		if limit >= len(items) {
			limit = len(items)
		}
		items = items[:limit]
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

// Count counts entities
func (m *kubeapi) Count(ctx context.Context, entity datastore.Entity, filterOptions *datastore.FilterOptions) (int64, error) {
	if entity.TableName() == "" {
		return 0, datastore.ErrTableNameEmpty
	}

	selector, err := labels.Parse(fmt.Sprintf("table=%s", entity.TableName()))
	if err != nil {
		return 0, datastore.NewDBError(err)
	}
	for k, v := range entity.Index() {
		rq, err := labels.NewRequirement(k, selection.Equals, []string{v})
		if err != nil {
			return 0, datastore.ErrIndexInvalid
		}
		selector = selector.Add(*rq)
	}
	if filterOptions != nil {
		for _, inFilter := range filterOptions.In {
			rq, err := labels.NewRequirement(inFilter.Key, selection.In, inFilter.Values)
			if err != nil {
				return 0, datastore.ErrIndexInvalid
			}
			selector = selector.Add(*rq)
		}
		for _, notFilter := range filterOptions.IsNotExist {
			rq, err := labels.NewRequirement(notFilter.Key, selection.DoesNotExist, []string{})
			if err != nil {
				log.Logger.Errorf("new list requirement failure %s", err.Error())
				return 0, datastore.ErrIndexInvalid
			}
			selector = selector.Add(*rq)
		}
	}

	options := &client.ListOptions{
		LabelSelector: selector,
		Namespace:     m.namespace,
	}

	var configMaps corev1.ConfigMapList
	if err := m.kubeClient.List(ctx, &configMaps, options); err != nil {
		if apierrors.IsNotFound(err) {
			return 0, nil
		}
		return 0, datastore.NewDBError(err)
	}
	items := configMaps.Items
	if filterOptions != nil && len(filterOptions.Queries) > 0 {
		items = _filterConfigMapByFuzzyQueryOptions(configMaps.Items, filterOptions.Queries)
	}
	return int64(len(items)), nil
}
