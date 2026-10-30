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
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/policy/placement"
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

	place, err := readPlacementDefault(fsys, "_module.cue")
	if err != nil {
		return nil, fmt.Errorf("parse module: read placement: %w", err)
	}

	var warnings []string
	auxiliary, err := readAuxiliaryDir(fsys, "auxiliary", &warnings)
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
		line, err := parseLine(fsys, entry.Name(), &warnings)
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

	return &Module{Name: name, Version: version, Placement: place, Warnings: warnings, Auxiliary: auxiliary, Lines: lines}, nil
}

// ParseModuleDir reads a module from a local directory. It is the
// filesystem-backed convenience over ParseModule, preserving callers that
// have a directory path rather than an fs.FS.
func ParseModuleDir(dir string) (*Module, error) {
	return ParseModule(os.DirFS(dir))
}

// parseLine reads one v<N> line directory from fsys: its apiVersion,
// Composition, and every file under definitions/ in sorted filename order.
func parseLine(fsys fs.FS, dir string, warnings *[]string) (*Line, error) {
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

	place, err := readPlacementDefault(fsys, versionPath)
	if err != nil {
		return nil, fmt.Errorf("read placement: %w", err)
	}

	auxiliary, err := readAuxiliaryDir(fsys, path.Join(dir, "auxiliary"), warnings)
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
		if err := checkObjectPlacement(rendered, defPath); err != nil {
			return nil, err
		}
		defs = append(defs, rendered)
	}

	return &Line{APIVersion: apiVersion, Enabled: enabled, Placement: place, Auxiliary: auxiliary, Definitions: defs}, nil
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

// readAuxiliaryDir reads every file directly under dir, then every file
// directly under dir/hub. A file in hub/ with no placement annotation is
// stamped resource.oam.dev/placement: hub, so the folder beats the module
// and line defaults. An annotation already on the object wins, and a
// runtime annotation there is a warning. hub/ is not recursed. Any other
// subdirectory is skipped with a warning. A missing dir is not an error:
// it means "no auxiliary objects".
func readAuxiliaryDir(fsys fs.FS, dir string, warnings *[]string) ([]map[string]interface{}, error) {
	objects, err := readAuxiliaryFiles(fsys, dir, false, warnings)
	if err != nil {
		return nil, err
	}
	if err := warnOtherAuxiliaryDirs(fsys, dir, warnings); err != nil {
		return nil, err
	}
	hubDir := path.Join(dir, "hub")
	if err := warnNestedHubDirs(fsys, hubDir, warnings); err != nil {
		return nil, err
	}
	hubObjects, err := readAuxiliaryFiles(fsys, hubDir, true, warnings)
	if err != nil {
		return nil, err
	}
	return append(objects, hubObjects...), nil
}

// readAuxiliaryFiles reads the files directly in dir, in filename order.
// Subdirectories are skipped. A missing dir returns no objects. When hubFolder
// is set, an object with no placement annotation is stamped hub.
func readAuxiliaryFiles(fsys fs.FS, dir string, hubFolder bool, warnings *[]string) ([]map[string]interface{}, error) {
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
		for _, obj := range decoded {
			if err := checkObjectPlacement(obj, p); err != nil {
				return nil, err
			}
			if !hubFolder {
				continue
			}
			if err := applyHubFolder(obj, p, warnings); err != nil {
				return nil, err
			}
		}
		objects = append(objects, decoded...)
	}
	return objects, nil
}

// warnOtherAuxiliaryDirs records subdirectories other than hub/. Those files
// are not installed. A missing dir is not an error.
func warnOtherAuxiliaryDirs(fsys fs.FS, dir string, warnings *[]string) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	names := dirNamesOtherThanHub(entries)
	for _, name := range names {
		notePlacement(warnings, fmt.Sprintf("%s is not a recognized folder and is ignored; only hub/ is recognized under auxiliary/", path.Join(dir, name)))
	}
	return nil
}

// warnNestedHubDirs records subdirectories of hub/. Those files are not
// installed, because hub/ is not recursed. A missing hub/ is not an error.
func warnNestedHubDirs(fsys fs.FS, hubDir string, warnings *[]string) error {
	entries, err := fs.ReadDir(fsys, hubDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		notePlacement(warnings, fmt.Sprintf("%s is not read; hub/ does not recurse into sub-folders", path.Join(hubDir, name)))
	}
	return nil
}

func dirNamesOtherThanHub(entries []fs.DirEntry) []string {
	var names []string
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "hub" {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

func notePlacement(warnings *[]string, msg string) {
	*warnings = append(*warnings, msg)
	klog.Warning(msg)
}

// checkObjectPlacement rejects a placement annotation that is not hub or
// runtime, naming the file. An object with no annotation is valid. A marker
// on a definition is checked the same way and then ignored: definitions
// install on the hub either way.
func checkObjectPlacement(obj map[string]interface{}, filePath string) error {
	_, _, err := objectPlacementValue(obj)
	if err != nil {
		return fmt.Errorf("%s: %w", filePath, err)
	}
	return nil
}

// applyHubFolder stamps hub on an object that has no placement annotation.
// An annotation already on the object wins. runtime inside hub/ is a warning.
func applyHubFolder(obj map[string]interface{}, filePath string, warnings *[]string) error {
	value, ok, err := objectPlacementValue(obj)
	if err != nil {
		return fmt.Errorf("%s: %w", filePath, err)
	}
	if !ok {
		if err := stampHub(obj); err != nil {
			return fmt.Errorf("stamp %s: %w", filePath, err)
		}
		return nil
	}
	if value == placement.Runtime {
		notePlacement(warnings, fmt.Sprintf("%s is in hub/ but its own placement annotation is %q; the annotation wins, so it is not hub-only", filePath, value))
	}
	return nil
}

func objectPlacementValue(obj map[string]interface{}) (string, bool, error) {
	meta, _ := obj["metadata"].(map[string]interface{})
	if meta == nil {
		return "", false, nil
	}
	rawAnns, exists := meta["annotations"]
	if !exists || rawAnns == nil {
		return "", false, nil
	}
	anns, ok := rawAnns.(map[string]interface{})
	if !ok {
		return "", false, fmt.Errorf("metadata.annotations is not an object")
	}
	raw, exists := anns[placement.Annotation]
	if !exists || raw == nil {
		return "", false, nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", false, fmt.Errorf("annotation %s must be a string", placement.Annotation)
	}
	if _, err := placement.Normalize(value); err != nil {
		return "", false, err
	}
	return value, true, nil
}

// stampHub sets resource.oam.dev/placement to hub. The caller has already
// checked that the object has no placement annotation of its own.
func stampHub(obj map[string]interface{}) error {
	raw, ok := obj["metadata"]
	if !ok || raw == nil {
		raw = map[string]interface{}{}
		obj["metadata"] = raw
	}
	meta, ok := raw.(map[string]interface{})
	if !ok {
		return fmt.Errorf("metadata is not an object")
	}
	var anns map[string]interface{}
	if rawAnn, exists := meta["annotations"]; exists && rawAnn != nil {
		anns, ok = rawAnn.(map[string]interface{})
		if !ok {
			return fmt.Errorf("metadata.annotations is not an object")
		}
	} else {
		anns = map[string]interface{}{}
		meta["annotations"] = anns
	}
	anns[placement.Annotation] = placement.Hub
	return nil
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
			if errors.Is(err, io.EOF) {
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

// readPlacementDefault reads the optional placement field. Absent means the
// file sets no default. A present value must be hub or runtime. Objects carry
// the same choice as the annotation resource.oam.dev/placement; this field is
// only the cue default.
func readPlacementDefault(fsys fs.FS, p string) (string, error) {
	const fieldName = "placement"
	field, exists, err := lookupCUEField(fsys, p, fieldName)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", nil
	}
	s, err := field.String()
	if err != nil {
		return "", fmt.Errorf("field %s in %s is not a string", fieldName, p)
	}
	if s == "" {
		return "", nil
	}
	if _, err := placement.Normalize(s); err != nil {
		return "", fmt.Errorf("%s: %w", p, err)
	}
	return s, nil
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
