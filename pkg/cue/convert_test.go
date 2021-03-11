package cue

import (
	"io/ioutil"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/types"
)

func TestGetParameter(t *testing.T) {
	data, _ := ioutil.ReadFile("testdata/workloads/metrics.cue")
	params, err := GetParameters(string(data))
	assert.NoError(t, err)
	assert.Equal(t, params, []types.Parameter{
		{Name: "format", Required: false, Default: "prometheus", Usage: "format of the metrics, " +
			"default as prometheus", Short: "f", Type: cue.StringKind},
		{Name: "enabled", Required: false, Default: true, Type: cue.BoolKind},
		{Name: "port", Required: false, Default: int64(8080), Type: cue.IntKind},
		{Name: "selector", Required: false, Usage: "the label selector for the pods, default is the workload labels", Type: cue.StructKind},
	})
	data, _ = ioutil.ReadFile("testdata/workloads/deployment.cue")
	params, err = GetParameters(string(data))
	assert.NoError(t, err)
	assert.Equal(t, []types.Parameter{
		{Name: "name", Required: true, Default: "", Type: cue.StringKind},
		{Name: "env", Required: false, Default: nil, Type: cue.ListKind},
		{Name: "image", Short: "i", Required: true, Usage: "Which image would you like to use for your service", Default: "", Type: cue.StringKind},
		{Name: "port", Short: "p", Required: false, Usage: "Which port do you want customer traffic sent to", Default: int64(8080),
			Type: cue.IntKind},
		{Name: "cpu", Short: "", Required: false, Usage: "", Default: "", Type: cue.StringKind}},
		params)

	data, _ = ioutil.ReadFile("testdata/workloads/test-param.cue")
	params, err = GetParameters(string(data))
	assert.NoError(t, err)
	assert.Equal(t, []types.Parameter{
		{Name: "name", Required: true, Default: "", Type: cue.StringKind},
		{Name: "env", Required: false, Default: nil, Type: cue.ListKind},
		{Name: "image", Short: "i", Required: true, Usage: "Which image would you like to use for your service", Default: "", Type: cue.StringKind},
		{Name: "port", Short: "p", Usage: "Which port do you want customer traffic sent to", Default: int64(8080), Type: cue.IntKind},
		{Name: "enable", Default: false, Type: cue.BoolKind},
		{Name: "fval", Default: 64.3, Type: cue.FloatKind},
		{Name: "nval", Default: float64(0), Required: true, Type: cue.NumberKind}}, params)
	data, _ = ioutil.ReadFile("testdata/workloads/empty.cue")
	params, err = GetParameters(string(data))
	assert.NoError(t, err)
	var exp []types.Parameter
	assert.Equal(t, exp, params)
}
