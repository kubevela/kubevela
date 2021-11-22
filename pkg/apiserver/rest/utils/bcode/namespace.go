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

// ErrNamespaceQuery query namespace failure from k8s api
var ErrNamespaceQuery = NewBcode(500, 30001, "query namespace list from cluster failure")

// ErrNamespaceIsExist namespace name is exist
var ErrNamespaceIsExist = NewBcode(400, 30002, "namespace name is exist")
