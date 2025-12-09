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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestCreateOrUpdateApplication(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, v1beta1.AddToScheme(scheme))

	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
	}

	t.Run("create application", func(t *testing.T) {
		builder := fake.NewClientBuilder().WithScheme(scheme)
		fakeClient := builder.Build()
		err := CreateOrUpdateApplication(context.Background(), fakeClient, app)
		assert.NoError(t, err)

		var created v1beta1.Application
		err = fakeClient.Get(context.Background(), client.ObjectKeyFromObject(app), &created)
		assert.NoError(t, err)
		assert.Equal(t, "test-app", created.Name)
	})

	t.Run("update application", func(t *testing.T) {
		appToUpdate := app.DeepCopy()
		appToUpdate.SetAnnotations(map[string]string{"key": "val"})

		builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(app)
		fakeClient := builder.Build()

		err := CreateOrUpdateApplication(context.Background(), fakeClient, appToUpdate)
		assert.NoError(t, err)

		var updated v1beta1.Application
		err = fakeClient.Get(context.Background(), client.ObjectKeyFromObject(app), &updated)
		assert.NoError(t, err)
		assert.Equal(t, "val", updated.Annotations["key"])
	})
}

func TestCreateOrUpdateObjects(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, corev1.AddToScheme(scheme))

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"initial": "true",
		},
	}

	t.Run("create object", func(t *testing.T) {
		builder := fake.NewClientBuilder().WithScheme(scheme)
		fakeClient := builder.Build()
		objects := []oam.Object{cm}

		err := CreateOrUpdateObjects(context.Background(), fakeClient, objects)
		assert.NoError(t, err)

		var created corev1.ConfigMap
		err = fakeClient.Get(context.Background(), client.ObjectKeyFromObject(cm), &created)
		assert.NoError(t, err)
		assert.Equal(t, "true", created.Data["initial"])
	})

	t.Run("update object", func(t *testing.T) {
		cmToUpdate := cm.DeepCopy()
		cmToUpdate.Data["initial"] = "false"

		builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm)
		fakeClient := builder.Build()
		objects := []oam.Object{cmToUpdate}

		err := CreateOrUpdateObjects(context.Background(), fakeClient, objects)
		assert.NoError(t, err)

		var updated corev1.ConfigMap
		err = fakeClient.Get(context.Background(), client.ObjectKeyFromObject(cm), &updated)
		assert.NoError(t, err)
		assert.Equal(t, "false", updated.Data["initial"])
	})
}

func TestRun(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, v1beta1.AddToScheme(scheme))
	assert.NoError(t, corev1.AddToScheme(scheme))

	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app-run",
			Namespace: "default",
		},
	}
	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm-run",
			Namespace: "default",
		},
	}

	builder := fake.NewClientBuilder().WithScheme(scheme)
	fakeClient := builder.Build()

	err := Run(context.Background(), fakeClient, app, []oam.Object{cm})
	assert.NoError(t, err)

	var createdApp v1beta1.Application
	err = fakeClient.Get(context.Background(), client.ObjectKeyFromObject(app), &createdApp)
	assert.NoError(t, err)
	assert.Equal(t, "test-app-run", createdApp.Name)

	var createdCM corev1.ConfigMap
	err = fakeClient.Get(context.Background(), client.ObjectKeyFromObject(cm), &createdCM)
	assert.NoError(t, err)
	assert.Equal(t, "test-cm-run", createdCM.Name)
}
