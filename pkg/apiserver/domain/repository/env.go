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

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils"
	velaerr "github.com/oam-dev/kubevela/pkg/utils/errors"
)

// MigrateKey marks the label key of the migrated data
const MigrateKey = "db.oam.dev/migrated"

// CreateEnv create the environment
func CreateEnv(ctx context.Context, kubeClient client.Client, ds datastore.DataStore, env *model.Env) error {
	tenv := &model.Env{}
	tenv.Name = env.Name

	//check if env name exists
	exist, err := IsExist(ctx, kubeClient, tenv)
	if err != nil {
		log.Logger.Errorf("check if env name exists failure %s", err.Error())
		return err
	}
	if exist {
		return bcode.ErrEnvAlreadyExists
	}
	if env.Namespace == "" {
		env.Namespace = env.Name
	}

	//check if namespace was already assigned to other env
	namespace, err := utils.GetNamespace(ctx, kubeClient, env.Namespace)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if namespace != nil {
		existedEnv := namespace.GetLabels()[oam.LabelNamespaceOfEnvName]
		if existedEnv != "" && existedEnv != env.Name {
			return fmt.Errorf("the namespace %s was already assigned to env %s", env.Namespace, existedEnv)
		}
	}

	if err = CreateNamespace(ctx, kubeClient, env); err != nil {
		return err
	}
	return nil
}

// GetEnv get the environment
func GetEnv(ctx context.Context, kubeClient client.Client, envName string) (*model.Env, error) {
	env := &model.Env{}
	env.Name = envName
	if err := GetNamespace(ctx, kubeClient, env); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrEnvNotExisted
		}
		return nil, err
	}
	return env, nil
}

// ListEnvs list the environments
func ListEnvs(ctx context.Context, kubeClient client.Client, listOption *datastore.ListOptions) ([]*model.Env, error) {
	var env = model.Env{}
	entities, err := ListNamespaces(ctx, kubeClient, &env, listOption)
	if err != nil {
		return nil, err
	}

	var envs []*model.Env
	for _, entity := range entities {
		apienv, ok := entity.(*model.Env)
		if !ok {
			continue
		}
		envs = append(envs, apienv)
	}
	return envs, nil
}

// CreateNamespace create namespace
func CreateNamespace(ctx context.Context, kubeClient client.Client, env *model.Env) error {
	if env.PrimaryKey() == "" {
		return datastore.ErrPrimaryEmpty
	}
	if env.TableName() == "" {
		return datastore.ErrTableNameEmpty
	}
	currentTime := time.Now()
	env.SetCreateTime(currentTime)
	env.SetUpdateTime(currentTime)

	labels := env.Index()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["table"] = env.TableName()
	labels["primaryKey"] = env.PrimaryKey()
	for k, v := range labels {
		labels[k] = verifyValue(v)
	}
	labels[oam.LabelControlPlaneNamespaceUsage] = oam.VelaNamespaceUsageEnv
	data, _ := json.Marshal(env)

	// create namespace with labels and annotations
	err := utils.CreateOrUpdateNamespace(ctx, kubeClient, env.Namespace,
		utils.MergeOverrideLabels(labels),
		utils.MergeNoConflictLabels(map[string]string{
			oam.LabelNamespaceOfEnvName: env.Name,
		}), utils.MergeOverrideAnnotations(map[string]string{
			"data": string(data),
		}))
	if err != nil {
		if velaerr.IsLabelConflict(err) {
			return bcode.ErrEnvNamespaceAlreadyBound
		}
		log.Logger.Errorf("update namespace label failure %s", err.Error())
		return bcode.ErrEnvNamespaceFail
	}
	return nil
}

func GetNamespace(ctx context.Context, kubeClient client.Client, entity datastore.Entity) error {
	if entity.PrimaryKey() == "" {
		return datastore.ErrPrimaryEmpty
	}
	if entity.TableName() == "" {
		return datastore.ErrTableNameEmpty
	}

	var nsList v1.NamespaceList
	if err := kubeClient.List(ctx, &nsList, client.MatchingLabels{oam.LabelControlPlaneNamespaceUsage: oam.VelaNamespaceUsageEnv,
		oam.LabelNamespaceOfEnvName: entity.PrimaryKey()}); err != nil {
		return err
	}
	if len(nsList.Items) < 1 {
		return datastore.ErrRecordNotExist
	}
	namespace := &nsList.Items[0]
	if err := json.Unmarshal([]byte(namespace.Annotations["data"]), entity); err != nil {
		return datastore.NewDBError(err)
	}
	return nil
}

func ListNamespaces(ctx context.Context, kubeClient client.Client, entity datastore.Entity, op *datastore.ListOptions) ([]datastore.Entity, error) {
	if entity.TableName() == "" {
		return nil, datastore.ErrTableNameEmpty
	}

	selector := labels.NewSelector()
	rq, _ := labels.NewRequirement(oam.LabelControlPlaneNamespaceUsage, selection.Equals, []string{oam.VelaNamespaceUsageEnv})
	selector = selector.Add(*rq)

	for k, v := range entity.Index() {
		rq, err := labels.NewRequirement(k, selection.Equals, []string{verifyValue(v)})
		if err != nil {
			return nil, datastore.ErrIndexInvalid
		}
		selector = selector.Add(*rq)
	}
	if op != nil {
		for _, inFilter := range op.In {
			var values []string
			for _, value := range inFilter.Values {
				values = append(values, verifyValue(value))
			}
			rq, err := labels.NewRequirement(inFilter.Key, selection.In, values)
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
	}
	var skip, limit int
	if op != nil && op.PageSize > 0 && op.Page > 0 {
		skip = op.PageSize * (op.Page - 1)
		limit = op.PageSize
		if skip < 0 {
			skip = 0
		}
	}

	var nsList v1.NamespaceList
	if err := kubeClient.List(ctx, &nsList, options); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, datastore.NewDBError(err)
	}

	items := nsList.Items
	if op != nil && len(op.Queries) > 0 {
		items = _filterConfigMapByFuzzyQueryOptions(items, op.Queries)
	}
	if op != nil && len(op.SortBy) > 0 {
		items = _sortConfigMapBySortOptions(items, op.SortBy)
	}
	if op != nil && op.PageSize > 0 && op.Page > 0 {
		if skip >= len(items) {
			items = []v1.Namespace{}
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
		data := item.Annotations["data"]
		if data == "" {
			ent = &model.Env{
				Name:      item.Labels[oam.LabelNamespaceOfEnvName],
				Namespace: item.Name}
		} else {
			if err := json.Unmarshal([]byte(item.Annotations["data"]), ent); err != nil {
				return nil, datastore.NewDBError(err)
			}
		}
		list = append(list, ent)
	}
	return list, nil
}

func Count(ctx context.Context, kubeClient client.Client, entity datastore.Entity, filterOptions *datastore.FilterOptions) (int64, error) {
	if entity.TableName() == "" {
		return 0, datastore.ErrTableNameEmpty
	}

	selector := labels.NewSelector()
	rq, _ := labels.NewRequirement(oam.LabelControlPlaneNamespaceUsage, selection.Equals, []string{oam.VelaNamespaceUsageEnv})
	selector = selector.Add(*rq)
	for k, v := range entity.Index() {
		rq, err := labels.NewRequirement(k, selection.Equals, []string{verifyValue(v)})
		if err != nil {
			return 0, datastore.ErrIndexInvalid
		}
		selector = selector.Add(*rq)
	}
	if filterOptions != nil {
		for _, inFilter := range filterOptions.In {
			var values []string
			for _, value := range inFilter.Values {
				values = append(values, verifyValue(value))
			}
			rq, err := labels.NewRequirement(inFilter.Key, selection.In, values)
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
	}

	var nsList v1.NamespaceList
	if err := kubeClient.List(ctx, &nsList, options); err != nil {
		if apierrors.IsNotFound(err) {
			return 0, nil
		}
		return 0, datastore.NewDBError(err)
	}

	items := nsList.Items
	if filterOptions != nil && len(filterOptions.Queries) > 0 {
		items = _filterConfigMapByFuzzyQueryOptions(nsList.Items, filterOptions.Queries)
	}
	return int64(len(items)), nil
}

func UpdateNamespace(ctx context.Context, kubeClient client.Client, env *model.Env) error {
	if env.PrimaryKey() == "" {
		return datastore.ErrPrimaryEmpty
	}
	if env.TableName() == "" {
		return datastore.ErrTableNameEmpty
	}
	// update labels
	labels := env.Index()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["table"] = env.TableName()
	labels["primaryKey"] = env.PrimaryKey()
	for k, v := range labels {
		labels[k] = verifyValue(v)
	}
	env.SetUpdateTime(time.Now())
	var namespace v1.Namespace
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: env.Namespace}, &namespace); err != nil {
		if apierrors.IsNotFound(err) {
			return datastore.ErrRecordNotExist
		}
		return datastore.NewDBError(err)
	}
	data, err := json.Marshal(env)
	if err != nil {
		return datastore.NewDBError(err)
	}
	namespace.Annotations["data"] = string(data)
	for k, v := range labels {
		namespace.Labels[k] = v
	}
	if err := kubeClient.Update(ctx, &namespace); err != nil {
		return datastore.NewDBError(err)
	}
	return nil
}

func IsExist(ctx context.Context, kubeClient client.Client, env *model.Env) (bool, error) {
	if env.PrimaryKey() == "" {
		return false, datastore.ErrPrimaryEmpty
	}
	if env.TableName() == "" {
		return false, datastore.ErrTableNameEmpty
	}
	var nsList v1.NamespaceList
	if err := kubeClient.List(ctx, &nsList, client.MatchingLabels{oam.LabelControlPlaneNamespaceUsage: oam.VelaNamespaceUsageEnv,
		oam.LabelNamespaceOfEnvName: env.Name}); err != nil {
		return false, datastore.NewDBError(err)
	}
	if len(nsList.Items) < 1 {
		return false, nil
	}
	return true, nil
}

func verifyValue(v string) string {
	s := strings.ReplaceAll(v, "@", "-")
	s = strings.ReplaceAll(s, " ", "-")
	return strings.ToLower(s)
}
func _filterConfigMapByFuzzyQueryOptions(items []v1.Namespace, queries []datastore.FuzzyQueryOption) []v1.Namespace {
	var _items []v1.Namespace
	for _, item := range items {
		data := item.Annotations["data"]
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

func _sortConfigMapBySortOptions(items []v1.Namespace, sortOptions []datastore.SortOption) []v1.Namespace {
	so := newBySortOptionConfigMap(items, sortOptions)
	sort.Sort(so)
	return so.items
}

func newBySortOptionConfigMap(items []v1.Namespace, sortBy []datastore.SortOption) bySortOptionConfigMap {
	s := bySortOptionConfigMap{
		items:   items,
		objects: make([]map[string]interface{}, len(items)),
		sortBy:  sortBy,
	}
	for i, item := range items {
		m := map[string]interface{}{}
		data := item.Annotations["data"]
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

type bySortOptionConfigMap struct {
	items   []v1.Namespace
	objects []map[string]interface{}
	sortBy  []datastore.SortOption
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
