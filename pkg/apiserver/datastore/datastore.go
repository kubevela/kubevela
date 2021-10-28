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

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

var (
	// ErrPrimaryEmpty Error that primary key is empty.
	ErrPrimaryEmpty = NewDBError(fmt.Errorf("entity primary is empty"))

	// ErrTableNameEmpty Error that table name is empty.
	ErrTableNameEmpty = NewDBError(fmt.Errorf("entity table name is empty"))

	// ErrNilEntity Error that entity is nil
	ErrNilEntity = NewDBError(fmt.Errorf("entity is nil"))

	// ErrRecordExist Error that entity primary key is exist
	ErrRecordExist = NewDBError(fmt.Errorf("data record is exist"))

	// ErrRecordNotExist Error that entity primary key is not exist
	ErrRecordNotExist = NewDBError(fmt.Errorf("data record is not exist"))

	// ErrIndexInvalid Error that entity index is invalid
	ErrIndexInvalid = NewDBError(fmt.Errorf("entity index is invalid"))
)

// DBError datastore error
type DBError struct {
	err error
}

func (d *DBError) Error() string {
	return d.err.Error()
}

// NewDBError new datastore error
func NewDBError(err error) error {
	return &DBError{err: err}
}

// Config datastore config
type Config struct {
	Type     string
	URL      string
	Database string
}

// Entity database data model
type Entity interface {
	SetCreateTime(time time.Time)
	SetUpdateTime(time time.Time)
	PrimaryKey() string
	TableName() string
	Index() map[string]string
}

// NewEntity Create a new object based on the input type
func NewEntity(in Entity) (Entity, error) {
	if in == nil {
		return nil, ErrNilEntity
	}
	t := reflect.TypeOf(in)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	new := reflect.New(t)
	return new.Interface().(Entity), nil
}

// ListOptions list api options
type ListOptions struct {
	Page     int
	PageSize int
}

// DataStore datastore interface
type DataStore interface {
	// add entity to database, Name() and TableName() can't return zero value.
	Add(ctx context.Context, entity Entity) error

	// batch add entity to database, Name() and TableName() can't return zero value.
	BatchAdd(ctx context.Context, entitys []Entity) error

	// Update entity to database, Name() and TableName() can't return zero value.
	Put(ctx context.Context, entity Entity) error

	// Delete entity from database, Name() and TableName() can't return zero value.
	Delete(ctx context.Context, entity Entity) error

	// Get entity from database, Name() and TableName() can't return zero value.
	Get(ctx context.Context, entity Entity) error

	// List entities from database, TableName() can't return zero value.
	List(ctx context.Context, query Entity, options *ListOptions) ([]Entity, error)

	// Count entities from database, TableName() can't return zero value.
	Count(ctx context.Context, entity Entity) (int64, error)

	// IsExist Name() and TableName() can't return zero value.
	IsExist(ctx context.Context, entity Entity) (bool, error)
}
