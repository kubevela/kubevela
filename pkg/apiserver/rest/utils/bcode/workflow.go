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

package bcode

// ErrWorkflowNotExist application workflow is not exist
var ErrWorkflowNotExist = NewBcode(404, 20002, "application workflow is not exist")

// ErrWorkflowExist application workflow is exist
var ErrWorkflowExist = NewBcode(404, 20003, "application workflow is exist")
