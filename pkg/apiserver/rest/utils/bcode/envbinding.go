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

// ErrEnvbindingDeliveryTargetNotAllExist application envbinding deliveryTarget is not exist
var ErrEnvbindingDeliveryTargetNotAllExist = NewBcode(400, 90001, "application envbinding deliveryTarget is not all exist")

// ErrFoundEnvbindingDeliveryTarget found application envbinding deliveryTarget failure
var ErrFoundEnvbindingDeliveryTarget = NewBcode(400, 90002, "found application envbinding deliveryTarget failure")

// ErrEnvBindingNotExist application envbinding  is not exist
var ErrEnvBindingNotExist = NewBcode(400, 90003, "application envbinding not exist")

// ErrEnvBindingsNotExist application envbindings  is not exist
var ErrEnvBindingsNotExist = NewBcode(400, 90004, "application envbinding snot exist")

// ErrEnvBindingExist application envbinding  is  exist
var ErrEnvBindingExist = NewBcode(400, 90005, "application envbinding is exist")
