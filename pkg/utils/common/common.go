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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/encoding/openapi"
	"github.com/AlecAivazis/survey/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/oam-dev/terraform-config-inspect/tfconfig"
	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	kruisev1alpha1 "github.com/openkruise/rollouts/api/v1alpha1"
	yamlv3 "gopkg.in/yaml.v3"
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
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	"sigs.k8s.io/yaml"

	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/cue/packages"
	clustergatewayapi "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	terraformapiv1 "github.com/oam-dev/terraform-controller/api/v1beta1"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta2"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev"
	"github.com/oam-dev/kubevela/apis/types"
	velacue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var (
	// Scheme defines the default KubeVela schema
	Scheme = k8sruntime.NewScheme()
)

// CreateCustomNamespace display the create namespace message
const CreateCustomNamespace = "create new namespace"

func init() {
	_ = clientgoscheme.AddToScheme(Scheme)
	_ = apiregistrationv1.AddToScheme(Scheme)
	_ = crdv1.AddToScheme(Scheme)
	_ = oamcore.AddToScheme(Scheme)
	_ = kruise.AddToScheme(Scheme)
	_ = terraformapi.AddToScheme(Scheme)
	_ = terraformapiv1.AddToScheme(Scheme)
	_ = ocmclusterv1alpha1.Install(Scheme)
	_ = ocmclusterv1.Install(Scheme)
	_ = ocmworkv1.Install(Scheme)
	_ = clustergatewayapi.AddToScheme(Scheme)
	_ = metricsV1beta1api.AddToScheme(Scheme)
	_ = kruisev1alpha1.AddToScheme(Scheme)
	_ = gatewayv1beta1.AddToScheme(Scheme)
	_ = workflowv1alpha1.AddToScheme(Scheme)
	// +kubebuilder:scaffold:scheme
}

// HTTPOption define the https options
type HTTPOption struct {
	Username        string `json:"username,omitempty"`
	Password        string `json:"password,omitempty"`
	CaFile          string `json:"caFile,omitempty"`
	CertFile        string `json:"certFile,omitempty"`
	KeyFile         string `json:"keyFile,omitempty"`
	InsecureSkipTLS bool   `json:"insecureSkipTLS,omitempty"`
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

// HTTPGetResponse use HTTP option and default client to send request and get raw response
func HTTPGetResponse(ctx context.Context, url string, opts *HTTPOption) (*http.Response, error) {
	// Change NewRequest to NewRequestWithContext and pass context it
	if _, err := neturl.ParseRequestURI(url); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{}
	if opts != nil && len(opts.Username) != 0 && len(opts.Password) != 0 {
		req.SetBasicAuth(opts.Username, opts.Password)
	}
	if opts != nil && opts.InsecureSkipTLS {
		httpClient.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} // nolint
	}
	// if specify the caFile, we cannot re-use the default httpClient, so create a new one.
	if opts != nil && (len(opts.CaFile) != 0 || len(opts.KeyFile) != 0 || len(opts.CertFile) != 0) {
		// must set MinVersion of TLS, otherwise will report GoSec error G402
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
		tr := http.Transport{}
		if len(opts.CaFile) != 0 {
			c := x509.NewCertPool()
			if !(c.AppendCertsFromPEM([]byte(opts.CaFile))) {
				return nil, fmt.Errorf("failed to append certificates")
			}
			tlsConfig.RootCAs = c
		}
		if len(opts.CertFile) != 0 && len(opts.KeyFile) != 0 {
			cert, err := tls.X509KeyPair([]byte(opts.CertFile), []byte(opts.KeyFile))
			if err != nil {
				return nil, err
			}
			tlsConfig.Certificates = append(tlsConfig.Certificates, cert)
		}
		tr.TLSClientConfig = tlsConfig
		defer tr.CloseIdleConnections()
		httpClient.Transport = &tr
	}
	return httpClient.Do(req)
}

// HTTPGetWithOption use HTTP option and default client to send get request
func HTTPGetWithOption(ctx context.Context, url string, opts *HTTPOption) ([]byte, error) {
	resp, err := HTTPGetResponse(ctx, url, opts)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// HTTPGetKubernetesObjects use HTTP requests to load resources from remote url
func HTTPGetKubernetesObjects(ctx context.Context, url string) ([]*unstructured.Unstructured, error) {
	resp, err := HTTPGetResponse(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck
	defer resp.Body.Close()
	decoder := yamlv3.NewDecoder(resp.Body)
	var uns []*unstructured.Unstructured
	for {
		obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
		if err := decoder.Decode(obj.Object); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("failed to decode object: %w", err)
		}
		uns = append(uns, obj)
	}
	return uns, nil
}

// GetCUEParameterValue converts definitions to cue format
func GetCUEParameterValue(cueStr string, pd *packages.PackageDiscover) (cue.Value, error) {
	template, err := value.NewValue(cueStr+velacue.BaseTemplate, pd, "")
	if err != nil {
		return cue.Value{}, err
	}
	val, err := template.LookupValue(process.ParameterFieldName)
	if err != nil || !val.CueValue().Exists() {
		return cue.Value{}, velacue.ErrParameterNotExist
	}

	return val.CueValue(), nil
}

// GenOpenAPI generates OpenAPI json schema from cue.Instance
func GenOpenAPI(val *value.Value) (b []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("invalid cue definition to generate open api: %v", r)
			debug.PrintStack()
			return
		}
	}()
	if val.CueValue().Err() != nil {
		return nil, val.CueValue().Err()
	}
	paramOnlyVal, err := RefineParameterValue(val)
	if err != nil {
		return nil, err
	}
	defaultConfig := &openapi.Config{ExpandReferences: true}
	b, err = openapi.Gen(paramOnlyVal, defaultConfig)
	if err != nil {
		return nil, err
	}
	var out = &bytes.Buffer{}
	_ = json.Indent(out, b, "", "   ")
	return out.Bytes(), nil
}

// GenOpenAPIWithCueX generates OpenAPI json schema from cue.Instance
func GenOpenAPIWithCueX(val cue.Value) (b []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("invalid cue definition to generate open api: %v", r)
			debug.PrintStack()
			return
		}
	}()
	if val.Err() != nil {
		return nil, val.Err()
	}
	paramOnlyVal := FillParameterDefinitionFieldIfNotExist(val)
	defaultConfig := &openapi.Config{ExpandReferences: true}
	b, err = openapi.Gen(paramOnlyVal, defaultConfig)
	if err != nil {
		return nil, err
	}
	var out = &bytes.Buffer{}
	_ = json.Indent(out, b, "", "   ")
	return out.Bytes(), nil
}

// RefineParameterValue refines cue value to merely include `parameter` identifier
func RefineParameterValue(val *value.Value) (cue.Value, error) {
	defaultValue := cuecontext.New().CompileString("#parameter: {}")
	parameterPath := cue.MakePath(cue.Def(process.ParameterFieldName))
	v, err := val.MakeValue("{}")
	if err != nil {
		return defaultValue, err
	}
	paramVal, err := val.LookupValue(process.ParameterFieldName)
	if err != nil {
		// nolint:nilerr
		return defaultValue, nil
	}
	switch k := paramVal.CueValue().IncompleteKind(); k {
	case cue.BottomKind:
		return defaultValue, nil
	default:
		paramOnlyVal := v.CueValue().FillPath(parameterPath, paramVal.CueValue())
		return paramOnlyVal, nil
	}
}

// FillParameterDefinitionFieldIfNotExist refines cue value to merely include `parameter` identifier
func FillParameterDefinitionFieldIfNotExist(val cue.Value) cue.Value {
	defaultValue := cuecontext.New().CompileString("#parameter: {}")
	defPath := cue.ParsePath("#" + process.ParameterFieldName)
	if paramVal := val.LookupPath(cue.ParsePath(process.ParameterFieldName)); paramVal.Exists() {
		if paramVal.IncompleteKind() == cue.BottomKind {
			return defaultValue
		}
		paramOnlyVal := val.Context().CompileString("{}").FillPath(defPath, paramVal)
		return paramOnlyVal
	}
	return defaultValue
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
