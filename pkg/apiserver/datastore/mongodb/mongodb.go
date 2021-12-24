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
	"errors"
	"fmt"
	"time"

	"cuelang.org/go/pkg/strings"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
)

type mongodb struct {
	client   *mongo.Client
	database string
}

// New new mongodb datastore instance
func New(ctx context.Context, cfg datastore.Config) (datastore.DataStore, error) {
	if !strings.HasPrefix(cfg.URL, "mongodb://") {
		cfg.URL = fmt.Sprintf("mongodb://%s", cfg.URL)
	}
	clientOpts := options.Client().ApplyURI(cfg.URL)
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
func (m *mongodb) Add(ctx context.Context, entity datastore.Entity) error {
	if entity.PrimaryKey() == "" {
		return datastore.ErrPrimaryEmpty
	}
	if entity.TableName() == "" {
		return datastore.ErrTableNameEmpty
	}
	entity.SetCreateTime(time.Now())
	if err := m.Get(ctx, entity); err == nil {
		return datastore.ErrRecordExist
	}
	collection := m.client.Database(m.database).Collection(entity.TableName())
	_, err := collection.InsertOne(ctx, entity)
	if err != nil {
		return datastore.NewDBError(err)
	}
	return nil
}

// BatchAdd batch add entity, this operation has some atomicity.
func (m *mongodb) BatchAdd(ctx context.Context, entitys []datastore.Entity) error {
	donotRollback := make(map[string]int)
	for i, saveEntity := range entitys {
		if err := m.Add(ctx, saveEntity); err != nil {
			if errors.Is(err, datastore.ErrRecordExist) {
				donotRollback[saveEntity.PrimaryKey()] = 1
			}
			for _, deleteEntity := range entitys[:i] {
				if _, exit := donotRollback[deleteEntity.PrimaryKey()]; !exit {
					if err := m.Delete(ctx, deleteEntity); err != nil {
						if !errors.Is(err, datastore.ErrRecordNotExist) {
							log.Logger.Errorf("rollback delete component failure %w", err)
						}
					}
				}
			}
			return datastore.NewDBError(fmt.Errorf("save components occur error, %w", err))
		}
	}
	return nil
}

// Get get data model
func (m *mongodb) Get(ctx context.Context, entity datastore.Entity) error {
	if entity.PrimaryKey() == "" {
		return datastore.ErrPrimaryEmpty
	}
	if entity.TableName() == "" {
		return datastore.ErrTableNameEmpty
	}
	collection := m.client.Database(m.database).Collection(entity.TableName())
	if err := collection.FindOne(ctx, makeNameFilter(entity.PrimaryKey())).Decode(entity); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return datastore.ErrRecordNotExist
		}
		return datastore.NewDBError(err)
	}
	return nil
}

// Put update data model
func (m *mongodb) Put(ctx context.Context, entity datastore.Entity) error {
	if entity.PrimaryKey() == "" {
		return datastore.ErrPrimaryEmpty
	}
	if entity.TableName() == "" {
		return datastore.ErrTableNameEmpty
	}
	entity.SetUpdateTime(time.Now())
	collection := m.client.Database(m.database).Collection(entity.TableName())
	_, err := collection.UpdateOne(ctx, makeNameFilter(entity.PrimaryKey()), makeEntityUpdate(entity))
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return datastore.ErrRecordNotExist
		}
		return datastore.NewDBError(err)
	}
	return nil
}

// IsExist determine whether data exists.
func (m *mongodb) IsExist(ctx context.Context, entity datastore.Entity) (bool, error) {
	if entity.PrimaryKey() == "" {
		return false, datastore.ErrPrimaryEmpty
	}
	if entity.TableName() == "" {
		return false, datastore.ErrTableNameEmpty
	}
	collection := m.client.Database(m.database).Collection(entity.TableName())
	err := collection.FindOne(ctx, makeNameFilter(entity.PrimaryKey())).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	} else if err != nil {
		return false, datastore.NewDBError(err)
	}

	return true, nil
}

// Delete delete data
func (m *mongodb) Delete(ctx context.Context, entity datastore.Entity) error {
	if entity.PrimaryKey() == "" {
		return datastore.ErrPrimaryEmpty
	}
	if entity.TableName() == "" {
		return datastore.ErrTableNameEmpty
	}
	// check entity is exist
	if err := m.Get(ctx, entity); err != nil {
		return err
	}
	collection := m.client.Database(m.database).Collection(entity.TableName())
	// delete at most one document in which the "name" field is "Bob" or "bob"
	// specify the SetCollation option to provide a collation that will ignore case for string comparisons
	opts := options.Delete().SetCollation(&options.Collation{
		Locale:    "en_US",
		Strength:  1,
		CaseLevel: false,
	})
	_, err := collection.DeleteOne(ctx, makeNameFilter(entity.PrimaryKey()), opts)
	if err != nil {
		log.Logger.Errorf("delete document failure %w", err)
		return datastore.NewDBError(err)
	}
	return nil
}

func _applyFilterOptions(filter bson.D, filterOptions datastore.FilterOptions) bson.D {
	if len(filterOptions.Queries) > 0 {
		for _, queryOp := range filterOptions.Queries {
			filter = append(filter, bson.E{Key: strings.ToLower(queryOp.Key), Value: bsonx.Regex(".*"+queryOp.Query+".*", "s")})
		}
	}
	return filter
}

// List list entity function
func (m *mongodb) List(ctx context.Context, entity datastore.Entity, op *datastore.ListOptions) ([]datastore.Entity, error) {
	if entity.TableName() == "" {
		return nil, datastore.ErrTableNameEmpty
	}
	collection := m.client.Database(m.database).Collection(entity.TableName())
	// bson.D{{}} specifies 'all documents'
	filter := bson.D{}
	if entity.Index() != nil {
		for k, v := range entity.Index() {
			filter = append(filter, bson.E{
				Key:   k,
				Value: v,
			})
		}
	}
	if op != nil && len(op.Queries) > 0 {
		filter = _applyFilterOptions(filter, op.FilterOptions)
	}
	var findOptions options.FindOptions
	if op != nil && op.PageSize > 0 && op.Page > 0 {
		findOptions.SetSkip(int64(op.PageSize * (op.Page - 1)))
		findOptions.SetLimit(int64(op.PageSize))
	}
	if op != nil && len(op.SortBy) > 0 {
		_d := bson.D{}
		for _, sortOp := range op.SortBy {
			key := strings.ToLower(sortOp.Key)
			if key == "createtime" || key == "updatetime" {
				key = "basemodel." + key
			}
			_d = append(_d, bson.E{Key: key, Value: int(sortOp.Order)})
		}
		findOptions.SetSort(_d)
	}
	cur, err := collection.Find(ctx, filter, &findOptions)
	if err != nil {
		return nil, datastore.NewDBError(err)
	}
	defer func() {
		if err := cur.Close(ctx); err != nil {
			log.Logger.Warnf("close mongodb cursor failure %s", err.Error())
		}
	}()
	var list []datastore.Entity
	for cur.Next(ctx) {
		item, err := datastore.NewEntity(entity)
		if err != nil {
			return nil, datastore.NewDBError(err)
		}
		if err := cur.Decode(item); err != nil {
			return nil, datastore.NewDBError(fmt.Errorf("decode entity failure %w", err))
		}
		list = append(list, item)
	}
	if err := cur.Err(); err != nil {
		return nil, datastore.NewDBError(err)
	}
	return list, nil
}

// Count counts entities
func (m *mongodb) Count(ctx context.Context, entity datastore.Entity, filterOptions *datastore.FilterOptions) (int64, error) {
	if entity.TableName() == "" {
		return 0, datastore.ErrTableNameEmpty
	}
	collection := m.client.Database(m.database).Collection(entity.TableName())
	filter := bson.D{}
	if entity.Index() != nil {
		for k, v := range entity.Index() {
			filter = append(filter, bson.E{
				Key:   k,
				Value: v,
			})
		}
	}
	if filterOptions != nil && len(filterOptions.Queries) > 0 {
		filter = _applyFilterOptions(filter, *filterOptions)
	}
	count, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return 0, datastore.NewDBError(err)
	}
	return count, nil
}

func makeNameFilter(name string) bson.D {
	return bson.D{{Key: "name", Value: name}}
}

func makeEntityUpdate(entity interface{}) bson.M {
	return bson.M{"$set": entity}
}
