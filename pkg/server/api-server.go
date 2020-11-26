package server

import (
	"context"
	"net/http"
	"time"

	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
)

// APIServer run a restful API server for dashboard
type APIServer struct {
	server     *http.Server
	KubeClient client.Client
	dm         discoverymapper.DiscoveryMapper
}

// New will create APIServer
func New(c types.Args, port, staticPath string) (*APIServer, error) {
	newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
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
