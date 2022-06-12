/*
Copyright 2022 The KubeVela Authors.

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

package filters

import (
	"testing"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	addonutil "github.com/oam-dev/kubevela/pkg/utils/addon"
)

func TestKeepAll(t *testing.T) {
	f := KeepAll()
	if !f(unstructured.Unstructured{}) {
		t.Errorf("expected true, got false")
	}
}

func TestKeepNone(t *testing.T) {
	f := KeepNone()
	if f(unstructured.Unstructured{}) {
		t.Errorf("expected false, got true")
	}
}

func TestByOwnerAddon(t *testing.T) {
	// Test with empty addon name
	f := ByOwnerAddon("")
	if !f(unstructured.Unstructured{}) {
		t.Errorf("expected true, got false")
	}

	f = ByOwnerAddon("addon-name")

	// Test with empty owner refs
	u := unstructured.Unstructured{}
	if f(u) {
		t.Errorf("expected false, got true")
	}

	// Test with right owner refs
	u.SetOwnerReferences([]v1.OwnerReference{{
		Name: addonutil.Convert2AppName("addon-name"),
	}})
	if !f(u) {
		t.Errorf("expected true, got false")
	}

	// Test with wrong owner refs
	u.SetOwnerReferences([]v1.OwnerReference{{
		Name: "addon-name-2",
	}})
	if f(u) {
		t.Errorf("expected false, got true")
	}
}

func TestByName(t *testing.T) {
	// Test with empty name
	f := ByName("")
	if !f(unstructured.Unstructured{}) {
		t.Errorf("expected true, got false")
	}

	f = ByName("name")

	// Test with empty name
	u := unstructured.Unstructured{}
	if f(u) {
		t.Errorf("expected false, got true")
	}

	// Test with right name
	u.SetName("name")
	if !f(u) {
		t.Errorf("expected true, got false")
	}

	// Test with wrong name
	u.SetName("name-2")
	if f(u) {
		t.Errorf("expected false, got true")
	}
}
