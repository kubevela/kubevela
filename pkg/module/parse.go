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

package module

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/definition"
)

// ParseModule reads the module source tree from fsys (rooted at the module
// directory) and returns its parsed Module. It reads fsys only; it dispatches
// nothing to a cluster and performs no identity or structural validation
// beyond what validate.go covers.
func ParseModule(fsys fs.FS) (*Module, error) {
	name, err := readCUEStringField(fsys, "_module.cue", "module")
	if err != nil {
		return nil, fmt.Errorf("parse module: read module name: %w", err)
	}
	if err := validateModuleName(name, "_module.cue"); err != nil {
		return nil, fmt.Errorf("parse module: %w", err)
	}

	version, err := readCUEStringField(fsys, "_module.cue", "version")
	if err != nil {
		return nil, fmt.Errorf("parse module: read version: %w", err)
	}
	if err := validateModuleVersion(version, "_module.cue"); err != nil {
		return nil, fmt.Errorf("parse module: %w", err)
	}

	auxiliary, err := readAuxiliaryDir(fsys, "auxiliary")
	if err != nil {
		return nil, fmt.Errorf("parse module: read auxiliary: %w", err)
	}

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("parse module: read root: %w", err)
	}

	lines := map[string]Line{}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "v") {
			continue
		}
		line, err := parseLine(fsys, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("parse line %s: %w", entry.Name(), err)
		}
		if _, exists := lines[line.APIVersion]; exists {
			return nil, fmt.Errorf("parse module: duplicate line %q (from directory %s)", line.APIVersion, entry.Name())
		}
		lines[line.APIVersion] = *line
	}

	if err := validateLines(lines); err != nil {
		return nil, fmt.Errorf("parse module: %w", err)
	}

	return &Module{Name: name, Version: version, Auxiliary: auxiliary, Lines: lines}, nil
}

// ParseModuleDir reads a module from a local directory. It is the
// filesystem-backed convenience over ParseModule, preserving callers that
// have a directory path rather than an fs.FS.
func ParseModuleDir(dir string) (*Module, error) {
	return ParseModule(os.DirFS(dir))
}

// parseLine reads one v<N> line directory from fsys: its apiVersion,
// Composition, and every file under definitions/ in sorted filename order.
func parseLine(fsys fs.FS, dir string) (*Line, error) {
	versionPath := path.Join(dir, "_version.cue")
	apiVersion, err := readCUEStringField(fsys, versionPath, "apiVersion")
	if err != nil {
		return nil, fmt.Errorf("read apiVersion: %w", err)
	}
	if err := validateAPIVersion(apiVersion, versionPath); err != nil {
		return nil, err
	}

	enabled, err := readCUEBoolField(fsys, versionPath, "enabled", true)
	if err != nil {
		return nil, fmt.Errorf("read enabled: %w", err)
	}

	auxiliary, err := readAuxiliaryDir(fsys, path.Join(dir, "auxiliary"))
	if err != nil {
		return nil, fmt.Errorf("read auxiliary: %w", err)
	}

	defsDir := path.Join(dir, "definitions")
	entries, err := fs.ReadDir(fsys, defsDir)
	if err != nil {
		return nil, fmt.Errorf("read definitions dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	if err := validateDefinitionsNotEmpty(names, defsDir); err != nil {
		return nil, err
	}

	defs := make([]map[string]interface{}, 0, len(names))
	for _, name := range names {
		defPath := path.Join(defsDir, name)
		data, err := fs.ReadFile(fsys, defPath)
		if err != nil {
			return nil, fmt.Errorf("read definition %s: %w", name, err)
		}
		rendered, err := renderDefinition(name, data)
		if err != nil {
			return nil, fmt.Errorf("render definition %s: %w", name, err)
		}
		if err := validateDefinitionName(rendered, defPath); err != nil {
			return nil, err
		}
		defs = append(defs, rendered)
	}

	return &Line{APIVersion: apiVersion, Enabled: enabled, Auxiliary: auxiliary, Definitions: defs}, nil
}

// renderDefinition compiles a single definition file into its unstructured
// object: .cue through KubeVela's own definition renderer, .yaml/.yml by
// unmarshaling; any other extension is rejected.
func renderDefinition(name string, data []byte) (map[string]interface{}, error) {
	switch {
	case strings.HasSuffix(name, ".cue"):
		def := &definition.Definition{}
		if err := def.FromCUEString(string(data), nil); err != nil {
			return nil, err
		}
		return def.Object, nil
	case strings.HasSuffix(name, ".yaml"), strings.HasSuffix(name, ".yml"):
		return unmarshalYAML(data)
	default:
		return nil, fmt.Errorf("unsupported definition file type: %s", name)
	}
}

// readAuxiliaryDir reads every file under dir (an auxiliary/ folder) and
// returns the objects they contain, in filename order. A missing dir is not
// an error: it means "no auxiliary objects" for that scope.
func readAuxiliaryDir(fsys fs.FS, dir string) ([]map[string]interface{}, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	var objects []map[string]interface{}
	for _, name := range names {
		p := path.Join(dir, name)
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		decoded, err := decodeAuxiliaryFile(name, data)
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w", p, err)
		}
		objects = append(objects, decoded...)
	}
	return objects, nil
}

// decodeAuxiliaryFile decodes one auxiliary file into the objects it
// contains: a .yaml/.yml file may hold multiple "---"-separated documents,
// each becoming its own object; a .cue file is a single object at its root.
// Any other extension is an error naming the file, not a silent skip.
func decodeAuxiliaryFile(name string, data []byte) ([]map[string]interface{}, error) {
	switch {
	case strings.HasSuffix(name, ".yaml"), strings.HasSuffix(name, ".yml"):
		return decodeYAMLDocuments(data)
	case strings.HasSuffix(name, ".cue"):
		obj, err := decodeCUEObject(data)
		if err != nil {
			return nil, err
		}
		return []map[string]interface{}{obj}, nil
	default:
		return nil, fmt.Errorf("unsupported auxiliary file type: %s", name)
	}
}

// decodeYAMLDocuments splits data on YAML document boundaries and unmarshals
// each into a generic map, skipping empty documents (a leading or trailing
// "---" produces one). This is what lets one auxiliary file hold several
// objects, the same way a plain Kubernetes manifest can.
func decodeYAMLDocuments(data []byte) ([]map[string]interface{}, error) {
	decoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), len(data))
	var docs []map[string]interface{}
	for {
		var raw map[string]interface{}
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if len(raw) == 0 {
			continue
		}
		docs = append(docs, raw)
	}
	return docs, nil
}

// decodeCUEObject compiles a CUE auxiliary file and decodes its root value
// into a generic map. Unlike definitions/*.cue (which goes through
// definition.FromCUEString, the vela-definition wrapper), an auxiliary CUE
// file is a plain Kubernetes object at the top level: apiVersion, kind,
// metadata, and so on directly, not wrapped under a named key.
func decodeCUEObject(data []byte) (map[string]interface{}, error) {
	val := cuecontext.New().CompileBytes(data)
	if err := val.Err(); err != nil {
		return nil, err
	}
	var obj map[string]interface{}
	if err := val.Decode(&obj); err != nil {
		return nil, fmt.Errorf("decode cue value: %w", err)
	}
	return obj, nil
}

func unmarshalYAML(data []byte) (map[string]interface{}, error) {
	obj := map[string]interface{}{}
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// lookupCUEField compiles the small identity CUE file at p in fsys and
// returns the value at fieldPath along with whether it exists, shared by
// readCUEStringField and readCUEBoolField.
func lookupCUEField(fsys fs.FS, p, fieldPath string) (cue.Value, bool, error) {
	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		return cue.Value{}, false, err
	}
	val := cuecontext.New().CompileBytes(data)
	if err := val.Err(); err != nil {
		return cue.Value{}, false, err
	}
	field := val.LookupPath(cue.ParsePath(fieldPath))
	return field, field.Exists(), nil
}

// readCUEStringField returns the string value at fieldPath (e.g. "module",
// "version", "apiVersion"); a missing field is an error since these are
// required identity fields.
func readCUEStringField(fsys fs.FS, p, fieldPath string) (string, error) {
	field, exists, err := lookupCUEField(fsys, p, fieldPath)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("field %s not found in %s", fieldPath, p)
	}
	return field.String()
}

// readCUEBoolField returns the bool value at fieldPath. A missing field
// yields def, so an optional switch like "enabled" can default without every
// module having to declare it.
func readCUEBoolField(fsys fs.FS, p, fieldPath string, def bool) (bool, error) {
	field, exists, err := lookupCUEField(fsys, p, fieldPath)
	if err != nil {
		return false, err
	}
	if !exists {
		return def, nil
	}
	return field.Bool()
}
