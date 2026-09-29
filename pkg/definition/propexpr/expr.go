/*
Copyright 2026 The KubeVela Authors.

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

// Package propexpr implements property expressions: the way an Application
// consumes a resolved source, and reads its own render context.
//
//	properties:
//	  cluster: '$(source.clusterInfo.region + "-cluster")'
//	  owner:   '$("owner" in context.appLabels ? context.appLabels["owner"] : "unassigned")'
//
// The binding name is written with a dot, as above. A name that is not a legal
// CUE identifier - one containing a hyphen - needs bracket syntax instead,
// source["my-source"].region, because CUE parses the dotted form as subtraction.
// Validate says so explicitly rather than letting the evaluator complain about
// an undefined field.
//
// An expression's result type is checked at admission against the parameter it
// feeds. That is only sound because the grammar admits nothing whose result type
// could depend on a value rather than on operand types - no conditionals, no
// comparisons, no function calls, and exactly one disjunction, the default. See
// Validate.
//
// This supersedes an earlier `fromSource:` directive, which could name a value
// but not compute with one. Expressions are a strict superset of it: whole
// structs and lists substitute, `default:` becomes `*x | y`, and paths are
// schema-checked the same way - so the directive was removed rather than kept
// alongside, which also removed a second set of enforcement paths that had
// already drifted apart once.
package propexpr

import (
	"errors"
	"fmt"
	"strings"
)

// SourceIdent is the only identifier an expression may reference. Everything a
// consumer is allowed to read hangs off it, so the sandbox is "this name and
// nothing else" rather than a denylist.
const SourceIdent = "source"

const (
	open   = "$("
	escape = "$$("
)

// Fragment is one piece of a property value: either literal text or an
// expression to evaluate.
type Fragment struct {
	Text string
	// Expr is the expression source, without the surrounding $( ).
	Expr string
}

// IsExpr reports whether this fragment needs evaluating.
func (f Fragment) IsExpr() bool { return f.Expr != "" }

// Parsed is a property value broken into fragments.
type Parsed struct {
	Fragments []Fragment
}

// HasExpr reports whether the value contains anything to evaluate. A value with
// none is left exactly as it was.
func (p Parsed) HasExpr() bool {
	for _, f := range p.Fragments {
		if f.IsExpr() {
			return true
		}
	}
	return false
}

// Literal renders the value's text with no expressions evaluated, which is what
// a value with nothing to evaluate should become.
//
// Not the raw string: `$$(` is the escape for a literal `$(`, so a value holding
// only escapes has still asked for something. Returning the raw string there
// left `$$(` in the object, and `$$(` is exactly what an author writes to keep
// Kubernetes' own `$(VAR)` expansion working - so the escape silently broke the
// thing it existed to protect. A value with no delimiter comes back
// byte-identical either way.
func (p Parsed) Literal() string {
	if len(p.Fragments) == 1 && !p.Fragments[0].IsExpr() {
		return p.Fragments[0].Text
	}
	var b strings.Builder
	for _, f := range p.Fragments {
		if !f.IsExpr() {
			b.WriteString(f.Text)
		}
	}
	return b.String()
}

// Whole reports whether the value is a single expression and nothing else.
//
// This is what decides the substituted value's type. A whole value is replaced
// by the expression's typed result, so `replicas: '$(source["s"].count)'` yields
// an int even though YAML carried it as a string. An expression embedded in
// surrounding text can only produce a string, since that is what concatenation
// means. Terraform and Helm draw the same line.
//
// Surrounding whitespace does not count as text. Without that, a single trailing
// space - invisible in YAML and easy to leave behind when editing - would flip
// the substituted type from int to string, and the resulting mismatch would
// surface against the parameter, far from the space that caused it. Adding a
// visible character is a deliberate act; adding a space is not.
func (p Parsed) Whole() bool {
	seen := false
	for _, f := range p.Fragments {
		if f.IsExpr() {
			if seen {
				return false
			}
			seen = true
			continue
		}
		if strings.TrimSpace(f.Text) != "" {
			return false
		}
	}
	return seen
}

// SoleExpr returns the expression of a whole value. It is the expression
// fragment, which is not necessarily the first: leading whitespace is a text
// fragment and still leaves the value whole.
func (p Parsed) SoleExpr() (string, bool) {
	if !p.Whole() {
		return "", false
	}
	for _, f := range p.Fragments {
		if f.IsExpr() {
			return f.Expr, true
		}
	}
	return "", false
}

// Parse splits a property value into literal and expression fragments.
//
// `$$(` is a literal `$(`, so a value that genuinely contains the delimiter can
// still be written. Nesting is not supported: the expression ends at the first
// unbalanced `)`, counting parens so that `$(f((a)))` works.
func Parse(raw string) (Parsed, error) {
	var out Parsed
	var lit strings.Builder

	for i := 0; i < len(raw); {
		if strings.HasPrefix(raw[i:], escape) {
			lit.WriteString(open)
			i += len(escape)
			continue
		}
		if !strings.HasPrefix(raw[i:], open) {
			lit.WriteByte(raw[i])
			i++
			continue
		}

		end, err := closingParen(raw, i+len(open))
		if err != nil {
			return Parsed{}, err
		}
		if lit.Len() > 0 {
			out.Fragments = append(out.Fragments, Fragment{Text: lit.String()})
			lit.Reset()
		}
		expr := strings.TrimSpace(raw[i+len(open) : end])
		if expr == "" {
			return Parsed{}, fmt.Errorf("empty expression at offset %d", i)
		}
		out.Fragments = append(out.Fragments, Fragment{Expr: expr})
		i = end + 1
	}
	if lit.Len() > 0 {
		out.Fragments = append(out.Fragments, Fragment{Text: lit.String()})
	}
	return out, nil
}

// closingParen finds the ')' matching the '$(' that opened at start-2, ignoring
// parens inside string literals so that $(f(")")) is not mis-terminated.
func closingParen(raw string, start int) (int, error) {
	depth := 0
	var quote byte
	for i := start; i < len(raw); i++ {
		c := raw[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			if depth == 0 {
				return i, nil
			}
			depth--
		}
	}
	return 0, fmt.Errorf("unterminated expression: no closing ')' for the %q at offset %d", open, start-len(open))
}

// HasExpression reports whether a decoded properties tree contains anything to
// substitute, so a caller can skip the work entirely when it does not.
func HasExpression(v interface{}) bool {
	found := false
	//nolint:errcheck // the visitor only signals, it never fails
	_ = Walk(v, "", func(_, raw string) error {
		// A parse failure counts as an expression. Callers use this to decide
		// whether to do expression work at all, and answering false for a typo
		// means nothing looks at it: admission skips its checks and the render
		// skips substitution, so `$(source.cfg.host` reaches the object as text.
		// Reporting it here is what lets the parse error be raised where it can
		// name the property it is in.
		parsed, err := Parse(raw)
		if err != nil || parsed.HasExpr() {
			found = true
			return errStopWalk
		}
		return nil
	})
	return found
}

// errStopWalk ends a Walk early. Walk stops at the first error, which is how a
// visitor says it has seen enough.
var errStopWalk = errors.New("stop")
