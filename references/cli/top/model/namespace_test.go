/*
Copyright 2022 The KubeVela Authors.

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

package model

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
)

func TestNamespaceList_Header(t *testing.T) {
	nsList := NamespaceList{
		title: []string{"Name", "Status", "Age"},
		data:  []Namespace{{Name: AllNamespace, Status: "*", Age: "*"}},
	}
	assert.Equal(t, nsList.Header(), []string{"Name", "Status", "Age"})
}

func TestNamespaceList_Body(t *testing.T) {
	nsList := NamespaceList{
		title: []string{"Name", "Status", "Age"},
		data:  []Namespace{{Name: AllNamespace, Status: "*", Age: "*"}},
	}
	assert.Equal(t, len(nsList.Body()), 1)
	assert.Equal(t, nsList.Body()[0], []string{AllNamespace, "*", "*"})
}

func TestTimeFormat(t *testing.T) {
	t1, err1 := time.ParseDuration("1.5h")
	assert.NoError(t, err1)
	assert.Equal(t, timeFormat(t1), "0d1h30m0ss")
	t2, err2 := time.ParseDuration("25h")
	assert.NoError(t, err2)
	assert.Equal(t, timeFormat(t2), "1d1h0m0ss")
}

var _ = Describe("test namespace", func() {
	ctx := context.Background()
	It("list namespace", func() {
		nsList := ListNamespaces(ctx, k8sClient)
		Expect(len(nsList.Header())).To(Equal(3))
		Expect(nsList.Header()).To(Equal([]string{"Name", "Status", "Age"}))
		Expect(len(nsList.Body())).To(Equal(6))
		Expect(nsList.Body()[0]).To(Equal([]string{"all", "*", "*"}))
	})
})
