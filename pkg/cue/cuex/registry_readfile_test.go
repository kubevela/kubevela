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

package cuex_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	cuexregistry "github.com/oam-dev/kubevela/pkg/cue/cuex/providers/registry"
	di "github.com/oam-dev/kubevela/pkg/registry"
	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
)

// fileStub answers from a fixed map; anything else is absent.
type fileStub struct {
	files map[string]string
	fail  error
}

func (f fileStub) ReadFile(_ context.Context, _, path, _ string) (string, error) {
	if f.fail != nil {
		return "", f.fail
	}
	if c, ok := f.files[path]; ok {
		return c, nil
	}
	return "", fmt.Errorf("reading %q: %w", path, velaerrors.ErrFileNotFound)
}

func stubFiles(t *testing.T, s fileStub) {
	t.Helper()
	snap := di.Snapshot()
	t.Cleanup(func() { di.Restore(snap) })
	di.RegisterAs[cuexregistry.FileReader](s)
}

// compileGitFile evaluates the git-file SourceDefinition template that ships in
// docs/examples/source-library, with the given parameter block substituted in.
//
// The shipped template is used rather than a copy: the point is to catch it
// breaking, and a copy would keep passing after the real one stopped compiling.
func compileGitFile(t *testing.T, params string) (map[string]interface{}, error) {
	t.Helper()

	tmpl, err := os.ReadFile("../../../vela-templates/definitions/internal/source/git-file.cue")
	require.NoError(t, err)

	// The template declares `parameter` as a schema; the caller supplies values.
	src := unwrapDefinitionTemplate(t, string(tmpl)) + "\n\nparameter: " + params + "\n"

	val, err := velacuex.WorkloadCompiler.Get().CompileString(context.Background(), src)
	if err != nil {
		return nil, err
	}
	if err := val.Err(); err != nil {
		return nil, err
	}

	var out struct {
		Output map[string]interface{} `json:"output"`
	}
	if err := val.Decode(&out); err != nil {
		return nil, err
	}
	return out.Output, nil
}

// TestGitFileTemplateHandlesAbsence exercises the shipped source template over
// the soft-fail path, which is the behaviour the provider change exists for.
func TestGitFileTemplateHandlesAbsence(t *testing.T) {
	const metadata = "version: 1.9.0\nname: velaux\n"

	t.Run("a file that is there parses", func(t *testing.T) {
		stubFiles(t, fileStub{files: map[string]string{"velaux/metadata.yaml": metadata}})

		out, err := compileGitFile(t, `{registry: "catalog", path: "velaux/metadata.yaml"}`)
		require.NoError(t, err)
		assert.Equal(t, true, out["found"])
		content, ok := out["content"].(map[string]interface{})
		require.True(t, ok, "a .yaml file must come back parsed, got %T", out["content"])
		assert.Equal(t, "1.9.0", content["version"])
	})

	t.Run("an optional file that is absent resolves", func(t *testing.T) {
		stubFiles(t, fileStub{files: map[string]string{}})

		out, err := compileGitFile(t, `{registry: "catalog", path: "velaux/override.yaml", required: false}`)
		require.NoError(t, err, "required: false must not fail on a missing file")
		assert.Equal(t, false, out["found"])
		assert.Nil(t, out["content"], "no content may be invented for a missing file")
	})

	t.Run("a required file that is absent fails", func(t *testing.T) {
		stubFiles(t, fileStub{files: map[string]string{}})

		_, err := compileGitFile(t, `{registry: "catalog", path: "velaux/override.yaml"}`)
		require.Error(t, err, "required is the default, so a missing file must fail resolution")
		// The reason has to reach the user. A bare "conflicting values false and
		// true" would leave them guessing which of several files was missing.
		msg := err.Error()
		assert.Contains(t, msg, "velaux/override.yaml", "the error must name the file")
		assert.Contains(t, msg, "catalog", "the error must name the registry")
		assert.Contains(t, msg, "required: false", "the error should say how to make it optional")
	})

	t.Run("a failing registry fails whether or not the file is required", func(t *testing.T) {
		stubFiles(t, fileStub{fail: errors.New("dial tcp: connection refused")})

		_, err := compileGitFile(t, `{registry: "catalog", path: "velaux/override.yaml", required: false}`)
		require.Error(t, err, "required: false softens absence, not failure")
		assert.True(t, strings.Contains(err.Error(), "connection refused"),
			"the cause should survive to the user, got: %v", err)
	})
}

// TestMustReadFileAsserts covers the strict variant of the provider contract,
// for a template that wants a missing file to fail rather than to branch.
func TestMustReadFileAsserts(t *testing.T) {
	compile := func(t *testing.T, path string) (map[string]interface{}, error) {
		t.Helper()
		src := `import "vela/registry"

out: registry.#MustReadFile & {
	$params: {registry: "catalog", path: "` + path + `"}
}
`
		val, err := velacuex.WorkloadCompiler.Get().CompileString(context.Background(), src)
		if err != nil {
			return nil, err
		}
		if err := val.Err(); err != nil {
			return nil, err
		}
		var out struct {
			Out struct {
				Returns map[string]interface{} `json:"$returns"`
			} `json:"out"`
		}
		if err := val.Decode(&out); err != nil {
			return nil, err
		}
		return out.Out.Returns, nil
	}

	t.Run("returns content when the file is there", func(t *testing.T) {
		stubFiles(t, fileStub{files: map[string]string{"a.txt": "hello"}})

		got, err := compile(t, "a.txt")
		require.NoError(t, err)
		assert.Equal(t, "hello", got["content"])
		assert.Equal(t, true, got["found"])
	})

	t.Run("fails when the file is not, naming it", func(t *testing.T) {
		stubFiles(t, fileStub{files: map[string]string{}})

		_, err := compile(t, "missing.txt")
		require.Error(t, err, "#MustReadFile exists to fail here; #ReadFile is the one that branches")
		assert.Contains(t, err.Error(), "missing.txt")
	})
}

// unwrapDefinitionTemplate returns the `template:` body of a `vela def` source
// file, with its imports hoisted back to the top.
//
// The shipped definition wraps the template in the metadata block vela def
// renders from; evaluating the template alone needs it unwrapped.
func unwrapDefinitionTemplate(t *testing.T, src string) string {
	t.Helper()

	lines := strings.Split(src, "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "template: {") {
			start = i
			break
		}
	}
	require.GreaterOrEqual(t, start, 0, "no template block in the definition")

	var imports []string
	for _, l := range lines[:start] {
		if strings.HasPrefix(l, "import ") || strings.HasPrefix(l, "\t\"") ||
			l == ")" || strings.HasPrefix(l, "\t") && strings.Contains(l, "\"") {
			imports = append(imports, l)
		}
	}

	var body []string
	for _, l := range lines[start+1:] {
		if l == "}" {
			break
		}
		body = append(body, strings.TrimPrefix(l, "\t"))
	}
	return strings.Join(append(imports, body...), "\n")
}
