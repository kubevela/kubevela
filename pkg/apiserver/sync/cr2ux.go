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
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const (
	// FromCR means the data source of truth is from k8s CR
	FromCR = "from-CR"
	// FromUX means the data source of truth is from velaux data store
	FromUX = "from-UX"
	// FromInner means the data source of truth is from KubeVela inner usage, such as addon or configuration that don't want to be synced
	FromInner = "from-inner"

	// SoT means the source of Truth from
	SoT = "SourceOfTruth"
)

// CheckSoTFromCR will check the source of truth of the application
func CheckSoTFromCR(targetApp *v1beta1.Application) string {

	if _, innerUse := targetApp.Annotations[oam.AnnotationSOTFromInner]; innerUse {
		return FromInner
	}
	if _, appName := targetApp.Annotations[oam.AnnotationAppName]; appName {
		return FromUX
	}
	return FromCR
}

// CheckSoTFromAppMeta will check the source of truth marked in datastore
func CheckSoTFromAppMeta(ctx context.Context, ds datastore.DataStore, appName, namespace string, sotFromCR string) string {

	app := &model.Application{Name: formatAppComposedName(appName, namespace)}
	err := ds.Get(ctx, app)
	if err != nil {
		app = &model.Application{Name: appName}
		err = ds.Get(ctx, app)
		if err != nil {
			return sotFromCR
		}
	}
	if app.Labels == nil || app.Labels[SoT] == "" {
		return sotFromCR
	}
	return app.Labels[SoT]
}

// CR2UX provides the Add/Update/Delete method
type CR2UX struct {
	ds    datastore.DataStore
	cli   client.Client
	cache sync.Map
}

func formatAppComposedName(name, namespace string) string {
	return name + "-" + namespace
}

// we need to prevent the case that one app is deleted ant it's name is pure appName, then other app with namespace suffix will be mixed
func (c *CR2UX) getAppMetaName(ctx context.Context, name, namespace string) string {
	alreadyCreated := &model.Application{Name: formatAppComposedName(name, namespace)}
	err := c.ds.Get(ctx, alreadyCreated)
	if err == nil {
		return formatAppComposedName(name, namespace)
	}

	// check if it's created the first in database
	existApp := &model.Application{Name: name}
	err = c.ds.Get(ctx, existApp)
	if err == nil {
		en := existApp.Labels[model.LabelSyncNamespace]
		if en != namespace {
			return formatAppComposedName(name, namespace)
		}
	}
	return name
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

	if err = StoreProject(ctx, dsApp.AppMeta.Project, ds); err != nil {
		log.Logger.Errorf("get or create project for sync process err %v", err)
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
	/*
		if err = StoreTargets(ctx, dsApp, ds); err != nil {
			log.Logger.Errorf("Store targets to data store err %v", err)
			return err
		}

	*/

	if err = StoreAppMeta(ctx, dsApp, ds); err != nil {
		log.Logger.Errorf("Store App Metadata to data store err %v", err)
		return err
	}

	// update cache
	c.updateCache(dsApp.AppMeta.PrimaryKey(), targetApp.Generation, int64(len(dsApp.Targets)))
	return nil
}

// DeleteApp will delete the application as the CR was deleted
func (c *CR2UX) DeleteApp(ctx context.Context, targetApp *v1beta1.Application) error {
	ds := c.ds

	if !c.shouldSync(ctx, targetApp, true) {
		return nil
	}
	appName := c.getAppMetaName(ctx, targetApp.Name, targetApp.Namespace)

	_ = ds.Delete(ctx, &model.Application{Name: appName})

	cmps, err := ds.List(ctx, &model.ApplicationComponent{AppPrimaryKey: appName}, &datastore.ListOptions{})
	if err != nil {
		return err
	}
	for _, entity := range cmps {
		comp := entity.(*model.ApplicationComponent)
		if comp.Creator == model.AutoGenComp {
			_ = ds.Delete(ctx, comp)
		}
	}

	plcs, err := ds.List(ctx, &model.ApplicationPolicy{AppPrimaryKey: appName}, &datastore.ListOptions{})
	if err != nil {
		return err
	}
	for _, entity := range plcs {
		comp := entity.(*model.ApplicationPolicy)
		if comp.Creator == model.AutoGenPolicy {
			_ = ds.Delete(ctx, comp)
		}
	}

	_ = ds.Delete(ctx, &model.Workflow{Name: model.AutoGenWorkflowNamePrefix + appName})

	return nil
}
