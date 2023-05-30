/*
Copyright 2023 The KubeVela Authors.

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

package log_test

import (
	"bytes"
	"flag"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/references/cli/log"
)

func TestLogger(t *testing.T) {
	fset := flag.NewFlagSet("logs", flag.ContinueOnError)
	klog.InitFlags(fset)
	err := fset.Parse([]string{"-v=7"})
	require.NoError(t, err)

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	logger := log.NewLoggerWithWriter("test", outBuf, errBuf)
	forked := logger.WithName("logs").WithValues("key", "value", "nothing")
	forked.Info("message info", "key", "override")
	forked.Error(fmt.Errorf("unknown"), "unk", "extra", "extra val")

	errStr := errBuf.String()
	require.Contains(t, errStr, "key=\"value\"")
	require.Contains(t, errStr, "extra=\"extra val\"")
	require.Contains(t, errStr, "Unk")

	outStr := outBuf.String()
	require.Contains(t, outStr, "Message info")
	require.Contains(t, outStr, "key=\"override\"")
	require.Contains(t, outStr, "nothing=\"(MISSING)\"")
	require.Contains(t, outStr, "logs")
}
