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

package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/validation"

	pkgmodule "github.com/oam-dev/kubevela/pkg/module"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	moduleInitPathFlag = "path"

	// moduleNamePlaceholder is substituted with the module name in every template.
	moduleNamePlaceholder = "__MODULE__"
)

type moduleInitOptions struct {
	name string
	path string
}

// scaffoldFile is one file written into the module directory, kept in order so
// the printed tree is deterministic.
type scaffoldFile struct {
	rel     string
	content string
}

// NewModuleInitCommand returns the vela module init command. It scaffolds a
// valid, ready-to-edit module directory: the module identity, the v1 API
// line, a definition, and empty auxiliary/ directories documented by a
// README, then validates the result against the module parser so the
// skeleton always parses.
func NewModuleInitCommand(_ common.Args, _ cmdutil.IOStreams) *cobra.Command {
	o := &moduleInitOptions{}
	cmd := &cobra.Command{
		Use:   "init <name>",
		Short: "Scaffold a new module.",
		Long:  "Scaffold a valid, ready-to-edit module directory with placeholder content and TODO markers, so you start from a working skeleton instead of assembling the layout by hand.",
		Example: `  Scaffold a module named s3 in the current directory:
	vela module init s3

  Scaffold under a chosen directory:
	vela module init s3 --path ./modules`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.name = args[0]
			return o.run(cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&o.path, moduleInitPathFlag, ".", "Directory to create the <name>/ module directory under.")
	return cmd
}

// run validates the name, guards an existing non-empty directory, writes the
// scaffold, and validates it against the module parser.
func (o *moduleInitOptions) run(out io.Writer) error {
	if errs := validation.IsDNS1123Label(o.name); len(errs) > 0 {
		return fmt.Errorf("invalid module name %q: %s", o.name, errs[0])
	}
	if o.path == "" {
		o.path = "."
	}

	target := filepath.Join(o.path, o.name)
	nonEmpty, err := dirExistsNonEmpty(target)
	if err != nil {
		return err
	}
	if nonEmpty {
		return fmt.Errorf("target directory %q already exists and is not empty; choose a different --path or clear the directory", target)
	}

	files := scaffoldFiles(o.name)
	for _, f := range files {
		full := filepath.Join(target, f.rel)
		if mkErr := os.MkdirAll(filepath.Dir(full), 0o755); mkErr != nil {
			return fmt.Errorf("create directory for %s: %w", f.rel, mkErr)
		}
		if wErr := os.WriteFile(full, []byte(f.content), 0o644); wErr != nil {
			return fmt.Errorf("write %s: %w", f.rel, wErr)
		}
	}

	auxDirs := moduleAuxiliaryDirs()
	for _, d := range auxDirs {
		if mkErr := os.MkdirAll(filepath.Join(target, d), 0o755); mkErr != nil {
			return fmt.Errorf("create directory %s: %w", d, mkErr)
		}
	}

	// The shipped scaffold must always be a valid skeleton. A parse failure here
	// is a bug in these templates, not the author's input, so fail loudly.
	if _, err := pkgmodule.ParseModuleDir(target); err != nil {
		return fmt.Errorf("scaffolded module failed validation (this is a bug in the init templates): %w", err)
	}

	printGuidance(out, o.name, target, files, auxDirs)
	return nil
}

// dirExistsNonEmpty reports whether path exists and is a non-empty directory. A
// missing path is not an error (returns false). An existing non-directory path
// counts as non-empty, so init refuses to write over it.
func dirExistsNonEmpty(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !info.IsDir() {
		return true, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) > 0, nil
}

// scaffoldFiles returns the module skeleton for the given name, in write order.
func scaffoldFiles(name string) []scaffoldFile {
	sub := func(s string) string { return strings.ReplaceAll(s, moduleNamePlaceholder, name) }
	return []scaffoldFile{
		{rel: "_module.cue", content: sub(moduleCUETemplate)},
		{rel: "README.md", content: sub(readmeTemplate)},
		{rel: filepath.Join("v1", "_version.cue"), content: versionCUETemplate},
		{rel: filepath.Join("v1", "definitions", "example.cue"), content: sub(definitionCUETemplate)},
	}
}

// moduleAuxiliaryDirs are the empty auxiliary directories init creates
// alongside the scaffold files.
func moduleAuxiliaryDirs() []string {
	return []string{"auxiliary", filepath.Join("v1", "auxiliary")}
}

// printGuidance prints the created tree and the next step for the author.
func printGuidance(out io.Writer, name, target string, files []scaffoldFile, auxDirs []string) {
	fmt.Fprintf(out, "Scaffolded module %q at %s:\n", name, target)
	for _, f := range files {
		fmt.Fprintf(out, "  %s\n", filepath.Join(name, f.rel))
	}
	for _, d := range auxDirs {
		fmt.Fprintf(out, "  %s/ (empty, see README.md)\n", filepath.Join(name, d))
	}
	fmt.Fprintf(out, "\nNext steps:\n")
	fmt.Fprintf(out, "  1. Edit the TODO markers in _module.cue (version, owners), and rename v1/definitions/example.cue to your capability.\n")
	fmt.Fprintf(out, "  2. If your module needs infrastructure, add it under auxiliary/ and v1/auxiliary/ (see README.md).\n")
	fmt.Fprintf(out, "  3. Publish it, then install with: vela module deploy %s\n", name)
}

const moduleCUETemplate = `module:      "__MODULE__"
version:     "0.1.0"                                  // TODO: your module's semver (major.minor.patch)
description: "TODO: one-line description of __MODULE__"
owners: ["TODO-your-team"]                            // TODO: the team(s) that own this module
`

const versionCUETemplate = `apiVersion: "v1"
enabled:    true
`

const definitionCUETemplate = `// TODO: rename this file and the "example" key below to the capability this
// module offers (for example "bucket"). The key becomes the definition's
// metadata.name, and the module installs it as <module>-<apiVersion>-<capability>.
// See README.md for what a ComponentDefinition needs.
"example": {
	type: "component"
}

template: {
	// TODO: define what this capability creates.
	output: {}
}
`

const readmeTemplate = `# __MODULE__

This module was scaffolded by "vela module init". It follows the layout the
module parser expects:

  __MODULE__/
    _module.cue              module identity: name, version, description, owners
    README.md                this file
    auxiliary/                module-wide auxiliary objects (currently empty)
    v1/
      _version.cue           this API line's identity: apiVersion, enabled
      auxiliary/              v1-only auxiliary objects (currently empty)
      definitions/
        example.cue          rename to your capability, e.g. bucket.cue

## What goes in auxiliary/

auxiliary/ (module-wide) and v1/auxiliary/ (this line only) hold whatever
Kubernetes objects your module needs to install alongside its capability
definitions, before consumers can use type: __MODULE__-v1-<capability>.
Neither is required: leave a folder empty if your module has no infrastructure 
to install.

Where to put things:

- auxiliary/ - objects shared by every API line.
- v1/auxiliary/ - objects specific to this line only.

File format:

- A file may be .yaml, .yml, or .cue.
- A .yaml file is a plain Kubernetes object (apiVersion, kind, metadata, ...),
  the same shape you would kubectl apply. One file can hold several objects
  separated by "---", the same as a normal manifest.
- A .cue file is a single plain object at its root.
- Every file in a scope is installed.

## What goes in v1/definitions/

Each file here is a capability this API line offers to consumers, as a
KubeVela ComponentDefinition. The definition's name comes from its own
content, not the filename: the module installs it as
__MODULE__-v1-<capability>, where <capability> is the top-level key inside
the file. Add one file per capability; a line with no files here is not valid,
every line must offer at least one capability.

A ComponentDefinition CUE file needs two things:

- A top-level key (its name) whose value sets type: "component".
- A template: {...} block with at least one field, typically an output:
  {...} describing what this capability creates and a parameter: {...}
  block for what a consumer of it sets.

The scaffolded example.cue has the smallest shape that satisfies both; fill
in the output and parameter to match what your capability actually
provisions.

## Next steps

1. Fill in the TODOs in _module.cue (version, description, owners).
2. Add your capability definition(s) under v1/definitions/ (rename
   example.cue to the capability it defines).
3. If your module needs infrastructure, add the objects under auxiliary/
   and v1/auxiliary/ as described above.
4. Publish it, then install with: vela module deploy __MODULE__
`
