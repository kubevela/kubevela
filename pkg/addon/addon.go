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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	"github.com/oam-dev/kubevela/pkg/utils/apply"
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

	// DefSchemaName is the addon definition schemas dir name
	DefSchemaName string = "schemas"
)

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
	// GetLevelOptions used when get or list addons
	GetLevelOptions = ListOptions{GetDetail: true, GetDefinition: true, GetParameter: true}

	// EnableLevelOptions used when enable addon
	EnableLevelOptions = ListOptions{GetDetail: true, GetDefinition: true, GetResource: true, GetTemplate: true, GetParameter: true, GetDefSchema: true}
)

// GetAddonsFromReader list addons from AsyncReader
func GetAddonsFromReader(r AsyncReader, opt ListOptions) ([]*types.Addon, error) {
	var addons []*types.Addon
	var err error
	var wg sync.WaitGroup
	var errs []error
	errCh := make(chan error)
	waitCh := make(chan struct{})

	_, items, err := r.Read(".")
	if err != nil {
		return nil, err
	}
	var l sync.Mutex
	for _, subItem := range items {
		if subItem.GetType() != "dir" {
			continue
		}
		wg.Add(1)
		go func(item Item) {
			defer wg.Done()
			ar := r.WithNewAddonAndMutex()
			addonRes, err := GetSingleAddonFromReader(ar, ar.RelativePath(item), opt)
			if err != nil {
				errCh <- err
				return
			}
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

// GetSingleAddonFromReader read single addon from Reader
func GetSingleAddonFromReader(r AsyncReader, addonName string, opt ListOptions) (*types.Addon, error) {
	var wg sync.WaitGroup
	var errs []error
	waitCh := make(chan struct{})
	readOption := map[string]struct {
		jumpConds bool
		read      func(wg *sync.WaitGroup, reader AsyncReader, path string)
	}{
		ReadmeFileName:     {!opt.GetDetail, readReadme},
		TemplateFileName:   {!opt.GetTemplate, readTemplate},
		MetadataFileName:   {false, readMetadata},
		DefinitionsDirName: {!opt.GetDefinition, readDefinitions},
		ResourcesDirName:   {!opt.GetResource && !opt.GetParameter, readResources},
		DefSchemaName:      {!opt.GetDefSchema, readDefSchemas},
	}

	_, items, err := r.Read(addonName)
	if err != nil {
		return nil, errors.Wrap(err, "fail to read")
	}
	for _, item := range items {
		itemName := strings.ToLower(item.GetName())
		switch itemName {
		case ReadmeFileName, MetadataFileName, DefinitionsDirName, ResourcesDirName, TemplateFileName, DefSchemaName:
			readMethod := readOption[itemName]
			if readMethod.jumpConds {
				break
			}

			wg.Add(1)
			go readMethod.read(&wg, r, r.RelativePath(item))
		}
	}
	go func() {
		wg.Wait()
		close(waitCh)
	}()

forLoop:
	for {
		select {
		case <-waitCh:
			break forLoop
		case err = <-r.ErrCh():
			errs = append(errs, err)
		}
	}

	if opt.GetParameter && r.Addon().Parameters != "" {
		err = genAddonAPISchema(r.Addon())
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return r.Addon(), compactErrors(fmt.Sprintf("error(s) happen when reading addon %s: ", addonName), errs)
	}

	return r.Addon(), nil
}

func readTemplate(wg *sync.WaitGroup, reader AsyncReader, readPath string) {
	defer wg.Done()
	data, _, err := reader.Read(readPath)
	if err != nil {
		reader.SendErr(err)
		return
	}
	dec := k8syaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	reader.Addon().AppTemplate = &v1beta1.Application{}
	_, _, err = dec.Decode([]byte(data), nil, reader.Addon().AppTemplate)
	if err != nil {
		reader.SendErr(err)
		return
	}
}

func readResources(wg *sync.WaitGroup, reader AsyncReader, readPath string) {
	defer wg.Done()
	_, items, err := reader.Read(readPath)
	if err != nil {
		reader.SendErr(err)
		return
	}
	for _, item := range items {
		switch item.GetType() {
		case "file":
			wg.Add(1)
			go readResFile(wg, reader, reader.RelativePath(item))
		case "dir":
			wg.Add(1)
			go readResources(wg, reader, reader.RelativePath(item))

		}
	}
}

func readDefSchemas(wg *sync.WaitGroup, reader AsyncReader, readPath string) {
	defer wg.Done()
	_, items, err := reader.Read(readPath)
	if err != nil {
		reader.SendErr(err)
		return
	}
	for _, item := range items {
		switch item.GetType() {
		case "file":
			wg.Add(1)
			go readDefSchemaFile(wg, reader, reader.RelativePath(item))
		case "dir":
			wg.Add(1)
			go readDefSchemas(wg, reader, reader.RelativePath(item))
		}
	}
}

// readResFile read single resource file
func readResFile(wg *sync.WaitGroup, reader AsyncReader, readPath string) {
	filename := path.Base(readPath)
	defer wg.Done()
	b, _, err := reader.Read(readPath)
	if err != nil {
		reader.SendErr(err)
		return
	}

	if filename == "parameter.cue" {
		reader.Addon().Parameters = b
		return
	}
	switch filepath.Ext(filename) {
	case ".cue":
		reader.Mutex().Lock()
		reader.Addon().CUETemplates = append(reader.Addon().CUETemplates, types.AddonElementFile{Data: b, Name: path.Base(readPath)})
		reader.Mutex().Unlock()
	default:
		reader.Mutex().Lock()
		reader.Addon().YAMLTemplates = append(reader.Addon().YAMLTemplates, types.AddonElementFile{Data: b, Name: path.Base(readPath)})
		reader.Mutex().Unlock()
	}
}

// readDefSchemaFile read single file of definition schema
func readDefSchemaFile(wg *sync.WaitGroup, reader AsyncReader, readPath string) {
	defer wg.Done()
	b, _, err := reader.Read(readPath)
	if err != nil {
		reader.SendErr(err)
		return
	}
	reader.Addon().DefSchemas = append(reader.Addon().DefSchemas, types.AddonElementFile{Data: b, Name: path.Base(readPath)})
}

func readDefinitions(wg *sync.WaitGroup, reader AsyncReader, readPath string) {
	defer wg.Done()
	_, items, err := reader.Read(readPath)
	if err != nil {
		reader.SendErr(err)
		return
	}
	for _, item := range items {
		switch item.GetType() {
		case "file":
			wg.Add(1)
			go readDefFile(wg, reader, reader.RelativePath(item))
		case "dir":
			wg.Add(1)
			go readDefinitions(wg, reader, reader.RelativePath(item))
		}
	}
}

// readDefFile read single definition file
func readDefFile(wg *sync.WaitGroup, reader AsyncReader, readPath string) {
	defer wg.Done()
	b, _, err := reader.Read(readPath)
	if err != nil {
		reader.SendErr(err)
		return
	}
	reader.Mutex().Lock()
	reader.Addon().Definitions = append(reader.Addon().Definitions, types.AddonElementFile{Data: b, Name: path.Base(readPath)})
	reader.Mutex().Unlock()
}

func readMetadata(wg *sync.WaitGroup, reader AsyncReader, readPath string) {
	defer wg.Done()
	b, _, err := reader.Read(readPath)
	if err != nil {
		reader.SendErr(err)
		return
	}
	err = yaml.Unmarshal([]byte(b), &reader.Addon().AddonMeta)
	if err != nil {
		reader.SendErr(err)
		return
	}
}

func readReadme(wg *sync.WaitGroup, reader AsyncReader, readPath string) {
	defer wg.Done()
	content, _, err := reader.Read(readPath)
	if err != nil {
		reader.SendErr(err)
		return
	}
	reader.Addon().Detail = content
}

func createGitHelper(content *utils.Content, token string) *gitHelper {
	var ts oauth2.TokenSource
	if token != "" {
		ts = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	}
	tc := oauth2.NewClient(context.Background(), ts)
	tc.Timeout = time.Second * 10
	cli := github.NewClient(tc)
	return &gitHelper{
		Client: cli,
		Meta:   content,
	}
}

// readRepo will read relative path (relative to Meta.Path)
func (h *gitHelper) readRepo(relativePath string) (*github.RepositoryContent, []*github.RepositoryContent, error) {
	file, items, _, err := h.Client.Repositories.GetContents(context.Background(), h.Meta.Owner, h.Meta.Repo, path.Join(h.Meta.Path, relativePath), nil)
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

// RenderAppAndResources render a K8s application
func RenderAppAndResources(addon *types.Addon, args map[string]interface{}) (*v1beta1.Application, []*unstructured.Unstructured, []*unstructured.Unstructured, error) {
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
			return nil, nil, nil, err
		}
		app.Spec.Components = append(app.Spec.Components, *comp)
	}
	for _, tmpl := range addon.CUETemplates {
		comp, err := renderCUETemplate(tmpl, addon.Parameters, args)
		if err != nil {
			return nil, nil, nil, ErrRenderCueTmpl
		}
		if addon.Name == "observability" && strings.HasSuffix(comp.Name, ".cue") {
			comp.Name = strings.Split(comp.Name, ".cue")[0]
		}
		app.Spec.Components = append(app.Spec.Components, *comp)
	}

	var schemaConfigmaps []*unstructured.Unstructured

	var defObjs []*unstructured.Unstructured

	if isDeployToRuntimeOnly(addon) {
		// Runtime cluster mode needs to deploy definitions to control plane k8s.
		for _, def := range addon.Definitions {
			obj, err := renderObject(def)
			if err != nil {
				return nil, nil, nil, err
			}
			defObjs = append(defObjs, obj)
		}
		for _, teml := range addon.DefSchemas {
			u, err := renderSchemaConfigmap(teml)
			if err != nil {
				return nil, nil, nil, err
			}
			schemaConfigmaps = append(schemaConfigmaps, u)
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
				return nil, nil, nil, err
			}
			app.Spec.Components = append(app.Spec.Components, *comp)
		}
		for _, teml := range addon.DefSchemas {
			u, err := renderSchemaConfigmap(teml)
			if err != nil {
				return nil, nil, nil, err
			}
			app.Spec.Components = append(app.Spec.Components, common2.ApplicationComponent{
				Name:       teml.Name,
				Type:       "raw",
				Properties: util.Object2RawExtension(u),
			})
		}
		if app.Spec.Workflow != nil && len(app.Spec.Workflow.Steps) == 0 {
			app.Spec.Workflow = nil
		}
	}

	return app, defObjs, schemaConfigmaps, nil
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
		Name: strings.ReplaceAll(elem.Name, ".", "-"),
	}
	obj, err := renderObject(elem)
	if err != nil {
		return nil, err
	}
	baseRawComponent.Properties = util.Object2RawExtension(obj)
	return &baseRawComponent, nil
}

func renderSchemaConfigmap(elem types.AddonElementFile) (*unstructured.Unstructured, error) {
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

// Convert2SecName generate addon argument secret name
func Convert2SecName(name string) string {
	return addonSecPrefix + name
}

// Handler helps addon enable, dependency-check, dispatch resources
type Handler struct {
	ctx    context.Context
	addon  *types.Addon
	cli    client.Client
	apply  apply.Applicator
	source Source
	args   map[string]interface{}
}

func newAddonHandler(ctx context.Context, addon *types.Addon, cli client.Client, apply apply.Applicator, source Source, args map[string]interface{}) Handler {
	return Handler{
		ctx:    ctx,
		addon:  addon,
		cli:    cli,
		apply:  apply,
		source: source,
		args:   args,
	}
}

func (h *Handler) enableAddon() error {
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
func (h *Handler) checkDependencies() error {
	var app v1beta1.Application
	for _, dep := range h.addon.Dependencies {
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
		// enable this addon if it's invisible
		depAddon, err := h.source.GetAddon(dep.Name, EnableLevelOptions)
		if err != nil {
			return errors.Wrap(err, "fail to find dependent addon in source repository")
		}
		// dep addon SHOULD be enabled without argument
		depHandler := *h
		depHandler.addon = depAddon
		depHandler.args = nil
		if err = depHandler.enableAddon(); err != nil {
			return errors.Wrap(err, "fail to dispatch dependent addon resource")
		}
	}
	return nil
}

func (h *Handler) dispatchAddonResource() error {
	app, defs, schemas, err := RenderAppAndResources(h.addon, h.args)
	if err != nil {
		return errors.Wrap(err, "render addon application fail")
	}

	err = h.apply.Apply(h.ctx, app)
	if err != nil {
		return errors.Wrap(err, "fail to create application")
	}

	for _, def := range defs {
		addOwner(def, app)
		err = h.apply.Apply(h.ctx, def)
		if err != nil {
			return err
		}
	}

	for _, schema := range schemas {
		addOwner(schema, app)
		err = h.apply.Apply(h.ctx, schema)
		if err != nil {
			return err
		}
	}

	if h.args != nil && len(h.args) > 0 {
		sec := RenderArgsSecret(h.addon, h.args)
		addOwner(sec, app)
		err = h.apply.Apply(h.ctx, sec)
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
