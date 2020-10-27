package cue

import (
	"testing"

	"cuelang.org/go/cue"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/api/types"

	"github.com/stretchr/testify/assert"
)

func TestEvalDeployment(t *testing.T) {
	name := "myapp"
	image := "nginx:v1"
	cr, err := Eval("testdata/workloads/deployment.cue", map[string]interface{}{
		"image": image,
		"port":  8080,
		"name":  name,
		"env": []interface{}{
			map[string]interface{}{
				"name":  "MYDB",
				"value": "true",
			},
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, cr.GetAPIVersion(), "apps/v1")
	assert.Equal(t, cr.GetKind(), "Deployment")
	assert.Equal(t, cr.GetName(), name)
	// get containers
	containers, found, err := unstructured.NestedSlice(cr.UnstructuredContent(), "spec", "template", "spec",
		"containers")
	assert.True(t, found)
	assert.Nil(t, err)
	// get first container
	c, ok := containers[0].(map[string]interface{})
	assert.True(t, ok)
	// verify image
	imageName, found, err := unstructured.NestedString(c, "image")
	assert.True(t, found)
	assert.Nil(t, err)
	assert.Equal(t, imageName, image)
	// verify env
	envs, found, err := unstructured.NestedSlice(c, "env")
	assert.True(t, found)
	assert.Nil(t, err)
	env, ok := envs[0].(map[string]interface{})
	assert.True(t, ok)
	envName, found, err := unstructured.NestedString(env, "name")
	assert.True(t, found)
	assert.Nil(t, err)
	assert.Equal(t, envName, "MYDB")
}

func TestGetParameter(t *testing.T) {
	params, err := GetParameters("testdata/workloads/metrics.cue")
	assert.NoError(t, err)
	assert.Equal(t, params, []types.Parameter{
		{Name: "format", Required: false, Default: "prometheus", Usage: "format of the metrics, " +
			"default as prometheus", Short: "f", Type: cue.StringKind},
		{Name: "enabled", Required: false, Default: true, Type: cue.BoolKind},
		{Name: "port", Required: false, Default: int64(8080), Type: cue.IntKind},
		{Name: "selector", Required: false, Usage: "the label selector for the pods, default is the workload labels", Type: cue.StructKind},
	})

	params, err = GetParameters("testdata/workloads/deployment.cue")
	assert.NoError(t, err)
	assert.Equal(t, []types.Parameter{
		{Name: "name", Required: true, Default: "", Type: cue.StringKind},
		{Name: "env", Required: false, Default: nil, Type: cue.ListKind},
		{Name: "image", Short: "i", Required: true, Usage: "specify app image", Default: "", Type: cue.StringKind},
		{Name: "port", Short: "p", Required: false, Usage: "specify port for container", Default: int64(8080),
			Type: cue.IntKind}},
		params)

	params, err = GetParameters("testdata/workloads/test-param.cue")
	assert.NoError(t, err)
	assert.Equal(t, []types.Parameter{
		{Name: "name", Required: true, Default: "", Type: cue.StringKind},
		{Name: "env", Required: false, Default: nil, Type: cue.ListKind},
		{Name: "image", Short: "i", Required: true, Usage: "specify app image", Default: "", Type: cue.StringKind},
		{Name: "port", Short: "p", Usage: "specify port for container", Default: int64(8080), Type: cue.IntKind},
		{Name: "enable", Default: false, Type: cue.BoolKind},
		{Name: "fval", Default: 64.3, Type: cue.FloatKind},
		{Name: "nval", Default: float64(0), Required: true, Type: cue.NumberKind}}, params)
}
