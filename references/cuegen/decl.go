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
	goast "go/ast"

	cueast "cuelang.org/go/cue/ast"
	cuetoken "cuelang.org/go/cue/token"
)

// Decl is an interface that can build a cueast.Decl.
type Decl interface {
	Build() cueast.Decl
}

// CommonFields is a struct that contains common fields for all decls.
type CommonFields struct {
	Expr    cueast.Expr
	Name    string
	Comment *goast.CommentGroup
	Doc     *goast.CommentGroup
	Pos     cuetoken.Pos
}

// Struct is a struct that represents a CUE struct.
type Struct struct {
	CommonFields
}

// Build creates a cueast.Decl from Struct.
func (s *Struct) Build() cueast.Decl {
	d := &cueast.Field{
		Label: cueast.NewIdent(s.Name),
		Value: s.Expr,
	}

	makeComments(d, &commentUnion{comment: s.Comment, doc: s.Doc})
	cueast.SetPos(d, s.Pos)

	return d
}
