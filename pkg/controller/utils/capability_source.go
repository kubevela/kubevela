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

package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/schema"
)

// ErrGenerateSourceSchemas marks a SourceDefinition template the schema
// generator cannot read, as distinct from a failure to store its schemas.
var ErrGenerateSourceSchemas = errors.New("failed to generate the schemas")

// CapabilitySourceDefinition is the Capability struct for SourceDefinition.
type CapabilitySourceDefinition struct {
	SourceDefinition v1beta1.SourceDefinition

	CapabilityBaseDefinition
}

// NewCapabilitySourceDef creates a CapabilitySourceDefinition.
func NewCapabilitySourceDef(sourceDefinition *v1beta1.SourceDefinition) CapabilitySourceDefinition {
	return CapabilitySourceDefinition{SourceDefinition: *sourceDefinition.DeepCopy()}
}

// sourceSchemas generates the ConfigMap data of the definition: its
// parameter's OpenAPI and default UI schemas, and the OpenAPI schema of its
// `schema` when it declares one.
func (def *CapabilitySourceDefinition) sourceSchemas(ctx context.Context) (map[string]string, error) {
	schematic := def.SourceDefinition.Spec.Schematic
	if schematic == nil || schematic.CUE == nil {
		return nil, fmt.Errorf("SourceDefinition %s has no CUE template", def.SourceDefinition.Name)
	}
	ss, err := schema.GenerateSourceSchemas(ctx, schematic.CUE.Template)
	if err != nil {
		return nil, err
	}
	openAPI, ui, err := marshalSchemas(ss.Parameter)
	if err != nil {
		return nil, err
	}
	data := map[string]string{
		types.OpenapiV3JSONSchema: string(openAPI),
	}
	if len(ui) > 0 {
		data[types.DefaultUISchema] = string(ui)
	}
	if ss.Output != nil {
		out, err := json.Marshal(ss.Output)
		if err != nil {
			return nil, err
		}
		data[types.SourceOutputSchema] = string(out)
	}
	return data, nil
}

// StoreOpenAPISchema stores the schemas of the SourceDefinition in a ConfigMap
// owned by it, and in another owned by the revision revName.
func (def *CapabilitySourceDefinition) StoreOpenAPISchema(ctx context.Context, k8sClient client.Client, namespace, revName string) (string, error) {
	sourceDefinition := def.SourceDefinition
	data, err := def.sourceSchemas(ctx)
	if err != nil {
		return "", fmt.Errorf("%w of SourceDefinition %s: %w", ErrGenerateSourceSchemas, sourceDefinition.Name, err)
	}
	cmName, err := def.storeSchemaConfigMap(ctx, k8sClient, namespace, sourceDefinition.Name, typeSourceDefinition,
		copyLabels(sourceDefinition.Labels), nil, data, []metav1.OwnerReference{ownerOf(&sourceDefinition, v1beta1.SourceDefinitionGroupVersionKind)})
	if err != nil {
		return cmName, err
	}

	defRev := new(v1beta1.DefinitionRevision)
	if err = k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revName}, defRev); err != nil {
		return cmName, err
	}
	_, err = def.storeSchemaConfigMap(ctx, k8sClient, namespace, revName, typeSourceDefinition,
		copyLabels(defRev.Spec.SourceDefinition.Labels), nil, data, []metav1.OwnerReference{ownerOf(defRev, v1beta1.DefinitionRevisionGroupVersionKind)})
	return cmName, err
}

// ownerOf names the kind explicitly, since an object read from the cache
// carries no TypeMeta.
func ownerOf(obj client.Object, gvk k8sschema.GroupVersionKind) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               obj.GetName(),
		UID:                obj.GetUID(),
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}
}

func copyLabels(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels))
	for k, v := range labels {
		out[k] = v
	}
	return out
}
