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

package hooks

import "context"

// PreStartHook defines a hook that should be run before the controller starts working.
// Pre-start hooks are used for validation, initialization, and safety checks that must
// pass before the controller begins processing resources.
type PreStartHook interface {
	// Run executes the hook's logic. If an error is returned, the controller
	// startup will be aborted.
	Run(ctx context.Context) error

	// Name returns a human-readable name for the hook, used in logging.
	Name() string
}
