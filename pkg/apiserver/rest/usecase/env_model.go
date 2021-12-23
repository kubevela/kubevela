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
	"errors"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

func getEnv(ctx context.Context, ds datastore.DataStore, envName string) (*model.Env, error) {
	env := &model.Env{}
	env.Name = envName
	if err := ds.Get(ctx, env); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrEnvNotExisted
		}
		return nil, err
	}
	return env, nil
}

func listEnvs(ctx context.Context, ds datastore.DataStore, listOption *datastore.ListOptions) ([]*model.Env, error) {
	var env = model.Env{}
	entities, err := ds.List(ctx, &env, listOption)
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
