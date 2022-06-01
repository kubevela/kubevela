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
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
)

func TestMongodb(t *testing.T) {
	_, err := New(context.TODO(), datastore.Config{
		URL:      "mongodb://localhost:27017",
		Database: "kubevela",
	})
	if err != nil {
		t.Fatal(err)
	}

	RegisterFailHandler(Fail)
	RunSpecs(t, "Mongodb Suite")
}
