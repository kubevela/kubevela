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

package util

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	//lint:ignore SA1019 We will use pkg/envtest before upgrading controller-runtime to v1.0.0
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func initScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = apiextensions.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = crdv1.AddToScheme(scheme)
	return scheme
}

func TestDoesNamespaceExist(t *testing.T) {
	scheme := initScheme()
	fakeClient := fake.NewFakeClientWithScheme(scheme)
	//test exist namespace
	mockNamespaceName := "test-ns"
	mockNamespaceObject := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: mockNamespaceName,
		},
	}
	err := fakeClient.Create(context.Background(), mockNamespaceObject, &client.CreateOptions{})
	assert.NoError(t, err)
	exist, err := DoesNamespaceExist(fakeClient, mockNamespaceName)
	assert.NoError(t, err)
	assert.Equal(t, true, exist)

	//test not exist namespace
	exist, err = DoesNamespaceExist(fakeClient, "not-exist-ns")
	assert.NoError(t, err)
	assert.Equal(t, false, exist)
}

func TestDoesCRDExist(t *testing.T) {
	scheme := initScheme()
	fakeClient := fake.NewFakeClientWithScheme(scheme)
	//test crd exist
	mockCRD := &apiextensions.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "crd-exist",
		},
	}
	err := fakeClient.Create(context.Background(), mockCRD, &client.CreateOptions{})
	assert.NoError(t, err)
	exist, err := DoesCRDExist(context.Background(), fakeClient, "crd-exist")
	assert.NoError(t, err)
	assert.Equal(t, true, exist)

	//test crd not exist
	exist, err = DoesCRDExist(context.Background(), fakeClient, "not-exist-crd")
	assert.NoError(t, err)
	assert.Equal(t, false, exist)
}
