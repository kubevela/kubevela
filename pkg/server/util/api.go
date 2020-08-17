package util

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/cloud-native-application/rudrx/pkg/server/apis"
)

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
	return "http://127.0.0.1:8080/api" + url
}
