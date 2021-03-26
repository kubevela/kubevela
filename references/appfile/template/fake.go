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

package template

// FakeTemplateManager defines a fake for test
type FakeTemplateManager struct {
	*manager
}

// NewFakeTemplateManager create a fake manager for test
func NewFakeTemplateManager() *FakeTemplateManager {
	return &FakeTemplateManager{
		manager: newManager(),
	}
}
