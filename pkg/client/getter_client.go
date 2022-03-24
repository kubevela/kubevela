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

package client

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetterClient override the original client's get function
type GetterClient struct {
	client.Client
	Getter func(ctx context.Context, key client.ObjectKey, obj client.Object) error
}

// Get resource by overridden getter
func (c GetterClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	return c.Getter(ctx, key, obj)
}
