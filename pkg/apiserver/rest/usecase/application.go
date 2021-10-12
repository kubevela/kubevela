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
	"encoding/json"
	"errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

// ApplicationUsecase application usecase
type ApplicationUsecase interface {
	CreateApplication(context.Context, apisv1.CreateApplicationRequest) (*apisv1.ApplicationBase, error)
}

type applicationUsecaseImpl struct {
	ds datastore.DataStore
}

// NewApplicationUsecase new cluster usecase
func NewApplicationUsecase(ds datastore.DataStore) ApplicationUsecase {
	return &applicationUsecaseImpl{ds: ds}
}

// CreateApplication create application
func (c *applicationUsecaseImpl) CreateApplication(ctx context.Context, req apisv1.CreateApplicationRequest) (*apisv1.ApplicationBase, error) {
	application := model.Application{
		Name:        req.Name,
		Description: req.Description,
		Icon:        req.Icon,
		Labels:      req.Labels,
		ClusterList: req.ClusterList,
	}
	// check clusters.

	// check can deploy
	var canDeploy bool
	if req.YamlConfig != "" {
		var oamApp v1beta1.Application
		if err := json.Unmarshal([]byte(req.YamlConfig), &oamApp); err != nil {
			log.Logger.Errorf("application yaml config is invalid,%s", err.Error())
			return nil, bcode.ErrApplicationConfig
		}
		// TODO: check oam spec

		// TODO: split the configuration and store it in the database.

		canDeploy = true
	}

	// add to db.
	if err := c.ds.Add(ctx, &application); err != nil {
		if errors.Is(err, datastore.ErrRecordExist) {
			return nil, bcode.ErrApplicationExist
		}
		return nil, err
	}
	// render app base info.
	base := c.renderAppBase(&application)
	// deploy to cluster if need.
	if req.Deploy && canDeploy {
		if err := c.Deploy(ctx, req.Name); err != nil {
			return nil, err
		}
	}
	return base, nil
}

// Deploy deploy app to cluster
// means to render oam application config and apply to cluster.
// An event record is generated for each deploy.
func (c *applicationUsecaseImpl) Deploy(ctx context.Context, appName string) error {
	// TODO:
	return nil
}

func (c *applicationUsecaseImpl) renderAppBase(app *model.Application) *apisv1.ApplicationBase {
	appBeas := &apisv1.ApplicationBase{
		Name:        app.Name,
		Description: app.Description,
		Icon:        app.Icon,
		Labels:      app.Labels,
	}
	// TODO: get and render app status
	return appBeas
}
