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

package sync

import (
	"context"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/service"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// CheckSoTFromCR will check the source of truth of the application
func CheckSoTFromCR(targetApp *v1beta1.Application) string {
	if sot := targetApp.Annotations[model.LabelSourceOfTruth]; sot != "" {
		return sot
	}
	// if no LabelSourceOfTruth label, it means the app is existing ones, check the existing labels and annotations
	if _, appName := targetApp.Annotations[oam.AnnotationAppName]; appName {
		return model.FromUX
	}
	// no labels mean it's created by K8s resources.
	return model.FromCR
}

// CheckSoTFromAppMeta will check the source of truth marked in datastore
func (c *CR2UX) CheckSoTFromAppMeta(ctx context.Context, appName, namespace string, sotFromCR string) string {

	app, _, err := c.getApp(ctx, appName, namespace)
	if err != nil {
		return sotFromCR
	}
	if app.Labels == nil || app.Labels[model.LabelSourceOfTruth] == "" {
		return sotFromCR
	}
	return app.Labels[model.LabelSourceOfTruth]
}

// getApp will return the app and appname if exists
func (c *CR2UX) getApp(ctx context.Context, name, namespace string) (*model.Application, string, error) {
	alreadyCreated := &model.Application{Name: formatAppComposedName(name, namespace)}
	err1 := c.ds.Get(ctx, alreadyCreated)
	if err1 == nil {
		return alreadyCreated, alreadyCreated.Name, nil
	}

	// check if it's created the first in database
	existApp := &model.Application{Name: name}
	err2 := c.ds.Get(ctx, existApp)
	if err2 == nil {
		en := existApp.Labels[model.LabelSyncNamespace]
		// it means the namespace/app is not created yet, the appname is occupied by app from other namespace
		if en != namespace {
			return nil, formatAppComposedName(name, namespace), err1
		}
		return existApp, name, nil
	}
	return nil, name, err2
}

// CR2UX provides the Add/Update/Delete method
type CR2UX struct {
	ds                 datastore.DataStore
	cli                client.Client
	cache              sync.Map
	projectService     service.ProjectService
	applicationService service.ApplicationService
}

func formatAppComposedName(name, namespace string) string {
	return name + "-" + namespace
}

// we need to prevent the case that one app is deleted ant it's name is pure appName, then other app with namespace suffix will be mixed
func (c *CR2UX) getAppMetaName(ctx context.Context, name, namespace string) string {
	_, appName, _ := c.getApp(ctx, name, namespace)
	return appName
}

// AddOrUpdate will sync application CR to storage of VelaUX automatically
func (c *CR2UX) AddOrUpdate(ctx context.Context, targetApp *v1beta1.Application) error {
	ds := c.ds
	if !c.shouldSync(ctx, targetApp, false) {
		return nil
	}

	dsApp, err := c.ConvertApp2DatastoreApp(ctx, targetApp)
	if err != nil {
		log.Logger.Errorf("Convert App to data store err %v", err)
		return err
	}
	if err = StoreProject(ctx, dsApp.AppMeta.Project, ds, c.projectService); err != nil {
		log.Logger.Errorf("get or create project for sync process err %v", err)
		return err
	}

	if err = StoreTargets(ctx, dsApp, ds); err != nil {
		log.Logger.Errorf("Store targets to data store err %v", err)
		return err
	}

	if err = StoreEnv(ctx, dsApp, ds); err != nil {
		log.Logger.Errorf("Store Env Metadata to data store err %v", err)
		return err
	}
	if err = StoreEnvBinding(ctx, dsApp.Eb, ds); err != nil {
		log.Logger.Errorf("Store EnvBinding Metadata to data store err %v", err)
		return err
	}
	if err = StoreComponents(ctx, dsApp.AppMeta.Name, dsApp.Comps, ds); err != nil {
		log.Logger.Errorf("Store Components Metadata to data store err %v", err)
		return err
	}
	if err = StorePolicy(ctx, dsApp.AppMeta.Name, dsApp.Policies, ds); err != nil {
		log.Logger.Errorf("Store Policy Metadata to data store err %v", err)
		return err
	}
	if err = StoreWorkflow(ctx, dsApp, ds); err != nil {
		log.Logger.Errorf("Store Workflow Metadata to data store err %v", err)
		return err
	}

	if err = StoreAppMeta(ctx, dsApp, ds); err != nil {
		log.Logger.Errorf("Store App Metadata to data store err %v", err)
		return err
	}

	// update cache
	c.syncCache(dsApp.AppMeta.PrimaryKey(), targetApp.Generation, int64(len(dsApp.Targets)))
	return nil
}

// DeleteApp will delete the application as the CR was deleted
func (c *CR2UX) DeleteApp(ctx context.Context, targetApp *v1beta1.Application) error {
	if !c.shouldSync(ctx, targetApp, true) {
		return nil
	}
	app, appName, err := c.getApp(ctx, targetApp.Name, targetApp.Namespace)
	if err != nil {
		return err
	}
	// Only for the unit test scenario
	if c.applicationService == nil {
		return c.ds.Delete(ctx, &model.Application{Name: appName})
	}
	return c.applicationService.DeleteApplication(ctx, app)
}
