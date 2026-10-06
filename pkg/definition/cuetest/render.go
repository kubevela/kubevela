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

package cuetest

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	wfprocess "github.com/kubevela/workflow/pkg/cue/process"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

// Kind is the definition kind a template renders as.
type Kind string

const (
	// KindComponent renders `output` and `outputs`.
	KindComponent Kind = "component"
	// KindTrait patches a workload and renders `outputs`.
	KindTrait Kind = "trait"
	// KindAddon is an addon, rendered as enabling it would install it.
	KindAddon Kind = "addon"
	// KindWorkflowStep is a workflow step.
	KindWorkflowStep Kind = "workflow-step"
	// KindSource is a source, resolved into a value Applications read.
	KindSource Kind = "source"
	// KindPolicy is a workload-bearing policy, rendering objects dispatched
	// beside the Application's components.
	KindPolicy Kind = "policy"
	// KindApplicationPolicy is an Application-scoped policy, transforming the
	// Application before it is parsed.
	KindApplicationPolicy Kind = "application-policy"
)

// Subject is the definition under test.
type Subject struct {
	Kind     Kind
	Name     string
	Template string
	// HealthPolicy, CustomStatus and Details are the definition's status snippets.
	HealthPolicy string
	CustomStatus string
	Details      string
	// Object is the definition as loaded. When nil, one is built from the
	// fields above.
	Object *unstructured.Unstructured
}

// Context is the settable part of the `context` a template sees: the fields
// the context registry (pkg/definition/propexpr) offers a component or trait,
// less those derivedContextFields names. Unset fields take the defaults of
// defkit's TestContext, so a CUE and a defkit test of one definition agree.
type Context struct {
	Name           string            `json:"name,omitempty"`
	Namespace      string            `json:"namespace,omitempty"`
	AppName        string            `json:"appName,omitempty"`
	AppRevision    string            `json:"appRevision,omitempty"`
	AppLabels      map[string]string `json:"appLabels,omitempty"`
	AppAnnotations map[string]string `json:"appAnnotations,omitempty"`
	Cluster        string            `json:"cluster,omitempty"`
	ClusterVersion *ClusterVersion   `json:"clusterVersion,omitempty"`
	PublishVersion string            `json:"publishVersion,omitempty"`
	WorkflowName   string            `json:"workflowName,omitempty"`
	Revision       string            `json:"revision,omitempty"`
	ReplicaKey     string            `json:"replicaKey,omitempty"`
	Custom         map[string]any    `json:"custom,omitempty"`
	StepName       string            `json:"stepName,omitempty"`
	// ComponentType is the type of the workload a trait under test patches.
	ComponentType string `json:"componentType,omitempty"`
	// PolicyName and the revision fields are an Application-scoped policy's.
	PolicyName         string `json:"policyName,omitempty"`
	PolicyRevisionName string `json:"policyRevisionName,omitempty"`
	PolicyRevisionHash string `json:"policyRevisionHash,omitempty"`
	PolicyRevision     int64  `json:"policyRevision,omitempty"`
}

// ClusterVersion is the `context.clusterVersion` a template sees.
type ClusterVersion struct {
	Major      string `json:"major"`
	Minor      int    `json:"minor"`
	GitVersion string `json:"gitVersion,omitempty"`
	Platform   string `json:"platform,omitempty"`
}

// derivedContextFields are, for each kind, the context fields a case
// determines some other way, with how.
var derivedContextFields = map[Kind]map[string]string{
	KindComponent: {
		"componentName":  "it is context.name",
		"componentType":  "it is the definition's type",
		"appRevisionNum": `set context.appRevision, e.g. "shop-v3"`,
	},
	KindTrait: {
		"componentName":  "it is context.name",
		"traitType":      "it is the definition's type",
		"appRevisionNum": `set context.appRevision, e.g. "shop-v3"`,
	},
	KindPolicy: {
		"cluster":        "a policy's objects are dispatched to the hub, so it is local",
		"policyName":     "it is context.name",
		"policyType":     "it is the definition's type",
		"appRevisionNum": `set context.appRevision, e.g. "shop-v3"`,
	},
	KindApplicationPolicy: {
		"policyType":     "it is the definition's type",
		"appRevisionNum": `set context.appRevision, e.g. "shop-v3"`,
	},
	KindWorkflowStep: {
		"name":           "in a step template it is the Application; set context.appName",
		"stepSessionID":  "the workflow engine sets it",
		"appRevisionNum": `set context.appRevision, e.g. "shop-v3"`,
	},
}

// contextSurfaces is the context registry surface each kind's template sees.
var contextSurfaces = map[Kind]propexpr.ContextSchema{
	KindComponent:         propexpr.ComponentContext,
	KindTrait:             propexpr.TraitContext,
	KindWorkflowStep:      propexpr.WorkflowStepTemplateContext,
	KindPolicy:            propexpr.RenderedPolicyContext,
	KindApplicationPolicy: propexpr.ScopedPolicyContext,
}

// Input is what a single case feeds the template.
type Input struct {
	Context   Context
	Parameter map[string]any
	// Workload is the object a trait patches; components ignore it.
	Workload map[string]any
	// Artifacts are the Application's rendered components a policy reads,
	// by component: {workload: {...}, traits: {<type>: {<output>: {...}}}}.
	Artifacts map[string]Artifact
	// Spec is the Application's spec an Application-scoped policy receives.
	Spec map[string]any
	// Traits are attached to a component, in order.
	Traits []TraitInput
	// Mocks answer the provider calls the templates make.
	Mocks Mocks
}

// TraitInput is a trait attached to the component under test.
type TraitInput struct {
	Subject   Subject
	Parameter map[string]any
}

// Rendered is what a template produced, as the controller would apply it.
type Rendered struct {
	// Output is the workload, after every trait's patch.
	Output map[string]any
	// Outputs holds the component's own `outputs`, by name.
	Outputs map[string]map[string]any
	// Traits holds each attached trait's outputs, by trait type.
	Traits map[string]RenderedTrait
	// Calls are the provider calls the render made, in the order they ran.
	Calls []Call
	// Context is the `context` templates see once the component and every
	// trait have rendered: context.output after the traits' patches, and
	// context.outputs holding every output by name, a later trait's
	// replacing an earlier one of the same name. It is as templates see it,
	// before the controller labels what it applies.
	Context map[string]any
	// ContextErr is why Context could not be read. The controller only reads
	// it when a later trait renders, so it fails a case only when asked for.
	ContextErr error
}

// RenderedTrait is what one trait type produced.
type RenderedTrait struct {
	Outputs map[string]map[string]any
}

// Render renders the definition the way the controller renders an
// Application's component: parsed into an appfile, with each trait evaluated
// in turn, and labelled as the controller labels what it applies.
func Render(s Subject, in Input) (*Rendered, error) {
	r, err := render(s, in)
	if err != nil {
		return nil, err
	}
	return r.Rendered, nil
}

// rendering is a render with the appfile state status evaluation continues
// from.
type rendering struct {
	*Rendered
	comp      *appfile.Component
	namespace string
}

// workloadDefinition is the type of the synthetic component a trait under
// test is attached to, unless its context names one.
const workloadDefinition = "cuetest-workload"

func render(s Subject, in Input) (*rendering, error) {
	component, componentParams, traits := s, in.Parameter, in.Traits
	switch s.Kind {
	case KindComponent:
	case KindTrait:
		if in.Workload == nil {
			return nil, fmt.Errorf("trait %q needs a workload to patch", s.Name)
		}
		base, err := json.Marshal(in.Workload)
		if err != nil {
			return nil, fmt.Errorf("encoding workload: %w", err)
		}
		component = Subject{Kind: KindComponent, Name: or(in.Context.ComponentType, workloadDefinition), Template: "output: " + string(base) + "\nparameter: {}"}
		componentParams = nil
		traits = []TraitInput{{Subject: s, Parameter: in.Parameter}}
	default:
		return nil, fmt.Errorf("definition kind %q is not supported", s.Kind)
	}

	defs := []*unstructured.Unstructured{definitionObject(component)}
	appTraits := make([]common.ApplicationTrait, 0, len(traits))
	for _, t := range traits {
		defs = append(defs, definitionObject(t.Subject))
		props, err := rawJSON(t.Parameter)
		if err != nil {
			return nil, fmt.Errorf("trait %s: %w", t.Subject.Name, err)
		}
		appTraits = append(appTraits, common.ApplicationTrait{Type: t.Subject.Name, Properties: props})
	}
	props, err := rawJSON(componentParams)
	if err != nil {
		return nil, err
	}
	c := in.Context
	app := application(c)
	app.Spec.Components = []common.ApplicationComponent{{
		Name:       or(c.Name, "test-component"),
		Type:       component.Name,
		Properties: props,
		Traits:     appTraits,
	}}

	name := app.Spec.Components[0].Name
	rendered, err := renderApplication(app, defs, c, in.Mocks, []string{name})
	if err != nil {
		return nil, err
	}
	return rendered[name], nil
}

// renderApplication renders the named components of app the way the
// controller renders an Application: parsed into an appfile against defs,
// each named component evaluated with its traits, and labelled as the
// controller labels what it applies. Mocks answer provider calls throughout,
// and each component's Calls are those its own render made.
func renderApplication(app *v1beta1.Application, defs []*unstructured.Unstructured, c Context, mocks Mocks, names []string) (map[string]*rendering, error) {
	restoreCompiler, calls, err := useMocks(mocks)
	if err != nil {
		return nil, err
	}
	defer restoreCompiler()

	namespace := or(app.Namespace, "default")
	ctx := oamutil.SetNamespaceInCtx(context.Background(), namespace)
	parser := appfile.NewDryRunApplicationParser(fake.NewClientBuilder().Build(), defs)
	af, err := parser.GenerateAppFileFromApp(ctx, app)
	if err != nil {
		return nil, err
	}
	af.AppRevisionName = or(c.AppRevision, app.Name+"-v1")
	mutate := func(d *process.ContextData) {
		// Status evaluation derives per-cluster contexts from this.
		if d.Ctx == nil {
			d.Ctx = context.Background()
		}
		if c.Custom != nil {
			// Where an Application-scoped policy's output.ctx is carried.
			d.Ctx = context.WithValue(d.Ctx, oam.PolicyAdditionalContextKey, c.Custom)
		}
		if c.Cluster != "" {
			d.Cluster = c.Cluster
		}
		d.ClusterVersion = clusterVersion(c.ClusterVersion)
		d.CompRevision = c.Revision
		d.ReplicaKey = c.ReplicaKey
	}

	rendered := map[string]*rendering{}
	for _, name := range names {
		var comp *appfile.Component
		for _, pc := range af.ParsedComponents {
			if pc.Name == name {
				comp = pc
			}
		}
		if comp == nil {
			return nil, fmt.Errorf("the application has no component %q", name)
		}
		start := len(calls.calls)
		cm, err := af.GenerateComponentManifest(comp, mutate)
		if err != nil {
			return nil, err
		}
		if err := af.SetOAMContract(cm); err != nil {
			return nil, err
		}
		out := &Rendered{
			Output:  cm.ComponentOutput.Object,
			Outputs: map[string]map[string]any{},
			Traits:  map[string]RenderedTrait{},
			Calls:   calls.calls[start:],
		}
		out.Context, out.ContextErr = templateContext(comp.Ctx)
		for _, obj := range cm.ComponentOutputsAndTraits {
			typ, output := obj.GetLabels()[oam.TraitTypeLabel], obj.GetLabels()[oam.TraitResource]
			if typ == definition.AuxiliaryWorkload {
				out.Outputs[output] = obj.Object
				continue
			}
			t, ok := out.Traits[typ]
			if !ok {
				t = RenderedTrait{Outputs: map[string]map[string]any{}}
				out.Traits[typ] = t
			}
			if _, clash := t.Outputs[output]; clash {
				return nil, fmt.Errorf("two %s traits both render outputs.%s", typ, output)
			}
			t.Outputs[output] = obj.Object
		}
		rendered[name] = &rendering{Rendered: out, comp: comp, namespace: namespace}
	}
	return rendered, nil
}

// templateContext decodes the `context` the process context gives templates.
func templateContext(pctx wfprocess.Context) (map[string]any, error) {
	file, err := pctx.BaseContextFile()
	if err != nil {
		return nil, fmt.Errorf("reading context: %w", err)
	}
	var ctx map[string]any
	if err := cuecontext.New().CompileString(file).LookupPath(cue.ParsePath("context")).Decode(&ctx); err != nil {
		return nil, fmt.Errorf("decoding context: %w", err)
	}
	return ctx, nil
}

// definitionObject is the definition as the controller would read it from
// the cluster.
func definitionObject(s Subject) *unstructured.Unstructured {
	if s.Object != nil {
		return s.Object.DeepCopy()
	}
	var kind string
	switch s.Kind {
	case KindTrait:
		kind = v1beta1.TraitDefinitionKind
	case KindPolicy:
		kind = v1beta1.PolicyDefinitionKind
	default:
		kind = v1beta1.ComponentDefinitionKind
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": v1beta1.SchemeGroupVersion.String(),
		"kind":       kind,
		"metadata":   map[string]any{"name": s.Name, "namespace": "vela-system"},
		"spec": map[string]any{
			"schematic": map[string]any{"cue": map[string]any{"template": s.Template}},
			"status": map[string]any{
				"healthPolicy": s.HealthPolicy,
				"customStatus": s.CustomStatus,
				"details":      s.Details,
			},
		},
	}}
}

// rawJSON is parameters as an Application carries them.
func rawJSON(v map[string]any) (*runtime.RawExtension, error) {
	if v == nil {
		v = map[string]any{}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encoding the parameters: %w", err)
	}
	return &runtime.RawExtension{Raw: b}, nil
}

func or(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// application is the Application a render runs in: its identity, labels and
// annotations from c, and nothing else.
func application(c Context) *v1beta1.Application {
	// The controller reads these two from the Application's annotations.
	annotations := map[string]string{}
	for k, v := range c.AppAnnotations {
		annotations[k] = v
	}
	if c.WorkflowName != "" {
		annotations[oam.AnnotationWorkflowName] = c.WorkflowName
	}
	if c.PublishVersion != "" {
		annotations[oam.AnnotationPublishVersion] = c.PublishVersion
	}
	return &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{APIVersion: v1beta1.SchemeGroupVersion.String(), Kind: v1beta1.ApplicationKind},
		ObjectMeta: metav1.ObjectMeta{
			Name: or(c.AppName, "test-app"), Namespace: or(c.Namespace, "default"),
			Labels: c.AppLabels, Annotations: annotations,
		},
		Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{}},
	}
}

// clusterVersion is v as the controller reports a cluster's version, or the
// version tests render against when a case gives none.
func clusterVersion(v *ClusterVersion) types.ClusterVersion {
	if v == nil {
		return types.ClusterVersion{Major: "1", Minor: "28"}
	}
	return types.ClusterVersion{Major: v.Major, Minor: strconv.Itoa(v.Minor), GitVersion: v.GitVersion, Platform: v.Platform}
}
