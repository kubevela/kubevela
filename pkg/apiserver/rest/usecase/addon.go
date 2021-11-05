package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	cueyaml "cuelang.org/go/encoding/yaml"
	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	restapis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	restutils "github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	cuemodel "github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils"
	addonutil "github.com/oam-dev/kubevela/pkg/utils/addon"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

const (
	// AddonReadmeFileName is the addon readme file name
	AddonReadmeFileName string = "readme.md"

	// AddonMetadataFileName is the addon meatadata.yaml file name
	AddonMetadataFileName string = "metadata.yaml"

	// AddonTemplateDirName is the addon template/ dir name
	AddonTemplateDirName string = "template"

	// AddonDefinitionsDirName is the addon definitions/ dir name
	AddonDefinitionsDirName string = "definitions"
)

// AddonUsecase addon usecase
type AddonUsecase interface {
	GetAddonRegistryModel(ctx context.Context, name string) (*model.AddonRegistry, error)
	CreateAddonRegistry(ctx context.Context, req apis.CreateAddonRegistryRequest) (*apis.AddonRegistryMeta, error)
	DeleteAddonRegistry(ctx context.Context, name string) error
	ListAddonRegistries(ctx context.Context) ([]*apis.AddonRegistryMeta, error)
	ListAddons(ctx context.Context, detailed bool, registry, query string) ([]*apis.DetailAddonResponse, error)
	StatusAddon(name string) (*apis.AddonStatusResponse, error)
	GetAddon(ctx context.Context, name string, registry string, detailed bool) (*apis.DetailAddonResponse, error)
	EnableAddon(ctx context.Context, name string, args apis.EnableAddonRequest) error
	DisableAddon(ctx context.Context, name string) error
}

// NewAddonUsecase returns a addon usecase
func NewAddonUsecase(ds datastore.DataStore) AddonUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		panic(err)
	}
	return &addonUsecaseImpl{
		ds:         ds,
		kubeClient: kubecli,
		apply:      apply.NewAPIApplicator(kubecli),
	}
}

type addonUsecaseImpl struct {
	ds         datastore.DataStore
	kubeClient client.Client
	apply      apply.Applicator
}

// GetAddon will get addon information, if detailed is not set, addon's componennt and internal definition won't be returned
func (u *addonUsecaseImpl) GetAddon(ctx context.Context, name string, registry string, detailed bool) (*apis.DetailAddonResponse, error) {
	addons, err := u.ListAddons(ctx, detailed, registry, "")
	if err != nil {
		return nil, err
	}

	for _, addon := range addons {
		if addon.Name == name {
			return addon, nil
		}
	}
	return nil, bcode.ErrAddonNotExist
}

func (u *addonUsecaseImpl) StatusAddon(name string) (*apis.AddonStatusResponse, error) {
	var app v1beta1.Application
	err := u.kubeClient.Get(context.Background(), client.ObjectKey{
		Namespace: types.DefaultKubeVelaNS,
		Name:      name,
	}, &app)
	if err != nil {
		if errors2.IsNotFound(err) {
			return &apis.AddonStatusResponse{
				Phase:            apis.AddonPhaseDisabled,
				EnablingProgress: nil,
			}, nil
		}
		return nil, bcode.ErrGetApplicationFail
	}

	switch app.Status.Phase {
	case common2.ApplicationRunning, common2.ApplicationWorkflowFinished:
		return &apis.AddonStatusResponse{
			Phase:            apis.AddonPhaseEnabled,
			EnablingProgress: nil,
		}, nil
	default:
		return &apis.AddonStatusResponse{
			Phase:            apis.AddonPhaseEnabling,
			EnablingProgress: nil,
		}, nil
	}
}

func (u *addonUsecaseImpl) ListAddons(ctx context.Context, detailed bool, registry, query string) ([]*apis.DetailAddonResponse, error) {
	var addons []*apis.DetailAddonResponse
	rs, err := u.ListAddonRegistries(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range rs {
		if registry != "" && r.Name != registry {
			continue
		}
		gitAddons, err := getAddonsFromGit(r.Git.URL, r.Git.Path, r.Git.Token, detailed)
		if err != nil {
			return nil, err
		}
		addons = mergeAddons(addons, gitAddons)
	}

	if query != "" {
		var filtered []*apis.DetailAddonResponse
		for i, addon := range addons {
			if strings.Contains(addon.Name, query) || strings.Contains(addon.Description, query) {
				filtered = append(filtered, addons[i])
			}
		}
		addons = filtered
	}

	sort.Slice(addons, func(i, j int) bool {
		return addons[i].Name < addons[j].Name
	})
	return addons, nil
}

func (u *addonUsecaseImpl) DeleteAddonRegistry(ctx context.Context, name string) error {
	return u.ds.Delete(ctx, &model.AddonRegistry{Name: name})
}

func (u *addonUsecaseImpl) CreateAddonRegistry(ctx context.Context, req apis.CreateAddonRegistryRequest) (*apis.AddonRegistryMeta, error) {
	r := addonRegistryModelFromCreateAddonRegistryRequest(req)

	err := u.ds.Add(ctx, r)
	if err != nil {
		if errors.Is(err, datastore.ErrRecordExist) {
			return nil, bcode.ErrAddonRegistryExist
		}
		return nil, err
	}

	return &apis.AddonRegistryMeta{
		Name: r.Name,
		Git:  r.Git,
	}, nil
}

func (u *addonUsecaseImpl) GetAddonRegistryModel(ctx context.Context, name string) (*model.AddonRegistry, error) {
	var r = model.AddonRegistry{
		Name: name,
	}
	err := u.ds.Get(ctx, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (u *addonUsecaseImpl) ListAddonRegistries(ctx context.Context) ([]*apis.AddonRegistryMeta, error) {
	var r = model.AddonRegistry{}
	entities, err := u.ds.List(ctx, &r, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	var list []*apis.AddonRegistryMeta
	for _, entity := range entities {
		list = append(list, restutils.ConvertAddonRegistryModel2AddonRegistryMeta(entity.(*model.AddonRegistry)))
	}
	return list, nil
}

func renderApplication(addon *restapis.DetailAddonResponse, args *apis.EnableAddonRequest) (*v1beta1.Application, error) {
	if args == nil {
		args = &apis.EnableAddonRequest{Args: map[string]string{}}
	}
	app := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: "Application"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      addon.Name,
			Namespace: types.DefaultKubeVelaNS,
			Labels: map[string]string{
				oam.LabelAddonName: addon.Name,
			},
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common2.ApplicationComponent{},
		},
	}
	for _, tmpl := range addon.YAMLTemplates {
		comp, err := renderRawComponent(tmpl)
		if err != nil {
			return nil, err
		}
		app.Spec.Components = append(app.Spec.Components, *comp)
	}
	for _, tmpl := range addon.CUETemplates {
		yamlData, err := renderCUETemplate(tmpl.Data, addon.Parameters, args.Args)
		if err != nil {
			log.Logger.Errorf("failed to render CUE template: %v", err)
			return nil, bcode.ErrAddonRenderFail
		}
		comp, err := renderRawComponent(apis.AddonElementFile{Data: yamlData, Name: tmpl.Name, Path: tmpl.Path})
		if err != nil {
			return nil, err
		}
		app.Spec.Components = append(app.Spec.Components, *comp)
	}
	return app, nil
}

func renderCUETemplate(template string, parameters string, args map[string]string) (string, error) {
	bt, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	var paramFile = cuemodel.ParameterFieldName + ": {}"
	if string(bt) != "null" {
		paramFile = fmt.Sprintf("%s: %s", cuemodel.ParameterFieldName, string(bt))
	}
	param := fmt.Sprintf("%s\n%s", paramFile, parameters)
	v, err := value.NewValue(param, nil, "")
	if err != nil {
		return "", err
	}
	out, err := v.LookupByScript(fmt.Sprintf("{%s}", template))
	if err != nil {
		return "", err
	}
	b, err := cueyaml.Encode(out.CueValue())
	return string(b), err
}

func (u *addonUsecaseImpl) EnableAddon(ctx context.Context, name string, args apis.EnableAddonRequest) error {
	addon, err := u.GetAddon(ctx, name, "", true)
	if err != nil {
		return err
	}
	app, err := renderApplication(addon, &args)
	if err != nil {
		return err
	}
	err = u.kubeClient.Create(ctx, app)
	if err != nil {
		log.Logger.Errorf("apply application fail: %s", err.Error())
		return bcode.ErrAddonApplyFail
	}
	return nil

}

func (u *addonUsecaseImpl) DisableAddon(ctx context.Context, name string) error {
	app := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: "Application"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: types.DefaultKubeVelaNS,
		},
	}
	err := u.kubeClient.Delete(ctx, app)
	if err != nil {
		log.Logger.Errorf("delete application fail: %s", err.Error())
		return err
	}
	return nil
}

// renderRawComponent will return a component in raw type from string
func renderRawComponent(elem apis.AddonElementFile) (*common2.ApplicationComponent, error) {
	baseRawComponent := common2.ApplicationComponent{
		Type: "raw",
		Name: strings.Join(append(elem.Path, elem.Name), "-"),
	}
	obj := &unstructured.Unstructured{}
	dec := k8syaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err := dec.Decode([]byte(elem.Data), nil, obj)
	if err != nil {
		fmt.Println(err)
	}
	baseRawComponent.Properties = util.Object2RawExtension(obj)
	return &baseRawComponent, nil
}

func addonRegistryModelFromCreateAddonRegistryRequest(req apis.CreateAddonRegistryRequest) *model.AddonRegistry {
	return &model.AddonRegistry{
		Name: req.Name,
		Git:  req.Git,
	}
}

func mergeAddons(a1, a2 []*apis.DetailAddonResponse) []*apis.DetailAddonResponse {
	for _, item := range a2 {
		if hasAddon(a1, item.Name) {
			continue
		}
		a1 = append(a1, item)
	}
	return a1
}

func hasAddon(addons []*apis.DetailAddonResponse, name string) bool {
	for _, addon := range addons {
		if addon.Name == name {
			return true
		}
	}
	return false
}

type gitHelper struct {
	Client *github.Client
	Meta   *utils.Content
}

func getAddonsFromGit(baseURL, dir, token string, detailed bool) ([]*apis.DetailAddonResponse, error) {
	var addons []*apis.DetailAddonResponse

	gith, err := createGitHelper(baseURL, dir, token)
	if err != nil {
		return nil, err
	}
	dirs, err := readRepo(gith)
	if err != nil {
		return nil, err
	}

	for _, subItems := range dirs {
		if subItems.GetType() != "dir" {
			continue
		}
		addonRes := &apis.DetailAddonResponse{}
		_, files, _, err := gith.Client.Repositories.GetContents(context.Background(), gith.Meta.Owner, gith.Meta.Repo, subItems.GetPath(), nil)
		if err != nil {
			log.Logger.Errorf("failed to read dir %s: %v", subItems.GetPath(), err)
			continue
		}
		for _, file := range files {
			var err error

			switch strings.ToLower(file.GetName()) {
			case AddonReadmeFileName:
				if !detailed {
					break
				}
				err = readReadme(addonRes, gith, file)
			case AddonMetadataFileName:
				err = readMetadata(addonRes, gith, file)
				addonRes.Name = addonutil.TransAddonName(addonRes.Name)
			case AddonDefinitionsDirName:
				if !detailed {
					break
				}
				err = readDefinitions(addonRes, gith, file)
			case AddonTemplateDirName:
				if !detailed {
					break
				}
				err = readTemplates(addonRes, gith, file)
			}

			if err != nil {
				log.Logger.Errorf("failed to read file %s: %v", file.GetPath(), err)
				continue
			}
		}

		addons = append(addons, addonRes)
	}
	return addons, nil
}

func readTemplates(addon *apis.DetailAddonResponse, h *gitHelper, dir *github.RepositoryContent) error {
	dirPath := strings.Split(dir.GetPath(), "/")
	// remove
	for i, d := range dirPath {
		if d == AddonTemplateDirName {
			dirPath = dirPath[i:]
			break
		}
		dirPath = dirPath[i:]
	}

	_, files, _, err := h.Client.Repositories.GetContents(context.Background(), h.Meta.Owner, h.Meta.Repo, *dir.Path, nil)
	if err != nil {
		return err
	}
	for _, file := range files {
		switch file.GetType() {
		case "file":
			content, _, _, err := h.Client.Repositories.GetContents(context.Background(), h.Meta.Owner, h.Meta.Repo, *file.Path, nil)
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
				addon.CUETemplates = append(addon.CUETemplates, apis.AddonElementFile{Data: b, Name: file.GetName(), Path: dirPath})
			default:
				addon.YAMLTemplates = append(addon.YAMLTemplates, apis.AddonElementFile{Data: b, Name: file.GetName(), Path: dirPath})
			}
		case "dir":
			err = readTemplates(addon, h, file)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func readDefinitions(addon *apis.DetailAddonResponse, h *gitHelper, dir *github.RepositoryContent) error {
	_, files, _, err := h.Client.Repositories.GetContents(context.Background(), h.Meta.Owner, h.Meta.Repo, *dir.Path, nil)
	if err != nil {
		return err
	}
	for _, file := range files {
		switch file.GetType() {
		case "file":
			content, _, _, err := h.Client.Repositories.GetContents(context.Background(), h.Meta.Owner, h.Meta.Repo, *file.Path, nil)
			if err != nil {
				return err
			}
			b, err := content.GetContent()
			if err != nil {
				return err
			}
			d, err := getDefinitionMetaFromYAML(b)
			if err != nil {
				return err
			}
			addon.Definitions = append(addon.Definitions, d)
		case "dir":
			err = readDefinitions(addon, h, file)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func getDefinitionMetaFromYAML(data string) (*apis.Definition, error) {
	dec := k8syaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	obj := &unstructured.Unstructured{}
	_, _, err := dec.Decode([]byte(data), nil, obj)
	if err != nil {
		return nil, bcode.ErrAddonRenderFail
	}
	d := &apis.Definition{
		Name: obj.GetName(),
		Kind: obj.GetKind(),
	}
	if ann := obj.GetAnnotations(); ann != nil {
		d.Description = ann[types.AnnDescription]
	}
	return d, nil
}

func readMetadata(addon *apis.DetailAddonResponse, h *gitHelper, file *github.RepositoryContent) error {
	content, _, _, err := h.Client.Repositories.GetContents(context.Background(), h.Meta.Owner, h.Meta.Repo, *file.Path, nil)
	if err != nil {
		return err
	}
	b, err := content.GetContent()
	if err != nil {
		return err
	}
	return yaml.Unmarshal([]byte(b), &addon.AddonMeta)
}

func readReadme(addon *apis.DetailAddonResponse, h *gitHelper, file *github.RepositoryContent) error {
	content, _, _, err := h.Client.Repositories.GetContents(context.Background(), h.Meta.Owner, h.Meta.Repo, *file.Path, nil)
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
		return nil, err
	}
	u.Path = path.Join(u.Path, dir)
	_, gitmeta, err := utils.Parse(u.String())
	if err != nil {
		return nil, err
	}

	return &gitHelper{
		Client: cli,
		Meta:   gitmeta,
	}, nil
}

func readRepo(h *gitHelper) ([]*github.RepositoryContent, error) {
	_, dirs, _, err := h.Client.Repositories.GetContents(context.Background(), h.Meta.Owner, h.Meta.Repo, h.Meta.Path, nil)
	if err != nil {
		return nil, err
	}
	return dirs, nil
}
