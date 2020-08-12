package plugins

import (
	"os"
	"testing"

	"github.com/cloud-native-application/rudrx/api/types"

	"github.com/stretchr/testify/assert"
)

func TestLocalSink(t *testing.T) {
	deployment := types.Capability{
		Name: "deployment",
		Type: types.TypeWorkload,
		Parameters: []types.Parameter{
			{
				Name:     "image",
				Short:    "i",
				Required: true,
			},
		},
	}
	statefulset := types.Capability{
		Name: "statefulset",
		Type: types.TypeWorkload,
		Parameters: []types.Parameter{
			{
				Name:     "image",
				Short:    "i",
				Required: true,
			},
		},
	}
	route := types.Capability{
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

	cases := map[string]struct {
		dir    string
		tmps   []types.Capability
		Type   types.DefinitionType
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
			Type:   types.TypeWorkload,
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
			Type:   types.TypeWorkload,
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

func testInDir(t *testing.T, casename, dir string, tmps, defexp []types.Capability, Type types.DefinitionType, err1 error) {
	err := os.MkdirAll(dir, 0755)
	assert.NoError(t, err, casename)
	defer os.RemoveAll(dir)
	number := SinkTemp2Local(tmps, dir)
	assert.Equal(t, len(tmps), number)
	gottmps, err := LoadTempFromLocal(dir)
	if err1 != nil {
		assert.Equal(t, err1, err)
	} else {
		assert.NoError(t, err, casename)
	}
	assert.Equal(t, tmps, gottmps, casename)
	if Type != "" {
		gotDef, err := GetDefFromLocal(dir, Type)
		assert.NoError(t, err, casename)
		assert.Equal(t, defexp, gotDef, casename)
	}
}
