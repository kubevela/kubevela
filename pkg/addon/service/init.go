/*
Copyright 2025 The KubeVela Authors.

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

package service

import (
	"sync"

	addonprovider "github.com/oam-dev/kubevela/pkg/cue/cuex/providers/addon"
)

var initOnce sync.Once

// InitAddonProvider initializes the addon provider with the service implementation
func InitAddonProvider() {
	initOnce.Do(func() {
		renderer := NewAddonRenderer()
		addonprovider.SetRenderer(renderer)
	})
}

// init automatically initializes the addon provider when the package is imported
func init() {
	InitAddonProvider()
}