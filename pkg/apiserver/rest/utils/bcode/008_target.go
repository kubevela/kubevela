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

// ErrTargetExist Target is exist
var ErrTargetExist = NewBcode(400, 80001, "target is exist")

// ErrTargetNotExist Target is not exist
var ErrTargetNotExist = NewBcode(404, 80002, "target is not exist")

// ErrTargetInUseCantDeleted Target being used
var ErrTargetInUseCantDeleted = NewBcode(404, 80003, "target in use, can't be deleted")
