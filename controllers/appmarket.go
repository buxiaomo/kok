package controllers

import (
	"github.com/gin-gonic/gin"
	"kok/pkg/appmarket"
	"net/http"
)

func AppmarketGet(c *gin.Context) {
	name := c.Query("name")
	if name == "" {

	} else {
		am := appmarket.New("")
		//fmt.Println(chart.Version, chart.Description)
		c.JSON(http.StatusOK, gin.H{
			"data": am.Chart().Search(name),
			"msg":  nil,
		})
		return
	}
}
