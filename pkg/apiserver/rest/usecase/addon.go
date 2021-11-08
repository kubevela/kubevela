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

	// AddonTemplateFileName is the addon template.yaml dir name
	AddonTemplateFileName string = "template.yaml"

	// AddonResourcesDirName is the addon resources/ dir name
	AddonResourcesDirName string = "resources"

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
		addonRegistryCache: make(map[string]*restutils.MemoryCache),
		addonRegistryDS:    ds,
		kubeClient:         kubecli,
		apply:              apply.NewAPIApplicator(kubecli),
	}
}

type addonUsecaseImpl struct {
	addonRegistryCache map[string]*restutils.MemoryCache
	addonRegistryDS    datastore.DataStore
	kubeClient         client.Client
	apply              apply.Applicator
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
		Name:      AddonName2AppName(name),
	}, &app)
	if err != nil {
		if errors2.IsNotFound(err) {
			return &apis.AddonStatusResponse{
				Phase:            apis.AddonPhaseDisabled,
				EnablingProgress: nil,
			}, nil
		}
		return nil, bcode.ErrGetAddonApplication
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

		var gitAddons []*apis.DetailAddonResponse
		if u.isRegistryCacheUpToDate(registry) {
			gitAddons = u.getRegistryCache(registry)
		} else {
			gitAddons, err = getAddonsFromGit(r.Git.URL, r.Git.Path, r.Git.Token, detailed)
			if err != nil {
				log.Logger.Errorf("fail to get addons from registry %s", r.Name)
				continue
			}
			u.putRegistryCache(registry, gitAddons)
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
	return u.addonRegistryDS.Delete(ctx, &model.AddonRegistry{Name: name})
}

func (u *addonUsecaseImpl) CreateAddonRegistry(ctx context.Context, req apis.CreateAddonRegistryRequest) (*apis.AddonRegistryMeta, error) {
	r := addonRegistryModelFromCreateAddonRegistryRequest(req)

	err := u.addonRegistryDS.Add(ctx, r)
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
	err := u.addonRegistryDS.Get(ctx, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (u *addonUsecaseImpl) ListAddonRegistries(ctx context.Context) ([]*apis.AddonRegistryMeta, error) {
	var r = model.AddonRegistry{}

	var list []*apis.AddonRegistryMeta
	entities, err := u.addonRegistryDS.List(ctx, &r, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, entity := range entities {
		list = append(list, ConvertAddonRegistryModel2AddonRegistryMeta(entity.(*model.AddonRegistry)))
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return list, nil
}

func renderApplication(addon *apis.DetailAddonResponse, args *apis.EnableAddonRequest) (*v1beta1.Application, error) {
	if args == nil {
		args = &apis.EnableAddonRequest{Args: map[string]string{}}
	}
	app := addon.AppTemplate
	if app == nil {
		app = &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: "Application"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      AddonName2AppName(addon.Name),
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
	app.Name = AddonName2AppName(addon.Name)
	app.Labels = util.MergeMapOverrideWithDst(app.Labels, map[string]string{oam.LabelAddonName: addon.Name})

	for _, tmpl := range addon.YAMLTemplates {
		comp, err := renderRawComponent(tmpl)
		if err != nil {
			return nil, err
		}
		app.Spec.Components = append(app.Spec.Components, *comp)
	}
	for _, tmpl := range addon.CUETemplates {
		comp, err := renderCUETemplate(tmpl, addon.Parameters, args.Args)
		if err != nil {
			log.Logger.Errorf("failed to render CUE template: %v", err)
			return nil, bcode.ErrAddonRender
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
		return bcode.ErrAddonApply
	}
	return nil
}

func (u *addonUsecaseImpl) getRegistryCache(name string) []*apis.DetailAddonResponse {
	return u.addonRegistryCache[name].GetData().([]*apis.DetailAddonResponse)
}

func (u *addonUsecaseImpl) putRegistryCache(name string, addons []*apis.DetailAddonResponse) {
	u.addonRegistryCache[name] = restutils.NewMemoryCache(addons, time.Minute*3)
}

func (u *addonUsecaseImpl) isRegistryCacheUpToDate(name string) bool {
	d, ok := u.addonRegistryCache[name]
	if !ok {
		return false
	}
	return !d.IsExpired()
}

func (u *addonUsecaseImpl) DisableAddon(ctx context.Context, name string) error {
	app := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: "Application"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      AddonName2AppName(name),
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
		return nil, err
	}
	baseRawComponent.Properties = util.Object2RawExtension(obj)
	return &baseRawComponent, nil
}

// renderCUETemplate will return a component from cue template
func renderCUETemplate(elem apis.AddonElementFile, parameters string, args map[string]string) (*common2.ApplicationComponent, error) {
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

	comp := common2.ApplicationComponent{
		Name: strings.Join(append(elem.Path, elem.Name), "-"),
	}
	err = yaml.Unmarshal(b, &comp)
	if err != nil {
		return nil, err
	}

	return &comp, err
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
			if bcode.IsGithubRateLimit(err) {
				return nil, bcode.ErrAddonRegistryRateLimit
			}
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
			case AddonResourcesDirName:
				if !detailed {
					break
				}
				err = readResources(addonRes, gith, file)
			case AddonTemplateFileName:
				if !detailed {
					break
				}
				err = readTemplate(addonRes, gith, file)
			}

			if err != nil {
				if bcode.IsGithubRateLimit(err) {
					return nil, bcode.ErrAddonRegistryRateLimit
				}
				log.Logger.Errorf("failed to read file %s: %v", file.GetPath(), err)
				continue
			}
		}

		addons = append(addons, addonRes)
	}
	return addons, nil
}

func cutPathUntil(path []string, end string) ([]string, error) {
	for i, d := range path {
		if d == end {
			return path[i:], nil
		}
	}
	return nil, errors.New("cut path fail, target directory name not found")
}

func readTemplate(addon *apis.DetailAddonResponse, h *gitHelper, file *github.RepositoryContent) error {
	content, _, _, err := h.Client.Repositories.GetContents(context.Background(), h.Meta.Owner, h.Meta.Repo, *file.Path, nil)
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

func readResources(addon *apis.DetailAddonResponse, h *gitHelper, dir *github.RepositoryContent) error {
	dirPath := strings.Split(dir.GetPath(), "/")
	dirPath, err := cutPathUntil(dirPath, AddonResourcesDirName)
	if err != nil {
		return err
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
			err = readResources(addon, h, file)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func readDefinitions(addon *apis.DetailAddonResponse, h *gitHelper, dir *github.RepositoryContent) error {
	dirPath := strings.Split(dir.GetPath(), "/")
	dirPath, err := cutPathUntil(dirPath, AddonDefinitionsDirName)
	if err != nil {
		return err
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
			addon.Definitions = append(addon.Definitions, apis.AddonElementFile{Data: b, Name: file.GetName(), Path: dirPath})
		case "dir":
			err = readDefinitions(addon, h, file)
			if err != nil {
				return err
			}
		}
	}
	return nil
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
		log.Logger.Errorf("parsing %s failed: %v", baseURL, err)
		return nil, bcode.ErrAddonRegistryInvalid
	}
	u.Path = path.Join(u.Path, dir)
	_, gitmeta, err := utils.Parse(u.String())
	if err != nil {
		log.Logger.Errorf("parsing %s failed: %v", u.String(), err)
		return nil, bcode.ErrAddonRegistryInvalid
	}

	return &gitHelper{
		Client: cli,
		Meta:   gitmeta,
	}, nil
}

func readRepo(h *gitHelper) ([]*github.RepositoryContent, error) {
	_, dirs, _, err := h.Client.Repositories.GetContents(context.Background(), h.Meta.Owner, h.Meta.Repo, h.Meta.Path, nil)
	if err != nil {
		log.Logger.Errorf("readRepo fail: %v", err)
		return nil, bcode.WrapGithubRateLimitErr(err)
	}
	return dirs, nil
}

// ConvertAddonRegistryModel2AddonRegistryMeta will convert from model to AddonRegistryMeta
func ConvertAddonRegistryModel2AddonRegistryMeta(r *model.AddonRegistry) *apis.AddonRegistryMeta {
	return &apis.AddonRegistryMeta{
		Name: r.Name,
		Git:  r.Git,
	}
}

const addonAppPrefix = "addon-"

// AddonName2AppName -
func AddonName2AppName(name string) string {
	return addonAppPrefix + name
}

// AppName2addonName -
func AppName2addonName(name string) string {
	return strings.TrimPrefix(name, addonAppPrefix)
}
