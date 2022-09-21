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

package utils

import (
	"context"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"
)

// EnablePprof listen to the pprofAddr and export the profiling results
func EnablePprof(ctx context.Context, pprofAddr string) {
	// Start pprof server if enabled
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	pprofServer := http.Server{
		Addr:    pprofAddr,
		Handler: mux,
	}
	defer func() {
		ctx, cancelFunc := context.WithTimeout(ctx, 60*time.Second)
		defer cancelFunc()

		if err := pprofServer.Shutdown(ctx); err != nil {
			klog.Error(err, "Failed to shutdown debug HTTP server")
		}
	}()

	klog.InfoS("Starting debug HTTP server", "addr", pprofServer.Addr)

	if err := pprofServer.ListenAndServe(); !errors.Is(http.ErrServerClosed, err) {
		klog.Error(err, "Failed to start debug HTTP server")
		panic(err)
	}
}
