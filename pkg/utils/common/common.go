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

package common

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/encoding/openapi"
	"github.com/AlecAivazis/survey/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/oam-dev/terraform-config-inspect/tfconfig"

	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	errors2 "github.com/pkg/errors"
	certmanager "github.com/wonderflow/cert-manager-api/pkg/apis/certmanager/v1"
	istioclientv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	v1 "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	metricsV1beta1api "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	ocmclusterv1 "open-cluster-management.io/api/cluster/v1"
	ocmclusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	ocmworkv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/yaml"

	prismclusterv1alpha1 "github.com/kubevela/prism/pkg/apis/cluster/v1alpha1"
	clustergatewayapi "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta2"

	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamstandard "github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/types"
	velacue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var (
	// Scheme defines the default KubeVela schema
	Scheme = k8sruntime.NewScheme()
)

const (
	// AddonObservabilityApplication is the application name for Addon Observability
	AddonObservabilityApplication = "addon-observability"
	// AddonObservabilityGrafanaSvc is grafana service name for Addon Observability
	AddonObservabilityGrafanaSvc = "grafana"
)

// CreateCustomNamespace display the create namespace message
const CreateCustomNamespace = "create new namespace"

func init() {
	_ = clientgoscheme.AddToScheme(Scheme)
	_ = apiregistrationv1.AddToScheme(Scheme)
	_ = crdv1.AddToScheme(Scheme)
	_ = oamcore.AddToScheme(Scheme)
	_ = oamstandard.AddToScheme(Scheme)
	_ = istioclientv1beta1.AddToScheme(Scheme)
	_ = certmanager.AddToScheme(Scheme)
	_ = kruise.AddToScheme(Scheme)
	_ = terraformapi.AddToScheme(Scheme)
	_ = ocmclusterv1alpha1.Install(Scheme)
	_ = ocmclusterv1.Install(Scheme)
	_ = ocmworkv1.Install(Scheme)
	_ = clustergatewayapi.AddToScheme(Scheme)
	_ = metricsV1beta1api.AddToScheme(Scheme)
	_ = prismclusterv1alpha1.AddToScheme(Scheme)
	// +kubebuilder:scaffold:scheme
}

// HTTPOption define the https options
type HTTPOption struct {
	Username string
	Password string
}

// InitBaseRestConfig will return reset config for create controller runtime client
func InitBaseRestConfig() (Args, error) {
	args := Args{
		Schema: Scheme,
	}
	_, err := args.GetConfig()
	if err != nil && os.Getenv("IGNORE_KUBE_CONFIG") != "true" {
		fmt.Println("get kubeConfig err", err)
		os.Exit(1)
	} else if err != nil {
		return Args{}, err
	}
	return args, nil
}

// globalClient will be a client for whole command lifecycle
var globalClient client.Client

// SetGlobalClient will set a client for one cli command
func SetGlobalClient(clt client.Client) error {
	globalClient = clt
	return nil
}

// GetClient will K8s client in args
func GetClient() (client.Client, error) {
	if globalClient != nil {
		return globalClient, nil
	}
	return nil, errors.New("client not set, call SetGlobalClient first")
}

// HTTPGetWithOption use HTTP option and default client to send get request
func HTTPGetWithOption(ctx context.Context, url string, opts *HTTPOption) ([]byte, error) {
	// Change NewRequest to NewRequestWithContext and pass context it
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if opts != nil && len(opts.Username) != 0 && len(opts.Password) != 0 {
		req.SetBasicAuth(opts.Username, opts.Password)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// GetCUEParameterValue converts definitions to cue format
func GetCUEParameterValue(cueStr string, pd *packages.PackageDiscover) (cue.Value, error) {
	var template *cue.Instance
	var err error
	if pd != nil {
		bi := build.NewContext().NewInstance("", nil)
		err := bi.AddFile("-", cueStr+velacue.BaseTemplate)
		if err != nil {
			return cue.Value{}, err
		}

		template, err = pd.ImportPackagesAndBuildInstance(bi)
		if err != nil {
			return cue.Value{}, err
		}
	} else {
		r := cue.Runtime{}
		template, err = r.Compile("", cueStr+velacue.BaseTemplate)
		if err != nil {
			return cue.Value{}, err
		}
	}
	tempStruct, err := template.Value().Struct()
	if err != nil {
		return cue.Value{}, err
	}
	// find the parameter definition
	var paraDef cue.FieldInfo
	var found bool
	for i := 0; i < tempStruct.Len(); i++ {
		paraDef = tempStruct.Field(i)
		if paraDef.Name == model.ParameterFieldName {
			found = true
			break
		}
	}
	if !found {
		return cue.Value{}, errors.New("parameter not exist")
	}
	arguments := paraDef.Value

	return arguments, nil
}

// GenOpenAPI generates OpenAPI json schema from cue.Instance
func GenOpenAPI(inst *cue.Instance) ([]byte, error) {
	if inst.Err != nil {
		return nil, inst.Err
	}
	paramOnlyIns, err := RefineParameterInstance(inst)
	if err != nil {
		return nil, err
	}
	defaultConfig := &openapi.Config{}
	b, err := openapi.Gen(paramOnlyIns, defaultConfig)
	if err != nil {
		return nil, err
	}
	var out = &bytes.Buffer{}
	_ = json.Indent(out, b, "", "   ")
	return out.Bytes(), nil
}

// extractParameterDefinitionNodeFromInstance extracts the `#parameter` ast.Node from root instance, if failed fall back to `parameter` by LookUpDef
func extractParameterDefinitionNodeFromInstance(inst *cue.Instance) ast.Node {
	opts := []cue.Option{cue.All(), cue.DisallowCycles(true), cue.ResolveReferences(true), cue.Docs(true)}
	node := inst.Value().Syntax(opts...)
	if fileNode, ok := node.(*ast.File); ok {
		for _, decl := range fileNode.Decls {
			if field, ok := decl.(*ast.Field); ok {
				if label, ok := field.Label.(*ast.Ident); ok && label.Name == "#"+model.ParameterFieldName {
					return decl.(*ast.Field).Value
				}
			}
		}
	}
	paramVal := inst.LookupDef(model.ParameterFieldName)
	return paramVal.Syntax(opts...)
}

// RefineParameterInstance refines cue instance to merely include `parameter` identifier
func RefineParameterInstance(inst *cue.Instance) (*cue.Instance, error) {
	r := cue.Runtime{}
	paramVal := inst.LookupDef(model.ParameterFieldName)
	var paramOnlyStr string
	switch k := paramVal.IncompleteKind(); k {
	case cue.StructKind, cue.ListKind:
		paramSyntax, _ := format.Node(extractParameterDefinitionNodeFromInstance(inst))
		paramOnlyStr = fmt.Sprintf("#%s: %s\n", model.ParameterFieldName, string(paramSyntax))
	case cue.IntKind, cue.StringKind, cue.FloatKind, cue.BoolKind:
		paramOnlyStr = fmt.Sprintf("#%s: %v", model.ParameterFieldName, paramVal)
	case cue.BottomKind:
		paramOnlyStr = fmt.Sprintf("#%s: {}", model.ParameterFieldName)
	default:
		return nil, fmt.Errorf("unsupport parameter kind: %s", k.String())
	}
	paramOnlyIns, err := r.Compile("-", paramOnlyStr)
	if err != nil {
		return nil, err
	}
	return paramOnlyIns, nil
}

// RealtimePrintCommandOutput prints command output in real time
// If logFile is "", it will prints the stdout, or it will write to local file
func RealtimePrintCommandOutput(cmd *exec.Cmd, logFile string) error {
	var writer io.Writer
	if logFile == "" {
		writer = io.MultiWriter(os.Stdout)
	} else {
		if _, err := os.Stat(filepath.Dir(logFile)); err != nil {
			return err
		}
		f, err := os.Create(filepath.Clean(logFile))
		if err != nil {
			return err
		}
		writer = io.MultiWriter(f)
	}
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// ClusterObject2Map convert ClusterObjectReference to a readable map
func ClusterObject2Map(refs []common.ClusterObjectReference) map[string]string {
	clusterResourceRefTmpl := "Cluster: %s | Namespace: %s | Kind: %s | Name: %s"
	objs := make(map[string]string, len(refs))
	for _, r := range refs {
		if r.Cluster == "" {
			r.Cluster = "local"
		}
		objs[r.Cluster+"/"+r.Namespace+"/"+r.Name+"/"+r.Kind] = fmt.Sprintf(clusterResourceRefTmpl, r.Cluster, r.Namespace, r.Kind, r.Name)
	}
	return objs
}

// ResourceLocation indicates the resource location
type ResourceLocation struct {
	Cluster   string
	Namespace string
}

type clusterObjectReferenceFilter func(common.ClusterObjectReference) bool

func clusterObjectReferenceTypeFilterGenerator(allowedKinds ...string) clusterObjectReferenceFilter {
	allowedKindMap := map[string]bool{}
	for _, allowedKind := range allowedKinds {
		allowedKindMap[allowedKind] = true
	}
	return func(item common.ClusterObjectReference) bool {
		_, exists := allowedKindMap[item.Kind]
		return exists
	}
}

var isWorkloadClusterObjectReferenceFilter = clusterObjectReferenceTypeFilterGenerator("Deployment", "StatefulSet", "CloneSet", "Job", "Configuration")
var isPortForwardEndpointClusterObjectReferenceFilter = clusterObjectReferenceTypeFilterGenerator("Deployment",
	"StatefulSet", "CloneSet", "Job", "Service", "HelmRelease")
var resourceNameClusterObjectReferenceFilter = func(resourceName []string) clusterObjectReferenceFilter {
	return func(reference common.ClusterObjectReference) bool {
		if len(resourceName) == 0 {
			return true
		}
		for _, r := range resourceName {
			if r == reference.Name {
				return true
			}
		}
		return false
	}
}

func filterResource(inputs []common.ClusterObjectReference, filters ...clusterObjectReferenceFilter) (outputs []common.ClusterObjectReference) {
	for _, item := range inputs {
		flag := true
		for _, filter := range filters {
			if !filter(item) {
				flag = false
				break
			}
		}
		if flag {
			outputs = append(outputs, item)
		}
	}
	return
}

func askToChooseOneResource(app *v1beta1.Application, filters ...clusterObjectReferenceFilter) (*common.ClusterObjectReference, error) {
	resources := app.Status.AppliedResources
	if len(resources) == 0 {
		return nil, fmt.Errorf("no resources in the application deployed yet")
	}
	resources = filterResource(resources, filters...)
	if app.Name == AddonObservabilityApplication {
		resources = filterClusterObjectRefFromAddonObservability(resources)
	}
	// filter locations
	if len(resources) == 0 {
		return nil, fmt.Errorf("no supported resources detected in deployed resources")
	}
	if len(resources) == 1 {
		return &resources[0], nil
	}
	opMap := ClusterObject2Map(resources)
	var ops []string
	for _, r := range opMap {
		ops = append(ops, r)
	}
	prompt := &survey.Select{
		Message: fmt.Sprintf("You have %d deployed resources in your app. Please choose one:", len(ops)),
		Options: ops,
	}
	var selectedRsc string
	err := survey.AskOne(prompt, &selectedRsc)
	if err != nil {
		return nil, fmt.Errorf("choosing resource err %w", err)
	}
	for k, resource := range ops {
		if selectedRsc == resource {
			return &resources[k], nil
		}
	}
	return nil, fmt.Errorf("choosing resource err %w", err)
}

// AskToChooseOneNamespace ask for choose one namespace as env
func AskToChooseOneNamespace(c client.Client, envMeta *types.EnvMeta) error {
	var nsList v1.NamespaceList
	if err := c.List(context.TODO(), &nsList); err != nil {
		return err
	}
	var ops = []string{CreateCustomNamespace}
	for _, r := range nsList.Items {
		ops = append(ops, r.Name)
	}
	prompt := &survey.Select{
		Message: "Would you like to choose an existing namespaces as your env?",
		Options: ops,
	}
	err := survey.AskOne(prompt, &envMeta.Namespace)
	if err != nil {
		return fmt.Errorf("choosing namespace err %w", err)
	}
	if envMeta.Namespace == CreateCustomNamespace {
		err = survey.AskOne(&survey.Input{
			Message: "Please name the new namespace:",
		}, &envMeta.Namespace)
		if err != nil {
			return err
		}
		return nil
	}
	for _, ns := range nsList.Items {
		if ns.Name == envMeta.Namespace && envMeta.Name == "" {
			envMeta.Name = ns.Labels[oam.LabelNamespaceOfEnvName]
			return nil
		}
	}
	return nil
}

func filterClusterObjectRefFromAddonObservability(resources []common.ClusterObjectReference) []common.ClusterObjectReference {
	var observabilityResources []common.ClusterObjectReference
	for _, res := range resources {
		if res.Namespace == types.DefaultKubeVelaNS && res.Name == AddonObservabilityGrafanaSvc {
			res.Kind = "Service"
			res.APIVersion = "v1"
			observabilityResources = append(observabilityResources, res)
		}
	}
	resources = observabilityResources
	return resources
}

func removeEmptyString(items []string) []string {
	r := []string{}
	for _, i := range items {
		if i != "" {
			r = append(r, i)
		}
	}
	return r
}

// AskToChooseOneEnvResource will ask users to select one applied resource of the application if more than one
// resource is a map for component to applied resources
// return the selected ClusterObjectReference
func AskToChooseOneEnvResource(app *v1beta1.Application, resourceName ...string) (*common.ClusterObjectReference, error) {
	filters := []clusterObjectReferenceFilter{isWorkloadClusterObjectReferenceFilter}
	_resourceName := removeEmptyString(resourceName)
	filters = append(filters, resourceNameClusterObjectReferenceFilter(_resourceName))
	return askToChooseOneResource(app, filters...)
}

// AskToChooseOnePortForwardEndpoint will ask user to select one applied resource as port forward endpoint
func AskToChooseOnePortForwardEndpoint(app *v1beta1.Application, resourceName ...string) (*common.ClusterObjectReference, error) {
	filters := []clusterObjectReferenceFilter{isPortForwardEndpointClusterObjectReferenceFilter}
	_resourceName := removeEmptyString(resourceName)
	filters = append(filters, resourceNameClusterObjectReferenceFilter(_resourceName))
	return askToChooseOneResource(app, filters...)
}

func askToChooseOneInApplication(category string, options []string) (decision string, err error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no %s exists in the application", category)
	}
	if len(options) == 1 {
		return options[0], nil
	}
	prompt := &survey.Select{
		Message: fmt.Sprintf("You have multiple %ss in your app. Please choose one %s: ", category, category),
		Options: options,
	}
	if err = survey.AskOne(prompt, &decision); err != nil {
		return "", errors2.Wrapf(err, "choosing %s failed", category)
	}
	return
}

// AskToChooseOneService will ask users to select one service of the application if more than one
func AskToChooseOneService(svcNames []string) (string, error) {
	return askToChooseOneInApplication("service", svcNames)
}

// AskToChooseOnePods will ask users to select one pods of the resource if more than one
func AskToChooseOnePods(podNames []string) (string, error) {
	return askToChooseOneInApplication("pod", podNames)
}

// ReadYamlToObject will read a yaml K8s object to runtime.Object
func ReadYamlToObject(path string, object k8sruntime.Object) error {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, object)
}

// ParseTerraformVariables get variables from Terraform Configuration
func ParseTerraformVariables(configuration string) (map[string]*tfconfig.Variable, map[string]*tfconfig.Output, error) {
	p := hclparse.NewParser()
	hclFile, diagnostic := p.ParseHCL([]byte(configuration), "")
	if diagnostic != nil {
		return nil, nil, errors.New(diagnostic.Error())
	}
	mod := tfconfig.Module{Variables: map[string]*tfconfig.Variable{}, Outputs: map[string]*tfconfig.Output{}}
	diagnostic = tfconfig.LoadModuleFromFile(hclFile, &mod)
	if diagnostic != nil {
		return nil, nil, errors.New(diagnostic.Error())
	}
	return mod.Variables, mod.Outputs, nil
}

// GenerateUnstructuredObj generate UnstructuredObj
func GenerateUnstructuredObj(name, ns string, gvk schema.GroupVersionKind) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	u.SetName(name)
	u.SetNamespace(ns)
	return u
}

// SetSpecObjIntoUnstructuredObj set UnstructuredObj spec field
func SetSpecObjIntoUnstructuredObj(spec interface{}, u *unstructured.Unstructured) error {
	bts, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	data := make(map[string]interface{})
	if err := json.Unmarshal(bts, &data); err != nil {
		return err
	}
	_ = unstructured.SetNestedMap(u.Object, data, "spec")
	return nil
}

// NewK8sClient init a local k8s client which add oamcore scheme
func NewK8sClient() (client.Client, error) {
	conf, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	scheme := k8sruntime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := oamcore.AddToScheme(scheme); err != nil {
		return nil, err
	}

	k8sClient, err := client.New(conf, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	return k8sClient, nil
}
