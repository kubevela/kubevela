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

package mock

// Action ...
type Action struct {
	Phase   string
	Message string
}

// Suspend makes the step suspend
func (act *Action) Suspend(message string) {
	act.Phase = "Suspend"
	act.Message = message
}

// Terminate makes the step terminate
func (act *Action) Terminate(message string) {
	act.Phase = "Terminate"
	act.Message = message
}

// Wait makes the step wait
func (act *Action) Wait(message string) {
	act.Phase = "Wait"
	act.Message = message
}

// Fail makes the step fail
func (act *Action) Fail(message string) {
	act.Phase = "Fail"
	act.Message = message
}
