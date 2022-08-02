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

package resourcetracker

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestCreateAndListResourceTrackers(t *testing.T) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	app := &v1beta1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "app", Namespace: "namespace", UID: types.UID("uid")},
	}
	rootRT, err := CreateRootResourceTracker(context.Background(), cli, app)
	r.NoError(err)
	crRT, err := CreateComponentRevisionResourceTracker(context.Background(), cli, app)
	r.NoError(err)
	var versionedRTs []*v1beta1.ResourceTracker
	for i := 0; i < 10; i++ {
		app.Status.LatestRevision = &apicommon.Revision{Name: fmt.Sprintf("app-v%d", i)}
		app.Generation = int64(i + 1)
		currentRT, err := CreateCurrentResourceTracker(context.Background(), cli, app)
		r.NoError(err)
		versionedRTs = append(versionedRTs, currentRT)
	}
	legacyRT := &v1beta1.ResourceTracker{
		ObjectMeta: v1.ObjectMeta{
			Name: "legacy-rt",
			Labels: map[string]string{
				oam.LabelAppName:      app.Name,
				oam.LabelAppNamespace: app.Namespace,
			},
		},
	}
	r.NoError(cli.Create(context.Background(), legacyRT))
	_rootRT, _currentRT, _historyRTs, _crRT, err := ListApplicationResourceTrackers(context.Background(), cli, app)
	r.NoError(err)
	r.Equal(rootRT, _rootRT)
	r.Equal(versionedRTs[len(versionedRTs)-1], _currentRT)
	r.Equal(versionedRTs[0:len(versionedRTs)-1], _historyRTs)
	r.Equal(crRT, _crRT)
	badRT := &v1beta1.ResourceTracker{
		ObjectMeta: v1.ObjectMeta{
			Name: "bad-rt",
			Labels: map[string]string{
				oam.LabelAppName:      app.Name,
				oam.LabelAppNamespace: app.Namespace,
				oam.LabelAppUID:       "bad-uid",
			},
		},
	}
	r.NoError(cli.Create(context.Background(), badRT))
	_, _, _, _, err = ListApplicationResourceTrackers(context.Background(), cli, app)
	r.Error(err)
	r.Contains(err.Error(), "controlled by another application")
}

func TestRecordAndDeleteManifestsInResourceTracker(t *testing.T) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	rt := &v1beta1.ResourceTracker{ObjectMeta: v1.ObjectMeta{Name: "rt"}}
	r.NoError(cli.Create(context.Background(), rt))
	n := 10
	var objs []*unstructured.Unstructured
	for i := 0; i < n; i++ {
		obj := &unstructured.Unstructured{}
		obj.SetName(fmt.Sprintf("workload-%d", i))
		objs = append(objs, obj)
		r.NoError(RecordManifestsInResourceTracker(context.Background(), cli, rt, []*unstructured.Unstructured{obj}, rand.Int()%2 == 0, false, ""))
	}
	rand.Shuffle(len(objs), func(i, j int) { objs[i], objs[j] = objs[j], objs[i] })
	for i := 0; i < n; i++ {
		r.NoError(DeletedManifestInResourceTracker(context.Background(), cli, rt, objs[i], true))
		r.Equal(len(rt.Spec.ManagedResources), n-i-1)
	}
}

func TestPublishedVersion(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	app := &v1beta1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "app", Namespace: "namespace", UID: types.UID("uid"), Generation: int64(1)},
	}
	meta.AddAnnotations(app, map[string]string{oam.AnnotationPublishVersion: "publish-version-v1"})
	rt1, err := CreateCurrentResourceTracker(ctx, cli, app)
	r.NoError(err)
	app.SetGeneration(int64(2))
	_, err = CreateCurrentResourceTracker(ctx, cli, app)
	r.True(errors.IsAlreadyExists(err))
	app.SetGeneration(int64(3))
	app.Annotations[oam.AnnotationPublishVersion] = "publish-version-v2"
	_, err = CreateCurrentResourceTracker(ctx, cli, app)
	r.NoError(err)
	app.SetGeneration(int64(4))
	app.Annotations[oam.AnnotationPublishVersion] = "publish-version-v3"
	_, err = CreateCurrentResourceTracker(ctx, cli, app)
	r.NoError(err)
	app.SetGeneration(int64(5))
	_, currentRT, historyRTs, _, err := ListApplicationResourceTrackers(ctx, cli, app)
	r.NoError(err)
	r.Equal(int64(4), currentRT.Spec.ApplicationGeneration)
	r.Equal(2, len(historyRTs))
	// use old publish version, check conflict
	app.SetGeneration(int64(6))
	app.Annotations[oam.AnnotationPublishVersion] = "publish-version-v2"
	_, _, _, _, err = ListApplicationResourceTrackers(ctx, cli, app)
	r.Error(err)
	r.Contains(err.Error(), "in-use and outdated")
	// use old deleted publish version, check no conflict
	rt1.SetFinalizers([]string{})
	r.NoError(cli.Update(ctx, rt1))
	r.NoError(cli.Delete(ctx, rt1))
	app.SetGeneration(int64(7))
	app.Annotations[oam.AnnotationPublishVersion] = "publish-version-v1"
	_, currentRT, historyRTs, _, err = ListApplicationResourceTrackers(ctx, cli, app)
	r.NoError(err)
	r.Nil(currentRT)
	r.Equal(2, len(historyRTs))
}

func TestListApplicationResourceTrackers(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	rt := &unstructured.Unstructured{}
	rt.SetGroupVersionKind(applicationResourceTrackerGroupVersionKind)
	rt.SetName("rt")
	rt.SetNamespace("example")
	rt.SetLabels(map[string]string{oam.LabelAppName: "app"})
	cli := &clientWithoutRTPermission{Client: fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(rt).Build()}
	app := &v1beta1.Application{}
	app.SetName("app")
	app.SetNamespace("example")
	_, err := listApplicationResourceTrackers(ctx, cli, app)
	r.NotNil(err)
	r.Contains(err.Error(), "no permission for ResourceTracker and vela-prism is not serving ApplicationResourceTracker")
	cli.recognizeApplicationResourceTracker = true
	rts, err := listApplicationResourceTrackers(ctx, cli, app)
	r.NoError(err)
	r.Equal(len(rts), 1)
	r.Equal(rts[0].Name, "rt-example")
}

type clientWithoutRTPermission struct {
	client.Client
	recognizeApplicationResourceTracker bool
}

func (c *clientWithoutRTPermission) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, isRTList := list.(*v1beta1.ResourceTrackerList); isRTList {
		return errors.NewForbidden(schema.GroupResource{}, "", nil)
	}
	if unsList, isUnsList := list.(*unstructured.UnstructuredList); isUnsList && unsList.GetKind() == applicationResourceTrackerGroupVersionKind.Kind && !c.recognizeApplicationResourceTracker {
		return &apimeta.NoKindMatchError{}
	}
	return c.Client.List(ctx, list, opts...)
}
