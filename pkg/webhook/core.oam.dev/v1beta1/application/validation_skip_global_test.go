/*
Copyright 2025 The KubeVela Authors.

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
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	commonconfig "github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestValidateSkipGlobalAnnotation(t *testing.T) {
	handler := &ValidatingHandler{}
	app := func(value string) *v1beta1.Application {
		a := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "tenant"}}
		if value != "" {
			a.Annotations = map[string]string{oam.AnnotationSkipGlobalPolicies: value}
		}
		return a
	}
	cases := []struct {
		name     string
		value    string
		allowed  bool
		wantErrs int
	}{
		{"skipping is rejected when global policies are mandatory", "true", false, 1},
		{"no annotation is fine", "", false, 0},
		{"an annotation that does not skip is fine", "false", false, 0},
		{"skipping is admitted when the controller permits it", "true", true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policyGates(t, true, true)
			setFlag(t, &commonconfig.AllowSkipGlobalPolicies, tc.allowed)
			errs := handler.ValidateAnnotations(context.Background(), app(tc.value), nil, admission.Request{})
			if len(errs) != tc.wantErrs {
				t.Fatalf("got %d errors (%v), want %d", len(errs), errs, tc.wantErrs)
			}
			if tc.wantErrs > 0 && !strings.Contains(errs[0].Error(), "mandatory") {
				t.Fatalf("the error should say global policies are mandatory: %v", errs[0])
			}
		})
	}
}

// An app that already carries skip-global stays updatable: the controller ignores the
// annotation, so only adding it is refused.
func TestValidateSkipGlobalAnnotationOnUpdate(t *testing.T) {
	handler := &ValidatingHandler{}
	app := func(value string) *v1beta1.Application {
		a := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "tenant"}}
		if value != "" {
			a.Annotations = map[string]string{oam.AnnotationSkipGlobalPolicies: value}
		}
		return a
	}
	cases := []struct {
		name     string
		old, new string
		wantErrs int
	}{
		{"an annotation carried forward is admitted", "true", "true", 0},
		{"adding the annotation is rejected", "", "true", 1},
		{"turning the annotation on is rejected", "false", "true", 1},
		{"removing the annotation is fine", "true", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policyGates(t, true, true)
			setFlag(t, &commonconfig.AllowSkipGlobalPolicies, false)
			errs := handler.ValidateAnnotations(context.Background(), app(tc.new), app(tc.old), admission.Request{})
			if len(errs) != tc.wantErrs {
				t.Fatalf("got %d errors (%v), want %d", len(errs), errs, tc.wantErrs)
			}
		})
	}
}

// skip-global has no effect while either policy gate is off, so it is not refused there.
func TestValidateSkipGlobalAnnotationWithPolicyGatesOff(t *testing.T) {
	handler := &ValidatingHandler{}
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "tenant",
		Annotations: map[string]string{oam.AnnotationSkipGlobalPolicies: "true"}}}
	cases := []struct {
		name              string
		appScoped, global bool
	}{
		{"both gates off", false, false},
		{"global policies off", true, false},
		{"application-scoped policies off", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policyGates(t, tc.appScoped, tc.global)
			setFlag(t, &commonconfig.AllowSkipGlobalPolicies, false)
			if errs := handler.ValidateAnnotations(context.Background(), app, nil, admission.Request{}); len(errs) != 0 {
				t.Fatalf("got %v, want the annotation admitted", errs)
			}
		})
	}
}
