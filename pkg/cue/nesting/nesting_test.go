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

package nesting

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/require"
)

func TestTheDeepestNestingIsMeasuredAsTheParserRecursesIt(t *testing.T) {
	for src, want := range map[string]int{
		``:                              0,
		`a: 1`:                          0,
		`a: {b: [1, (2)]}`:              3,
		`a: "{{{[[[((("`:                0,
		`a: """` + "\n{{{{\n" + `"""`:   0,
		`a: #"[[[["#`:                   0,
		"// {{{{\na: 1":                 0,
		`a: "x\(b + "\(c)")"`:           2,
		`a: "\([1])\({b: 2})"`:          2,
		`a: #"\#({x: [1]})"#`:           3,
		`a: [` + strings.Repeat("{", 5): 6,
		`a: [1]]]]` + `, b: {c: [d]}`:   2,
		`a: b: c: 1`:                    2,
		`a?: "b"!: 1`:                   1,
		`a: b: 1, c: d: 2`:              1,
		`a: b: {c: d: e: 1}`:            4,
		`a: [b: c: 1]`:                  2,
	} {
		require.Equal(t, want, Depth(src), "%q", src)
	}
}

// Closing brackets in strings cannot hide real nesting: strings are skipped, not counted.
func TestStringsCannotHideNesting(t *testing.T) {
	src := "a: " + strings.Repeat(`["]]]]",`, 2000)
	require.Equal(t, 2000, Depth(src))
}

// A chain of labels nests a struct per label without a brace, and the parser recurses through
// each: a long enough chain overflows the stack as surely as brackets do.
func TestALabelChainIsNesting(t *testing.T) {
	chain := strings.Repeat("a: ", 2_000_000) + "1"
	require.ErrorContains(t, Check(chain, MaxDepth), "nested deeper than 1000 levels")
	require.NoError(t, Check(strings.Repeat("a: ", MaxDepth)+"1", MaxDepth))
}

func TestTooDeepIsRefusedBeforeCUEParsesIt(t *testing.T) {
	poc := "package main\noutput: {\n" + strings.Repeat("[", 374907)
	err := Check(poc, MaxDepth)
	require.ErrorContains(t, err, "nested deeper than 1000 levels")

	ok := "a: " + strings.Repeat("[", MaxDepth-1) + strings.Repeat("]", MaxDepth-1)
	require.NoError(t, Check(ok, MaxDepth))
	_, err = parser.ParseFile("ok.cue", ok)
	require.NoError(t, err, "anything Check lets through, CUE parses without trouble")
}

// The limit leaves real definitions far behind it.
func TestShippedDefinitionsAreFarBelowTheLimit(t *testing.T) {
	deepest, where, longest, longestWhere := 0, "", 0, ""
	require.NoError(t, filepath.WalkDir("../../../vela-templates/definitions", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".cue") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if n := Depth(string(src)); n > deepest {
			deepest, where = n, path
		}
		if _, n := measure(string(src), -1, -1); n > longest {
			longest, longestWhere = n, path
		}
		return nil
	}))
	t.Logf("deepest shipped definition: %d levels (%s); longest chain: %d (%s)", deepest, where, longest, longestWhere)
	require.Positive(t, deepest)
	require.Less(t, deepest, MaxDepth/10, where)
	require.Less(t, longest, MaxDepth*chainFactor/10, longestWhere)
}

// Every string an object carries is checked, so a template in any field is covered, and the
// refusal names the field.
func TestEveryStringOfAnObjectIsChecked(t *testing.T) {
	deep := strings.Repeat("{", MaxDepth+1)
	obj := map[string]interface{}{"spec": map[string]interface{}{
		"schematic": map[string]interface{}{"cue": map[string]interface{}{"template": "output: {}"}},
		"status":    map[string]interface{}{"healthPolicy": deep},
		"list":      []interface{}{"a", map[string]interface{}{"x": "b"}},
	}}
	raw, err := json.Marshal(obj)
	require.NoError(t, err)
	err = CheckJSONStrings(raw, MaxDepth)
	require.ErrorContains(t, err, "spec.status.healthPolicy")
	require.ErrorContains(t, err, "nested deeper than 1000 levels")

	obj["spec"].(map[string]interface{})["status"] = map[string]interface{}{"healthPolicy": "isHealth: true"}
	raw, err = json.Marshal(obj)
	require.NoError(t, err)
	require.NoError(t, CheckJSONStrings(raw, MaxDepth))
}

func TestAnUndecodableDocumentIsLeftToItsDecoder(t *testing.T) {
	require.NoError(t, CheckJSONStrings([]byte("{not json"), MaxDepth))
}

// A prefix operator applies to the operand after it, and the parser recurses once per operator,
// so a run of them nests as deeply as brackets do.
func TestUnaryOperatorsAreNesting(t *testing.T) {
	for src, want := range map[string]int{
		`a: !!!!true`:      4,
		`a: -1`:            1,
		`a: 1 - -1`:        1,
		`a: >=0 & <=10`:    1,
		`a: *"x" | "y"`:    1,
		`a!: 1`:            0,
		`a?: !b`:           1,
		`a: [!x, -y]`:      2,
		`a: !(!(!b))`:      5,
		`a: "\(!x)"`:       2,
		`if !a.b { c: 1 }`: 1,
	} {
		require.Equal(t, want, Depth(src), "%q", src)
	}
	require.ErrorContains(t, Check("a: "+strings.Repeat("!", MaxDepth+1)+"true", MaxDepth), "nested deeper than 1000 levels")
	require.ErrorContains(t, Check("a: "+strings.Repeat("-", MaxDepth+1)+"1", MaxDepth), "nested deeper than 1000 levels")
	ok := "a: " + strings.Repeat("!", MaxDepth) + "true"
	require.NoError(t, Check(ok, MaxDepth))
	_, err := parser.ParseFile("ok.cue", ok)
	require.NoError(t, err)
}

// Operations applied in sequence (operators, selectors, indexes, calls) build an expression as
// deep as the chain is long, which later passes recurse through, so a chain far longer than any
// real template is refused too.
func TestLongChainsOfOperationsAreRefused(t *testing.T) {
	long := 10*MaxDepth + 1
	for name, src := range map[string]string{
		"operators": "a: 1" + strings.Repeat("+1", long),
		"selectors": "a: b" + strings.Repeat(".c", long),
		"indexes":   "a: b" + strings.Repeat("[0]", long),
		"calls":     "a: f" + strings.Repeat("()", long),
	} {
		require.ErrorContains(t, Check(src, MaxDepth), "nested deeper than 1000 levels", name)
	}
	enum := make([]string, MaxDepth+1)
	for i := range enum {
		enum[i] = fmt.Sprintf("%q", fmt.Sprint("v", i))
	}
	require.NoError(t, Check("a: "+strings.Join(enum, " | "), MaxDepth), "an enum of a thousand values is not deep")
	require.NoError(t, Check("a: context.output.status.conditions[0].reason", MaxDepth))
}
