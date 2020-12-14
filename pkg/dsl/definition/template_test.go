package definition

import (
	"testing"

	"github.com/bmizerany/assert"
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
		ctx := process.NewContext("test")
		wt := NewWDTemplater("-", v.templ)
		if err := wt.Params(v.params).Complete(ctx); err != nil {
			t.Error(err)
			return
		}
		base, assists := ctx.Output()
		assert.Equal(t, 0, len(assists))
		assert.Equal(t, false, base == nil)
		baseObj, err := base.Object(nil)
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
	ctx := process.NewContext("test")
	wt := NewWDTemplater("-", baseTemplate)
	if err := wt.Params(map[string]interface{}{
		"replicas": 2,
	}).Complete(ctx); err != nil {
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
	 _containers: context.input.spec.template.spec.containers+[parameter]
      sepc: template: spec: containers: _containers
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
		td := NewTDTemplater("-", v.templ)
		if err := td.Params(v.params).Complete(ctx); err != nil {
			t.Error(err)
			return
		}
	}

	base, assists := ctx.Output()
	assert.Equal(t, 0, len(assists))
	assert.Equal(t, false, base == nil)
	obj, err := base.Object(nil)
	assert.Equal(t, nil, err)
	expect := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "test"}, "sepc": map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{"containers": []interface{}{map[string]interface{}{"image": "website:0.1", "name": "main"}, map[string]interface{}{"image": "metrics-agent:0.2", "name": "sidecar"}}}}}, "spec": map[string]interface{}{"replicas": int64(2), "template": map[string]interface{}{"spec": map[string]interface{}{"containers": []interface{}{map[string]interface{}{"image": "website:0.1", "name": "main"}}}}}}}
	assert.Equal(t, expect, obj)
}
