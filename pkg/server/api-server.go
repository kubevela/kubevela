package server

import (
	"context"
	"net/http"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

type ApiServer struct {
	server *http.Server
}

func (s *ApiServer) Launch() {
	s.server = &http.Server{
		Addr:         ":8080",
		Handler:      setupRoute(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	s.server.SetKeepAlivesEnabled(true)

	go (func() error {
		err := s.server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			ctrl.Log.Error(err, "failed to start the server")
		}
		return err
	})()
}

func (s *ApiServer) Shutdown(ctx context.Context) error {
	ctrl.Log.Info("sever shutting down")
	return s.server.Shutdown(ctx)
}
