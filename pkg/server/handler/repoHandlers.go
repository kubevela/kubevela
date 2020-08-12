package handler

import (
	"github.com/gin-gonic/gin"
	ctrl "sigs.k8s.io/controller-runtime"
)

// repo related handlers
const (
	repoUrlKey = "repoUrl"
)

func UpdateDefinition(c *gin.Context) {
	//ctx := util.GetContext(c)
	categoryName := c.Param("categoryName")
	definitionName := c.Param("definitionName")
	repoUrl, found := c.GetQuery(repoUrlKey)
	if !found {
		panic("no repoUrl in update")
	}
	ctrl.Log.Info("Get Update Repo Definition request for", "categoryName", categoryName, "definitionName", definitionName, "repoUrl", repoUrl)
}

func GetDefinition(c *gin.Context) {
	categoryName := c.Param("categoryName")
	definitionName := c.Param("definitionName")
	repoUrl, found := c.GetQuery(repoUrlKey)
	if !found {
		panic("no repoUrl in update")
	}
	ctrl.Log.Info("Get Repo Definition request for", "categoryName", categoryName, "definitionName", definitionName, "repoUrl", repoUrl)
}

func ListDefinition(c *gin.Context) {
	categoryName := c.Param("categoryName")
	repoUrl, found := c.GetQuery(repoUrlKey)
	if !found {
		panic("no repoUrl in update")
	}
	ctrl.Log.Info("Get Repo Definition request for", "categoryName", categoryName, repoUrl)
}
