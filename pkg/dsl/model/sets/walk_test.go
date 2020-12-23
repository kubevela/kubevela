package sets

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"github.com/bmizerany/assert"
)

func TestWalk(t *testing.T) {

	testCases := []string{
		`x: "124"`,

		`{ x: y: string }`,

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

		`   x: 12
		if x==12 {
			y: "test string"
		}
	`,

		`   item1: {
			x: 12
			if x==12 {
				y: "test string"
			}
		}
        output: [item1]
	`,
		`import "strings"

		#User: {
		    tags_str: string
		    tags_map: {
 		        for k, v in strings.Split(tags_str, " ") {
  		           "\(v)": string
  		       	}
  		       "{a}": string
  		  	}
		}

		user: {
		    #User
		    tags_str: "b {c}"
		}
	`,
	}

	for _, src := range testCases {
		var r cue.Runtime
		inst, err := r.Compile("-", src)
		if err != nil {
			t.Error(err)
			return
		}
		nsrc, err := toString(inst.Value())
		if err != nil {
			t.Error(err)
			return
		}

		f, err := parser.ParseFile("-", nsrc)
		if err != nil {
			t.Error(err)
			return
		}

		newWalker(func(node ast.Node, ctx walkCtx) {
			if len(ctx.Pos()) == 0 {
				return
			}

			if _, ok := node.(ast.Expr); !ok {
				return
			}
			if _, ok := node.(*ast.CallExpr); ok {
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
