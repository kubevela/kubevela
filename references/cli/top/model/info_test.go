package model

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
)

func TestInfo_CurrentContext(t *testing.T) {
	info := NewInfo()
	assert.NotEqual(t, info.CurrentContext(), "UNKNOWN")
}

func TestInfo_ClusterNum(t *testing.T) {
	info := NewInfo()
	assert.NotEqual(t, info.ClusterNum(), "0")
}

func TestInfo_VelaCLIVersion(t *testing.T) {
	assert.Equal(t, VelaCLIVersion() == "UNKNOWN", true)
}

func TestInfo_VelaCoreVersion(t *testing.T) {
	assert.Equal(t, VelaCoreVersion() != "UNKNOWN", true)
}

func TestInfo_GOLangVersion(t *testing.T) {
	assert.Contains(t, GOLangVersion(), "go")
}

var _ = Describe("test info", func() {
	It("test kubernetes version", func() {
		Expect(K8SVersion(cfg) != "UNKNOWN").To(BeTrue())
	})
})
