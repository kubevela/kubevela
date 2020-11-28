package builder

import (
	"testing"

	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/parser"
)

func TestBuild(t *testing.T) {
	_, _, err := Build("default", parser.TestExceptApp)
	if err != nil {
		t.Error(err)
	}
}
