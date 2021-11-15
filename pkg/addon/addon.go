package addon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	"cuelang.org/go/cue"
	cueyaml "cuelang.org/go/encoding/yaml"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	utils2 "github.com/oam-dev/kubevela/pkg/controller/utils"
	cuemodel "github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils"
	addonutil "github.com/oam-dev/kubevela/pkg/utils/addon"
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

// GetAddon get a detailed addon info from GitAddonSource
func GetAddon(name string, git *GitAddonSource) (*types.Addon, error) {
	addons, err := ListAddons(true, git)
	if err != nil {
		return nil, err
	}

	for _, addon := range addons {
		if addon.Name == name {
			return addon, nil
		}
	}
	return nil, errors.New("addon not exist")
}

// ListAddons list addons' info from GitAddonSource, if not detailed, result only contains types.AddonMeta
func ListAddons(detailed bool, git *GitAddonSource) ([]*types.Addon, error) {
	var gitAddons []*types.Addon
	gitAddons, err := getAddonsFromGit(git.URL, git.Path, git.Token, detailed)
	if err != nil {
		return nil, err
	}
	return gitAddons, nil
}

func getAddonsFromGit(baseURL, dir, token string, detailed bool) ([]*types.Addon, error) {
	var addons []*types.Addon

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
		addonRes := &types.Addon{}
		_, files, err := gith.readRepo(subItems.GetPath())
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			var err error

			switch strings.ToLower(file.GetName()) {
			case ReadmeFileName:
				if !detailed {
					break
				}
				err = readReadme(addonRes, gith, file)
			case MetadataFileName:
				err = readMetadata(addonRes, gith, file)
				addonRes.Name = addonutil.TransAddonName(addonRes.Name)
			case DefinitionsDirName:
				if !detailed {
					break
				}
				err = readDefinitions(addonRes, gith, file)
			case ResourcesDirName:
				if !detailed {
					break
				}
				err = readResources(addonRes, gith, file)
			case TemplateFileName:
				if !detailed {
					break
				}
				err = readTemplate(addonRes, gith, file)
			}

			if err != nil {
				return nil, err
			}
		}

		if detailed && addonRes.Parameters != "" {
			err = genAddonAPISchema(addonRes)
			if err != nil {
				continue
			}
		}
		addons = append(addons, addonRes)
	}
	return addons, nil
}

func readTemplate(addon *types.Addon, h *gitHelper, file *github.RepositoryContent) error {
	content, _, err := h.readRepo(*file.Path)
	if err != nil {
		return err
	}
	data, err := content.GetContent()
	if err != nil {
		return err
	}
	dec := k8syaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	addon.AppTemplate = &v1beta1.Application{}
	_, _, err = dec.Decode([]byte(data), nil, addon.AppTemplate)
	if err != nil {
		return err
	}
	return nil
}

func readResources(addon *types.Addon, h *gitHelper, dir *github.RepositoryContent) error {
	dirPath := strings.Split(dir.GetPath(), "/")
	dirPath, err := cutPathUntil(dirPath, ResourcesDirName)
	if err != nil {
		return err
	}

	_, files, err := h.readRepo(*dir.Path)
	if err != nil {
		return err
	}
	for _, file := range files {
		switch file.GetType() {
		case "file":
			content, _, err := h.readRepo(*file.Path)
			if err != nil {
				return err
			}
			b, err := content.GetContent()
			if err != nil {
				return err
			}

			if file.GetName() == "parameter.cue" {
				addon.Parameters = b
				break
			}
			switch filepath.Ext(file.GetName()) {
			case ".cue":
				addon.CUETemplates = append(addon.CUETemplates, types.AddonElementFile{Data: b, Name: file.GetName(), Path: dirPath})
			default:
				addon.YAMLTemplates = append(addon.YAMLTemplates, types.AddonElementFile{Data: b, Name: file.GetName(), Path: dirPath})
			}
		case "dir":
			err = readResources(addon, h, file)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func readDefinitions(addon *types.Addon, h *gitHelper, dir *github.RepositoryContent) error {
	dirPath := strings.Split(dir.GetPath(), "/")
	dirPath, err := cutPathUntil(dirPath, DefinitionsDirName)
	if err != nil {
		return err
	}
	_, files, err := h.readRepo(*dir.Path)
	if err != nil {
		return err
	}
	for _, file := range files {
		switch file.GetType() {
		case "file":
			content, _, err := h.readRepo(*file.Path)
			if err != nil {
				return err
			}
			b, err := content.GetContent()
			if err != nil {
				return err
			}
			addon.Definitions = append(addon.Definitions, types.AddonElementFile{Data: b, Name: file.GetName(), Path: dirPath})
		case "dir":
			err = readDefinitions(addon, h, file)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func readMetadata(addon *types.Addon, h *gitHelper, file *github.RepositoryContent) error {
	content, _, err := h.readRepo(*file.Path)
	if err != nil {
		return err
	}
	b, err := content.GetContent()
	if err != nil {
		return err
	}
	return yaml.Unmarshal([]byte(b), &addon.AddonMeta)
}

func readReadme(addon *types.Addon, h *gitHelper, file *github.RepositoryContent) error {
	content, _, err := h.readRepo(*file.Path)
	if err != nil {
		return err
	}
	addon.Detail, err = content.GetContent()
	return err
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
	schema := &openapi3.Schema{}
	if err := schema.UnmarshalJSON(data); err != nil {
		return err
	}
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
func RenderApplication(addon *types.Addon, args map[string]string) (*v1beta1.Application, error) {
	if args == nil {
		args = map[string]string{}
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

	for _, tmpl := range addon.YAMLTemplates {
		comp, err := renderRawComponent(tmpl)
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
		app.Spec.Components = append(app.Spec.Components, *comp)
	}
	for _, def := range addon.Definitions {
		comp, err := renderRawComponent(def)
		if err != nil {
			return nil, err
		}
		app.Spec.Components = append(app.Spec.Components, *comp)
	}

	return app, nil
}

// renderRawComponent will return a component in raw type from string
func renderRawComponent(elem types.AddonElementFile) (*common2.ApplicationComponent, error) {
	baseRawComponent := common2.ApplicationComponent{
		Type: "raw",
		Name: strings.Join(append(elem.Path, elem.Name), "-"),
	}
	obj := &unstructured.Unstructured{}
	dec := k8syaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err := dec.Decode([]byte(elem.Data), nil, obj)
	if err != nil {
		return nil, err
	}
	baseRawComponent.Properties = util.Object2RawExtension(obj)
	return &baseRawComponent, nil
}

// renderCUETemplate will return a component from cue template
func renderCUETemplate(elem types.AddonElementFile, parameters string, args map[string]string) (*common2.ApplicationComponent, error) {
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
	out, err := v.LookupByScript(fmt.Sprintf("{%s}", elem.Data))
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
		Name: strings.Join(append(elem.Path, elem.Name), "-"),
	}
	err = yaml.Unmarshal(b, &comp)
	if err != nil {
		return nil, err
	}

	return &comp, err
}

const addonAppPrefix = "addon-"

// Convert2AppName -
func Convert2AppName(name string) string {
	return addonAppPrefix + name
}

// Convert2AddonName -
func Convert2AddonName(name string) string {
	return strings.TrimPrefix(name, addonAppPrefix)
}
