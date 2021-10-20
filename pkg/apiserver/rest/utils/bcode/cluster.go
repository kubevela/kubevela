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

// ErrInvalidCloudClusterProvider provider is not support now
var ErrInvalidCloudClusterProvider = NewBcode(400, 40000, "provider is not support")

// ErrKubeConfigSecretNotSupport kubeConfig secret is not support
var ErrKubeConfigSecretNotSupport = NewBcode(400, 40001, "kubeConfig secret is not supported now")

// ErrKubeConfigAndSecretIsNotSet kubeConfig and kubeConfigSecret are not set
var ErrKubeConfigAndSecretIsNotSet = NewBcode(400, 40002, "kubeConfig or kubeConfig secret must be provided")

// ErrClusterNotFoundInDataStore cluster not found in datastore
var ErrClusterNotFoundInDataStore = NewBcode(404, 40003, "cluster not found in data store")

// ErrClusterAlreadyExistInDataStore cluster exists in datastore
var ErrClusterAlreadyExistInDataStore = NewBcode(400, 40004, "cluster already exists in data store")
