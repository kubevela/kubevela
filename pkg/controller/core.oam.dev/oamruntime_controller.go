/*
Copyright 2020 The KubeVela Authors.

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

package core_oam_dev

// Args args used by controller
type Args struct {
	// RevisionLimit is the maximum number of revisions that will be maintained.
	// The default value is 50.
	RevisionLimit int

	// ApplyOnceOnly indicates whether workloads and traits should be
	// affected if no spec change is made in the ApplicationConfiguration.
	ApplyOnceOnly bool
}
