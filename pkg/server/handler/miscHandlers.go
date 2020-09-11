package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/oam-dev/kubevela/pkg/server/apis"
	"github.com/oam-dev/kubevela/version"
)

func GetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, apis.Response{
		Code: http.StatusOK,
		Data: map[string]string{"version": version.VelaVersion},
	})
}
