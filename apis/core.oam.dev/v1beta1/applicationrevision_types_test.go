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

package v1beta1

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kubevela/pkg/util/compression"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

func TestApplicationRevisionCompression(t *testing.T) {
	// Fill data
	spec := &ApplicationRevisionSpec{}
	spec.Application = Application{Spec: ApplicationSpec{Components: []common.ApplicationComponent{{Name: "test-name"}}}}
	spec.ComponentDefinitions = make(map[string]*ComponentDefinition)
	spec.ComponentDefinitions["def"] = &ComponentDefinition{Spec: ComponentDefinitionSpec{PodSpecPath: "path"}}
	spec.WorkloadDefinitions = make(map[string]WorkloadDefinition)
	spec.WorkloadDefinitions["def"] = WorkloadDefinition{Spec: WorkloadDefinitionSpec{Reference: common.DefinitionReference{Name: "testdef"}}}
	spec.TraitDefinitions = make(map[string]*TraitDefinition)
	spec.TraitDefinitions["def"] = &TraitDefinition{Spec: TraitDefinitionSpec{ControlPlaneOnly: true}}
	spec.ScopeDefinitions = make(map[string]ScopeDefinition)
	spec.ScopeDefinitions["def"] = ScopeDefinition{Spec: ScopeDefinitionSpec{AllowComponentOverlap: true}}
	spec.PolicyDefinitions = make(map[string]PolicyDefinition)
	spec.PolicyDefinitions["def"] = PolicyDefinition{Spec: PolicyDefinitionSpec{ManageHealthCheck: true}}
	spec.WorkflowStepDefinitions = make(map[string]*WorkflowStepDefinition)
	spec.WorkflowStepDefinitions["def"] = &WorkflowStepDefinition{Spec: WorkflowStepDefinitionSpec{Reference: common.DefinitionReference{Name: "testname"}}}
	spec.ReferredObjects = []common.ReferredObject{{RawExtension: runtime.RawExtension{Raw: []byte("123")}}}

	testAppRev := &ApplicationRevision{Spec: *spec}

	marshalAndUnmarshal := func(in *ApplicationRevision) (*ApplicationRevision, int) {
		out := &ApplicationRevision{}
		b, err := json.Marshal(in)
		assert.NoError(t, err)
		if in.Spec.Compression.Type != compression.Uncompressed {
			assert.Contains(t, string(b), fmt.Sprintf("\"type\":\"%s\",\"data\":\"", in.Spec.Compression.Type))
		}
		err = json.Unmarshal(b, out)
		assert.NoError(t, err)
		assert.Equal(t, out.Spec.Compression.Type, in.Spec.Compression.Type)
		assert.Equal(t, out.Spec.Compression.Data, "")
		return out, len(b)
	}

	// uncompressed
	testAppRev.Spec.Compression.SetType(compression.Uncompressed)
	uncomp, uncompsize := marshalAndUnmarshal(testAppRev)

	// zstd compressed
	testAppRev.Spec.Compression.SetType(compression.Zstd)
	zstdcomp, zstdsize := marshalAndUnmarshal(testAppRev)
	// We will compare content later. Clear compression methods since it will interfere
	// comparison and is verified earlier.
	zstdcomp.Spec.Compression.SetType(compression.Uncompressed)

	// gzip compressed
	testAppRev.Spec.Compression.SetType(compression.Gzip)
	gzipcomp, gzipsize := marshalAndUnmarshal(testAppRev)
	gzipcomp.Spec.Compression.SetType(compression.Uncompressed)

	assert.Equal(t, uncomp, zstdcomp)
	assert.Equal(t, zstdcomp, gzipcomp)

	assert.Less(t, zstdsize, uncompsize)
	assert.Less(t, gzipsize, uncompsize)
}
