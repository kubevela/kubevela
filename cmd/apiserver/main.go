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
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/go-openapi/spec"
	"github.com/google/uuid"

	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest"
	"github.com/oam-dev/kubevela/version"
)

func main() {
	s := &Server{}
	flag.StringVar(&s.restCfg.BindAddr, "bind-addr", "0.0.0.0:8000", "The bind address used to serve the http APIs.")
	flag.StringVar(&s.restCfg.MetricPath, "metrics-path", "/metrics", "The path to expose the metrics.")
	flag.StringVar(&s.restCfg.Datastore.Type, "datastore-type", "kubeapi", "Metadata storage driver type, support kubeapi and mongodb")
	flag.StringVar(&s.restCfg.Datastore.Database, "datastore-database", "kubevela", "Metadata storage database name, takes effect when the storage driver is mongodb.")
	flag.StringVar(&s.restCfg.Datastore.URL, "datastore-url", "", "Metadata storage database url,takes effect when the storage driver is mongodb.")
	flag.StringVar(&s.restCfg.LeaderConfig.ID, "id", uuid.New().String(), "the holder identity name")
	flag.StringVar(&s.restCfg.LeaderConfig.LockName, "lock-name", "apiserver-lock", "the lease lock resource name")
	flag.DurationVar(&s.restCfg.LeaderConfig.Duration, "duration", time.Second*5, "the lease lock resource name")
	flag.DurationVar(&s.restCfg.AddonCacheTime, "addon-cache-duration", time.Minute*10, "how long between two addon cache operation")
	flag.BoolVar(&s.restCfg.DisableStatisticCronJob, "disable-statistic-cronJob", false, "close the system statistic info calculating cronJob")
	flag.Parse()

	if len(os.Args) > 2 && os.Args[1] == "build-swagger" {
		func() {
			swagger, err := s.buildSwagger()
			if err != nil {
				log.Logger.Fatal(err.Error())
			}
			outData, err := json.MarshalIndent(swagger, "", "\t")
			if err != nil {
				log.Logger.Fatal(err.Error())
			}
			swaggerFile, err := os.OpenFile(os.Args[2], os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				log.Logger.Fatal(err.Error())
			}
			defer func() {
				if err := swaggerFile.Close(); err != nil {
					log.Logger.Errorf("close swagger file failure %s", err.Error())
				}
			}()
			_, err = swaggerFile.Write(outData)
			if err != nil {
				log.Logger.Fatal(err.Error())
			}
			fmt.Println("build swagger config file success")
		}()
		return
	}

	srvc := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		if err := s.run(ctx); err != nil {
			log.Logger.Errorf("failed to run apiserver: %v", err)
		}
		close(srvc)
	}()
	var term = make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	select {
	case <-term:
		log.Logger.Infof("Received SIGTERM, exiting gracefully...")
		cancel()
	case <-srvc:
		cancel()
		os.Exit(1)
	}
	log.Logger.Infof("See you next time!")
}

// Server apiserver
type Server struct {
	restCfg rest.Config
}

func (s *Server) run(ctx context.Context) error {
	log.Logger.Infof("KubeVela information: version: %v, gitRevision: %v", version.VelaVersion, version.GitRevision)

	server, err := rest.New(s.restCfg)
	if err != nil {
		return fmt.Errorf("create apiserver failed : %w ", err)
	}

	return server.Run(ctx)
}

func (s *Server) buildSwagger() (*spec.Swagger, error) {
	server, err := rest.New(s.restCfg)
	if err != nil {
		return nil, fmt.Errorf("create apiserver failed : %w ", err)
	}
	return restfulspec.BuildSwagger(server.RegisterServices(context.Background(), false)), nil
}
