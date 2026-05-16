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
	"sync"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestMemoryCacheIsExpired(t *testing.T) {
	g := NewWithT(t)
	g.Expect(newMemoryCache("test", 10*time.Hour).IsExpired(time.Now())).Should(BeFalse())
	g.Expect(newMemoryCache("test", 0).IsExpired(time.Now())).Should(BeFalse())
	g.Expect(newMemoryCache("test", -1*time.Second).IsExpired(time.Now())).Should(BeFalse())
}

func TestMemoryCacheStoreTTL(t *testing.T) {
	g := NewWithT(t)
	store := NewMemoryCacheStore(context.TODO(), WithSweepInterval(50*time.Millisecond))
	defer store.Close()
	store.Put("test", "test data", time.Second*2)
	store.Put("test2", "test data", 0)
	store.Put("test3", "test data", -1)
	time.Sleep(3 * time.Second)
	g.Expect(store.Get("test")).Should(BeNil())
	g.Expect(store.Get("test2")).Should(Equal("test data"))
	g.Expect(store.Get("test3")).Should(Equal("test data"))
}

func TestMemoryCacheStoreDeleteKey(t *testing.T) {
	g := NewWithT(t)
	store := NewMemoryCacheStore(context.TODO())
	defer store.Close()
	store.Put("test", "test data", time.Minute*2)
	store.Delete("test")
	g.Expect(store.Get("test")).Should(BeNil())
}

func TestMemoryCacheStoreShutdown(t *testing.T) {
	g := NewWithT(t)
	store := NewMemoryCacheStore(context.TODO(), WithSweepInterval(50*time.Millisecond))
	start := time.Now()
	store.Close()
	g.Expect(time.Since(start)).Should(BeNumerically("<", 2*time.Second))
}

func TestMemoryCacheStoreTTLBoundaries(t *testing.T) {
	g := NewWithT(t)
	store := NewMemoryCacheStore(context.TODO(), WithSweepInterval(50*time.Millisecond))
	defer store.Close()

	store.Put("soon", "value", 100*time.Millisecond)
	store.Put("keep", "value", 0)

	g.Eventually(func() interface{} {
		return store.Get("soon")
	}, 5*time.Second, 50*time.Millisecond).Should(BeNil())

	g.Expect(store.Get("keep")).Should(Equal("value"))
}

func TestMemoryCacheStoreIdempotentClose(t *testing.T) {
	g := NewWithT(t)
	store := NewMemoryCacheStore(context.TODO())
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store.Close()
		}()
	}
	wg.Wait()

	g.Expect(store.Get("any")).Should(BeNil())
	store.Put("key", "val", 0)
	g.Expect(store.Get("key")).Should(Equal("val"))
}

func TestMemoryCacheStoreConcurrency(t *testing.T) {
	store := NewMemoryCacheStore(context.TODO(), WithSweepInterval(100*time.Millisecond))
	defer store.Close()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", id)
			for j := 0; j < 50; j++ {
				store.Put(key, j, time.Minute)
				store.Get(key)
				store.Delete(key + "-del")
			}
		}(i)
	}
	wg.Wait()
}

func BenchmarkMemoryCacheWrite(b *testing.B) {
	store := NewMemoryCacheStore(context.TODO())
	defer store.Close()

	for i := 0; i < b.N; i++ {
		store.Put(fmt.Sprintf("%d", i), i, 0)
	}
}

func BenchmarkMemoryCacheRead(b *testing.B) {
	store := NewMemoryCacheStore(context.TODO())
	defer store.Close()

	for i := 0; i < 1000; i++ {
		store.Put(fmt.Sprintf("%d", i), i, time.Hour)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Get(fmt.Sprintf("%d", i%1000))
	}
}

func BenchmarkMemoryCacheRW(b *testing.B) {
	store := NewMemoryCacheStore(context.TODO())
	defer store.Close()

	for i := 0; i < b.N; i++ {
		store.Put(fmt.Sprintf("%d", i), i, 1)
		store.Get(fmt.Sprintf("%d", i))
	}
}
