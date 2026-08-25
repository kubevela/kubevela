/*
Copyright 2026 The KubeVela Authors.

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

package errors

import "errors"

// ErrFileNotFound reports that a file store does not contain a file, as distinct
// from being unreachable, misconfigured, or refusing the credentials.
//
// Absence is often an ordinary outcome worth branching on - a per-cluster
// override only some clusters carry - while a failure is not. It lives here
// because the addon registry readers and the CUE registry provider cannot import
// each other.
var ErrFileNotFound = errors.New("file not found")
