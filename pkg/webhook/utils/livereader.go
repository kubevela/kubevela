/*
Copyright 2026 The KubeVela Authors.

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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReadWithLiveRetry runs a read against the cached client and, if what it
// needed was not found, runs the whole thing again against the API server.
//
// The retry is per lookup rather than per Get on purpose. oamutil.GetDefinition
// tries the Application's namespace before the system one, so in the usual
// layout — definitions in vela-system, the app elsewhere — its first Get is a
// guaranteed miss. A client wrapper that went live on each miss would pay a
// round trip on every lookup, and admission sits on the whole write path,
// including the updates a controller makes while reconciling. Retrying the
// lookup instead costs nothing when the cache can answer at all.
//
// Only absence is retried: any other error says nothing about the cache being
// behind. A nil live client runs the read once, which is what a test supplying a
// fake wants and what a manager that could not build an uncached client falls
// back to.
func ReadWithLiveRetry(cached, live client.Client, read func(client.Client) error) error {
	err := read(cached)
	if live == nil || !apierrors.IsNotFound(err) {
		return err
	}
	return read(live)
}

// LiveClient returns a client that reads straight from the API server, for use
// behind ReadWithLiveRetry.
//
// A full client rather than mgr.GetAPIReader(): resolving a pinned revision
// asserts its reader back to a client.Client.
//
// Returns nil if a client cannot be built, so a caller can fall back to the
// cached one rather than refuse to start: stale reads are a worse failure than
// no webhook, but neither is worth a crash loop.
func LiveClient(mgr ctrl.Manager) client.Client {
	cli, err := client.New(mgr.GetConfig(), client.Options{
		Scheme: mgr.GetScheme(),
		Mapper: mgr.GetRESTMapper(),
	})
	if err != nil {
		klog.ErrorS(err, "no uncached client for admission; definition chains will be resolved from the cache "+
			"and may briefly not see a parent written alongside its child")
		return nil
	}
	return cli
}
