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

package rest

import (
	"context"
	"fmt"
	"net/http"

	echo "github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/services"
	"github.com/oam-dev/kubevela/pkg/utils/k8sutil"
)

var _ APIServer = &restServer{}

// Config config for server
type Config struct {
	Port int
}

// APIServer interface for call api server
type APIServer interface {
	Run(context.Context) error
}

type restServer struct {
	server    *echo.Echo
	k8sClient client.Client
	cfg       Config
}

const (
	// DefaultUINamespace default namespace for configmap info management in velaux system
	DefaultUINamespace = "velaux-system"
)

// New create restserver with config data
func New(cfg Config) (APIServer, error) {
	client, err := k8sutil.NewK8sClient()
	if err != nil {
		return nil, fmt.Errorf("create client for clusterService failed")
	}
	s := &restServer{
		server:    newEchoInstance(),
		k8sClient: client,
		cfg:       cfg,
	}

	return s, nil
}

func newEchoInstance() *echo.Echo {
	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.Gzip())
	e.Use(middleware.Logger())
	e.Pre(middleware.RemoveTrailingSlash())

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodPost, http.MethodDelete},
		AllowCredentials: true,
		MaxAge:           86400,
	}))

	return e
}

func (s *restServer) Run(ctx context.Context) error {
	err := s.registerServices()
	if err != nil {
		return err
	}
	return s.startHTTP(ctx)
}

func (s *restServer) registerServices() error {

	// create specific namespace for better resource management
	var ns v1.Namespace
	if err := s.k8sClient.Get(context.Background(), types.NamespacedName{Name: DefaultUINamespace}, &ns); err != nil && apierrors.IsNotFound(err) {
		// not found
		ns = v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: DefaultUINamespace,
			},
		}
		err := s.k8sClient.Create(context.Background(), &ns)
		if err != nil {
			log.Logger.Errorf("create namespace for velaux system failed %s ", err.Error())
			return err
		}
	}

	// catalog
	catalogService := services.NewCatalogService(s.k8sClient)
	s.server.GET("/catalogs", catalogService.ListCatalogs)
	s.server.POST("/catalogs", catalogService.AddCatalog)
	s.server.PUT("/catalogs", catalogService.UpdateCatalog)
	s.server.GET("/catalogs/:catalogName", catalogService.GetCatalog)
	s.server.DELETE("/catalogs/:catalogName", catalogService.DelCatalog)

	return nil
}

func (s *restServer) startHTTP(ctx context.Context) error {
	// Start HTTP apiserver
	log.Logger.Infof("HTTP APIs are being served on port: %d, ctx: %s", s.cfg.Port, ctx)
	addr := fmt.Sprintf(":%d", s.cfg.Port)
	return s.server.Start(addr)
}
