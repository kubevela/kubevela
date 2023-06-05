/*
Copyright 2021 The KubeVela Authors.

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

package oam

import (
	"context"
	"encoding/json"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kubevela/pkg/util/singleton"
	"github.com/kubevela/workflow/pkg/mock"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/application"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/legacy/types"
)

func setupHandlers(ctx context.Context, t *testing.T) (client.Client, *appfile.Appfile, *application.AppHandler) {
	r := require.New(t)
	scheme := runtime.NewScheme()
	r.NoError(v1beta1.AddToScheme(scheme))
	r.NoError(appsv1.AddToScheme(scheme))
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	singleton.KubeClient.Set(cli)
	p := appfile.NewApplicationParser(cli)
	handler, err := application.NewAppHandler(ctx, &application.Reconciler{
		Client: cli,
	}, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: "default"}}, p)
	r.NoError(err)
	appfile := &appfile.Appfile{
		AppRevision: &v1beta1.ApplicationRevision{
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					ComponentDefinitions: map[string]*v1beta1.ComponentDefinition{
						"test": {
							ObjectMeta: metav1.ObjectMeta{Name: "test"},
							Spec: v1beta1.ComponentDefinitionSpec{
								Status: &common.Status{
									HealthPolicy: `isHealth: false`,
								},
								Schematic: &common.Schematic{
									CUE: &common.CUE{
										Template: `output: {
	apiVersion: "v1"
	kind:       "Pod"
	metadata: {
		labels: {
			app: "web"
		}
		name: "test"
	}
}
outputs: {
	service: {
		apiVersion: "v1"
		kind:       "Service"
		metadata: {
			labels: {
				"trait.oam.dev/resource": "service"
			}
			name: "service"
		}
	}
}`,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	handler.PrepareCurrentAppRevision(ctx, appfile)
	return cli, appfile, handler
}

func TestParser(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	act := &mock.Action{}
	cuectx := cuecontext.New()
	_, appfile, handler := setupHandlers(ctx, t)

	v := cuectx.CompileString("")
	_, err := ApplyComponent(ctx, &oamprovidertypes.OAMParams[cue.Value]{
		Params: v,
		RuntimeParams: oamprovidertypes.RuntimeParams{
			AppHandler: handler,
		},
	})
	r.Equal(err.Error(), "failed to lookup value: var(path=value) not exist")
	v = cuectx.CompileString(`value: {
	name: "test",
	type: "test",
}`)
	res, err := ApplyComponent(ctx, &oamprovidertypes.OAMParams[cue.Value]{
		Params: v,
		RuntimeParams: oamprovidertypes.RuntimeParams{
			AppHandler: handler,
			Appfile:    appfile,
			Action:     act,
		},
	})
	r.NoError(err)
	output, err := res.LookupPath(cue.ParsePath("output.metadata.name")).String()
	r.NoError(err)
	r.Equal(output, "test")

	outputs, err := res.LookupPath(cue.ParsePath("outputs.service.metadata.name")).String()
	r.NoError(err)
	r.Equal(outputs, "service")

	r.Equal(act.Phase, "Wait")
}

func TestLoadComponent(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	act := &mock.Action{}
	cli, appfile, handler := setupHandlers(ctx, t)
	res, err := LoadComponent(ctx, &oamprovidertypes.OAMParams[LoadVars]{
		Params: LoadVars{},
		RuntimeParams: oamprovidertypes.RuntimeParams{
			AppHandler: handler,
			Appfile:    appfile,
			Action:     act,
			App: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name:       "c1",
							Type:       "test",
							Properties: &runtime.RawExtension{Raw: []byte(`{"image": "busybox"}`)},
						},
					},
				},
			},
		},
	})
	r.NoError(err)
	b, err := json.Marshal(res.Value)
	r.NoError(err)
	r.Equal(string(b), `{"c1":{"name":"c1","type":"test","properties":{"image":"busybox"}}}`)

	app2 := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test2",
			Namespace: "default",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       "c2",
					Type:       "test",
					Properties: &runtime.RawExtension{Raw: []byte(`{"image": "nginx"}`)},
				},
			},
		},
	}
	err = cli.Create(ctx, app2)
	r.NoError(err)
	res, err = LoadComponent(ctx, &oamprovidertypes.OAMParams[LoadVars]{
		Params: LoadVars{
			App: "test2",
		},
		RuntimeParams: oamprovidertypes.RuntimeParams{
			AppHandler: handler,
			Appfile:    appfile,
			Action:     act,
			App:        app2,
		},
	})
	r.NoError(err)
	b, err = json.Marshal(res.Value)
	r.NoError(err)
	r.Equal(string(b), `{"c2":{"name":"c2","type":"test","properties":{"image":"nginx"}}}`)
}

func TestLoadComponentInOrder(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	act := &mock.Action{}
	_, appfile, handler := setupHandlers(ctx, t)
	res, err := LoadComponentInOrder(ctx, &oamprovidertypes.OAMParams[LoadVars]{
		Params: LoadVars{},
		RuntimeParams: oamprovidertypes.RuntimeParams{
			AppHandler: handler,
			Appfile:    appfile,
			Action:     act,
			App: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name:       "c1",
							Type:       "test",
							Properties: &runtime.RawExtension{Raw: []byte(`{"image": "busybox"}`)},
						},
						{
							Name:       "c2",
							Type:       "test",
							Properties: &runtime.RawExtension{Raw: []byte(`{"image": "busybox"}`)},
						},
					},
				},
			},
		},
	})
	r.NoError(err)
	b, err := json.Marshal(res.Value)
	r.NoError(err)
	r.Equal(string(b), `[{"name":"c1","type":"test","properties":{"image":"busybox"}},{"name":"c2","type":"test","properties":{"image":"busybox"}}]`)
}
