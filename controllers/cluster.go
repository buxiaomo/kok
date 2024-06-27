package controllers

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"golang.org/x/exp/maps"
	"kok/pkg/appmarket"
	"kok/pkg/control"
	"kok/pkg/version"
	"net/http"
	"sort"
	"time"
)

func Plugin(namespace string, kubeconfig string, network string, opts map[string]map[string]interface{}) {
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
				i, _ := c.Status(namespace)
				if i.Status.ReadyReplicas == 1 {
					am := appmarket.New(kubeconfig)
					switch network {
					case "flannel":
						am.Flannel("kube-system", "flannel").Install(opts["flannel"])
					case "calico":
						//am.Flannel("kube-system", "flannel").Install()
					case "canal":
						//am.Flannel("kube-system", "flannel").Install()
					case "antrea":
						//am.Flannel("kube-system", "flannel").Install()
					case "cilium":
						//am.Flannel("kube-system", "flannel").Install()
					default:
						am.Flannel("kube-system", "flannel").Install(opts["flannel"])
					}
					am.CoreDNS("kube-system", "coredns").Install(opts["coredns"])

					am.MetricsServer("kube-system", "metrics-server").Install(opts["metrics"])
					finish <- true
					return
				}
				count++
			}
			//fmt.Println("wait control-plane is reday.")
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
	ins, err := k.GetInstance()
	if err != nil {
		panic(err)
	}
	for _, a := range ins.Items {
		instance = append(instance, map[string]interface{}{
			"Status":    a.Status.Phase,
			"StartTime": a.Status.StartTime,
			"Namespace": a.Namespace,
			"Name":      a.Name,
			"Version":   a.Labels["version"],
		})
	}
	c.HTML(http.StatusOK, "cluster.html", gin.H{
		"items":    strKeys,
		"instance": instance,
	})
}

func ClusterCreate(c *gin.Context) {
	type CREATE struct {
		Namespace   string `json:"namespace" binding:"required"`
		Registry    string `json:"registry" binding:"required"`
		Version     string `json:"version" binding:"required"`
		ServiceCidr string `json:"serviceCidr" binding:"required"`
		PodCidr     string `json:"podCidr" binding:"required"`
		DnsSvc      string `json:"dnsSvc" binding:"required"`
		Network     string `json:"network" binding:"required"`
		NodePort    string `json:"nodePort" binding:"required"`
	}
	var info CREATE

	err := c.BindJSON(&info)
	if err != nil {
		panic("bind: " + err.Error())
	}

	fmt.Println(info)
	kok := control.New()
	ns := kok.CreateNS(info.Namespace)
	ns.CreatePVC("control-plane-vol")
	ns.CreateKubeApiserverConfig()
	ns.CreateKubeproxyConfig()
	ns.CreateKubeconfig()
	ip := ns.CreateSvc()
	ns.CreateDeploy("control-plane", info.Registry, info.Version, ip, info.ServiceCidr, info.PodCidr, info.NodePort)
	go Plugin(info.Namespace, fmt.Sprintf("./kubeconfig/%s.kubeconfig", info.Namespace), info.Network, map[string]map[string]interface{}{
		"coredns": {
			"replicaCount": 1,
			"clusterIP":    info.DnsSvc,
		},
		"flannel": {
			"subNet": info.PodCidr,
		},
	})

	//kok.DeleteAll(ns.Name)

	v := version.GetVersion(info.Version)
	c.JSON(http.StatusOK, gin.H{
		"cmd": fmt.Sprintf(
			"curl -s http://172.16.0.183:8080/install | bash -s kok --master %s --containerd %s --runc %s --kubernetes %s",
			*ip,
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
