package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig"
	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
	"github.com/oam-dev/kubevela/pkg/utils"
	addonutil "github.com/oam-dev/kubevela/pkg/utils/addon"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

const (
	// AddonFileName is the addon file name
	AddonFileName string = "addon.yaml"
	// AddonReadmeFileName is the addon readme file name
	AddonReadmeFileName string = "readme.md"
)

// AddonUsecase addon usecase
type AddonUsecase interface {
	GetAddonRegistryModel(ctx context.Context, name string) (*model.AddonRegistry, error)
	CreateAddonRegistry(ctx context.Context, req apis.CreateAddonRegistryRequest) (*apis.AddonRegistryMeta, error)
	ListAddonRegistries(ctx context.Context) ([]*apis.AddonRegistryMeta, error)
	ListAddons(ctx context.Context, detailed bool, query string) ([]*apis.DetailAddonResponse, error)
	StatusAddon(name string) (*apis.AddonStatusResponse, error)
	GetAddon(ctx context.Context, name string) (*apis.DetailAddonResponse, error)
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

func (u *addonUsecaseImpl) GetAddon(ctx context.Context, name string) (*apis.DetailAddonResponse, error) {
	addons, err := u.ListAddons(ctx, true, "")
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
	_, err := u.GetAddon(context.TODO(), name)
	if err != nil {
		return nil, err
	}

	var app v1beta1.Application
	err = u.kubeClient.Get(context.Background(), client.ObjectKey{
		Namespace: types.DefaultKubeVelaNS,
		Name:      addonutil.TransAddonName(name),
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

func (u *addonUsecaseImpl) ListAddons(ctx context.Context, detailed bool, query string) ([]*apis.DetailAddonResponse, error) {
	// Backward compatibility with ConfigMap addons.
	// We will deprecate ConfigMap and use Git based registry.
	addons, err := getAddonsFromConfigMap(detailed)
	if err != nil {
		return nil, err
	}

	rs, err := u.ListAddonRegistries(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range rs {
		gitAddons, err := getAddonsFromGit(r.Git.URL, r.Git.Path, r.Git.Token, detailed)
		if err != nil {
			log.Logger.Errorf("list addons from registry %s failure %s", r.Name, err.Error())
			continue
		}
		addons = mergeAddons(addons, gitAddons)
	}
	if query != "" {
		var new []*apis.DetailAddonResponse
		for i, addon := range addons {
			if strings.Contains(addon.Name, query) || strings.Contains(addon.Description, query) {
				new = append(new, addons[i])
			}
		}
		addons = new
	}
	sort.Slice(addons, func(i, j int) bool {
		return addons[i].Name < addons[j].Name
	})
	return addons, nil
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

func (u *addonUsecaseImpl) EnableAddon(ctx context.Context, name string, args apis.EnableAddonRequest) error {
	addon, err := u.GetAddon(ctx, name)
	if err != nil {
		return err
	}
	err = u.applyAddonData(addon.DeployData, args)
	if err != nil {
		return err
	}
	return nil

}

func (u *addonUsecaseImpl) DisableAddon(ctx context.Context, name string) error {
	addon, err := u.GetAddon(ctx, name)
	if err != nil {
		return err
	}
	err = u.deleteAddonData(addon.DeployData)
	if err != nil {
		return err
	}
	return nil
}

func (u *addonUsecaseImpl) applyAddonData(data string, request apis.EnableAddonRequest) error {
	app, err := renderAddonApp(data, &request)
	if err != nil {
		return err
	}
	applicator := apply.NewAPIApplicator(u.kubeClient)
	err = applicator.Apply(context.TODO(), app)
	if err != nil {
		log.Logger.Errorf("apply application fail: %s", err.Error())
		return bcode.ErrAddonApplyFail
	}
	return nil
}

func (u *addonUsecaseImpl) deleteAddonData(data string) error {
	app, err := renderAddonApp(data, nil)
	if err != nil {
		return err
	}
	err = u.kubeClient.Get(context.Background(), client.ObjectKey{
		Namespace: app.GetNamespace(),
		Name:      app.GetName(),
	}, app)
	if err != nil {
		return bcode.ErrAddonNotEnabled
	}
	err = u.kubeClient.Delete(context.Background(), app)
	if err != nil {
		return bcode.ErrAddonDisableFail
	}
	return nil

}

// renderAddonApp can render string to unstructured, args can be nil
func renderAddonApp(data string, args *apis.EnableAddonRequest) (*unstructured.Unstructured, error) {
	if args == nil {
		args = &apis.EnableAddonRequest{Args: map[string]string{}}
	}

	t, err := template.New("addon-template").Delims("[[", "]]").Funcs(sprig.TxtFuncMap()).Parse(data)
	if err != nil {
		return nil, bcode.ErrAddonRenderFail
	}
	buf := bytes.Buffer{}
	err = t.Execute(&buf, args)
	if err != nil {
		return nil, bcode.ErrAddonRenderFail
	}
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	obj := &unstructured.Unstructured{}
	_, _, err = dec.Decode(buf.Bytes(), nil, obj)
	if err != nil {
		return nil, bcode.ErrAddonRenderFail
	}
	return obj, nil
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

func getAddonsFromGit(baseURL, dir, token string, detailed bool) ([]*apis.DetailAddonResponse, error) {
	addons := []*apis.DetailAddonResponse{}
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	var tc *http.Client
	if token != "" {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		tc = oauth2.NewClient(context.Background(), ts)
	}
	tc.Timeout = time.Second * 10
	clt := github.NewClient(tc)
	// TODO add error handling
	baseURL = strings.TrimSuffix(baseURL, ".git")
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, dir)
	_, content, err := utils.Parse(u.String())
	if err != nil {
		return nil, err
	}
	_, dirs, _, err := clt.Repositories.GetContents(context.Background(), content.Owner, content.Repo, content.Path, nil)
	if err != nil {
		return nil, err
	}
	for _, subItems := range dirs {
		if *subItems.Type == "file" {
			continue
		}
		addonRes := apis.DetailAddonResponse{
			AddonMeta: apis.AddonMeta{
				Name: converAddonName(*subItems.Name),
			},
		}
		var err error
		_, files, _, err := clt.Repositories.GetContents(context.Background(), content.Owner, content.Repo, *subItems.Path, nil)
		// get addon.yaml and readme.md
		for _, file := range files {
			switch *file.Name {
			case AddonFileName:
				addonContent, _, _, err := clt.Repositories.GetContents(context.Background(), content.Owner, content.Repo, *file.Path, nil)
				if err != nil {
					break
				}
				addonStr, _ := addonContent.GetContent()
				obj := &unstructured.Unstructured{}
				_, _, err = dec.Decode([]byte(addonStr), nil, obj)
				if err != nil {
					break
				}
				addonRes.AddonMeta.Description = obj.GetAnnotations()[addonutil.DescAnnotation]
				addonRes.DeployData = addonStr
			case AddonReadmeFileName:
				if detailed {
					detailContent, _, _, err := clt.Repositories.GetContents(context.Background(), content.Owner, content.Repo, *file.Path, nil)
					if err != nil {
						break
					}
					addonRes.Detail, err = detailContent.GetContent()
					if err != nil {
						break
					}
				}
			default:
				continue
			}

		}
		if err != nil {
			continue
		}
		addons = append(addons, &addonRes)
	}
	return addons, nil
}

func getAddonsFromConfigMap(detailed bool) ([]*apis.DetailAddonResponse, error) {
	repo, err := addonutil.NewAddonRepo()
	if err != nil {
		return nil, fmt.Errorf("failed to get configMap addon repo: %w", err)
	}
	cliAddons := repo.ListAddons()
	addons := []*apis.DetailAddonResponse{}
	for _, addon := range cliAddons {
		d := &apis.DetailAddonResponse{
			AddonMeta: apis.AddonMeta{
				Name: converAddonName(addon.Name),
				// TODO add actual Version, Icon, tags
				Version:     "v1alpha1",
				Description: addon.Description,
				Icon:        "",
				Tags:        nil,
			},
			DeployData: addon.Data,
		}
		if detailed {
			d.Detail = addon.Detail
		}
		addons = append(addons, d)
	}
	return addons, nil
}

func converAddonName(name string) string {
	return strings.ReplaceAll(name, "/", "-")
}
