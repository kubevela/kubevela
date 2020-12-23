package util

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/oam-dev/kubevela/pkg/server/apis"
)

// DefaultDashboardPort refers to the default port number of dashboard
var DefaultDashboardPort = ":38081"

// DefaultAPIServerPort refers to the default port number of APIServer
const DefaultAPIServerPort = ":38081"

// AssembleResponse assembles response data to return
func AssembleResponse(c *gin.Context, data interface{}, err error) {
	var code = http.StatusOK
	if err != nil {
		code = http.StatusInternalServerError
		c.JSON(code, apis.Response{
			Code: code,
			Data: err.Error(),
		})
		return
	}

	c.JSON(code, apis.Response{
		Code: code,
		Data: data,
	})
}

// URL returns the URL of Dashboard based on default port
func URL(url string) string {
	return fmt.Sprintf("http://127.0.0.1%s/api%s", DefaultDashboardPort, url)
}
