/*
Copyright 2021 The KubeVela Authors.

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

package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"

	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

func TestLoadUISchemaFiles(t *testing.T) {
	files, err := loadUISchemaFiles("test-data/uischema")
	assert.Nil(t, err)
	assert.True(t, len(files) == 2)
}

var _ = Describe("Test ui schema cli", func() {
	It("Test apply", func() {
		arg := common2.Args{}
		arg.SetClient(k8sClient)
		buffer := bytes.NewBuffer(nil)
		cmd := NewUISchemaCommand(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"apply", "test-data/uischema"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(strings.Contains(buffer.String(), "failure")).Should(BeFalse())
	})
})
