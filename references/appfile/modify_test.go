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

package appfile

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/references/appfile/api"
	"github.com/oam-dev/kubevela/references/appfile/template"
)

func TestSetWorkload(t *testing.T) {
	tm := template.NewFakeTemplateManager()

	t.Run("app is nil", func(t *testing.T) {
		err := SetWorkload(nil, "comp", "worker", nil)
		assert.EqualError(t, err, errorAppNilPointer.Error())
	})

	t.Run("add new component", func(t *testing.T) {
		app := NewApplication(nil, tm)
		app.Name = "test-app"
		workloadData := map[string]interface{}{"image": "test-image", "cmd": []string{"sleep", "1000"}}
		err := SetWorkload(app, "my-comp", "worker", workloadData)
		assert.NoError(t, err)

		assert.Len(t, app.Services, 1)
		svc, ok := app.Services["my-comp"]
		assert.True(t, ok)
		assert.NotNil(t, svc)
		assert.Equal(t, "worker", svc["type"])
		assert.Equal(t, "test-image", svc["image"])
		assert.Equal(t, []string{"sleep", "1000"}, svc["cmd"])
	})

	t.Run("update existing component", func(t *testing.T) {
		app := NewApplication(nil, tm)
		app.Name = "test-app"
		app.Services["my-comp"] = api.Service{
			"type":  "worker",
			"image": "initial-image",
		}

		updatedWorkloadData := map[string]interface{}{"image": "updated-image", "port": 8080}
		err := SetWorkload(app, "my-comp", "webservice", updatedWorkloadData)
		assert.NoError(t, err)

		assert.Len(t, app.Services, 1)
		svc, ok := app.Services["my-comp"]
		assert.True(t, ok)
		assert.NotNil(t, svc)
		assert.Equal(t, "webservice", svc["type"])
		assert.Equal(t, "updated-image", svc["image"])
		assert.Equal(t, 8080, svc["port"])
	})

	t.Run("add to existing services", func(t *testing.T) {
		app := NewApplication(nil, tm)
		app.Name = "test-app"
		app.Services["comp-1"] = api.Service{
			"type": "worker",
		}

		workloadData := map[string]interface{}{"image": "test-image"}
		err := SetWorkload(app, "comp-2", "task", workloadData)
		assert.NoError(t, err)

		assert.Len(t, app.Services, 2)
		assert.Contains(t, app.Services, "comp-1")
		assert.Contains(t, app.Services, "comp-2")
		svc2 := app.Services["comp-2"]
		assert.Equal(t, "task", svc2["type"])
		assert.Equal(t, "test-image", svc2["image"])
	})
}
