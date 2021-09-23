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

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	restful "github.com/emicklei/go-restful/v3"
	"github.com/go-openapi/spec"

	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/webservice"
)

var _ APIServer = &restServer{}

// Config config for server
type Config struct {
	// openapi server listen port
	Port int
	// openapi server bind host
	BindHost string
	// monitor metric path
	MetricPath string
}

// APIServer interface for call api server
type APIServer interface {
	Run(context.Context) error
}

type restServer struct {
	webContainer *restful.Container
	cfg          Config
}

// New create restserver with config data
func New(cfg Config) (APIServer, error) {
	s := &restServer{
		webContainer: restful.NewContainer(),
		cfg:          cfg,
	}
	return s, nil
}

func (s *restServer) Run(ctx context.Context) error {
	webservice.Init(ctx)
	err := s.registerServices()
	if err != nil {
		return err
	}
	return s.startHTTP(ctx)
}

func (s *restServer) registerServices() error {

	/* **************************************************************  */
	/* *************       Open API Route Group     *****************  */
	/* **************************************************************  */

	// Add container filter to enable CORS
	cors := restful.CrossOriginResourceSharing{
		ExposeHeaders:  []string{"X-My-Header"},
		AllowedHeaders: []string{"Content-Type", "Accept"},
		AllowedMethods: []string{"GET", "POST"},
		CookiesAllowed: false,
		Container:      s.webContainer}
	s.webContainer.Filter(cors.Filter)

	// Add container filter to respond to OPTIONS
	s.webContainer.Filter(s.webContainer.OPTIONSFilter)

	// Regist all custom webservice
	for _, handler := range webservice.GetRegistedWebService() {
		s.webContainer.Add(handler.GetWebService())
	}

	config := restfulspec.Config{
		WebServices:                   s.webContainer.RegisteredWebServices(), // you control what services are visible
		APIPath:                       "/apidocs.json",
		PostBuildSwaggerObjectHandler: enrichSwaggerObject}
	s.webContainer.Add(restfulspec.NewOpenAPIService(config))
	return nil
}

func enrichSwaggerObject(swo *spec.Swagger) {
	swo.Info = &spec.Info{
		InfoProps: spec.InfoProps{
			Title:       "Kubevela openapi doc",
			Description: "Kubevela openapi doc",
			Contact: &spec.ContactInfo{
				ContactInfoProps: spec.ContactInfoProps{
					Name:  "kubevela",
					Email: "zengqg@yiyun.pro",
					URL:   "https://kubevela.io/",
				},
			},
			License: &spec.License{
				LicenseProps: spec.LicenseProps{
					Name: "Apache License 2.0",
					URL:  "https://github.com/oam-dev/kubevela/blob/master/LICENSE",
				},
			},
			Version: "v1alpha2",
		},
	}
}

func (s *restServer) startHTTP(ctx context.Context) error {
	// Start HTTP apiserver
	log.Logger.Infof("HTTP APIs are being served on: %s:%d, ctx: %s", s.cfg.BindHost, s.cfg.Port, ctx)
	addr := fmt.Sprintf("%s:%d", s.cfg.BindHost, s.cfg.Port)
	server := &http.Server{Addr: addr, Handler: s.webContainer}
	return server.ListenAndServe()
}
