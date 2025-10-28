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
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
)

func TestE2EMain(t *testing.T) {
	fmt.Println("this is e2e test")
	var (
		args []string
		run  bool
	)

	for _, arg := range os.Args {
		switch {
		case strings.HasPrefix(arg, "__DEVEL__E2E"):
			run = true
		case strings.HasPrefix(arg, "-test"):
		default:
			args = append(args, arg)
		}
	}

	if !run {
		return
	}

	waitCh := make(chan int, 1)

	//args=append(args, "leader-election-namespace='someNS'")
	os.Args = args
	go func() {
		main()
		close(waitCh)
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP)
	select {
	case <-signalCh:
	case <-waitCh:
	}
	fmt.Println("exit test e2e main")
}
