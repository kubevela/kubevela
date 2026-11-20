/*
 Copyright 2026. The KubeVela Authors.

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

// Package nesting measures how deeply CUE source nests before it is parsed. CUE's parser
// recurses once per level with no limit, and a deep enough source overflows the stack, which
// is fatal to the process: no recover catches it.
package nesting

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"cuelang.org/go/cue/scanner"
	"cuelang.org/go/cue/token"
)

// MaxDepth is the deepest nesting CUE source handed to the parser may have. Shipped definitions
// nest at most 18 levels; at about 3 KB of stack a level, this costs a few megabytes.
const MaxDepth = 1000

// chainFactor scales the depth limit to the longest chain of operations one expression may apply
// in sequence: a chain builds an expression as deep as it is long, which the compiler recurses
// through, but real templates chain far less than they nest, so it is given more room.
const chainFactor = 10

// Check refuses src when it nests deeper than max, or chains more than chainFactor times max
// operations in one expression.
func Check(src string, max int) error {
	if levels, chain := measure(src, max, max*chainFactor); levels > max || chain > max*chainFactor {
		return fmt.Errorf("CUE is nested deeper than %d levels", max)
	}
	return nil
}

// Depth is the deepest nesting of src: braces, brackets, parentheses, string interpolations,
// chained labels (a: b: c: nests a struct per label after the first) and prefix operators, as the
// parser recurses through them. Strings and comments do not count.
func Depth(src string) int {
	levels, _ := measure(src, -1, -1)
	return levels
}

// level is what one bracket level holds while it is read.
type level struct {
	// interpolation marks a level opened by a string interpolation, which the scanner leaves the
	// caller to resume at its closing parenthesis.
	interpolation bool
	// labels counts the labels the field being read has chained.
	labels int
	// prefix counts the prefix operators applied to the operand being read.
	prefix int
	// ops counts the operations (binary operators, selectors, indexes, calls) the expression being
	// read has applied in sequence.
	ops int
}

// meter is the state measure keeps while it reads src.
type meter struct {
	s                     scanner.Scanner
	levels                []level
	stopLevels, stopChain int
	deepest, longest      int
	// chained and prefix total, over the open levels, the depth labels and prefix operators add.
	chained, prefix int
	// operand says the last token ended an operand, which decides whether an operator is binary
	// or prefix and whether a bracket indexes or calls what came before it.
	interpolating, label, operand bool
}

// measure reads src with CUE's scanner, which is iterative, and returns its deepest nesting and
// its longest chain of operations, stopping once either passes its stop (when not negative).
func measure(src string, stopLevels, stopChain int) (deepest, longest int) {
	m := &meter{levels: []level{{}}, stopLevels: stopLevels, stopChain: stopChain}
	file := token.NewFile("", -1, len(src))
	m.s.Init(file, []byte(src), func(token.Pos, string, []interface{}) {}, scanner.ScanComments)
	for {
		_, tok, lit := m.s.Scan()
		if tok == token.EOF || m.next(tok, lit) {
			return m.deepest, m.longest
		}
	}
}

func (m *meter) top() *level { return &m.levels[len(m.levels)-1] }

// stop records the depth so far and reports whether either stop is passed.
func (m *meter) stop() bool {
	if d := len(m.levels) - 1 + m.chained + m.prefix; d > m.deepest {
		m.deepest = d
	}
	return m.stopLevels >= 0 && m.deepest > m.stopLevels || m.stopChain >= 0 && m.longest > m.stopChain
}

func (m *meter) endOperand() {
	m.prefix -= m.top().prefix
	m.top().prefix = 0
}

func (m *meter) endExpr() {
	m.endOperand()
	if n := m.top().labels; n > 1 {
		m.chained -= n - 1
	}
	m.top().labels, m.top().ops = 0, 0
}

// apply counts an operation applied to the expression being read.
func (m *meter) apply() bool {
	m.top().ops++
	if m.top().ops > m.longest {
		m.longest = m.top().ops
	}
	return m.stop()
}

// addPrefix counts a prefix operator applied to the operand being read.
func (m *meter) addPrefix() bool {
	m.top().prefix++
	m.prefix++
	return m.stop()
}

// next reads one token, reporting whether a stop is passed.
func (m *meter) next(tok token.Token, lit string) bool {
	wasOperand := m.operand
	m.operand = false
	switch {
	case tok == token.COMMENT || tok == token.ATTRIBUTE:
		m.operand = wasOperand
		return false
	case tok == token.INTERPOLATION:
		m.interpolating, m.label = true, false
		return false
	case tok.IsLiteral() || tok == token.TRUE || tok == token.FALSE || tok == token.NULL:
		return m.literal(tok, lit, wasOperand)
	case tok == token.OPTION || tok == token.NOT && wasOperand:
		// a?: and a!: mark a label
		m.interpolating, m.operand = false, wasOperand
		return false
	case tok == token.COLON:
		if m.colon() {
			return true
		}
	case tok == token.COMMA || tok == token.SEMICOLON:
		m.endExpr()
	case tok == token.LBRACE || tok == token.LBRACK || tok == token.LPAREN:
		if m.open(tok, wasOperand) {
			return true
		}
	case tok == token.RBRACE || tok == token.RBRACK || tok == token.RPAREN:
		if m.close() {
			return false
		}
	case tok.IsOperator():
		if m.operator(tok, wasOperand) {
			return true
		}
	}
	m.interpolating, m.label = false, false
	return false
}

// literal reads an identifier, string, number or keyword literal. quo, rem, div and mod after an
// operand are the binary operators of those names.
func (m *meter) literal(tok token.Token, lit string, wasOperand bool) bool {
	if tok == token.IDENT && wasOperand && (lit == "quo" || lit == "rem" || lit == "div" || lit == "mod") {
		m.endOperand()
		m.interpolating, m.label = false, false
		return m.apply()
	}
	m.interpolating, m.operand = false, true
	m.label = tok == token.IDENT || tok == token.STRING
	return false
}

// operator reads an operator token: binary after an operand, prefix otherwise where CUE allows it.
func (m *meter) operator(tok token.Token, wasOperand bool) bool {
	switch tok {
	case token.NOT:
		return m.addPrefix()
	case token.ADD, token.SUB, token.MUL, token.LSS, token.LEQ, token.GEQ, token.GTR,
		token.NEQ, token.MAT, token.NMAT, token.EQL:
		if !wasOperand {
			return m.addPrefix()
		}
		m.endOperand()
		return m.apply()
	case token.AND, token.OR, token.QUO, token.POW, token.LAND, token.LOR:
		m.endOperand()
		return m.apply()
	case token.BIND, token.ARROW:
		m.endOperand()
		m.top().ops = 0
	case token.PERIOD:
		return wasOperand && m.apply()
	default:
	}
	return false
}

// colon ends the value read before it and, after a label, chains the field's labels.
func (m *meter) colon() bool {
	m.endOperand()
	m.top().ops = 0
	if !m.label {
		return false
	}
	m.top().labels++
	if m.top().labels > 1 {
		m.chained++
		return m.stop()
	}
	return false
}

// open reads an opening bracket: an index or call after an operand, a struct after a
// comprehension clause's expression, a new level either way.
func (m *meter) open(tok token.Token, wasOperand bool) bool {
	if wasOperand && tok == token.LBRACE {
		m.endOperand()
		m.top().ops = 0
	} else if wasOperand && m.apply() {
		return true
	}
	m.levels = append(m.levels, level{interpolation: tok == token.LPAREN && m.interpolating})
	return m.stop()
}

// close reads a closing bracket, reporting whether it resumed a string interpolation, which sets
// the state for the rest of the string itself.
func (m *meter) close() bool {
	if len(m.levels) == 1 {
		return false
	}
	m.endExpr()
	closed := *m.top()
	m.levels = m.levels[:len(m.levels)-1]
	if closed.interpolation {
		// The rest of the string, up to its end or its next interpolation.
		m.interpolating, m.label = strings.HasSuffix(m.s.ResumeInterpolation(), "("), false
		m.operand = !m.interpolating
		return true
	}
	m.operand = true
	return false
}

// CheckJSONStrings refuses a JSON document any of whose strings nests deeper than max as CUE,
// naming the first such field. It is for objects, such as definitions, that carry CUE in their
// string fields. A document that does not decode is left to the caller's decoder to report.
func CheckJSONStrings(raw []byte, max int) error {
	var doc interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil //nolint:nilerr // the caller's decoder reports it
	}
	return checkStrings(doc, "", max)
}

func checkStrings(v interface{}, path string, max int) error {
	switch val := v.(type) {
	case string:
		if err := Check(val, max); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child := k
			if path != "" {
				child = path + "." + k
			}
			if err := checkStrings(val[k], child, max); err != nil {
				return err
			}
		}
	case []interface{}:
		for i, item := range val {
			if err := checkStrings(item, fmt.Sprintf("%s[%d]", path, i), max); err != nil {
				return err
			}
		}
	}
	return nil
}
