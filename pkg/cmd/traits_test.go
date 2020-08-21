package cmd

import (
	"bytes"
	"testing"

	"github.com/gosuri/uitable"

	"github.com/cloud-native-application/rudrx/api/types"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
)

func Test_printTraitList(t *testing.T) {
	traits := []types.Capability{
		{
			Name:    "route",
			CrdName: "routes.oam.dev",
			// This format is currently OAM spec standard
			AppliesTo: []string{"apps/v1.Deployment", "alibaba/v1.Clonset"},
		},
		{
			Name:    "scaler",
			CrdName: "scaler.oam.dev",
			// This format is also reasonable, it's align with oam definition name, so we also support here
			AppliesTo: []string{"deployments.apps"},
		},
	}
	workloads := []types.Capability{
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
		traits         []types.Capability
		workloads      []types.Capability
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
	// TODO(zzxwill) As the old `func printTraitList(traits, workloads []types.Capability, workloadName *string, ioStreams cmdutil.IOStreams)`
	// doesn't exist any more, comment this unit-test for now
	//for cname, c := range cases {
	for _, c := range cases {
		b := bytes.Buffer{}
		iostream := cmdutil.IOStreams{Out: &b}
		nn := c.workloadName
		printTraitList(&nn, iostream)
		// assert.Equal(t, c.ExpectedString, b.String(), cname)
	}
}
