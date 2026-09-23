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

package sources

import (
	"time"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"

	"k8s.io/klog/v2"
)

func (r *sourceResolver) readSourceCache(cacheKey string, ttl time.Duration) (map[string]interface{}, bool, bool, time.Time, error) {
	if r.cacheStore == nil || cacheKey == "" {
		return nil, false, false, time.Time{}, nil
	}
	return r.cacheStore.Read(r.goCtx, cacheKey, ttl)
}

func (r *sourceResolver) writeSourceCache(cacheKey, sourceType string, data map[string]interface{},
	ttl time.Duration, keyInputs []string, identity identityInputs) error {
	if r.cacheStore == nil || cacheKey == "" {
		return nil
	}
	namespace, _ := r.ctxValues[velaprocess.ContextNamespace].(string)
	meta := velaprocess.SourceCacheWriteMeta{
		TTL:                ttl,
		SourceDefName:      sourceType,
		SourceDefNamespace: namespace,
		TemplateName:       sourceCacheTemplateName(sourceType, r.sourceSchemas[sourceType]),
		KeyInputs:          keyInputs,
		Context:            identity.Context,
		Properties:         identity.Properties,
		TemplateHash:       identity.Template,
	}
	return r.cacheStore.Write(r.goCtx, cacheKey, sourceType, data, meta)
}

// touchSourceCache advances the last-accessed marker for a stale entry that is
// being served, if the backing store supports it. Failures are non-fatal: a
// missed touch only risks the sweep collecting a still-used entry one cycle
// early, which the next render re-creates.
func (r *sourceResolver) touchSourceCache(cacheKey string) {
	if r.cacheStore == nil || cacheKey == "" {
		return
	}
	toucher, ok := r.cacheStore.(velaprocess.SourceCacheToucher)
	if !ok {
		return
	}
	if err := toucher.Touch(r.goCtx, cacheKey); err != nil {
		klog.Warningf("touch source cache failed for %s: %v", cacheKey, err)
	}
}

// formatExpiry renders a cache expiry, and renders nothing when there is not
// one. A source whose definition sets no storageTTL has a zero time, and
// formatting that put "0001-01-01T00:00:00Z" into status - a date, where the
// honest answer is silence.
func formatExpiry(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
