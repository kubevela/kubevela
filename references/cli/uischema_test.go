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
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

func TestLoadUISchemaFiles(t *testing.T) {
	files, err := loadUISchemaFiles("test-data/uischema")
	assert.Nil(t, err)
	assert.True(t, len(files) == 2)
}

func TestNewUISchemaCommand(t *testing.T) {
	cmd := NewUISchemaCommand(common2.Args{}, "", util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	initCommand(cmd)
	cmd.SetArgs([]string{"apply", "test-data/uischema"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute definition command: %v", err)
	}
}
