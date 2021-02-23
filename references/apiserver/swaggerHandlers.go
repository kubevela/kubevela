package apiserver

import (
	"github.com/gin-gonic/gin"
	"github.com/swaggo/swag"
)

// SwaggerJSON use /swagger.json and /doc.json 404
func (s *APIServer) SwaggerJSON(c *gin.Context) {
	path := c.Param("any")
	switch path {
	case "/doc.json":
		c.String(404, "404 page not found")
	case "/swagger.json":
		c.Header("Content-Type", "application/json")
		doc, err := swag.ReadDoc()
		if err != nil {
			panic(err)
		}
		c.String(200, doc)
	default:
		return
	}
	c.Abort()
}
