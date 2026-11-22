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

package template

import (
	"context"
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
)

// standardMethods is the method set RFC 9110 names, plus PATCH (RFC 5789).
var standardMethods = []string{"GET", "HEAD", "POST", "PUT", "DELETE", "CONNECT", "OPTIONS", "TRACE", "PATCH"}

// The request step's parameter schema accepts every standard method and rejects
// anything else. Sending one still has to unify with vela/http, which is the
// provider's own schema.
func TestRequestStepParameterMethods(t *testing.T) {
	prev := cuex.EnableExternalPackageForDefaultCompiler
	cuex.EnableExternalPackageForDefaultCompiler = false
	t.Cleanup(func() { cuex.EnableExternalPackageForDefaultCompiler = prev })

	src := readWorkflowStepCue(t, "request.cue")
	def := definition.Definition{Unstructured: unstructured.Unstructured{}}
	require.NoError(t, def.FromCUEString(src, nil))
	tmpl, found, err := unstructured.NestedString(def.Object, definition.DefinitionTemplateKeys...)
	require.NoError(t, err)
	require.True(t, found)
	compiler := cuex.NewCompilerWithInternalPackages(providers.WorkflowPackages()...)

	for _, method := range standardMethods {
		t.Run(method, func(t *testing.T) {
			got, err := requestParameterMethod(t, compiler, tmpl, method)
			require.NoError(t, err)
			require.Equal(t, method, got)
		})
	}
	t.Run("unknown", func(t *testing.T) {
		_, err := requestParameterMethod(t, compiler, tmpl, "FOO")
		require.Error(t, err)
	})
}

func requestParameterMethod(t *testing.T, compiler *cuex.Compiler, tmpl, method string) (string, error) {
	t.Helper()
	v, err := compiler.CompileStringWithOptions(context.Background(), tmpl+"\n"+fmt.Sprintf(`
parameter: {
	url: "https://api.example.com/thing/1"
	method: %q
}
`, method), cuex.DisableResolveProviderFunctions{})
	require.NoError(t, err)
	return v.LookupPath(cue.ParsePath("parameter.method")).String()
}
