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

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/multicluster"
)

const (

	// DefaultInitName is default object name for initialization
	DefaultInitName = "default"
	// DefaultInitNamespace is default namespace name for initialization
	DefaultInitNamespace = "default"

	// DefaultTargetDescription describes default target created
	DefaultTargetDescription = "Default target is created by velaux system automatically."
	// DefaultEnvDescription describes default env created
	DefaultEnvDescription = "Default environment is created by velaux system automatically."
	// DefaultProjectDescription describes the default project created
	DefaultProjectDescription = "Default project is created by velaux system automatically."
)

// ProjectUsecase project manage usecase.
type ProjectUsecase interface {
	GetProject(ctx context.Context, projectName string) (*model.Project, error)
	ListProjects(ctx context.Context) ([]*apisv1.ProjectBase, error)
	CreateProject(ctx context.Context, req apisv1.CreateProjectRequest) (*apisv1.ProjectBase, error)
}

type projectUsecaseImpl struct {
	ds        datastore.DataStore
	k8sClient client.Client
}

// NewProjectUsecase new project usecase
func NewProjectUsecase(ds datastore.DataStore) ProjectUsecase {
	k8sClient, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("get k8sClient failure: %s", err.Error())
	}
	p := &projectUsecaseImpl{ds: ds, k8sClient: k8sClient}
	p.initDefaultProjectEnvTarget()
	return p
}

// initDefaultProjectEnvTarget will initialize a default project with a default env that contain a default target
// the default env and default target both using the `default` namespace in control plane cluster
func (p *projectUsecaseImpl) initDefaultProjectEnvTarget() {

	ctx := context.Background()
	entities, err := listProjects(ctx, p.ds)
	if err != nil {
		log.Logger.Errorf("initialize project failed %v", err)
		return
	}
	if len(entities) > 0 {
		return
	}
	log.Logger.Info("no default project found, adding a default project with default env and target")

	// initialize default target first
	err = createTarget(ctx, p.ds, &model.Target{
		Name:        DefaultInitName,
		Alias:       "Default",
		Description: DefaultTargetDescription,
		Cluster: &model.ClusterTarget{
			ClusterName: multicluster.ClusterLocalName,
			Namespace:   DefaultInitNamespace,
		},
	})
	// for idempotence, ignore default target already exist error
	if err != nil && errors.Is(err, bcode.ErrTargetExist) {
		log.Logger.Errorf("initialize default target failed %v", err)
		return
	}

	// initialize default target first
	err = createEnv(ctx, p.k8sClient, p.ds, &model.Env{
		EnvBase: model.EnvBase{
			Name:        DefaultInitName,
			Alias:       "Default",
			Description: DefaultEnvDescription,

			Project:   DefaultInitName,
			Namespace: DefaultInitNamespace,
			Targets:   []string{DefaultInitName},
		},
	})
	// for idempotence, ignore default env already exist error
	if err != nil && errors.Is(err, bcode.ErrEnvAlreadyExists) {
		log.Logger.Errorf("initialize default environment failed %v", err)
		return
	}

	_, err = p.CreateProject(ctx, apisv1.CreateProjectRequest{
		Name:        DefaultInitName,
		Alias:       "Default",
		Description: DefaultProjectDescription,
	})
	if err != nil {
		log.Logger.Errorf("initialize project failed %v", err)
		return
	}
}

// GetProject get project
func (p *projectUsecaseImpl) GetProject(ctx context.Context, projectName string) (*model.Project, error) {
	project := &model.Project{Name: projectName}
	if err := p.ds.Get(ctx, project); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrProjectIsNotExist
		}
		return nil, err
	}
	return project, nil
}

func listProjects(ctx context.Context, ds datastore.DataStore) ([]*apisv1.ProjectBase, error) {
	var project = model.Project{}
	entitys, err := ds.List(ctx, &project, &datastore.ListOptions{SortBy: []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}}})
	if err != nil {
		return nil, err
	}
	var projects []*apisv1.ProjectBase
	for _, entity := range entitys {
		project := entity.(*model.Project)
		projects = append(projects, convertProjectModel2Base(project))
	}
	return projects, nil
}

// ListProjects list projects
func (p *projectUsecaseImpl) ListProjects(ctx context.Context) ([]*apisv1.ProjectBase, error) {
	return listProjects(ctx, p.ds)
}

// DeleteProject delete a project
func (p *projectUsecaseImpl) DeleteProject(ctx context.Context, name string) error {

	// TODO(@wonderflow): it's not supported for delete a project now, just used in test
	// we should prevent delete a project that contain any application/env inside.

	return p.ds.Delete(ctx, &model.Project{Name: name})
}

// CreateProject create project
func (p *projectUsecaseImpl) CreateProject(ctx context.Context, req apisv1.CreateProjectRequest) (*apisv1.ProjectBase, error) {

	exist, err := p.ds.IsExist(ctx, &model.Project{Name: req.Name})
	if err != nil {
		log.Logger.Errorf("check project name is exist failure %s", err.Error())
		return nil, bcode.ErrProjectIsExist
	}
	if exist {
		return nil, bcode.ErrProjectIsExist
	}

	newProject := &model.Project{
		Name:        req.Name,
		Description: req.Description,
		Alias:       req.Alias,
	}

	if err := p.ds.Add(ctx, newProject); err != nil {
		return nil, err
	}

	return &apisv1.ProjectBase{
		Name:        newProject.Name,
		Alias:       newProject.Alias,
		Description: newProject.Description,
		CreateTime:  newProject.CreateTime,
		UpdateTime:  newProject.UpdateTime,
	}, nil
}

func convertProjectModel2Base(project *model.Project) *apisv1.ProjectBase {
	return &apisv1.ProjectBase{
		Name:        project.Name,
		Description: project.Description,
		Alias:       project.Alias,
		CreateTime:  project.CreateTime,
		UpdateTime:  project.UpdateTime,
	}
}
