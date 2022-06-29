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

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/repository"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/multicluster"
)

// TargetService Target manage api
type TargetService interface {
	GetTarget(ctx context.Context, TargetName string) (*model.Target, error)
	DetailTarget(ctx context.Context, Target *model.Target) (*apisv1.DetailTargetResponse, error)
	DeleteTarget(ctx context.Context, TargetName string) error
	CreateTarget(ctx context.Context, req apisv1.CreateTargetRequest) (*apisv1.DetailTargetResponse, error)
	UpdateTarget(ctx context.Context, Target *model.Target, req apisv1.UpdateTargetRequest) (*apisv1.DetailTargetResponse, error)
	ListTargets(ctx context.Context, page, pageSize int, projectName string) (*apisv1.ListTargetResponse, error)
	ListTargetCount(ctx context.Context, projectName string) (int64, error)
	Init(ctx context.Context) error
}

type targetServiceImpl struct {
	Store     datastore.DataStore `inject:"datastore"`
	K8sClient client.Client       `inject:"kubeClient"`
}

// NewTargetService new Target service
func NewTargetService() TargetService {
	return &targetServiceImpl{}
}

func (dt *targetServiceImpl) Init(ctx context.Context) error {
	targets, err := dt.Store.List(ctx, &model.Target{}, &datastore.ListOptions{FilterOptions: datastore.FilterOptions{
		IsNotExist: []datastore.IsNotExistQueryOption{
			{
				Key: "project",
			},
		},
	}})
	if err != nil {
		return fmt.Errorf("list target failure %w", err)
	}
	for _, target := range targets {
		t := target.(*model.Target)
		t.Project = model.DefaultInitName
		if err := dt.Store.Put(ctx, t); err != nil {
			return err
		}
		if err := managePrivilegesForTarget(ctx, dt.K8sClient, t, false); err != nil {
			return err
		}
	}
	return nil
}
func (dt *targetServiceImpl) ListTargets(ctx context.Context, page, pageSize int, projectName string) (*apisv1.ListTargetResponse, error) {
	targets, err := repository.ListTarget(ctx, dt.Store, projectName, &datastore.ListOptions{
		Page:     page,
		PageSize: pageSize,
		SortBy:   []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}},
	})
	if err != nil {
		return nil, err
	}
	resp := &apisv1.ListTargetResponse{
		Targets: []apisv1.TargetBase{},
	}
	for _, raw := range targets {
		resp.Targets = append(resp.Targets, *(dt.convertFromTargetModel(ctx, raw)))
	}
	count, err := dt.Store.Count(ctx, &model.Target{Project: projectName}, nil)
	if err != nil {
		return nil, err
	}
	resp.Total = count

	return resp, nil
}

func (dt *targetServiceImpl) ListTargetCount(ctx context.Context, projectName string) (int64, error) {
	return dt.Store.Count(ctx, &model.Target{Project: projectName}, nil)
}

// DeleteTarget delete application Target
func (dt *targetServiceImpl) DeleteTarget(ctx context.Context, targetName string) error {
	target := &model.Target{
		Name: targetName,
	}
	ddt, err := dt.GetTarget(ctx, targetName)
	if errors.Is(err, datastore.ErrRecordExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = repository.DeleteTargetNamespace(ctx, dt.K8sClient, ddt.Cluster.ClusterName, ddt.Cluster.Namespace, targetName); err != nil {
		return err
	}
	if err = managePrivilegesForTarget(ctx, dt.K8sClient, ddt, true); err != nil {
		return err
	}
	if err = dt.Store.Delete(ctx, target); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrTargetNotExist
		}
		return err
	}
	return nil
}

// CreateTarget will create a delivery target binding with a cluster and namespace, by default, it will use local cluster and namespace align with targetName
// TODO(@wonderflow): we should support empty target in the future which only delivery cloud resources
func (dt *targetServiceImpl) CreateTarget(ctx context.Context, req apisv1.CreateTargetRequest) (*apisv1.DetailTargetResponse, error) {
	var project = model.Project{
		Name: req.Project,
	}
	if err := dt.Store.Get(ctx, &project); err != nil {
		return nil, bcode.ErrProjectIsNotExist
	}
	target := convertCreateReqToTargetModel(req)
	if req.Cluster == nil {
		req.Cluster = &apisv1.ClusterTarget{ClusterName: multicluster.ClusterLocalName, Namespace: req.Name}
	}
	if err := repository.CreateTargetNamespace(ctx, dt.K8sClient, req.Cluster.ClusterName, req.Cluster.Namespace, req.Name); err != nil {
		return nil, err
	}
	if err := managePrivilegesForTarget(ctx, dt.K8sClient, &target, false); err != nil {
		return nil, err
	}
	err := repository.CreateTarget(ctx, dt.Store, &target)
	if err != nil {
		return nil, err
	}
	return dt.DetailTarget(ctx, &target)
}

func (dt *targetServiceImpl) UpdateTarget(ctx context.Context, target *model.Target, req apisv1.UpdateTargetRequest) (*apisv1.DetailTargetResponse, error) {
	targetModel := convertUpdateReqToTargetModel(target, req)
	if err := dt.Store.Put(ctx, targetModel); err != nil {
		return nil, err
	}
	// Compatible with historical data, if the existing Target has not been authorized, perform an update action.
	if err := managePrivilegesForTarget(ctx, dt.K8sClient, targetModel, false); err != nil {
		return nil, err
	}
	return dt.DetailTarget(ctx, targetModel)
}

// DetailTarget detail Target
func (dt *targetServiceImpl) DetailTarget(ctx context.Context, target *model.Target) (*apisv1.DetailTargetResponse, error) {
	return &apisv1.DetailTargetResponse{
		TargetBase: *dt.convertFromTargetModel(ctx, target),
	}, nil
}

// GetTarget get Target model
func (dt *targetServiceImpl) GetTarget(ctx context.Context, targetName string) (*model.Target, error) {
	Target := &model.Target{
		Name: targetName,
	}
	if err := dt.Store.Get(ctx, Target); err != nil {
		return nil, err
	}
	return Target, nil
}

func convertUpdateReqToTargetModel(target *model.Target, req apisv1.UpdateTargetRequest) *model.Target {
	target.Alias = req.Alias
	target.Description = req.Description
	target.Variable = req.Variable
	return target
}

func convertCreateReqToTargetModel(req apisv1.CreateTargetRequest) model.Target {
	target := model.Target{
		Name:        req.Name,
		Alias:       req.Alias,
		Description: req.Description,
		Cluster:     (*model.ClusterTarget)(req.Cluster),
		Variable:    req.Variable,
		Project:     req.Project,
	}
	return target
}

func (dt *targetServiceImpl) convertFromTargetModel(ctx context.Context, target *model.Target) *apisv1.TargetBase {
	var appNum int64 = 0
	// TODO: query app num in target
	targetBase := &apisv1.TargetBase{
		Name:        target.Name,
		Alias:       target.Alias,
		Description: target.Description,
		Cluster:     (*apisv1.ClusterTarget)(target.Cluster),
		Variable:    target.Variable,
		CreateTime:  target.CreateTime,
		UpdateTime:  target.UpdateTime,
		AppNum:      appNum,
	}
	if target.Project != "" {
		var project = model.Project{
			Name: target.Project,
		}
		if err := dt.Store.Get(ctx, &project); err != nil {
			log.Logger.Errorf("get project failure %s", err.Error())
		}
		targetBase.Project = apisv1.NameAlias{Name: project.Name, Alias: project.Alias}
	}
	if targetBase.Cluster != nil && targetBase.Cluster.ClusterName != "" {
		cluster, err := _getClusterFromDataStore(ctx, dt.Store, target.Cluster.ClusterName)
		if err != nil {
			log.Logger.Errorf("query cluster info failure %s", err.Error())
		}
		if cluster != nil {
			targetBase.ClusterAlias = cluster.Alias
		}
	}
	return targetBase
}

// managePrivilegesForTarget grant or revoke privileges for target
func managePrivilegesForTarget(ctx context.Context, cli client.Client, target *model.Target, revoke bool) error {
	if target.Cluster == nil {
		return nil
	}
	p := &auth.ScopedPrivilege{Cluster: target.Cluster.ClusterName, Namespace: target.Cluster.Namespace}
	identity := &auth.Identity{Groups: []string{utils.KubeVelaProjectGroupPrefix + target.Project}}
	writer := &bytes.Buffer{}
	f, msg := auth.GrantPrivileges, "GrantPrivileges"
	if revoke {
		f, msg = auth.RevokePrivileges, "RevokePrivileges"
	}
	if err := f(ctx, cli, []auth.PrivilegeDescription{p}, identity, writer); err != nil {
		return err
	}
	log.Logger.Debugf("%s: %s", msg, writer.String())
	return nil
}

// NewTestTargetService create the target service instance for testing
func NewTestTargetService(ds datastore.DataStore, c client.Client) TargetService {
	return &targetServiceImpl{Store: ds, K8sClient: c}
}
