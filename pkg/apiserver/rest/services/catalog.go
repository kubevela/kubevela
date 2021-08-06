package services

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	echo "github.com/labstack/echo/v4"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/proto/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/apis"
)

// CatalogService catalog service
type CatalogService struct {
	k8sClient client.Client
}

// NewCatalogService new catalog service
func NewCatalogService(client client.Client) *CatalogService {

	return &CatalogService{
		k8sClient: client,
	}
}

// ListCatalogs list method for catalog configmap
func (s *CatalogService) ListCatalogs(c echo.Context) error {
	var cmList v1.ConfigMapList
	labels := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"catalog": "configdata",
		},
	}
	selector, err := metav1.LabelSelectorAsSelector(labels)
	if err != nil {
		return err
	}
	err = s.k8sClient.List(context.Background(), &cmList, &client.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return err
	}
	var catalogList = make([]*model.Catalog, 0, len(cmList.Items))
	for i, c := range cmList.Items {
		UpdateInt, err := strconv.ParseInt(cmList.Items[i].Data["UpdatedAt"], 10, 64)
		if err != nil {
			return err
		}
		catalog := model.Catalog{
			Name:      c.Name,
			UpdatedAt: UpdateInt,
			Desc:      cmList.Items[i].Data["Desc"],
			Type:      cmList.Items[i].Data["Type"],
			Url:       cmList.Items[i].Data["Url"],
			Token:     cmList.Items[i].Data["Token"],
		}
		catalogList = append(catalogList, &catalog)
	}

	return c.JSON(http.StatusOK, model.CatalogListResponse{Catalogs: catalogList})
}

// GetCatalog get method for catalog configmap
func (s *CatalogService) GetCatalog(c echo.Context) error {
	catalogName := c.Param("catalogName")

	var cm v1.ConfigMap
	err := s.k8sClient.Get(context.Background(), client.ObjectKey{Namespace: DefaultVelaNamespace, Name: catalogName}, &cm)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("get config for %s failed %s", catalogName, err.Error()))
	}
	UpdatedInt, err := strconv.ParseInt(cm.Data["UpdatedAt"], 10, 64)
	if err != nil {
		return fmt.Errorf("unable to resolve update parameter in %s:%s ", catalogName, err.Error())
	}
	var catalog = model.Catalog{
		Name:      catalogName,
		Desc:      cm.Data["Desc"],
		UpdatedAt: UpdatedInt,
		Type:      cm.Data["Type"],
		Url:       cm.Data["Url"],
		Token:     cm.Data["Token"],
	}
	return c.JSON(http.StatusOK, model.CatalogResponse{Catalog: &catalog})
}

// AddCatalog add method for catalog configmap
func (s *CatalogService) AddCatalog(c echo.Context) error {
	catalogReq := new(apis.CatalogRequest)
	if err := c.Bind(catalogReq); err != nil {
		return err
	}
	exist, err := s.checkCatalogExist(catalogReq.Name)
	if err != nil {
		return err
	}
	if exist {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("catalog %s exist", catalogReq.Name))
	}

	var cm *v1.ConfigMap
	configdata := map[string]string{
		"Name":      catalogReq.Name,
		"Desc":      catalogReq.Desc,
		"UpdatedAt": time.Now().String(),
	}

	label := map[string]string{
		"catalog": "configdata",
	}
	cm, err = ToConfigMap(catalogReq.Name, DefaultUINamespace, label, configdata)
	if err != nil {
		return fmt.Errorf("convert config map failed %s ", err.Error())
	}
	err = s.k8sClient.Create(context.Background(), cm)
	if err != nil {
		return fmt.Errorf("unable to create configmap for %s : %s ", catalogReq.Name, err.Error())
	}
	catalog := convertToCatalog(catalogReq)
	return c.JSON(http.StatusCreated, apis.CatalogMeta{Catalog: &catalog})
}

// UpdateCatalog update method for catalog configmap
func (s *CatalogService) UpdateCatalog(c echo.Context) error {
	catalogReq := new(apis.CatalogRequest)
	if err := c.Bind(catalogReq); err != nil {
		return err
	}
	catalog := convertToCatalog(catalogReq)
	var cm *v1.ConfigMap
	configdata := map[string]string{
		"Name":      catalogReq.Name,
		"Desc":      catalogReq.Desc,
		"UpdatedAt": time.Now().String(),
	}

	label := map[string]string{
		"catalog": "configdata",
	}
	cm, err := ToConfigMap(catalogReq.Name, DefaultUINamespace, label, configdata)
	if err != nil {
		return fmt.Errorf("convert config map failed %s ", err.Error())
	}
	err = s.k8sClient.Update(context.Background(), cm)
	if err != nil {
		return fmt.Errorf("unable to update configmap for %s : %s ", catalogReq.Name, err.Error())
	}

	return c.JSON(http.StatusOK, apis.CatalogMeta{Catalog: &catalog})
}

// DelCatalog delete method for catalog configmap
func (s *CatalogService) DelCatalog(c echo.Context) error {
	catalogName := c.Param("catalogName")

	var cm v1.ConfigMap
	cm.SetName(catalogName)
	cm.SetNamespace(DefaultUINamespace)
	if err := s.k8sClient.Delete(context.Background(), &cm); err != nil {
		return c.JSON(http.StatusInternalServerError, false)
	}

	return c.JSON(http.StatusOK, true)
}

// checkCatalogExist check whether catalog exist with name
func (s *CatalogService) checkCatalogExist(catalogName string) (bool, error) {
	var cm v1.ConfigMap
	err := s.k8sClient.Get(context.Background(), client.ObjectKey{Namespace: DefaultUINamespace, Name: catalogName}, &cm)
	if err != nil {
		if apierrors.IsNotFound(err) { // not found
			return false, nil
		}
		// other error
		return false, err
	}
	// found
	return true, nil
}

// convertToCatalog get catalog model from request
func convertToCatalog(catalogReq *apis.CatalogRequest) model.Catalog {
	return model.Catalog{
		Name:      catalogReq.Name,
		Desc:      catalogReq.Desc,
		UpdatedAt: time.Now().Unix(),
		Type:      catalogReq.Type,
		Url:       catalogReq.URL,
		Token:     catalogReq.Token,
	}
}
