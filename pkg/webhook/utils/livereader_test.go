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

package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

// counter records how often a client was asked to read, so a test can say not
// just what an answer was but where it came from.
type counter struct {
	client.Client
	gets, lists int
}

func (c *counter) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	c.gets++
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *counter) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	c.lists++
	return c.Client.List(ctx, list, opts...)
}

func componentDef(namespace, name string) *v1beta1.ComponentDefinition {
	return &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
	}
}

func readerScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(s))
	return s
}

// lookUp is the read a definition webhook actually performs: the app's
// namespace first, then the system one.
func lookUp(appNS, name string) func(client.Client) error {
	return func(c client.Client) error {
		ctx := oamutil.SetNamespaceInCtx(context.Background(), appNS)
		return oamutil.GetDefinition(ctx, c, &v1beta1.ComponentDefinition{}, name)
	}
}

// The retry is per lookup, not per Get. GetDefinition tries the Application's
// namespace before the system one, so in the usual layout — definitions in
// vela-system, the app elsewhere — its first Get is a guaranteed miss. Retrying
// that Get live would cost a round trip on every lookup, and admission is on the
// whole write path, including the updates a controller makes while reconciling.
func TestALookupSatisfiedFromAnotherNamespaceNeverGoesLive(t *testing.T) {
	s := readerScheme(t)
	cached := &counter{Client: fake.NewClientBuilder().WithScheme(s).
		WithObjects(componentDef("vela-system", "base")).Build()}
	live := &counter{Client: fake.NewClientBuilder().WithScheme(s).
		WithObjects(componentDef("vela-system", "base")).Build()}

	require.NoError(t, ReadWithLiveRetry(cached, live, lookUp("team-a", "base")))
	require.Greater(t, cached.gets, 1, "the cache was asked in both namespaces")
	require.Zero(t, live.gets, "the cache had the answer, in its second namespace")
}

// A lookup that finds nothing anywhere is the case the live read exists for: a
// parent written milliseconds before its child has not reached the cache, and
// refusing the child for naming a definition that is plainly there is the bug
// this fixes.
func TestALookupThatFindsNothingIsRetriedLive(t *testing.T) {
	s := readerScheme(t)
	cached := &counter{Client: fake.NewClientBuilder().WithScheme(s).Build()}
	live := &counter{Client: fake.NewClientBuilder().WithScheme(s).
		WithObjects(componentDef("vela-system", "base")).Build()}

	require.NoError(t, ReadWithLiveRetry(cached, live, lookUp("team-a", "base")))
	require.Greater(t, live.gets, 0, "the miss was retried against the API server")
}

// An error that is not absence says nothing about the cache being behind, so
// retrying it would just make the same complaint twice.
func TestAnErrorThatIsNotAbsenceIsNotRetried(t *testing.T) {
	s := readerScheme(t)
	live := &counter{Client: fake.NewClientBuilder().WithScheme(s).Build()}

	boom := context.Canceled
	err := ReadWithLiveRetry(live.Client, live, func(client.Client) error { return boom })
	require.ErrorIs(t, err, boom)
	require.Zero(t, live.gets)
}

// With no live client there is nothing to fall back to, which is what a test
// supplying a fake wants and what a manager that could not build an uncached
// client falls back to.
func TestWithNoLiveClientTheLookupRunsOnceOnTheCache(t *testing.T) {
	s := readerScheme(t)
	cached := &counter{Client: fake.NewClientBuilder().WithScheme(s).Build()}

	require.Error(t, ReadWithLiveRetry(cached, nil, lookUp("team-a", "base")))
	require.Greater(t, cached.gets, 0)
}
