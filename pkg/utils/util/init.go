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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// OAMLabel defines the label of namespace automatically created by kubevela
var OAMLabel = map[string]string{"app.kubernetes.io/part-of": "kubevela"}

// DoesNamespaceExist check namespace exist
func DoesNamespaceExist(c client.Client, namespace string) (bool, error) {
	var ns corev1.Namespace
	err := c.Get(context.Background(), types.NamespacedName{Name: namespace}, &ns)
	if err != nil {
		return false, client.IgnoreNotFound(err)
	}
	return true, nil
}

// NewNamespace create namespace
func NewNamespace(c client.Client, namespace string) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace,
		// marking a special label for prometheus monitoring.
		Labels: OAMLabel}}
	err := c.Create(context.Background(), ns)
	if err != nil {
		return err
	}
	return nil
}

// DoesCRDExist check CRD exist
func DoesCRDExist(cxt context.Context, c client.Client, crdName string) (bool, error) {
	err := c.Get(cxt, types.NamespacedName{Name: crdName}, &apiextensions.CustomResourceDefinition{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
