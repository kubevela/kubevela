package common

import (
	"context"
	"fmt"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
)

func TestConstructExtract(t *testing.T) {
	tests := []string{"tam1", "test-comp", "xx", "tt-x-x-c"}
	revisionNum := []int64{1, 5, 10, 100000}
	for idx, componentName := range tests {
		t.Run(fmt.Sprintf("tests %d for component[%s]", idx, componentName), func(t *testing.T) {
			revisionName := ConstructRevisionName(componentName, revisionNum[idx])
			got := ExtractComponentName(revisionName)
			if got != componentName {
				t.Errorf("want to get %s from %s but got %s", componentName, revisionName, got)
			}
		})
	}
}

func TestCompareWithRevision(t *testing.T) {
	ctx := context.TODO()
	logger := logging.NewLogrLogger(ctrl.Log.WithName("util-test"))
	componentName := "testComp"
	nameSpace := "namespace"
	latestRevision := "revision"
	imageV1 := "wordpress:4.6.1-apache"
	namespaceName := "test"
	cwV1 := v1alpha2.ContainerizedWorkload{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ContainerizedWorkload",
			APIVersion: "core.oam.dev/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespaceName,
		},
		Spec: v1alpha2.ContainerizedWorkloadSpec{
			Containers: []v1alpha2.Container{
				{
					Name:  "wordpress",
					Image: imageV1,
					Ports: []v1alpha2.ContainerPort{
						{
							Name: "wordpress",
							Port: 80,
						},
					},
				},
			},
		},
	}
	baseComp := &v1alpha2.Component{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Component",
			APIVersion: "core.oam.dev/v1alpha2",
		}, ObjectMeta: metav1.ObjectMeta{
			Name:      "myweb",
			Namespace: namespaceName,
			Labels:    map[string]string{"application.oam.dev": "test"},
		},
		Spec: v1alpha2.ComponentSpec{
			Workload: runtime.RawExtension{
				Object: &cwV1,
			},
		}}

	revisionBase := v1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "revisionName",
			Namespace: baseComp.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
					Kind:       v1alpha2.ComponentKind,
					Name:       baseComp.Name,
					UID:        baseComp.UID,
					Controller: pointer.BoolPtr(true),
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
			getFunc: func(obj runtime.Object) error {
				o, ok := obj.(*v1.ControllerRevision)
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
			same, err := CompareWithRevision(ctx, &tclient, logger, componentName, nameSpace, latestRevision,
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
