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

package application

// mandatoryGlobalError is a global policy that failed to load or render while global policies are
// mandatory (--allow-skip-global-policies off).
type mandatoryGlobalError struct {
	policy string
	err    error
}

func (e *mandatoryGlobalError) Error() string { return e.err.Error() }

func (e *mandatoryGlobalError) Unwrap() error { return e.err }

// explicitPolicyError is an explicit policy that failed to render; policy is its definition.
type explicitPolicyError struct {
	policy string
	err    error
}

func (e *explicitPolicyError) Error() string { return e.err.Error() }

func (e *explicitPolicyError) Unwrap() error { return e.err }
