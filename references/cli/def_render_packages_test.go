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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	cuexv1alpha1 "github.com/kubevela/pkg/apis/cue/v1alpha1"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"

	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/kubeconfig"
)

// renderPackagesChildEnv carries the render flags into the re-executed test.
const renderPackagesChildEnv = "VELA_TEST_DEF_RENDER_PACKAGES"

// vela def render reads no CUE packages from a cluster unless asked to, so
// rendering is the same wherever the kubeconfig points. Each case runs in a
// fresh process: the compiler is a process-wide singleton that loads the
// cluster's packages once, on first use.
func TestDefinitionRenderReadsClusterPackagesOnlyWhenAsked(t *testing.T) {
	if flags, ok := os.LookupEnv(renderPackagesChildEnv); ok {
		renderCountingPackageLists(t, strings.Fields(flags))
		return
	}
	for flags, want := range map[string]string{"": "listed=0", "--" + FlagClusterPackages: "listed=1"} {
		t.Run("flags="+flags, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestDefinitionRenderReadsClusterPackagesOnlyWhenAsked$", "-test.v")
			cmd.Env = append(os.Environ(), renderPackagesChildEnv+"="+flags,
				"KUBECONFIG="+filepath.Join(t.TempDir(), "absent"))
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, string(out))
			require.Contains(t, string(out), want)
		})
	}
}

// renderCountingPackageLists renders a trait with the cluster's Package list
// served by a fake that counts the requests, and prints the count.
func renderCountingPackageLists(t *testing.T, flags []string) {
	defer kubeconfig.AssumeAvailable()()
	cuex.EnableExternalPackageForDefaultCompiler = true
	var listed atomic.Int32
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{cuexv1alpha1.PackageGroupVersionResource: "PackageList"})
	dyn.PrependReactor("list", cuexv1alpha1.PackageResource, func(clienttesting.Action) (bool, runtime.Object, error) {
		listed.Add(1)
		return false, nil, nil
	})
	singleton.DynamicClient.Set(dyn)

	cmd := NewDefinitionRenderCommand(common2.Args{})
	initCommand(cmd)
	cmd.SetArgs(append([]string{createLocalTraitAt("packages-probe", t.TempDir(), t)}, flags...))
	require.NoError(t, cmd.Execute())
	fmt.Printf("listed=%d\n", listed.Load())
}
