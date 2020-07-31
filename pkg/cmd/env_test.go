package cmd

import (
	"bytes"
	"context"
	"os"
	"testing"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/stretchr/testify/assert"
)

func TestENV(t *testing.T) {
	ctx := context.Background()

	// Create Default Env
	err := InitDefaultEnv()
	assert.NoError(t, err)

	// check and compare create default env success
	curEnvName, err := GetCurrentEnvName()
	assert.NoError(t, err)
	assert.Equal(t, "default", curEnvName)
	gotEnv, err := GetEnv()
	assert.NoError(t, err)
	assert.Equal(t, &EnvMeta{
		Namespace: "default",
	}, gotEnv)

	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	exp := &EnvMeta{
		Namespace: "test1",
	}

	// Create env1
	err = CreateOrUpdateEnv(ctx, exp, []string{"env1"}, ioStream)
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
	assert.Equal(t, &EnvMeta{
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
