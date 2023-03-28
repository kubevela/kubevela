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

package utils

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/stretchr/testify/assert"
	v12 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

func TestCheckDisabledCapabilities(t *testing.T) {
	disableCaps := "all"
	err := CheckDisabledCapabilities(disableCaps)
	assert.NoError(t, err)

	disableCaps = ""
	err = CheckDisabledCapabilities(disableCaps)
	assert.NoError(t, err)

	disableCaps = "rollout,healthscope"
	err = CheckDisabledCapabilities(disableCaps)
	assert.NoError(t, err)

	disableCaps = "abc,def"
	err = CheckDisabledCapabilities(disableCaps)
	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), "abc in disable caps list is not built-in")
}

func TestConstructExtract(t *testing.T) {
	tests := []string{"tam1", "test-comp", "xx", "tt-x-x-c"}
	revisionNum := []int{1, 5, 10, 100000}
	for idx, componentName := range tests {
		t.Run(fmt.Sprintf("tests %d for component[%s]", idx, componentName), func(t *testing.T) {
			revisionName := ConstructRevisionName(componentName, int64(revisionNum[idx]))
			got := ExtractComponentName(revisionName)
			if got != componentName {
				t.Errorf("want to get %s from %s but got %s", componentName, revisionName, got)
			}
			revision, _ := ExtractRevision(revisionName)
			if revision != revisionNum[idx] {
				t.Errorf("want to get %d from %s but got %d", revisionNum[idx], revisionName, revision)
			}
		})
	}
	badRevision := []string{"xx", "yy-", "zz-0.1"}
	t.Run(fmt.Sprintf("tests %s for extractRevision", badRevision), func(t *testing.T) {
		for _, revisionName := range badRevision {
			_, err := ExtractRevision(revisionName)
			if err == nil {
				t.Errorf("want to get err from %s but got nil", revisionName)
			}
		}
	})
}

func TestCompareWithRevision(t *testing.T) {
	ctx := context.TODO()
	componentName := "testComp"
	nameSpace := "namespace"
	latestRevision := "revision"
	imageV1 := "wordpress:4.6.1-apache"
	namespaceName := "test"
	cwV1 := v12.Deployment{
		TypeMeta: v1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: v1.ObjectMeta{
			Namespace: namespaceName,
		},
		Spec: v12.DeploymentSpec{
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "wordpress",
				},
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "wordpress",
							Image: imageV1,
						},
					},
				},
				ObjectMeta: v1.ObjectMeta{Labels: map[string]string{"app": "wordpress"}},
			},
		},
	}
	baseComp := &v1alpha2.Component{
		TypeMeta: v1.TypeMeta{
			Kind:       "Component",
			APIVersion: "core.oam.dev/v1alpha2",
		}, ObjectMeta: v1.ObjectMeta{
			Name:      "myweb",
			Namespace: namespaceName,
			Labels:    map[string]string{"application.oam.dev": "test"},
		},
		Spec: v1alpha2.ComponentSpec{
			Workload: runtime.RawExtension{
				Object: &cwV1,
			},
		}}

	revisionBase := v12.ControllerRevision{
		ObjectMeta: v1.ObjectMeta{
			Name:      "revisionName",
			Namespace: baseComp.Namespace,
			OwnerReferences: []v1.OwnerReference{
				{
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
					Kind:       v1alpha2.ComponentKind,
					Name:       baseComp.Name,
					UID:        baseComp.UID,
					Controller: pointer.Bool(true),
				},
			},
			Labels: map[string]string{
				"controller.oam.dev/component": baseComp.Name,
			},
		},
		Revision: 2,
		Data:     runtime.RawExtension{Object: baseComp},
	}

	tests := map[string]struct {
		getFunc        test.ObjectFn
		curCompSpec    *v1alpha2.ComponentSpec
		expectedResult bool
		expectedErr    error
	}{
		"compare object": {
			getFunc: func(obj client.Object) error {
				o, ok := obj.(*v12.ControllerRevision)
				if !ok {
					t.Errorf("the object %+v is not of type controller revision", o)
				}
				*o = revisionBase
				return nil
			},
			curCompSpec: &v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: baseComp,
				},
			},
			expectedResult: true,
			expectedErr:    nil,
		},
		// TODO: add test cases
		// compare raw with object
		// raw with raw
		// diff in object meta
		// diff in namespace
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tclient := test.MockClient{
				MockGet: test.NewMockGetFn(nil, tt.getFunc),
			}
			same, err := CompareWithRevision(ctx, &tclient, componentName, nameSpace, latestRevision,
				tt.curCompSpec)
			if err != tt.expectedErr {
				t.Errorf("CompareWithRevision() error = %v, wantErr %v", err, tt.expectedErr)
				return
			}
			if same != tt.expectedResult {
				t.Errorf("CompareWithRevision() got = %t, want %t", same, tt.expectedResult)
			}
		})
	}
}

func TestGetAppRevison(t *testing.T) {
	revisionName, latestRevision := GetAppNextRevision(nil)
	assert.Equal(t, revisionName, "")
	assert.Equal(t, latestRevision, int64(0))
	// the first is always 1
	app := &v1beta1.Application{}
	app.Name = "myapp"
	revisionName, latestRevision = GetAppNextRevision(app)
	assert.Equal(t, revisionName, "myapp-v1")
	assert.Equal(t, latestRevision, int64(1))
	app.Status.LatestRevision = &common.Revision{
		Name:     "myapp-v1",
		Revision: 1,
	}
	// we always automatically advance the revision
	revisionName, latestRevision = GetAppNextRevision(app)
	assert.Equal(t, revisionName, "myapp-v2")
	assert.Equal(t, latestRevision, int64(2))
	// we generate new revisions if the app is rolling
	app.SetAnnotations(map[string]string{oam.AnnotationAppRollout: strconv.FormatBool(true)})
	revisionName, latestRevision = GetAppNextRevision(app)
	assert.Equal(t, revisionName, "myapp-v2")
	assert.Equal(t, latestRevision, int64(2))
	app.Status.LatestRevision = &common.Revision{
		Name:     revisionName,
		Revision: latestRevision,
	}
	// try again
	revisionName, latestRevision = GetAppNextRevision(app)
	assert.Equal(t, revisionName, "myapp-v3")
	assert.Equal(t, latestRevision, int64(3))
	app.Status.LatestRevision = &common.Revision{
		Name:     revisionName,
		Revision: latestRevision,
	}
	// remove the annotation and it will still advance
	oamutil.RemoveAnnotations(app, []string{oam.AnnotationAppRollout})
	revisionName, latestRevision = GetAppNextRevision(app)
	assert.Equal(t, revisionName, "myapp-v4")
	assert.Equal(t, latestRevision, int64(4))
}
