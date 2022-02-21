package util

import (
	"testing"

	"github.com/oam-dev/kubevela/version"
)

func TestGenerateLeaderElectionID(t *testing.T) {
	version.VelaVersion = "v10.13.0"
	if id := GenerateLeaderElectionID("kubevela", true); id != "kubevela-v10-13-0" {
		t.Errorf("id is not as expected(%s != kubevela-v10-13-0)", id)
		return
	}
	if id := GenerateLeaderElectionID("kubevela", false); id != "kubevela" {
		t.Errorf("id is not as expected(%s != kubevela)", id)
		return
	}
}
