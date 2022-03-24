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
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"github.com/go-openapi/spec"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore/kubeapi"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore/mongodb"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/webservice"
	velasync "github.com/oam-dev/kubevela/pkg/apiserver/sync"
	utils2 "github.com/oam-dev/kubevela/pkg/utils"
)

var _ APIServer = &restServer{}

// Config config for server
type Config struct {
	// api server bind address
	BindAddr string
	// monitor metric path
	MetricPath string

	// Datastore config
	Datastore datastore.Config

	// LeaderConfig for leader election
	LeaderConfig leaderConfig

	// AddonCacheTime is how long between two cache operations
	AddonCacheTime time.Duration
}

type leaderConfig struct {
	ID       string
	LockName string
	Duration time.Duration
}

// APIServer interface for call api server
type APIServer interface {
	Run(context.Context) error
	RegisterServices() restfulspec.Config
}

type restServer struct {
	webContainer *restful.Container
	cfg          Config
	dataStore    datastore.DataStore
}

// New create restserver with config data
func New(cfg Config) (a APIServer, err error) {
	var ds datastore.DataStore
	switch cfg.Datastore.Type {
	case "mongodb":
		ds, err = mongodb.New(context.Background(), cfg.Datastore)
		if err != nil {
			return nil, fmt.Errorf("create mongodb datastore instance failure %w", err)
		}
	case "kubeapi":
		ds, err = kubeapi.New(context.Background(), cfg.Datastore)
		if err != nil {
			return nil, fmt.Errorf("create kubeapi datastore instance failure %w", err)
		}
	default:
		return nil, fmt.Errorf("not support datastore type %s", cfg.Datastore.Type)
	}

	s := &restServer{
		webContainer: restful.NewContainer(),
		cfg:          cfg,
		dataStore:    ds,
	}
	return s, nil
}

func (s *restServer) Run(ctx context.Context) error {
	s.RegisterServices()

	if err := s.InitAdmin(ctx); err != nil {
		return err
	}

	l, err := s.setupLeaderElection()
	if err != nil {
		return err
	}

	go func() {
		leaderelection.RunOrDie(ctx, *l)
	}()
	return s.startHTTP(ctx)
}

func (s *restServer) setupLeaderElection() (*leaderelection.LeaderElectionConfig, error) {
	restCfg := ctrl.GetConfigOrDie()

	rl, err := resourcelock.NewFromKubeconfig(resourcelock.LeasesResourceLock, types.DefaultKubeVelaNS, s.cfg.LeaderConfig.LockName, resourcelock.ResourceLockConfig{
		Identity: s.cfg.LeaderConfig.ID,
	}, restCfg, time.Second*10)
	if err != nil {
		klog.ErrorS(err, "Unable to setup the resource lock")
		return nil, err
	}

	return &leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: time.Second * 15,
		RenewDeadline: time.Second * 10,
		RetryPeriod:   time.Second * 2,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				go velasync.Start(ctx, s.dataStore, restCfg)
				s.runWorkflowRecordSync(ctx, s.cfg.LeaderConfig.Duration)
			},
			OnStoppedLeading: func() {
				klog.Infof("leader lost: %s", s.cfg.LeaderConfig.ID)
				// Currently, the started goroutine will all closed by the context, so there seems no need to call os.Exit here.
				// But it can be safe to stop the process as leader lost.
				os.Exit(0)
			},
			OnNewLeader: func(identity string) {
				if identity == s.cfg.LeaderConfig.ID {
					return
				}
				klog.Infof("new leader elected: %s", identity)
			},
		},
		ReleaseOnCancel: true,
	}, nil
}

func (s restServer) runWorkflowRecordSync(ctx context.Context, duration time.Duration) {
	klog.Infof("start to syncing workflow record")
	w := usecase.NewWorkflowUsecase(s.dataStore, usecase.NewEnvUsecase(s.dataStore))

	t := time.NewTicker(duration)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			if err := w.SyncWorkflowRecord(ctx); err != nil {
				klog.ErrorS(err, "syncWorkflowRecordError")
			}
		case <-ctx.Done():
			return
		}
	}
}

// RegisterServices register web service
func (s *restServer) RegisterServices() restfulspec.Config {
	webservice.Init(s.dataStore, s.cfg.AddonCacheTime)
	/* **************************************************************  */
	/* *************       Open API Route Group     *****************  */
	/* **************************************************************  */

	// Add container filter to enable CORS
	cors := restful.CrossOriginResourceSharing{
		ExposeHeaders:  []string{},
		AllowedHeaders: []string{"Content-Type", "Accept", "Authorization", "RefreshToken"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		CookiesAllowed: true,
		Container:      s.webContainer}
	s.webContainer.Filter(cors.Filter)

	// Add container filter to respond to OPTIONS
	s.webContainer.Filter(s.webContainer.OPTIONSFilter)

	// Add request log
	s.webContainer.Filter(s.requestLog)

	// Register all custom webservice
	for _, handler := range webservice.GetRegisteredWebService() {
		s.webContainer.Add(handler.GetWebService())
	}

	config := restfulspec.Config{
		WebServices:                   s.webContainer.RegisteredWebServices(), // you control what services are visible
		APIPath:                       "/apidocs.json",
		PostBuildSwaggerObjectHandler: enrichSwaggerObject}
	s.webContainer.Add(restfulspec.NewOpenAPIService(config))
	return config
}

// RegisterServices register web service
func (s *restServer) InitAdmin(ctx context.Context) error {
	admin := "admin"
	if err := s.dataStore.Get(ctx, &model.User{
		Name: admin,
	}); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			pwd := utils2.RandomString(8)
			encrypted, err := usecase.GeneratePasswordHash(pwd)
			if err != nil {
				return err
			}
			if err := s.dataStore.Add(ctx, &model.User{
				Name:      admin,
				Password:  encrypted,
				UserRoles: []string{"admin"},
			}); err != nil {
				return err
			}
			kubecli, err := clients.GetKubeClient()
			if err != nil {
				log.Logger.Fatalf("failed to get kube client: %s", err.Error())
				return err
			}
			secret := &corev1.Secret{}
			if err := kubecli.Get(ctx, k8stypes.NamespacedName{
				Name:      admin,
				Namespace: types.DefaultKubeVelaNS,
			}, secret); err != nil {
				if apierrors.IsNotFound(err) {
					if err := kubecli.Create(ctx, &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      admin,
							Namespace: types.DefaultKubeVelaNS,
						},
						StringData: map[string]string{
							admin: pwd,
						},
					}); err != nil {
						return err
					}
				} else {
					return err
				}
			}
		} else {
			return err
		}
	}
	return nil
}

func (s *restServer) requestLog(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
	start := time.Now()
	c := utils.NewResponseCapture(resp.ResponseWriter)
	resp.ResponseWriter = c
	chain.ProcessFilter(req, resp)
	takeTime := time.Since(start)
	log.Logger.With(
		"clientIP", utils2.Sanitize(utils.ClientIP(req.Request)),
		"path", utils2.Sanitize(req.Request.URL.Path),
		"method", req.Request.Method,
		"status", c.StatusCode(),
		"time", takeTime.String(),
		"responseSize", len(c.Bytes()),
	).Infof("request log")
}

func enrichSwaggerObject(swo *spec.Swagger) {
	swo.Info = &spec.Info{
		InfoProps: spec.InfoProps{
			Title:       "Kubevela api doc",
			Description: "Kubevela api doc",
			Contact: &spec.ContactInfo{
				ContactInfoProps: spec.ContactInfoProps{
					Name:  "kubevela",
					Email: "feedback@mail.kubevela.io",
					URL:   "https://kubevela.io/",
				},
			},
			License: &spec.License{
				LicenseProps: spec.LicenseProps{
					Name: "Apache License 2.0",
					URL:  "https://github.com/oam-dev/kubevela/blob/master/LICENSE",
				},
			},
			Version: "v1beta1",
		},
	}
}

func (s *restServer) startHTTP(ctx context.Context) error {
	// Start HTTP apiserver
	log.Logger.Infof("HTTP APIs are being served on: %s, ctx: %s", s.cfg.BindAddr, ctx)
	server := &http.Server{Addr: s.cfg.BindAddr, Handler: s.webContainer}
	return server.ListenAndServe()
}
