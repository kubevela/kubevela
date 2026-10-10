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
	cleaned := false
	t.Cleanup(func() {
		if !cleaned {
			_ = cleanup()
		}
	})
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
	cleaned = true
	if got := os.Getenv(system.VelaHomeEnv); got != prior {
		t.Fatalf("restored VELA_HOME = %q, want %q", got, prior)
	}
	if _, err := os.Stat(home); err == nil {
		t.Fatalf("worker home %s still exists after cleanup", home)
	} else if !os.IsNotExist(err) {
		t.Fatalf("checking worker home %s: %v", home, err)
	}
}
