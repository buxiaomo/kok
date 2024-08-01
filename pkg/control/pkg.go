package control

import (
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"os"
	"path/filepath"
)

func createKubeconfig(kubeconfig string) (config *rest.Config, err error) {
	if kubeconfig == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}
	_, err = os.Stat(kubeconfig)
	if err == nil {

		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		//panic(err)
		return
	} else {
		config, err = rest.InClusterConfig()
		return
	}
}
