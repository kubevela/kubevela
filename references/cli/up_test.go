package cli

import (
	"fmt"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
)

func TestUp(t *testing.T) {
	ioStream := util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	env := types.EnvMeta{
		Name:      "up",
		Namespace: "env-up",
		Issuer:    "up",
	}
	o := common.AppfileOptions{
		IO:  ioStream,
		Env: &env,
	}
	app := &v1alpha2.Application{}
	app.Name = "app-up"
	msg := o.Info(app)
	assert.Contains(t, msg, "App has been deployed")
	assert.Contains(t, msg, fmt.Sprintf("App status: vela status %s", app.Name))
}

func TestNewUpCommandPersistentPreRunE(t *testing.T) {
	io := util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	fakeC := common2.Args{}
	cmd := NewUpCommand(fakeC, io)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}
