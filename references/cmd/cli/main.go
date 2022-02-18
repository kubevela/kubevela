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

	"github.com/oam-dev/kubevela/apis/types"

	"github.com/oam-dev/kubevela/references/a/preimport"
	"github.com/oam-dev/kubevela/references/cli"
)

func main() {
	preimport.ResumeLogging()
	rand.Seed(time.Now().UnixNano())

	if ns := os.Getenv("DEFAULT_VELA_NS"); len(ns) != 0 {
		types.DefaultKubeVelaNS = ns
	}

	command := cli.NewCommand()

	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
