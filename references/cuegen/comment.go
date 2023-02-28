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
	gotypes "go/types"
	"strings"

	cueast "cuelang.org/go/cue/ast"
)

type commentUnion struct {
	comment *goast.CommentGroup
	doc     *goast.CommentGroup
}

// fieldComments returns the comments for each field in a go struct.
//
// The comments are same order as the fields.
func (g *Generator) fieldComments(x *gotypes.Struct) []*commentUnion {
	comments := make([]*commentUnion, x.NumFields())

	st, ok := g.types[x]
	if !ok {
		return comments
	}

	for i, field := range st.Fields.List {
		comments[i] = &commentUnion{comment: field.Comment, doc: field.Doc}
	}

	return comments
}

// makeComments adds comments to a cue node.
//
// go docs/comments are converted to cue comments.
func makeComments(node cueast.Node, c *commentUnion) {
	if c == nil {
		return
	}
	cg := make([]*cueast.Comment, 0)

	if comment := makeComment(c.comment); comment != nil && len(comment.List) > 0 {
		cg = append(cg, comment.List...)
	}
	if doc := makeComment(c.doc); doc != nil && len(doc.List) > 0 {
		cg = append(cg, doc.List...)
	}

	// avoid nil comment groups which will cause panics
	if len(cg) > 0 {
		cueast.AddComment(node, &cueast.CommentGroup{List: cg})
	}
}

// makeComment converts a go CommentGroup to a cue CommentGroup.
//
// All /*-style comments are converted to //-style comments.
func makeComment(cg *goast.CommentGroup) *cueast.CommentGroup {
	if cg == nil {
		return nil
	}

	var comments []*cueast.Comment

	for _, comment := range cg.List {
		c := comment.Text

		// Remove comment markers.
		// The parser has given us exactly the comment text.
		switch c[1] {
		case '/':
			// -style comment (no newline at the end)
			comments = append(comments, &cueast.Comment{Text: c})

		case '*':
			/*-style comment */
			c = c[2 : len(c)-2]
			if len(c) > 0 && c[0] == '\n' {
				c = c[1:]
			}

			lines := strings.Split(c, "\n")

			// Find common space prefix
			i := 0
			line := lines[0]
			for ; i < len(line); i++ {
				if c := line[i]; c != ' ' && c != '\t' {
					break
				}
			}

			for _, l := range lines {
				for j := 0; j < i && j < len(l); j++ {
					if line[j] != l[j] {
						i = j
						break
					}
				}
			}

			// Strip last line if empty.
			if n := len(lines); n > 1 && len(lines[n-1]) < i {
				lines = lines[:n-1]
			}

			// Print lines.
			for _, l := range lines {
				if i >= len(l) {
					comments = append(comments, &cueast.Comment{Text: "//"})
					continue
				}
				comments = append(comments, &cueast.Comment{Text: "// " + l[i:]})
			}
		}
	}

	return &cueast.CommentGroup{List: comments}
}
