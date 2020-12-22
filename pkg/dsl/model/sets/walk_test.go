package sets

import (
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"github.com/bmizerany/assert"
)

func TestWalk(t *testing.T) {

	testCases := []string{
	`x: "124"`,

	`x: y: 124`,

	`x: {y: 124}`,

	`kind: "Deployment"
    metadata: name: "test"
    spec: replicas: 12`,

	`sidecar: {
		name: "agent"
        image: "test.com/agent:0.1"
	}
	containers: [{
		name: "main"
		image: "webserver:0.2"
	},sidecar]
	`,

	}

	for _, src := range testCases {
		f, err := parser.ParseFile("-", src)
		if err != nil {
			t.Error(err)
			return
		}

		newWalker(func(node ast.Node, ctx walkCtx) {
			if _, ok := node.(*ast.Field); ok {
				return
			}
			n, err := lookUp(f, ctx.Pos()...)
			if err != nil {
				t.Error(err)
			}
			assert.Equal(t, n, node)
		}).walk(f)
	}

}
