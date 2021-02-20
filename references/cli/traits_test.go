package cli

import (
	"bytes"
	"os"
	"testing"

	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
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
		table := newUITable()
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
		assert.NoError(t, printTraitList(&nn, iostream))
	}
}

func TestNewTraitsCommandPersistentPreRunE(t *testing.T) {
	io := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	fakeC := types.Args{}
	cmd := NewTraitsCommand(fakeC, io)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestTraitsAppliedToAllWorkloads(t *testing.T) {
	trait := types.Capability{
		Name:      "route",
		CrdName:   "routes.oam.dev",
		AppliesTo: []string{"*"},
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
	assert.Equal(t, []string{"*"}, common.ConvertApplyTo(trait.AppliesTo, workloads))
}
