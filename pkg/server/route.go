package server

import (
	"fmt"
	"net/http"
	"os"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"

	// swagger json data
	_ "github.com/oam-dev/kubevela/pkg/server/docs"
	"github.com/oam-dev/kubevela/pkg/server/util"
)

// setup the gin http server handler
// @title KubeVela API
// @version 0.0.1
// @description An KubeVela API.

// @BasePath /api
func (s *APIServer) setupRoute(staticPath string) http.Handler {
	// if deploying static Dashboard, set the mode to `release`, or to `debug`
	if staticPath != "" {
		gin.SetMode(gin.ReleaseMode)
	}
	// create the router
	router := gin.New()
	loggerConfig := gin.LoggerConfig{
		Output: os.Stdout,
		Formatter: func(param gin.LogFormatterParams) string {
			return fmt.Sprintf("%v | %3d | %13v | %15s | %-7s %s | %s\n",
				param.TimeStamp.Format("2006/01/02 - 15:04:05"),
				param.StatusCode,
				param.Latency,
				param.ClientIP,
				param.Method,
				param.Path,
				param.ErrorMessage,
			)
		},
	}

	if staticPath != "" {
		router.Use(static.Serve("/", static.LocalFile(staticPath, false)))
	}

	router.Use(gin.LoggerWithConfig(loggerConfig))
	router.Use(util.SetRequestID())
	router.Use(util.SetContext())
	router.Use(gin.Recovery())
	router.Use(util.ValidateHeaders())
	// all requests start with /api
	api := router.Group(util.RootPath)
	// env related operation
	envs := api.Group(util.EnvironmentPath)
	{
		envs.POST("/", s.CreateEnv)
		envs.PUT("/:envName", s.UpdateEnv)
		envs.GET("/:envName", s.GetEnv)
		envs.GET("/", s.ListEnv)
		// Allow levaing out `/` to make API more friendly
		envs.GET("", s.ListEnv)
		envs.DELETE("/:envName", s.DeleteEnv)
		envs.PATCH("/:envName", s.SetEnv)
		// app related operation
		apps := envs.Group("/:envName/apps")
		{
			apps.GET("/:appName", s.GetApp)
			apps.PUT("/:appName", s.UpdateApps)
			apps.GET("/", s.ListApps)
			apps.GET("", s.ListApps)
			apps.DELETE("/:appName", s.DeleteApps)

			// component related operation
			components := apps.Group("/:appName/components")
			{
				components.GET("/:compName", s.GetComponent)
				components.PUT("/:compName", s.GetComponent)
				components.GET("/", s.GetApp)
				components.GET("", s.GetApp)
				components.DELETE("/:compName", s.DeleteComponent)

				traitWorkload := components.Group("/:compName/" + util.TraitDefinitionPath)
				{
					traitWorkload.POST("/", s.AttachTrait)
					traitWorkload.DELETE("/:traitName", s.DetachTrait)
				}
			}
		}
	}
	// workload related api
	workload := api.Group(util.WorkloadDefinitionPath)
	{
		workload.POST("/", s.CreateWorkload)
		workload.GET("/:workloadName", s.GetWorkload)
		workload.PUT("/:workloadName", s.UpdateWorkload)
		workload.GET("/", s.ListWorkload)
		workload.GET("", s.ListWorkload)
	}
	// trait related api
	trait := api.Group(util.TraitDefinitionPath)
	{
		trait.GET("/:traitName", s.GetTrait)
		trait.GET("/", s.ListTrait)
		trait.GET("", s.ListTrait)
	}
	// scope related api
	scopes := api.Group(util.ScopeDefinitionPath)
	{
		scopes.POST("/", s.CreateScope)
		scopes.GET("/:scopeName", s.GetScope)
		scopes.PUT("/:scopeName", s.UpdateScope)
		scopes.GET("/", s.ListScope)
		scopes.GET("", s.ListScope)
		scopes.DELETE("/:scopeName", s.DeleteScope)
	}

	// capability center related api
	capCenters := api.Group(util.CapabilityCenterPath)
	{
		capCenters.PUT("/", s.AddCapabilityCenter)
		capCenters.GET("/", s.ListCapabilityCenters)
		capCenters.GET("", s.ListCapabilityCenters)
		capCenters.DELETE("/:capabilityCenterName", s.DeleteCapabilityCenter)

		caps := capCenters.Group("/:capabilityCenterName" + util.CapabilityPath)
		{
			caps.PUT("/", s.SyncCapabilityCenter)
			caps.PUT("/:capabilityName", s.AddCapabilityIntoCluster)
		}
	}

	// capability related api
	caps := api.Group(util.CapabilityPath)
	{
		caps.DELETE("/:capabilityName", s.RemoveCapabilityFromCluster)
		caps.DELETE("/", s.RemoveCapabilityFromCluster)
		caps.GET("/", s.ListCapabilities)
		caps.GET("", s.ListCapabilities)
	}

	// version
	api.GET(util.VersionPath, s.GetVersion)

	// swagger
	url := ginSwagger.URL("/swagger/doc.json")
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, url))

	// default
	router.NoRoute(util.NoRoute())

	return router
}
