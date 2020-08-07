package cmd

import (
	"bytes"
	"testing"

	"github.com/gosuri/uitable"

	"gotest.tools/assert"

	"github.com/cloud-native-application/rudrx/api/types"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
)

func Test_printTraitList(t *testing.T) {
	traits := []types.Template{
		{
			Name:      "route",
			CrdName:   "routes.oam.dev",
			AppliesTo: []string{"deployments.apps", "clonsets.alibaba"},
		},
		{
			Name:      "scaler",
			CrdName:   "scaler.oam.dev",
			AppliesTo: []string{"deployments.apps"},
		},
	}
	workloads := []types.Template{
		{
			Name:    "deployment",
			CrdName: "deployments.apps",
		},
		{
			Name:    "clonset",
			CrdName: "clonsets.alibaba",
		},
	}
	newTable := func() *uitable.Table {
		table := uitable.New()
		table.MaxColWidth = 60
		table.AddRow("NAME", "DEFINITION", "APPLIES TO")
		return table
	}
	tb1 := newTable()
	tb1.AddRow("route", "routes.oam.dev", "deployment")
	tb1.AddRow("", "", "clonset")
	tb1.AddRow("scaler", "scaler.oam.dev", "deployment")

	tb2 := newTable()
	tb2.AddRow("route", "routes.oam.dev", "deployment")
	tb2.AddRow("scaler", "scaler.oam.dev", "deployment")

	tb3 := newTable()
	tb3.AddRow("route", "routes.oam.dev", "clonset")

	cases := map[string]struct {
		traits         []types.Template
		workloads      []types.Template
		workloadName   string
		iostream       cmdutil.IOStreams
		ExpectedString string
	}{
		"All Workloads": {
			traits:         traits,
			workloads:      workloads,
			ExpectedString: tb1.String() + "\n",
		},
		"Specify Workload Name deployment": {
			traits:         traits,
			workloads:      workloads,
			workloadName:   "deployment",
			ExpectedString: tb2.String() + "\n",
		},
		"Specify Workload Name clonset": {
			traits:         traits,
			workloads:      workloads,
			workloadName:   "clonset",
			ExpectedString: tb3.String() + "\n",
		},
	}
	for cname, c := range cases {
		b := bytes.Buffer{}
		iostream := cmdutil.IOStreams{Out: &b}
		nn := c.workloadName
		printTraitList(c.traits, c.workloads, &nn, iostream)
		assert.Equal(t, c.ExpectedString, b.String(), cname)
	}
}
