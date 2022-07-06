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
	"errors"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
)

// ListApplicationPolicies query the application policies
func ListApplicationPolicies(ctx context.Context, store datastore.DataStore, app *model.Application) (list []*model.ApplicationPolicy, err error) {
	var policy = model.ApplicationPolicy{
		AppPrimaryKey: app.PrimaryKey(),
	}
	policies, err := store.List(ctx, &policy, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, policy := range policies {
		pm := policy.(*model.ApplicationPolicy)
		list = append(list, pm)
	}
	return
}

// ListApplicationEnvPolicies list the policies that only belong to the specified env
func ListApplicationEnvPolicies(ctx context.Context, store datastore.DataStore, app *model.Application, envName string) (list []*model.ApplicationPolicy, err error) {
	var policy = model.ApplicationPolicy{
		AppPrimaryKey: app.PrimaryKey(),
		EnvName:       envName,
	}
	policies, err := store.List(ctx, &policy, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, policy := range policies {
		pm := policy.(*model.ApplicationPolicy)
		list = append(list, pm)
	}
	return
}

// ListApplicationCommonPolicies list the policies that common to all environments
func ListApplicationCommonPolicies(ctx context.Context, store datastore.DataStore, app *model.Application) (list []*model.ApplicationPolicy, err error) {
	var policy = model.ApplicationPolicy{
		AppPrimaryKey: app.PrimaryKey(),
	}
	policies, err := store.List(ctx, &policy, &datastore.ListOptions{
		FilterOptions: datastore.FilterOptions{
			IsNotExist: []datastore.IsNotExistQueryOption{{
				Key: "envName",
			}},
		},
	})
	if err != nil {
		return nil, err
	}
	for _, policy := range policies {
		pm := policy.(*model.ApplicationPolicy)
		list = append(list, pm)
	}
	return
}

// DeleteApplicationEnvPolicies delete the policies via app name and env name
func DeleteApplicationEnvPolicies(ctx context.Context, store datastore.DataStore, app *model.Application, envName string) error {
	log.Logger.Debugf("clear the policies via app name %s and env name %s", app.PrimaryKey(), envName)
	policies, err := ListApplicationEnvPolicies(ctx, store, app, envName)
	if err != nil {
		return err
	}
	for _, policy := range policies {
		if err := store.Delete(ctx, policy); err != nil && !errors.Is(err, datastore.ErrRecordNotExist) {
			log.Logger.Errorf("fail to clear the policies belong to the env %w", err)
			continue
		}
	}
	return nil
}

// GetApplicationRevision get the application revision
// If the version is empty, will query the latest revision of the application
func GetApplicationRevision(ctx context.Context, store datastore.DataStore, appName, version string) (*model.ApplicationRevision, error) {
	ar := &model.ApplicationRevision{AppPrimaryKey: appName}
	if version != "" {
		ar.Version = version
	}
	revisions, err := store.List(ctx, ar, &datastore.ListOptions{
		Page:     1,
		PageSize: 1,
		SortBy:   []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}},
	})
	if err != nil || len(revisions) == 0 {
		return nil, bcode.ErrApplicationRevisionNotExist
	}
	latestRevisionRaw := revisions[0]
	latestRevision, ok := latestRevisionRaw.(*model.ApplicationRevision)
	if !ok {
		return nil, errors.New("convert application revision error")
	}
	return latestRevision, nil
}
