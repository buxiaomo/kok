package controllers

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"golang.org/x/exp/maps"
	"kok/pkg/appmarket"
	"kok/pkg/control"
	"kok/pkg/utils"
	"kok/pkg/version"
	"net/http"
	"sort"
	"strings"
	"time"
)

func Plugin(namespace, kubeconfig, network, version string, opts map[string]map[string]interface{}) {

	c := control.New()

	timeout := time.After(time.Minute * 15)
	finish := make(chan bool)
	count := 1

	go func() {
		for {
			select {
			case <-timeout:
				fmt.Println("wait timeout..")
				finish <- true
				return
			default:
				scheduler, _ := c.DeploymentStatus(namespace, "kube-scheduler")
				controllerMgr, _ := c.DeploymentStatus(namespace, "kube-controller-manager")
				//fmt.Println(fmt.Sprintf("wait for cluster is ready, kube-scheduler: %v, kube-controller-manager: %v...", scheduler.Status.ReadyReplicas, controllerMgr.Status.ReadyReplicas))
				if scheduler.Status.ReadyReplicas >= 1 && controllerMgr.Status.ReadyReplicas >= 1 {
					am := appmarket.New(kubeconfig)
					switch network {
					case "flannel":
						am.Flannel("kube-system", "flannel", version).Install(opts["flannel"])
					case "calico":
						am.Calico("kube-system", "calico", version).Install(opts["calico"])
					case "canal":
						am.Canal("kube-system", "canal", version).Install(opts["canal"])
					case "antrea":
						am.Antrea("kube-system", "antrea", version).Install(opts["antrea"])
					case "cilium":
						//am.Flannel("kube-system", "flannel").Install(opts["cilium"])
					default:
						am.Flannel("kube-system", "flannel", version).Install(opts["flannel"])
					}
					am.CoreDNS("kube-system", "coredns").Install(opts["coredns"])

					am.MetricsServer("kube-system", "metrics-server").Install(opts["metrics"])
					finish <- true
					return
				}
				count++
			}
			time.Sleep(time.Second * 1)
		}
	}()

	<-finish
}

func ClusterPages(c *gin.Context) {
	strKeys := maps.Keys(version.List)
	sort.Strings(strKeys)
	var instance []map[string]interface{}
	k := control.New()
	ins, err := k.NamespaceList()
	if err != nil {

		panic(err)
	}

	for _, ns := range ins.Items {
		components := map[string]string{}

		api, _ := k.GetDeployment(ns.Name, "kube-apiserver")
		if api.Status.ReadyReplicas >= 1 {
			components["Apiserver"] = "Running"
		} else {
			components["Apiserver"] = "Init"
		}

		scheduler, _ := k.GetDeployment(ns.Name, "kube-scheduler")
		if scheduler.Status.ReadyReplicas >= 1 {
			components["Scheduler"] = "Running"
		} else {
			components["Scheduler"] = "Init"
		}

		controller, _ := k.GetDeployment(ns.Name, "kube-controller-manager")
		if controller.Status.ReadyReplicas >= 1 {
			components["controllerManager"] = "Running"
		} else {
			components["controllerManager"] = "Init"
		}

		instance = append(instance, map[string]interface{}{
			"Name":         ns.Name,
			"Version":      ns.Labels["kubernetes"],
			"Status":       ns.Status.Phase,
			"Network":      ns.Labels["network"],
			"LoadBalancer": ns.Labels["loadBalancer"],
			"Components":   components,
		})
	}
	c.HTML(http.StatusOK, "cluster.html", gin.H{
		"items":    strKeys,
		"instance": instance,
	})
}

func ClusterCreate(c *gin.Context) {
	type CREATE struct {
		Project        string `json:"project" binding:"required"`
		Env            string `json:"env" binding:"required"`
		Registry       string `json:"registry" binding:"required"`
		Version        string `json:"version" binding:"required"`
		ServiceCidr    string `json:"serviceCidr" binding:"required"`
		PodCidr        string `json:"podCidr" binding:"required"`
		DnsSvc         string `json:"dnsSvc" binding:"required"`
		Network        string `json:"network" binding:"required"`
		NetworkVersion string `json:"networkVersion" binding:"required"`
		NodePort       string `json:"nodePort" binding:"required"`
	}
	var info CREATE

	err := c.BindJSON(&info)
	if err != nil {
		panic("bind: " + err.Error())
	}

	//ns, err := kok.CreateNS(info.Namespace)
	//if err != nil {
	//	log.Printf("Create ns error: %s", err.Error())
	//	c.JSON(http.StatusInternalServerError, gin.H{
	//		"cmd": nil,
	//		"msg": err.Error(),
	//	})
	//	return
	//}
	//
	//err = ns.CreateSA("control-plane")
	//if err != nil {
	//	log.Printf("Create sa error: %s", err.Error())
	//	c.JSON(http.StatusInternalServerError, gin.H{
	//		"cmd": nil,
	//		"msg": err.Error(),
	//	})
	//	return
	//}
	//
	//err = ns.CreateRBAC()
	//if err != nil {
	//	log.Printf("Create rbac error: %s", err.Error())
	//	c.JSON(http.StatusInternalServerError, gin.H{
	//		"cmd": nil,
	//		"msg": err.Error(),
	//	})
	//	return
	//}
	//
	//err = ns.CreatePVC("control-plane-vol")
	//if err != nil {
	//	log.Printf("Create pvc error: %s", err.Error())
	//	c.JSON(http.StatusInternalServerError, gin.H{
	//		"cmd": nil,
	//		"msg": err.Error(),
	//	})
	//	return
	//}
	//
	//err = ns.CreateKubeApiserverConfig()
	//if err != nil {
	//	log.Printf("Create configmap error: %s", err.Error())
	//	c.JSON(http.StatusInternalServerError, gin.H{
	//		"cmd": nil,
	//		"msg": err.Error(),
	//	})
	//	return
	//}
	//err = ns.CreateKubeproxyConfig()
	//if err != nil {
	//	log.Printf("Create configmap error: %s", err.Error())
	//	c.JSON(http.StatusInternalServerError, gin.H{
	//		"cmd": nil,
	//		"msg": err.Error(),
	//	})
	//	return
	//}
	//err = ns.CreateKubeconfig()
	//if err != nil {
	//	log.Printf("Create configmap error: %s", err.Error())
	//	c.JSON(http.StatusInternalServerError, gin.H{
	//		"cmd": nil,
	//		"msg": err.Error(),
	//	})
	//	return
	//}
	//
	//_, err = ns.CreateIngress(viper.GetString("DOMAIN_NAME"))
	//if err != nil {
	//	log.Printf("Create ingress error: %s", err.Error())
	//	c.JSON(http.StatusInternalServerError, gin.H{
	//		"cmd": nil,
	//		"msg": err.Error(),
	//	})
	//	return
	//}
	//
	//ip := ns.CreateSvc()
	//
	//ns.CreateDeploy("control-plane", info.Registry, info.Version, ip, info.ServiceCidr, info.PodCidr, info.NodePort)

	v := version.GetVersion(info.Version)

	minIp, _ := utils.GetCidrIpRange(strings.Replace(strings.Replace(info.ServiceCidr, "/", "-", 1), "-", "/", 1))

	kok := control.New()
	namespace := fmt.Sprintf("%s-%s", info.Project, info.Env)

	_, err = kok.CreateNS(namespace, map[string]string{
		"project":        info.Project,
		"env":            info.Env,
		"kubernetes":     v["kubernetes"],
		"etcd":           v["etcd"],
		"containerd":     v["containerd"],
		"runc":           v["runc"],
		"registry":       info.Registry,
		"nodePort":       info.NodePort,
		"serviceSubnet":  strings.Replace(info.ServiceCidr, "/", "-", 1),
		"podSubnet":      strings.Replace(info.PodCidr, "/", "-", 1),
		"network":        info.Network,
		"fieldManager":   "control-plane",
		"networkVersion": info.NetworkVersion,
		"pause":          v["pause"],
		"clusterDNS":     minIp,
	})

	if err != nil {
		panic(err.Error())
	}
	err = kok.CreateAll(info.Registry, info.Version, info.Project, info.Env, info.NodePort, info.ServiceCidr, info.PodCidr)
	if err != nil {
		panic(err.Error())
	}

	go Plugin(namespace, fmt.Sprintf("./kubeconfig/%s.kubeconfig", namespace), info.Network, info.NetworkVersion, map[string]map[string]interface{}{
		"coredns": {
			"replicaCount": 1,
			"clusterIP":    info.DnsSvc,
		},
		"flannel": {
			"subNet": info.PodCidr,
		},
		"calico": {
			"subNet": info.PodCidr,
		},
		"canal": {
			"subNet": info.PodCidr,
		},
		"antrea": {
			"subNet": info.PodCidr,
		},
	})

	//kok.DeleteAll(ns.Name)
	c.JSON(http.StatusOK, gin.H{
		"cmd": fmt.Sprintf(
			"curl -s %s/install | bash -s kok --master %s --containerd %s --runc %s --kubernetes %s",
			viper.GetString("WEBHOOK_URL"),
			"EXT_IP",
			v["containerd"],
			v["runc"],
			v["kubernetes"],
		),
		"msg": nil,
	})
}

func ClusterDelete(c *gin.Context) {
	name := c.Query("name")
	fmt.Println(name)
	kok := control.New()
	kok.DeleteAll(name)
	c.JSON(http.StatusOK, gin.H{
		"cmd": nil,
		"msg": nil,
	})
}

func ClusterReDeploy(c *gin.Context) {
	namespace := c.Query("namespace")
	name := c.Query("name")
	kok := control.New()
	e := kok.DeletePod(namespace, name)
	if e != nil {
		c.JSON(http.StatusOK, gin.H{
			"cmd": nil,
			"msg": e.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"cmd": nil,
		"msg": nil,
	})
	return

}

func ClusterEnableHA(c *gin.Context) {
	name := c.Query("name")
	kok := control.New()
	err := kok.ScaleService(name)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"cmd": nil,
			"msg": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"cmd": nil,
		"msg": nil,
	})
	return
}
