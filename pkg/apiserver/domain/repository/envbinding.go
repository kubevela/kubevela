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

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	assembler "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/assembler/v1"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
)

const (
	// EnvBindingPolicyDefaultName default policy name
	EnvBindingPolicyDefaultName string = "env-bindings"
)

// EnvListOption the option for listing the env
type EnvListOption struct {
	AppPrimaryKey string
	EnvName       string
	ProjectName   string
}

// ListFullEnvBinding list the envbinding and convert to DTO
func ListFullEnvBinding(ctx context.Context, ds datastore.DataStore, option EnvListOption) ([]*apisv1.EnvBindingBase, error) {
	envBindings, err := ListEnvBindings(ctx, ds, option)
	if err != nil {
		return nil, bcode.ErrEnvBindingsNotExist
	}
	targets, err := ListTarget(ctx, ds, option.ProjectName, nil)
	if err != nil {
		return nil, err
	}
	var listOption *datastore.ListOptions
	if option.ProjectName != "" {
		listOption = &datastore.ListOptions{
			FilterOptions: datastore.FilterOptions{
				In: []datastore.InQueryOption{
					{
						Key:    "project",
						Values: []string{option.ProjectName},
					},
				},
			},
		}
	}
	envs, err := ListEnvs(ctx, ds, listOption)
	if err != nil {
		return nil, err
	}
	var list []*apisv1.EnvBindingBase
	for _, eb := range envBindings {
		env, err := pickEnv(envs, eb.Name)
		if err != nil {
			log.Logger.Errorf("envbinding invalid %s", err.Error())
			continue
		}
		list = append(list, assembler.ConvertEnvBindingModelToBase(eb, env, targets))
	}
	return list, nil
}

// ListEnvBindings list the envbinding
func ListEnvBindings(ctx context.Context, ds datastore.DataStore, listOption EnvListOption) ([]*model.EnvBinding, error) {
	var envBinding = model.EnvBinding{}
	if listOption.AppPrimaryKey != "" {
		envBinding.AppPrimaryKey = listOption.AppPrimaryKey
	}
	if listOption.EnvName != "" {
		envBinding.Name = listOption.EnvName
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

func pickEnv(envs []*model.Env, name string) (*model.Env, error) {
	for _, e := range envs {
		if e.Name == name {
			return e, nil
		}
	}
	return nil, bcode.ErrEnvNotExisted
}
