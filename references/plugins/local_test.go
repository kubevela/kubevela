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

package plugins

import (
	"os"
	"testing"

	"github.com/oam-dev/kubevela/apis/types"

	"github.com/stretchr/testify/assert"
)

var (
	deployment = types.Capability{
		Name: "deployment",
		Type: types.TypeComponentDefinition,
		Parameters: []types.Parameter{
			{
				Name:     "image",
				Short:    "i",
				Required: true,
			},
		},
	}
	statefulset = types.Capability{
		Name: "statefulset",
		Type: types.TypeComponentDefinition,
		Parameters: []types.Parameter{
			{
				Name:     "image",
				Short:    "i",
				Required: true,
			},
		},
	}
	route = types.Capability{
		Name: "route",
		Type: types.TypeTrait,
		Parameters: []types.Parameter{
			{
				Name:     "domain",
				Short:    "d",
				Required: true,
			},
		},
	}
)

func TestLocalSink(t *testing.T) {

	cases := map[string]struct {
		dir    string
		tmps   []types.Capability
		Type   types.CapType
		expDef []types.Capability
		err    error
	}{
		"Test No Templates": {
			dir:  "vela-test1",
			tmps: nil,
		},
		"Test Only Workload": {
			dir:    "vela-test2",
			tmps:   []types.Capability{deployment, statefulset},
			Type:   types.TypeComponentDefinition,
			expDef: []types.Capability{deployment, statefulset},
		},
		"Test Only Trait": {
			dir:    "vela-test3",
			tmps:   []types.Capability{route},
			Type:   types.TypeTrait,
			expDef: []types.Capability{route},
		},
		"Test Only Workload But want trait": {
			dir:    "vela-test3",
			tmps:   []types.Capability{deployment, statefulset},
			Type:   types.TypeTrait,
			expDef: nil,
		},
		"Test Both have Workload and trait But want Workload": {
			dir:    "vela-test4",
			tmps:   []types.Capability{deployment, route, statefulset},
			Type:   types.TypeComponentDefinition,
			expDef: []types.Capability{deployment, statefulset},
		},
		"Test Both have Workload and trait But want Trait": {
			dir:    "vela-test5",
			tmps:   []types.Capability{deployment, route, statefulset},
			Type:   types.TypeTrait,
			expDef: []types.Capability{route},
		},
	}
	for name, c := range cases {
		testInDir(t, name, c.dir, c.tmps, c.expDef, c.Type, c.err)
	}
}

func testInDir(t *testing.T, casename, dir string, tmps, defexp []types.Capability, Type types.CapType, err1 error) {
	err := os.MkdirAll(dir, 0755)
	assert.NoError(t, err, casename)
	defer os.RemoveAll(dir)
	number := SinkTemp2Local(tmps, dir)
	assert.Equal(t, len(tmps), number)
	if Type != "" {
		gotDef, err := loadInstalledCapabilityWithType(dir, Type)
		assert.NoError(t, err, casename)
		assert.Equal(t, defexp, gotDef, casename)
	}
}

func TestRemoveLegacyTemps(t *testing.T) {

	cases := []struct {
		caseName string
		newTemps []types.Capability
		rmNum    int
	}{
		{
			caseName: "remove all",
			newTemps: []types.Capability{},
			rmNum:    3,
		},
		{
			caseName: "nothing removed",
			newTemps: []types.Capability{deployment, statefulset, route},
			rmNum:    0,
		},
		{
			caseName: "remove part of existings",
			newTemps: []types.Capability{statefulset, route},
			rmNum:    1,
		},
	}
	for _, c := range cases {
		runInDirRemoveLegacyTemps(t, c.caseName, c.newTemps, c.rmNum)
	}
}

func runInDirRemoveLegacyTemps(t *testing.T, caseName string, newTemps []types.Capability, rmNum int) {
	dir := "vela-test-rm-temps"
	err := os.MkdirAll(dir, 0755)
	assert.NoError(t, err, caseName)
	defer os.RemoveAll(dir)
	existingTemps := []types.Capability{deployment, statefulset, route}
	number := SinkTemp2Local(existingTemps, dir)
	assert.Equal(t, 3, number)
	resultRemoveNum := RemoveLegacyTemps(newTemps, dir)
	assert.Equal(t, rmNum, resultRemoveNum, caseName)
}
