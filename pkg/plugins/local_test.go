package plugins

import (
	"os"
	"testing"

	"github.com/cloud-native-application/rudrx/api/types"

	"github.com/stretchr/testify/assert"
)

func TestLocalSink(t *testing.T) {
	deployment := types.Template{
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
	statefulset := types.Template{
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
	route := types.Template{
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
		tmps   []types.Template
		Type   types.DefinitionType
		expDef []types.Template
	}{
		"Test No Templates": {
			dir:  "vela-test1",
			tmps: nil,
		},
		"Test Only Workload": {
			dir:    "vela-test2",
			tmps:   []types.Template{deployment, statefulset},
			Type:   types.TypeWorkload,
			expDef: []types.Template{deployment, statefulset},
		},
		"Test Only Trait": {
			dir:    "vela-test3",
			tmps:   []types.Template{route},
			Type:   types.TypeTrait,
			expDef: []types.Template{route},
		},
		"Test Only Workload But want trait": {
			dir:    "vela-test3",
			tmps:   []types.Template{deployment, statefulset},
			Type:   types.TypeTrait,
			expDef: nil,
		},
		"Test Both have Workload and trait But want Workload": {
			dir:    "vela-test4",
			tmps:   []types.Template{deployment, route, statefulset},
			Type:   types.TypeWorkload,
			expDef: []types.Template{deployment, statefulset},
		},
		"Test Both have Workload and trait But want Trait": {
			dir:    "vela-test5",
			tmps:   []types.Template{deployment, route, statefulset},
			Type:   types.TypeTrait,
			expDef: []types.Template{route},
		},
	}
	for name, c := range cases {
		testInDir(t, name, c.dir, c.tmps, c.expDef, c.Type)
	}
}

func testInDir(t *testing.T, casename, dir string, tmps, defexp []types.Template, Type types.DefinitionType) {
	err := os.MkdirAll(dir, 0755)
	assert.NoError(t, err, casename)
	defer os.RemoveAll(dir)
	err = SinkTemp2Local(tmps, dir)
	assert.NoError(t, err, casename)
	gottmps, err := LoadTempFromLocal(dir)
	assert.NoError(t, err, casename)
	assert.Equal(t, tmps, gottmps, casename)
	if Type != "" {
		gotDef, err := GetDefFromLocal(dir, Type)
		assert.NoError(t, err, casename)
		assert.Equal(t, defexp, gotDef, casename)
	}
}
