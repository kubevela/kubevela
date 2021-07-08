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

package process

import (
	"bytes"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/literal"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"fmt"
	"github.com/pkg/errors"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"github.com/bmizerany/assert"

	"github.com/oam-dev/kubevela/pkg/cue/model"
)

func TestContext(t *testing.T) {
	baseTemplate := `
image: "myserver"
`

	var r cue.Runtime
	inst, err := r.Compile("-", baseTemplate)
	if err != nil {
		t.Error(err)
		return
	}
	base, err := model.NewBase(inst.Value())
	if err != nil {
		t.Error(err)
		return
	}

	serviceTemplate := `
	apiVersion: "v1"
    kind:       "ConfigMap"
`

	svcInst, err := r.Compile("-", serviceTemplate)
	if err != nil {
		t.Error(err)
		return
	}

	svcIns, err := model.NewOther(svcInst.Value())
	if err != nil {
		t.Error(err)
		return
	}

	svcAux := Auxiliary{
		Ins:  svcIns,
		Name: "service",
	}
	targetRequiredSecrets := []RequiredSecrets{{
		ContextName: "conn1",
		Data:        map[string]interface{}{"password": "123"},
	}}

	ctx := NewContext("myns", "mycomp", "myapp", "myapp-v1")
	ctx.InsertSecrets("db-conn", targetRequiredSecrets)
	ctx.SetBase(base)
	ctx.AppendAuxiliaries(svcAux)

	ctxInst, err := r.Compile("-", ctx.ExtendedContextFile())
	if err != nil {
		t.Error(err)
		return
	}

	gName, err := ctxInst.Lookup("context", ContextName).String()
	assert.Equal(t, nil, err)
	assert.Equal(t, "mycomp", gName)

	myAppName, err := ctxInst.Lookup("context", ContextAppName).String()
	assert.Equal(t, nil, err)
	assert.Equal(t, "myapp", myAppName)

	myAppRevision, err := ctxInst.Lookup("context", ContextAppRevision).String()
	assert.Equal(t, nil, err)
	assert.Equal(t, "myapp-v1", myAppRevision)

	myAppRevisionNum, err := ctxInst.Lookup("context", ContextAppRevisionNum).Int64()
	assert.Equal(t, nil, err)
	assert.Equal(t, int64(1), myAppRevisionNum)

	inputJs, err := ctxInst.Lookup("context", OutputFieldName).MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, `{"image":"myserver"}`, string(inputJs))

	outputsJs, err := ctxInst.Lookup("context", OutputsFieldName, "service").MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, "{\"apiVersion\":\"v1\",\"kind\":\"ConfigMap\"}", string(outputsJs))

	ns, err := ctxInst.Lookup("context", ContextNamespace).String()
	assert.Equal(t, nil, err)
	assert.Equal(t, "myns", ns)

	requiredSecrets, err := ctxInst.Lookup("context", "conn1").MarshalJSON()
	assert.Equal(t, nil, err)
	assert.Equal(t, "{\"password\":\"123\"}", string(requiredSecrets))
}

func TestX (t *testing.T){

	src:=`
schema:{
  spec: replicas: *2|int
  continue: *false|bool
  _replicas: int
  if _replicas==2{
	continue: true
  }
  script: " \n#code: {abc: spec.replicas,  uc: string}"
  //continue: spec.replicas!=2
}
 
`
	var r cue.Runtime
	instBase,_:=r.Compile("-",src)
	f,_:=parser.ParseFile("-",src)
	ast.Walk(f,nil, func(node ast.Node) {
		field,ok:=node.(*ast.Field)
		if ok{
			basic,ok:=field.Value.(*ast.BasicLit)
			if ok&&basic.Kind==token.STRING{
				str, _ := literal.Unquote(basic.Value)
				str=strings.TrimSpace(str)
				if strings.HasPrefix(str,"#code:"){
					str=str[6:]
					fmt.Println(str)
					expr,err:=parser.ParseExpr("-",str)
					if err!=nil{
						fmt.Println(err)
					}
					field.Value=expr
				}
			}
		}
	})

	testv,err:=r.CompileFile(f)
	if err!=nil{
		fmt.Println(err)
		return
	}
	fmt.Println(toString(testv.Value()))

	base,_:=model.NewBase(testv.Value())
	patchv,_:=r.Compile("-",`
    schema: {_replicas: 2}
`)
	patchm,_:=model.NewOther(patchv.Value())
	base.Unify(patchm)
	fmt.Println(base.String())
	return

fmt.Println(toString(instBase.Value().Lookup("schema")))
	instP,_:=r.Compile("-",`
spec: replicas: int
_replicas:   spec.replicas
`)

result:=instBase.Value().Fill(instP.Value(),"schema")
	bt,err:=result.MarshalJSON()
fmt.Println(string(bt),err)
}


func toString(v cue.Value) (string, error) {
	v = v.Eval()
	syopts := []cue.Option{cue.All(), cue.DisallowCycles(true), cue.ResolveReferences(true), cue.Docs(true)}

	var w bytes.Buffer
	useSep := false
	format := func(name string, n ast.Node) error {
		if name != "" {
			fmt.Fprintf(&w, "// %s\n", filepath.Base(name))
		} else if useSep {
			fmt.Fprintf(&w, "// ---")
		}
		useSep = true

		f, err := toFile(n)
		if err != nil {
			return err
		}
		b, err := format.Node(f)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	}

	if err := format("", v.Syntax(syopts...)); err != nil {
		return "", err
	}
	instStr := w.String()
	return instStr, nil
}
func toFile(n ast.Node) (*ast.File, error) {
	switch x := n.(type) {
	case nil:
		return nil, nil
	case *ast.StructLit:
		return &ast.File{Decls: x.Elts}, nil
	case ast.Expr:
		ast.SetRelPos(x, token.NoSpace)
		return &ast.File{Decls: []ast.Decl{&ast.EmbedDecl{Expr: x}}}, nil
	case *ast.File:
		return x, nil
	default:
		return nil, errors.Errorf("Unsupported node type %T", x)
	}
}
/*

#cr: {
  kind: string
  apiVersion: string
}

Vector: {
	[string]: #cr
}

vela.#Task & {
	input: "db"
    outputVector: "db-secret"
}

vela.#Task & {
	vector: {
       "server": "myserver"
       "dbSecret": "db-secret"
    }
    #up: {
       patch: op.Patch&{
           target: server
           patch: {
              replicas: 12
           }
       }
    }
}

vela.#Task & {

    vector: {
       "app": "server"
    }
    #up: {
      apply: op.Apply & {
        app & {spec: replicas: 3}
      }
      continue: apply.status.ready!=3
    }

}

server: {

}

input: vela.#Vector & {
	"server": {
        kind: "Deployment"
		...
    }
}

#up: []

output: vela.#Vector & {

}

m:=vela.#Model & {
  up:
}

dbSchema: {
  kind: "Mysql"
  spec: {...}

}



appSchema: {
  Kind: "Deployment"
  spec: {
    podTemplate: {
       spec: containers: [{envs: [{name: "DBConn",value: dbSchema.status.connstring}]}]
    }
  }
}


*/