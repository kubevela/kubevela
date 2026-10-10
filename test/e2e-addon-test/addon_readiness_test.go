/*
Copyright 2026 The KubeVela Authors.

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

package controllers_test

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	common "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestAddonSuiteReadinessRequiresControllersAndServedCRDs(t *testing.T) {
	s := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{v1beta1.AddToScheme, appsv1.AddToScheme, crdv1.AddToScheme} {
		if err := add(s); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func([]client.Object) []client.Object
		want   string
	}{
		{name: "ready"},
		{name: "missing application", mutate: func(objects []client.Object) []client.Object { return objects[1:] }, want: "addon-terraform-alibaba"},
		{name: "suspended application", mutate: func(objects []client.Object) []client.Object {
			objects[0].(*v1beta1.Application).Status.Phase = common.ApplicationWorkflowSuspending
			return objects
		}, want: "addon-terraform-alibaba"},
		{name: "stale application status", mutate: func(objects []client.Object) []client.Object {
			objects[1].(*v1beta1.Application).Status.ObservedGeneration = 1
			return objects
		}, want: "addon-vela-workflow"},
		{name: "missing controller", mutate: func(objects []client.Object) []client.Object { return append(objects[:2], objects[3:]...) }, want: "terraform-controller"},
		{name: "stale controller status", mutate: func(objects []client.Object) []client.Object {
			objects[3].(*appsv1.Deployment).Status.ObservedGeneration = 1
			return objects
		}, want: "vela-workflow"},
		{name: "controller pod not ready", mutate: func(objects []client.Object) []client.Object {
			objects[3].(*appsv1.Deployment).Status.ReadyReplicas = 0
			return objects
		}, want: "vela-workflow"},
		{name: "controller pod not available", mutate: func(objects []client.Object) []client.Object {
			objects[2].(*appsv1.Deployment).Status.AvailableReplicas = 0
			return objects
		}, want: "terraform-controller"},
		{name: "old controller revision ready", mutate: func(objects []client.Object) []client.Object {
			objects[3].(*appsv1.Deployment).Status.UpdatedReplicas = 0
			return objects
		}, want: "vela-workflow"},
		{name: "scaled down controller", mutate: func(objects []client.Object) []client.Object {
			zero := int32(0)
			objects[3].(*appsv1.Deployment).Spec.Replicas = &zero
			return objects
		}, want: "vela-workflow"},
		{name: "missing CRD", mutate: func(objects []client.Object) []client.Object { return objects[:6] }, want: "workflowruns.core.oam.dev"},
		{name: "unestablished CRD", mutate: func(objects []client.Object) []client.Object {
			objects[4].(*crdv1.CustomResourceDefinition).Status.Conditions[0].Status = crdv1.ConditionFalse
			return objects
		}, want: "configurations.terraform.core.oam.dev"},
		{name: "required version not served", mutate: func(objects []client.Object) []client.Object {
			objects[5].(*crdv1.CustomResourceDefinition).Spec.Versions[0].Served = false
			return objects
		}, want: "providers.terraform.core.oam.dev"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objects := readyAddonSuiteObjects()
			if tc.mutate != nil {
				objects = tc.mutate(objects)
			}
			cli := fake.NewClientBuilder().WithScheme(s).WithObjects(objects...).Build()
			err := addonSuiteReady(context.Background(), cli)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("readiness error = %v; want resource %q", err, tc.want)
			}
		})
	}
}

func readyAddonSuiteObjects() []client.Object {
	var objects []client.Object
	for _, name := range []string{"addon-terraform-alibaba", "addon-vela-workflow"} {
		app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vela-system", Generation: 2}}
		app.Status.Phase = common.ApplicationRunning
		app.Status.ObservedGeneration = 2
		objects = append(objects, app)
	}
	for _, key := range []client.ObjectKey{{Namespace: "vela-system", Name: "terraform-controller"}, {Namespace: "vela-system", Name: "vela-workflow"}} {
		one := int32(1)
		objects = append(objects, &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace, Generation: 2},
			Spec:       appsv1.DeploymentSpec{Replicas: &one},
			Status:     appsv1.DeploymentStatus{ObservedGeneration: 2, ReadyReplicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1},
		})
	}
	for _, item := range []struct{ name, version string }{
		{"configurations.terraform.core.oam.dev", "v1beta1"},
		{"providers.terraform.core.oam.dev", "v1beta1"},
		{"workflowruns.core.oam.dev", "v1alpha1"},
	} {
		objects = append(objects, &crdv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: item.name},
			Spec: crdv1.CustomResourceDefinitionSpec{Versions: []crdv1.CustomResourceDefinitionVersion{
				{Name: item.version, Served: true},
			}},
			Status: crdv1.CustomResourceDefinitionStatus{Conditions: []crdv1.CustomResourceDefinitionCondition{
				{Type: crdv1.Established, Status: crdv1.ConditionTrue},
			}},
		})
	}
	return objects
}
