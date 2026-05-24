/*
Copyright 2024 The KubeVela Authors.

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
	"testing"
	"time"
)

func TestLRUEviction(t *testing.T) {
	store := NewMemoryCacheStoreWithMaxSize(context.TODO(), 2)
	store.Put("a", "val-a", 0)
	store.Put("b", "val-b", 0)
	store.Put("c", "val-c", 0)
	if store.Get("a") != nil {
		t.Error("expected a to be evicted")
	}
	if store.Get("b") != "val-b" {
		t.Error("expected b to exist")
	}
	if store.Get("c") != "val-c" {
		t.Error("expected c to exist")
	}
}

func TestLRUGetUpdatesOrder(t *testing.T) {
	store := NewMemoryCacheStoreWithMaxSize(context.TODO(), 2)
	store.Put("a", "val-a", 0)
	store.Put("b", "val-b", 0)
	store.Get("a")
	store.Put("c", "val-c", 0)
	if store.Get("b") != nil {
		t.Error("expected b to be evicted")
	}
	if store.Get("a") != "val-a" {
		t.Error("expected a to exist")
	}
	if store.Get("c") != "val-c" {
		t.Error("expected c to exist")
	}
}

func TestLRUDuplicatePutNoEviction(t *testing.T) {
	store := NewMemoryCacheStoreWithMaxSize(context.TODO(), 2)
	store.Put("a", "val-a", 0)
	store.Put("b", "val-b", 0)
	store.Put("a", "val-a-new", 0)
	store.Put("c", "val-c", 0)
	if store.Get("b") != nil {
		t.Error("expected b to be evicted")
	}
	if store.Get("a") != "val-a-new" {
		t.Errorf("expected val-a-new, got %v", store.Get("a"))
	}
	if store.Get("c") != "val-c" {
		t.Error("expected c to exist")
	}
}

func TestLRUMaxSizeZeroIsUnbounded(t *testing.T) {
	store := NewMemoryCacheStoreWithMaxSize(context.TODO(), 0)
	for i := 0; i < 100; i++ {
		store.Put(i, i, 0)
	}
	if store.Get(0) != 0 {
		t.Error("expected key 0 to exist in unbounded store")
	}
	if store.Get(99) != 99 {
		t.Error("expected key 99 to exist in unbounded store")
	}
}

func TestLRUExpiredEntryNotReturned(t *testing.T) {
	store := NewMemoryCacheStoreWithMaxSize(context.TODO(), 5)
	store.Put("x", "val-x", 1*time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	if store.Get("x") != nil {
		t.Error("expected expired entry to return nil")
	}
}

func TestLRUDelete(t *testing.T) {
	store := NewMemoryCacheStoreWithMaxSize(context.TODO(), 3)
	store.Put("a", "val-a", 0)
	store.Delete("a")
	if store.Get("a") != nil {
		t.Error("expected deleted key to return nil")
	}
}
