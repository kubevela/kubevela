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

// ErrGetCloudClusterFailure get cloud cluster failed
var ErrGetCloudClusterFailure = NewBcode(500, 40005, "get cloud cluster information failed")

// ErrClusterExistsInKubernetes cluster exists in kubernetes
var ErrClusterExistsInKubernetes = NewBcode(400, 40006, "cluster already exists in kubernetes")

// ErrLocalClusterReserved cluster name reserved for local
var ErrLocalClusterReserved = NewBcode(400, 40007, "local cluster is reserved")

// ErrLocalClusterImmutable local cluster kubeConfig is immutable
var ErrLocalClusterImmutable = NewBcode(400, 40008, "local cluster is immutable")

// ErrCloudClusterAlreadyExists cloud cluster already exists
var ErrCloudClusterAlreadyExists = NewBcode(400, 40009, "cloud cluster already exists")

// ErrTerraformConfigurationNotFound cannot find terraform configuration
var ErrTerraformConfigurationNotFound = NewBcode(404, 40010, "cannot find terraform configuration")

// ErrClusterIDNotFoundInTerraformConfiguration cannot find cluster_id in terraform configuration
var ErrClusterIDNotFoundInTerraformConfiguration = NewBcode(500, 40011, "cannot find cluster_id in terraform configuration")

// ErrBootstrapTerraformConfiguration failed to bootstrap terraform configuration
var ErrBootstrapTerraformConfiguration = NewBcode(500, 40012, "failed to bootstrap terraform configuration")

// ErrInvalidAccessKeyOrSecretKey access key or secret key is invalid
var ErrInvalidAccessKeyOrSecretKey = NewBcode(400, 40013, "access key or secret key is invalid")
