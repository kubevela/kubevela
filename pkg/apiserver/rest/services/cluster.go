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

// ClusterService cluster service
type ClusterService struct {
	k8sClient client.Client
}

// NewClusterService new cluster service
func NewClusterService(client client.Client) *ClusterService {

	return &ClusterService{
		k8sClient: client,
	}
}

// GetClusterNames list method for all cluster names
func (s *ClusterService) GetClusterNames(c echo.Context) error {
	var cmList v1.ConfigMapList
	labels := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"cluster": "configdata",
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

	names := []string{}
	for i := range cmList.Items {
		names = append(names, cmList.Items[i].Name)
	}

	return c.JSON(http.StatusOK, apis.ClustersMeta{Clusters: names})
}

// ListClusters list method for all cluster
func (s *ClusterService) ListClusters(c echo.Context) error {

	var cmList v1.ConfigMapList
	labels := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"cluster": "configdata",
		},
	}
	selector, err := metav1.LabelSelectorAsSelector(labels)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("create label selector for configmap failed : %s", err.Error()))
	}
	err = s.k8sClient.List(context.Background(), &cmList, &client.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("list configmap for cluster info failed : %s", err.Error()))
	}

	var clusterList = make([]*model.Cluster, 0, len(cmList.Items))
	for i, c := range cmList.Items {
		UpdateInt, err := strconv.ParseInt(cmList.Items[i].Data["UpdatedAt"], 10, 64)
		if err != nil {
			return err
		}
		cluster := model.Cluster{
			Name:      c.Name,
			UpdatedAt: UpdateInt,
			Desc:      cmList.Items[i].Data["Desc"],
		}
		clusterList = append(clusterList, &cluster)
	}

	return c.JSON(http.StatusOK, model.ClusterListResponse{Clusters: clusterList})
}

// GetCluster get method for cluster
func (s *ClusterService) GetCluster(c echo.Context) error {
	clusterName := c.QueryParam("clusterName")

	var cm v1.ConfigMap
	err := s.k8sClient.Get(context.Background(), client.ObjectKey{Namespace: DefaultUINamespace, Name: clusterName}, &cm)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("client get configmap for %s failed :%s ", clusterName, err.Error()))
	}
	var cluster model.Cluster
	cluster.Name = cm.Data["Name"]
	cluster.Desc = cm.Data["Desc"]
	cluster.Kubeconfig = cm.Data["Kubeconfig"]
	cluster.UpdatedAt, err = strconv.ParseInt(cm.Data["UpdatedAt"], 10, 64)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("unable to resolve update parameter in %s:%s ", clusterName, err.Error()))
	}

	return c.JSON(http.StatusOK, model.ClusterResponse{Cluster: &cluster})
}

// AddCluster add method for cluster
func (s *ClusterService) AddCluster(c echo.Context) error {
	clusterReq := new(apis.ClusterRequest)
	if err := c.Bind(clusterReq); err != nil {
		return c.JSON(http.StatusBadRequest, fmt.Sprintf("resolve request failed %s ", err.Error()))
	}
	var cm v1.ConfigMap
	err := s.k8sClient.Get(context.Background(), client.ObjectKey{Namespace: DefaultUINamespace, Name: clusterReq.Name}, &cm)
	if err != nil && apierrors.IsNotFound(err) {
		// not found
		if err != nil {
			return c.JSON(http.StatusInternalServerError, fmt.Sprintf("get cluster config failed: %s ", err.Error()))
		}
		var cm *v1.ConfigMap
		configdata := map[string]string{
			"Name":      clusterReq.Name,
			"Desc":      clusterReq.Desc,
			"UpdatedAt": time.Now().String(),
			"Kubecofig": clusterReq.Kubeconfig,
		}
		label := map[string]string{
			"cluster": "configdata",
		}
		cm, err = ToConfigMap(clusterReq.Name, DefaultUINamespace, label, configdata)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, fmt.Sprintf("convert config map failed %s ", err.Error()))
		}
		err = s.k8sClient.Create(context.Background(), cm)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, fmt.Sprintf("unable to create configmap for %s : %s ", clusterReq.Name, err.Error()))
		}
	} else {
		// found
		return c.JSON(http.StatusBadRequest, fmt.Sprintf("cluster %s has exist", clusterReq.Name))
	}
	cluster := convertToCluster(clusterReq)
	return c.JSON(http.StatusCreated, apis.ClusterMeta{Cluster: &cluster})
}

// UpdateCluster update method for cluster
func (s *ClusterService) UpdateCluster(c echo.Context) error {
	clusterReq := new(apis.ClusterRequest)
	if err := c.Bind(clusterReq); err != nil {
		return c.JSON(http.StatusBadRequest, fmt.Sprintf("resolve request failed %s ", err.Error()))
	}
	cluster := convertToCluster(clusterReq)
	var cm *v1.ConfigMap
	configdata := map[string]string{
		"Name":      clusterReq.Name,
		"Desc":      clusterReq.Desc,
		"UpdatedAt": time.Now().String(),
		"Kubecofig": clusterReq.Kubeconfig,
	}

	label := map[string]string{
		"cluster": "configdata",
	}
	cm, err := ToConfigMap(clusterReq.Name, DefaultUINamespace, label, configdata)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("convert config map failed %s ", err.Error()))
	}
	err = s.k8sClient.Update(context.Background(), cm)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("unable to update configmap for %s : %s ", clusterReq.Name, err.Error()))
	}
	return c.JSON(http.StatusOK, apis.ClusterMeta{Cluster: &cluster})
}

// DelCluster delete method for cluster
func (s *ClusterService) DelCluster(c echo.Context) error {
	clusterName := c.Param("clusterName")
	var cm v1.ConfigMap
	cm.SetName(clusterName)
	cm.SetNamespace(DefaultUINamespace)
	if err := s.k8sClient.Delete(context.Background(), &cm); err != nil {
		return c.JSON(http.StatusInternalServerError, false)
	}
	return c.JSON(http.StatusOK, true)
}

// checkClusterExist check whether cluster exist with name
func (s *ClusterService) checkClusterExist(clusterName string) (bool, error) {
	var cm v1.ConfigMap
	err := s.k8sClient.Get(context.Background(), client.ObjectKey{Namespace: DefaultUINamespace, Name: clusterName}, &cm)
	if err != nil && apierrors.IsNotFound(err) { // not found
		return false, err
	}
	// found
	return true, nil
}

// convertToCluster get cluster model from request
func convertToCluster(clusterReq *apis.ClusterRequest) model.Cluster {
	return model.Cluster{
		Name:       clusterReq.Name,
		Desc:       clusterReq.Desc,
		UpdatedAt:  time.Now().Unix(),
		Kubeconfig: clusterReq.Kubeconfig,
	}
}
