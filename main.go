package main

import (
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"kok/pkg/control"
	"kok/routers"
)

func init() {
	viper.SetDefault("GIN_MODE", "debug")
	viper.SetDefault("GIN_HOST", ":8080")
	viper.SetDefault("JWT_TOKEN", "secret")
	viper.SetDefault("DB_URL", "./kok.sqlite")
	viper.SetDefault("DB_TYPE", "sqlite")
	viper.SetDefault("WEBHOOK_URL", "http://127.0.0.1:8080")
	viper.SetDefault("DOMAIN_NAME", "example.com")

	viper.AutomaticEnv()
	//db.ConnectDB(viper.GetString("DB_URL"), viper.GetString("DB_TYPE"))

	kok := control.New()
	if !kok.HasDefaultSC() {
		panic("cluster not has default storageclass!")
	}

}

func test() {

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
