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

package common_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/encoding/openapi"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/schema"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
)

// compileTemplate compiles a definition file's template the way the
// controller does: its imports and template body with KubeVela's base template.
func compileTemplate(t *testing.T, path string) (cue.Value, bool) {
	t.Helper()
	src, err := os.ReadFile(path)
	require.NoError(t, err)
	f, err := parser.ParseFile(path, src, parser.ParseComments)
	require.NoError(t, err)
	var body []byte
	for _, d := range f.Decls {
		switch x := d.(type) {
		case *ast.ImportDecl:
			b, err := format.Node(x)
			require.NoError(t, err)
			body = append(body, append(b, '\n')...)
		case *ast.Field:
			if name, _, _ := ast.LabelName(x.Label); name == "template" {
				b, err := format.Node(x.Value)
				require.NoError(t, err)
				body = append(body, b...)
			}
		}
	}
	if !strings.Contains(string(body), "parameter") {
		return cue.Value{}, false
	}
	val, err := providers.DefaultCompiler.Get().CompileStringWithOptions(context.Background(),
		string(body)+"\n"+schema.BaseTemplate, cuex.DisableResolveProviderFunctions{})
	if err != nil || val.Err() != nil {
		return cue.Value{}, false
	}
	return val, true
}

// The fallback must never change a schema. For every built-in definition that
// encodes today, a forced fallback either produces the same document or
// declines with an error, in which case GenOpenAPI keeps the encoder's own
// result. A built-in that fails today on the bounded-default bug must encode.
func TestBuiltinDefinitionSchemas(t *testing.T) {
	cuex.EnableExternalPackageForDefaultCompiler = false
	files, err := filepath.Glob("../../../vela-templates/definitions/internal/*/*.cue")
	require.NoError(t, err)
	require.NotEmpty(t, files)

	var compared, declined, fixed, skipped int
	for _, path := range files {
		if strings.HasSuffix(path, "_test.cue") {
			continue
		}
		name := filepath.Base(filepath.Dir(path)) + "/" + filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			template, ok := compileTemplate(t, path)
			if !ok {
				skipped++
				t.Skip("template does not compile standalone")
			}
			params, err := common.RefineParameterValue(template)
			require.NoError(t, err)
			want, encErr := openapi.Gen(params, common.ExpandRefs)
			if encErr != nil {
				require.Contains(t, encErr.Error(), common.BoundedDefaultError, "only the bounded-default bug is expected")
				_, err := common.GenOpenAPI(template)
				require.NoError(t, err, "GenOpenAPI now encodes it")
				fixed++
				return
			}
			got, err := common.EncodeWithoutNumberDefaults(template, params, common.RefineParameterValue)
			if err != nil {
				declined++
				t.Logf("fallback declines: %v", err)
				return
			}
			var w, g any
			require.NoError(t, json.Unmarshal(want, &w))
			require.NoError(t, json.Unmarshal(got, &g))
			require.Equal(t, w, g, "the fallback changes the schema of %s", name)
			compared++
		})
	}
	t.Logf("%d schemas identical through the fallback, %d declined, %d fixed, %d skipped", compared, declined, fixed, skipped)
	require.GreaterOrEqual(t, compared, 80, "most built-ins are compared")
}
