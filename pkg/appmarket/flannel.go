package appmarket

import (
	"fmt"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"log"
)

type flannel struct {
	Namespace string
	Name      string
	chartPath string
	cfg       *action.Configuration
}

func (c flannel) Install(vals map[string]interface{}) {
	chart, err := loader.Load(c.chartPath)
	if err != nil {
		log.Printf(err.Error())
	}

	// 检查 release 是否存在
	histClient := action.NewHistory(c.cfg)
	histClient.Max = 1
	if _, err := histClient.Run(c.Name); err != nil {
		client := action.NewInstall(c.cfg)
		client.Namespace = c.Namespace
		client.ReleaseName = c.Name
		client.IsUpgrade = true
		client.Force = true
		rel, err := client.Run(chart, vals)
		if err != nil {
			panic(err)
		}
		log.Printf("Installed Chart from path: %s in namespace: %s\n", rel.Name, rel.Namespace)
		return
	}

	upgrade := action.NewUpgrade(c.cfg)
	// 设置 Upgrade 参数
	upgrade.Namespace = c.Namespace
	upgrade.Timeout = 300
	rel, err := upgrade.Run(c.Name, chart, vals)
	log.Printf("Upgrade Chart from path: %s in namespace: %s\n", rel.Name, rel.Namespace)
}

func (c flannel) UnInstall() {
	uninstall := action.NewUninstall(c.cfg)
	uninstall.Timeout = 30e9 // 设置超时时间300秒
	uninstall.KeepHistory = false
	resp, err := uninstall.Run(c.Name)
	if err != nil {
		panic(fmt.Errorf("卸载失败\n%s", err))
	}
	log.Printf("%s 成功卸载\n", resp.Release.Name)
}

func (app appMark) Flannel(namespace, name, version string) flannel {
	return flannel{
		Namespace: namespace,
		Name:      name,
		chartPath: fmt.Sprintf("./appmarket/assets/flannel/flannel-%s.tgz", version),
		cfg:       app.cfg,
	}
}
