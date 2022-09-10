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
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/workflow/pkg/tasks/template"
	wfTypes "github.com/kubevela/workflow/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
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

// WorkflowStepLoader load workflowStep task definition template.
type WorkflowStepLoader struct {
	loadCapabilityDefinition func(ctx context.Context, capName string) (*appfile.Template, error)
}

// LoadTemplate gets the workflowStep definition.
func (loader *WorkflowStepLoader) LoadTemplate(ctx context.Context, name string) (string, error) {
	files, err := templateFS.ReadDir(templateDir)
	if err != nil {
		return "", err
	}
	if name == wfTypes.WorkflowStepTypeApplyComponent {
		name = wfTypes.WorkflowStepTypeBuiltinApplyComponent
	}
	staticFilename := name + ".cue"
	for _, file := range files {
		if staticFilename == file.Name() {
			fileName := fmt.Sprintf("%s/%s", templateDir, file.Name())
			content, err := templateFS.ReadFile(fileName)
			return string(content), err
		}
	}

	templ, err := loader.loadCapabilityDefinition(ctx, name)
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
func NewWorkflowStepTemplateLoader(client client.Client, dm discoverymapper.DiscoveryMapper) template.Loader {
	return &WorkflowStepLoader{
		loadCapabilityDefinition: func(ctx context.Context, capName string) (*appfile.Template, error) {
			return appfile.LoadTemplate(ctx, dm, client, capName, types.TypeWorkflowStep)
		},
	}
}

// NewWorkflowStepTemplateRevisionLoader create a task template loader from ApplicationRevision.
func NewWorkflowStepTemplateRevisionLoader(rev *v1beta1.ApplicationRevision, dm discoverymapper.DiscoveryMapper) template.Loader {
	return &WorkflowStepLoader{
		loadCapabilityDefinition: func(ctx context.Context, capName string) (*appfile.Template, error) {
			return appfile.LoadTemplateFromRevision(capName, types.TypeWorkflowStep, rev, dm)
		},
	}
}

// ViewLoader load view task definition template.
type ViewLoader struct {
	client    client.Client
	namespace string
}

// LoadTemplate gets the workflowStep definition.
func (loader *ViewLoader) LoadTemplate(ctx context.Context, name string) (string, error) {
	cm := new(corev1.ConfigMap)
	cmKey := client.ObjectKey{Name: name, Namespace: loader.namespace}
	if err := loader.client.Get(ctx, cmKey, cm); err != nil {
		return "", errors.Wrapf(err, "fail to get view template %v from configMap", cmKey)
	}
	return cm.Data["template"], nil
}

// NewViewTemplateLoader create a view task template loader.
func NewViewTemplateLoader(client client.Client, namespace string) template.Loader {
	return &ViewLoader{
		client:    client,
		namespace: namespace,
	}
}

// EchoLoader will load data from input as it is.
type EchoLoader struct {
}

// LoadTemplate gets the echo content exactly what it is .
func (ll *EchoLoader) LoadTemplate(_ context.Context, content string) (string, error) {
	return content, nil
}
