package cue

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEval(t *testing.T) {
	_, workloadType, err := GetParameters("testdata/workloads/deployment/deployment.cue")
	assert.NoError(t, err)
	data, err := Eval("testdata/workloads/deployment/deployment.cue", "testdata/apps/myapp.cue", workloadType)
	assert.NoError(t, err)
	assert.Equal(t, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"myapp"},"spec":{"containers":[{"name":"myapp","env":[{"name":"MYDB","value":"true"}],"image":"nginx:v1","ports":[{"name":"default","containerPort":8080,"protocol":"TCP"}]}]}}`,
		data)
}

func TestGetparam(t *testing.T) {
	params, workloadType, err := GetParameters("testdata/workloads/deployment/deployment.cue")
	assert.NoError(t, err)
	assert.Equal(t, "deployment", workloadType)
	assert.Equal(t, []CueParameter{{Name: "name"}, {Name: "env", Default: "[]"}, {Name: "image"}, {Name: "port", Default: "8080"}}, params)
}
