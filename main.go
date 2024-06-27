package main

import (
	"embed"
	_ "embed"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"kok/routers"
)

// go:embed static/*
// go:embed static/js/*
// go:embed static/css/*
// go:embed static/fonts/*
// go:embed static/images/*
var static embed.FS

// go:embed templates/*
var templates embed.FS

// go:embed appmarket/*
var appmarket embed.FS

func init() {
	viper.SetDefault("GIN_MODE", "debug")
	viper.SetDefault("GIN_HOST", ":8080")
	viper.SetDefault("JWT_TOKEN", "secret")
	viper.SetDefault("DB_URL", "./kok.sqlite")
	viper.SetDefault("DB_TYPE", "sqlite")
	viper.AutomaticEnv()
	//db.ConnectDB(viper.GetString("DB_URL"), viper.GetString("DB_TYPE"))
}

func test() {
	//kok := control.New()
	//if !kok.HasDefaultSC() {
	//	panic("cluster not has default storageclass!")
	//}

	//am := appmarket.New()
	//am.CoreDNS().Install()
	//am.CoreDNS().UnInstall()
	//am.Flannel("kube-system", "flannel").Install()
	//am.Flannel("kube-system", "flannel").UnInstall()
	////am.Install()

	//controllers.Plugin("demo117", fmt.Sprintf("./kubeconfig/%s.kubeconfig", "demo117"))
}
func main() {
	gin.SetMode(viper.GetString("GIN_MODE"))
	r := routers.SetupRouter()
	r.Run(viper.GetString("GIN_HOST"))
}
