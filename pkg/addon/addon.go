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
	"context"
	"encoding/json"
	"fmt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"cuelang.org/go/cue"
	cueyaml "cuelang.org/go/encoding/yaml"
	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	utils2 "github.com/oam-dev/kubevela/pkg/controller/utils"
	cuemodel "github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

const (
	// ReadmeFileName is the addon readme file name
	ReadmeFileName string = "readme.md"

	// MetadataFileName is the addon meatadata.yaml file name
	MetadataFileName string = "metadata.yaml"

	// TemplateFileName is the addon template.yaml dir name
	TemplateFileName string = "template.yaml"

	// ResourcesDirName is the addon resources/ dir name
	ResourcesDirName string = "resources"

	// DefinitionsDirName is the addon definitions/ dir name
	DefinitionsDirName string = "definitions"
)

// ListOptions contains flags mark what files should be read in an addon directory
type ListOptions struct {
	GetDetail     bool
	GetDefinition bool
	GetResource   bool
	GetParameter  bool
	GetTemplate   bool
}

var (
	// GetLevelOptions used when get or list addons
	GetLevelOptions = ListOptions{GetDetail: true, GetDefinition: true, GetParameter: true}

	// EnableLevelOptions used when enable addon
	EnableLevelOptions = ListOptions{GetDetail: true, GetDefinition: true, GetResource: true, GetTemplate: true, GetParameter: true}
)

// aError is internal error type of addon
type aError error

var (
	// ErrNotExist means addon not exists
	ErrNotExist aError = errors.New("addon not exist")
)

// gitHelper helps get addon's file by git
type gitHelper struct {
	Client *github.Client
	Meta   *utils.Content
}

// GitAddonSource defines the information about the Git as addon source
type GitAddonSource struct {
	URL   string `json:"url,omitempty" validate:"required"`
	Path  string `json:"path,omitempty"`
	Token string `json:"token,omitempty"`
}

// asyncReader helps async read files of addon
type asyncReader struct {
	addon   *types.Addon
	h       *gitHelper
	item    *github.RepositoryContent
	errChan chan error
	// mutex is needed when append to addon's Definitions/CUETemplate/YAMLTemplate slices
	mutex *sync.Mutex
}

// SetReadContent set which file to read
func (r *asyncReader) SetReadContent(content *github.RepositoryContent) {
	r.item = content
}

// GetAddon get a addon info from GitAddonSource, can be used for get or enable
func GetAddon(name string, git *GitAddonSource, opt ListOptions) (*types.Addon, error) {
	addon, err := getSingleAddonFromGit(git.URL, git.Path, name, git.Token, opt)
	if err != nil {
		return nil, err
	}
	return addon, nil
}

// ListAddons list addons' info from GitAddonSource
func ListAddons(git *GitAddonSource, opt ListOptions) ([]*types.Addon, error) {
	gitAddons, err := getAddonsFromGit(git.URL, git.Path, git.Token, opt)
	if err != nil {
		return nil, err
	}
	return gitAddons, nil
}

func getAddonsFromGit(baseURL, dir, token string, opt ListOptions) ([]*types.Addon, error) {
	var addons []*types.Addon
	var err error
	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	gith, err := createGitHelper(baseURL, dir, token)
	if err != nil {
		return nil, err
	}
	_, items, err := gith.readRepo(gith.Meta.Path)
	if err != nil {
		return nil, err
	}

	for _, subItems := range items {
		if subItems.GetType() != "dir" {
			continue
		}
		wg.Add(1)
		go func(item *github.RepositoryContent) {
			defer wg.Done()
			addonRes, err := getSingleAddonFromGit(baseURL, dir, item.GetName(), token, opt)
			if err != nil {
				errChan <- err
				return
			}
			addons = append(addons, addonRes)
		}(subItems)
	}
	wg.Wait()
	if len(errChan) != 0 {
		return nil, <-errChan
	}
	return addons, nil
}

func getSingleAddonFromGit(baseURL, dir, addonName, token string, opt ListOptions) (*types.Addon, error) {
	var wg sync.WaitGroup
	readOption := map[string]struct {
		jumpConds bool
		read      func(wg *sync.WaitGroup, reader asyncReader)
	}{
		ReadmeFileName:     {!opt.GetDetail, readReadme},
		TemplateFileName:   {!opt.GetTemplate, readTemplate},
		MetadataFileName:   {false, readMetadata},
		DefinitionsDirName: {!opt.GetDefinition, readDefinitions},
		ResourcesDirName:   {!opt.GetResource && !opt.GetParameter, readResources},
	}

	gith, err := createGitHelper(baseURL, path.Join(dir, addonName), token)
	if err != nil {
		return nil, err
	}
	_, items, err := gith.readRepo(gith.Meta.Path)
	if err != nil {
		return nil, err
	}

	reader := asyncReader{
		addon:   &types.Addon{},
		h:       gith,
		errChan: make(chan error, 1),
		mutex:   &sync.Mutex{},
	}
	for _, item := range items {
		itemName := strings.ToLower(item.GetName())
		switch itemName {
		case ReadmeFileName, MetadataFileName, DefinitionsDirName, ResourcesDirName, TemplateFileName:
			readMethod := readOption[itemName]
			if readMethod.jumpConds {
				break
			}
			reader.SetReadContent(item)
			wg.Add(1)
			go readMethod.read(&wg, reader)
		}
	}
	wg.Wait()

	if opt.GetParameter && reader.addon.Parameters != "" {
		err = genAddonAPISchema(reader.addon)
		if err != nil {
			return nil, err
		}
	}
	return reader.addon, nil

}

func readTemplate(wg *sync.WaitGroup, reader asyncReader) {
	defer wg.Done()
	content, _, err := reader.h.readRepo(*reader.item.Path)
	if err != nil {
		reader.errChan <- err
		return
	}
	data, err := content.GetContent()
	if err != nil {
		reader.errChan <- err
		return
	}
	dec := k8syaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	reader.addon.AppTemplate = &v1beta1.Application{}
	_, _, err = dec.Decode([]byte(data), nil, reader.addon.AppTemplate)
	if err != nil {
		reader.errChan <- err
		return
	}
}

func readResources(wg *sync.WaitGroup, reader asyncReader) {
	defer wg.Done()
	dirPath := strings.Split(reader.item.GetPath(), "/")
	dirPath, err := cutPathUntil(dirPath, ResourcesDirName)
	if err != nil {
		reader.errChan <- err
	}

	_, items, err := reader.h.readRepo(*reader.item.Path)
	if err != nil {
		reader.errChan <- err
		return
	}
	for _, item := range items {
		switch item.GetType() {
		case "file":
			reader.SetReadContent(item)
			wg.Add(1)
			go readResFile(wg, reader, dirPath)
		case "dir":
			reader.SetReadContent(item)
			wg.Add(1)
			go readResources(wg, reader)

		}
	}
}

// readResFile read single resource file
func readResFile(wg *sync.WaitGroup, reader asyncReader, dirPath []string) {
	defer wg.Done()
	content, _, err := reader.h.readRepo(*reader.item.Path)
	if err != nil {
		reader.errChan <- err
		return
	}
	b, err := content.GetContent()
	if err != nil {
		reader.errChan <- err
		return
	}

	if reader.item.GetName() == "parameter.cue" {
		reader.addon.Parameters = b
		return
	}
	switch filepath.Ext(reader.item.GetName()) {
	case ".cue":
		reader.mutex.Lock()
		reader.addon.CUETemplates = append(reader.addon.CUETemplates, types.AddonElementFile{Data: b, Name: reader.item.GetName(), Path: dirPath})
		reader.mutex.Unlock()
	default:
		reader.mutex.Lock()
		reader.addon.YAMLTemplates = append(reader.addon.YAMLTemplates, types.AddonElementFile{Data: b, Name: reader.item.GetName(), Path: dirPath})
		reader.mutex.Unlock()
	}
}

func readDefinitions(wg *sync.WaitGroup, reader asyncReader) {
	defer wg.Done()
	dirPath := strings.Split(reader.item.GetPath(), "/")
	dirPath, err := cutPathUntil(dirPath, DefinitionsDirName)
	if err != nil {
		reader.errChan <- err
		return
	}
	_, items, err := reader.h.readRepo(*reader.item.Path)
	if err != nil {
		reader.errChan <- err
		return
	}
	for _, item := range items {
		switch item.GetType() {
		case "file":
			reader.SetReadContent(item)
			wg.Add(1)
			go readDefFile(wg, reader, dirPath)
		case "dir":
			reader.SetReadContent(item)
			wg.Add(1)
			go readDefinitions(wg, reader)
		}
	}
}

// readDefFile read single definition file
func readDefFile(wg *sync.WaitGroup, reader asyncReader, dirPath []string) {
	defer wg.Done()
	content, _, err := reader.h.readRepo(*reader.item.Path)
	if err != nil {
		reader.errChan <- err
		return
	}
	b, err := content.GetContent()
	if err != nil {
		reader.errChan <- err
		return
	}
	reader.mutex.Lock()
	reader.addon.Definitions = append(reader.addon.Definitions, types.AddonElementFile{Data: b, Name: reader.item.GetName(), Path: dirPath})
	reader.mutex.Unlock()
}

func readMetadata(wg *sync.WaitGroup, reader asyncReader) {
	defer wg.Done()
	content, _, err := reader.h.readRepo(*reader.item.Path)
	if err != nil {
		reader.errChan <- err
		return
	}
	b, err := content.GetContent()
	if err != nil {
		reader.errChan <- err
		return
	}
	err = yaml.Unmarshal([]byte(b), &reader.addon.AddonMeta)
	if err != nil {
		reader.errChan <- err
		return
	}
}

func readReadme(wg *sync.WaitGroup, reader asyncReader) {
	defer wg.Done()
	content, _, err := reader.h.readRepo(*reader.item.Path)
	if err != nil {
		reader.errChan <- err
		return
	}
	reader.addon.Detail, err = content.GetContent()
	if err != nil {
		reader.errChan <- err
		return
	}
}

func createGitHelper(baseURL, dir, token string) (*gitHelper, error) {
	var ts oauth2.TokenSource
	if token != "" {
		ts = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	}
	tc := oauth2.NewClient(context.Background(), ts)
	tc.Timeout = time.Second * 10
	cli := github.NewClient(tc)

	baseURL = strings.TrimSuffix(baseURL, ".git")
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, errors.New("addon registry invalid")
	}
	u.Path = path.Join(u.Path, dir)
	_, gitmeta, err := utils.Parse(u.String())
	if err != nil {
		return nil, errors.New("addon registry invalid")
	}

	return &gitHelper{
		Client: cli,
		Meta:   gitmeta,
	}, nil
}

func (h *gitHelper) readRepo(path string) (*github.RepositoryContent, []*github.RepositoryContent, error) {
	file, items, _, err := h.Client.Repositories.GetContents(context.Background(), h.Meta.Owner, h.Meta.Repo, path, nil)
	if err != nil {
		return nil, nil, WrapErrRateLimit(err)
	}
	return file, items, nil
}

func genAddonAPISchema(addonRes *types.Addon) error {
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

func cutPathUntil(path []string, end string) ([]string, error) {
	for i, d := range path {
		if d == end {
			return path[i:], nil
		}
	}
	return nil, errors.New("cut path fail, target directory name not found")
}

// RenderApplication render a K8s application
func RenderApplication(addon *types.Addon, args map[string]interface{}) (*v1beta1.Application, []*unstructured.Unstructured, error) {
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
	app.Name = Convert2AppName(addon.Name)
	app.Labels = util.MergeMapOverrideWithDst(app.Labels, map[string]string{oam.LabelAddonName: addon.Name})
	if app.Spec.Workflow == nil {
		app.Spec.Workflow = &v1beta1.Workflow{}
	}
	for _, namespace := range addon.NeedNamespace {
		comp := common2.ApplicationComponent{
			Type:       "raw",
			Name:       fmt.Sprintf("%s-namespace", namespace),
			Properties: util.Object2RawExtension(renderNamespace(namespace)),
		}
		app.Spec.Components = append(app.Spec.Components, comp)
	}

	for _, tmpl := range addon.YAMLTemplates {
		comp, err := renderRawComponent(tmpl)
		if err != nil {
			return nil, nil, err
		}
		app.Spec.Components = append(app.Spec.Components, *comp)
	}
	for _, tmpl := range addon.CUETemplates {
		comp, err := renderCUETemplate(tmpl, addon.Parameters, args)
		if err != nil {
			return nil, nil, ErrRenderCueTmpl
		}
		if addon.Name == "observability" && strings.HasSuffix(comp.Name, ".cue") {
			comp.Name = strings.Split(comp.Name, ".cue")[0]
		}
		app.Spec.Components = append(app.Spec.Components, *comp)
	}

	var defObjs []*unstructured.Unstructured

	if isDeployToRuntimeOnly(addon) {
		// Runtime cluster mode needs to deploy definitions to control plane k8s.
		for _, def := range addon.Definitions {
			obj, err := renderObject(def)
			if err != nil {
				return nil, nil, err
			}
			defObjs = append(defObjs, obj)
		}
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
	} else {
		for _, def := range addon.Definitions {
			comp, err := renderRawComponent(def)
			if err != nil {
				return nil, nil, err
			}
			app.Spec.Components = append(app.Spec.Components, *comp)
		}
	}

	return app, defObjs, nil
}

func isDeployToRuntimeOnly(addon *types.Addon) bool {
	if addon.DeployTo == nil {
		return false
	}
	return addon.DeployTo.RuntimeCluster
}

func renderObject(elem types.AddonElementFile) (*unstructured.Unstructured, error) {
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

// renderRawComponent will return a component in raw type from string
func renderRawComponent(elem types.AddonElementFile) (*common2.ApplicationComponent, error) {
	baseRawComponent := common2.ApplicationComponent{
		Type: "raw",
		Name: elem.Name,
	}
	obj, err := renderObject(elem)
	if err != nil {
		return nil, err
	}
	baseRawComponent.Properties = util.Object2RawExtension(obj)
	return &baseRawComponent, nil
}

// renderCUETemplate will return a component from cue template
func renderCUETemplate(elem types.AddonElementFile, parameters string, args map[string]interface{}) (*common2.ApplicationComponent, error) {
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
	comp := common2.ApplicationComponent{
		Name: elem.Name,
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

// Convert2AddonName -
func Convert2AddonName(name string) string {
	return strings.TrimPrefix(name, addonAppPrefix)
}

// RenderArgsSecret TODO add desc
func RenderArgsSecret(addon *types.Addon, args map[string]interface{}) *unstructured.Unstructured {
	data := make(map[string]string)
	for k, v := range args {
		switch v := v.(type) {
		case bool:
			data[k] = strconv.FormatBool(v)
		default:
			data[k] = fmt.Sprintf("%v", v)
		}
	}
	sec := v1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      Convert2SecName(addon.Name),
			Namespace: types.DefaultKubeVelaNS,
		},
		StringData: data,
		Type:       v1.SecretTypeOpaque,
	}
	u, err := util.Object2Unstructured(sec)
	if err != nil {
		return nil
	}
	return u
}

// Convert2SecName TODO add desc
func Convert2SecName(name string) string {
	return addonSecPrefix + name
}

type AddonHandler struct {
	ctx    context.Context
	addon  *types.Addon
	clt    client.Client
	source *GitAddonSource
	args   map[string]interface{}
}

func newAddonHandler(ctx context.Context, addon *types.Addon, clt client.Client, source *GitAddonSource, args map[string]interface{}) AddonHandler {
	return AddonHandler{
		ctx:    ctx,
		addon:  addon,
		clt:    clt,
		source: source,
		args:   args,
	}
}

func EnableAddon(ctx context.Context, addon *types.Addon, clt client.Client, source *GitAddonSource, args map[string]interface{}) error {
	h := newAddonHandler(ctx, addon, clt, source, args)
	err := h.enableAddon()
	if err != nil {
		return err
	}
	return nil
}

func (h *AddonHandler) enableAddon() error {
	var err error
	if err = h.checkDependencies(); err != nil {
		return err
	}
	if err = h.dispatchAddonResource(); err != nil {
		return err
	}
	return nil
}

// checkDependencies checks if addon's dependent addons is enabled
func (h *AddonHandler) checkDependencies() error {
	var app v1beta1.Application
	for _, dep := range h.addon.Dependencies {
		err := h.clt.Get(h.ctx, client.ObjectKey{
			Namespace: types.DefaultKubeVelaNS,
			Name:      Convert2AppName(dep.Name),
		}, &app)
		if err == nil {
			continue
		}
		if !apierrors.IsNotFound(err) {
			return err
		}
		// enable this addon if it's invisible
		depAddon, err := GetAddon(dep.Name, h.source, EnableLevelOptions)
		if err != nil {
			return errors.Wrap(err, "fail to find dependent addon in source repository")
		}
		if !depAddon.Invisible {
			return fmt.Errorf("dependent addon %s cannot be enabled automatically", depAddon.Name)
		}
		// invisible addon SHOULD be enabled without argument
		depHandler := *h
		depHandler.addon = depAddon
		depHandler.args = nil
		if err = depHandler.enableAddon(); err != nil {
			return errors.Wrap(err, "fail to dispatch dependent addon resource")
		}
	}
	return nil
}

func (h *AddonHandler) dispatchAddonResource() error {
	app, defs, err := RenderApplication(h.addon, h.args)
	if err != nil {
		return errors.Wrap(err, "render addon application fail")
	}

	err = h.clt.Get(h.ctx, client.ObjectKeyFromObject(app), app)
	if err == nil {
		return errors.New("addon is already enabled")
	}

	err = h.clt.Create(h.ctx, app)
	if err != nil {
		return errors.Wrap(err, "fail to create application")
	}

	for _, def := range defs {
		addOwner(def, app)
		err = h.clt.Create(h.ctx, def)
		if err != nil {
			return err
		}
	}

	if h.args != nil && len(h.args) > 0 {
		sec := RenderArgsSecret(h.addon, h.args)
		addOwner(sec,app)
		err = h.clt.Create(h.ctx, sec)
		if err != nil {
			return err
		}
	}
	return nil
}

func addOwner(child *unstructured.Unstructured, app *v1beta1.Application) {
	child.SetOwnerReferences(append(child.GetOwnerReferences(),
		*metav1.NewControllerRef(app, v1beta1.ApplicationKindVersionKind)))
}
