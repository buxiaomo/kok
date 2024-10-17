package appmarket

import (
	"fmt"
	"github.com/hashicorp/go-version"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/kube"
	"helm.sh/helm/v3/pkg/repo"
	"log"
)

type appMark struct {
	kubeconfig string
	cfg        *action.Configuration
	//settings   *cli.EnvSettings
}

func New(kubeconfig string) *appMark {
	actionConfig := new(action.Configuration)
	//settings := cli.New()
	if err := actionConfig.Init(kube.GetConfig(kubeconfig, "", "kube-system"), "kube-system", "secret", func(format string, v ...interface{}) {
		fmt.Sprintf(format, v)
	}); err != nil {
		panic(err.Error())
	}
	return &appMark{
		kubeconfig: kubeconfig,
		cfg:        actionConfig,
		//settings:   settings,
	}
}

type chart struct {
	kubeconfig string
	cfg        *action.Configuration
	//settings   *cli.EnvSettings
}

func (app appMark) Chart() chart {
	return chart{
		cfg:        app.cfg,
		kubeconfig: app.kubeconfig,
		//settings:   app.settings,
	}
}

type Chart struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

func (app chart) Get(chatName string) bool {
	histClient := action.NewHistory(app.cfg)
	histClient.Max = 1
	if _, err := histClient.Run(chatName); err != nil {
		return false
	}
	return true
}

func (app chart) Search(name, kubeVersion string) (chats []Chart) {
	indexFile, err := repo.LoadIndexFile("./appmarket/assets/index.yaml")
	if err != nil {
		panic(err.Error())
	}
	for _, entry := range indexFile.Entries[name] {
		isOk, err := isKubeVersionCompatible(kubeVersion, entry.Metadata.KubeVersion)
		//fmt.Println(kubeVersion, entry.Metadata.KubeVersion, isOk, err)
		if err != nil {
			fmt.Println(err.Error())
		}
		if isOk {
			chats = append(chats, Chart{
				Name:        entry.Name,
				Version:     entry.AppVersion,
				Description: entry.Description,
			})
		}
	}
	return chats
}

func (app chart) Install(namespace, releaseName, chatName string, GenerateName bool, version string, vals map[string]interface{}) error {
	//app.settings.SetNamespace(namespace)

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
		client.GenerateName = GenerateName
		client.ReleaseName = releaseName
		client.Namespace = namespace
		client.CreateNamespace = true
		rel, err := client.Run(chart, vals)
		if err != nil {
			fmt.Println(client)
			log.Printf(err.Error())
			return err
		}
		//fmt.Println(rel)
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

// 比较 Kubernetes 版本是否在 Chart 的 kubeVersion 兼容范围内
func isKubeVersionCompatible(kubeVersion string, chartKubeVersion string) (bool, error) {
	// 如果 chart 没有定义 kubeVersion，默认认为兼容
	if chartKubeVersion == "" {
		return true, nil
	}

	// 解析版本约束范围
	constraint, err := version.NewConstraint(chartKubeVersion)
	if err != nil {
		return false, fmt.Errorf("invalid kubeVersion constraint in chart: %v", err)
	}

	// 解析当前 Kubernetes 版本
	kubeVer, err := version.NewVersion(kubeVersion)
	if err != nil {
		return false, fmt.Errorf("invalid Kubernetes version: %v", err)
	}

	// 检查 Kubernetes 版本是否满足约束
	return constraint.Check(kubeVer), nil
}
