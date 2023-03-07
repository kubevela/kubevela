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
	"errors"
	"strings"

	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	"github.com/oam-dev/kubevela/pkg/oam"
)

type cached struct {
	revision string
	targets  int64
}

// initCache will initialize the cache
func (c *CR2UX) initCache(ctx context.Context) error {
	appsRaw, err := c.ds.List(ctx, &model.Application{}, &datastore.ListOptions{})
	if err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil
		}
		return err
	}
	for _, appR := range appsRaw {
		app, ok := appR.(*model.Application)
		if !ok {
			continue
		}
		revision, ok := app.Labels[model.LabelSyncRevision]
		if !ok {
			continue
		}
		namespace := app.Labels[model.LabelSyncNamespace]
		var key = formatAppComposedName(app.Name, namespace)
		if strings.HasSuffix(app.Name, namespace) {
			key = app.Name
		}

		// we should check targets if we synced from app status
		c.syncCache(key, revision, 0)
	}
	return nil
}

func (c *CR2UX) shouldSync(ctx context.Context, targetApp *v1beta1.Application, del bool) bool {
	if targetApp != nil && targetApp.Labels != nil {
		// the source is inner and is not the addon application, ignore it.
		if targetApp.Labels[types.LabelSourceOfTruth] == types.FromInner && targetApp.Labels[oam.LabelAddonName] == "" {
			return false
		}
		// the source is UX, ignore it
		if targetApp.Labels[types.LabelSourceOfTruth] == types.FromUX {
			return false
		}
	}

	// if no LabelSourceOfTruth label, it means the app is existing ones, check the existing labels and annotations
	if targetApp.Annotations != nil {
		if _, exist := targetApp.Annotations[oam.AnnotationAppName]; exist {
			return false
		}
	}

	key := formatAppComposedName(targetApp.Name, targetApp.Namespace)
	cachedData, ok := c.cache.Load(key)
	if ok {
		cd := cachedData.(*cached)
		// if app meta not exist, we should ignore the cache
		_, _, err := c.getApp(ctx, targetApp.Name, targetApp.Namespace)
		if del || err != nil {
			c.cache.Delete(key)
		} else if cd.revision == getRevision(*targetApp) {
			klog.V(5).Infof("app %s/%s with resource revision(%v) hasn't updated, ignore the sync event..", targetApp.Name, targetApp.Namespace, targetApp.ResourceVersion)
			return false
		}
	}
	return true
}

func (c *CR2UX) syncCache(key string, revision string, targets int64) {
	// update cache
	c.cache.Store(key, &cached{revision: revision, targets: targets})
}
