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
	"sync"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const paramCacheTemplate = `
#Port: {port: int, name?: string}

parameter: {
	image: string
	replicas: *1 | int
	ports?: [...#Port]
}

output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
}
`

// TestParameterBlockCompilesFreshEachTime is the safety property, and the reason
// only the reduced text is cached rather than the compiled value.
//
// cue documents that values created from the same Context are not safe for
// concurrent use. Admission requests are concurrent, so handing two of them the
// same cue.Value would be a data race - the caching equivalent of saving a few
// microseconds by corrupting the evaluator.
func TestParameterBlockCompilesFreshEachTime(t *testing.T) {

	first, ok := parameterBlockOnly(paramCacheTemplate)
	require.True(t, ok)
	second, ok := parameterBlockOnly(paramCacheTemplate)
	require.True(t, ok)

	assert.NotSame(t, first, second, "each call must get its own compiled value")

	// Same answers from both, which is what makes sharing the text sound.
	for _, c := range []*cueStruct{first, second} {
		kind, declared := c.kindAt(segs("image"))
		require.True(t, declared)
		assert.Equal(t, cue.StringKind, kind)
		kind, declared = c.kindAt(segs("replicas"))
		require.True(t, declared)
		assert.Equal(t, cue.IntKind, kind)
	}
}

// TestParameterBlockSourceIsKeyedOnTheTemplate guards the cache key. Two
// definitions sharing a cached parameter block would type-check an Application
// against the wrong contract.
func TestParameterBlockSourceIsKeyedOnTheTemplate(t *testing.T) {
	const other = `
parameter: {
	bucket: string
	region: string
}
`
	a, ok := parameterBlockSource(paramCacheTemplate)
	require.True(t, ok)
	b, ok := parameterBlockSource(other)
	require.True(t, ok)

	assert.NotEqual(t, a, b, "different templates must not share an entry")
	assert.Contains(t, a, "image")
	assert.Contains(t, b, "bucket")
	assert.NotContains(t, b, "image")

	// Repeats are served from the cache and must not have changed.
	again, ok := parameterBlockSource(paramCacheTemplate)
	require.True(t, ok)
	assert.Equal(t, a, again)
}

// TestParameterBlockRemembersAbsence covers the negative entry. A template with
// no parameter block costs the same parse to discover that, so the answer is
// worth keeping too.
func TestParameterBlockRemembersAbsence(t *testing.T) {
	const noParams = `
output: {
	apiVersion: "v1"
	kind:       "ConfigMap"
}
`
	for i := 0; i < 3; i++ {
		_, ok := parameterBlockSource(noParams)
		assert.False(t, ok, "a template with no parameter block must report so every time")
	}
	_, ok := parameterBlockOnly(noParams)
	assert.False(t, ok)
}

// TestParameterBlockIsConcurrencySafe exercises the shape admission uses. Under
// -race this is the real assertion.
func TestParameterBlockIsConcurrencySafe(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, ok := parameterBlockOnly(paramCacheTemplate)
			if !ok {
				t.Error("extraction failed under concurrency")
				return
			}
			if _, declared := c.kindAt(segs("image")); !declared {
				t.Error("compiled value was unusable")
			}
		}()
	}
	wg.Wait()
}
