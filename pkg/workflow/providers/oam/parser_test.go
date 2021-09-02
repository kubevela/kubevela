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

package oam

import (
	"context"
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
)

func TestParser(t *testing.T) {
	testCases := []struct {
		src      string
		manifest string
		err      string
	}{
		{
			src: `
settings: {
   name: "c-1"
   properties: {
      kind: "test"
      metadata: name: "web"
      metadata: namespace: "test-ns"
      spec: image: "busybox"
   }
   traits: [
      {
         name: "scale"
         properties: {
            kind: "test"
            metadata: name: "scale"
            metadata: namespace: "test-ns"
            spec: replicas: 10
      }}
   ]
}`,
			manifest: `Name:             ""
Namespace:        ""
RevisionName:     ""
RevisionHash:     ""
ExternalRevision: ""
StandardWorkload: {
	kind: "test"
	metadata: {
		name:      "web"
		namespace: "test-ns"
	}
	spec: {
		image: "busybox"
	}
}
Traits: [{
	kind: "test"
	metadata: {
		name:      "scale"
		namespace: "test-ns"
	}
	spec: {
		replicas: 10
	}
}]
Scopes:                    null
PackagedWorkloadResources: null
PackagedTraitResources:    null
InsertConfigNotReady:      false
`,
		},
		{
			src: `{}`,
			err: "var(path=settings) not exist",
		},
		{
			src: `
settings: {
   name: "c-1"
   properties: {
      metadata: name: "web"
      metadata: namespace: "test-ns"
      spec: image: "busybox"
   }
}`,
			err: `render component(c-1): Object 'Kind' is missing in '{"metadata":{"name":"web","namespace":"test-ns"},"spec":{"image":"busybox"}}'`,
		},
	}

	p := &provider{
		parse: simpleParserForTest,
	}
	for _, tCase := range testCases {
		v, err := value.NewValue(tCase.src, nil)
		assert.NilError(t, err)
		err = p.Parse(nil, v, nil)
		if tCase.err != "" {
			assert.Error(t, err, tCase.err)
			continue
		}
		assert.NilError(t, err)
		val, err := v.LookupValue("value")
		assert.NilError(t, err)
		s, err := val.String()
		assert.NilError(t, err)
		assert.Equal(t, s, tCase.manifest)
	}
}

func simpleParserForTest(ctx context.Context, comp common.ApplicationComponent) (*types.ComponentManifest, error) {
	manifest := new(types.ComponentManifest)
	manifest.StandardWorkload = &unstructured.Unstructured{}
	compJson, err := comp.Properties.MarshalJSON()
	if err != nil {
		return nil, err
	}
	if err := manifest.StandardWorkload.UnmarshalJSON(compJson); err != nil {
		return nil, err
	}

	for _, trait := range comp.Traits {
		traitObject := &unstructured.Unstructured{}
		traitJson, err := trait.Properties.MarshalJSON()
		if err != nil {
			return nil, err
		}
		if err := traitObject.UnmarshalJSON(traitJson); err != nil {
			return nil, err
		}
		manifest.Traits = append(manifest.Traits, traitObject)
	}
	return manifest, nil
}
