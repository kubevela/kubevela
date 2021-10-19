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

// ErrApplicationConfig application config does not comply with OAM specification
var ErrApplicationConfig = NewBcode(400, 10000, "application config does not comply with OAM specification")

// ErrComponentTypeNotSupport an unsupported component type was used.
var ErrComponentTypeNotSupport = NewBcode(400, 10001, "An unsupported component type was used.")

// ErrApplicationExist application is exist
var ErrApplicationExist = NewBcode(400, 10002, "application name is exist")

// ErrInvalidProperties properties(trait or component or others) is invalid
var ErrInvalidProperties = NewBcode(400, 10003, "properties is invalid")

// ErrDeployEventConflict Occurs when a new event is triggered before the last deployment event has completed.
var ErrDeployEventConflict = NewBcode(400, 10004, "application deploy event conflict")

// ErrDeployApplyFail Failed to update an application to the control cluster.
var ErrDeployApplyFail = NewBcode(500, 10005, "application deploy apply failure")

// ErrNoComponent no component
var ErrNoComponent = NewBcode(200, 10006, "application not have components, can not deploy")

// ErrApplicationComponetExist application component is exist
var ErrApplicationComponetExist = NewBcode(400, 10007, "application component is exist")

// ErrApplicationComponetNotExist  application component is not exist
var ErrApplicationComponetNotExist = NewBcode(404, 10008, "application component is not exist")

// ErrApplicationPolicyExist application policy is exist
var ErrApplicationPolicyExist = NewBcode(400, 10009, "application policy is exist")

// ErrApplicationPolicyNotExist  application policy is not exist
var ErrApplicationPolicyNotExist = NewBcode(404, 10010, "application policy is not exist")

// ErrCreateNamespace auto create namespace failure before deploy app
var ErrCreateNamespace = NewBcode(500, 10011, "auto create namespace failure")
