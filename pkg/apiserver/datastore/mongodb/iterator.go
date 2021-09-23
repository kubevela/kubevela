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

package mongodb

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
)

var _ datastore.Iterator = &Iterator{}

// Iterator mongo iterator implementation
type Iterator struct {
	cur *mongo.Cursor
}

// Close iterator close
func (i *Iterator) Close(ctx context.Context) error {
	return i.cur.Close(ctx)
}

// Next read next data
func (i *Iterator) Next(ctx context.Context) bool {
	return i.cur.Next(ctx)
}

// Decode decode data
func (i *Iterator) Decode(entity interface{}) error {
	return i.cur.Decode(entity)
}
