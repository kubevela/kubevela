package server

import (
	"context"
	"net/http"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ApiServer struct {
	server *http.Server
}

func (s *ApiServer) Launch(kubeClient client.Client, port, staticPath string, errChan chan<- error) {
	s.server = &http.Server{
		Addr:         port,
		Handler:      setupRoute(kubeClient, staticPath),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	s.server.SetKeepAlivesEnabled(true)

	go func() {
		err := s.server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()
}

func (s *ApiServer) Shutdown(ctx context.Context) error {
	ctrl.Log.Info("sever shutting down")
	return s.server.Shutdown(ctx)
}
