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

	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/pkg/utils/system"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

func TestConfigCommand(t *testing.T) {
	// Set VELA_HOME to local
	assert.NoError(t, os.Setenv(system.VelaHomeEnv, ".test_vela"))
	home, err := system.GetVelaHomeDir()
	assert.NoError(t, err)
	assert.Equal(t, true, strings.HasSuffix(home, ".test_vela"))
	defer os.RemoveAll(home)

	// Create Default Env
	err = system.InitDefaultEnv()
	assert.NoError(t, err)

	// vela config set test a=b
	io := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	err = setConfig([]string{"test", "a=b"}, io, nil)
	if err != nil {
		t.Fatal(err)
	}

	// vela config get test
	var b bytes.Buffer
	io.Out = &b
	err = getConfig([]string{"test"}, io, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "Data:\n  a: b\n", b.String())

	// vela config set test2 c=d
	io.Out = os.Stdout
	err = setConfig([]string{"test2", "c=d"}, io, nil)
	if err != nil {
		t.Fatal(err)
	}

	// vela config ls
	b = bytes.Buffer{}
	io.Out = &b
	err = ListConfigs(io, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "NAME \ntest \ntest2\n", b.String())

	// vela config del test
	io.Out = os.Stdout
	err = deleteConfig([]string{"test"}, io, nil)
	if err != nil {
		t.Fatal(err)
	}

	// vela config ls
	b = bytes.Buffer{}
	io.Out = &b
	err = ListConfigs(io, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "NAME \ntest2\n", b.String())
}
