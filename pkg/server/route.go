package server

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/cloud-native-application/rudrx/pkg/server/handler"
	"github.com/cloud-native-application/rudrx/pkg/server/util"
)

// setup the gin http server handler
func SetupRoute() http.Handler {
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
		envs.POST("/", handler.CreateEnv)
		envs.GET("/:envName", handler.GetEnv)
		envs.GET("/", handler.ListEnv)
		envs.DELETE("/:envName", handler.DeleteEnv)
		// app related operation
		apps := envs.Group("/:envName/apps")
		{
			apps.POST("/", handler.CreateApps)
			apps.GET("/:appName", handler.GetApps)
			apps.PUT("/:appName", handler.UpdateApps)
			apps.GET("/", handler.ListApps)
			apps.DELETE("/:appName", handler.DeleteApps)
		}
	}
	// workload related api
	workload := api.Group(util.WorkloadDefinitionPath)
	{
		workload.POST("/", handler.CreateWorkload)
		workload.GET("/:workloadName", handler.GetWorkload)
		workload.PUT("/:workloadName", handler.UpdateWorkload)
		workload.GET("/", handler.ListWorkload)
		workload.DELETE("/:workloadName", handler.DeleteWorkload)
	}
	// trait related api
	trait := api.Group(util.TraitDefinitionPath)
	{
		trait.POST("/", handler.CreateTrait)
		trait.GET("/:traitName", handler.GetTrait)
		trait.PUT("/:traitName", handler.UpdateTrait)
		trait.GET("/", handler.ListTrait)
		trait.DELETE("/:traitName", handler.DeleteTrait)
	}
	// scope related api
	scopes := api.Group(util.ScopeDefinitionPath)
	{
		scopes.POST("/", handler.CreateScope)
		scopes.GET("/:scopeName", handler.GetScope)
		scopes.PUT("/:scopeName", handler.UpdateScope)
		scopes.GET("/", handler.ListScope)
		scopes.DELETE("/:scopeName", handler.DeleteScope)
	}

	// scope related api
	repo := api.Group(util.RepoPath)
	{
		repo.GET("/:categoryName/:definitionName", handler.GetDefinition)
		repo.PUT("/:categoryName/:definitionName", handler.UpdateDefinition)
		repo.GET("/:categoryName", handler.ListDefinition)
	}
	// version
	api.GET(util.VersionPath, handler.GetVersion)
	// default
	router.NoRoute(util.NoRoute())

	return router
}
