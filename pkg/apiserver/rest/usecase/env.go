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

	apierror "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/oam"
	util "github.com/oam-dev/kubevela/pkg/utils"
	velaerr "github.com/oam-dev/kubevela/pkg/utils/errors"
)

// EnvUsecase defines the API of Env.
type EnvUsecase interface {
	GetEnv(ctx context.Context, envName string) (*model.Env, error)
	ListEnvs(ctx context.Context) ([]*apisv1.Env, error)
	DeleteEnv(ctx context.Context, envName string) error
	CreateEnv(ctx context.Context, req apisv1.CreateEnvRequest) (*apisv1.Env, error)
}

type envUsecaseImpl struct {
	ds         datastore.DataStore
	kubeClient client.Client
}

// NewEnvUsecase new env usecase
func NewEnvUsecase(ds datastore.DataStore) EnvUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("get kubeclient failure %s", err.Error())
	}
	return &envUsecaseImpl{kubeClient: kubecli, ds: ds}
}

// GetEnv get env
func (p *envUsecaseImpl) GetEnv(ctx context.Context, envName string) (*model.Env, error) {
	env := &model.Env{}
	env.Name = envName
	if err := p.ds.Get(ctx, env); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrEnvNotExisted
		}
		return nil, err
	}
	return env, nil
}

// DeleteEnv delete an env by name
// the function assume applications contain in env already empty.
// it won't delete the namespace created by the Env, but it will update the label
func (p *envUsecaseImpl) DeleteEnv(ctx context.Context, envName string) error {
	env := &model.Env{}
	env.Name = envName

	if err := p.ds.Get(ctx, env); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil
		}
		return err
	}
	// reset the labels
	err := util.UpdateNamespace(ctx, p.kubeClient, env.Namespace, util.MergeOverrideLabels(map[string]string{
		oam.LabelNamespaceOfEnv: "",
		oam.LabelUsageNamespace: "",
	}))
	if err != nil && apierror.IsNotFound(err) {
		return err
	}

	if err = p.ds.Delete(ctx, env); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil
		}
		return err
	}
	return nil
}

// ListEnvs list envs
func (p *envUsecaseImpl) ListEnvs(ctx context.Context) ([]*apisv1.Env, error) {
	var env = model.Env{}
	entities, err := p.ds.List(ctx, &env, &datastore.ListOptions{SortBy: []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}}})
	if err != nil {
		return nil, err
	}
	var envs []*apisv1.Env
	for _, entity := range entities {
		apienv := entity.(*model.Env)
		envs = append(envs, convertEnvModel2Base(apienv))
	}
	return envs, nil
}

// CreateEnv create an env for request
func (p *envUsecaseImpl) CreateEnv(ctx context.Context, req apisv1.CreateEnvRequest) (*apisv1.Env, error) {

	env := &model.Env{}
	env.Name = req.Name

	exist, err := p.ds.IsExist(ctx, env)
	if err != nil {
		log.Logger.Errorf("check if env name exists failure %s", err.Error())
		return nil, bcode.ErrEnvAlreadyExists
	}
	if exist {
		return nil, bcode.ErrEnvAlreadyExists
	}
	if req.Namespace == "" {
		req.Namespace = req.Name
	}
	newEnv := &model.Env{}
	newEnv.EnvBase = model.EnvBase(req)

	// create namespace at first
	err = util.CreateOrUpdateNamespace(ctx, p.kubeClient, newEnv.Namespace,
		util.MergeOverrideLabels(map[string]string{
			oam.LabelUsageNamespace: oam.VelaUsageEnv,
		}), util.MergeNoConflictLabels(map[string]string{
			oam.LabelNamespaceOfEnv: newEnv.Name,
		}))
	if err != nil {
		if velaerr.IsLabelConflict(err) {
			return nil, bcode.ErrEnvNamespaceAlreadyBound
		}
		log.Logger.Errorf("update namespace label failure %s", err.Error())
		return nil, bcode.ErrEnvNamespaceFail
	}
	if err := p.ds.Add(ctx, newEnv); err != nil {
		return nil, err
	}
	resp := apisv1.Env(*newEnv)
	return &resp, nil
}

func convertEnvModel2Base(project *model.Env) *apisv1.Env {
	data := apisv1.Env(*project)
	return &data
}
