package model

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
)

func TestInfo_Cluster(t *testing.T) {
	info := NewInfo()
	assert.Equal(t, info.Cluster(), "local")
}

func TestInfo_VelaCLIVersion(t *testing.T) {
	info := NewInfo()
	assert.Equal(t, info.VelaCLIVersion() == "UNKNOWN", true)
}

func TestInfo_VelaCoreVersion(t *testing.T) {
	info := NewInfo()
	assert.Equal(t, info.VelaCoreVersion() != "UNKNOWN", true)
}

func TestInfo_GOLangVersion(t *testing.T) {
	info := NewInfo()
	assert.Contains(t, info.GOLangVersion(), "go")
}

var _ = Describe("test info", func() {
	info := NewInfo()
	It("test kubernetes version", func() {
		Expect(info.K8SVersion(cfg) != "UNKNOWN").To(BeTrue())
	})
})
