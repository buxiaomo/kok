package controllers

import (
	"bytes"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"kok/pkg/control"
	"kok/pkg/utils"
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
}

func NodeInit(c *gin.Context) {
	name := c.Query("name")
	kok := control.New("")
	ns, err := kok.GetNamespace(name)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	serviceSubnet := strings.Replace(ns.Labels["serviceSubnet"], "-", "/", 1)
	minIp, _ := utils.GetCidrIpRange(serviceSubnet)

	tmpl, err := template.ParseFiles("./templates/install.sh")
	if err != nil {
		fmt.Println("create template failed, err:", err)
		return
	}

	cm, _ := kok.GetConfigMap(name, "pki")
	fmt.Println()

	//var temp io.Writer
	buf := new(bytes.Buffer)
	err = tmpl.Execute(buf, install{
		Runc:          ns.Labels["runc"],
		Containerd:    ns.Labels["containerd"],
		Registry:      ns.Labels["registry"],
		Kubernetes:    ns.Labels["kubernetes"],
		LoadBalancer:  ns.Labels["loadBalancer"],
		ClusterDNS:    minIp,
		Pause:         ns.Labels["pause"],
		ServiceSubnet: serviceSubnet,
		Project:       ns.Labels["project"],
		Env:           ns.Labels["env"],
		Pkiurl:        viper.GetString("PKI_URL"),
		Ca:            cm.Data["ca.crt"],
		Key:           cm.Data["ca.key"],
	})
	if err != nil {
		panic(err)
	}

	//tpl, _ := pongo2.FromFile("./install.sh")
	//ctx := pongo2.Context{
	//	"runc":         ns.Labels["runc"],
	//	"containerd":   ns.Labels["containerd"],
	//	"registry":     ns.Labels["registry"],
	//	"kubernetes":   ns.Labels["kubernetes"],
	//	"loadBalancer": ns.Labels["loadBalancer"],
	//	"clusterDNS":   minIp,
	//	"pause":        ns.Labels["pause"],
	//}
	//out, err := tpl.Execute(ctx)
	//if err != nil {
	//	panic(err)
	//}
	//
	//fmt.Println(out)
	c.String(http.StatusOK, buf.String())
	return
}
