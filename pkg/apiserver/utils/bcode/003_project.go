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

// ErrProjectDenyDeleteByApplication the project can't be deleted as there are applications inside
var ErrProjectDenyDeleteByApplication = NewBcode(400, 30005, "the project can't be deleted as there are applications inside")

// ErrProjectDenyDeleteByEnvironment the project can't be deleted because there are environments inside
var ErrProjectDenyDeleteByEnvironment = NewBcode(400, 30006, "the project can't be deleted before you clean up all the environments inside")

// ErrProjectDenyDeleteByTarget the project can't be deleted as there are targets inside
var ErrProjectDenyDeleteByTarget = NewBcode(400, 30007, "the project can't be deleted before you clean up all these targets inside")

// ErrProjectRoleCheckFailure means the specified role does't belong to this project or not exist
var ErrProjectRoleCheckFailure = NewBcode(400, 30008, "the specified role does't belong to this project or not exist")

// ErrProjectUserExist means the user is already exist in this project
var ErrProjectUserExist = NewBcode(400, 30009, "the user is already exist in this project")

// ErrProjectOwnerIsNotExist means the project owner name is invalid
var ErrProjectOwnerIsNotExist = NewBcode(400, 30010, "the project owner name is invalid")
