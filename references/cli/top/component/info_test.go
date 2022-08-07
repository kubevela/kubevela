package component

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestInfo(t *testing.T) {
	testEnv := &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute * 3,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       pointer.BoolPtr(false),
	}
	cfg, err := testEnv.Start()
	assert.NoError(t, err)
	info := NewInfo()
	info.Init(cfg)
	assert.Equal(t, info.GetColumnCount(), 2)
	assert.Equal(t, info.GetRowCount(), 6)
	assert.Equal(t, info.GetCell(0, 0).Text, "Context:")
}
