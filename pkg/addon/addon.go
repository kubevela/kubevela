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

package addon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"cuelang.org/go/cue"
	cueyaml "cuelang.org/go/encoding/yaml"
	"github.com/google/go-github/v32/github"
	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8syaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	types2 "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	utils2 "github.com/oam-dev/kubevela/pkg/controller/utils"
	cuemodel "github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	version2 "github.com/oam-dev/kubevela/version"
)

const (
	// ReadmeFileName is the addon readme file name
	ReadmeFileName string = "readme.md"

	// MetadataFileName is the addon meatadata.yaml file name
	MetadataFileName string = "metadata.yaml"

	// TemplateFileName is the addon template.yaml file name
	TemplateFileName string = "template.yaml"

	// ResourcesDirName is the addon resources/ dir name
	ResourcesDirName string = "resources"

	// DefinitionsDirName is the addon definitions/ dir name
	DefinitionsDirName string = "definitions"

	// DefSchemaName is the addon definition schemas dir name
	DefSchemaName string = "schemas"

	// AddonParameterDataKey is the key of parameter in addon args secrets
	AddonParameterDataKey string = "addonParameterDataKey"

	// DefaultGiteeURL is the addon repository of gitee api
	DefaultGiteeURL string = "https://gitee.com/api/v5/"
)

// ParameterFileName is the addon resources/parameter.cue file name
var ParameterFileName = filepath.Join("resources", "parameter.cue")

// ListOptions contains flags mark what files should be read in an addon directory
type ListOptions struct {
	GetDetail     bool
	GetDefinition bool
	GetResource   bool
	GetParameter  bool
	GetTemplate   bool
	GetDefSchema  bool
}

var (
	// UIMetaOptions get Addon metadata for UI display
	UIMetaOptions = ListOptions{GetDetail: true, GetDefinition: true, GetParameter: true}

	// CLIMetaOptions get Addon metadata for CLI display
	CLIMetaOptions = ListOptions{}
)

const (
	// ObservabilityAddon is the name of the observability addon
	ObservabilityAddon = "observability"
	// ObservabilityAddonEndpointComponent is the endpoint component name of the observability addon
	ObservabilityAddonEndpointComponent = "grafana"
	// ObservabilityAddonDomainArg is the domain argument name of the observability addon
	ObservabilityAddonDomainArg = "domain"
	// LocalAddonRegistryName is the addon-registry name for those installed by local dir
	LocalAddonRegistryName = "local"
)

// ObservabilityEnvironment contains the Observability addon's domain for each cluster
type ObservabilityEnvironment struct {
	Cluster           string
	Domain            string
	LoadBalancerIP    string
	ServiceExternalIP string
}

// ObservabilityEnvBindingValues is a list of ObservabilityEnvironment and will be used to render observability-env-binding.yaml
type ObservabilityEnvBindingValues struct {
	Envs []ObservabilityEnvironment
}

const (
	// ObservabilityEnvBindingEnvTag is the env Tag for env-binding settings for observability addon
	ObservabilityEnvBindingEnvTag = `        envs:`

	// ObservabilityEnvBindingEnvTmpl is the env values for env-binding settings for observability addon
	ObservabilityEnvBindingEnvTmpl = `
        {{ with .Envs}}
          {{ range . }}
          - name: {{.Cluster}}
            placement:
              clusterSelector:
                name: {{.Cluster}}
          {{ end }}
        {{ end }}`

	// ObservabilityWorkflowStepsTag is the workflow steps Tag for observability addon
	ObservabilityWorkflowStepsTag = `steps:`

	// ObservabilityWorkflow4EnvBindingTmpl is the workflow for env-binding settings for observability addon
	ObservabilityWorkflow4EnvBindingTmpl = `
{{ with .Envs}}
  {{ range . }}
  - name: {{ .Cluster }}
    type: deploy2env
    properties:
      policy: domain
      env: {{ .Cluster }}
      parallel: true
  {{ end }}
{{ end }}`
)

// ErrorNoDomain is the error when no domain is found
var ErrorNoDomain = errors.New("domain is not set")

// Pattern indicates the addon framework file pattern, all files should match at least one of the pattern.
type Pattern struct {
	IsDir bool
	Value string
}

// Patterns is the file pattern that the addon should be in
var Patterns = []Pattern{{Value: ReadmeFileName}, {Value: MetadataFileName}, {Value: TemplateFileName}, {Value: ParameterFileName}, {IsDir: true, Value: ResourcesDirName}, {IsDir: true, Value: DefinitionsDirName}, {IsDir: true, Value: DefSchemaName}}

// GetPatternFromItem will check if the file path has a valid pattern, return empty string if it's invalid.
// AsyncReader is needed to calculate relative path
func GetPatternFromItem(it Item, r AsyncReader, rootPath string) string {
	relativePath := r.RelativePath(it)
	for _, p := range Patterns {
		if strings.HasPrefix(relativePath, filepath.Join(rootPath, p.Value)) {
			return p.Value
		}
	}
	return ""
}

// ListAddonUIDataFromReader list addons from AsyncReader
func ListAddonUIDataFromReader(r AsyncReader, registryMeta map[string]SourceMeta, registryName string, opt ListOptions) ([]*UIData, error) {
	var addons []*UIData
	var err error
	var wg sync.WaitGroup
	var errs []error
	errCh := make(chan error)
	waitCh := make(chan struct{})

	var l sync.Mutex
	for _, subItem := range registryMeta {
		wg.Add(1)
		go func(addonMeta SourceMeta) {
			defer wg.Done()
			addonRes, err := GetUIDataFromReader(r, &addonMeta, opt)
			if err != nil {
				errCh <- err
				return
			}
			addonRes.RegistryName = registryName
			l.Lock()
			addons = append(addons, addonRes)
			l.Unlock()
		}(subItem)
	}
	// in another goroutine for wait group to finish
	go func() {
		wg.Wait()
		close(waitCh)
	}()
forLoop:
	for {
		select {
		case <-waitCh:
			break forLoop
		case err = <-errCh:
			errs = append(errs, err)
		}
	}
	if len(errs) != 0 {
		return addons, compactErrors("error(s) happen when reading from registry: ", errs)
	}
	return addons, nil
}

func compactErrors(message string, errs []error) error {
	errForPrint := make([]string, 0)
	for _, e := range errs {
		errForPrint = append(errForPrint, e.Error())
	}

	return errors.New(message + strings.Join(errForPrint, ","))

}

// GetUIDataFromReader read ui metadata of addon from Reader, used to be displayed in UI
func GetUIDataFromReader(r AsyncReader, meta *SourceMeta, opt ListOptions) (*UIData, error) {
	addonContentsReader := map[string]struct {
		skip bool
		read func(a *UIData, reader AsyncReader, readPath string) error
	}{
		ReadmeFileName:     {!opt.GetDetail, readReadme},
		MetadataFileName:   {false, readMetadata},
		DefinitionsDirName: {!opt.GetDefinition, readDefFile},
		ParameterFileName:  {!opt.GetParameter, readParamFile},
	}
	ptItems := ClassifyItemByPattern(meta, r)
	var addon = &UIData{}
	for contentType, method := range addonContentsReader {
		if method.skip {
			continue
		}
		items := ptItems[contentType]
		for _, it := range items {
			err := method.read(addon, r, r.RelativePath(it))
			if err != nil {
				return nil, fmt.Errorf("fail to read addon %s file %s: %w", meta.Name, r.RelativePath(it), err)
			}
		}
	}

	if opt.GetParameter && addon.Parameters != "" {
		err := genAddonAPISchema(addon)
		if err != nil {
			return nil, fmt.Errorf("fail to generate openAPIschema for addon %s : %w", meta.Name, err)
		}
	}
	return addon, nil
}

// GetInstallPackageFromReader get install package of addon from Reader, this is used to enable an addon
func GetInstallPackageFromReader(r AsyncReader, meta *SourceMeta, uiData *UIData) (*InstallPackage, error) {
	addonContentsReader := map[string]func(a *InstallPackage, reader AsyncReader, readPath string) error{
		TemplateFileName: readTemplate,
		ResourcesDirName: readResFile,
		DefSchemaName:    readDefSchemaFile,
	}
	ptItems := ClassifyItemByPattern(meta, r)

	// Read the installed data from UI metadata object to reduce network payload
	var addon = &InstallPackage{
		Meta:           uiData.Meta,
		Definitions:    uiData.Definitions,
		CUEDefinitions: uiData.CUEDefinitions,
		Parameters:     uiData.Parameters,
	}

	for contentType, method := range addonContentsReader {
		items := ptItems[contentType]
		for _, it := range items {
			err := method(addon, r, r.RelativePath(it))
			if err != nil {
				return nil, fmt.Errorf("fail to read addon %s file %s: %w", meta.Name, r.RelativePath(it), err)
			}
		}
	}

	return addon, nil
}

func readTemplate(a *InstallPackage, reader AsyncReader, readPath string) error {
	data, err := reader.ReadFile(readPath)
	if err != nil {
		return err
	}
	dec := k8syaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	a.AppTemplate = &v1beta1.Application{}

	// try to check it's a valid app template
	_, _, err = dec.Decode([]byte(data), nil, a.AppTemplate)
	if err != nil {
		return err
	}
	return nil
}

// readParamFile read single resource/parameter.cue file
func readParamFile(a *UIData, reader AsyncReader, readPath string) error {
	b, err := reader.ReadFile(readPath)
	if err != nil {
		return err
	}
	a.Parameters = b
	return nil
}

// readResFile read single resource file
func readResFile(a *InstallPackage, reader AsyncReader, readPath string) error {
	filename := path.Base(readPath)
	b, err := reader.ReadFile(readPath)
	if err != nil {
		return err
	}

	if filename == "parameter.cue" {
		return nil
	}
	file := ElementFile{Data: b, Name: path.Base(readPath)}
	switch filepath.Ext(filename) {
	case ".cue":
		a.CUETemplates = append(a.CUETemplates, file)
	default:
		a.YAMLTemplates = append(a.YAMLTemplates, file)
	}
	return nil
}

// readDefSchemaFile read single file of definition schema
func readDefSchemaFile(a *InstallPackage, reader AsyncReader, readPath string) error {
	b, err := reader.ReadFile(readPath)
	if err != nil {
		return err
	}
	a.DefSchemas = append(a.DefSchemas, ElementFile{Data: b, Name: path.Base(readPath)})
	return nil
}

// readDefFile read single definition file
func readDefFile(a *UIData, reader AsyncReader, readPath string) error {
	b, err := reader.ReadFile(readPath)
	if err != nil {
		return err
	}
	filename := path.Base(readPath)
	file := ElementFile{Data: b, Name: path.Base(readPath)}
	switch filepath.Ext(filename) {
	case ".cue":
		a.CUEDefinitions = append(a.CUEDefinitions, file)
	default:
		a.Definitions = append(a.Definitions, file)
	}
	return nil
}

func readMetadata(a *UIData, reader AsyncReader, readPath string) error {
	b, err := reader.ReadFile(readPath)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal([]byte(b), &a.Meta)
	if err != nil {
		return err
	}
	return nil
}

func readReadme(a *UIData, reader AsyncReader, readPath string) error {
	content, err := reader.ReadFile(readPath)
	if err != nil {
		return err
	}
	a.Detail = content
	return nil
}

func createGitHelper(content *utils.Content, token string) *gitHelper {
	var ts oauth2.TokenSource
	if token != "" {
		ts = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	}
	tc := oauth2.NewClient(context.Background(), ts)
	tc.Timeout = time.Second * 20
	cli := github.NewClient(tc)
	return &gitHelper{
		Client: cli,
		Meta:   content,
	}
}

func createGiteeHelper(content *utils.Content, token string) *giteeHelper {
	var ts oauth2.TokenSource
	if token != "" {
		ts = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	}
	tc := oauth2.NewClient(context.Background(), ts)
	tc.Timeout = time.Second * 20
	cli := NewGiteeClient(tc, nil)
	return &giteeHelper{
		Client: cli,
		Meta:   content,
	}
}

// readRepo will read relative path (relative to Meta.Path)
func (h *gitHelper) readRepo(relativePath string) (*github.RepositoryContent, []*github.RepositoryContent, error) {
	file, items, _, err := h.Client.Repositories.GetContents(context.Background(), h.Meta.GithubContent.Owner, h.Meta.GithubContent.Repo, path.Join(h.Meta.GithubContent.Path, relativePath), nil)
	if err != nil {
		return nil, nil, WrapErrRateLimit(err)
	}
	return file, items, nil
}

// readRepo will read relative path (relative to Meta.Path)
func (h *giteeHelper) readRepo(relativePath string) (*github.RepositoryContent, []*github.RepositoryContent, error) {
	file, items, err := h.Client.GetGiteeContents(context.Background(), h.Meta.GiteeContent.Owner, h.Meta.GiteeContent.Repo, path.Join(h.Meta.GiteeContent.Path, relativePath), h.Meta.GiteeContent.Ref)
	if err != nil {
		return nil, nil, WrapErrRateLimit(err)
	}
	return file, items, nil
}

func (c *Client) GetGiteeContents(ctx context.Context, owner, repo, path, ref string) (fileContent *github.RepositoryContent, directoryContent []*github.RepositoryContent, err error) {
	escapedPath := (&url.URL{Path: path}).String()
	u := fmt.Sprintf(c.BaseURL.String()+"repos/%s/%s/contents/%s", owner, repo, escapedPath)
	if ref != "" {
		u = fmt.Sprintf(u+"?ref=%s", ref)
	}

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}
	response, err := c.Client.Do(req.WithContext(ctx))
	defer response.Body.Close()
	if err != nil {
		return nil, nil, err
	}
	content, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, nil, err
	}
	fileUnmarshalError := json.Unmarshal(content, &fileContent)
	if fileUnmarshalError == nil {
		return fileContent, nil, nil
	}
	directoryUnmarshalError := json.Unmarshal(content, &directoryContent)
	if directoryUnmarshalError == nil {
		return nil, directoryContent, nil
	}
	return nil, nil, fmt.Errorf("unmarshalling failed for both file and directory content: %s and %s", fileUnmarshalError, directoryUnmarshalError)
}

func genAddonAPISchema(addonRes *UIData) error {
	param, err := utils2.PrepareParameterCue(addonRes.Name, addonRes.Parameters)
	if err != nil {
		return err
	}
	var r cue.Runtime
	cueInst, err := r.Compile("-", param)
	if err != nil {
		return err
	}
	data, err := common.GenOpenAPI(cueInst)
	if err != nil {
		return err
	}
	schema, err := utils2.ConvertOpenAPISchema2SwaggerObject(data)
	if err != nil {
		return err
	}
	utils2.FixOpenAPISchema("", schema)
	addonRes.APISchema = schema
	return nil
}

// RenderApp render a K8s application
func RenderApp(ctx context.Context, addon *InstallPackage, config *rest.Config, k8sClient client.Client, args map[string]interface{}) (*v1beta1.Application, error) {
	if args == nil {
		args = map[string]interface{}{}
	}
	app := addon.AppTemplate
	if app == nil {
		app = &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: "Application"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      Convert2AppName(addon.Name),
				Namespace: types.DefaultKubeVelaNS,
				Labels: map[string]string{
					oam.LabelAddonName: addon.Name,
				},
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common2.ApplicationComponent{},
			},
		}
	}
	app.Labels = util.MergeMapOverrideWithDst(app.Labels, map[string]string{oam.LabelAddonName: addon.Name})

	// force override the namespace defined vela with DefaultVelaNS,this value can be modified by Env
	app.SetNamespace(types.DefaultKubeVelaNS)

	for _, namespace := range addon.NeedNamespace {
		// vela-system must exist before rendering vela addon
		if namespace == types.DefaultKubeVelaNS {
			continue
		}
		comp := common2.ApplicationComponent{
			Type:       "raw",
			Name:       fmt.Sprintf("%s-namespace", namespace),
			Properties: util.Object2RawExtension(renderNamespace(namespace)),
		}
		app.Spec.Components = append(app.Spec.Components, comp)
	}

	if len(addon.YAMLTemplates) != 0 {
		comp, err := renderK8sObjectsComponent(addon.YAMLTemplates, addon.Name)
		if err != nil {
			return nil, err
		}
		app.Spec.Components = append(app.Spec.Components, *comp)
	}

	for _, tmpl := range addon.CUETemplates {
		comp, err := renderCUETemplate(tmpl, addon.Parameters, args)
		if err != nil {
			return nil, ErrRenderCueTmpl
		}
		if addon.Name == ObservabilityAddon && strings.HasSuffix(comp.Name, ".cue") {
			comp.Name = strings.Split(comp.Name, ".cue")[0]
		}
		app.Spec.Components = append(app.Spec.Components, *comp)
	}

	switch {
	case isDeployToRuntimeOnly(addon):
		if app.Spec.Workflow == nil {
			app.Spec.Workflow = &v1beta1.Workflow{Steps: make([]v1beta1.WorkflowStep, 0)}
		}
		app.Spec.Workflow.Steps = append(app.Spec.Workflow.Steps,
			v1beta1.WorkflowStep{
				Name: "deploy-control-plane",
				Type: "apply-application",
			},
			v1beta1.WorkflowStep{
				Name: "deploy-runtime",
				Type: "deploy2runtime",
			})
	case addon.Name == ObservabilityAddon:
		clusters, err := allocateDomainForAddon(ctx, k8sClient)
		if err != nil {
			return nil, err
		}

		policies, err := preparePolicies4Observability(clusters)
		if err != nil {
			return nil, errors.Wrap(err, "fail to render the policies for Add-on Observability")
		}
		app.Spec.Policies = policies

		if len(clusters) > 0 {
			app.Spec.Workflow = &v1beta1.Workflow{
				Steps: []v1beta1.WorkflowStep{{
					Name: "deploy-control-plane",
					Type: "apply-application-in-parallel",
				}},
			}
		} else {
			app.Spec.Workflow = &v1beta1.Workflow{
				Steps: []v1beta1.WorkflowStep{{
					Name: "deploy-control-plane",
					Type: "apply-application",
				}},
			}
		}

		workflowSteps, err := prepareWorkflow4Observability(clusters)
		if err != nil {
			return nil, errors.Wrap(err, "fail to prepare the workflow for Add-on Observability")
		}
		app.Spec.Workflow.Steps = append(app.Spec.Workflow.Steps, workflowSteps...)

	default:
		for _, cueDef := range addon.CUEDefinitions {
			def := definition.Definition{Unstructured: unstructured.Unstructured{}}
			err := def.FromCUEString(cueDef.Data, config)
			if err != nil {
				return nil, errors.Wrapf(err, "fail to render definition: %s in cue's format", cueDef.Name)
			}
			if def.Unstructured.GetNamespace() == "" {
				def.Unstructured.SetNamespace(types.DefaultKubeVelaNS)
			}
			app.Spec.Components = append(app.Spec.Components, common2.ApplicationComponent{
				Name:       cueDef.Name,
				Type:       "raw",
				Properties: util.Object2RawExtension(&def.Unstructured),
			})
		}
		for _, teml := range addon.DefSchemas {
			u, err := renderSchemaConfigmap(teml)
			if err != nil {
				return nil, err
			}
			app.Spec.Components = append(app.Spec.Components, common2.ApplicationComponent{
				Name:       teml.Name,
				Type:       "raw",
				Properties: util.Object2RawExtension(u),
			})
		}
		// set to nil so workflow mode will be set to "DAG" automatically
		if app.Spec.Workflow != nil && len(app.Spec.Workflow.Steps) == 0 {
			app.Spec.Workflow = nil
		}
	}

	return app, nil
}

// RenderDefinitions render definition objects if needed
func RenderDefinitions(addon *InstallPackage, config *rest.Config) ([]*unstructured.Unstructured, error) {
	defObjs := make([]*unstructured.Unstructured, 0)

	// No matter runtime mode or control mode , definition only needs to control plane k8s.
	for _, def := range addon.Definitions {
		obj, err := renderObject(def)
		if err != nil {
			return nil, err
		}
		// we should ignore the namespace defined in definition yaml, override the filed by DefaultKubeVelaNS
		obj.SetNamespace(types.DefaultKubeVelaNS)
		defObjs = append(defObjs, obj)
	}

	for _, cueDef := range addon.CUEDefinitions {
		def := definition.Definition{Unstructured: unstructured.Unstructured{}}
		err := def.FromCUEString(cueDef.Data, config)
		if err != nil {
			return nil, errors.Wrapf(err, "fail to render definition: %s in cue's format", cueDef.Name)
		}
		// we should ignore the namespace defined in definition yaml, override the filed by DefaultKubeVelaNS
		def.SetNamespace(types.DefaultKubeVelaNS)
		defObjs = append(defObjs, &def.Unstructured)
	}

	return defObjs, nil
}

// RenderDefinitionSchema will render definitions' schema in addons.
func RenderDefinitionSchema(addon *InstallPackage) ([]*unstructured.Unstructured, error) {
	schemaConfigmaps := make([]*unstructured.Unstructured, 0)

	if isDeployToRuntimeOnly(addon) {
		for _, teml := range addon.DefSchemas {
			u, err := renderSchemaConfigmap(teml)
			if err != nil {
				return nil, err
			}
			schemaConfigmaps = append(schemaConfigmaps, u)
		}
	}
	return schemaConfigmaps, nil
}

func allocateDomainForAddon(ctx context.Context, k8sClient client.Client) ([]ObservabilityEnvironment, error) {
	secrets, err := multicluster.ListExistingClusterSecrets(ctx, k8sClient)
	if err != nil {
		klog.Error(err, "failed to list existing cluster secrets")
		return nil, err
	}

	envs := make([]ObservabilityEnvironment, len(secrets))

	for i, secret := range secrets {
		cluster := secret.Name
		envs[i] = ObservabilityEnvironment{
			Cluster: cluster,
		}
	}

	return envs, nil
}

func preparePolicies4Observability(clusters []ObservabilityEnvironment) ([]v1beta1.AppPolicy, error) {
	if clusters == nil {
		return nil, nil
	}
	envProperties, err := render(clusters, ObservabilityEnvBindingEnvTmpl)
	if err != nil {
		return nil, err
	}

	var properties runtime.RawExtension
	envs := fmt.Sprintf("%s\n%s", ObservabilityEnvBindingEnvTag, envProperties)
	envJSON, err := yaml.YAMLToJSON([]byte(envs))
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(envJSON, &properties)
	if err != nil {
		return nil, err
	}

	policies := []v1beta1.AppPolicy{{
		Name:       "domain",
		Type:       "env-binding",
		Properties: &properties,
	}}

	return policies, nil
}

func prepareWorkflow4Observability(clusters []ObservabilityEnvironment) ([]v1beta1.WorkflowStep, error) {
	envBindingWorkflow, err := render(clusters, ObservabilityWorkflow4EnvBindingTmpl)
	if err != nil {
		return nil, err
	}

	var workflow v1beta1.Workflow
	envs := fmt.Sprintf("%s\n%s", ObservabilityWorkflowStepsTag, envBindingWorkflow)
	envJSON, err := yaml.YAMLToJSON([]byte(envs))
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(envJSON, &workflow)
	if err != nil {
		return nil, err
	}

	return workflow.Steps, nil
}

func render(envs []ObservabilityEnvironment, tmpl string) (string, error) {
	todos := ObservabilityEnvBindingValues{
		Envs: envs,
	}

	t := template.Must(template.New("grafana").Parse(tmpl))
	var rendered bytes.Buffer
	err := t.Execute(&rendered, todos)
	if err != nil {
		return "", err
	}

	return rendered.String(), nil
}

func isDeployToRuntimeOnly(addon *InstallPackage) bool {
	if addon.DeployTo == nil {
		return false
	}
	return addon.DeployTo.RuntimeCluster || addon.DeployTo.LegacyRuntimeCluster
}

func renderObject(elem ElementFile) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	dec := k8syaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err := dec.Decode([]byte(elem.Data), nil, obj)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func renderNamespace(namespace string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion("v1")
	u.SetKind("Namespace")
	u.SetName(namespace)
	return u
}

func renderK8sObjectsComponent(elems []ElementFile, addonName string) (*common2.ApplicationComponent, error) {
	var objects []*unstructured.Unstructured
	for _, elem := range elems {
		obj, err := renderObject(elem)
		if err != nil {
			return nil, err
		}
		objects = append(objects, obj)
	}
	properties := map[string]interface{}{"objects": objects}
	propJSON, err := json.Marshal(properties)
	if err != nil {
		return nil, err
	}
	baseRawComponent := common2.ApplicationComponent{
		Type:       "k8s-objects",
		Name:       addonName + "-resources",
		Properties: &runtime.RawExtension{Raw: propJSON},
	}
	return &baseRawComponent, nil
}

func renderSchemaConfigmap(elem ElementFile) (*unstructured.Unstructured, error) {
	jsonData, err := yaml.YAMLToJSON([]byte(elem.Data))
	if err != nil {
		return nil, err
	}
	cm := v1.ConfigMap{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{Namespace: types.DefaultKubeVelaNS, Name: strings.Split(elem.Name, ".")[0]},
		Data: map[string]string{
			types.UISchema: string(jsonData),
		}}
	return util.Object2Unstructured(cm)
}

// renderCUETemplate will return a component from cue template
func renderCUETemplate(elem ElementFile, parameters string, args map[string]interface{}) (*common2.ApplicationComponent, error) {
	bt, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	var paramFile = cuemodel.ParameterFieldName + ": {}"
	if string(bt) != "null" {
		paramFile = fmt.Sprintf("%s: %s", cuemodel.ParameterFieldName, string(bt))
	}
	param := fmt.Sprintf("%s\n%s", paramFile, parameters)
	v, err := value.NewValue(param, nil, "")
	if err != nil {
		return nil, err
	}
	out, err := v.LookupByScript(elem.Data)
	if err != nil {
		return nil, err
	}
	compContent, err := out.LookupValue("output")
	if err != nil {
		return nil, err
	}
	b, err := cueyaml.Encode(compContent.CueValue())
	if err != nil {
		return nil, err
	}
	fileName := strings.ReplaceAll(elem.Name, path.Ext(elem.Name), "")
	comp := common2.ApplicationComponent{
		Name: strings.ReplaceAll(fileName, ".", "-"),
	}
	err = yaml.Unmarshal(b, &comp)
	if err != nil {
		return nil, err
	}

	return &comp, err
}

const addonAppPrefix = "addon-"
const addonSecPrefix = "addon-secret-"

// Convert2AppName -
func Convert2AppName(name string) string {
	return addonAppPrefix + name
}

// RenderArgsSecret render addon enable argument to secret
func RenderArgsSecret(addon *InstallPackage, args map[string]interface{}) *unstructured.Unstructured {
	argsByte, err := json.Marshal(args)
	if err != nil {
		return nil
	}
	sec := v1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      Convert2SecName(addon.Name),
			Namespace: types.DefaultKubeVelaNS,
		},
		Data: map[string][]byte{
			AddonParameterDataKey: argsByte,
		},
		Type: v1.SecretTypeOpaque,
	}
	u, err := util.Object2Unstructured(sec)
	if err != nil {
		return nil
	}
	return u
}

// FetchArgsFromSecret fetch addon args from secrets
func FetchArgsFromSecret(sec *v1.Secret) (map[string]interface{}, error) {
	res := map[string]interface{}{}
	if args, ok := sec.Data[AddonParameterDataKey]; ok {
		err := json.Unmarshal(args, &res)
		if err != nil {
			return nil, err
		}
		return res, nil
	}

	// this is backward compatibility code for old way to storage parameter
	res = make(map[string]interface{}, len(sec.Data))
	for k, v := range sec.Data {
		res[k] = string(v)
	}
	return res, nil
}

// Convert2SecName generate addon argument secret name
func Convert2SecName(name string) string {
	return addonSecPrefix + name
}

// Installer helps addon enable, dependency-check, dispatch resources
type Installer struct {
	ctx          context.Context
	config       *rest.Config
	addon        *InstallPackage
	cli          client.Client
	apply        apply.Applicator
	r            *Registry
	registryMeta map[string]SourceMeta
	args         map[string]interface{}
	cache        *Cache
	dc           *discovery.DiscoveryClient
}

// NewAddonInstaller will create an installer for addon
func NewAddonInstaller(ctx context.Context, cli client.Client, discoveryClient *discovery.DiscoveryClient, apply apply.Applicator, config *rest.Config, r *Registry, args map[string]interface{}, cache *Cache) Installer {
	return Installer{
		ctx:    ctx,
		config: config,
		cli:    cli,
		apply:  apply,
		r:      r,
		args:   args,
		cache:  cache,
		dc:     discoveryClient,
	}
}

func (h *Installer) enableAddon(addon *InstallPackage) error {
	var err error
	h.addon = addon
	err = checkAddonVersionMeetRequired(h.ctx, addon.SystemRequirements, h.cli, h.dc)
	if err != nil {
		return ErrVersionMismatch
	}

	if err = h.installDependency(addon); err != nil {
		return err
	}
	if err = h.dispatchAddonResource(addon); err != nil {
		return err
	}
	// we shouldn't put continue func into dispatchAddonResource, because the re-apply app maybe already update app and
	// the suspend will set with false automatically
	if err := h.continueOrRestartWorkflow(); err != nil {
		return err
	}
	return nil
}

func (h *Installer) loadInstallPackage(name string) (*InstallPackage, error) {
	metas, err := h.getAddonMeta()
	if err != nil {
		return nil, errors.Wrap(err, "fail to get addon meta")
	}

	meta, ok := metas[name]
	if !ok {
		return nil, ErrNotExist
	}
	var uiData *UIData
	uiData, err = h.cache.GetUIData(*h.r, name)
	if err != nil {
		return nil, err
	}
	// enable this addon if it's invisible
	installPackage, err := h.r.GetInstallPackage(&meta, uiData)
	if err != nil {
		return nil, errors.Wrap(err, "fail to find dependent addon in source repository")
	}
	return installPackage, nil
}

func (h *Installer) getAddonMeta() (map[string]SourceMeta, error) {
	var err error
	if h.registryMeta == nil {
		if h.registryMeta, err = h.cache.ListAddonMeta(*h.r); err != nil {
			return nil, err
		}
	}
	return h.registryMeta, nil
}

// installDependency checks if addon's dependency and install it
func (h *Installer) installDependency(addon *InstallPackage) error {
	var app v1beta1.Application
	for _, dep := range addon.Dependencies {
		err := h.cli.Get(h.ctx, client.ObjectKey{
			Namespace: types.DefaultKubeVelaNS,
			Name:      Convert2AppName(dep.Name),
		}, &app)
		if err == nil {
			continue
		}
		if !apierrors.IsNotFound(err) {
			return err
		}
		depAddon, err := h.loadInstallPackage(dep.Name)
		if err != nil {
			return err
		}
		depHandler := *h
		depHandler.args = nil
		if err = depHandler.enableAddon(depAddon); err != nil {
			return errors.Wrap(err, "fail to dispatch dependent addon resource")
		}
	}
	return nil
}

// checkDependency checks if addon's dependency
func (h *Installer) checkDependency(addon *InstallPackage) ([]string, error) {
	var app v1beta1.Application
	var needEnable []string
	for _, dep := range addon.Dependencies {
		err := h.cli.Get(h.ctx, client.ObjectKey{
			Namespace: types.DefaultKubeVelaNS,
			Name:      Convert2AppName(dep.Name),
		}, &app)
		if err == nil {
			continue
		}
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		needEnable = append(needEnable, dep.Name)
	}
	return needEnable, nil
}

func (h *Installer) dispatchAddonResource(addon *InstallPackage) error {
	app, err := RenderApp(h.ctx, addon, h.config, h.cli, h.args)
	if err != nil {
		return errors.Wrap(err, "render addon application fail")
	}

	appName, err := determineAddonAppName(h.ctx, h.cli, h.addon.Name)
	if err != nil {
		return err
	}
	app.Name = appName

	app.SetLabels(util.MergeMapOverrideWithDst(app.GetLabels(), map[string]string{oam.LabelAddonRegistry: h.r.Name}))

	defs, err := RenderDefinitions(addon, h.config)
	if err != nil {
		return errors.Wrap(err, "render addon definitions fail")
	}

	schemas, err := RenderDefinitionSchema(addon)
	if err != nil {
		return errors.Wrap(err, "render addon definitions' schema fail")
	}

	err = h.apply.Apply(h.ctx, app, apply.DisableUpdateAnnotation())
	if err != nil {
		klog.Errorf("fail to create application: %v", err)
		return errors.Wrap(err, "fail to create application")
	}

	for _, def := range defs {
		addOwner(def, app)
		err = h.apply.Apply(h.ctx, def, apply.DisableUpdateAnnotation())
		if err != nil {
			return err
		}
	}

	for _, schema := range schemas {
		addOwner(schema, app)
		err = h.apply.Apply(h.ctx, schema, apply.DisableUpdateAnnotation())
		if err != nil {
			return err
		}
	}

	if h.args != nil && len(h.args) > 0 {
		sec := RenderArgsSecret(addon, h.args)
		addOwner(sec, app)
		err = h.apply.Apply(h.ctx, sec, apply.DisableUpdateAnnotation())
		if err != nil {
			return err
		}
	}
	return nil
}

// this func will handle such two case
// 1. if last apply failed an workflow have suspend, this func will continue the workflow
// 2. restart the workflow, if the new cluster have been added in KubeVela
func (h *Installer) continueOrRestartWorkflow() error {
	app, err := FetchAddonRelatedApp(h.ctx, h.cli, h.addon.Name)
	if err != nil {
		return err
	}

	switch {
	// this case means user add a new cluster and user want to restart workflow to dispatch addon resources to new cluster
	// re-apply app won't help app restart workflow
	case app.Status.Phase == common2.ApplicationRunning:
		// we can use retry on conflict here in CLI, because we want to update the status in this CLI operation.
		return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
			if err = h.cli.Get(h.ctx, client.ObjectKey{Namespace: app.Namespace, Name: app.Name}, app); err != nil {
				return
			}
			app.Status.Workflow = nil
			return h.cli.Status().Update(h.ctx, app)
		})
	// this case means addon last installation meet some error and workflow has been suspended by app controller
	// re-apply app won't help app workflow continue
	case app.Status.Workflow != nil && app.Status.Workflow.Suspend:
		// we can use retry on conflict here in CLI, because we want to update the status in this CLI operation.
		return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
			if err = h.cli.Get(h.ctx, client.ObjectKey{Namespace: app.Namespace, Name: app.Name}, app); err != nil {
				return
			}
			mergePatch := client.MergeFrom(app.DeepCopy())
			app.Status.Workflow.Suspend = false
			return h.cli.Status().Patch(h.ctx, app, mergePatch)
		})
	}
	return nil
}

func addOwner(child *unstructured.Unstructured, app *v1beta1.Application) {
	child.SetOwnerReferences(append(child.GetOwnerReferences(),
		*metav1.NewControllerRef(app, v1beta1.ApplicationKindVersionKind)))
}

// determine app name, if app is already exist, use the application name
func determineAddonAppName(ctx context.Context, cli client.Client, addonName string) (string, error) {
	app, err := FetchAddonRelatedApp(ctx, cli, addonName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return "", err
		}
		// if the app still not exist, use addon-{addonName}
		return Convert2AppName(addonName), nil
	}
	return app.Name, nil
}

// FetchAddonRelatedApp will fetch the addon related app, this func will use NamespacedName(vela-system, addon-addonName) to get app
// if not find will try to get 1.1 legacy addon related app by using NamespacedName(vela-system, `addonName`)
func FetchAddonRelatedApp(ctx context.Context, cli client.Client, addonName string) (*v1beta1.Application, error) {
	app := &v1beta1.Application{}
	if err := cli.Get(ctx, types2.NamespacedName{Namespace: types.DefaultKubeVelaNS, Name: Convert2AppName(addonName)}, app); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		// for 1.1 addon app compatibility code
		if err := cli.Get(ctx, types2.NamespacedName{Namespace: types.DefaultKubeVelaNS, Name: addonName}, app); err != nil {
			return nil, err
		}
	}
	return app, nil
}

// checkAddonVersionMeetRequired will check the version of cli/ux and kubevela-core-controller whether meet the addon requirement, if not will return an error
// please notice that this func is for check production environment which vela cli/ux or vela core is officalVersion
// if version is for test or debug eg: latest/commit-id/branch-name this func will return nil error
func checkAddonVersionMeetRequired(ctx context.Context, require *SystemRequirements, k8sClient client.Client, dc *discovery.DiscoveryClient) error {
	if require == nil {
		return nil
	}

	// if not semver version, bypass check cli/ux. eg: {branch name/git commit id/UNKNOWN}
	if version2.IsOfficialKubeVelaVersion(version2.VelaVersion) {
		res, err := checkSemVer(version2.VelaVersion, require.VelaVersion)
		if err != nil {
			return err
		}
		if !res {
			return fmt.Errorf("vela cli/ux version: %s cannot meet requirement", version2.VelaVersion)
		}
	}

	// check vela core controller version
	imageVersion, err := fetchVelaCoreImageTag(ctx, k8sClient)
	if err != nil {
		return err
	}

	// if not semver version, bypass check vela-core.
	if version2.IsOfficialKubeVelaVersion(imageVersion) {
		res, err := checkSemVer(imageVersion, require.VelaVersion)
		if err != nil {
			return err
		}
		if !res {
			return fmt.Errorf("the vela core controller: %s cannot meet requirement ", imageVersion)
		}
	}

	// discovery client is nil so bypass check kubernetes version
	if dc == nil {
		return nil
	}

	k8sVersion, err := dc.ServerVersion()
	if err != nil {
		return err
	}
	// if not semver version, bypass check kubernetes version.
	if version2.IsOfficialKubeVelaVersion(k8sVersion.GitVersion) {
		res, err := checkSemVer(k8sVersion.GitVersion, require.KubernetesVersion)
		if err != nil {
			return err
		}

		if !res {
			return fmt.Errorf("the kubernetes version %s cannot meet requirement", k8sVersion.GitVersion)
		}
	}

	return nil
}

func checkSemVer(actual string, require string) (bool, error) {
	if len(require) == 0 {
		return true, nil
	}
	smeVer := strings.TrimPrefix(actual, "v")
	l := strings.ReplaceAll(require, "v", " ")
	constraint, err := version.NewConstraint(l)
	if err != nil {
		return false, err
	}
	v, err := version.NewVersion(smeVer)
	if err != nil {
		return false, err
	}
	return constraint.Check(v), nil
}

func fetchVelaCoreImageTag(ctx context.Context, k8sClient client.Client) (string, error) {
	deploy := &appsv1.Deployment{}
	if err := k8sClient.Get(ctx, types2.NamespacedName{Namespace: types.DefaultKubeVelaNS, Name: types.KubeVelaControllerDeployment}, deploy); err != nil {
		return "", err
	}
	var tag string
	for _, c := range deploy.Spec.Template.Spec.Containers {
		if c.Name == types.DefaultKubeVelaReleaseName {
			l := strings.Split(c.Image, ":")
			if len(l) == 1 {
				// if tag is empty mean use latest image
				return "latest", nil
			}
			tag = l[1]
		}
	}
	return tag, nil
}
