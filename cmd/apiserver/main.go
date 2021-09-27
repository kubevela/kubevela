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
	"os/signal"
	"syscall"

	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest"
	"github.com/oam-dev/kubevela/version"
)

func main() {
	s := &server{}
	flag.StringVar(&s.restCfg.BindAddr, "bind-addr", "0.0.0.0:8000", "The bind address used to serve the http APIs.")
	flag.StringVar(&s.restCfg.MetricPath, "metrics-path", "/metrics", "The path to expose the metrics.")
	flag.StringVar(&s.restCfg.Datastore.Type, "datastore-type", "kubeapi", "Metadata storage driver type, support kubeapi and mongodb")
	flag.StringVar(&s.restCfg.Datastore.Database, "datastore-database", "kubevela", "Metadata storage database name, takes effect when the storage driver is mongodb.")
	flag.StringVar(&s.restCfg.Datastore.URL, "datastore-url", "", "Metadata storage database url,takes effect when the storage driver is mongodb.")
	flag.Parse()

	srvc := make(chan struct{})

	go func() {
		if err := s.run(); err != nil {
			log.Logger.Errorf("failed to run apiserver: %v", err)
		}
		close(srvc)
	}()
	var term = make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	select {
	case <-term:
		log.Logger.Infof("Received SIGTERM, exiting gracefully...")
	case <-srvc:
		os.Exit(1)
	}
	log.Logger.Infof("See you next time!")
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
