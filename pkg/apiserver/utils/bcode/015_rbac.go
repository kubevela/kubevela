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

var (
	// ErrRolePermissionCheckFailure means the perm policy is invalid where create or update role
	ErrRolePermissionCheckFailure = NewBcode(400, 15001, "the permissions are invalid")
	// ErrRoleIsExist means the role is exist
	ErrRoleIsExist = NewBcode(400, 15002, "the role name is exist")
	// ErrRoleIsNotExist means the role is not exist
	ErrRoleIsNotExist = NewBcode(400, 15003, "the role is not exist")
	// ErrPermissionNotExist means the permission is not exist
	ErrPermissionNotExist = NewBcode(404, 15004, "the permission is not exist")
	// ErrPermissionIsExist means the he permission is exist
	ErrPermissionIsExist = NewBcode(400, 15005, "the permission name is exist")
	// ErrPermissionIsUsed means the permission is bound by role, can not be deleted
	ErrPermissionIsUsed = NewBcode(400, 15006, "the permission have been used")
)
