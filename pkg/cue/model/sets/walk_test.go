/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sets

import (
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/require"
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
		`import "strings"

		b: string	
		user: {
		    tags_str: strings.Compare(b,"c")
		}
	`,
	}

	for _, src := range testCases {
		re := require.New(t)
		inst := cuecontext.New().CompileString(src)
		nsrc, err := toString(inst.Value())
		re.NoError(err)
		f, err := parser.ParseFile("-", nsrc)
		re.NoError(err)

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
			re.NoError(err)

			re.Equal(n, node, nsrc)
		}).walk(f)
	}

}

func TestRemoveTmpVar(t *testing.T) {
	src := `spec: {
    _tmp: "x"
	list: [{
		_tmp: "x"
		retain: "y"
	}, {
		_tmp: "x"
		retain: "z"
	}]
	retain: "y"
}
`
	r := require.New(t)
	v := cuecontext.New().CompileString(src)
	s, err := toString(v, removeTmpVar)
	r.NoError(err)
	r.Equal(`spec: {
	list: [{
		retain: "y"
	}, {
		retain: "z"
	}]
	retain: "y"
}
`, s)
}
