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

package schema_test

import (
	"os"
	"testing"

	"github.com/kubevela/pkg/cue/cuex"
)

// TestMain keeps CueX offline: rendering shipped definitions must not read
// packages from whatever cluster the kubeconfig points at.
func TestMain(m *testing.M) {
	cuex.EnableExternalPackageForDefaultCompiler = false
	os.Exit(m.Run())
}
