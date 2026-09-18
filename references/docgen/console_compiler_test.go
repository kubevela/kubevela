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

package docgen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
)

// `vela def show` is the discovery step the docs send people to first, and it
// went through the upstream cuex compiler, which knows nothing of KubeVela's own
// provider packages. Any definition importing one failed outright with
// "parameter not exist" - two of the nine shipped sources, git-file on
// vela/registry and vela-config on vela/velaconfig, plus anything a platform
// team authors against them.
//
// MarkdownReference already carried a Compiler field for exactly this. The
// console path is the one people actually run.
func TestConsoleShowsDefinitionsImportingVelaProviders(t *testing.T) {
	const tmpl = `
import "vela/registry"

_file: registry.#ReadFile & {
	$params: {
		registry: parameter.registry
		path:     parameter.path
	}
}

parameter: {
	// +usage=Name of a registry configured in this cluster
	registry: string
	// +usage=Path of the file within that registry
	path: string
}

output: content: _file.$returns.content
`
	cap := &types.Capability{Name: "git-file", CueTemplate: tmpl}

	t.Run("default compiler cannot resolve the import", func(t *testing.T) {
		ref := &ConsoleReference{}
		_, _, err := ref.GenerateCUETemplateProperties(cap)
		require.Error(t, err, "expected the upstream compiler to lack vela/registry")
	})

	t.Run("the vela providers compiler can", func(t *testing.T) {
		ref := &ConsoleReference{Compiler: providers.DefaultCompiler.Get()}
		_, console, err := ref.GenerateCUETemplateProperties(cap)
		require.NoError(t, err)
		// The console path renders into tablewriter objects rather than a string,
		// so this asserts the extraction succeeded and produced a table. That the
		// table names the parameters is covered end to end by `vela def show`.
		require.NotEmpty(t, console, "expected at least one parameter table")
	})
}
