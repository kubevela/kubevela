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
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-github/v32/github"
	"github.com/imdario/mergo"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/pkg/errors"
	"github.com/xanzy/go-gitlab"
	"golang.org/x/oauth2"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8syaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	types2 "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	stringslices "k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/cue/script"
	"github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils"
	addonutil "github.com/oam-dev/kubevela/pkg/utils/addon"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/velaql"
	version2 "github.com/oam-dev/kubevela/version"
)

const (
	// ReadmeFileName is the addon readme file name
	ReadmeFileName string = "README.md"

	// LegacyReadmeFileName is the addon readme lower case file name
	LegacyReadmeFileName string = "readme.md"

	// MetadataFileName is the addon meatadata.yaml file name
	MetadataFileName string = "metadata.yaml"

	// TemplateFileName is the addon template.yaml file name
	TemplateFileName string = "template.yaml"

	// AppTemplateCueFileName is the addon application template.cue file name
	AppTemplateCueFileName string = "template.cue"

	// NotesCUEFileName is the addon notes print to end users when installed
	NotesCUEFileName string = "NOTES.cue"

	// KeyWordNotes is the keyword in NOTES.cue which will render the notes message out.
	KeyWordNotes string = "notes"

	// GlobalParameterFileName is the addon global parameter.cue file name
	GlobalParameterFileName string = "parameter.cue"

	// ResourcesDirName is the addon resources/ dir name
	ResourcesDirName string = "resources"

	// DefinitionsDirName is the addon definitions/ dir name
	DefinitionsDirName string = "definitions"

	// ConfigTemplateDirName is the addon config-templates/ dir name
	ConfigTemplateDirName string = "config-templates"

	// DefSchemaName is the addon definition schemas dir name
	DefSchemaName string = "schemas"

	// ViewDirName is the addon views dir name
	ViewDirName string = "views"

	// AddonParameterDataKey is the key of parameter in addon args secrets
	AddonParameterDataKey string = "addonParameterDataKey"

	// DefaultGiteeURL is the addon repository of gitee api
	DefaultGiteeURL string = "https://gitee.com/api/v5/"

	// InstallerRuntimeOption inject install runtime info into addon options
	InstallerRuntimeOption string = "installerRuntimeOption"
)

// ParameterFileName is the addon resources/parameter.cue file name
var ParameterFileName = strings.Join([]string{"resources", "parameter.cue"}, "/")

// ListOptions contains flags mark what files should be read in an addon directory
type ListOptions struct {
	GetDetail         bool
	GetDefinition     bool
	GetConfigTemplate bool
	GetResource       bool
	GetParameter      bool
	GetTemplate       bool
	GetDefSchema      bool
}

var (
	// UIMetaOptions get Addon metadata for UI display
	UIMetaOptions = ListOptions{GetDetail: true, GetDefinition: true, GetParameter: true, GetConfigTemplate: true}

	// CLIMetaOptions get Addon metadata for CLI display
	CLIMetaOptions = ListOptions{}

	// UnInstallOptions used for addon uninstalling
	UnInstallOptions = ListOptions{GetDefinition: true}
)

const (
	// LocalAddonRegistryName is the addon-registry name for those installed by local dir
	LocalAddonRegistryName = "local"
	// ClusterLabelSelector define the key of topology cluster label selector
	ClusterLabelSelector = "clusterLabelSelector"
)

// Pattern indicates the addon framework file pattern, all files should match at least one of the pattern.
type Pattern struct {
	IsDir bool
	Value string
}

// Patterns is the file pattern that the addon should be in
var Patterns = []Pattern{
	// config-templates pattern
	{IsDir: true, Value: ConfigTemplateDirName},
	// single file reader pattern
	{Value: ReadmeFileName}, {Value: MetadataFileName}, {Value: TemplateFileName},
	// parameter in resource directory
	{Value: ParameterFileName},
	// directory files
	{IsDir: true, Value: ResourcesDirName}, {IsDir: true, Value: DefinitionsDirName}, {IsDir: true, Value: DefSchemaName}, {IsDir: true, Value: ViewDirName},
	// CUE app template, parameter and notes
	{Value: AppTemplateCueFileName}, {Value: GlobalParameterFileName}, {Value: NotesCUEFileName},
	{Value: LegacyReadmeFileName}}

// GetPatternFromItem will check if the file path has a valid pattern, return empty string if it's invalid.
// AsyncReader is needed to calculate relative path
func GetPatternFromItem(it Item, r AsyncReader, rootPath string) string {
	relativePath := r.RelativePath(it)
	for _, p := range Patterns {
		if strings.HasPrefix(relativePath, strings.Join([]string{rootPath, p.Value}, "/")) {
			return p.Value
		}
		if strings.HasPrefix(relativePath, filepath.Join(rootPath, p.Value)) {
			// for enable addon by load dir, compatible with linux or windows os
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
		ReadmeFileName:          {!opt.GetDetail, readReadme},
		LegacyReadmeFileName:    {!opt.GetDetail, readReadme},
		MetadataFileName:        {false, readMetadata},
		DefinitionsDirName:      {!opt.GetDefinition, readDefFile},
		ConfigTemplateDirName:   {!opt.GetConfigTemplate, readConfigTemplateFile},
		ParameterFileName:       {!opt.GetParameter, readParamFile},
		GlobalParameterFileName: {!opt.GetParameter, readGlobalParamFile},
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

	if opt.GetParameter && (len(addon.Parameters) != 0 || len(addon.GlobalParameters) != 0) {
		if addon.GlobalParameters != "" {
			if addon.Parameters != "" {
				klog.Warning("both legacy parameter and global parameter are provided, but only global parameter will be used. Consider removing the legacy parameters.")
			}
			addon.Parameters = addon.GlobalParameters
		}
		err := genAddonAPISchema(addon)
		if err != nil {
			return nil, fmt.Errorf("fail to generate openAPIschema for addon %s : %w (parameter: %s)", meta.Name, err, addon.Parameters)
		}
	}
	addon.AvailableVersions = []string{addon.Version}
	return addon, nil
}

// GetInstallPackageFromReader get install package of addon from Reader, this is used to enable an addon
func GetInstallPackageFromReader(r AsyncReader, meta *SourceMeta, uiData *UIData) (*InstallPackage, error) {
	addonContentsReader := map[string]func(a *InstallPackage, reader AsyncReader, readPath string) error{
		TemplateFileName:       readTemplate,
		ResourcesDirName:       readResFile,
		DefSchemaName:          readDefSchemaFile,
		ViewDirName:            readViewFile,
		AppTemplateCueFileName: readAppCueTemplate,
		NotesCUEFileName:       readNotesFile,
	}
	ptItems := ClassifyItemByPattern(meta, r)

	// Read the installed data from UI metadata object to reduce network payload
	var addon = &InstallPackage{
		Meta:            uiData.Meta,
		Definitions:     uiData.Definitions,
		CUEDefinitions:  uiData.CUEDefinitions,
		Parameters:      uiData.Parameters,
		ConfigTemplates: uiData.ConfigTemplates,
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

func readAppCueTemplate(a *InstallPackage, reader AsyncReader, readPath string) error {
	data, err := reader.ReadFile(readPath)
	if err != nil {
		return err
	}
	a.AppCueTemplate = ElementFile{Data: data, Name: filepath.Base(readPath)}
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

// readNotesFile read single NOTES.cue file
func readNotesFile(a *InstallPackage, reader AsyncReader, readPath string) error {
	data, err := reader.ReadFile(readPath)
	if err != nil {
		return err
	}
	a.Notes = ElementFile{Data: data, Name: filepath.Base(readPath)}
	return nil
}

// readGlobalParamFile read global parameter file.
func readGlobalParamFile(a *UIData, reader AsyncReader, readPath string) error {
	b, err := reader.ReadFile(readPath)
	if err != nil {
		return err
	}
	a.GlobalParameters = b
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
	file := ElementFile{Data: b, Name: filepath.Base(readPath)}
	switch filepath.Ext(filename) {
	case ".cue":
		a.CUETemplates = append(a.CUETemplates, file)
	case ".yaml", ".yml":
		a.YAMLTemplates = append(a.YAMLTemplates, file)
	default:
		// skip other file formats
	}
	return nil
}

// readDefSchemaFile read single file of definition schema
func readDefSchemaFile(a *InstallPackage, reader AsyncReader, readPath string) error {
	b, err := reader.ReadFile(readPath)
	if err != nil {
		return err
	}
	a.DefSchemas = append(a.DefSchemas, ElementFile{Data: b, Name: filepath.Base(readPath)})
	return nil
}

// readDefFile read single definition file
func readDefFile(a *UIData, reader AsyncReader, readPath string) error {
	b, err := reader.ReadFile(readPath)
	if err != nil {
		return err
	}
	filename := path.Base(readPath)
	file := ElementFile{Data: b, Name: filepath.Base(readPath)}
	switch filepath.Ext(filename) {
	case ".cue":
		a.CUEDefinitions = append(a.CUEDefinitions, file)
	case ".yaml", ".yml":
		a.Definitions = append(a.Definitions, file)
	default:
		// skip other file formats
	}
	return nil
}

// readConfigTemplateFile read single template file of the config
func readConfigTemplateFile(a *UIData, reader AsyncReader, readPath string) error {
	b, err := reader.ReadFile(readPath)
	if err != nil {
		return err
	}
	filename := path.Base(readPath)
	if filepath.Ext(filename) != ".cue" {
		return nil
	}
	file := ElementFile{Data: b, Name: filepath.Base(readPath)}
	a.ConfigTemplates = append(a.ConfigTemplates, file)
	return nil
}

// readViewFile read single view file
func readViewFile(a *InstallPackage, reader AsyncReader, readPath string) error {
	b, err := reader.ReadFile(readPath)
	if err != nil {
		return err
	}
	filename := path.Base(readPath)
	switch filepath.Ext(filename) {
	case ".cue":
		a.CUEViews = append(a.CUEViews, ElementFile{Data: b, Name: filepath.Base(readPath)})
	case ".yaml", ".yml":
		a.YAMLViews = append(a.YAMLViews, ElementFile{Data: b, Name: filepath.Base(readPath)})
	default:
		// skip other file formats
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
	// the detail will contain readme.md or README.md, if the content already is filled, don't read another.
	if len(a.Detail) != 0 {
		return nil
	}
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

func createGitlabHelper(content *utils.Content, token string) (*gitlabHelper, error) {
	newClient, err := gitlab.NewClient(token, gitlab.WithBaseURL(content.GitlabContent.Host))

	return &gitlabHelper{
		Client: newClient,
		Meta:   content,
	}, err
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

// GetGiteeContents can return either the metadata and content of a single file
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
	if err != nil {
		return nil, nil, err
	}
	//nolint:errcheck
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, nil, err
	}
	return unmarshalToContent(body)
}

func unmarshalToContent(content []byte) (fileContent *github.RepositoryContent, directoryContent []*github.RepositoryContent, err error) {
	fileUnmarshalError := json.Unmarshal(content, &fileContent)
	if fileUnmarshalError == nil {
		return fileContent, nil, nil
	}
	directoryUnmarshalError := json.Unmarshal(content, &directoryContent)
	if directoryUnmarshalError == nil {
		return nil, directoryContent, nil
	}
	return nil, nil, fmt.Errorf("unmarshalling failed for both file and directory content: %s and %w", fileUnmarshalError, directoryUnmarshalError)
}

func genAddonAPISchema(addonRes *UIData) error {
	cueScript := script.CUE(addonRes.Parameters)
	schema, err := cueScript.ParsePropertiesToSchema()
	if err != nil {
		return err
	}
	addonRes.APISchema = schema
	return nil
}

func getClusters(args map[string]interface{}) []string {
	ccr, ok := args[types.ClustersArg]
	if !ok {
		return nil
	}
	cc, ok := ccr.([]string)
	if ok {
		return cc
	}
	ccrslice, ok := ccr.([]interface{})
	if !ok {
		return nil
	}
	var ccstring []string
	for _, c := range ccrslice {
		if cstring, ok := c.(string); ok {
			ccstring = append(ccstring, cstring)
		}
	}
	return ccstring
}

// renderNeededNamespaceAsComps will convert namespace as app components to create namespace for managed clusters
func renderNeededNamespaceAsComps(addon *InstallPackage) []common2.ApplicationComponent {
	var nscomps []common2.ApplicationComponent
	// create namespace for managed clusters
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
		nscomps = append(nscomps, comp)
	}
	return nscomps
}

func checkDeployClusters(ctx context.Context, k8sClient client.Client, args map[string]interface{}) ([]string, error) {
	deployClusters := getClusters(args)
	if len(deployClusters) == 0 || k8sClient == nil {
		return nil, nil
	}

	clusters, err := multicluster.NewClusterClient(k8sClient).List(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "fail to get registered cluster")
	}

	clusterNames := sets.Set[string]{}
	if len(clusters.Items) != 0 {
		for _, cluster := range clusters.Items {
			clusterNames.Insert(cluster.Name)
		}
	}

	var res []string
	for _, c := range deployClusters {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if !clusterNames.Has(c) {
			return nil, errors.Errorf("cluster %s not exist", c)
		}
		res = append(res, c)
	}
	return res, nil
}

// RenderDefinitions render definition objects if needed
func RenderDefinitions(addon *InstallPackage, config *rest.Config) ([]*unstructured.Unstructured, error) {
	defObjs := make([]*unstructured.Unstructured, 0)

	// No matter runtime mode or control mode, definition only needs to control plane k8s.
	for _, def := range addon.Definitions {
		obj, err := renderObject(def)
		if err != nil {
			return nil, errors.Wrapf(err, "render definition file %s", def.Name)
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

// RenderConfigTemplates render the config template
func RenderConfigTemplates(addon *InstallPackage, cli client.Client) ([]*unstructured.Unstructured, error) {
	templates := make([]*unstructured.Unstructured, 0)

	factory := config.NewConfigFactory(cli)
	for _, templateFile := range addon.ConfigTemplates {
		t, err := factory.ParseTemplate("", []byte(templateFile.Data))
		if err != nil {
			return nil, err
		}
		t.ConfigMap.Namespace = types.DefaultKubeVelaNS
		obj, err := util.Object2Unstructured(t.ConfigMap)
		if err != nil {
			return nil, err
		}
		obj.SetKind("ConfigMap")
		obj.SetAPIVersion("v1")
		templates = append(templates, obj)
	}

	return templates, nil
}

// RenderDefinitionSchema will render definitions' schema in addons.
func RenderDefinitionSchema(addon *InstallPackage) ([]*unstructured.Unstructured, error) {
	schemaConfigmaps := make([]*unstructured.Unstructured, 0)

	// No matter runtime mode or control mode , definition schemas only needs to control plane k8s.
	for _, teml := range addon.DefSchemas {
		u, err := renderSchemaConfigmap(teml)
		if err != nil {
			return nil, errors.Wrapf(err, "render uiSchema file %s", teml.Name)
		}
		schemaConfigmaps = append(schemaConfigmaps, u)
	}
	return schemaConfigmaps, nil
}

// RenderViews will render views in addons.
func RenderViews(addon *InstallPackage) ([]*unstructured.Unstructured, error) {
	views := make([]*unstructured.Unstructured, 0)
	for _, view := range addon.YAMLViews {
		obj, err := renderObject(view)
		if err != nil {
			return nil, errors.Wrapf(err, "render velaQL view file %s", view.Name)
		}
		views = append(views, obj)
	}
	for _, view := range addon.CUEViews {
		obj, err := renderCUEView(view)
		if err != nil {
			return nil, errors.Wrapf(err, "render velaQL view file %s", view.Name)
		}
		views = append(views, obj)
	}
	return views, nil
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
			return nil, errors.Wrapf(err, "render resource file %s", elem.Name)
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

func renderCUEView(elem ElementFile) (*unstructured.Unstructured, error) {
	name, err := utils.GetFilenameFromLocalOrRemote(elem.Name)
	if err != nil {
		return nil, err
	}

	cm, err := velaql.ParseViewIntoConfigMap(elem.Data, name)
	if err != nil {
		return nil, err
	}

	return util.Object2Unstructured(*cm)
}

// RenderArgsSecret render addon enable argument to secret to remember when restart or upgrade
func RenderArgsSecret(addon *InstallPackage, args map[string]interface{}) *unstructured.Unstructured {
	argsByte, err := json.Marshal(args)
	if err != nil {
		return nil
	}
	sec := v1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      addonutil.Addon2SecName(addon.Name),
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

// Installer helps addon enable, dependency-check, dispatch resources
type Installer struct {
	ctx                 context.Context
	config              *rest.Config
	addon               *InstallPackage
	cli                 client.Client
	apply               apply.Applicator
	r                   *Registry
	registryMeta        map[string]SourceMeta
	args                map[string]interface{}
	cache               *Cache
	dc                  *discovery.DiscoveryClient
	skipVersionValidate bool
	overrideDefs        bool

	dryRun     bool
	dryRunBuff *bytes.Buffer

	installerRuntime map[string]interface{}

	registries []Registry
}

// NewAddonInstaller will create an installer for addon
func NewAddonInstaller(ctx context.Context, cli client.Client, discoveryClient *discovery.DiscoveryClient, apply apply.Applicator, config *rest.Config, r *Registry, args map[string]interface{}, cache *Cache, registries []Registry, opts ...InstallOption) Installer {
	if args == nil {
		args = map[string]interface{}{}
	}
	i := Installer{
		ctx:        ctx,
		config:     config,
		cli:        cli,
		apply:      apply,
		r:          r,
		args:       args,
		cache:      cache,
		dc:         discoveryClient,
		dryRunBuff: &bytes.Buffer{},
		registries: registries,
	}
	ir := args[InstallerRuntimeOption]
	if irr, ok := ir.(map[string]interface{}); ok {
		i.installerRuntime = irr
	} else {
		i.installerRuntime = map[string]interface{}{}
	}
	// clean injected data from runtime option
	delete(args, InstallerRuntimeOption)

	for _, opt := range opts {
		opt(&i)
	}
	return i
}

func (h *Installer) enableAddon(addon *InstallPackage) (string, error) {
	var err error
	h.addon = addon
	if !h.skipVersionValidate {
		err = checkAddonVersionMeetRequired(h.ctx, addon.SystemRequirements, h.cli, h.dc)
		if err != nil {
			version := h.getAddonVersionMeetSystemRequirement(addon.Name)
			return "", VersionUnMatchError{addonName: addon.Name, err: err, userSelectedAddonVersion: addon.Version, availableVersion: version}
		}
	}

	if err = h.installDependency(addon); err != nil {
		return "", err
	}
	if err = h.dispatchAddonResource(addon); err != nil {
		return "", err
	}
	// we shouldn't put continue func into dispatchAddonResource, because the re-apply app maybe already update app and
	// the suspend will set with false automatically
	if err := h.continueOrRestartWorkflow(); err != nil {
		return "", err
	}
	additionalInfo, err := h.renderNotes(addon)
	if err != nil {
		klog.Warningf("fail to render notes for addon %s: %v\n", addon.Name, err)
		// notes don't affect the installation, so just print warn logs instead of abort with errors
		return "", nil
	}
	return additionalInfo, nil
}

func (h *Installer) loadInstallPackage(name, version string) (*InstallPackage, error) {
	var installPackage *InstallPackage
	var err error
	if !IsVersionRegistry(*h.r) {
		metas, err := h.getAddonMeta()
		if err != nil {
			return nil, errors.Wrap(err, "fail to get addon meta")
		}

		meta, ok := metas[name]
		if !ok {
			return nil, ErrNotExist
		}
		var uiData *UIData
		uiData, err = h.cache.GetUIData(*h.r, name, version)
		if err != nil {
			return nil, err
		}
		// enable this addon if it's invisible
		installPackage, err = h.r.GetInstallPackage(&meta, uiData)
		if err != nil {
			return nil, errors.Wrap(err, "fail to find dependent addon in source repository")
		}
	} else {
		versionedRegistry := BuildVersionedRegistry(h.r.Name, h.r.Helm.URL, &common.HTTPOption{
			Username:        h.r.Helm.Username,
			Password:        h.r.Helm.Password,
			InsecureSkipTLS: h.r.Helm.InsecureSkipTLS,
		})
		installPackage, err = versionedRegistry.GetAddonInstallPackage(context.Background(), name, version)
		if err != nil {
			return nil, err
		}
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
	var dependencies []string
	var addonClusters = getClusters(h.args)
	for _, dep := range addon.Dependencies {
		needInstallAddonDep, depClusters, err := checkDependencyNeedInstall(h.ctx, h.cli, dep.Name, addonClusters)
		if err != nil {
			return err
		}
		if !needInstallAddonDep {
			continue
		}

		dependencies = append(dependencies, dep.Name)
		if h.dryRun {
			continue
		}
		depHandler := *h
		// get dependency addon original parameters
		depArgs, depArgsErr := GetAddonLegacyParameters(h.ctx, h.cli, dep.Name)
		if depArgsErr != nil {
			if !apierrors.IsNotFound(depArgsErr) {
				return depArgsErr
			}
		}
		if depArgs == nil {
			depArgs = map[string]interface{}{}
		}
		// reset the cluster arg
		depArgs[types.ClustersArg] = depClusters

		depHandler.args = depArgs

		var depAddon *InstallPackage
		// try to install the dependent addon from the same registry with the current addon
		depAddon, err = h.loadInstallPackage(dep.Name, dep.Version)
		if err == nil {
			additionalInfo, err := depHandler.enableAddon(depAddon)
			if err != nil {
				return errors.Wrap(err, "fail to dispatch dependent addon resource")
			}
			if len(additionalInfo) > 0 {
				klog.Infof("addon %s installed with additional info: %s\n", addon.Name, additionalInfo)
			}
			return nil
		}
		if !errors.Is(err, ErrNotExist) {
			return err
		}
		for _, registry := range h.registries {
			// try to install dependent addon from other registries
			depHandler.r = &Registry{
				Name: registry.Name, Helm: registry.Helm, OSS: registry.OSS, Git: registry.Git, Gitee: registry.Gitee, Gitlab: registry.Gitlab,
			}
			depAddon, err = depHandler.loadInstallPackage(dep.Name, dep.Version)
			if err == nil {
				break
			}
			if errors.Is(err, ErrNotExist) {
				continue
			}
			return err
		}
		if err == nil {
			additionalInfo, err := depHandler.enableAddon(depAddon)
			if err != nil {
				return errors.Wrap(err, "fail to dispatch dependent addon resource")
			}
			if len(additionalInfo) > 0 {
				klog.Infof("addon %s installed with additional info: %s\n", addon.Name, additionalInfo)
			}
			return nil
		}
		return fmt.Errorf("dependency addon: %s with version: %s cannot be found from all registries", dep.Name, dep.Version)
	}
	if h.dryRun && len(dependencies) > 0 {
		klog.Warningf("dry run addon won't install dependencies, please make sure your system has already installed these addons: %v", strings.Join(dependencies, ", "))
		return nil
	}
	return nil
}

// checkDependencyNeedInstall checks whether dependency addon needs to be installed on other clusters
func checkDependencyNeedInstall(ctx context.Context, k8sClient client.Client, depName string, addonClusters []string) (bool, []string, error) {
	depApp, err := FetchAddonRelatedApp(ctx, k8sClient, depName)
	var needInstallAddonDep = false
	var depClusters []string
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return needInstallAddonDep, depClusters, err
		}
		// depApp is not exist
		needInstallAddonDep = true
		depClusters = addonClusters
	} else {
		// get the runtime clusters of current dependency addon
		for _, r := range depApp.Status.AppliedResources {
			if r.Cluster != "" && !stringslices.Contains(depClusters, r.Cluster) {
				depClusters = append(depClusters, r.Cluster)
			}
		}

		// determine if there are no dependencies on the cluster to be installed
		for _, addonCluster := range addonClusters {
			if !stringslices.Contains(depClusters, addonCluster) {
				depClusters = append(depClusters, addonCluster)
				needInstallAddonDep = true
			}
		}
	}
	return needInstallAddonDep, depClusters, nil
}

// checkDependency checks if addon's dependency
func (h *Installer) checkDependency(addon *InstallPackage) ([]string, error) {
	var app v1beta1.Application
	var needEnable []string
	for _, dep := range addon.Dependencies {
		err := h.cli.Get(h.ctx, client.ObjectKey{
			Namespace: types.DefaultKubeVelaNS,
			Name:      addonutil.Addon2AppName(dep.Name),
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

// createOrUpdate will return true if updated
func (h *Installer) createOrUpdate(app *v1beta1.Application) (bool, error) {
	// Set the publish version for the addon application
	oam.SetPublishVersion(app, util.GenerateVersion("addon"))
	var existApp v1beta1.Application
	err := h.cli.Get(h.ctx, client.ObjectKey{Name: app.Name, Namespace: app.Namespace}, &existApp)
	if apierrors.IsNotFound(err) {
		return false, h.cli.Create(h.ctx, app)
	}
	if err != nil {
		return false, err
	}
	existApp.Spec = app.Spec
	existApp.Labels = app.Labels
	existApp.Annotations = app.Annotations
	err = h.cli.Update(h.ctx, &existApp)
	if err != nil {
		klog.Errorf("fail to create application: %v", err)
		return false, errors.Wrap(err, "fail to create application")
	}
	existApp.DeepCopyInto(app)
	return true, nil
}

func (h *Installer) dispatchAddonResource(addon *InstallPackage) error {
	app, auxiliaryOutputs, err := RenderApp(h.ctx, addon, h.cli, h.args)
	if err != nil {
		return errors.Wrap(err, "render addon application fail")
	}
	appName, err := determineAddonAppName(h.ctx, h.cli, h.addon.Name)
	if err != nil {
		return err
	}
	app.Name = appName

	app.SetLabels(util.MergeMapOverrideWithDst(app.GetLabels(), map[string]string{oam.LabelAddonRegistry: h.r.Name}))

	// Step1: Render the definitions
	defs, err := RenderDefinitions(addon, h.config)
	if err != nil {
		return errors.Wrap(err, "render addon definitions fail")
	}

	if !h.overrideDefs {
		existDefs, err := checkConflictDefs(h.ctx, h.cli, defs, app.Name)
		if err != nil {
			return err
		}
		if len(existDefs) != 0 {
			return produceDefConflictError(existDefs)
		}
	}

	// Step2: Render the config templates
	templates, err := RenderConfigTemplates(addon, h.cli)
	if err != nil {
		return errors.Wrap(err, "render the config template fail")
	}

	// Step3: Render the definition schemas
	schemas, err := RenderDefinitionSchema(addon)
	if err != nil {
		return errors.Wrap(err, "render addon definitions' schema fail")
	}

	// Step4: Render the velaQL views
	views, err := RenderViews(addon)
	if err != nil {
		return errors.Wrap(err, "render addon views fail")
	}

	if err := passDefInAppAnnotation(defs, app); err != nil {
		return errors.Wrapf(err, "cannot pass definition to addon app's annotation")
	}

	if h.dryRun {
		result, err := yaml.Marshal(app)
		if err != nil {
			return errors.Wrapf(err, "dry-run marshal app into yaml %s", app.Name)
		}
		h.dryRunBuff.Write(result)
		h.dryRunBuff.WriteString("\n")
	} else {
		updated, err := h.createOrUpdate(app)
		if err != nil {
			return err
		}
		if updated {
			h.installerRuntime["upgrade"] = true
		}
	}

	auxiliaryOutputs = append(auxiliaryOutputs, defs...)
	auxiliaryOutputs = append(auxiliaryOutputs, templates...)
	auxiliaryOutputs = append(auxiliaryOutputs, schemas...)
	auxiliaryOutputs = append(auxiliaryOutputs, views...)

	for _, o := range auxiliaryOutputs {
		// bind-component means the content is related with the component
		// if component not exists, the resources shouldn't be applied
		if !checkBondComponentExist(*o, *app) {
			continue
		}
		if h.dryRun {
			result, err := yaml.Marshal(o)
			if err != nil {
				return errors.Wrapf(err, "dry-run marshal auxiliary object into yaml %s", o.GetName())
			}
			h.dryRunBuff.WriteString("---\n")
			h.dryRunBuff.Write(result)
			h.dryRunBuff.WriteString("\n")
			continue
		}
		addOwner(o, app)
		err = h.apply.Apply(h.ctx, o, apply.DisableUpdateAnnotation())
		if err != nil {
			return err
		}
	}

	if h.dryRun {
		fmt.Print(h.dryRunBuff.String())
		return nil
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

func (h *Installer) renderNotes(addon *InstallPackage) (string, error) {
	if len(addon.Notes.Data) == 0 {
		return "", nil
	}
	r := addonCueTemplateRender{
		addon:     addon,
		inputArgs: h.args,
		contextInfo: map[string]interface{}{
			"installer": h.installerRuntime,
		},
	}
	contextFile, err := r.formatContext()
	if err != nil {
		return "", err
	}
	notesFile := contextFile + "\n" + addon.Notes.Data
	val, err := value.NewValue(notesFile, nil, "")
	if err != nil {
		return "", errors.Wrap(err, "build values for NOTES.cue")
	}
	notes, err := val.LookupValue(KeyWordNotes)
	if err != nil {
		return "", errors.Wrap(err, "look up notes in NOTES.cue")
	}
	notesStr, err := notes.CueValue().String()
	if err != nil {
		return "", errors.Wrap(err, "convert notes to string")
	}
	return notesStr, nil
}

// this func will handle such two case
// 1. if last apply failed an workflow have suspend, this func will continue the workflow
// 2. restart the workflow, if the new cluster have been added in KubeVela
func (h *Installer) continueOrRestartWorkflow() error {
	if h.dryRun {
		return nil
	}
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

// getAddonVersionMeetSystemRequirement return the addon's latest version which meet the system requirements
func (h *Installer) getAddonVersionMeetSystemRequirement(addonName string) string {
	if h.r != nil && IsVersionRegistry(*h.r) {
		versionedRegistry := BuildVersionedRegistry(h.r.Name, h.r.Helm.URL, &common.HTTPOption{
			Username: h.r.Helm.Username,
			Password: h.r.Helm.Password,
		})
		versions, err := versionedRegistry.GetAddonAvailableVersion(addonName)
		if err != nil {
			return ""
		}
		for _, version := range versions {
			req := LoadSystemRequirements(version.Annotations)
			if checkAddonVersionMeetRequired(h.ctx, req, h.cli, h.dc) == nil {
				return version.Version
			}
		}
	}
	return ""
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
		return addonutil.Addon2AppName(addonName), nil
	}
	return app.Name, nil
}

// FetchAddonRelatedApp will fetch the addon related app, this func will use NamespacedName(vela-system, addon-addonName) to get app
// if not find will try to get 1.1 legacy addon related app by using NamespacedName(vela-system, `addonName`)
func FetchAddonRelatedApp(ctx context.Context, cli client.Client, addonName string) (*v1beta1.Application, error) {
	app := &v1beta1.Application{}
	if err := cli.Get(ctx, types2.NamespacedName{Namespace: types.DefaultKubeVelaNS, Name: addonutil.Addon2AppName(addonName)}, app); err != nil {
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
			return fmt.Errorf("vela cli/ux version: %s  require: %s", version2.VelaVersion, require.VelaVersion)
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
			return fmt.Errorf("the vela core controller: %s require: %s", imageVersion, require.VelaVersion)
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
			return fmt.Errorf("the kubernetes version %s require: %s", k8sVersion.GitVersion, require.KubernetesVersion)
		}
	}

	return nil
}

func checkSemVer(actual string, require string) (bool, error) {
	if len(require) == 0 {
		return true, nil
	}
	semVer := strings.TrimPrefix(actual, "v")
	l := strings.ReplaceAll(require, "v", " ")
	constraint, err := semver.NewConstraint(l)
	if err != nil {
		klog.Errorf("fail to new constraint: %s", err.Error())
		return false, err
	}
	v, err := semver.NewVersion(semVer)
	if err != nil {
		klog.Errorf("fail to new version %s: %s", semVer, err.Error())
		return false, err
	}
	if constraint.Check(v) {
		return true, nil
	}
	if strings.Contains(actual, "-") && !strings.Contains(require, "-") {
		semVer := strings.TrimPrefix(actual[:strings.Index(actual, "-")], "v") // nolint
		if strings.Contains(require, ">=") && require[strings.Index(require, "=")+1:] == semVer {
			// for case: `actual` is 1.5.0-beta.1 require is >=`1.5.0`
			return false, nil
		}
		v, err := semver.NewVersion(semVer)
		if err != nil {
			klog.Errorf("fail to new version %s: %s", semVer, err.Error())
			return false, err
		}
		if constraint.Check(v) {
			return true, nil
		}
	}
	return false, nil
}

func fetchVelaCoreImageTag(ctx context.Context, k8sClient client.Client) (string, error) {
	deployList := &appsv1.DeploymentList{}
	if err := k8sClient.List(ctx, deployList, client.MatchingLabels{oam.LabelControllerName: oam.ApplicationControllerName}); err != nil {
		return "", err
	}
	deploy := appsv1.Deployment{}
	if len(deployList.Items) == 0 {
		// backward compatible logic old version which vela-core controller has no this label
		if err := k8sClient.Get(ctx, types2.NamespacedName{Namespace: types.DefaultKubeVelaNS, Name: types.KubeVelaControllerDeployment}, &deploy); err != nil {
			if apierrors.IsNotFound(err) {
				return "", errors.New("can't find a running KubeVela instance, please install it first")
			}
			return "", err
		}
	} else {
		deploy = deployList.Items[0]
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

// PackageAddon package vela addon directory into a helm chart compatible archive and return its absolute path
func PackageAddon(addonDictPath string) (string, error) {
	// save the Chart.yaml file in order to be compatible with helm chart
	err := MakeChartCompatible(addonDictPath, true)
	if err != nil {
		return "", err
	}

	ch, err := loader.LoadDir(addonDictPath)
	if err != nil {
		return "", err
	}

	dest, err := os.Getwd()
	if err != nil {
		return "", err
	}
	archive, err := chartutil.Save(ch, dest)
	if err != nil {
		return "", err
	}
	return archive, nil
}

// GetAddonLegacyParameters get addon's legacy parameters, that is stored in Secret
func GetAddonLegacyParameters(ctx context.Context, k8sClient client.Client, addonName string) (map[string]interface{}, error) {
	var sec v1.Secret
	err := k8sClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: addonutil.Addon2SecName(addonName)}, &sec)
	if err != nil {
		return nil, err
	}
	args, err := FetchArgsFromSecret(&sec)
	if err != nil {
		return nil, err
	}
	return args, nil
}

// MergeAddonInstallArgs merge addon's legacy parameter and new input args
func MergeAddonInstallArgs(ctx context.Context, k8sClient client.Client, addonName string, args map[string]interface{}) (map[string]interface{}, error) {
	legacyParams, err := GetAddonLegacyParameters(ctx, k8sClient, addonName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		return args, nil
	}

	if args == nil && legacyParams == nil {
		return args, nil
	}

	r := make(map[string]interface{})
	if err := mergo.Merge(&r, legacyParams, mergo.WithOverride); err != nil {
		return nil, err
	}

	if err := mergo.Merge(&r, args, mergo.WithOverride); err != nil {
		return nil, err
	}
	return r, nil
}
