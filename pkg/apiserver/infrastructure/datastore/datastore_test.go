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

package datastore

import (
	"encoding/json"
	"fmt"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
)

var _ = Describe("Test new entity function", func() {

	It("Test new application entity", func() {
		var app model.Application
		new, err := NewEntity(&app)
		Expect(err).To(BeNil())
		json.Unmarshal([]byte(`{"name":"demo"}`), new)
		Expect(err).To(BeNil())
		diff := cmp.Diff(new.PrimaryKey(), "demo")
		Expect(diff).Should(BeEmpty())
	})

	It("Test new multiple application entity", func() {
		var app model.Application
		var list []Entity
		var n = 3
		for n > 0 {
			new, err := NewEntity(&app)
			Expect(err).To(BeNil())
			json.Unmarshal([]byte(fmt.Sprintf(`{"name":"demo %d"}`, n)), new)
			Expect(err).To(BeNil())
			diff := cmp.Diff(new.PrimaryKey(), fmt.Sprintf("demo %d", n))
			Expect(diff).Should(BeEmpty())
			list = append(list, new)
			n--
		}
		diff := cmp.Diff(list[0].PrimaryKey(), "demo 3")
		Expect(diff).Should(BeEmpty())
	})

})
