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

package context

import (
	"fmt"
	"sync"

	v1 "k8s.io/api/core/v1"
)

var (
	// EnableInMemoryContext optimize workflow context storage by storing it in memory instead of etcd
	EnableInMemoryContext = false
)

type inMemoryContextStorage struct {
	mu       sync.Mutex
	contexts map[string]*v1.ConfigMap
}

// MemStore store in-memory context
var MemStore = &inMemoryContextStorage{
	contexts: map[string]*v1.ConfigMap{},
}

func (o *inMemoryContextStorage) getKey(cm *v1.ConfigMap) string {
	ns := cm.GetNamespace()
	if ns == "" {
		ns = "default"
	}
	name := cm.GetName()
	return ns + "/" + name
}

func (o *inMemoryContextStorage) GetOrCreateInMemoryContext(cm *v1.ConfigMap) {
	if obj := o.GetInMemoryContext(cm.Name, cm.Namespace); obj != nil {
		obj.DeepCopyInto(cm)
	} else {
		o.CreateInMemoryContext(cm)
	}
}

func (o *inMemoryContextStorage) GetInMemoryContext(name, ns string) *v1.ConfigMap {
	return o.contexts[ns+"/"+name]
}

func (o *inMemoryContextStorage) CreateInMemoryContext(cm *v1.ConfigMap) {
	o.mu.Lock()
	defer o.mu.Unlock()
	cm.Data = map[string]string{}
	o.contexts[o.getKey(cm)] = cm
}

func (o *inMemoryContextStorage) UpdateInMemoryContext(cm *v1.ConfigMap) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.contexts[o.getKey(cm)] = cm
}

func (o *inMemoryContextStorage) DeleteInMemoryContext(appName string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	key := fmt.Sprintf("workflow-%s-context", appName)
	delete(o.contexts, key)
}
