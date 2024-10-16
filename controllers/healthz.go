package controllers

import (
	"github.com/gin-gonic/gin"
	db "kok/models"
	"net/http"
)

func Healthz(c *gin.Context) {
	if err := db.Healthz(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"msg": err.Error()})
		return
	}
	c.Status(http.StatusOK)
	return
}
