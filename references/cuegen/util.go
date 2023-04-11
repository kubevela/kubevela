/*
Copyright 2023 The KubeVela Authors.

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

package cuegen

import (
	"fmt"
	gotypes "go/types"
	"strconv"

	cueast "cuelang.org/go/cue/ast"
	cuetoken "cuelang.org/go/cue/token"
)

// Ident returns a new cue identifier with the given name.
func Ident(name string, isDef bool) *cueast.Ident {
	if isDef {
		name = "#" + name
	}
	return cueast.NewIdent(name)
}

func basicType(x *gotypes.Basic) cueast.Expr {
	// byte is an alias for uint8 in go/types

	switch t := x.String(); t {
	case "uintptr":
		return Ident("uint64", false)
	case "byte":
		return Ident("uint8", false)
	default:
		return Ident(t, false)
	}
}

func anyLit() cueast.Expr {
	return &cueast.StructLit{Elts: []cueast.Decl{&cueast.Ellipsis{}}}
}

func basicLabel(t *gotypes.Basic, v string) (cueast.Expr, error) {
	switch {
	case t.Info()&gotypes.IsInteger != 0:
		if _, err := strconv.ParseInt(v, 10, 64); err != nil {
			return nil, err
		}
		return &cueast.BasicLit{Kind: cuetoken.INT, Value: v}, nil
	case t.Info()&gotypes.IsFloat != 0:
		if _, err := strconv.ParseFloat(v, 64); err != nil {
			return nil, err
		}
		return &cueast.BasicLit{Kind: cuetoken.FLOAT, Value: v}, nil
	case t.Info()&gotypes.IsBoolean != 0:
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, err
		}
		return cueast.NewBool(b), nil
	case t.Info()&gotypes.IsString != 0:
		return cueast.NewString(v), nil
	default:
		return nil, fmt.Errorf("unsupported basic type %s", t)
	}
}
