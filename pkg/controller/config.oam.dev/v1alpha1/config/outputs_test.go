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

package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	icontext "github.com/oam-dev/kubevela/pkg/config/context"
	"github.com/oam-dev/kubevela/pkg/cue/script"
)

const noOutputsTemplate = `
template: {
	parameter: value: string
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		stringData: value: parameter.value
	}
}
`

const secretPlusOneConfigMapTemplate = `
context: {
	name:      string
	namespace: string
}
template: {
	parameter: value: string
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {name: context.name, namespace: context.namespace}
		stringData: value: parameter.value
	}
	outputs: {
		"companion-cm": {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {name: "\(context.name)-extra", namespace: "some-other-namespace"}
			data: value: parameter.value
		}
	}
}
`

const secretPlusTwoConfigMapsTemplate = `
context: {
	name:      string
	namespace: string
}
template: {
	parameter: value: string
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {name: context.name, namespace: context.namespace}
		stringData: value: parameter.value
	}
	outputs: {
		"cm-b": {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: name: "\(context.name)-b"
			data: value: "b"
		}
		"cm-a": {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: name: "\(context.name)-a"
			data: value: "a"
		}
	}
}
`

const twoSecretsOutputTemplate = `
context: {
	name:      string
	namespace: string
}
template: {
	parameter: value: string
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {name: context.name, namespace: context.namespace}
		stringData: value: parameter.value
	}
	outputs: {
		"companion-secret": {
			apiVersion: "v1"
			kind:       "Secret"
			metadata: name: "\(context.name)-companion"
			stringData: value: parameter.value
		}
	}
}
`

const missingKindOutputTemplate = `
context: {
	name:      string
	namespace: string
}
template: {
	parameter: value: string
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {name: context.name, namespace: context.namespace}
		stringData: value: parameter.value
	}
	outputs: {
		"bad": {
			metadata: name: "whatever"
		}
	}
}
`

func TestRenderOutputs(t *testing.T) {
	ctx := context.Background()
	contextValue := icontext.ConfigRenderContext{Name: "cfg1", Namespace: "target-ns"}
	props := map[string]interface{}{"value": "hello"}

	t.Run("no outputs field renders nothing", func(t *testing.T) {
		val, err := script.CUE(noOutputsTemplate).RunAndOutputWithCueX(ctx, contextValue, props)
		require.NoError(t, err)
		outputs, err := renderOutputs(val, "target-ns")
		require.NoError(t, err)
		assert.Empty(t, outputs)
	})

	t.Run("secret output plus one companion configmap", func(t *testing.T) {
		val, err := script.CUE(secretPlusOneConfigMapTemplate).RunAndOutputWithCueX(ctx, contextValue, props)
		require.NoError(t, err)
		outputs, err := renderOutputs(val, "target-ns")
		require.NoError(t, err)
		require.Len(t, outputs, 1)
		cm := outputs[0]
		assert.Equal(t, "ConfigMap", cm.GetKind())
		assert.Equal(t, "cfg1-extra", cm.GetName())
		// namespace is forced to the Config's namespace regardless of what the template set
		assert.Equal(t, "target-ns", cm.GetNamespace())
	})

	t.Run("secret output plus two configmaps, deterministically ordered", func(t *testing.T) {
		val, err := script.CUE(secretPlusTwoConfigMapsTemplate).RunAndOutputWithCueX(ctx, contextValue, props)
		require.NoError(t, err)
		outputs, err := renderOutputs(val, "target-ns")
		require.NoError(t, err)
		require.Len(t, outputs, 2)
		// sorted by output key: "cm-a" before "cm-b"
		assert.Equal(t, "cfg1-a", outputs[0].GetName())
		assert.Equal(t, "cfg1-b", outputs[1].GetName())
		for _, o := range outputs {
			assert.Equal(t, "ConfigMap", o.GetKind())
			assert.Equal(t, "target-ns", o.GetNamespace())
		}
	})

	t.Run("secret output plus a companion secret", func(t *testing.T) {
		val, err := script.CUE(twoSecretsOutputTemplate).RunAndOutputWithCueX(ctx, contextValue, props)
		require.NoError(t, err)
		outputs, err := renderOutputs(val, "target-ns")
		require.NoError(t, err)
		require.Len(t, outputs, 1)
		assert.Equal(t, "Secret", outputs[0].GetKind())
		assert.Equal(t, "cfg1-companion", outputs[0].GetName())
	})

	t.Run("output missing apiVersion/kind is rejected", func(t *testing.T) {
		val, err := script.CUE(missingKindOutputTemplate).RunAndOutputWithCueX(ctx, contextValue, props)
		require.NoError(t, err)
		_, err = renderOutputs(val, "target-ns")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must set apiVersion and kind")
	})
}
