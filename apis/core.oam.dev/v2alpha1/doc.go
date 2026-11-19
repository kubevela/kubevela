/*
 Copyright 2026. The KubeVela Authors.

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

// Package v2alpha1 contains the work-in-progress implementation of the
// Operations KEP (KEP 2.15): OperationTemplate and Operation.
//
// TODO(KEP 2.15): the permission model isn't implemented yet (no admission
// webhooks, no SubjectAccessReview checks) -- do not run outside a
// disposable namespace against non-destructive templates.
// +kubebuilder:object:generate=true
// +groupName=core.oam.dev
// +versionName=v2alpha1
package v2alpha1
