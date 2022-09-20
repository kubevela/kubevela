/*
Copyright 2022 The KubeVela Authors.

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
	assert.NotEmpty(t, info.ClusterNum())
}

func TestInfo_VelaCLIVersion(t *testing.T) {
	assert.NotEmpty(t, VelaCLIVersion())
}

func TestInfo_VelaCoreVersion(t *testing.T) {
	assert.NotEmpty(t, VelaCoreVersion())
}

func TestInfo_GOLangVersion(t *testing.T) {
	assert.Contains(t, GOLangVersion(), "go")
}

var _ = Describe("test info", func() {
	It("running app num", func() {
		Expect(ApplicationRunningNum(cfg)).To(Equal("1/1"))
	})

	It("test kubernetes version", func() {
		Expect(K8SVersion(cfg) != "UNKNOWN").To(BeTrue())
	})
})
