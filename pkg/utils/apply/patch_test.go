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

package apply

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/pkg/oam"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestAddLastAppliedConfig(t *testing.T) {
	cases := map[string]struct {
		reason  string
		obj     runtime.Object
		wantObj runtime.Object
		wantErr error
	}{
		"ErrCannotAccessMetadata": {
			reason:  "An error should be returned if cannot access metadata",
			obj:     testNoMetaObject{},
			wantObj: testNoMetaObject{},
			wantErr: errors.Wrap(fmt.Errorf("object does not implement the Object interfaces"), "cannot access metadata.annotations"),
		},
	}

	for caseName, tc := range cases {
		t.Run(caseName, func(t *testing.T) {
			err := addLastAppliedConfigAnnotation(tc.obj)
			if diff := cmp.Diff(tc.wantObj, tc.obj); diff != "" {
				t.Errorf("\n%s\ngetModifiedConfig(...): -want , +got \n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.wantErr, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ngetModifiedConfig(...): -want , +got \n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestGetOriginalConfig(t *testing.T) {
	objNoAnno := &unstructured.Unstructured{}
	objNoAnno.SetAnnotations(make(map[string]string))

	objHasAnno := &unstructured.Unstructured{}
	annoMap := make(map[string]string)
	annoMap[oam.AnnotationLastAppliedConfig] = "oam obj record"
	annoMap[corev1.LastAppliedConfigAnnotation] = "kubectl obj record"
	objHasAnno.SetAnnotations(annoMap)

	objOnlyHasKubectlAnno := &unstructured.Unstructured{}
	annoOnlyKubectlMap := make(map[string]string)
	annoOnlyKubectlMap[corev1.LastAppliedConfigAnnotation] = "kubectl obj record"
	objOnlyHasKubectlAnno.SetAnnotations(annoOnlyKubectlMap)

	cases := map[string]struct {
		reason     string
		obj        runtime.Object
		wantConfig string
		wantErr    error
	}{
		"ErrCannotAccessMetadata": {
			reason:  "An error should be returned if cannot access metadata",
			obj:     testNoMetaObject{},
			wantErr: errors.Wrap(fmt.Errorf("object does not implement the Object interfaces"), "cannot access metadata.annotations"),
		},
		"NoAnnotations": {
			reason: "No error should be returned if cannot find last-applied-config annotaion",
			obj:    &unstructured.Unstructured{},
		},
		"LastAppliedConfigAnnotationNotFound": {
			reason: "No error should be returned if cannot find last-applied-config annotaion",
			obj:    objNoAnno,
		},
		"OAMLastAppliedConfigAnnotationFound": {
			reason:     "No error should be returned if find oam last-applied-config annotaion ",
			obj:        objHasAnno,
			wantConfig: "oam obj record",
		},
		"KubectlLastAppliedConfigAnnotationFound": {
			reason:     "No error should be returned if find last-applied-config annotaion, prefer oam annotations, followed by kubectl ",
			obj:        objOnlyHasKubectlAnno,
			wantConfig: "kubectl obj record",
		},
	}

	for caseName, tc := range cases {
		t.Run(caseName, func(t *testing.T) {
			r, err := getOriginalConfiguration(tc.obj)
			if diff := cmp.Diff(tc.wantConfig, string(r)); diff != "" {
				t.Errorf("\n%s\ngetModifiedConfig(...): -want , +got \n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.wantErr, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ngetModifiedConfig(...): -want , +got \n%s\n", tc.reason, diff)
			}
		})
	}
}
