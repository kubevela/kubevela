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
	"testing"

	"github.com/stretchr/testify/require"
	v12 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestResourceKeeper_ComponentRevision(t *testing.T) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	app := &v1beta1.Application{ObjectMeta: v1.ObjectMeta{Name: "app", Namespace: "default"}}
	rk, err := NewResourceKeeper(context.Background(), cli, app)
	r.NoError(err)
	cr := &v12.ControllerRevision{ObjectMeta: v1.ObjectMeta{Name: "app-comp-v1", Namespace: "default"}, Data: runtime.RawExtension{Raw: []byte(`{}`)}}
	cr.SetLabels(map[string]string{oam.LabelAppName: "app", oam.LabelAppNamespace: "default"})
	r.NoError(rk.DispatchComponentRevision(context.Background(), cr))
	r.Error(rk.DispatchComponentRevision(context.Background(), cr.DeepCopy()))
	r.NoError(rk.DeleteComponentRevision(context.Background(), cr))
}
