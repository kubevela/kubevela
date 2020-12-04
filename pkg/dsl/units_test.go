package dsl

import (
	"fmt"
	"github.com/oam-dev/kubevela/pkg/dsl/definition"
	"github.com/oam-dev/kubevela/pkg/dsl/processer"
	"testing"
)

func TestNode(t *testing.T) {
	wd:=definition.NewWDTemplater("website",`
output: {
    apiVersion: "apps/v1"
    kind: "Deployment"
	metadata: name: context.name
    spec: template: spec: containers: [
		{ name: "myservice"
          image: parameter.image
          ports: [ {containerPort: 3000}]
          env:[{name: "VERSION"
            value: "CPU HEAVY"}]
        }
    ]
}
parameter: {
	image: *"some/nodeserver:v1" | string
}
`)

	td:=definition.NewTDTemplater("scaler",`
	patch: {
		_containers: context.input.spec.template.spec.containers+[parameter]
        spec: template: spec: containers: _containers
	}
	parameter: {
	   name: string
       image: string
	}
`)

	ctx:=processer.NewContext("test")

	wd.Params(map[string]interface{}{
		"image": "busybox",
	}).Complete(ctx)

	td.Params(map[string]interface{}{
		"name": "sidercar",
		"image": "slslog",
	}).Complete(ctx)

	base,_:=ctx.Output()
	o,_:=base.Object()
	_tt,_:=o.MarshalJSON()
	fmt.Println(string(_tt))

}
