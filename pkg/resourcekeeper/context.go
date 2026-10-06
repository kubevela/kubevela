/*
Copyright 2023 The KubeVela Authors.

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

package resourcekeeper

import (
	"context"
)

type contextKey int

const contextFailedRunKey contextKey = iota

// WithFailedRun records whether the owner's current run failed (for an Application, its
// workflow), so a garbage-collect policy with continueOnFailure still collects.
func WithFailedRun(ctx context.Context, failed bool) context.Context {
	return context.WithValue(ctx, contextFailedRunKey, failed)
}

// failedRun reports what WithFailedRun recorded.
func failedRun(ctx context.Context) bool {
	failed, _ := ctx.Value(contextFailedRunKey).(bool)
	return failed
}
