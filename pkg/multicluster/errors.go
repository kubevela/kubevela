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

package multicluster

import (
	"fmt"
	"strings"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	// ErrClusterExists cluster already exists
	ErrClusterExists = ClusterManagementError(fmt.Errorf("cluster already exists"))
	// ErrClusterNotExists cluster not exists
	ErrClusterNotExists = ClusterManagementError(fmt.Errorf("no such cluster"))
	// ErrReservedLocalClusterName reserved cluster name is used
	ErrReservedLocalClusterName = ClusterManagementError(fmt.Errorf("cluster name `local` is reserved for kubevela hub cluster"))
	// ErrDetectClusterGateway fail to wait for ClusterGateway service ready
	ErrDetectClusterGateway = ClusterManagementError(fmt.Errorf("failed to wait for cluster gateway, unable to use multi-cluster"))
)

// ClusterManagementError multicluster management error
type ClusterManagementError error

// IsClusterNotExists check if error is cluster not exists
func IsClusterNotExists(err error) bool {
	return strings.Contains(err.Error(), "no such cluster")
}

// IsNotFoundOrClusterNotExists check if error is not found or cluster not exists
func IsNotFoundOrClusterNotExists(err error) bool {
	return kerrors.IsNotFound(err) || IsClusterNotExists(err)
}

// IsClusterDisconnect check if error is cluster disconnect
func IsClusterDisconnect(err error) bool {
	return strings.Contains(err.Error(), "dial tcp")
}
