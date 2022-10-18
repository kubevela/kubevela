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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("test log output", func() {
	var logC chan string
	var err error

	ctx, cancelFunc := context.WithCancel(context.Background())

	It("generate channel", func() {
		logC, err = PrintLogOfPod(ctx, cfg, "local", "default", "pod1", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(len(logC)).To(Equal(0))

		flag := false
		select {
		case <-ctx.Done():
			flag = true
		default:
		}
		Expect(flag).To(Equal(false))
	})

	It("close channel", func() {
		cancelFunc()
		flag := false
		select {
		case <-ctx.Done():
			flag = true
		default:
		}
		Expect(flag).To(Equal(true))
	})
})
