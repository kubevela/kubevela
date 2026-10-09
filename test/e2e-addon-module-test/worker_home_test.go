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
