/*
Copyright 2026 The KubeVela Authors.

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

package application

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/cache"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// heldUpdateTTL bounds how long a held verdict is reused, so a live version held because the
// cluster data it reads made it fail is tried again.
const heldUpdateTTL = 10 * time.Minute

// heldUpdates remembers the policy updates an Application held, so a known held update does not
// render again on every reconcile that renders the policies.
var heldUpdates = heldUpdateCache{entries: cache.NewLRUExpireCache(10000)}

type heldUpdateCache struct {
	entries *cache.LRUExpireCache
}

// heldVerdict is why a policy update was held.
type heldVerdict struct {
	// failsToRender is set when the live version fails to render in the sequence, rather than
	// being held by its apply-policy-updates setting.
	failsToRender bool
}

// held returns the verdict recorded for the live version of the named policy for app at rev, and
// whether one is recorded. A removed policy (no live version) has nothing to render and is never
// recorded.
func (c heldUpdateCache) held(app *v1beta1.Application, rev *v1beta1.ApplicationRevision, name string, liveDef *v1beta1.PolicyDefinition) (heldVerdict, bool) {
	if liveDef == nil {
		return heldVerdict{}, false
	}
	v, ok := c.entries.Get(heldUpdateKey(app, rev, name, liveDef))
	if !ok {
		return heldVerdict{}, false
	}
	verdict, _ := v.(heldVerdict)
	return verdict, true
}

func (c heldUpdateCache) hold(app *v1beta1.Application, rev *v1beta1.ApplicationRevision, name string, liveDef *v1beta1.PolicyDefinition, verdict heldVerdict) {
	if liveDef != nil {
		c.entries.Add(heldUpdateKey(app, rev, name, liveDef), verdict, heldUpdateTTL)
	}
}

// heldUpdateKey identifies what a held verdict depends on: the app's spec and metadata, the
// revision it is pinned to, and the live version of the policy with the revision the render reads
// into its context.
func heldUpdateKey(app *v1beta1.Application, rev *v1beta1.ApplicationRevision, name string, liveDef *v1beta1.PolicyDefinition) string {
	appHash, _ := apply.ComputeSpecHash(struct {
		Spec        v1beta1.ApplicationSpec
		Labels      map[string]string
		Annotations map[string]string
	}{app.Spec, app.Labels, app.Annotations})
	liveHash, _ := apply.ComputeSpecHash(struct {
		Spec           v1beta1.PolicyDefinitionSpec
		LatestRevision *common.Revision
	}{liveDef.Spec, liveDef.Status.LatestRevision})
	return fmt.Sprintf("%s/%s/%s/%s/%s/%s/%s", app.Namespace, app.Name, app.UID, appHash, rev.Name, name, liveHash)
}
