package routers

import (
	"fmt"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"kok/controllers"
	"net/http"
	"time"
)

var secret = []byte("secret")

func SetupRouter() *gin.Engine {
	r := gin.New()
	r.Use(sessions.Sessions("mysession", cookie.NewStore(secret)))

	r.Use(gin.Recovery())
	r.LoadHTMLGlob("templates/*")
	r.Static("/static", "./static")

	// 设置日志格式
	r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf(`{"time":"%s","dest_ip":"%s","http_method":"%s","uri_path":"%s","proto":"%s","status":%d,"response_time":"%s","http_user_agent":"%s","bytes_in":%d,"errmsg":"%s"}%v`,
			param.TimeStamp.Format(time.UnixDate),
			param.ClientIP,
			param.Method,
			param.Path,
			param.Request.Proto,
			param.StatusCode,
			param.Latency,
			param.Request.UserAgent(),
			param.BodySize,
			param.ErrorMessage,
			"\r\n",
		)
	}))

	r.GET("/healthz", controllers.Healthz)
	r.GET("/login", func(c *gin.Context) {
		c.HTML(http.StatusOK, "login.html", gin.H{})
	})
	r.POST("/login", func(c *gin.Context) {
		session := sessions.Default(c)
		username := c.PostForm("username")
		password := c.PostForm("password")
		if username == "admin" && password == "admin" {
			session.Set("username", username)
			session.Save()
			c.Redirect(http.StatusFound, "/console/index")
			return
		}
		c.HTML(http.StatusBadRequest, "login.html", gin.H{
			"errorMessage": "Invalid email or password",
		})
		return
	})
	r.GET("/install", controllers.NodeInit)

	private := r.Group("/console")
	//private.Use(AuthRequired)
	{
		private.GET("/index", func(c *gin.Context) {
			c.HTML(http.StatusOK, "index.html", gin.H{})
		})
		private.PUT("/ha", controllers.ClusterEnableHA)
		private.GET("/cluster", controllers.ClusterPages)
		private.GET("/version", controllers.VersionPages)
		private.DELETE("/version", controllers.DeleteVersion)
		private.POST("/version", controllers.CreateVersion)
		private.GET("/cluster/kubeconfig", controllers.Kubeconfig)
		private.GET("/cluster/status", controllers.ClusterStatus)
		private.PUT("/cluster/monitor", controllers.ClusterMonitor)
		private.PUT("/cluster/log", controllers.ClusterLog)
		private.POST("/cluster", controllers.ClusterCreate)
		private.GET("/cluster/appmarket", controllers.AppmarketList)
		//private.POST("/cluster/appmarket", controllers.AppmarketDeploy)
		private.DELETE("/cluster", controllers.ClusterDelete)
		private.GET("/appmarket", controllers.AppmarketGet)
	}

	return r
}

//// AuthRequired is a simple middleware to check the session.
//func AuthRequired(c *gin.Context) {
//	session := sessions.Default(c)
//	user := session.Get("username")
//	if user == nil {
//		// Abort the request with the appropriate error code
//		//c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
//		//c.HTML(http.StatusBadRequest, "/login", gin.H{
//		//	"errorMessage": "Invalid email or password",
//		//})
//		c.Redirect(http.StatusFound, "/login")
//		return
//	}
//	// Continue down the chain to handler etc
//	c.Next()
//}
