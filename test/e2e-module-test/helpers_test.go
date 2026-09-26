/*
Copyright 2026 The KubeVela Authors.

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

package controllers_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// applyManifestFile reads a multi-document YAML/JSON file and creates each
// object, ignoring AlreadyExists so a re-run of a spec that left its fixture
// behind does not fail. The kinds handled are exactly the ones this suite's
// fixtures use.
func applyManifestFile(ctx context.Context, k8sClient client.Client, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	decoder := yaml.NewYAMLOrJSONDecoder(bufio.NewReader(f), 4096)
	for {
		raw := map[string]any{}
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if len(raw) == 0 {
			continue
		}
		obj, err := decodeKubeObject(raw)
		if err != nil {
			return err
		}
		if err := k8sClient.Create(ctx, obj); err != nil && !apierrIsAlreadyExists(err) {
			return err
		}
	}
}

func decodeKubeObject(raw map[string]any) (client.Object, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	kind, _ := raw["kind"].(string)
	var obj client.Object
	switch kind {
	case "Namespace":
		obj = &corev1.Namespace{}
	case "ConfigMap":
		obj = &corev1.ConfigMap{}
	case "Secret":
		obj = &corev1.Secret{}
	case "Service":
		obj = &corev1.Service{}
	case "Deployment":
		obj = &appsv1.Deployment{}
	case "Application":
		obj = &v1beta1.Application{}
	default:
		return nil, fmt.Errorf("decodeKubeObject: unsupported kind %q", kind)
	}
	if err := json.Unmarshal(b, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// apierrIsAlreadyExists locally inlines the kerrors.IsAlreadyExists check to
// avoid bringing in another import just for one branch.
func apierrIsAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "already exists")
}
