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

// ErrProjectIsExist project name is exist
var ErrProjectIsExist = NewBcode(400, 30001, "project name already exists")

// ErrProjectIsNotExist project is not exist
var ErrProjectIsNotExist = NewBcode(404, 30002, "project is not existed")

// ErrProjectNamespaceFail project bind namespace failure
var ErrProjectNamespaceFail = NewBcode(400, 30003, "project bind namespace failure")

// ErrProjectNamespaceIsExist the namespace belongs to the other project
var ErrProjectNamespaceIsExist = NewBcode(400, 30004, "the namespace belongs to the other project")

// ErrProjectDenyDeleteByApplication the project cannot be deleted because there are some applications
var ErrProjectDenyDeleteByApplication = NewBcode(400, 30005, "the project cannot be deleted because there are some applications")

// ErrProjectDenyDeleteByEnvironment the project cannot be deleted because there are some environments
var ErrProjectDenyDeleteByEnvironment = NewBcode(400, 30006, "the project cannot be deleted because there are some environments")

// ErrProjectDenyDeleteByTarget the project cannot be deleted because there are some targets
var ErrProjectDenyDeleteByTarget = NewBcode(400, 30007, "the project cannot be deleted because there are some targets")

// ErrProjectRoleCheckFailure means role is not exit or role is not belong to this project.
var ErrProjectRoleCheckFailure = NewBcode(400, 30008, "the role is not exit or not belong to this project")

// ErrProjectUserExist means the use is exit in this project
var ErrProjectUserExist = NewBcode(400, 30009, "the use is exit in this project")
