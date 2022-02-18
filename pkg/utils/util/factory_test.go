package util

import (
	"testing"

	"github.com/oam-dev/kubevela/version"
)

func TestGenerateLeaderElectionID(t *testing.T) {
	version.VelaVersion = "v10.13.0"
	if id := GenerateLeaderElectionID("kubevela", true); id != "kubevela-v10z13z0" {
		t.Errorf("id is not as expected(%s != kubevela-v10z13z0)", id)
		return
	}
}
