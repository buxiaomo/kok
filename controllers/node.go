package controllers

import (
	"bytes"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/hashicorp/go-version"
	"github.com/spf13/viper"
	"kok/pkg/control"
	"net/http"
	"strings"
	"text/template"
)

type install struct {
	Project       string
	Env           string
	Runc          string
	Containerd    string
	Registry      string
	Kubernetes    string
	LoadBalancer  string
	ClusterDNS    string
	Pause         string
	ServiceSubnet string
	Pkiurl        string
	Ca            string
	Key           string
	KubePorxyArgs string
	KubeletArgs   string
}

func NodeInit(c *gin.Context) {
	name := c.Query("name")
	kubeControl := control.New("")

	ns, err := kubeControl.Namespace().Get(name)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	tmpl, err := template.ParseFiles("./templates/install.sh")
	if err != nil {
		fmt.Println("create template failed, err:", err)
		return
	}

	cm, _ := kubeControl.ConfigMaps().Get(ns.Name, "cluster-ca")

	info := install{
		Runc:          ns.Labels["runc"],
		Containerd:    ns.Labels["containerd"],
		Registry:      ns.Labels["registry"],
		Kubernetes:    ns.Labels["kubernetes"],
		LoadBalancer:  ns.Labels["loadBalancer"],
		ClusterDNS:    ns.Labels["clusterDNS"],
		Pause:         ns.Labels["pause"],
		ServiceSubnet: strings.Replace(ns.Labels["serviceSubnet"], "-", "/", 1),
		Project:       ns.Labels["project"],
		Env:           ns.Labels["env"],
		Pkiurl:        viper.GetString("PKI_URL"),
		Ca:            cm.Data["ca.crt"],
		Key:           cm.Data["ca.key"],
	}
	v1, err := version.NewVersion(ns.Labels["kubernetes"])
	constraints, err := version.NewConstraint(">= v1.14, < v1.24")
	if constraints.Check(v1) {
		info.KubeletArgs = "--container-runtime-endpoint=unix:///run/containerd/containerd.sock --container-runtime=remote"
	} else {
		info.KubeletArgs = "--container-runtime-endpoint=unix:///run/containerd/containerd.sock"
	}
	buf := new(bytes.Buffer)
	err = tmpl.Execute(buf, info)
	if err != nil {
		panic(err)
	}

	c.String(http.StatusOK, buf.String())
	return
}
