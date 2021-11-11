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

package template

import (
	"context"
	"embed"
	"path/filepath"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
)

var (
	//go:embed static
	templateFS embed.FS
)

const (
	templateDir = "static"
)

// Loader load task definition template.
type Loader interface {
	LoadTaskTemplate(ctx context.Context, name string) (string, error)
}

// WorkflowStepLoader load workflowStep task definition template.
type WorkflowStepLoader struct {
	client client.Client
	dm     discoverymapper.DiscoveryMapper
}

// LoadTaskTemplate gets the workflowStep definition.
func (loader *WorkflowStepLoader) LoadTaskTemplate(ctx context.Context, name string) (string, error) {
	files, err := templateFS.ReadDir(templateDir)
	if err != nil {
		return "", err
	}

	staticFilename := name + ".cue"
	for _, file := range files {
		if staticFilename == file.Name() {
			content, err := templateFS.ReadFile(filepath.Join(templateDir, file.Name()))
			return string(content), err
		}
	}

	templ, err := appfile.LoadTemplate(ctx, loader.dm, loader.client, name, types.TypeWorkflowStep)
	if err != nil {
		return "", err
	}
	schematic := templ.WorkflowStepDefinition.Spec.Schematic
	if schematic != nil && schematic.CUE != nil {
		return schematic.CUE.Template, nil
	}

	return "", errors.New("custom workflowStep only support cue")
}

// NewWorkflowStepTemplateLoader create a task template loader.
func NewWorkflowStepTemplateLoader(client client.Client, dm discoverymapper.DiscoveryMapper) Loader {
	return &WorkflowStepLoader{
		client: client,
		dm:     dm,
	}
}

// ViewLoader load view task definition template.
type ViewLoader struct {
	client    client.Client
	namespace string
}

// LoadTaskTemplate gets the workflowStep definition.
func (loader *ViewLoader) LoadTaskTemplate(ctx context.Context, name string) (string, error) {
	cm := new(corev1.ConfigMap)
	cmKey := client.ObjectKey{Name: name, Namespace: loader.namespace}
	if err := loader.client.Get(ctx, cmKey, cm); err != nil {
		return "", errors.Wrapf(err, "fail to get view template %v from configMap", cmKey)
	}
	return cm.Data["template"], nil
}

// NewViewTemplateLoader create a view task template loader.
func NewViewTemplateLoader(client client.Client, namespace string) Loader {
	return &ViewLoader{
		client:    client,
		namespace: namespace,
	}
}
