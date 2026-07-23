/*
Copyright 2024 The KubeVela Authors.

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

package providers

import (
	"context"
	"fmt"
	"testing"

	"github.com/kubevela/pkg/cue/cuex"
	"github.com/stretchr/testify/assert"
)

// TestDefaultCompilerResolvesAddonPackage guards the fix for the addon
// ComponentDefinition schema-generation / definition-validation path. That path
// compiles definition templates through DefaultCompiler with provider function
// resolution disabled, so the imported "vela/addon" package must be known to the
// compiler even though addon.#Render is never executed.
func TestDefaultCompilerResolvesAddonPackage(t *testing.T) {
	ctx := context.Background()
	compiler := DefaultCompiler.Get()

	template := `import "vela/addon"
_x: addon.#Render & {$params: {addon: "x"}}
`
	_, err := compiler.CompileStringWithOptions(ctx, template, cuex.DisableResolveProviderFunctions{})
	assert.NoError(t, err, "compiling a template importing vela/addon should not fail with functions disabled")
	if err != nil {
		assert.NotContains(t, err.Error(), `builtin package "vela/addon" undefined`)
	}
}

// TestDefaultCompilerResolvesInternalPackages is a regression guard: registering
// the addon package must not drop any package that internal definitions
// (component/trait/policy/workflow-step) import for schema generation.
func TestDefaultCompilerResolvesInternalPackages(t *testing.T) {
	ctx := context.Background()
	compiler := DefaultCompiler.Get()

	packages := []string{
		"addon",
		"op",
		"builtin",
		"multicluster",
		"query",
		"util",
		"email",
		"metrics",
		"http",
		"kube",
		"config",
		"helm",
	}
	for _, pkg := range packages {
		t.Run(pkg, func(t *testing.T) {
			template := fmt.Sprintf("import \"vela/%s\"\n_ref: %s\n", pkg, pkg)
			_, err := compiler.CompileStringWithOptions(ctx, template, cuex.DisableResolveProviderFunctions{})
			if err != nil {
				assert.NotContains(t, err.Error(), fmt.Sprintf(`builtin package "vela/%s" undefined`, pkg),
					"package vela/%s must be resolvable by DefaultCompiler", pkg)
			}
		})
	}
}
