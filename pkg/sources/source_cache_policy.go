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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	upstreamcuex "github.com/kubevela/pkg/cue/cuex"

	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/pkg/errors"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/cue/render"
	"github.com/oam-dev/kubevela/pkg/definition/cachekey"
)

type sourceCachePolicy struct {
	Key string
	// KeyInputs names the values folded into the identity hash, as generated
	// alongside the key. Recorded rather than re-derived, so inference stays a
	// build-time concern.
	KeyInputs      []string
	TTL            time.Duration
	OnStaleFailure string
}

func (r *sourceResolver) resolveCachePolicy(sourceName, sourceType, sourceTemplate string, props map[string]interface{}) (sourceCachePolicy, error) {
	policy := sourceCachePolicy{
		TTL:            sourceCacheTTL,
		OnStaleFailure: sourceCachePolicyUseStale,
	}
	if sourceTemplate == "" {
		return policy, fmt.Errorf("source definition %q has no cue template", sourceType)
	}
	paramFile := velaprocess.ParameterFieldName + ": {}"
	if len(props) > 0 {
		if raw, err := json.Marshal(props); err == nil {
			paramFile = fmt.Sprintf("%s: %s", velaprocess.ParameterFieldName, string(raw))
		}
	}
	c, err := sourceContext(r.ctxValues, sourceName, r.surface)
	if err != nil {
		return policy, err
	}
	// storage: is pure interpolation over context and parameter values, so it is
	// resolved WITHOUT running provider functions. Resolving them here would
	// perform the very I/O the cache exists to avoid - on every reconcile, before
	// the cache is even consulted.
	src := strings.Join([]string{render.Template(sourceTemplate), paramFile, c}, "\n")

	// This source text is the policy's whole input, so the policy it produces can
	// be kept against it. See policyCache for why that is worth doing.
	if cached, ok := lookupCachedPolicy(src); ok {
		return cached, nil
	}

	val, err := r.compiler.CompileStringWithOptions(r.goCtx, src,
		upstreamcuex.DisableResolveProviderFunctions{})
	if err != nil {
		return policy, errors.WithMessagef(err, "evaluate storage block for source %q", sourceName)
	}
	// The generated block. It is written by `vela def` and re-derived at
	// admission, so a definition that reached the cluster always has one.
	internal := val.LookupPath(value.FieldPath(cachekey.InternalField))
	if !internal.Exists() {
		return policy, fmt.Errorf("source definition %q has no %s block; apply it with `vela def apply` "+
			"so the cache key is generated", sourceType, cachekey.InternalField)
	}

	cacheKey := ""
	if err := internal.LookupPath(value.FieldPath(cachekey.KeyField)).Decode(&cacheKey); err != nil {
		return policy, errors.WithMessagef(err, "resolve %s.%s for source %q",
			cachekey.InternalField, cachekey.KeyField, sourceName)
	}
	if err := cachekey.ValidateCacheKey(cacheKey); err != nil {
		return policy, errors.WithMessagef(err, "source %q", sourceName)
	}
	policy.Key = cacheKey

	// Generated alongside the key and equally required: without it the identity
	// hash loses every context dimension, so two Applications reading the same
	// source from different namespaces would share one cache entry. Admission
	// refuses a definition missing it, so reaching here means the block was
	// edited by hand after it was stamped.
	var keyInputs []string
	if err := internal.LookupPath(value.FieldPath(cachekey.KeyInputsField)).Decode(&keyInputs); err != nil {
		return policy, errors.WithMessagef(err, "resolve %s.%s for source %q",
			cachekey.InternalField, cachekey.KeyInputsField, sourceName)
	}
	policy.KeyInputs = keyInputs

	// storage: is authored and entirely optional - a source with no caching
	// preferences declares nothing.
	storage := val.LookupPath(value.FieldPath("storage"))

	// A field that is present but of the wrong type is an authoring mistake, not
	// an absent preference. Defaulting it would quietly change how fresh a source
	// is, or turn a definition that asked to fail on stale data into one that
	// serves it.
	ttlRaw := ""
	if f := storage.LookupPath(value.FieldPath("storageTTL")); f.Exists() {
		if err := f.Decode(&ttlRaw); err != nil {
			return policy, errors.WithMessagef(err, "source %q has an invalid storageTTL", sourceName)
		}
	}
	if ttlRaw != "" {
		ttl, err := time.ParseDuration(ttlRaw)
		if err != nil {
			return policy, fmt.Errorf("source %q has an invalid storageTTL %q: %w", sourceName, ttlRaw, err)
		}
		if ttl <= 0 {
			return policy, fmt.Errorf("source %q has a non-positive storageTTL %q", sourceName, ttlRaw)
		}
		policy.TTL = ttl
	}

	onStaleFailure := ""
	if f := storage.LookupPath(value.FieldPath("onStaleFailure")); f.Exists() {
		if err := f.Decode(&onStaleFailure); err != nil {
			return policy, errors.WithMessagef(err, "source %q has an invalid onStaleFailure", sourceName)
		}
	}
	if onStaleFailure != "" {
		switch onStaleFailure {
		case sourceCachePolicyUseStale, sourceCachePolicyFail:
			policy.OnStaleFailure = onStaleFailure
		default:
			return policy, fmt.Errorf("source %q has an unknown onStaleFailure %q: expected %q or %q",
				sourceName, onStaleFailure, sourceCachePolicyUseStale, sourceCachePolicyFail)
		}
	}

	storeCachedPolicy(src, policy)
	return policy, nil
}
