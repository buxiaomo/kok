package controllers

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func Healthz(c *gin.Context) {
	//if err := db.Healthz(); err != nil {
	//	c.JSON(http.StatusServiceUnavailable, gin.H{"msg": "cache connection exception"})
	//	return
	//}
	c.Status(http.StatusOK)
	return
}
