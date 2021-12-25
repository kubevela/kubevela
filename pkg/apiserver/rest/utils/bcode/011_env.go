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

// ErrEnvAlreadyExists Env name is existed
var ErrEnvAlreadyExists = NewBcode(400, 11001, "env name already exists")

// ErrEnvNotExisted means env is not existed
var ErrEnvNotExisted = NewBcode(404, 11002, "env is not existed")

// ErrEnvNamespaceFail env binds namespace failure
var ErrEnvNamespaceFail = NewBcode(400, 11003, "env bind namespace failure")

// ErrEnvNamespaceAlreadyBound indicates the namespace already belongs to other env
var ErrEnvNamespaceAlreadyBound = NewBcode(400, 11004, "the namespace specified already belongs to other env")

// ErrDeleteEnvButAppExist reports an error when delete an Env but still has apps inside
var ErrDeleteEnvButAppExist = NewBcode(400, 11005, "env can't be deleted as app existed inside")

// ErrEnvTargetConflict in one project, one target can only belong to one env
var ErrEnvTargetConflict = NewBcode(400, 11006, "in one project, one target can only belong to one env.")
