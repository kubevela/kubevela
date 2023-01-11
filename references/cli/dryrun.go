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

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	wfv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/workflow/step"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	corev1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile/dryrun"
	pkgdef "github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// DryRunCmdOptions contains dry-run cmd options
type DryRunCmdOptions struct {
	cmdutil.IOStreams
	ApplicationFiles     []string
	DefinitionFile       string
	OfflineMode          bool
	MergeStandaloneFiles bool
}

// NewDryRunCommand creates `dry-run` command
func NewDryRunCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	o := &DryRunCmdOptions{IOStreams: ioStreams}
	cmd := &cobra.Command{
		Use:                   "dry-run",
		DisableFlagsInUseLine: true,
		Short:                 "Dry Run an application, and output the K8s resources as result to stdout",
		Long: `Dry-run application locally, render the Kubernetes resources as result to stdout.
	vela dry-run -d /definition/directory/or/file/ -f /path/to/app.yaml

You can also specify a remote url for app:
	vela dry-run -d /definition/directory/or/file/ -f https://remote-host/app.yaml

And more, you can specify policy and workflow with application file:
	vela dry-run -d /definition/directory/or/file/ -f /path/to/app.yaml -f /path/to/policy.yaml -f /path/to/workflow.yaml, OR
	vela dry-run -d /definition/directory/or/file/ -f /path/to/app.yaml,/path/to/policy.yaml,/path/to/workflow.yaml

Additionally, if the provided policy and workflow files are not referenced by application file, warning message will show up
and those files will be ignored. You can use "merge" flag to make those standalone files effective:
	vela dry-run -d /definition/directory/or/file/ -f /path/to/app.yaml,/path/to/policy.yaml,/path/to/workflow.yaml --merge

Limitation:
	1. Only support one object per file(yaml) for "-f" flag. More support will be added in the future improvement.
	2. Dry Run with policy and workflow will only take override/topology policies and deploy workflow step into considerations. Other workflow step will be ignored.
`,
		Example: `
# dry-run application 
vela dry-run -f app.yaml

# dry-run application with policy and workflow
vela dry-run -f app.yaml -f policy.yaml -f workflow.yaml
`,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				// We need to return an error only if not in offline mode
				if !o.OfflineMode {
					return err
				}

				// Set the namespace to default to match behavior of `GetFlagNamespaceOrEnv`
				namespace = types.DefaultAppNamespace
			}

			buff, err := DryRunApplication(o, c, namespace)
			if err != nil {
				return err
			}
			o.Info(buff.String())
			return nil
		},
	}

	cmd.Flags().StringSliceVarP(&o.ApplicationFiles, "file", "f", []string{"app.yaml"}, "application related file names")
	cmd.Flags().StringVarP(&o.DefinitionFile, "definition", "d", "", "specify a definition file or directory, it will only be used in dry-run rather than applied to K8s cluster")
	cmd.Flags().BoolVar(&o.OfflineMode, "offline", false, "Run `dry-run` in offline / local mode, all validation steps will be skipped")
	cmd.Flags().BoolVar(&o.MergeStandaloneFiles, "merge", false, "Merge standalone files to produce dry-run results")
	addNamespaceAndEnvArg(cmd)
	cmd.SetOut(ioStreams.Out)
	return cmd
}

// DryRunApplication will dry-run an application and return the render result
func DryRunApplication(cmdOption *DryRunCmdOptions, c common.Args, namespace string) (bytes.Buffer, error) {
	var err error
	var buff = bytes.Buffer{}

	objs := []oam.Object{}
	if cmdOption.DefinitionFile != "" {
		objs, err = ReadDefinitionsFromFile(cmdOption.DefinitionFile)
		if err != nil {
			return buff, err
		}
	}

	// Load a kubernetes client
	var newClient client.Client
	if cmdOption.OfflineMode {
		// We will load a fake client with all the objects present in the definitions file preloaded
		newClient, err = c.GetFakeClient(objs)
	} else {
		// Load an actual client here
		newClient, err = c.GetClient()
	}
	if err != nil {
		return buff, err
	}

	pd, err := c.GetPackageDiscover()
	if err != nil {
		return buff, err
	}
	config, err := c.GetConfig()
	if err != nil {
		return buff, err
	}
	dm, err := discoverymapper.New(config)
	if err != nil {
		return buff, err
	}

	dryRunOpt := dryrun.NewDryRunOption(newClient, config, dm, pd, objs, false)
	ctx := oamutil.SetNamespaceInCtx(context.Background(), namespace)

	// Perform validation only if not in offline mode
	if !cmdOption.OfflineMode {
		for _, applicationFile := range cmdOption.ApplicationFiles {
			err = dryRunOpt.ValidateApp(ctx, applicationFile)
			if err != nil {
				return buff, errors.WithMessagef(err, "validate application: %s by dry-run", applicationFile)
			}
		}
	}

	app, err := readApplicationFromFiles(cmdOption, &buff)
	if err != nil {
		return buff, errors.WithMessagef(err, "read application files: %s", cmdOption.ApplicationFiles)
	}

	err = dryRunOpt.ExecuteDryRunWithPolicies(ctx, app, &buff)
	if err != nil {
		return buff, err
	}
	return buff, nil
}

func readObj(path string) (oam.Object, error) {
	switch {
	case strings.HasSuffix(path, ".cue"):
		def := pkgdef.Definition{Unstructured: unstructured.Unstructured{}}
		defBytes, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return nil, err
		}
		if err := def.FromCUEString(string(defBytes), nil); err != nil {
			return nil, errors.Wrapf(err, "failed to parse CUE for definition")
		}
		obj := &unstructured.Unstructured{Object: def.UnstructuredContent()}
		return obj, nil
	default:
		obj := &unstructured.Unstructured{}
		err := common.ReadYamlToObject(path, obj)
		if err != nil {
			return nil, err
		}
		return obj, nil
	}
}

// ReadDefinitionsFromFile will read objects from file or dir in the format of yaml
func ReadDefinitionsFromFile(path string) ([]oam.Object, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		obj, err := readObj(path)
		if err != nil {
			return nil, err
		}
		return []oam.Object{obj}, nil
	}

	var objs []oam.Object
	//nolint:gosec
	fis, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	for _, fi := range fis {
		if fi.IsDir() {
			continue
		}
		fileType := filepath.Ext(fi.Name())
		if fileType != ".yaml" && fileType != ".yml" && fileType != ".cue" {
			continue
		}
		obj, err := readObj(filepath.Join(path, fi.Name()))
		if err != nil {
			return nil, err
		}
		objs = append(objs, obj)
	}
	return objs, nil
}

func readApplicationFromFile(filename string) (*corev1beta1.Application, error) {
	fileContent, err := utils.ReadRemoteOrLocalPath(filename, true)
	if err != nil {
		return nil, err
	}

	fileType := filepath.Ext(filename)
	switch fileType {
	case ".yaml", ".yml":
		fileContent, err = yaml.YAMLToJSON(fileContent)
		if err != nil {
			return nil, err
		}
	}

	app := new(corev1beta1.Application)
	err = json.Unmarshal(fileContent, app)
	return app, err
}

func readApplicationFromFiles(cmdOption *DryRunCmdOptions, buff *bytes.Buffer) (*corev1beta1.Application, error) {
	var app *corev1beta1.Application
	var policies []*v1alpha1.Policy
	var wf *wfv1alpha1.Workflow
	policyNameMap := make(map[string]struct{})

	for _, filename := range cmdOption.ApplicationFiles {
		fileContent, err := utils.ReadRemoteOrLocalPath(filename, true)
		if err != nil {
			return nil, err
		}

		fileType := filepath.Ext(filename)
		switch fileType {
		case ".yaml", ".yml":
			// only support one object in one yaml file
			fileContent, err = yaml.YAMLToJSON(fileContent)
			if err != nil {
				return nil, err
			}
			decode := scheme.Codecs.UniversalDeserializer().Decode
			// cannot guarantee get the object, but gkv is enough
			_, gkv, _ := decode(fileContent, nil, nil)

			jsonFileContent, err := yaml.YAMLToJSON(fileContent)
			if err != nil {
				return nil, err
			}

			switch *gkv {
			case corev1beta1.ApplicationKindVersionKind:
				if app != nil {
					return nil, errors.New("more than one applications provided")
				}
				app = new(corev1beta1.Application)
				err = json.Unmarshal(jsonFileContent, app)
				if err != nil {
					return nil, err
				}
			case v1alpha1.PolicyGroupVersionKind:
				policy := new(v1alpha1.Policy)
				err = json.Unmarshal(jsonFileContent, policy)
				if err != nil {
					return nil, err
				}
				policies = append(policies, policy)
			case v1alpha1.WorkflowGroupVersionKind:
				if wf != nil {
					return nil, errors.New("more than one external workflow provided")
				}
				wf = new(wfv1alpha1.Workflow)
				err = json.Unmarshal(jsonFileContent, wf)
				if err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("file %s is not application, policy or workflow", filename)
			}
		}
	}

	// only allow one application
	if app == nil {
		return nil, errors.New("no application provided")
	}

	// workflow not referenced by application
	if !cmdOption.MergeStandaloneFiles {
		if wf != nil &&
			((app.Spec.Workflow != nil && app.Spec.Workflow.Ref != wf.Name) || app.Spec.Workflow == nil) {
			buff.WriteString(fmt.Sprintf("WARNING: workflow %s not referenced by application\n\n", wf.Name))
		}
	} else {
		if wf != nil {
			app.Spec.Workflow = &corev1beta1.Workflow{
				Ref:   "",
				Steps: wf.Steps,
			}
		}
		err := getPolicyNameFromWorkflow(wf, policyNameMap)
		if err != nil {
			return nil, err
		}
	}

	for _, policy := range policies {
		// check standalone policies
		if _, exist := policyNameMap[policy.Name]; !exist && !cmdOption.MergeStandaloneFiles {
			buff.WriteString(fmt.Sprintf("WARNING: policy %s not referenced by application\n\n", policy.Name))
			continue
		}
		app.Spec.Policies = append(app.Spec.Policies, corev1beta1.AppPolicy{
			Name:       policy.Name,
			Type:       policy.Type,
			Properties: policy.Properties,
		})
	}
	return app, nil
}

func getPolicyNameFromWorkflow(wf *wfv1alpha1.Workflow, policyNameMap map[string]struct{}) error {

	checkPolicy := func(wfsb wfv1alpha1.WorkflowStepBase, policyNameMap map[string]struct{}) error {
		workflowStepSpec := &step.DeployWorkflowStepSpec{}
		if err := utils.StrictUnmarshal(wfsb.Properties.Raw, workflowStepSpec); err != nil {
			return err
		}
		for _, p := range workflowStepSpec.Policies {
			policyNameMap[p] = struct{}{}
		}
		return nil
	}

	if wf == nil {
		return nil
	}

	for _, wfs := range wf.Steps {
		if wfs.Type == step.DeployWorkflowStep {
			err := checkPolicy(wfs.WorkflowStepBase, policyNameMap)
			if err != nil {
				return err
			}
			for _, sub := range wfs.SubSteps {
				if sub.Type == step.DeployWorkflowStep {
					err = checkPolicy(sub, policyNameMap)
					if err != nil {
						return err
					}
				}
			}

		}
	}
	return nil
}
