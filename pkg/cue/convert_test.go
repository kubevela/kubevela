package cue

import (
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/pkg/encoding/json"

	"github.com/cloud-native-application/rudrx/api/types"

	"github.com/stretchr/testify/assert"
)

func TestEval(t *testing.T) {
	_, workloadType, err := GetParameters("testdata/workloads/deployment.cue")
	assert.NoError(t, err)
	data, err := Eval("testdata/workloads/deployment.cue", workloadType, map[string]interface{}{
		"image": "nginx:v1",
		"port":  8080,
		"name":  "myapp",
		"env": []interface{}{
			map[string]interface{}{
				"name":  "MYDB",
				"value": "true",
			},
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"myapp"},"spec":{"selector":{"matchLabels":{"app":"myapp"}},"template":{"metadata":{"labels":{"app":"myapp"}},"spec":{"containers":[{"name":"myapp","env":[{"name":"MYDB","value":"true"}],"image":"nginx:v1","ports":[{"name":"default","containerPort":8080,"protocol":"TCP"}]}]}}}}`,
		data)
}

func TestGetparam(t *testing.T) {
	params, workloadType, err := GetParameters("testdata/workloads/deployment.cue")
	assert.NoError(t, err)
	assert.Equal(t, "deployment", workloadType)
	assert.Equal(t, []types.Parameter{
		{Name: "name", Required: true, Default: "", Type: cue.StringKind},
		{Name: "env", Required: false, Default: nil, Type: cue.ListKind},
		{Name: "image", Short: "i", Required: true, Usage: "specify app image", Default: "", Type: cue.StringKind},
		{Name: "port", Short: "p", Usage: "specify port for container", Default: int64(8080), Type: cue.IntKind}}, params)

	params, workloadType, err = GetParameters("testdata/workloads/test-param.cue")
	assert.NoError(t, err)
	assert.Equal(t, "deployment", workloadType)
	assert.Equal(t, []types.Parameter{
		{Name: "name", Required: true, Default: "", Type: cue.StringKind},
		{Name: "env", Required: false, Default: nil, Type: cue.ListKind},
		{Name: "image", Short: "i", Required: true, Usage: "specify app image", Default: "", Type: cue.StringKind},
		{Name: "port", Short: "p", Usage: "specify port for container", Default: int64(8080), Type: cue.IntKind},
		{Name: "enable", Default: false, Type: cue.BoolKind},
		{Name: "fval", Default: 64.3, Type: cue.FloatKind},
		{Name: "nval", Default: float64(0), Required: true, Type: cue.NumberKind}}, params)
}

func TestName(t *testing.T) {
	var r cue.Runtime
	ins, _ := r.Compile("testdata/workloads/deployment.cue", nil)
	ins.Value()
	fmt.Println(json.Marshal(ins.Value()))
}
