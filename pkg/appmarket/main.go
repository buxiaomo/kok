package appmarket

import (
	"fmt"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/kube"
	"os"
)

type appMark struct {
	cfg *action.Configuration
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
