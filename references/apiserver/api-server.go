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

package apiserver

import (
	"context"
	"net/http"
	"time"

	"github.com/oam-dev/kubevela/pkg/utils/common"

	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
)

// APIServer run a restful API server for dashboard
type APIServer struct {
	server     *http.Server
	KubeClient client.Client
	dm         discoverymapper.DiscoveryMapper
	c          common.Args
}

// New will create APIServer
func New(c common.Args, port, staticPath string) (*APIServer, error) {
	newClient, err := c.GetClient()
	if err != nil {
		return nil, err
	}
	dm, err := discoverymapper.New(c.Config)
	if err != nil {
		return nil, err
	}
	s := &APIServer{
		KubeClient: newClient,
		dm:         dm,
		c:          c,
	}
	server := &http.Server{
		Addr:         port,
		Handler:      s.setupRoute(staticPath),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	server.SetKeepAlivesEnabled(true)
	s.server = server
	return s, nil
}

// Launch will start the apiserver
func (s *APIServer) Launch(errChan chan<- error) {
	go func() {
		err := s.server.ListenAndServe()
		if err != nil && errors.Is(err, http.ErrServerClosed) {

			errChan <- err
		}
	}()
}

// Shutdown will close the apiserver
func (s *APIServer) Shutdown(ctx context.Context) error {
	ctrl.Log.Info("sever shutting down")
	return s.server.Shutdown(ctx)
}
