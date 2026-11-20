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

package cuetest

import (
	"testing"

	"github.com/kubevela/pkg/cue/cuex"
	"github.com/stretchr/testify/require"
)

// A render swaps compilers that load external packages from a cluster when
// first read, so it refuses, saying how to fix it, unless the caller turned
// that off: the switch is process-wide, so it is the entry point's to set.
func TestRendersRefuseWithExternalPackagesOn(t *testing.T) {
	cuex.EnableExternalPackageForDefaultCompiler = true
	defer func() { cuex.EnableExternalPackageForDefaultCompiler = false }()

	_, err := Render(Subject{Kind: KindComponent, Name: "c", Template: `output: {apiVersion: "v1", kind: "ConfigMap"}
parameter: {}`}, Input{})
	require.ErrorContains(t, err, "set cuex.EnableExternalPackageForDefaultCompiler = false")

	_, err = RenderApplicationPolicy(Subject{Kind: KindApplicationPolicy, Name: "p"}, Input{})
	require.ErrorContains(t, err, "set cuex.EnableExternalPackageForDefaultCompiler = false")
}
