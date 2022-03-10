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
	"strings"

	"github.com/pkg/errors"
)

const (
	separator = "/"
)

// ParseTarget parse target into cluster and namespace
// TODO: in future, we might have Target CRD where we should allow user to get target from existing CR object
func ParseTarget(target string) (cluster string, namespace string, err error) {
	parts := strings.Split(target, separator)
	if len(parts) != 2 {
		return "", "", errors.Errorf("invalid target %s", target)
	}
	cluster = ClusterLocalName
	if parts[0] != "" {
		cluster = parts[0]
	}
	namespace = parts[1]
	return cluster, namespace, nil
}
