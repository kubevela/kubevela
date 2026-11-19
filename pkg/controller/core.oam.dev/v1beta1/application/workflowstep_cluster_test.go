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

package application

import (
	"testing"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/appfile"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
)

// An empty cluster name means the local cluster: multicluster.IsLocal treats ""
// and "local" as the same place, and routing relies on that.
//
// Nothing that compares or hashes the name does. ClusterObjectReference.Equal
// compares the string, and the source cache hashes it, so a surface that leaves
// cluster empty gets a second cache entry for a cluster that already has one.
// The policy render sets it explicitly for exactly this reason, and its comment
// says so; the workflow-step render is the surface that was missed.
func TestWorkflowStepContextCarriesTheLocalCluster(t *testing.T) {
	ctxData := appfile.GenerateContextDataFromAppFile(
		&appfile.Appfile{Name: "app", Namespace: "default"}, "notify")
	pCtx := velaprocess.NewContext(ctxData)

	require.Equal(t, pkgmulticluster.Local, pCtx.GetData(velaprocess.ContextCluster),
		`a step keying a source on "" while the component beside it keys on "local" `+
			`gives one cluster two cache entries`)
}
