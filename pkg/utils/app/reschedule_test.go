/*
Copyright 2023 The KubeVela Authors.

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

package app_test

import (
	"context"
	"testing"

	"github.com/kubevela/pkg/controller/sharding"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	apputil "github.com/oam-dev/kubevela/pkg/utils/app"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestReschedule(t *testing.T) {
	ctx := context.Background()

	app := &v1beta1.Application{}
	app.SetName("app")
	app.SetNamespace("default")

	labels := map[string]string{
		oam.LabelAppName:      "app",
		oam.LabelAppNamespace: "default",
	}
	appRev := &v1beta1.ApplicationRevision{}
	appRev.SetName("a1")
	appRev.SetNamespace("default")
	appRev.SetLabels(labels)
	rt := &v1beta1.ResourceTracker{}
	rt.SetName("r1")
	rt.SetLabels(labels)
	rt.Spec.Type = v1beta1.ResourceTrackerTypeRoot

	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(app, appRev, rt).Build()
	err := apputil.RescheduleAppRevAndRT(ctx, cli, app, "s1")
	require.NoError(t, err)

	require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(appRev), appRev))
	require.Equal(t, "s1", appRev.GetLabels()[sharding.LabelKubeVelaScheduledShardID])
	require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(rt), rt))
	require.Equal(t, "s1", rt.GetLabels()[sharding.LabelKubeVelaScheduledShardID])
}
