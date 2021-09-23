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

import "context"

// Config datastore config
type Config struct {
	URL      string
	Database string
}

// DataStore datastore interface
type DataStore interface {
	Add(ctx context.Context, kind string, entity interface{}) error

	Put(ctx context.Context, kind, name string, entity interface{}) error

	Delete(ctx context.Context, kind, name string) error

	Get(ctx context.Context, kind, name string, decodeTo interface{}) error

	// Find executes a find command and returns an iterator over the matching items.
	Find(ctx context.Context, kind string) (Iterator, error)

	FindOne(ctx context.Context, kind, name string) (Iterator, error)

	IsExist(ctx context.Context, kind, name string) (bool, error)
}

// Iterator dataset query
type Iterator interface {
	// Next gets the next item for this cursor.
	Next(ctx context.Context) bool

	// Decode will unmarshal the current item into given entity.
	Decode(entity interface{}) error

	Close(ctx context.Context)
}
