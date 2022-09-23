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
	"net/http"
	"net/http/pprof"

	"k8s.io/klog/v2"
)

// EnablePprof listen to the pprofAddr and export the profiling results
// If the errChan is nil, this function will panic when the listening error occurred.
func EnablePprof(pprofAddr string, errChan chan error) {
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

	klog.InfoS("Starting debug HTTP server", "addr", pprofServer.Addr)

	if err := pprofServer.ListenAndServe(); err != nil {
		klog.Error(err, "Failed to start debug HTTP server")
		if errChan != nil {
			errChan <- err
		} else {
			panic(err)
		}
	}
}
