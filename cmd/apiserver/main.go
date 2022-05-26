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

	"github.com/oam-dev/kubevela/pkg/apiserver"
	"github.com/oam-dev/kubevela/pkg/apiserver/config"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/version"
)

func main() {
	s := &Server{}
	flag.StringVar(&s.serverConfig.BindAddr, "bind-addr", "0.0.0.0:8000", "The bind address used to serve the http APIs.")
	flag.StringVar(&s.serverConfig.MetricPath, "metrics-path", "/metrics", "The path to expose the metrics.")
	flag.StringVar(&s.serverConfig.Datastore.Type, "datastore-type", "kubeapi", "Metadata storage driver type, support kubeapi and mongodb")
	flag.StringVar(&s.serverConfig.Datastore.Database, "datastore-database", "kubevela", "Metadata storage database name, takes effect when the storage driver is mongodb.")
	flag.StringVar(&s.serverConfig.Datastore.URL, "datastore-url", "", "Metadata storage database url,takes effect when the storage driver is mongodb.")
	flag.StringVar(&s.serverConfig.LeaderConfig.ID, "id", uuid.New().String(), "the holder identity name")
	flag.StringVar(&s.serverConfig.LeaderConfig.LockName, "lock-name", "apiserver-lock", "the lease lock resource name")
	flag.DurationVar(&s.serverConfig.LeaderConfig.Duration, "duration", time.Second*5, "the lease lock resource name")
	flag.DurationVar(&s.serverConfig.AddonCacheTime, "addon-cache-duration", time.Minute*10, "how long between two addon cache operation")
	flag.BoolVar(&s.serverConfig.DisableStatisticCronJob, "disable-statistic-cronJob", false, "close the system statistic info calculating cronJob")
	flag.Float64Var(&s.serverConfig.KubeQPS, "kube-api-qps", 100, "the qps for kube clients. Low qps may lead to low throughput. High qps may give stress to api-server.")
	flag.IntVar(&s.serverConfig.KubeBurst, "kube-api-burst", 300, "the burst for kube clients. Recommend setting it qps*3.")

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

	errChan := make(chan error)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := s.run(ctx, errChan); err != nil {
			errChan <- fmt.Errorf("failed to run apiserver: %w", err)
		}
	}()
	var term = make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	select {
	case <-term:
		log.Logger.Infof("Received SIGTERM, exiting gracefully...")
	case err := <-errChan:
		log.Logger.Errorf("Received an error: %s, exiting gracefully...", err.Error())
	}
	log.Logger.Infof("See you next time!")
}

// Server apiserver
type Server struct {
	serverConfig config.Config
}

func (s *Server) run(ctx context.Context, errChan chan error) error {
	log.Logger.Infof("KubeVela information: version: %v, gitRevision: %v", version.VelaVersion, version.GitRevision)

	server := apiserver.New(s.serverConfig)

	return server.Run(ctx, errChan)
}

func (s *Server) buildSwagger() (*spec.Swagger, error) {
	server := apiserver.New(s.serverConfig)
	config, err := server.BuildRestfulConfig()
	if err != nil {
		return nil, err
	}
	return restfulspec.BuildSwagger(*config), nil
}
