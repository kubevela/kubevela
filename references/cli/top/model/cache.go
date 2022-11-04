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

// LRU type cache
type LRU struct {
	capacity int
	cache    []data
}

type data struct {
	key   string
	value interface{}
}

// NewLRUCache return the LRU type cache
func NewLRUCache(capacity int) *LRU {
	return &LRU{
		capacity: capacity,
		cache:    make([]data, 0),
	}
}

// Get return the value of the key
func (c *LRU) Get(key string) interface{} {
	index := c.exist(key)
	if index != -1 {
		c.upgrade(index)
		return c.cache[len(c.cache)-1].value
	}
	return nil
}

// Put store a new k-v pair
func (c *LRU) Put(key string, value interface{}) {
	index := c.exist(key)
	if index != -1 {
		c.cache[index].value = value
		c.upgrade(index)
	} else {
		c.cache = append(c.cache, data{key: key, value: value})
		if len(c.cache) > c.capacity {
			c.cache = c.cache[1:]
		}
	}
}

func (c *LRU) exist(key string) int {
	for i := 0; i < len(c.cache); i++ {
		if c.cache[i].key == key {
			return i
		}
	}
	return -1
}

func (c *LRU) upgrade(index int) {
	size := len(c.cache)
	item := c.cache[index]
	copy(c.cache[index:size-1], c.cache[index+1:])
	c.cache[size-1] = item
}
