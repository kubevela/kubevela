/*
Copyright 2022 The KubeVela Authors.

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

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
)

// ListRoles list roles from store
func ListRoles(ctx context.Context, store datastore.DataStore, projectName string, page, pageSize int) ([]*model.Role, int64, error) {
	var role = model.Role{
		Project: projectName,
	}
	var filter datastore.FilterOptions
	if projectName == "" {
		filter.IsNotExist = append(filter.IsNotExist, datastore.IsNotExistQueryOption{
			Key: "project",
		})
	}
	entities, err := store.List(ctx, &role, &datastore.ListOptions{FilterOptions: filter, Page: page, PageSize: pageSize, SortBy: []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}}})
	if err != nil {
		return nil, 0, err
	}
	var roles []*model.Role
	for i := range entities {
		roles = append(roles, entities[i].(*model.Role))
	}
	count := int64(len(roles))
	if page > 0 && pageSize > 0 {
		var err error
		count, err = store.Count(ctx, &role, &filter)
		if err != nil {
			return nil, 0, err
		}
	}
	return roles, count, nil
}
