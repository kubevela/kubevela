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

package common

import (
	"context"

	"github.com/oam-dev/kubevela/references/apiserver/apis"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RetrieveComponent will get component status
func RetrieveComponent(ctx context.Context, c client.Reader, applicationName, componentName,
	namespace string) (apis.ComponentMeta, error) {
	var componentMeta apis.ComponentMeta
	applicationMeta, err := RetrieveApplicationStatusByName(ctx, c, applicationName, namespace)
	if err != nil {
		return componentMeta, err
	}

	for _, com := range applicationMeta.Components {
		if com.Name != componentName {
			continue
		}
		return com, nil
	}
	return componentMeta, nil
}
