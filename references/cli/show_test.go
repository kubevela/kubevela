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

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/types"
)

const BaseDir = "testdata"

var TestDir = filepath.Join(BaseDir, "show")

func TestCreateTestDir(t *testing.T) {
	if _, err := os.Stat(TestDir); err != nil && os.IsNotExist(err) {
		err := os.MkdirAll(TestDir, 0750)
		assert.NoError(t, err)
	}
}

func TestGenerateSideBar(t *testing.T) {
	workloadName := "component1"
	traitName := "trait1"

	cases := map[string]struct {
		reason       string
		capabilities []types.Capability
		want         error
	}{
		"ComponentTypeAndTraitCapability": {
			reason: "valid capabilities",
			capabilities: []types.Capability{
				{
					Name: workloadName,
					Type: types.TypeComponentDefinition,
				},
				{
					Name: traitName,
					Type: types.TypeTrait,
				},
			},
			want: nil,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := generateSideBar(tc.capabilities, TestDir)
			if diff := cmp.Diff(tc.want, got, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ngenerateSideBar(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			data, err := os.ReadFile(filepath.Clean(filepath.Join(TestDir, SideBar)))
			assert.NoError(t, err)
			for _, c := range tc.capabilities {
				assert.Contains(t, string(data), c.Name)
			}
		})
	}
}

func TestGenerateNavBar(t *testing.T) {
	assert.NoError(t, generateNavBar(TestDir))
	_, err := os.Stat(filepath.Clean(filepath.Join(TestDir, NavBar)))
	assert.NoError(t, err)
}

func TestGenerateIndexHTML(t *testing.T) {
	assert.NoError(t, generateIndexHTML(TestDir))
	_, err := os.Stat(filepath.Clean(filepath.Join(TestDir, IndexHTML)))
	assert.NoError(t, err)
}

func TestGenerateCustomCSS(t *testing.T) {
	assert.NoError(t, generateCustomCSS(TestDir))
	_, err := os.Stat(filepath.Clean(filepath.Join(TestDir, CSS)))
	assert.NoError(t, err)
}

func TestGenerateREADME(t *testing.T) {
	workloadName := "component1"
	traitName := "trait1"

	cases := map[string]struct {
		reason       string
		capabilities []types.Capability
		want         error
	}{
		"ComponentTypeAndTraitCapability": {
			reason: "valid capabilities",
			capabilities: []types.Capability{
				{
					Name: workloadName,
					Type: types.TypeComponentDefinition,
				},
				{
					Name: traitName,
					Type: types.TypeTrait,
				},
			},
			want: nil,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := generateREADME(tc.capabilities, TestDir)
			if diff := cmp.Diff(tc.want, got, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ngenerateREADME(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			data, err := os.ReadFile(filepath.Clean(filepath.Join(TestDir, README)))
			assert.NoError(t, err)
			for _, c := range tc.capabilities {
				switch c.Type {
				case types.TypeComponentDefinition:
					assert.Contains(t, string(data), fmt.Sprintf("  - [%s](%s/%s.md)\n", c.Name, types.TypeComponentDefinition, c.Name))
				case types.TypeTrait:
					assert.Contains(t, string(data), fmt.Sprintf("  - [%s](%s/%s.md)\n", c.Name, types.TypeTrait, c.Name))
				}
			}
		})
	}
}

func TestGetWorkloadAndTraits(t *testing.T) {
	type want struct {
		workloads []string
		traits    []string
		policies  []string
	}

	var (
		workloadName = "component1"
		traitName    = "trait1"
		scopeName    = "scope1"
		policyName   = "policy1"
	)

	cases := map[string]struct {
		reason       string
		capabilities []types.Capability
		want         want
	}{
		"ComponentTypeAndTraitCapability": {
			reason: "valid capabilities",
			capabilities: []types.Capability{
				{
					Name: workloadName,
					Type: types.TypeComponentDefinition,
				},
				{
					Name: traitName,
					Type: types.TypeTrait,
				},
			},
			want: want{
				workloads: []string{workloadName},
				traits:    []string{traitName},
			},
		},
		"ScopeTypeCapability": {
			reason: "invalid capabilities",
			capabilities: []types.Capability{
				{
					Name: scopeName,
					Type: types.TypeScope,
				},
			},
			want: want{
				workloads: nil,
				traits:    nil,
			},
		},
		"PolicyTypeCapability": {
			capabilities: []types.Capability{
				{
					Name: policyName,
					Type: types.TypePolicy,
				},
			},
			want: want{
				policies: []string{policyName},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			gotWorkloads, gotTraits, _, gotPolicies := getDefinitions(tc.capabilities)
			assert.Equal(t, tc.want, want{workloads: gotWorkloads, traits: gotTraits, policies: gotPolicies})
		})
	}
}

func TestDeleteTestDir(t *testing.T) {
	if _, err := os.Stat(BaseDir); err == nil {
		err := os.RemoveAll(BaseDir)
		assert.NoError(t, err)
	}
}
