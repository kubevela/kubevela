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

	"gotest.tools/assert"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	addonutil "github.com/oam-dev/kubevela/pkg/utils/addon"
)

func TestKeepAll(t *testing.T) {
	f := KeepAll()
	assert.Equal(t, true, f(unstructured.Unstructured{}))
}

func TestKeepNone(t *testing.T) {
	f := KeepNone()
	assert.Equal(t, false, f(unstructured.Unstructured{}))
}

func TestByOwnerAddon(t *testing.T) {
	// Test with empty addon name
	f := ByOwnerAddon("")
	assert.Equal(t, true, f(unstructured.Unstructured{}))

	f = ByOwnerAddon("addon-name")

	// Test with empty owner refs
	u := unstructured.Unstructured{}
	assert.Equal(t, false, f(u))

	// Test with right owner refs
	u.SetOwnerReferences([]v1.OwnerReference{{
		Name: addonutil.Convert2AppName("addon-name"),
	}})
	assert.Equal(t, true, f(u))

	// Test with wrong owner refs
	u.SetOwnerReferences([]v1.OwnerReference{{
		Name: "addon-name-2",
	}})
	assert.Equal(t, false, f(u))
}

func TestByName(t *testing.T) {
	// Test with empty name
	f := ByName("")
	assert.Equal(t, true, f(unstructured.Unstructured{}))

	f = ByName("name")

	// Test with empty name
	u := unstructured.Unstructured{}
	assert.Equal(t, false, f(u))

	// Test with right name
	u.SetName("name")
	assert.Equal(t, true, f(u))

	// Test with wrong name
	u.SetName("name-2")
	assert.Equal(t, false, f(u))
}
