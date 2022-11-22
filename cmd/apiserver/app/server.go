/*
Copyright 2022 The KubeVela Authors.

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

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/fatih/color"
	"github.com/go-openapi/spec"
	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/cmd/apiserver/app/options"
	"github.com/oam-dev/kubevela/pkg/apiserver"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/version"
)

// NewAPIServerCommand creates a *cobra.Command object with default parameters
func NewAPIServerCommand() *cobra.Command {
	s := options.NewServerRunOptions()
	cmd := &cobra.Command{
		Use: "apiserver",
		Long: `The KubeVela API server validates and configures data for the API objects. 
The API Server services REST operations and provides the frontend to the
cluster's shared state through which all other components interact.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := s.Validate(); err != nil {
				return err
			}
			return Run(s)
		},
		SilenceUsage: true,
	}

	fs := cmd.Flags()
	namedFlagSets := s.Flags()
	for _, set := range namedFlagSets.FlagSets {
		fs.AddFlagSet(set)
	}

	buildSwaggerCmd := &cobra.Command{
		Use:   "build-swagger",
		Short: "Build swagger documentation of KubeVela apiserver",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := "docs/apidoc/latest-swagger.json"
			if len(args) > 0 {
				name = args[0]
			}
			func() {
				swagger, err := buildSwagger(s)
				if err != nil {
					log.Logger.Fatal(err.Error())
				}
				outData, err := json.MarshalIndent(swagger, "", "\t")
				if err != nil {
					log.Logger.Fatal(err.Error())
				}
				swaggerFile, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
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
			return nil
		},
	}

	cmd.AddCommand(buildSwaggerCmd)

	return cmd
}

// Run runs the specified APIServer. This should never exit.
func Run(s *options.ServerRunOptions) error {
	// The server is not terminal, there is no color default.
	// Force set to false, this is useful for the dry-run API.
	color.NoColor = false

	errChan := make(chan error)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if s.GenericServerRunOptions.PprofAddr != "" {
		go utils.EnablePprof(s.GenericServerRunOptions.PprofAddr, errChan)
	}

	go func() {
		if err := run(ctx, s, errChan); err != nil {
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
		return err
	}
	log.Logger.Infof("See you next time!")
	return nil
}

func run(ctx context.Context, s *options.ServerRunOptions, errChan chan error) error {
	log.Logger.Infof("KubeVela information: version: %v, gitRevision: %v", version.VelaVersion, version.GitRevision)

	server := apiserver.New(*s.GenericServerRunOptions)

	return server.Run(ctx, errChan)
}

func buildSwagger(s *options.ServerRunOptions) (*spec.Swagger, error) {
	server := apiserver.New(*s.GenericServerRunOptions)
	config, err := server.BuildRestfulConfig()
	if err != nil {
		return nil, err
	}
	return restfulspec.BuildSwagger(*config), nil
}
