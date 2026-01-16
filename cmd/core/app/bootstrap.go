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

package app

import (
	"k8s.io/klog/v2"
)

// bootstrapProviderRegistry registers framework-level providers that need
// to be available globally. This is called early in prepareRun before
// controllers are initialized.
//
// The provider registry is a fallback mechanism for breaking import cycles
// that block development. Providers registered here enable immediate feature
// work while longer-term refactoring efforts can be planned.
//
// Best practices when adding a provider:
// 1. Document which packages have the cycle (in comments below)
// 2. Define narrow, focused interfaces (< 5 methods)
// 3. Consider opportunities for future refactoring to eliminate the cycle
// 4. Prefer constructor injection for new code without cycles
//
// See pkg/registry/README.md for feature overview and pkg/registry package docs for guidelines.
func bootstrapProviderRegistry() {
	klog.V(2).InfoS("Bootstrapping provider registry")

	// ────────────────────────────────────────────────────────────────────
	// Add providers below following this pattern:
	// ────────────────────────────────────────────────────────────────────
	//
	// ProviderInterface - Brief description
	// Cycle: pkg/foo ↔ pkg/bar (explain the circular dependency)
	// Note: Consider refactoring to extract shared interfaces
	// registry.RegisterAs[ProviderInterface](implementation)

	klog.V(2).InfoS("Provider registry bootstrap complete")
}
