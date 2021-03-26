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
	"context"
	"os"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/env"
	"github.com/oam-dev/kubevela/pkg/utils/system"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

func TestENV(t *testing.T) {
	ctx := context.Background()

	assert.NoError(t, os.Setenv(system.VelaHomeEnv, ".test_vela"))
	home, err := system.GetVelaHomeDir()
	assert.NoError(t, err)
	assert.Equal(t, true, strings.HasSuffix(home, ".test_vela"))
	defer os.RemoveAll(home)
	// Create Default Env
	err = system.InitDefaultEnv()
	assert.NoError(t, err)

	// check and compare create default env success
	curEnvName, err := env.GetCurrentEnvName()
	assert.NoError(t, err)
	assert.Equal(t, "default", curEnvName)
	gotEnv, err := GetEnv(nil)
	assert.NoError(t, err)
	assert.Equal(t, &types.EnvMeta{
		Namespace: "default",
		Name:      "default",
	}, gotEnv)

	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	exp := &types.EnvMeta{
		Namespace: "test1",
		Name:      "env1",
	}
	client := test.NewMockClient()
	// Create env1
	err = CreateOrUpdateEnv(ctx, client, exp, []string{"env1"}, ioStream)
	assert.NoError(t, err)

	// check and compare create env success
	curEnvName, err = env.GetCurrentEnvName()
	assert.NoError(t, err)
	assert.Equal(t, "env1", curEnvName)
	gotEnv, err = GetEnv(nil)
	assert.NoError(t, err)
	assert.Equal(t, exp, gotEnv)

	// List all env
	var b bytes.Buffer
	ioStream.Out = &b
	err = ListEnvs([]string{}, ioStream)
	assert.NoError(t, err)
	assert.Equal(t, "NAME   \tCURRENT\tNAMESPACE\tEMAIL\tDOMAIN\ndefault\t       \tdefault  \t     \t      \nenv1   \t*      \ttest1    \t     \t      \n", b.String())
	b.Reset()
	err = ListEnvs([]string{"env1"}, ioStream)
	assert.NoError(t, err)
	assert.Equal(t, "NAME\tCURRENT\tNAMESPACE\tEMAIL\tDOMAIN\nenv1\t       \ttest1    \t     \t      \n", b.String())
	ioStream.Out = os.Stdout

	// can not delete current env
	err = DeleteEnv(ctx, []string{"env1"}, ioStream)
	assert.Error(t, err)

	// set as default env
	err = SetEnv([]string{"default"}, ioStream)
	assert.NoError(t, err)

	// check env set success
	gotEnv, err = GetEnv(nil)
	assert.NoError(t, err)
	assert.Equal(t, &types.EnvMeta{
		Namespace: "default",
		Name:      "default",
	}, gotEnv)

	// delete env
	err = DeleteEnv(ctx, []string{"env1"}, ioStream)
	assert.NoError(t, err)

	// can not set as a non-exist env
	err = SetEnv([]string{"env1"}, ioStream)
	assert.Error(t, err)

	// set success
	err = SetEnv([]string{"default"}, ioStream)
	assert.NoError(t, err)
}

func TestEnvInitCommandPersistentPreRunE(t *testing.T) {
	io := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	fakeC := common.Args{}
	cmd := NewEnvInitCommand(fakeC, io)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}
