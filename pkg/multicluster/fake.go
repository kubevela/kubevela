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

package multicluster

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FakeClient set default client and multicluster clients
type FakeClient struct {
	client.Client
	clients map[string]client.Client
}

// NewFakeClient create a new fake client
func NewFakeClient(baseClient client.Client) *FakeClient {
	return &FakeClient{Client: baseClient, clients: map[string]client.Client{}}
}

// AddCluster add cluster to client map
func (c *FakeClient) AddCluster(cluster string, cli client.Client) {
	c.clients[cluster] = cli
}

func (c *FakeClient) getClient(ctx context.Context) client.Client {
	cluster := ClusterNameInContext(ctx)
	if cli, exists := c.clients[cluster]; exists {
		return cli
	}
	return c.Client
}

// Get retrieves an obj for the given object key from the Kubernetes Cluster.
// obj must be a struct pointer so that obj can be updated with the response
// returned by the Server.
func (c *FakeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	return c.getClient(ctx).Get(ctx, key, obj)
}

// List retrieves list of objects for a given namespace and list options. On a
// successful call, Items field in the list will be populated with the
// result returned from the server.
func (c *FakeClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.getClient(ctx).List(ctx, list, opts...)
}

// Create saves the object obj in the Kubernetes cluster.
func (c *FakeClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return c.getClient(ctx).Create(ctx, obj, opts...)
}

// Delete deletes the given obj from Kubernetes cluster.
func (c *FakeClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return c.getClient(ctx).Delete(ctx, obj, opts...)
}

// Update updates the given obj in the Kubernetes cluster. obj must be a
// struct pointer so that obj can be updated with the content returned by the Server.
func (c *FakeClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return c.getClient(ctx).Update(ctx, obj, opts...)
}

// Patch patches the given obj in the Kubernetes cluster. obj must be a
// struct pointer so that obj can be updated with the content returned by the Server.
func (c *FakeClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return c.getClient(ctx).Patch(ctx, obj, patch, opts...)
}

// DeleteAllOf deletes all objects of the given type matching the given options.
func (c *FakeClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return c.getClient(ctx).DeleteAllOf(ctx, obj, opts...)
}
