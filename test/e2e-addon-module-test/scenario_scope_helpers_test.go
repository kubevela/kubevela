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

package addonmoduletest

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/literal"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Each scenario has independent identities even when definitions are installed
// in vela-system. The original fixture trees remain untouched. Scope fields are
// initialized on every worker by the synchronized setup before any specs run.
type scenarioScope struct {
	ID        string
	Root      string
	Namespace string
	Group     string
	suffix    string
}

var scenarioIDs = []string{"01", "02", "03", "04", "05", "06", "08", "09", "10", "11", "12", "13", "14", "15", "16", "18"}
var scenarioScopes = func() map[string]*scenarioScope {
	scopes := make(map[string]*scenarioScope, len(scenarioIDs))
	for _, id := range scenarioIDs {
		scopes[id] = &scenarioScope{ID: id}
	}
	return scopes
}()

var fixtureModules = []string{"widget-kit", "gadget-kit", "probe-kit", "ghost-kit"}
var fixtureAddons = []string{"widget-platform", "widget-latest", "kit-suite", "import-options", "tenant-widgets", "cache-probe", "broken-imports", "ghost-addon"}
var fixtureExports = []string{"widget", "gadget", "probe", "labeler", "note", "gauge", "platform-owner"}
var fixtureToken = regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9._/-]*`)

func newScenarioScope(runID, id, root string) *scenarioScope {
	suffix := "-s" + id + "-" + runID
	return &scenarioScope{
		ID: id, Root: filepath.Join(root, id), suffix: suffix,
		Namespace: "am" + suffix, Group: "s" + id + "-" + runID + ".kit.example.com",
	}
}

func (s *scenarioScope) Path(parts ...string) string {
	return filepath.Join(append([]string{s.Root}, parts...)...)
}

func (s *scenarioScope) GVK(kind string) schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: s.Group, Version: "v1alpha1", Kind: kind}
}

func (s *scenarioScope) Texts(values []string) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = s.Text(value)
	}
	return out
}

func containsFixtureName(names []string, name string) bool {
	for _, candidate := range names {
		if candidate == name {
			return true
		}
	}
	return false
}

// identity maps complete, explicitly recognized identities. Label keys,
// parameter names, versions, API lines and object kinds are not renamed.
func (s *scenarioScope) identity(value string) string {
	if value == "default" {
		return s.Namespace
	}
	if value == "kit-tenant" {
		return value + s.suffix
	}
	if value == "kit.example.com" {
		return s.Group
	}
	for _, plural := range []string{"widgets", "widgetclasses", "gadgets"} {
		if value == plural+".kit.example.com" {
			return plural + "." + s.Group
		}
	}
	if containsFixtureName(fixtureExports, value) {
		return value + s.suffix
	}
	if parts := strings.Split(value, "/"); len(parts) > 1 {
		if parts[0] == "kit.example.com" {
			if parts[1] == "v1alpha1" {
				return s.Group + "/" + strings.Join(parts[1:], "/")
			}
			return value // Label keys retain their fixed domain.
		}
		if parts[0] == "default" || parts[0] == "vela-system" || parts[0] == "modules" {
			for i, part := range parts {
				parts[i] = s.identity(part)
			}
			return strings.Join(parts, "/")
		}
		// Preserve invalid two/four-segment forms as invalid; only their
		// scenario identities change, never the number of segments.
		changed := false
		for i, part := range parts {
			if containsFixtureName(fixtureModules, part) || containsFixtureName(fixtureAddons, part) || containsFixtureName(fixtureExports, part) {
				parts[i], changed = s.identity(part), true
			}
		}
		if changed {
			return strings.Join(parts, "/")
		}
		return value
	}
	for _, prefix := range []string{"addon-secret-", "addon-", "module-"} {
		if strings.HasPrefix(value, prefix) {
			name := strings.TrimPrefix(value, prefix)
			if mapped := s.identity(name); mapped != name {
				return prefix + mapped
			}
		}
	}
	for _, module := range fixtureModules {
		if value == module {
			return module + s.suffix
		}
		if tail, found := strings.CutPrefix(value, module+"-"); found {
			for _, line := range []string{"v1", "v2", "v1beta1"} {
				if name, ok := strings.CutPrefix(tail, line+"-"); ok && containsFixtureName(fixtureExports, name) {
					return module + s.suffix + "-" + line + "-" + name + s.suffix
				}
			}
			return module + s.suffix + "-" + tail
		}
	}
	for _, addon := range fixtureAddons {
		if value == addon {
			return addon + s.suffix
		}
		if tail, found := strings.CutPrefix(value, addon+"-"); found {
			return addon + s.suffix + "-" + tail
		}
	}
	return value
}

// Text also handles quoted identities and object paths in real error messages
// and embedded CUE templates. Ordinary descriptive prose remains unchanged.
// This transforms fixture inputs and expected values, never API responses.
func (s *scenarioScope) Text(value string) string {
	if mapped := s.identity(value); mapped != value {
		return mapped
	}
	var out strings.Builder
	last := 0
	for _, position := range fixtureToken.FindAllStringIndex(value, -1) {
		start, end := position[0], position[1]
		word := value[start:end]
		quoted := start > 0 && (value[start-1] == '"' || value[start-1] == '\'')
		if !quoted && !strings.Contains(word, "/") {
			continue
		}
		if mapped := s.identity(word); mapped != word {
			out.WriteString(value[last:start])
			out.WriteString(mapped)
			last = end
		}
	}
	if last == 0 {
		return value
	}
	out.WriteString(value[last:])
	return out.String()
}

func (s *scenarioScope) rewriteCUE(data []byte, definition bool) ([]byte, error) {
	f, err := parser.ParseFile("fixture.cue", data, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	if definition {
		for _, declaration := range f.Decls {
			if field, ok := declaration.(*ast.Field); ok {
				name, _, err := ast.LabelName(field.Label)
				if err == nil && containsFixtureName(fixtureExports, name) {
					field.Label = ast.NewString(s.identity(name))
				}
			}
		}
	}
	ast.Walk(f, func(node ast.Node) bool {
		if str, ok := node.(*ast.BasicLit); ok && str.Kind == token.STRING {
			// Interpolation fragments are not complete literals. Their
			// context/parameter expressions and output suffixes stay intact.
			if text, err := literal.Unquote(str.Value); err == nil {
				str.Value = literal.String.Quote(s.Text(text))
			}
		}
		return true
	}, nil)
	return format.Node(f)
}

func (s *scenarioScope) rewriteYAML(data []byte) ([]byte, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	var rewrite func(*yaml.Node)
	rewrite = func(node *yaml.Node) {
		if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
			node.Value = s.Text(node.Value)
		}
		for i, child := range node.Content {
			if node.Kind != yaml.MappingNode || i%2 == 1 {
				rewrite(child)
			}
		}
	}
	for {
		var doc yaml.Node
		if err := decoder.Decode(&doc); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		rewrite(&doc)
		if err := encoder.Encode(&doc); err != nil {
			return nil, err
		}
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (s *scenarioScope) Materialize(source string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := s.Path(rel)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		data, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}
		// Invalid publication fixtures are byte-identical, so isolation
		// cannot accidentally repair what the CLI must reject.
		if !strings.HasPrefix(filepath.ToSlash(rel), "modules/invalid/") && rel != "registry.yaml" {
			switch filepath.Ext(path) {
			case ".cue":
				data, err = s.rewriteCUE(data, strings.Contains(filepath.ToSlash(rel), "/definitions/"))
			case ".yaml", ".yml":
				data, err = s.rewriteYAML(data)
			}
			if err != nil {
				return fmt.Errorf("scope scenario %s fixture %s: %w", s.ID, rel, err)
			}
		}
		return os.WriteFile(destination, data, 0o644)
	})
}
