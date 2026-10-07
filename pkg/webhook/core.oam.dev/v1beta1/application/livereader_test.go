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
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// A definition the cache has not caught up with is read live, because an
// Application may be applied in the same breath as the definitions it names.
// A stale cache reports a definition as absent, and an absent definition is not
// abstract, so the marking would be skipped exactly when two objects arrive
// together.
func TestADefinitionMissingFromTheCacheIsReadLive(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)

	// The cached client has a stale view: no definitions at all.
	stale := fake.NewClientBuilder().WithScheme(scheme).Build()
	// The live one sees what was just written.
	live := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(abstractDef("team-a", "base")).Build()

	h := &ValidatingHandler{Client: stale, Live: live}

	errs := h.ValidateAbstractTypes(context.Background(), appNaming("team-a", "base"))
	require.Len(t, errs, 1, "the miss was retried against the API server")
	require.Contains(t, errs[0].Error(), "abstract and cannot be used directly")
}

// With no live client the cached one answers alone, which is what a test
// supplying a fake wants and what a manager that could not build an uncached
// client falls back to.
func TestTheCachedClientAnswersAloneWithoutALiveOne(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)

	cached := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(abstractDef("team-a", "base")).Build()

	h := &ValidatingHandler{Client: cached}

	require.Len(t, h.ValidateAbstractTypes(context.Background(), appNaming("team-a", "base")), 1)
}
