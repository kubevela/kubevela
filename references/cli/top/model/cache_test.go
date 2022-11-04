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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLRUCache(t *testing.T) {
	cache := NewLRUCache(2)
	cache.Put("app1-vela-system", 1)
	cache.Put("app2-vela-system", 2)
	value, ok := cache.Get("app1-vela-system").(int)
	assert.Equal(t, ok, true)
	assert.Equal(t, value, 1)
	cache.Put("app3-vela-system", 3)
	_, ok = cache.Get("app2-vela-system").(int)
	assert.Equal(t, ok, false)
	cache.Put("app4-vela-system", 4)
	_, ok = cache.Get("app1-vela-system").(int)
	assert.Equal(t, ok, false)
	value, ok = cache.Get("app3-vela-system").(int)
	assert.Equal(t, ok, true)
	assert.Equal(t, value, 3)
	value, ok = cache.Get("app4-vela-system").(int)
	assert.Equal(t, ok, true)
	assert.Equal(t, value, 4)
}
