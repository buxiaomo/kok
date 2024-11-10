package controllers

import (
	"github.com/gin-gonic/gin"
	db "kok/models"
	"net/http"
)

func GetVersion(c *gin.Context) {
	var version db.Version
	versions, _ := version.SelectAll()

	c.JSON(http.StatusOK, gin.H{
		"version": versions,
	})
}

func VersionPages(c *gin.Context) {
	var version db.Version
	versions, _ := version.SelectAll()
	//v := []string{}
	//for _, versions := range versions {
	//	v = append(v, versions.Kubernetes)
	//}
	//b, _ := json.Marshal(versions)

	c.HTML(http.StatusOK, "version.html", gin.H{
		"versions": versions,
	})
}

func DeleteVersion(c *gin.Context) {
	name := c.Query("name")
	var version db.Version
	version.Del(name)
	c.JSON(http.StatusOK, gin.H{
		"cmd": nil,
		"msg": nil,
	})
}

func CreateVersion(c *gin.Context) {
	var version db.Version
	err := c.BindJSON(&version)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"cmd": nil,
			"msg": err.Error(),
		})
		return
	}
	err = version.Add(version)
	if err != nil {
		panic(err.Error())
	}
	c.JSON(http.StatusOK, gin.H{
		"cmd": nil,
		"msg": nil,
	})
}
