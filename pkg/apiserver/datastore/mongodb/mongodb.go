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
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
)

var _ datastore.DataStore = &mongodb{}

type mongodb struct {
	client   *mongo.Client
	database string
}

// New new mongodb datastore instance
func New(ctx context.Context, cfg datastore.Config) (datastore.DataStore, error) {
	url := fmt.Sprintf("mongodb://%s", cfg.URL)
	clientOpts := options.Client().ApplyURI(url)
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return nil, err
	}

	m := &mongodb{
		client:   client,
		database: cfg.Database,
	}
	return m, nil
}

// Add add data model
func (m *mongodb) Add(ctx context.Context, kind string, entity interface{}) error {
	collection := m.client.Database(m.database).Collection(kind)
	_, err := collection.InsertOne(ctx, entity)
	if err != nil {
		return err
	}
	return nil
}

// Get get data model
func (m *mongodb) Get(ctx context.Context, kind, name string, decodeTo interface{}) error {
	collection := m.client.Database(m.database).Collection(kind)
	return collection.FindOne(ctx, makeNameFilter(name)).Decode(decodeTo)
}

// Put update data model
func (m *mongodb) Put(ctx context.Context, kind, name string, entity interface{}) error {
	collection := m.client.Database(m.database).Collection(kind)
	_, err := collection.UpdateOne(ctx, makeNameFilter(name), makeEntityUpdate(entity))
	if err != nil {
		return err
	}
	return nil
}

// Find find data model
func (m *mongodb) Find(ctx context.Context, kind string) (datastore.Iterator, error) {
	collection := m.client.Database(m.database).Collection(kind)
	// bson.D{{}} specifies 'all documents'
	filter := bson.D{}
	cur, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &Iterator{cur: cur}, nil
}

// FindOne find one data model
func (m *mongodb) FindOne(ctx context.Context, kind, name string) (datastore.Iterator, error) {
	collection := m.client.Database(m.database).Collection(kind)
	filter := bson.M{"name": name}
	cur, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &Iterator{cur: cur}, nil
}

// IsExist determine whether data exists.
func (m *mongodb) IsExist(ctx context.Context, kind, name string) (bool, error) {
	collection := m.client.Database(m.database).Collection(kind)
	err := collection.FindOne(ctx, makeNameFilter(name)).Err()
	if err == mongo.ErrNoDocuments {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

// Delete delete data
func (m *mongodb) Delete(ctx context.Context, kind, name string) error {
	collection := m.client.Database(m.database).Collection(kind)
	// delete at most one document in which the "name" field is "Bob" or "bob"
	// specify the SetCollation option to provide a collation that will ignore case for string comparisons
	opts := options.Delete().SetCollation(&options.Collation{
		Locale:    "en_US",
		Strength:  1,
		CaseLevel: false,
	})
	_, err := collection.DeleteOne(ctx, makeNameFilter(name), opts)
	return err
}

func makeNameFilter(name string) bson.D {
	return bson.D{{Key: "name", Value: name}}
}

func makeEntityUpdate(entity interface{}) bson.M {
	return bson.M{"$set": entity}
}
