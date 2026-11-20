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

package application

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestACreateIsRefusedWhileAnEarlierApplicationsTrackersRemain(t *testing.T) {
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "default", UID: "new-uid"}}
	tracker := func(uid string) *v1beta1.ResourceTracker {
		labels := map[string]string{oam.LabelAppName: "shop", oam.LabelAppNamespace: "default"}
		if uid != "" {
			labels[oam.LabelAppUID] = uid
		}
		return &v1beta1.ResourceTracker{ObjectMeta: metav1.ObjectMeta{Name: "shop-v1-default", Labels: labels}}
	}
	validate := func(objs ...*v1beta1.ResourceTracker) field.ErrorList {
		scheme := runtime.NewScheme()
		require.NoError(t, v1beta1.AddToScheme(scheme))
		b := fake.NewClientBuilder().WithScheme(scheme)
		for _, o := range objs {
			b = b.WithObjects(o)
		}
		return (&ValidatingHandler{Client: b.Build()}).ValidateEarlierTrackers(context.Background(), app)
	}

	errs := validate(tracker("old-uid"))
	require.Len(t, errs, 1)
	for _, says := range []string{
		"ResourceTracker shop-v1-default of an earlier Application default/shop (uid old-uid) still exists",
		"kubectl label resourcetracker shop-v1-default app.oam.dev/uid-",
		`kubectl patch resourcetracker shop-v1-default --type merge -p '{"metadata":{"finalizers":null}}'`,
	} {
		require.Contains(t, errs[0].Detail, says)
	}

	require.Empty(t, validate(), "no tracker")
	require.Empty(t, validate(tracker("")), "a tracker whose uid label was removed is taken over")
	require.Empty(t, validate(tracker("new-uid")), "a tracker naming this Application")

	// The API server gives a create its uid before admission, and refuses one of an existing name
	// only afterwards, so a create of a stored Application must reach that AlreadyExists.
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	stored := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "default", UID: "old-uid"}}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stored, tracker("old-uid")).Build()
	require.Empty(t, (&ValidatingHandler{Client: cli}).ValidateEarlierTrackers(context.Background(), app),
		"the trackers of the Application stored under this name")
}
