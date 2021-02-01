package definition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/pkg/dsl/process"
)

func TestWDTemplate(t *testing.T) {

	testCases := []struct {
		templ     string
		params    map[string]interface{}
		expectObj runtime.Object
	}{
		{
			templ: `
output:{
	apiVersion: "apps/v1"
    kind: "Deployment"
	metadata: name: context.name
    spec: replicas: parameter.replicas
}

parameter: {
	replicas: *1 | int
}
`,
			params: map[string]interface{}{
				"replicas": 2,
			},
			expectObj: &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "test"}, "spec": map[string]interface{}{"replicas": int64(2)}}},
		},
	}

	for _, v := range testCases {
		ctx := process.NewContext("test", "myapp")
		wt := NewWorkloadAbstractEngine("-")
		if err := wt.Params(v.params).Complete(ctx, v.templ); err != nil {
			t.Error(err)
			return
		}
		base, assists := ctx.Output()
		assert.Equal(t, 0, len(assists))
		assert.Equal(t, false, base == nil)
		baseObj, err := base.Unstructured()
		assert.Equal(t, nil, err)
		assert.Equal(t, v.expectObj, baseObj)

	}

}

func TestTDTemplate(t *testing.T) {
	baseTemplate := `
output:{
	apiVersion: "apps/v1"
    kind: "Deployment"
	metadata: name: context.name
    spec: {
		replicas: parameter.replicas
		template: spec: {
			containers: [{image: "website:0.1",name:"main"}]
		}	
	}
}

parameter: {
	replicas: *1 | int
}
`
	ctx := process.NewContext("test", "myapp")
	wt := NewWorkloadAbstractEngine("-")
	if err := wt.Params(map[string]interface{}{
		"replicas": 2,
	}).Complete(ctx, baseTemplate); err != nil {
		t.Error(err)
		return
	}

	tds := []struct {
		templ  string
		params map[string]interface{}
	}{
		{
			templ: `
patch: {
      // +patchKey=name
      spec: template: spec: containers: [parameter]
}

parameter: {
	name: string
	image: string
	command?: [...string]
}
`,
			params: map[string]interface{}{
				"name":  "sidecar",
				"image": "metrics-agent:0.2",
			},
		},
	}

	for _, v := range tds {
		td := NewTraitAbstractEngine("-")
		if err := td.Params(v.params).Complete(ctx, v.templ); err != nil {
			t.Error(err)
			return
		}
	}

	base, assists := ctx.Output()
	assert.Equal(t, 0, len(assists))
	assert.Equal(t, false, base == nil)
	obj, err := base.Unstructured()
	assert.Equal(t, nil, err)
	expect := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "test"},
			"spec": map[string]interface{}{
				"replicas": int64(2),
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{map[string]interface{}{"image": "website:0.1", "name": "main"},
							map[string]interface{}{"image": "metrics-agent:0.2", "name": "sidecar"}}}}},
		}}
	assert.Equal(t, expect, obj)
}

func TestCheckHealth(t *testing.T) {
	cases := map[string]struct {
		tpContext  map[string]interface{}
		healthTemp string
		exp        bool
	}{
		"normal-equal": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{
					"status": map[string]interface{}{
						"readyReplicas": 4,
						"replicas":      4,
					},
				},
			},
			healthTemp: "isHealth:  context.output.status.readyReplicas == context.output.status.replicas",
			exp:        true,
		},
		"normal-false": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{
					"status": map[string]interface{}{
						"readyReplicas": 4,
						"replicas":      5,
					},
				},
			},
			healthTemp: "isHealth: context.output.status.readyReplicas == context.output.status.replicas",
			exp:        false,
		},
		"array-case-equal": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{"status": "True"},
						},
					},
				},
			},
			healthTemp: `isHealth: context.output.status.conditions[0].status == "True"`,
			exp:        true,
		},
	}
	for message, ca := range cases {
		healthy, err := checkHealth(ca.tpContext, ca.healthTemp)
		assert.NoError(t, err, message)
		assert.Equal(t, ca.exp, healthy)
	}
}
