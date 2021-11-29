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

package resourcekeeper

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestNewResourceKeeper(t *testing.T) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	app := &v1beta1.Application{ObjectMeta: v1.ObjectMeta{
		Name:       "app",
		Namespace:  "default",
		Generation: 6,
	}}
	app.Spec.Policies = []v1beta1.AppPolicy{{
		Type:       "apply-once",
		Properties: &runtime.RawExtension{Raw: []byte(`bad value`)},
	}}
	_, err := NewResourceKeeper(context.Background(), cli, app)
	r.Error(err)
	r.Contains(err.Error(), "failed to parse apply-once policy")
	app.Spec.Policies = []v1beta1.AppPolicy{{
		Type:       "garbage-collect",
		Properties: &runtime.RawExtension{Raw: []byte(`bad value`)},
	}}
	_, err = NewResourceKeeper(context.Background(), cli, app)
	r.Error(err)
	r.Contains(err.Error(), "failed to parse garbage-collect policy")
	app.Spec.Policies = []v1beta1.AppPolicy{{
		Type:       "garbage-collect",
		Properties: &runtime.RawExtension{Raw: []byte(`{"keepLegacyResource":true}`)},
	}}
	util.AddLabels(app, map[string]string{oam.LabelAddonName: "test"})
	for i := 1; i <= 5; i++ {
		appName := "app"
		if i < 3 {
			appName = "app-another"
		}
		rt := &v1beta1.ResourceTracker{
			ObjectMeta: v1.ObjectMeta{
				Name: fmt.Sprintf("app-v%d", i),
				Labels: map[string]string{
					oam.LabelAppName:      appName,
					oam.LabelAppNamespace: "default",
				},
			},
			Spec: v1beta1.ResourceTrackerSpec{
				Type:                  v1beta1.ResourceTrackerTypeVersioned,
				ApplicationGeneration: int64(i),
			},
		}
		r.NoError(cli.Create(context.Background(), rt))
	}
	_rk, err := NewResourceKeeper(context.Background(), cli, app)
	r.NoError(err)
	rk := _rk.(*resourceKeeper)
	r.NotNil(rk.applyOncePolicy)
	r.True(rk.applyOncePolicy.Enable)
	r.NotNil(rk.garbageCollectPolicy)
	r.True(rk.garbageCollectPolicy.KeepLegacyResource)
	rootRT, err := rk.getRootRT(context.Background())
	r.NoError(err)
	r.NotNil(rootRT)
	crRT, err := rk.getComponentRevisionRT(context.Background())
	r.NoError(err)
	r.NotNil(crRT)
	currentRT, err := rk.getCurrentRT(context.Background())
	r.NoError(err)
	r.NotNil(currentRT)
	r.Equal(3, len(rk._historyRTs))
}
