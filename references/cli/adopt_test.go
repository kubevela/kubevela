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
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

func TestDefaultNamespace(t *testing.T) {
	testcase := []struct {
		namespace string
		args      []string
	}{
		{
			namespace: "kube-system",
			args:      []string{"deployment/kube-system/metrics-server"},
		},
		{
			namespace: "default",
			args:      []string{"deployment/metrics-server"},
		},
	}

	for _, c := range testcase {
		opt := &AdoptOptions{
			Type: adoptTypeNative,
			Mode: adoptModeReadOnly,
		}
		f := velacmd.NewDeferredFactory(config.GetConfig)
		ioStream := util.IOStreams{}
		cmd := NewAdoptCommand(f, ioStream)
		err := opt.Complete(f, cmd, c.args)
		if err != nil {
			t.Fatalf("failed to parse resourceRef: %v", err)
		}
		assert.Equal(t, opt.AppNamespace, c.namespace)
	}
}
