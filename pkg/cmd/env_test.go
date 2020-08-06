package cmd

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"

	"github.com/cloud-native-application/rudrx/api/types"

	"github.com/cloud-native-application/rudrx/pkg/utils/system"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/stretchr/testify/assert"
)

func TestENV(t *testing.T) {
	ctx := context.Background()

	// Create Default Env
	err := system.InitDefaultEnv()
	assert.NoError(t, err)

	// check and compare create default env success
	curEnvName, err := GetCurrentEnvName()
	assert.NoError(t, err)
	assert.Equal(t, "default", curEnvName)
	gotEnv, err := GetEnv()
	assert.NoError(t, err)
	assert.Equal(t, &types.EnvMeta{
		Namespace: "default",
	}, gotEnv)

	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	exp := &types.EnvMeta{
		Namespace: "test1",
	}
	client := test.NewMockClient()
	// Create env1
	err = CreateOrUpdateEnv(ctx, client, exp, []string{"env1"}, ioStream)
	assert.NoError(t, err)

	// check and compare create env success
	curEnvName, err = GetCurrentEnvName()
	assert.NoError(t, err)
	assert.Equal(t, "env1", curEnvName)
	gotEnv, err = GetEnv()
	assert.NoError(t, err)
	assert.Equal(t, exp, gotEnv)

	// List all env
	var b bytes.Buffer
	ioStream.Out = &b
	err = ListEnvs(ctx, []string{}, ioStream)
	assert.NoError(t, err)
	assert.Equal(t, `NAME   	NAMESPACE
default	default  
env1   	test1    `, b.String())
	b.Reset()
	err = ListEnvs(ctx, []string{"env1"}, ioStream)
	assert.NoError(t, err)
	assert.Equal(t, `NAME	NAMESPACE
env1	test1    `, b.String())
	ioStream.Out = os.Stdout

	// can not delete current env
	err = DeleteEnv(ctx, []string{"env1"}, ioStream)
	assert.Error(t, err)

	// switch to default env
	err = SwitchEnv(ctx, []string{"default"}, ioStream)
	assert.NoError(t, err)

	// check switch success
	gotEnv, err = GetEnv()
	assert.NoError(t, err)
	assert.Equal(t, &types.EnvMeta{
		Namespace: "default",
	}, gotEnv)

	// delete env
	err = DeleteEnv(ctx, []string{"env1"}, ioStream)
	assert.NoError(t, err)

	// can not switch to non-exist env
	err = SwitchEnv(ctx, []string{"env1"}, ioStream)
	assert.Error(t, err)

	// switch success
	err = SwitchEnv(ctx, []string{"default"}, ioStream)
	assert.NoError(t, err)
}
