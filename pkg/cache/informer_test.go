/*
Copyright 2023 The KubeVela Authors.

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

package cache

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"
)

func TestObjectCache_Get(t *testing.T) {
	type testCase[T any] struct {
		name string
		in   ObjectCache[T]
		hash string
		want *T
	}
	tests := []testCase[string]{
		{
			name: "test cache not found",
			in:   ObjectCache[string]{},
			hash: "test",
			want: nil,
		},
		{
			name: "test cache found",
			in: ObjectCache[string]{
				objects: map[string]*ObjectCacheEntry[string]{
					"test": {
						ptr:          pointer.String("test"),
						refs:         nil,
						lastAccessed: time.Now(),
					},
				},
			},
			hash: "test",
			want: pointer.String("test"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.Get(tt.hash); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Get() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestObjectCache_Add(t *testing.T) {
	type args[T any] struct {
		hash string
		obj  *T
		ref  string
	}
	type testCase[T any] struct {
		name string
		in   ObjectCache[T]
		args args[T]
		want *T
	}
	tests := []testCase[string]{
		{
			name: "test cache not found should add",
			in: ObjectCache[string]{
				objects: map[string]*ObjectCacheEntry[string]{
					"test": {
						ptr:          pointer.String("test"),
						refs:         nil,
						lastAccessed: time.Now(),
					},
				},
			},
			args: args[string]{
				hash: "test2",
				obj:  pointer.String("test2"),
			},
			want: pointer.String("test2"),
		},
		{
			name: "test cache found should update",
			in: ObjectCache[string]{
				objects: map[string]*ObjectCacheEntry[string]{
					"test": {
						ptr:          pointer.String("test"),
						refs:         nil,
						lastAccessed: time.Now(),
					},
				},
			},
			args: args[string]{
				hash: "test",
				obj:  pointer.String("test"),
			},
			want: pointer.String("test"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.Add(tt.args.hash, tt.args.obj, tt.args.ref); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Add() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestObjectCache_DeleteRef(t *testing.T) {
	type args struct {
		hash string
		ref  string
	}
	type testCase[T any] struct {
		name     string
		in       ObjectCache[T]
		args     args
		validate func(in *ObjectCache[T]) bool
	}
	tests := []testCase[string]{
		{
			name: "test cache not found should do nothing",
			in: ObjectCache[string]{
				objects: map[string]*ObjectCacheEntry[string]{
					"test": {
						ptr:          pointer.String("test"),
						refs:         nil,
						lastAccessed: time.Now(),
					},
				},
			},
			args: args{
				hash: "test2",
				ref:  "test2",
			},
			validate: func(in *ObjectCache[string]) bool {
				return len(in.objects) == 1
			},
		},
		{
			name: "test cache found should delete",
			in: ObjectCache[string]{
				objects: map[string]*ObjectCacheEntry[string]{
					"test": {
						ptr: pointer.String("test"),
					},
				},
			},
			args: args{
				hash: "test",
				ref:  "test",
			},
			validate: func(in *ObjectCache[string]) bool {
				return len(in.objects) == 0
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.DeleteRef(tt.args.hash, tt.args.ref)
			if !tt.validate(&tt.in) {
				t.Errorf("DeleteRef() = %v", tt.in)
			}
		})
	}
}

func TestObjectCache_Size(t *testing.T) {
	type testCase[T any] struct {
		name string
		in   ObjectCache[T]
		want int
	}
	tests := []testCase[string]{
		{
			name: "normal test",
			in: ObjectCache[string]{
				objects: map[string]*ObjectCacheEntry[string]{
					"test": {
						ptr: pointer.String("test"),
					},
					"test2": {
						ptr: pointer.String("test2"),
					},
				},
			},
			want: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.Size(); got != tt.want {
				t.Errorf("Size() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestObjectCache_Prune(t *testing.T) {
	in := NewObjectCache[string]()
	in.Add("test", pointer.String("test"), "test")
	in.Add("test2", pointer.String("test2"), "test")
	in.Add("test3", pointer.String("test3"), "test")
	time.Sleep(200 * time.Millisecond)
	in.Prune(100 * time.Millisecond)
	assert.True(t, in.Size() == 0)
}
