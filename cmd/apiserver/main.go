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
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest"
	"github.com/oam-dev/kubevela/version"
)

func main() {
	s := &server{}

	flag.IntVar(&s.restCfg.Port, "port", 8000, "The port number used to serve the http APIs.")
	flag.Parse()

	if err := s.run(); err != nil {
		log.Logger.Errorf("failed to run apiserver: %v", err)
		os.Exit(1)
	}
}

type server struct {
	restCfg rest.Config
}

func (s *server) run() error {
	log.Logger.Infof("KubeVela information: version: %v, gitRevision: %v", version.VelaVersion, version.GitRevision)

	ctx := context.Background()

	server, err := rest.New(s.restCfg)
	if err != nil {
		return fmt.Errorf("create apiserver failed : %w ", err)
	}
	return server.Run(ctx)
}
