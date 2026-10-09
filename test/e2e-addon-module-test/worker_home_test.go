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

package addonmoduletest

import (
	"os"
	"strings"
	"testing"

	"github.com/oam-dev/kubevela/pkg/utils/system"
)

func TestAddonModuleWorkerHomeIsPrivateAndRestored(t *testing.T) {
	const prior = "existing-addon-module-home"
	t.Setenv(system.VelaHomeEnv, prior)
	cleanup, err := prepareAddonModuleWorkerHome()
	if err != nil {
		t.Fatal(err)
	}
	home := os.Getenv(system.VelaHomeEnv)
	if home == prior || !strings.Contains(home, "kubevela-addon-module-e2e-") {
		t.Fatalf("worker home %q is not private", home)
	}
	if _, err := os.Stat(home); err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(system.VelaHomeEnv); got != prior {
		t.Fatalf("restored VELA_HOME = %q, want %q", got, prior)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("worker home still exists: %v", err)
	}
}
