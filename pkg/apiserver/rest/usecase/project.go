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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// ProjectUsecase project manage usecase.
type ProjectUsecase interface {
	GetProject(ctx context.Context, projectName string) (*model.Project, error)
	ListProjects(ctx context.Context) ([]*apisv1.ProjectBase, error)
	CreateProject(ctx context.Context, req apisv1.CreateProjectRequest) (*apisv1.ProjectBase, error)
}

type projectUsecaseImpl struct {
	ds         datastore.DataStore
	kubeClient client.Client
}

// NewProjectUsecase new project usecase
func NewProjectUsecase(ds datastore.DataStore) ProjectUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("get kubeclient failure %s", err.Error())
	}
	return &projectUsecaseImpl{kubeClient: kubecli, ds: ds}
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

// ListProjects list projects
func (p *projectUsecaseImpl) ListProjects(ctx context.Context) ([]*apisv1.ProjectBase, error) {
	var project = model.Project{}
	entitys, err := p.ds.List(ctx, &project, &datastore.ListOptions{SortBy: []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}}})
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

	new := &model.Project{
		Name:        req.Name,
		Description: req.Description,
		Alias:       req.Alias,
		Namespace:   fmt.Sprintf("project-%s", req.Name),
	}

	// create namespace at first
	if err := p.kubeClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: new.Namespace,
			Labels: map[string]string{
				oam.LabelProjectNamesapce: new.Name,
			},
		},
		Spec: corev1.NamespaceSpec{},
	}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, err
		}
	}

	if err := p.ds.Add(ctx, new); err != nil {
		return nil, err
	}

	return &apisv1.ProjectBase{
		Name:        new.Name,
		Alias:       new.Alias,
		Namespace:   new.Namespace,
		Description: new.Description,
		CreateTime:  new.CreateTime,
		UpdateTime:  new.UpdateTime,
	}, nil
}

func convertProjectModel2Base(project *model.Project) *apisv1.ProjectBase {
	return &apisv1.ProjectBase{
		Name:        project.Name,
		Namespace:   project.Namespace,
		Description: project.Description,
		Alias:       project.Alias,
		CreateTime:  project.CreateTime,
		UpdateTime:  project.UpdateTime,
	}
}
