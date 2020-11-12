package server

import (
	"context"
	"net/http"
	"time"

	"github.com/oam-dev/kubevela/api/types"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/discoverymapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type APIServer struct {
	server     *http.Server
	KubeClient client.Client
	dm         discoverymapper.DiscoveryMapper
}

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

func (s *APIServer) Launch(errChan chan<- error) {
	go func() {
		err := s.server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()
}

func (s *APIServer) Shutdown(ctx context.Context) error {
	ctrl.Log.Info("sever shutting down")
	return s.server.Shutdown(ctx)
}
