package services

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/duration"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/kubectl/pkg/util/event"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/apply"

	"github.com/oam-dev/kubevela/pkg/apiserver/proto/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/runtime"
)

// ApplicationService application service
type ApplicationService struct {
	k8sClient client.Client
}

const (
	// DefaultUINamespace default namespace for configmap info management in velaux system
	DefaultUINamespace = "velaux-system"
	// DefaultAppNamespace default namespace for application
	DefaultAppNamespace = "default"
	// DefaultVelaNamespace default namespace for vela system
	DefaultVelaNamespace = "vela-system"
)

// NewApplicationService new application service
func NewApplicationService(client client.Client) *ApplicationService {

	return &ApplicationService{
		k8sClient: client,
	}
}

// GetApplications get applications only with configmap info from cluster
func (s *ApplicationService) GetApplications(c echo.Context) error {
	// appName := c.QueryParam("appName") // change to get application

	var cmList v1.ConfigMapList
	labels := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": "configdata",
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
	var appList = make([]*model.Application, 0, len(cmList.Items))
	for i, c := range cmList.Items {
		UpdateInt, err := strconv.ParseInt(cmList.Items[i].Data["UpdatedAt"], 10, 64)
		if err != nil {
			return err
		}
		app := model.Application{
			Name:      c.Name,
			Namespace: c.Namespace,
			Desc:      cmList.Items[i].Data["Desc"],
			UpdatedAt: UpdateInt,
		}
		appList = append(appList, &app)
	}

	return c.JSON(http.StatusOK, model.ApplicationListResponse{
		Applications: appList,
	})
}

// GetApplicationDetail get application detail
func (s *ApplicationService) GetApplicationDetail(c echo.Context) error {
	appName := c.Param("application")
	clusterName := c.Param("cluster")

	cli, err := s.getClientByClusterName(clusterName)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("get client info failed: %s ", err.Error()))
	}

	// get application configmap info
	var cm v1.ConfigMap
	err = s.k8sClient.Get(context.Background(), client.ObjectKey{Namespace: DefaultUINamespace, Name: appName}, &cm)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("client get configmap for %s failed :%s ", appName, err.Error()))
	}

	var app model.Application

	app.Name = appName
	app.Desc = cm.Data["Desc"]
	app.Namespace = cm.Data["Namespace"]
	app.ClusterName = clusterName
	app.UpdatedAt, err = strconv.ParseInt(cm.Data["UpdatedAt"], 10, 64)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("unable to resolve update parameter in %s:%s ", clusterName, err.Error()))
	}

	var appObj = v1beta1.Application{}
	if err := cli.Get(context.Background(), client.ObjectKey{Namespace: DefaultAppNamespace, Name: appName}, &appObj); err != nil { // application crd info
		return err
	}

	for i, c := range appObj.Status.Services {
		comp := model.ComponentType{
			Name:      c.Name,
			Namespace: DefaultAppNamespace,
			Workload:  c.WorkloadDefinition.Kind,
			Type:      appObj.Spec.Components[i].Type,
			Health:    c.Healthy,
			Phase:     string(appObj.Status.Phase),
		}
		app.Components = append(app.Components, &comp)
	}

	el := v1.EventList{}
	fieldStr := fmt.Sprintf("involvedObject.kind=Application,involvedObject.name=%s,,involvedObject.namespace=%s", appName, app.Namespace)
	if err := cli.List(context.Background(), &el, &client.ListOptions{
		Namespace: DefaultAppNamespace,
		Raw:       &metav1.ListOptions{FieldSelector: fieldStr},
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("client get event failed :%s ", err.Error()))
	}
	// sort event
	sort.Sort(event.SortableEvents(el.Items))
	for _, e := range el.Items {
		var age string
		if e.Count > 1 {
			age = fmt.Sprintf("%s (x%d over %s)", translateTimestampSince(e.LastTimestamp), e.Count, translateTimestampSince(e.FirstTimestamp))
		} else {
			age = translateTimestampSince(e.FirstTimestamp)
			if e.FirstTimestamp.IsZero() {
				age = translateMicroTimestampSince(e.EventTime)
			}
		}
		app.Events = append(app.Events, &model.AppEventType{
			Type:    e.Type,
			Age:     age,
			Reason:  e.Reason,
			Message: e.Message,
		})
	}

	return c.JSON(http.StatusOK, model.ApplicationResponse{
		Application: &app,
	})
}

// AddApplications for add applications to cluster
func (s *ApplicationService) AddApplications(c echo.Context) error {
	clusterName := c.Param("cluster")
	app := new(model.Application)
	if err := c.Bind(app); err != nil {
		return c.JSON(http.StatusBadRequest, fmt.Sprintf("resolve request failed %s ", err.Error()))
	}
	app.ClusterName = clusterName
	isAppExist, err := s.checkAppExist(app.Name)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("check app existed failed :%s ", err.Error()))
	}
	if isAppExist {
		return c.JSON(http.StatusBadRequest, fmt.Sprintf("application %s has existed: %s ", app.Name, err.Error()))
	}

	cli, err := s.getClientByClusterName(clusterName)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("get client failed: %s ", err.Error()))
	}

	expectApp, err := runtime.ParseCoreApplication(app)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("parse app failed: %s ", err.Error()))
	}
	if err := cli.Create(context.Background(), &expectApp); err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("create app failed: %s ", err.Error()))
	}

	var cm *v1.ConfigMap
	configdata := map[string]string{
		"Name":        app.Name,
		"Desc":        app.Desc,
		"Namespace":   app.Namespace,
		"UpdatedAt":   time.Now().String(),
		"ClusterName": clusterName,
	}

	label := map[string]string{
		"app": "configdata",
	}
	cm, err = ToConfigMap(app.Name, DefaultUINamespace, label, configdata)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("convert config map failed %s ", err.Error()))
	}
	err = s.k8sClient.Create(context.Background(), cm)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("create configmap for %s failed: %s ", app.Name, err.Error()))
	}

	return c.JSON(http.StatusCreated, model.ApplicationResponse{
		Application: app,
	})
}

// AddApplicationYaml for add applications with yaml to cluster
func (s *ApplicationService) AddApplicationYaml(c echo.Context) error {
	clusterName := c.Param("cluster")
	appYaml := new(model.AppYaml)
	if err := c.Bind(appYaml); err != nil {
		return c.JSON(http.StatusBadRequest, fmt.Sprintf("resolve request failed %s ", err.Error()))
	}
	cli, err := s.getClientByClusterName(clusterName)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("get client failed: %s ", err.Error()))
	}

	decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(appYaml.Yaml)), 100)
	var appObj v1beta1.Application
	if err = decoder.Decode(&appObj); err != nil {
		return c.JSON(http.StatusBadRequest, fmt.Sprintf("decode for app yaml failed: %s ", err.Error()))
	}
	if appObj.Namespace == "" {
		appObj.Namespace = DefaultAppNamespace
	}

	if err := cli.Create(context.Background(), &appObj); err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("client app failed: %s ", err.Error()))
	}
	app, err := runtime.ParseApplicationYaml(&appObj)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("client parse app failed: %s ", err.Error()))
	}

	var cm *v1.ConfigMap
	configdata := map[string]string{
		"Name":        app.Name,
		"Desc":        app.Desc,
		"UpdatedAt":   time.Now().String(),
		"ClusterName": clusterName,
	}

	label := map[string]string{
		"app": "configdata",
	}
	cm, err = ToConfigMap(app.Name, DefaultUINamespace, label, configdata)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("convert config map failed %s ", err.Error()))
	}
	err = s.k8sClient.Create(context.Background(), cm)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("unable to create configmap for %s : %s ", app.Name, err.Error()))
	}

	return c.JSON(http.StatusOK, model.ApplicationResponse{
		Application: app,
	})
}

// UpdateApplications for update application
func (s *ApplicationService) UpdateApplications(c echo.Context) error {
	clusterName := c.Param("cluster")
	app := new(model.Application)
	if err := c.Bind(app); err != nil {
		return c.JSON(http.StatusBadRequest, fmt.Sprintf("resolve request failed %s ", err.Error()))
	}
	app.ClusterName = clusterName

	isAppExist, err := s.checkAppExist(app.Name)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("check app existed failed :%s ", err.Error()))
	}
	if !isAppExist {
		return c.JSON(http.StatusBadRequest, fmt.Sprintf("application %s do not existed: %s ", app.Name, err.Error()))
	}

	cli, err := s.getClientByClusterName(clusterName)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("get client failed :%s ", err.Error()))
	}

	expectApp, err := runtime.ParseCoreApplication(app)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("client parse app failed :%s ", err.Error()))
	}
	expectAppObj, err := k8sruntime.DefaultUnstructuredConverter.ToUnstructured(&expectApp)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("convert app to unstructrue failed :%s ", err.Error()))
	}

	expectAppUnStruct := &unstructured.Unstructured{Object: expectAppObj}
	expectAppUnStruct.SetGroupVersionKind(v1beta1.ApplicationKindVersionKind)

	applicator := apply.NewAPIApplicator(cli)
	if err := applicator.Apply(context.Background(), expectAppUnStruct); err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("apply app failed :%s ", err.Error()))
	}

	var cm *v1.ConfigMap
	configdata := map[string]string{
		"Name":        app.Name,
		"Desc":        app.Desc,
		"UpdatedAt":   time.Now().String(),
		"ClusterName": clusterName,
	}

	label := map[string]string{
		"app": "configdata",
	}
	cm, err = ToConfigMap(app.Name, DefaultUINamespace, label, configdata)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("convert config map failed %s ", err.Error()))
	}

	err = s.k8sClient.Create(context.Background(), cm)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("unable to create configmap for %s : %s ", app.Name, err.Error()))
	}

	return c.JSON(http.StatusOK, model.ApplicationResponse{
		Application: app,
	})
}

// RemoveApplications for remove application from cluster
func (s *ApplicationService) RemoveApplications(c echo.Context) error {
	appName := c.Param("application")
	clusterName := c.Param("cluster")
	// get namespace for application
	var cm v1.ConfigMap
	err := s.k8sClient.Get(context.Background(), client.ObjectKey{Namespace: DefaultUINamespace, Name: appName}, &cm)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("client get configmap failed: %s ", err.Error()))
	}

	cli, err := s.getClientByClusterName(clusterName)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("get client by name failed: %s ", err.Error()))
	}

	application := &model.Application{Name: appName, Namespace: cm.Data["Namespace"]}
	expectApp, err := runtime.ParseCoreApplication(application)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("parse app failed: %s ", err.Error()))
	}
	if err := cli.Delete(context.Background(), &expectApp); err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("delete app failed: %s ", err.Error()))
	}

	// delete configmap for app info
	cm.SetName(appName)
	cm.SetNamespace(DefaultUINamespace)
	if err := s.k8sClient.Delete(context.Background(), &cm); err != nil {
		return c.JSON(http.StatusInternalServerError, false)
	}

	return c.JSON(http.StatusOK, model.ApplicationResponse{
		Application: &model.Application{Name: appName},
	})
}

func translateTimestampSince(timestamp metav1.Time) string {
	if timestamp.IsZero() {
		return "<unknown>"
	}

	return duration.HumanDuration(time.Since(timestamp.Time))
}

func translateMicroTimestampSince(timestamp metav1.MicroTime) string {
	if timestamp.IsZero() {
		return "<unknown>"
	}

	return duration.HumanDuration(time.Since(timestamp.Time))
}

// checkAppExist check whether app is existed
func (s *ApplicationService) checkAppExist(appName string) (bool, error) {
	var cm v1.ConfigMap
	err := s.k8sClient.Get(context.Background(), client.ObjectKey{Namespace: DefaultUINamespace, Name: appName}, &cm)
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

// getClientByClusterName get client by cluster name
func (s *ApplicationService) getClientByClusterName(clusterName string) (client.Client, error) {
	var cm v1.ConfigMap
	// k8sClient is a common client for getting configmap info in current cluster.
	err := s.k8sClient.Get(context.Background(), client.ObjectKey{Namespace: DefaultUINamespace, Name: clusterName}, &cm) // cluster configmap info
	if err != nil {
		return nil, fmt.Errorf("unable to find configmap parameters in %s:%w ", clusterName, err)
	}

	// cli is the client running in specific cluster to get specific k8s cr resource.
	cli, err := runtime.GetClient([]byte(cm.Data["Kubeconfig"]))
	if err != nil {
		return nil, err
	}
	return cli, nil
}

// ToConfigMap convert map value to configmap format
func ToConfigMap(name, namespace string, label map[string]string, configData map[string]string) (*v1.ConfigMap, error) {
	var cm = v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
	}
	cm.SetName(name)
	cm.SetNamespace(namespace)
	cm.SetLabels(label)
	cm.Data = configData
	return &cm, nil
}
