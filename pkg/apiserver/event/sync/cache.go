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
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
)

type cached struct {
	generation int64
	targets    int64
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
		gen, ok := app.Labels[model.LabelSyncGeneration]
		if !ok || gen == "" {
			continue
		}
		namespace := app.Labels[model.LabelSyncNamespace]
		var key = formatAppComposedName(app.Name, namespace)
		if strings.HasSuffix(app.Name, namespace) {
			key = app.Name
		}
		generation, _ := strconv.ParseInt(gen, 10, 64)

		// we should check targets if we synced from app status
		c.syncCache(key, generation, 0)
	}
	return nil
}

func (c *CR2UX) shouldSync(ctx context.Context, targetApp *v1beta1.Application, del bool) bool {

	if targetApp != nil && targetApp.Labels != nil && targetApp.Labels[model.LabelSourceOfTruth] == model.FromInner {
		return false
	}

	key := formatAppComposedName(targetApp.Name, targetApp.Namespace)
	cachedData, ok := c.cache.Load(key)
	if ok {
		cd := cachedData.(*cached)

		// TODO(wonderflow): we should check targets if we sync that, it can avoid missing the status changed for targets updated in multi-cluster deploy, e.g. resumed suspend case.

		// if app meta not exist, we should ignore the cache
		_, _, err := c.getApp(ctx, targetApp.Name, targetApp.Namespace)
		if del || err != nil {
			c.cache.Delete(key)
		} else if cd.generation == targetApp.Generation {
			logrus.Infof("app %s/%s with generation(%v) hasn't updated, ignore the sync event..", targetApp.Name, targetApp.Namespace, targetApp.Generation)
			return false
		}
	}

	// This is a double check to make sure the app not be converted and un-deployed
	sot := c.CheckSoTFromAppMeta(ctx, targetApp.Name, targetApp.Namespace, CheckSoTFromCR(targetApp))
	switch sot {
	case model.FromUX:
		// we don't sync if the application is not created from CR
		return false
	case model.FromInner:
		// we don't sync if the application is not created from CR
		return false
	case model.FromCR:
	default:
	}
	return true
}

func (c *CR2UX) syncCache(key string, generation, targets int64) {
	// update cache
	c.cache.Store(key, &cached{generation: generation, targets: targets})
}
