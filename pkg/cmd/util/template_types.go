/*


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

package util

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TemplateSpec defines the desired state of Template
type Template struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Alias            string                    `json:"alias,omitempty"`
	Object           unstructured.Unstructured `json:"object,omitempty"`
	LastCommandParam string                    `json:"lastCommandParam,omitempty"`
	Parameters       []Parameter               `json:"parameters,omitempty"`
}

type Parameter struct {
	Name       string   `json:"name"`
	Short      string   `json:"short,omitempty"`
	Required   bool     `json:"required,omitempty"`
	FieldPaths []string `json:"fieldPaths"`
	Default    string   `json:"default,omitempty"`
	Usage      string   `json:"usage,omitempty"`
	Type       string   `json:"type,omitempty"`
}

// ConvertTemplateJson2Object convert spec.extension to object
func ConvertTemplateJson2Object(in *runtime.RawExtension) (Template, error) {
	var t Template
	var extension Template
	if in == nil {
		return t, fmt.Errorf("extension field is nil")
	}
	if in.Raw == nil {
		return t, fmt.Errorf("template object is nil")
	}
	err := json.Unmarshal(in.Raw, &extension)
	if err == nil {
		t = extension
	}

	return t, err
}
