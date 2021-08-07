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

package server

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/pkg/apiserver/rest"
)

type server struct {
	restCfg rest.Config
}

// NewServerCommand create server command
func NewServerCommand() *cobra.Command {
	s := &server{}

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start running apiserver.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.run()
		},
	}

	cmd.Flags().IntVar(&s.restCfg.Port, "port", 8000, "The port number used to serve the http APIs.")

	return cmd
}

func (s *server) run() error {
	ctx := context.Background()

	server, err := rest.New(s.restCfg)
	if err != nil {
		return fmt.Errorf("create apiserver failed : %w ", err)
	}
	return server.Run(ctx)
}
