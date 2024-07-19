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

	kok := control.New("")
	if !kok.HasDefaultSC() {
		panic("cluster not has default storageclass!")
	}
	go kok.Node()
	//version.Version()
}

func main() {
	gin.SetMode(viper.GetString("GIN_MODE"))
	r := routers.SetupRouter()
	r.Run(viper.GetString("GIN_HOST"))
}
