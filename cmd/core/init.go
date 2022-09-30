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

package main

import (
	_ "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// NOTE: the import package in this file will be initialized prior to the
// main.go, it is used to ensure that certain packages initialization

// controller-runtime/pkg/metrics must be initialized before
// k8s.io/component-base/metrics/prometheus/workqueue, otherwise the workqueue
// metrics will not be correctly registered and used
