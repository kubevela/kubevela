/*
 Copyright 2021. The KubeVela Authors.

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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/pkg/features"
)

// fakeRESTMapper implements meta.RESTMapper for tests (minimal).
type fakeRESTMapper struct {
	known map[string]bool // key: group|kind|version
}

func newFakeRESTMapper(entries ...[3]string) *fakeRESTMapper {
	m := &fakeRESTMapper{known: map[string]bool{}}
	for _, e := range entries {
		m.known[e[0]+"|"+e[1]+"|"+e[2]] = true
	}
	return m
}

func (f *fakeRESTMapper) key(gk schema.GroupKind, version string) string {
	return gk.Group + "|" + gk.Kind + "|" + version
}

func (f *fakeRESTMapper) Reset() {}
func (f *fakeRESTMapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, errors.New("not implemented")
}
func (f *fakeRESTMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeRESTMapper) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return schema.GroupVersionResource{}, errors.New("not implemented")
}
func (f *fakeRESTMapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	for _, v := range versions {
		if f.known[f.key(gk, v)] {
			return &meta.RESTMapping{
				Resource: schema.GroupVersionResource{
					Group:    gk.Group,
					Version:  v,
					Resource: "",
				},
				GroupVersionKind: schema.GroupVersionKind{
					Group:   gk.Group,
					Version: v,
					Kind:    gk.Kind,
				},
			}, nil
		}
	}
	return nil, errors.New("no match for kind")
}
func (f *fakeRESTMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	m, err := f.RESTMapping(gk, versions...)
	if err != nil {
		return nil, err
	}
	return []*meta.RESTMapping{m}, nil
}
func (f *fakeRESTMapper) ResourceSingularizer(resource string) (string, error) {
	return resource, nil
}

func TestExtractResourceInfo(t *testing.T) {
	tests := map[string]struct {
		cue     string
		want    []ResourceInfo
		wantErr bool
	}{
		"singleOutput": {
			cue: `
output: {
  apiVersion: "apps/v1"
  kind: "Deployment"
  metadata: { name: "my-deploy" }
  spec: {}
}`,
			want: []ResourceInfo{
				{APIVersion: "apps/v1", Kind: "Deployment", Name: "my-deploy"},
			},
		},
		"multipleOutputs": {
			cue: `
outputs: {
  first: {
    apiVersion: "v1"
    kind: "ConfigMap"
    metadata: { name: "cfg" }
  }
  second: {
    apiVersion: "batch/v1"
    kind: "Job"
  }
  third: { kind: "Service" } // missing apiVersion ignored
}`,
			want: []ResourceInfo{
				{APIVersion: "v1", Kind: "ConfigMap", Name: "cfg"},
				{APIVersion: "batch/v1", Kind: "Job", Name: "second"},
			},
		},
		"parseError": {
			cue: `
output: {
  apiVersion: "v1"
  kind: "Pod"
`,
			wantErr: true,
		},
		"outputWithoutKindIgnored": {
			cue: `
output: {
  apiVersion: "v1"
  metadata: { name: "x" }
}`,
			want: []ResourceInfo{},
		},
		"metadataNameOverridesLabel": {
			cue: `
outputs: {
  cfg: { apiVersion: "v1", kind: "ConfigMap", metadata: { name: "real" } }
}`,
			want: []ResourceInfo{
				{APIVersion: "v1", Kind: "ConfigMap", Name: "real"},
			},
		},
		"fallbackNameCoreGroup": {
			cue: `
outputs: {
  svc: { apiVersion: "v1", kind: "Service" }
}`,
			want: []ResourceInfo{
				{APIVersion: "v1", Kind: "Service", Name: "svc"},
			},
		},
		"deepMetadataNameIgnoredFallbackToLabel": {
			cue: `
outputs: {
  deep: { apiVersion: "v1", kind: "ConfigMap", metadata: { other: { name: "inner" } } }
}`,
			want: []ResourceInfo{
				{APIVersion: "v1", Kind: "ConfigMap", Name: "deep"},
			},
		},
		"nonStructOutputsValueIgnored": {
			cue: `
outputs: [
  { apiVersion: "v1", kind: "ConfigMap" }
]`,
			want: []ResourceInfo{},
		},
		"missingAPIVersionAndKindEntriesIgnored": {
			cue: `
outputs: {
  a: { apiVersion: "v1" }
  b: { kind: "ConfigMap" }
  c: { metadata: { name: "x" } }
}`,
			want: []ResourceInfo{},
		},
	}

	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			got, err := ExtractResourceInfo(tc.cue)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			if len(tc.want) == 0 {
				assert.Len(t, got, 0)
			} else {
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestValidateOutputResourcesExist(t *testing.T) {
	// Enable the ValidateResourcesExist feature gate for these tests
	originalState := utilfeature.DefaultMutableFeatureGate.Enabled(features.ValidateResourcesExist)
	defer utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
		string(features.ValidateResourcesExist): originalState,
	})
	utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
		string(features.ValidateResourcesExist): true,
	})

	tests := map[string]struct {
		cue     string
		mapper  *fakeRESTMapper
		wantErr string
	}{
		"singleSuccess": {
			cue: `
output: {
  apiVersion: "apps/v1"
  kind: "Deployment"
  metadata: { name: "my-deploy" }
}`,
			mapper: newFakeRESTMapper([3]string{"apps", "Deployment", "v1"}),
		},
		"multipleSuccess": {
			cue: `
outputs: {
  cm: { apiVersion: "v1", kind: "ConfigMap" }
  jobRes: { apiVersion: "batch/v1", kind: "Job" }
}`,
			mapper: newFakeRESTMapper(
				[3]string{"", "ConfigMap", "v1"},
				[3]string{"batch", "Job", "v1"},
			),
		},
		"missingResource": {
			cue: `
output: {
  apiVersion: "apps/v1"
  kind: "Deployment"
}`,
			mapper:  newFakeRESTMapper(),
			wantErr: "resource type not found on cluster: apps/v1/Deployment",
		},
		"firstOkSecondMissing": {
			cue: `
outputs: {
  a: { apiVersion: "v1", kind: "ConfigMap" }
  b: { apiVersion: "batch/v1", kind: "Job" }
}`,
			mapper:  newFakeRESTMapper([3]string{"", "ConfigMap", "v1"}),
			wantErr: "resource type not found on cluster: batch/v1/Job",
		},
		"ignoreIncompleteEntries": {
			cue: `
outputs: {
  a: { apiVersion: "v1" }
  b: { kind: "ConfigMap" }
  c: { apiVersion: "v1", kind: "ConfigMap" }
}`,
			mapper: newFakeRESTMapper([3]string{"", "ConfigMap", "v1"}),
		},
		"duplicateResourceTypes": {
			cue: `
outputs: {
  one: { apiVersion: "v1", kind: "ConfigMap" }
  two: { apiVersion: "v1", kind: "ConfigMap" }
}`,
			mapper: newFakeRESTMapper([3]string{"", "ConfigMap", "v1"}),
		},
		"malformedApiVersionTrailingSlash": {
			cue: `
outputs: {
  bad: { apiVersion: "apps/", kind: "Deployment" }
}`,
			mapper:  newFakeRESTMapper([3]string{"apps", "Deployment", "v1"}),
			wantErr: "resource type not found on cluster: apps//Deployment",
		},
		"deepMetadataNameDoesNotAffectValidation": {
			cue: `
outputs: {
  deep: { apiVersion: "v1", kind: "ConfigMap", metadata: { other: { name: "x" } } }
}`,
			mapper: newFakeRESTMapper([3]string{"", "ConfigMap", "v1"}),
		},
	}

	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			// Pass nil object for tests - feature gate is disabled by default
			err := ValidateOutputResourcesExist(tc.cue, tc.mapper, nil)
			if tc.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
