package main

import (
	"fmt"
	"kok/pkg/pki"
	"os"
)

func main() {
	hostname, err := os.Hostname()
	if err != nil {
		panic(err.Error())
	} else {
		hostname = os.Getenv("POD_NAME")
	}
	certMeta, err := pki.NewSealosCertMetaData(
		"/etc/kubernetes/pki",
		"/etc/kubernetes/pki/etcd",
		[]string{
			"kube-apiserver",
			fmt.Sprintf("kube-apiserver.%s.svc", os.Getenv("NAMESPACE")),
			fmt.Sprintf("%s.etcd", hostname),
			os.Getenv("EXTERNAL_IP"),
			os.Getenv("CLUSTERDNS"),
		},
		hostname,
		os.Getenv("POD_IP"),
		"cluster.local",
	)
	if err != nil {
		panic(err)
	}
	err = certMeta.GenerateAll()
	if err != nil {
		panic(err)
	}
}
