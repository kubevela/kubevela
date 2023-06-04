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
	"testing"

	cueast "cuelang.org/go/cue/ast"
	cuetoken "cuelang.org/go/cue/token"
	"github.com/stretchr/testify/assert"
)

func TestStructBuild(t *testing.T) {
	s := Struct{CommonFields{
		Expr:    cueast.NewIdent("foo"),
		Name:    "bar",
		Comment: &goast.CommentGroup{List: []*goast.Comment{{Text: "foo"}}},
		Doc:     &goast.CommentGroup{List: []*goast.Comment{{Text: "bar"}}},
		Pos:     cuetoken.Newline.Pos(),
	}}

	expected := &cueast.Field{
		Label: cueast.NewIdent("bar"),
		Value: cueast.NewIdent("foo"),
	}
	makeComments(expected, &commentUnion{
		comment: &goast.CommentGroup{List: []*goast.Comment{{Text: "foo"}}},
		doc:     &goast.CommentGroup{List: []*goast.Comment{{Text: "bar"}}},
	})
	cueast.SetPos(expected, cuetoken.Newline.Pos())

	assert.Equal(t, expected, s.Build())
}
