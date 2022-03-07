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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestNamespaceAdmissionHandler_Validate(t *testing.T) {
	AllowCrossNamespaceResource = false
	defer func() {
		AllowCrossNamespaceResource = true
	}()
	handler := &NamespaceAdmissionHandler{
		app: &v1beta1.Application{ObjectMeta: v1.ObjectMeta{Namespace: "test"}},
	}
	objs := []*unstructured.Unstructured{{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "demo",
				"namespace": "demo",
			},
		},
	}}
	err := handler.Validate(context.Background(), objs)
	r := require.New(t)
	r.NotNil(err)
	r.Contains(err.Error(), "forbidden resource")
	AllowCrossNamespaceResource = true
	r.NoError(handler.Validate(context.Background(), objs))
}

func TestResourceTypeAdmissionHandler_Validate(t *testing.T) {
	defer func() {
		AllowResourceTypes = ""
	}()
	r := require.New(t)
	objs := []*unstructured.Unstructured{{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "demo",
				"namespace": "demo",
			},
		},
	}}
	AllowResourceTypes = "blacklist:Service.v1,Secret.v1"
	err := (&ResourceTypeAdmissionHandler{}).Validate(context.Background(), objs)
	r.NotNil(err)
	r.Contains(err.Error(), "forbidden resource")
	AllowResourceTypes = "blacklist:ConfigMap.v1,Deployment.v1.apps"
	r.NoError((&ResourceTypeAdmissionHandler{}).Validate(context.Background(), objs))
	AllowResourceTypes = "whitelist:ConfigMap.v1,Deployment.v1.apps"
	err = (&ResourceTypeAdmissionHandler{}).Validate(context.Background(), objs)
	r.NotNil(err)
	r.Contains(err.Error(), "forbidden resource")
	AllowResourceTypes = "whitelist:Service.v1,Secret.v1"
	r.NoError((&ResourceTypeAdmissionHandler{}).Validate(context.Background(), objs))
}
