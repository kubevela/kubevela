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
	"fmt"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
)

func TestUp(t *testing.T) {
	ioStream := util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	namespace := "up-ns"
	o := common.AppfileOptions{
		IO:        ioStream,
		Namespace: namespace,
	}
	app := &v1beta1.Application{}
	app.Name = "app-up"
	msg := o.Info(app)
	assert.Contains(t, msg, "App has been deployed")
	assert.Contains(t, msg, fmt.Sprintf("App status: vela status %s", app.Name))
}

func TestNewUpCommandPersistentPreRunE(t *testing.T) {
	io := util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	fakeC := common2.Args{}
	cmd := NewUpCommand(fakeC, "", io)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}
