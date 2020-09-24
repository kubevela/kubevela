package util

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/oam-dev/kubevela/pkg/server/apis"
)

var DefaultDashboardPort = ":38081"

const DefaultAPIServerPort = ":8081"

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

func URL(url string) string {
	return fmt.Sprintf("http://127.0.0.1%s/api%s", DefaultDashboardPort, url)
}
