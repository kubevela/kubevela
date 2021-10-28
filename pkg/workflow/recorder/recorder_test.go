/*Copyright 2021 The KubeVela Authors.

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

package recorder

import (
	"context"
	"fmt"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/pkg/errors"
	"gotest.tools/assert"
	apps "k8s.io/api/apps/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestRecord(t *testing.T) {
	cli := makeMockClient()
	app := &v1beta1.Application{}
	app.Namespace = "default"
	app.Name = "test-app"
	data := []byte(`{"spec": "mock-data"}`)
	err := With(cli, app).Save("v1", data).
		Save("v2", data).
		Save("v3", data).Limit(2).Error()
	assert.NilError(t, err)

	crs := &apps.ControllerRevisionList{}
	err = cli.List(context.Background(), crs)
	assert.NilError(t, err)
	assert.Equal(t, len(crs.Items), 2)

	assert.Equal(t, crs.Items[0].Name, "record-test-app-v2")
	assert.Equal(t, crs.Items[1].Name, "record-test-app-v3")

	// check update old recorder.
	err = With(cli, app).Save("v3", data).Error()
	assert.NilError(t, err)

	creatErrorEnable = true
	err = With(cli, app).Save("v1", data).Error()
	assert.Equal(t, err.Error(), "save record default/record-test-app-v1: mock create error")

	creatErrorEnable = false
	listErrorEnable = true
	err = With(cli, app).Save("v1", data).Limit(3).Error()
	assert.Equal(t, err.Error(), "limit recorder: list controllerRevision (source=test-app): mock list error")

	listErrorEnable = false
	cli = makeMockClient()
	err = With(cli, app).Save("", data).Limit(1).Error()
	assert.NilError(t, err)
	crs = &apps.ControllerRevisionList{}
	err = cli.List(context.Background(), crs)
	assert.NilError(t, err)
	assert.Equal(t, crs.Items[0].Name, fmt.Sprintf("record-%s-%d", app.Name, crs.Items[0].Revision))
}

var (
	listErrorEnable  bool
	creatErrorEnable bool
)

func makeMockClient() client.Client {
	items := []apps.ControllerRevision{}
	return &test.MockClient{
		MockList: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			if listErrorEnable {
				return errors.New("mock list error")
			}
			crList, ok := list.(*apps.ControllerRevisionList)
			if ok {
				*crList = apps.ControllerRevisionList{
					Items: items,
				}
			}
			return nil
		},
		MockUpdate: func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
			o, ok := obj.(*apps.ControllerRevision)
			if ok {
				for index, item := range items {
					if item.Name == o.Name && item.Namespace == o.Namespace {
						items[index] = *o
						return nil
					}
				}
			}
			return kerrors.NewNotFound(apps.Resource("ControllerRevision"), o.Name)
		},
		MockCreate: func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
			if creatErrorEnable {
				return errors.New("mock create error")
			}
			o, ok := obj.(*apps.ControllerRevision)
			if ok {
				for _, item := range items {
					if item.Name == o.Name && item.Namespace == o.Namespace {
						return kerrors.NewAlreadyExists(apps.Resource("ControllerRevision"), o.Name)
					}
				}
				items = append(items, *o)
			}
			return nil
		},
		MockDelete: func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
			o, ok := obj.(*apps.ControllerRevision)
			if ok {
				newItems := []apps.ControllerRevision{}
				for index := range items {
					if items[index].Name != o.Name || items[index].Namespace != o.Namespace {
						newItems = append(newItems, items[index])
					}
				}
				items = newItems
			}
			return nil
		},
	}
}
