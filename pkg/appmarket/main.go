package appmarket

import (
	"fmt"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/kube"
	"helm.sh/helm/v3/pkg/repo"
	"os"
)

type appMark struct {
	kubeconfig string
	cfg        *action.Configuration
}

func New(kubeconfig string) *appMark {
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(kube.GetConfig(kubeconfig, "", "kube-system"), "kube-system", os.Getenv("HELM_DRIVER"), func(format string, v ...interface{}) {
		fmt.Sprintf(format, v)
	}); err != nil {
		panic(err.Error())
	}
	return &appMark{
		cfg: actionConfig,
	}
}

type chart struct {
	cfg *action.Configuration
}

func (app appMark) Chart() chart {
	return chart{
		app.cfg,
	}
}

type Chart struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

func (app chart) Search(name string) (chats []Chart) {
	indexFile, err := repo.LoadIndexFile("./appmarket/assets/index.yaml")
	if err != nil {
		panic(err.Error())
	}
	for _, entry := range indexFile.Entries[name] {
		chats = append(chats, Chart{
			Name:        entry.Name,
			Version:     entry.AppVersion,
			Description: entry.Description,
		})
	}
	return chats
}
