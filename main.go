package main

import (
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"kok/models"
	"kok/pkg/control"
	"kok/routers"
	"os"
)

func init() {
	viper.SetDefault("GIN_MODE", "debug")
	viper.SetDefault("GIN_HOST", ":8080")
	viper.SetDefault("JWT_TOKEN", "secret")
	viper.SetDefault("DB_URL", "./kok.sqlite")
	viper.SetDefault("DB_TYPE", "sqlite")
	viper.SetDefault("WEBHOOK_URL", "http://127.0.0.1:8080")
	viper.SetDefault("DOMAIN_NAME", "example.com")
	viper.SetDefault("PROMETHEUS_URL", "http://prometheus.kok.svc:9090")
	viper.SetDefault("ELASTICSEARCH_URL", "http://elasticsearch.kok.svc:9200")

	viper.AutomaticEnv()

	if _, err := os.Stat("./kubeconfig"); err != nil {
		os.Mkdir("./kubeconfig", 0755)
	}

	kc := control.New("")
	if !kc.HasDefaultSC() {
		panic("cluster not has default storageclass!")
	}
	go kc.ClearPodOnFaultyNode()

	models.ConnectDB(viper.GetString("DB_TYPE"), viper.GetString("DB_URL"))
}

func main() {
	gin.SetMode(viper.GetString("GIN_MODE"))
	r := routers.SetupRouter()
	r.Run(viper.GetString("GIN_HOST"))
}
