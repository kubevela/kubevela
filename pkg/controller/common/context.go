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

package common

import (
	"context"
	"time"
)

var (
	// PerfEnabled identify whether to add performance log for controllers
	PerfEnabled = false
)

var (
	// ReconcileTimeout timeout for controller to reconcile
	ReconcileTimeout = time.Minute * 3
	// ApplicationReSyncPeriod re-sync period to reconcile application
	ApplicationReSyncPeriod = time.Minute * 5
)

// NewReconcileContext create context with default timeout (60s)
func NewReconcileContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, ReconcileTimeout)
}
