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

package utils

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test cache utils", func() {
	It("should return false for IsExpired()", func() {
		c := newMemoryCache("test", 10*time.Hour)
		Expect(c.IsExpired()).Should(BeFalse())
	})

	It("test cache store", func() {
		store := NewMemoryCacheStore(context.TODO())
		store.Put("test", "test data", time.Second*2)
		store.Put("test2", "test data", 0)
		store.Put("test3", "test data", -1)
		time.Sleep(3 * time.Second)
		Expect(store.Get("test")).Should(BeNil())
		Expect(store.Get("test2")).Should(Equal("test data"))
		Expect(store.Get("test3")).Should(Equal("test data"))
	})

	It("test cache store delete key", func() {
		store := NewMemoryCacheStore(context.TODO())
		store.Put("test", "test data", time.Minute*2)
		store.Delete("test")
		Expect(store.Get("test")).Should(BeNil())
	})
})

var store *MemoryCacheStore

// BenchmarkWrite
func BenchmarkWrite(b *testing.B) {
	store = NewMemoryCacheStore(context.TODO())

	for i := 0; i < b.N; i++ {
		store.Put(fmt.Sprintf("%d", i), i, 0)
	}
}

// BenchmarkRead
func BenchmarkRead(b *testing.B) {
	for i := 0; i < b.N; i++ {
		store.Get(fmt.Sprintf("%d", i))
	}
}

// BenchmarkRW
func BenchmarkRW(b *testing.B) {
	for i := 0; i < b.N; i++ {
		store.Put(fmt.Sprintf("%d", i), i, 1)
		store.Get(fmt.Sprintf("%d", i))
	}
}
