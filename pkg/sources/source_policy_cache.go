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
	"k8s.io/utils/lru"
)

// policyCacheSize bounds the number of distinct (template, properties, context)
// combinations kept.
const policyCacheSize = 512

// policyCache holds resolved cache policies against the CUE source that produced
// them.
//
// A policy is a pure function of that source - the template, the binding's
// properties and the readable context - with provider functions disabled, so
// there is no I/O and no clock. The text is therefore not an approximation of
// the inputs; it is the inputs.
//
// Keyed on the text rather than a hash of it. A collision here would send a
// lookup to another binding's entry, which is a wrong answer rather than a slow
// one.
//
// lru.Cache locks internally, so this needs no mutex.
var policyCache = lru.New(policyCacheSize)

// lookupCachedPolicy returns a previously resolved policy for this source.
//
// The returned policy owns its KeyInputs, so a caller cannot reach back into the
// cache through it.
func lookupCachedPolicy(src string) (sourceCachePolicy, bool) {
	hit, ok := policyCache.Get(src)
	if !ok {
		return sourceCachePolicy{}, false
	}
	policy, ok := hit.(sourceCachePolicy)
	if !ok {
		return sourceCachePolicy{}, false
	}
	policy.KeyInputs = append([]string(nil), policy.KeyInputs...)
	return policy, true
}

// storeCachedPolicy records a resolved policy. Only successes are stored: an
// error would outlive the condition that caused it.
func storeCachedPolicy(src string, policy sourceCachePolicy) {
	policy.KeyInputs = append([]string(nil), policy.KeyInputs...)
	policyCache.Add(src, policy)
}
