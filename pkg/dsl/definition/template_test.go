package definition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/pkg/dsl/process"
)

func TestWorkloadTemplateComplete(t *testing.T) {

	testCases := []struct {
		workloadTemplate string
		params           map[string]interface{}
		expectObj        runtime.Object
		expAssObjs       map[string]runtime.Object
	}{
		{
			workloadTemplate: `
output:{
	apiVersion: "apps/v1"
    kind: "Deployment"
	metadata: name: context.name
    spec: replicas: parameter.replicas
}
parameter: {
	replicas: *1 | int
	type: string
	host: string
}
`,
			params: map[string]interface{}{
				"replicas": 2,
				"type":     "ClusterIP",
				"host":     "example.com",
			},
			expectObj: &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "test"}, "spec": map[string]interface{}{"replicas": int64(2)}}},
		},
		{
			workloadTemplate: `
output:{
	apiVersion: "apps/v1"
    kind: "Deployment"
	metadata: name: context.name
    spec: replicas: parameter.replicas
}
outputs: service: {
	apiVersion: "v1"
    kind: "Service"
	metadata: name: context.name
    spec: type: parameter.type
}
outputs: ingress: {
	apiVersion: "extensions/v1beta1"
    kind: "Ingress"
	metadata: name: context.name
    spec: rules: [{host: parameter.host}]
}

parameter: {
	replicas: *1 | int
	type: string
	host: string
}
`,
			params: map[string]interface{}{
				"replicas": 2,
				"type":     "ClusterIP",
				"host":     "example.com",
			},
			expectObj: &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "test"}, "spec": map[string]interface{}{"replicas": int64(2)}}},
			expAssObjs: map[string]runtime.Object{
				"service": &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Service", "metadata": map[string]interface{}{"name": "test"}, "spec": map[string]interface{}{"type": "ClusterIP"}}},
				"ingress": &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "extensions/v1beta1", "kind": "Ingress", "metadata": map[string]interface{}{"name": "test"}, "spec": map[string]interface{}{"rules": []interface{}{map[string]interface{}{
					"host": "example.com",
				}}}}},
			},
		},
	}

	for _, v := range testCases {
		ctx := process.NewContext("test", "myapp")
		wt := NewWorkloadAbstractEngine("testworkload")
		assert.NoError(t, wt.Params(v.params).Complete(ctx, v.workloadTemplate))
		base, assists := ctx.Output()
		assert.Equal(t, len(v.expAssObjs), len(assists))
		assert.NotNil(t, base)
		baseObj, err := base.Unstructured()
		assert.Equal(t, nil, err)
		assert.Equal(t, v.expectObj, baseObj)
		for _, ss := range assists {
			assert.Equal(t, AuxiliaryWorkload, ss.Type)
			got, err := ss.Ins.Unstructured()
			assert.NoError(t, err)
			assert.Equal(t, got, v.expAssObjs[ss.Name])
		}
	}

}

func TestTraitTemplateComplete(t *testing.T) {

	tds := map[string]struct {
		traitName     string
		traitTemplate string
		params        map[string]interface{}
		expWorkload   *unstructured.Unstructured
		expAssObjs    map[string]runtime.Object
	}{
		"patch trait": {
			traitTemplate: `
patch: {
      // +patchKey=name
      spec: template: spec: containers: [parameter]
}

parameter: {
	name: string
	image: string
	command?: [...string]
}`,
			params: map[string]interface{}{
				"name":  "sidecar",
				"image": "metrics-agent:0.2",
			},
			expWorkload: &unstructured.Unstructured{
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
				}},
		},
		"output trait": {
			traitTemplate: `
output: {
	apiVersion: "v1"
    kind: "Service"
	metadata: name: context.name
    spec: type: parameter.type
}
parameter: {
	type: string
}`,
			params: map[string]interface{}{
				"type": "ClusterIP",
			},
			expWorkload: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata":   map[string]interface{}{"name": "test"},
					"spec": map[string]interface{}{
						"replicas": int64(2),
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{map[string]interface{}{"image": "website:0.1", "name": "main"}}}}},
				}},
			traitName: "t1",
			expAssObjs: map[string]runtime.Object{
				"t1": &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Service", "metadata": map[string]interface{}{"name": "test"}, "spec": map[string]interface{}{"type": "ClusterIP"}}},
			},
		},
		"outputs trait": {
			traitTemplate: `
output: {
	apiVersion: "v1"
    kind: "Service"
	metadata: name: context.name
    spec: type: parameter.type
}
outputs: ingress: {
	apiVersion: "extensions/v1beta1"
    kind: "Ingress"
	metadata: name: context.name
    spec: rules: [{host: parameter.host}]
}
parameter: {
	type: string
	host: string
}`,
			params: map[string]interface{}{
				"type": "ClusterIP",
				"host": "example.com",
			},
			expWorkload: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata":   map[string]interface{}{"name": "test"},
					"spec": map[string]interface{}{
						"replicas": int64(2),
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{map[string]interface{}{"image": "website:0.1", "name": "main"}}}}},
				}},
			traitName: "t2",
			expAssObjs: map[string]runtime.Object{
				"t2": &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Service", "metadata": map[string]interface{}{"name": "test"}, "spec": map[string]interface{}{"type": "ClusterIP"}}},
				"t2ingress": &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "extensions/v1beta1", "kind": "Ingress", "metadata": map[string]interface{}{"name": "test"}, "spec": map[string]interface{}{"rules": []interface{}{map[string]interface{}{
					"host": "example.com",
				}}}}},
			},
		},
	}

	for cassinfo, v := range tds {
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
		td := NewTraitAbstractEngine(v.traitName)
		assert.NoError(t, td.Params(v.params).Complete(ctx, v.traitTemplate))
		base, assists := ctx.Output()
		assert.Equal(t, len(v.expAssObjs), len(assists), cassinfo)
		assert.NotNil(t, base)
		obj, err := base.Unstructured()
		assert.NoError(t, err)
		assert.Equal(t, v.expWorkload, obj, cassinfo)
		for _, ss := range assists {
			got, err := ss.Ins.Unstructured()
			assert.NoError(t, err, cassinfo)
			assert.Equal(t, got, v.expAssObjs[ss.Type+ss.Name], cassinfo, ss.Type+ss.Name)
		}
	}
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
		assert.Equal(t, ca.exp, healthy, message)
	}
}

func TestGetStatus(t *testing.T) {
	cases := map[string]struct {
		tpContext  map[string]interface{}
		statusTemp string
		expMessage string
	}{
		"field-with-array-and-outputs": {
			tpContext: map[string]interface{}{
				"outputs": map[string]interface{}{
					"service": map[string]interface{}{
						"spec": map[string]interface{}{
							"type":      "NodePort",
							"clusterIP": "10.0.0.1",
							"ports": []interface{}{
								map[string]interface{}{
									"port": 80,
								},
							},
						},
					},
					"ingress": map[string]interface{}{
						"rules": []interface{}{
							map[string]interface{}{
								"host": "example.com",
							},
						},
					},
				},
			},
			statusTemp: `message: "type: " + context.outputs.service.spec.type + " clusterIP:" + context.outputs.service.spec.clusterIP + " ports:" + "\(context.outputs.service.spec.ports[0].port)" + " domain:" + context.outputs.ingress.rules[0].host`,
			expMessage: "type: NodePort clusterIP:10.0.0.1 ports:80 domain:example.com",
		},
		"complex status": {
			tpContext: map[string]interface{}{
				"outputs": map[string]interface{}{
					"ingress": map[string]interface{}{
						"spec": map[string]interface{}{
							"rules": []interface{}{
								map[string]interface{}{
									"host": "example.com",
								},
							},
						},
						"status": map[string]interface{}{
							"loadBalancer": map[string]interface{}{
								"ingress": []interface{}{
									map[string]interface{}{
										"ip": "10.0.0.1",
									},
								},
							},
						},
					},
				},
			},
			statusTemp: `if len(context.outputs.ingress.status.loadBalancer.ingress) > 0 {
	message: "Visiting URL: " + context.outputs.ingress.spec.rules[0].host + ", IP: " + context.outputs.ingress.status.loadBalancer.ingress[0].ip
}
if len(context.outputs.ingress.status.loadBalancer.ingress) == 0 {
	message: "No loadBalancer found, visiting by using 'vela port-forward " + context.appName + " --route'\n"
}`,
			expMessage: "Visiting URL: example.com, IP: 10.0.0.1",
		},
	}
	for message, ca := range cases {
		gotMessage, err := getStatusMessage(ca.tpContext, ca.statusTemp)
		assert.NoError(t, err, message)
		assert.Equal(t, ca.expMessage, gotMessage, message)
	}
}
