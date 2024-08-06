package appmarket

import (
	"fmt"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/kube"
	"helm.sh/helm/v3/pkg/repo"
	"log"
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

func (app chart) Install(namespace, releaseName, chatName string, GenerateName bool, version string, vals map[string]interface{}) error {
	chatPath := fmt.Sprintf("./appmarket/assets/%s/%s-%s.tgz", chatName, chatName, version)
	chart, err := loader.Load(chatPath)
	if err != nil {
		log.Printf(err.Error())
		return err
	}

	// 检查 release 是否存在
	histClient := action.NewHistory(app.cfg)
	histClient.Max = 1
	if _, err := histClient.Run(chatName); err != nil {
		client := action.NewInstall(app.cfg)
		client.Namespace = namespace
		client.IsUpgrade = true
		client.Force = true
		client.ReleaseName = releaseName
		rel, err := client.Run(chart, vals)
		if err != nil {
			log.Printf(err.Error())
			return err
		}
		log.Printf("Installed Chart from path: %s in namespace: %s\n", rel.Name, rel.Namespace)
		return nil
	}

	upgrade := action.NewUpgrade(app.cfg)
	// 设置 Upgrade 参数
	upgrade.Namespace = namespace
	upgrade.Timeout = 300
	rel, err := upgrade.Run(releaseName, chart, vals)
	if err != nil {
		log.Printf(err.Error())
		return err
	}
	log.Printf("Upgrade Chart from path: %s in namespace: %s\n", rel.Name, rel.Namespace)
	return nil

}

func (app chart) UnInstall(name string) (err error) {
	uninstall := action.NewUninstall(app.cfg)
	uninstall.Timeout = 30e9 // 设置超时时间300秒
	uninstall.KeepHistory = false
	resp, err := uninstall.Run(name)
	if err != nil {
		log.Printf("卸载失败: %s\n", resp.Release.Name, err)
		return
	}
	log.Printf("%s 成功卸载\n", resp.Release.Name)
	return
}
