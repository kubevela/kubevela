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

package main

import (
	"math/rand"
	"os"
	"time"

	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/pkg/stdlib"
	"github.com/oam-dev/kubevela/pkg/utils/system"
	"github.com/oam-dev/kubevela/references/a/preimport"
	"github.com/oam-dev/kubevela/references/cli"
)

func main() {
	preimport.ResumeLogging()
	rand.Seed(time.Now().UnixNano())
	_ = utilfeature.DefaultMutableFeatureGate.Set("AllAlpha=true")
	system.BindEnvironmentVariables()

	command := cli.NewCommand()

	if err := command.Execute(); err != nil {
		os.Exit(1)
	}

	if err := stdlib.SetupBuiltinImports(); err != nil {
		klog.ErrorS(err, "Unable to set up builtin imports on package initialization")
		os.Exit(1)
	}
}
