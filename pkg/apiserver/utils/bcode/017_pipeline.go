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

package bcode

import "fmt"

var (
	// ErrContextNotFound means the certain context is not found
	ErrContextNotFound = NewBcode(400, 17001, "pipeline context is not found")
	// ErrContextAlreadyExist means the certain context already exists
	ErrContextAlreadyExist = NewBcode(400, 17002, "pipeline context of pipeline already exist")
	// ErrNoLogConfig means the certain pipeline step has no log config
	ErrNoLogConfig = NewBcode(400, 17003, "no log config found in pipeline run step")
	// ErrFindingLogPods means no valid pod found
	ErrFindingLogPods = func(err error) *Bcode {
		return NewBcode(400, 17004, fmt.Sprintf("failed to find log pods: %v when try to log pipeline run step", err))
	}
)
