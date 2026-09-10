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

package v1alpha1

import (
	"reflect"

	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// Package type metadata.
const (
	Group   = "config.oam.dev"
	Version = "v1alpha1"
)

var (
	// SchemeGroupVersion is group version used to register these objects
	SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: Version}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: SchemeGroupVersion}

	// AddToScheme is a global function that registers this API group & version to a scheme
	AddToScheme = SchemeBuilder.AddToScheme
)

// ConfigTemplate type metadata.
var (
	ConfigTemplateKind             = reflect.TypeOf(ConfigTemplate{}).Name()
	ConfigTemplateGroupKind        = schema.GroupKind{Group: Group, Kind: ConfigTemplateKind}.String()
	ConfigTemplateKindAPIVersion   = ConfigTemplateKind + "." + SchemeGroupVersion.String()
	ConfigTemplateGroupVersionKind = SchemeGroupVersion.WithKind(ConfigTemplateKind)
	ConfigTemplateGVR              = SchemeGroupVersion.WithResource("configtemplates")
)

// Config type metadata.
var (
	ConfigKind             = reflect.TypeOf(Config{}).Name()
	ConfigGroupKind        = schema.GroupKind{Group: Group, Kind: ConfigKind}.String()
	ConfigKindAPIVersion   = ConfigKind + "." + SchemeGroupVersion.String()
	ConfigGroupVersionKind = SchemeGroupVersion.WithKind(ConfigKind)
	ConfigGVR              = SchemeGroupVersion.WithResource("configs")
)

func init() {
	SchemeBuilder.Register(&ConfigTemplate{}, &ConfigTemplateList{})
	SchemeBuilder.Register(&Config{}, &ConfigList{})
	_ = SchemeBuilder.AddToScheme(k8sscheme.Scheme)
}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}
