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

package usecase

import (
	"context"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
)

type envListOption struct {
	appPrimaryKey string
	envName       string
	projectName   string
}

func listEnvBindings(ctx context.Context, ds datastore.DataStore, listOption envListOption) ([]*model.EnvBinding, error) {
	var envBinding = model.EnvBinding{}
	if listOption.appPrimaryKey != "" {
		envBinding.AppPrimaryKey = listOption.appPrimaryKey
	}
	if listOption.envName != "" {
		envBinding.Name = listOption.envName
	}
	envBindings, err := ds.List(ctx, &envBinding, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	var ret []*model.EnvBinding
	for _, et := range envBindings {
		eb, ok := et.(*model.EnvBinding)
		if !ok {
			continue
		}
		ret = append(ret, eb)
	}
	return ret, nil
}
