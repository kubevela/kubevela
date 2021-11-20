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

// ErrWorkflowNoDefault application default workflow is not exist
var ErrWorkflowNoDefault = NewBcode(404, 20004, "application default workflow is not exist")

// ErrMustQueryByApp you can only query the Workflow list based on applications.
var ErrMustQueryByApp = NewBcode(404, 20005, "you can only query the Workflow list based on applications.")

// ErrWorkflowNoEnv workflow have not env
var ErrWorkflowNoEnv = NewBcode(400, 20006, "workflow must set env name")

// ErrWorkflowRecordNotExist workflow record is not exist
var ErrWorkflowRecordNotExist = NewBcode(404, 20007, "workflow record is not exist")
