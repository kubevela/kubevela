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

package cachekey

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue/ast"
	cueformat "cuelang.org/go/cue/format"
	"cuelang.org/go/cue/literal"
	cueparser "cuelang.org/go/cue/parser"
	cuetoken "cuelang.org/go/cue/token"
)

// RulesAnnotation records which inference policy generated a definition's cache
// key. Validation loads the rules with this hash rather than the current ones, so
// changing inference never invalidates a definition already generated.
const RulesAnnotation = "definition.oam.dev/cache-key-rules"

// KeyInputsField is the generated sibling of the key, inside InternalField: the values the resolver
// folds into the identity hash. It is recorded rather than re-derived so that
// inference stays a build-time concern, and so the resolver hashes exactly what
// the template reads rather than, say, every label on the object.
const KeyInputsField = "keyInputs"

// RulesVersionAnnotation records the readable name of the same policy. The hash
// above is what validation looks up; this is for whoever is reading the object
// and wants to know which policy applied without computing anything.
const RulesVersionAnnotation = "definition.oam.dev/cache-key-rules-version"

// Stamp writes the inferred cache key into a SourceDefinition template and
// returns it alongside the hash of the rules used.
//
// A key already present is accepted when it matches what inference produces and
// rejected when it does not. Accepting a match is what lets a definition
// round-trip: `vela def get` emits the stored template, key included, so
// rejecting any key at all would break get, edit, apply. Rejecting a mismatch
// rather than overwriting it keeps the author's belief about how their source is
// cached from being silently corrected.
//
// So there is one rule for both the authored CUE and the applied object: the key
// equals what inference produces, and is written for you when absent.
//
// It is idempotent - the generated key is not itself read when inferring - so
// re-stamping an already stamped template yields the same bytes. That matters
// because a re-apply of an unchanged definition must not show a diff.
func Stamp(definitionName, template string) (string, *Rules, error) {
	rules, err := LoadRules()
	if err != nil {
		return "", nil, err
	}

	dims, err := Infer(template, rules)
	if err != nil {
		return "", nil, err
	}
	expr, err := KeyExpression(definitionName, dims, rules)
	if err != nil {
		return "", nil, err
	}

	file, err := cueparser.ParseFile("-", template, cueparser.ParseComments)
	if err != nil {
		return "", nil, fmt.Errorf("parse cue template: %w", err)
	}
	inputs := KeyInputs(dims)
	if existing, ok := existingInternal(file); ok {
		if existing.hasKey && existing.Key != expr {
			return "", nil, fmt.Errorf("%s.%s is computed from the context this template reads, and %s "+
				"does not match: expected %s. Correct it, or leave the %s block out and it will be "+
				"written for you", InternalField, KeyField, existing.Key, expr, InternalField)
		}
		if err := existing.incomplete(); err != nil {
			return "", nil, err
		}
		if existing.MalformedInputs || !equalStrings(existing.KeyInputs, inputs) {
			return "", nil, fmt.Errorf("%s.%s is computed from the context this template reads, and %v "+
				"does not match: expected %v. Correct it, or leave the %s block out and it will be "+
				"written for you", InternalField, KeyInputsField, existing.KeyInputs, inputs, InternalField)
		}
	}
	setInternal(file, internal{Key: expr, KeyInputs: inputs})

	out, err := cueformat.Node(file)
	if err != nil {
		return "", nil, fmt.Errorf("format stamped template: %w", err)
	}
	return string(out), rules, nil
}

// generatedNotice heads the $internal block in every stamped template.
const generatedNotice = "// Generated from the context this template reads - do not edit. " +
	"Admission re-derives these and rejects a mismatch."

// internal is the generated block: what Stamp writes and Verify re-derives.
type internal struct {
	Key       string
	KeyInputs []string
	// MalformedInputs records that keyInputs was present but unreadable, so it
	// fails comparison rather than passing as the empty list.
	MalformedInputs bool
	// hasKey and hasInputs distinguish a field that is absent from one that is
	// present and empty - "" and [] are both legitimate generated values.
	hasKey, hasInputs bool
}

// incomplete reports a block missing one of its two generated fields.
//
// The block is written whole, so a partial one was hand-edited. Saying that
// plainly beats comparing the half that is there, which would blame whichever
// field the check happened to reach first.
func (in internal) incomplete() error {
	var missing []string
	if !in.hasKey {
		missing = append(missing, KeyField)
	}
	if !in.hasInputs {
		missing = append(missing, KeyInputsField)
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("the %s block is incomplete: %s missing. %s and %s are generated together, so "+
		"leave the block out and both will be written for you",
		InternalField, strings.Join(missing, " and ")+" is", KeyField, KeyInputsField)
}

// setInternal replaces the $internal block, or prepends one.
//
// It is written whole rather than field by field, which is the practical payoff
// of keeping generated values in their own block: there is no merging to do, and
// no way for a stale generated field to survive a regeneration. It leads the
// template so a reader meets the generated part before the authored one.
func setInternal(file *ast.File, in internal) {
	elts := make([]ast.Expr, 0, len(in.KeyInputs))
	for _, s := range in.KeyInputs {
		elts = append(elts, ast.NewString(s))
	}
	block := &ast.Field{
		Label: ast.NewIdent(InternalField),
		Value: &ast.StructLit{Elts: []ast.Decl{
			&ast.Field{Label: ast.NewIdent(KeyField), Value: ast.NewLit(cuetoken.STRING, in.Key)},
			&ast.Field{Label: ast.NewIdent(KeyInputsField), Value: ast.NewList(elts...)},
		}},
	}

	// The block name says it is not yours; the comment says why editing it will
	// not work. Someone reading a stored definition has no other cue.
	ast.SetComments(block, []*ast.CommentGroup{{
		Doc:  true,
		List: []*ast.Comment{{Text: generatedNotice}},
	}})

	for i, decl := range file.Decls {
		field, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		if name, _, err := ast.LabelName(field.Label); err == nil && name == InternalField {
			file.Decls[i] = block
			return
		}
	}
	// After the package clause and any imports, which CUE requires to come first,
	// but before the authored fields - so a reader meets the generated block up
	// front rather than discovering it at the bottom.
	at := 0
	for i, decl := range file.Decls {
		switch decl.(type) {
		case *ast.Package, *ast.ImportDecl, *ast.CommentGroup:
			at = i + 1
		default:
			file.Decls = append(file.Decls[:at:at], append([]ast.Decl{block}, file.Decls[at:]...)...)
			return
		}
	}
	file.Decls = append(file.Decls, block)
}

// existingInternal reads the $internal block already in the template.
//
// A block that is present but malformed is reported as present with whatever
// could be read, so it fails the comparison in Stamp and Verify rather than
// being skipped: the alternative is that a malformed block reads as absent and
// slips through.
func existingInternal(file *ast.File) (internal, bool) {
	var out internal

	st, ok := internalBlock(file)
	if !ok {
		return out, false
	}
	for _, elt := range st.Elts {
		f, ok := elt.(*ast.Field)
		if !ok {
			continue
		}
		name, _, err := ast.LabelName(f.Label)
		if err != nil {
			continue
		}
		switch name {
		case KeyField:
			out.hasKey = true
			if text, err := cueformat.Node(f.Value); err == nil {
				out.Key = string(text)
			}
		case KeyInputsField:
			out.hasInputs = true
			list, ok := stringList(f.Value)
			if !ok {
				// A malformed list must not read as the empty list, which is a
				// legitimate value for a source that reads no context - it would
				// then compare equal and be accepted.
				out.MalformedInputs = true
				continue
			}
			out.KeyInputs = list
		}
	}
	return out, true
}

func internalBlock(file *ast.File) (*ast.StructLit, bool) {
	for _, decl := range file.Decls {
		field, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		if name, _, err := ast.LabelName(field.Label); err != nil || name != InternalField {
			continue
		}
		st, ok := field.Value.(*ast.StructLit)
		return st, ok
	}
	return nil, false
}

// stringList reads a CUE list of string literals, reporting whether it could.
func stringList(expr ast.Expr) ([]string, bool) {
	list, ok := expr.(*ast.ListLit)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(list.Elts))
	for _, elt := range list.Elts {
		lit, ok := elt.(*ast.BasicLit)
		if !ok || lit.Kind != cuetoken.STRING {
			return nil, false
		}
		s, err := literal.Unquote(lit.Value)
		if err != nil {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// Verify re-derives the cache key from a stored template and checks the key the
// template carries matches it.
//
// This is the admission-side half of the contract. The CLI writes the key, but
// nothing stops a stored object being edited or written by hand, so the key is
// re-derived rather than trusted.
//
// rulesHash names the policy that produced the key, taken from the object's
// annotation. It is used in preference to the current rules so that changing
// inference does not invalidate definitions already generated and committed -
// which matters because GitOps re-applies them continuously. An empty hash means
// the object was not produced by `vela def`, and the current rules apply: the key
// still has to be right, just by today's policy.
func Verify(definitionName, template, rulesHash string) error {
	rules, err := rulesFor(rulesHash)
	if err != nil {
		return err
	}

	dims, err := Infer(template, rules)
	if err != nil {
		return err
	}
	expected, err := KeyExpression(definitionName, dims, rules)
	if err != nil {
		return err
	}

	// The stamped policy settles what the key must be, so a definition already
	// committed and re-applied by GitOps keeps passing. It does not settle what
	// the resolver will supply: sourceContext is built from the current rules,
	// so a field those rules no longer key is one the template reads and the
	// render cannot provide. Saying so here beats a definition that is admitted
	// and then loses a context value.
	if err := readsSurviveCurrentRules(template, rules); err != nil {
		return err
	}

	file, err := cueparser.ParseFile("-", template, cueparser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse cue template: %w", err)
	}
	wantInputs := KeyInputs(dims)

	existing, ok := existingInternal(file)
	if !ok {
		return fmt.Errorf("the %s block is missing; %s should be %s and %s should be %v, both computed "+
			"from the context this template reads. Apply the definition with `vela def apply` and it "+
			"will be written for you",
			InternalField, KeyField, expected, KeyInputsField, wantInputs)
	}
	if existing.hasKey && existing.Key != expected {
		return fmt.Errorf("%s.%s is computed from the context this template reads, and %s does not "+
			"match: expected %s", InternalField, KeyField, existing.Key, expected)
	}
	if err := existing.incomplete(); err != nil {
		return err
	}

	// keyInputs needs checking on its own account, not as a formality. Only some
	// fields are inlined into the key, so dropping a hashed-only one - a label
	// value, say - leaves the key matching perfectly while collapsing entries that
	// should be distinct onto a single cache entry.
	if existing.MalformedInputs || !equalStrings(existing.KeyInputs, wantInputs) {
		return fmt.Errorf("%s.%s is computed from the context this template reads, and %v does not "+
			"match: expected %v", InternalField, KeyInputsField, existing.KeyInputs, wantInputs)
	}
	return nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// readsSurviveCurrentRules reports a context read the stamped policy allows and
// the current one does not.
//
// A no-op while one policy is embedded, which is the point: it fails on the
// change that introduces a second and drops a field, rather than leaving that
// to be noticed at render.
func readsSurviveCurrentRules(template string, stamped *Rules) error {
	current, err := LoadRules()
	if err != nil {
		return err
	}
	if current.Hash == stamped.Hash {
		return nil
	}
	if _, err := Infer(template, current); err != nil {
		return fmt.Errorf("this definition was generated against cache-key rules %s and reads context "+
			"the current rules %s do not key, so the resolver could not supply it: %w",
			stamped.Hash, current.Hash, err)
	}
	return nil
}

// rulesFor loads the named policy, or the current one when nothing is named.
func rulesFor(rulesHash string) (*Rules, error) {
	if rulesHash == "" {
		return LoadRules()
	}
	return LoadRulesByHash(rulesHash)
}
