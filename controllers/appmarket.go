package controllers

import (
	"github.com/gin-gonic/gin"
	"kok/pkg/appmarket"
	"net/http"
)

func AppmarketGet(c *gin.Context) {
	name := c.Query("name")
	kubeVersion := c.Query("kubeVersion")

	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"data": nil,
			"msg":  "need name",
		})
		return
	} else {
		am := appmarket.New("")
		c.JSON(http.StatusOK, gin.H{
			"data": am.Chart().Search(name, kubeVersion),
			"msg":  nil,
		})
		return
	}
}
