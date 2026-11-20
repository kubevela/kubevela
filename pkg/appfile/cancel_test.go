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

package appfile

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
)

// An admission request the API server has given up on is cancelled; rendering the
// rest of a large Application for it is work nobody will read.
func TestValidationStopsWhenTheRequestIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	af := &Appfile{Name: "big", Namespace: "default", ParsedComponents: []*Component{{
		Name: "web", Type: "worker", CapabilityCategory: types.CUECategory,
		Params:       map[string]any{},
		FullTemplate: &Template{TemplateStr: `output: {apiVersion: "v1", kind: "ConfigMap"}`},
		engine:       definition.NewWorkloadAbstractEngine("web"),
	}}}
	require.ErrorIs(t, NewApplicationParser(nil).ValidateCUESchematicAppfile(ctx, af), context.Canceled)
}

func TestParsingStopsWhenTheRequestIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	reads := 0
	cli := interceptor.NewClient(fake.NewClientBuilder().WithScheme(scheme).Build(), interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			reads++
			return c.Get(ctx, key, obj, opts...)
		},
	})
	app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{
		{Name: "a", Type: "worker"}, {Name: "b", Type: "worker"},
	}}}
	err := NewApplicationParser(cli).parseComponents(ctx, &Appfile{app: app})
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, reads, "no definition was loaded")
}

type appfileCtxKey struct{}

// renderContext is the request's context when the Appfile carries none, as one built from a
// published revision does, and the Appfile's own otherwise; either way a cancelled request
// stops the render.
func TestRenderContextFollowsTheRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, renderContext(ctx, nil).Err(), context.Canceled)
	own := context.WithValue(ctx, appfileCtxKey{}, "appfile")
	require.Equal(t, own, renderContext(ctx, own), "the Appfile's own context, when it has one")
	require.ErrorIs(t, renderContext(ctx, own).Err(), context.Canceled)
}
