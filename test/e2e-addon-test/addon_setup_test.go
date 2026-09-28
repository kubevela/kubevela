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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestAddonPresenceDistinguishesExistingFromNew(t *testing.T) {
	s := runtime.NewScheme()
	if err := v1beta1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(
		&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "addon-terraform-alibaba", Namespace: "vela-system"}},
	).Build()
	for _, tc := range []struct {
		name string
		want bool
	}{
		{name: "terraform-alibaba", want: true},
		{name: "vela-workflow", want: false},
	} {
		got, err := addonApplicationExists(context.Background(), cli, tc.name)
		if err != nil || got != tc.want {
			t.Errorf("addonApplicationExists(%q) = %v, %v; want %v", tc.name, got, err, tc.want)
		}
	}
}
