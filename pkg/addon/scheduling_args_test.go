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

package addon

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NewAddonInstaller and the caller's args", func() {
	It("keeps the scheduling args so the next registry installs with them too", func() {
		args := map[string]interface{}{
			SchedulingArg:          map[string]interface{}{"nodeSelector": map[string]interface{}{"disktype": "ssd"}},
			InstallerRuntimeOption: map[string]interface{}{"upgrade": true},
			"replicas":             2,
		}

		for range 2 {
			h := NewAddonInstaller(context.Background(), nil, nil, nil, nil, &Registry{Name: "first"}, args, nil, nil)
			Expect(h.schedulingErr).NotTo(HaveOccurred())
			Expect(h.scheduling.NodeSelector).To(HaveKeyWithValue("disktype", "ssd"))
			Expect(h.installerRuntime).To(HaveKeyWithValue("upgrade", true))
			Expect(h.args).NotTo(HaveKey(SchedulingArg))
			Expect(h.args).NotTo(HaveKey(InstallerRuntimeOption))
		}

		Expect(args).To(HaveKey(SchedulingArg))
		Expect(args).To(HaveKey(InstallerRuntimeOption))
		Expect(args).To(HaveKeyWithValue("replicas", 2))
	})
})
