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
	"strconv"

	"github.com/sirupsen/logrus"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func (c *CR2UX) shouldSync(ctx context.Context, targetApp *v1beta1.Application, del bool) bool {
	key := formatAppComposedName(targetApp.Name, targetApp.Namespace)
	ann, ok := c.cache.Load(key)
	if ok {
		sann := ann.(string)
		if sann == strconv.FormatInt(targetApp.Generation, 10) && !del {
			logrus.Infof("app %s %s generation is %v hasn't updated, will not sync..", targetApp.Name, targetApp.Namespace, targetApp.Generation)
			return false
		}
		if del {
			c.cache.Delete(key)
		}
	}

	sot := CheckSoTFromCR(targetApp)

	// This is a double check to make sure the app not be converted and un-deployed
	sot = CheckSoTFromAppMeta(ctx, c.ds, targetApp.Name, targetApp.Namespace, sot)

	switch sot {
	case FromUX, FromInner:
		// we don't sync if the application is not created from CR
		return false
	default:
	}
	return true
}

func (c *CR2UX) updateCache(key string, generation int64) {
	// update cache
	c.cache.Store(key, strconv.FormatInt(generation, 10))
}
