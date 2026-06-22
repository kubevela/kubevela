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

package addon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestAppliedObjectSinkOption(t *testing.T) {
	var got []client.Object
	opt := WithAppliedObjectSink(func(o client.Object) { got = append(got, o) })
	h := &Installer{}
	opt(h)
	assert.NotNil(t, h.objectSink)
	cm := &unstructured.Unstructured{}
	cm.SetName("x")
	h.emitObject(cm)
	assert.Len(t, got, 1)
	assert.Equal(t, "x", got[0].GetName())
}

func TestEmitObjectNoSinkIsNoop(t *testing.T) {
	h := &Installer{}
	cm := &unstructured.Unstructured{}
	cm.SetName("x")
	assert.NotPanics(t, func() { h.emitObject(cm) })
}
