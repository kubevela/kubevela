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

package dryrun

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/util/openapi"
	"k8s.io/kubectl/pkg/util/openapi/validation"
	kval "k8s.io/kubectl/pkg/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/kubevela/workflow/pkg/cue/packages"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/policy/envbinding"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/workflow/step"
)

// DryRun executes dry-run on an application
type DryRun interface {
	ExecuteDryRun(ctx context.Context, app *v1beta1.Application) ([]*types.ComponentManifest, []*unstructured.Unstructured, error)
}

// NewDryRunOption creates a dry-run option
func NewDryRunOption(c client.Client, cfg *rest.Config, dm discoverymapper.DiscoveryMapper, pd *packages.PackageDiscover, as []oam.Object, serverSideDryRun bool) *Option {
	parser := appfile.NewDryRunApplicationParser(c, dm, pd, as)
	return &Option{c, dm, pd, parser, parser.GenerateAppFileFromApp, cfg, as, serverSideDryRun}
}

// GenerateAppFileFunc generate the app file model from an application
type GenerateAppFileFunc func(ctx context.Context, app *v1beta1.Application) (*appfile.Appfile, error)

// Option contains options to execute dry-run
type Option struct {
	Client          client.Client
	DiscoveryMapper discoverymapper.DiscoveryMapper
	PackageDiscover *packages.PackageDiscover
	Parser          *appfile.Parser
	GenerateAppFile GenerateAppFileFunc
	cfg             *rest.Config
	// Auxiliaries are capability definitions used to parse application.
	// DryRun will use capabilities in Auxiliaries as higher priority than
	// getting one from cluster.
	Auxiliaries []oam.Object

	// serverSideDryRun If set to true, means will dry run via the apiserver.
	serverSideDryRun bool
}

// validateObjectFromFile will read file into Unstructured object
func (d *Option) validateObjectFromFile(filename string) (*unstructured.Unstructured, error) {
	fileContent, err := os.ReadFile(filepath.Clean(filename))
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

	dc, err := discovery.NewDiscoveryClientForConfig(d.cfg)
	if err != nil {
		return nil, err
	}
	openAPIGetter := openapi.NewOpenAPIGetter(dc)
	resources, err := openapi.NewOpenAPIParser(openAPIGetter).Parse()
	if err != nil {
		return nil, err
	}

	valids := kval.ConjunctiveSchema{validation.NewSchemaValidation(resources), kval.NoDoubleKeySchema{}}
	if err = valids.ValidateBytes(fileContent); err != nil {
		return nil, err
	}

	app := new(unstructured.Unstructured)
	err = json.Unmarshal(fileContent, app)
	return app, err
}

// ValidateApp will validate app with client schema check and server side dry-run
func (d *Option) ValidateApp(ctx context.Context, filename string) error {
	app, err := d.validateObjectFromFile(filename)
	if err != nil {
		return err
	}
	if len(app.GetNamespace()) == 0 {
		app.SetNamespace(corev1.NamespaceDefault)
	}
	app2 := app.DeepCopy()

	err = d.Client.Get(ctx, client.ObjectKey{Namespace: app.GetNamespace(), Name: app.GetName()}, app2)
	if err == nil {
		app.SetResourceVersion(app2.GetResourceVersion())
		return d.Client.Update(ctx, app, client.DryRunAll)
	}
	return d.Client.Create(ctx, app, client.DryRunAll)
}

// ExecuteDryRun simulates applying an application into cluster and returns rendered
// resources but not persist them into cluster.
func (d *Option) ExecuteDryRun(ctx context.Context, application *v1beta1.Application) ([]*types.ComponentManifest, []*unstructured.Unstructured, error) {
	app := application.DeepCopy()
	if app.Namespace != "" {
		ctx = oamutil.SetNamespaceInCtx(ctx, app.Namespace)
	}
	appFile, err := d.GenerateAppFile(ctx, app)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "cannot generate appFile from application")
	}
	if appFile.Namespace == "" {
		appFile.Namespace = corev1.NamespaceDefault
	}

	comps, err := appFile.GenerateComponentManifests()
	if err != nil {
		return nil, nil, errors.WithMessage(err, "cannot generate manifests from components and traits")
	}
	policyManifests, err := appFile.GeneratePolicyManifests(ctx)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "cannot generate manifests from policies")
	}
	if d.serverSideDryRun {
		applyUtil := apply.NewAPIApplicator(d.Client)
		if err := applyUtil.Apply(ctx, app, apply.DryRunAll()); err != nil {
			return nil, nil, err
		}
	}
	return comps, policyManifests, nil
}

// PrintDryRun will print the result of dry-run
func (d *Option) PrintDryRun(buff *bytes.Buffer, appName string, comps []*types.ComponentManifest, policies []*unstructured.Unstructured) error {
	var components = make(map[string]*unstructured.Unstructured)
	for _, comp := range comps {
		components[comp.Name] = comp.StandardWorkload
	}
	for _, c := range comps {
		if _, err := fmt.Fprintf(buff, "---\n# Application(%s) -- Component(%s) \n---\n\n", appName, c.Name); err != nil {
			return errors.Wrap(err, "fail to write buff")
		}
		result, err := yaml.Marshal(components[c.Name])
		if err != nil {
			return errors.New("marshal result for component " + c.Name + " object in yaml format")
		}
		buff.Write(result)
		buff.WriteString("\n---\n")
		for _, t := range c.Traits {
			traitType := t.GetLabels()[oam.TraitTypeLabel]
			switch {
			case traitType == definition.AuxiliaryWorkload:
				buff.WriteString("## From the auxiliary workload \n")
			case traitType != "":
				buff.WriteString(fmt.Sprintf("## From the trait %s \n", traitType))
			}
			result, err := yaml.Marshal(t)
			if err != nil {
				return errors.New("marshal result for Component " + c.Name + " trait " + t.GetName() + " object in yaml format")
			}
			buff.Write(result)
			buff.WriteString("\n---\n")
		}
		buff.WriteString("\n")
	}
	for _, plc := range policies {
		if _, err := fmt.Fprintf(buff, "---\n# Application(%s) -- Policy(%s) \n---\n\n", appName, plc.GetName()); err != nil {
			return errors.Wrap(err, "fail to write buff")
		}
		result, err := yaml.Marshal(plc)
		if err != nil {
			return errors.New("marshal result for policy " + plc.GetName() + " object in yaml format")
		}
		buff.Write(result)
		buff.WriteString("\n---\n")
	}
	return nil
}

// ExecuteDryRunWithPolicies is similar to ExecuteDryRun func, but considers deploy workflow step and topology+override policies
func (d *Option) ExecuteDryRunWithPolicies(ctx context.Context, application *v1beta1.Application, buff *bytes.Buffer) error {

	app := application.DeepCopy()
	if app.Namespace == "" {
		app.Namespace = corev1.NamespaceDefault
	} else {
		ctx = oamutil.SetNamespaceInCtx(ctx, app.Namespace)
	}
	parser := appfile.NewDryRunApplicationParser(d.Client, d.DiscoveryMapper, d.PackageDiscover, d.Auxiliaries)
	af, err := parser.GenerateAppFileFromApp(ctx, app)
	if err != nil {
		return err
	}

	deployWorkflowCount := 0
	for _, wfs := range af.WorkflowSteps {
		if wfs.Type == step.DeployWorkflowStep {
			deployWorkflowCount++
			deployWorkflowStepSpec := &step.DeployWorkflowStepSpec{}
			if err := utils.StrictUnmarshal(wfs.Properties.Raw, deployWorkflowStepSpec); err != nil {
				return err
			}

			topologyPolicies, overridePolicies, err := filterPolicies(af.Policies, deployWorkflowStepSpec.Policies)
			if err != nil {
				return err
			}
			if len(topologyPolicies) > 0 {
				for _, tp := range topologyPolicies {
					patchedApp, err := patchApp(app, overridePolicies)
					if err != nil {
						return err
					}
					comps, pms, err := d.ExecuteDryRun(ctx, patchedApp)
					if err != nil {
						return err
					}
					err = d.PrintDryRun(buff, fmt.Sprintf("%s with topology %s", patchedApp.Name, tp.Name), comps, pms)
					if err != nil {
						return err
					}
				}
			} else {
				patchedApp, err := patchApp(app, overridePolicies)
				if err != nil {
					return err
				}
				comps, pms, err := d.ExecuteDryRun(ctx, patchedApp)
				if err != nil {
					return err
				}
				err = d.PrintDryRun(buff, fmt.Sprintf("%s only with override policies", patchedApp.Name), comps, pms)
				if err != nil {
					return err
				}
			}
		}
	}

	if deployWorkflowCount == 0 {
		comps, pms, err := d.ExecuteDryRun(ctx, app)
		if err != nil {
			return err
		}
		err = d.PrintDryRun(buff, app.Name, comps, pms)
		if err != nil {
			return err
		}
	}

	return nil
}

func filterPolicies(policies []v1beta1.AppPolicy, policyNames []string) ([]v1beta1.AppPolicy, []v1beta1.AppPolicy, error) {
	policyMap := make(map[string]v1beta1.AppPolicy)
	for _, policy := range policies {
		policyMap[policy.Name] = policy
	}
	var topologyPolicies []v1beta1.AppPolicy
	var overridePolicies []v1beta1.AppPolicy
	for _, policyName := range policyNames {
		if policy, found := policyMap[policyName]; found {
			switch policy.Type {
			case v1alpha1.TopologyPolicyType:
				topologyPolicies = append(topologyPolicies, policy)
			case v1alpha1.OverridePolicyType:
				overridePolicies = append(overridePolicies, policy)
			}
		} else {
			return nil, nil, errors.Errorf("policy %s not found", policyName)
		}
	}
	return topologyPolicies, overridePolicies, nil
}

func patchApp(application *v1beta1.Application, overridePolicies []v1beta1.AppPolicy) (*v1beta1.Application, error) {
	app := application.DeepCopy()
	for _, policy := range overridePolicies {

		if policy.Properties == nil {
			return nil, fmt.Errorf("override policy %s must not have empty properties", policy.Name)
		}
		overrideSpec := &v1alpha1.OverridePolicySpec{}
		if err := utils.StrictUnmarshal(policy.Properties.Raw, overrideSpec); err != nil {
			return nil, errors.Wrapf(err, "failed to parse override policy %s", policy.Name)
		}
		overrideComps, err := envbinding.PatchComponents(app.Spec.Components, overrideSpec.Components, overrideSpec.Selector)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to apply override policy %s", policy.Name)
		}
		app.Spec.Components = overrideComps
	}

	return app, nil
}
