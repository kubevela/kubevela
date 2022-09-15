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

package e2e

import (
	"github.com/onsi/ginkgo"

	"github.com/oam-dev/kubevela/e2e"
)

var _ = ginkgo.Describe("Trait", func() {
	e2e.TraitCapabilityListContext()
})

var _ = ginkgo.Describe("Test vela show", func() {
	e2e.ShowCapabilityReference("show ingress", "ingress")
	e2e.ShowCapabilityReferenceMarkdown("show ingress markdown", "ingress")

	env := "namespace-xxxfwrr23erfm"
	e2e.EnvInitWithNamespaceOptionContext("env init", env, env)
	e2e.EnvSetContext("env switch", env)
	e2e.ShowCapabilityReference("show ingress", "ingress")
	e2e.EnvSetContext("env switch", "default")
	e2e.EnvDeleteContext("env delete", env)
})
