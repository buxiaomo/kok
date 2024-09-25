package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	applyappsv1 "k8s.io/client-go/applyconfigurations/apps/v1"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	applyrbacv1 "k8s.io/client-go/applyconfigurations/rbac/v1"
	"k8s.io/client-go/rest"
	db "kok/models"
	"kok/pkg/appmarket"
	"kok/pkg/cert"
	"kok/pkg/control"
	"kok/pkg/utils"
	"log"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"
)

func kubeApiserverCfg() map[string]string {
	return map[string]string{
		"encryption-config.yaml": `kind: EncryptionConfig
apiVersion: v1
resources:
  - resources:
      - secrets
    providers:
      - aescbc:
          keys:
            - name: key1
              secret: Tsg7sO4Ki/W3s9bfwGfTi8ECcp+/3uDedQMq6rLQTIY=
      - identity: {}`,
		"audit-policy-minimal.yaml": `apiVersion: audit.k8s.io/v1
kind: Policy
rules:
  # Do not log from kube-system accounts
  - level: None
    userGroups:
      - system:serviceaccounts:kube-system
  - level: None
    users:
      - system:apiserver
      - system:kube-scheduler
      - system:volume-scheduler
      - system:kube-controller-manager
      - system:node

  # Do not log from collector
  - level: None
    users:
      - system:serviceaccount:collectorforkubernetes:collectorforkubernetes

  # Don't log nodes communications
  - level: None
    userGroups:
      - system:nodes

  # Don't log these read-only URLs.
  - level: None
    nonResourceURLs:
      - /healthz*
      - /version
      - /swagger*

  # Log configmap and secret changes in all namespaces at the metadata level.
  - level: Metadata
    resources:
      - resources: ["secrets", "configmaps"]

  # A catch-all rule to log all other requests at the request level.
  - level: Request`,
	}
}

func plugin(remoteKubeControl *control.Kc, info createInfo, namespace string) {
	err := waitForClusterReady(remoteKubeControl, namespace)
	ns, _ := remoteKubeControl.Namespace().Get(namespace)
	if err == nil {
		remoteAppMarket := appmarket.New(fmt.Sprintf("./kubeconfig/%s.kubeconfig", namespace))

		switch info.Network {
		case "flannel":
			remoteAppMarket.Chart().Install("kube-system", "flannel", "flannel", false, info.NetworkVersion, map[string]interface{}{
				"subNet": info.PodCidr,
			})
		case "calico":
			remoteAppMarket.Chart().Install("kube-system", "calico", "calico", false, info.NetworkVersion, map[string]interface{}{
				"subNet": info.PodCidr,
			})
		case "canal":
			remoteAppMarket.Chart().Install("kube-system", "canal", "canal", false, info.NetworkVersion, map[string]interface{}{
				"subNet": info.PodCidr,
			})
		case "antrea":
			remoteAppMarket.Chart().Install("kube-system", "antrea", "antrea", false, info.NetworkVersion, map[string]interface{}{
				"subNet": info.PodCidr,
			})
		case "none":
			fmt.Println("network plugin is none, skip..")
		}

		remoteAppMarket.Chart().Install("kube-system", "coredns", "coredns", false, ns.Labels["CoreDNS"], map[string]interface{}{
			"replicaCount": 1,
			"clusterIP":    info.DnsSvc,
		})
		remoteAppMarket.Chart().Install("kube-system", "kube-state-metrics", "kube-state-metrics", false, ns.Labels["kube-state-metrics"], map[string]interface{}{
			"replicaCount": 1,
		})

		localAppMarket := appmarket.New("")
		localAppMarket.Chart().Install(namespace, fmt.Sprintf("event-exporter-%s", namespace), "event-exporter", false, "1.7.0", map[string]interface{}{
			"clusterName": ns.Labels["project"],
			"clusterEnv":  ns.Labels["env"],
			"stdout": map[string]interface{}{
				"elasticsearch": map[string]interface{}{
					"hosts":       []string{viper.GetString("ELASTICSEARCH_URL")},
					"index":       "devops-kube-event",
					"indexFormat": "devops-kube-event-{2006.01.02}",
				},
			},
		})
	}
}

func waitForClusterReady(kubeControl *control.Kc, namespace string) (err error) {
	timeout := time.After(time.Minute * 15)
	finish := make(chan bool)
	count := 1
	go func() {
		for {
			select {
			case <-timeout:
				err = errors.New("wait timeout..")
				finish <- true
				return
			default:
				scheduler, _ := kubeControl.Deployment().Get(namespace, "kube-scheduler")
				controllerMgr, _ := kubeControl.Deployment().Get(namespace, "kube-controller-manager")
				if scheduler.Status.ReadyReplicas >= 1 && controllerMgr.Status.ReadyReplicas >= 1 {
					err = nil
					finish <- true
					return
				}
				count++
			}
			time.Sleep(time.Second * 1)
		}
	}()
	<-finish
	return err
}

func ClusterMonitor(c *gin.Context) {
	var token string
	name := c.Query("name")

	localkubeControl := control.New("")
	remoteKubeControl := control.New(fmt.Sprintf("./kubeconfig/%s.kubeconfig", name))

	ns, err := localkubeControl.Namespace().Get(name)

	remoteAppMarket := appmarket.New(fmt.Sprintf("./kubeconfig/%s.kubeconfig", name))
	remoteAppMarket.Chart().Install("kube-system", "prometheus", "prometheus", false, "1.0.0", map[string]interface{}{
		"replicaCount": 1,
		"remoteWrite":  viper.GetString("PROMETHEUS_URL"),
		"clusterName":  ns.Labels["project"],
		"clusterEnv":   ns.Labels["env"],
	})

	sa, err := remoteKubeControl.ServiceAccount().Get("kube-system", "prometheus")
	if err != nil {
		panic(err)
	}

	if len(sa.Secrets) == 0 {
		t, err := remoteKubeControl.ServiceAccount().CreateToken("kube-system", "prometheus", int64(315360000))
		if err != nil {
			panic(err)
		}
		token = t.Status.Token
	} else {
		sc, err := remoteKubeControl.Secrets().Get("kube-system", sa.Secrets[0].Name)
		if err != nil {
			panic(err)
		}
		token = string(sc.Data["token"])
	}

	prometheusSA, err := localkubeControl.ServiceAccount().Apply(name, "prometheus")
	if err != nil {
		panic(err)
	}

	cr, err := localkubeControl.ClusterRoles().Apply(fmt.Sprintf("application:control-plane:%s:prometheus", name), []applyrbacv1.PolicyRuleApplyConfiguration{
		{
			Verbs: []string{
				"get",
				"list",
				"watch",
			},
			APIGroups: []string{
				"",
			},
			Resources: []string{
				"endpoints",
				"pods",
				"nodes",
				"services",
			},
		},
		{
			Verbs: []string{
				"get",
			},
			NonResourceURLs: []string{
				"/metrics",
			},
		},
	})
	if err != nil {
		panic(err)
	}

	_, err = localkubeControl.ClusterRoleBindings().Apply(fmt.Sprintf("application:control-plane:%s:prometheus", name), []applyrbacv1.SubjectApplyConfiguration{
		{
			Kind: func() *string {
				name := "ServiceAccount"
				return &name
			}(),
			Name:      &prometheusSA.Name,
			Namespace: &name,
		},
	}, &applyrbacv1.RoleRefApplyConfiguration{
		APIGroup: func() *string {
			name := "rbac.authorization.k8s.io"
			return &name
		}(),
		Kind: func() *string {
			name := "ClusterRole"
			return &name
		}(),
		Name: &cr.Name,
	})
	if err != nil {
		panic(err)
	}

	type info struct {
		Token         string
		PrometheusUrl string
		ClusterName   string
		ClusterEnv    string
	}
	tmpl, err := template.ParseFiles("./templates/prometheus.yml")
	if err != nil {
		fmt.Println("create template failed, err:", err)
		return
	}
	buf := new(bytes.Buffer)
	err = tmpl.Execute(buf, info{
		token,
		viper.GetString("PROMETHEUS_URL"),
		ns.Labels["project"],
		ns.Labels["env"],
	})
	if err != nil {
		panic(err)
	}

	cm, _ := localkubeControl.ConfigMaps().Apply(name, "prometheus", map[string]string{
		"prometheus.yml": buf.String(),
	})
	localkubeControl.Deployment().Apply(name, "prometheus", &applyappsv1.DeploymentSpecApplyConfiguration{
		Replicas: func() *int32 {
			mode := int32(1)
			return &mode
		}(),
		Strategy: &applyappsv1.DeploymentStrategyApplyConfiguration{
			Type: func() *v1.DeploymentStrategyType {
				x := v1.RecreateDeploymentStrategyType
				return &x
			}(),
		},
		Selector: &applymetav1.LabelSelectorApplyConfiguration{
			MatchLabels: map[string]string{
				"app": "prometheus",
			},
		},
		Template: &applycorev1.PodTemplateSpecApplyConfiguration{
			ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
				Name: func() *string {
					a := "prometheus"
					return &a
				}(),
				Namespace: &name,
				Labels: map[string]string{
					"app": "prometheus",
				},
			},
			Spec: &applycorev1.PodSpecApplyConfiguration{
				Volumes: []applycorev1.VolumeApplyConfiguration{

					{
						Name: func() *string {
							a := "prometheus"
							return &a
						}(),
						VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
							ConfigMap: &applycorev1.ConfigMapVolumeSourceApplyConfiguration{
								LocalObjectReferenceApplyConfiguration: applycorev1.LocalObjectReferenceApplyConfiguration{
									Name: &cm.Name,
								},
								DefaultMode: func() *int32 {
									a := int32(0755)
									return &a
								}(),
							},
						},
					},
				},
				ServiceAccountName: &prometheusSA.Name,
				Containers: []applycorev1.ContainerApplyConfiguration{
					{
						Name: func() *string {
							a := "prometheus"
							return &a
						}(),
						Image: func() *string {
							a := "prom/prometheus:v2.53.1"
							return &a
						}(),
						ImagePullPolicy: func() *corev1.PullPolicy {
							a := corev1.PullIfNotPresent
							return &a
						}(),
						Ports: []applycorev1.ContainerPortApplyConfiguration{
							{
								Name: func() *string {
									a := "http"
									return &a
								}(),
								ContainerPort: func() *int32 {
									a := int32(9090)
									return &a
								}(),
								Protocol: func() *corev1.Protocol {
									a := corev1.ProtocolTCP
									return &a
								}(),
							},
						},
						Resources: &applycorev1.ResourceRequirementsApplyConfiguration{
							Limits: nil,
							Requests: &corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("5m"),
								corev1.ResourceMemory: resource.MustParse("100Mi"),
							},
						},
						VolumeMounts: []applycorev1.VolumeMountApplyConfiguration{
							{
								Name: &cm.Name,
								MountPath: func() *string {
									a := "/etc/prometheus/prometheus.yml"
									return &a
								}(),
								SubPath: func() *string {
									a := "prometheus.yml"
									return &a
								}(),
							},
						},
						LivenessProbe: &applycorev1.ProbeApplyConfiguration{
							ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
								TCPSocket: &applycorev1.TCPSocketActionApplyConfiguration{
									Port: &intstr.IntOrString{
										Type:   0,
										IntVal: 9090,
										StrVal: "9090",
									},
								},
							},
							InitialDelaySeconds: func() *int32 {
								a := int32(10)
								return &a
							}(),
							TimeoutSeconds: func() *int32 {
								a := int32(15)
								return &a
							}(),
							PeriodSeconds: func() *int32 {
								a := int32(10)
								return &a
							}(),
							FailureThreshold: func() *int32 {
								a := int32(8)
								return &a
							}(),
						},
						ReadinessProbe: &applycorev1.ProbeApplyConfiguration{
							ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
								TCPSocket: &applycorev1.TCPSocketActionApplyConfiguration{
									Port: &intstr.IntOrString{
										Type:   0,
										IntVal: 9090,
										StrVal: "9090",
									},
								},
							},
							InitialDelaySeconds: func() *int32 {
								a := int32(5)
								return &a
							}(),
							TimeoutSeconds: func() *int32 {
								a := int32(15)
								return &a
							}(),
							PeriodSeconds: func() *int32 {
								a := int32(1)
								return &a
							}(),
							FailureThreshold: func() *int32 {
								a := int32(3)
								return &a
							}(),
						},
					},
				},
			},
		},
	})
	localkubeControl.Service().Apply(name, "prometheus", &applycorev1.ServiceSpecApplyConfiguration{
		Ports: []applycorev1.ServicePortApplyConfiguration{
			{
				Name: func() *string {
					v := "http"
					return &v
				}(),
				Port: func() *int32 {
					v := int32(9090)
					return &v
				}(),
				TargetPort: &intstr.IntOrString{
					Type:   0,
					IntVal: 9090,
					StrVal: "9090",
				},
			},
		},
		Selector: map[string]string{
			"app": "prometheus",
		},
		Type: func() *corev1.ServiceType {
			v := corev1.ServiceTypeClusterIP
			return &v
		}(),
	})

	// patch namespace set enable prometheus
	patchBytes, _ := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]string{
				"prometheus": "true",
			},
		},
	})
	localkubeControl.Namespace().Patch(name, types.MergePatchType, patchBytes)
	c.JSON(http.StatusOK, gin.H{})
}

func ClusterStatus(c *gin.Context) {
	name := c.Query("name")

	var networkName string

	l := control.New("")
	ns, _ := l.Namespace().Get(name)

	switch ns.Labels["network"] {
	case "flannel":
		networkName = "kube-flannel-ds"
	case "calico":
		networkName = "calico-node"
	case "canal":
		networkName = "canal"
	case "antrea":
		networkName = "antrea-agent"
	}

	remoteKubeControl := control.New(fmt.Sprintf("./kubeconfig/%s.kubeconfig", name))
	cd, _ := remoteKubeControl.Deployment().Get("kube-system", "coredns")
	ms, _ := remoteKubeControl.Deployment().Get("kube-system", "metrics-server")
	nw, _ := remoteKubeControl.DaemonSets().Get("kube-system", networkName)

	c.JSON(http.StatusOK, gin.H{
		"coredns":       fmt.Sprintf("%d/%d", cd.Status.AvailableReplicas, cd.Status.Replicas),
		"metricsServer": fmt.Sprintf("%d/%d", ms.Status.AvailableReplicas, ms.Status.Replicas),
		"network": map[string]string{
			"name":   ns.Labels["network"],
			"status": fmt.Sprintf("%d/%d", nw.Status.CurrentNumberScheduled, nw.Status.DesiredNumberScheduled),
		},
	})
}

func ClusterPages(c *gin.Context) {
	var instance []map[string]interface{}
	//k := control_bak.New("")

	kubeControl := control.New("")

	ins, err := kubeControl.Namespace().List(metav1.ListOptions{
		LabelSelector: "fieldManager=control-plane",
	})
	//ins, err := k.NamespaceList()
	if err != nil {
		panic(err)
	}

	for _, ns := range ins.Items {
		components := map[string]string{}
		api, _ := kubeControl.Deployment().Get(ns.Name, "kube-apiserver")
		//api, _ := k.GetDeployment(ns.Name, "kube-apiserver")
		if api.Status.ReadyReplicas >= 1 {
			components["Apiserver"] = "Running"
		} else {
			components["Apiserver"] = "Init"
		}

		scheduler, _ := kubeControl.Deployment().Get(ns.Name, "kube-scheduler")
		//scheduler, _ := k.GetDeployment(ns.Name, "kube-scheduler")
		if scheduler.Status.ReadyReplicas >= 1 {
			components["Scheduler"] = "Running"
		} else {
			components["Scheduler"] = "Init"
		}

		controller, _ := kubeControl.Deployment().Get(ns.Name, "kube-controller-manager")
		//controller, _ := k.GetDeployment(ns.Name, "kube-controller-manager")
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

	var v db.Version
	versions, _ := v.SelectAll()

	c.HTML(http.StatusOK, "cluster.html", gin.H{
		"items":    versions,
		"instance": instance,
	})
}

type createInfo struct {
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

func ClusterCreate(c *gin.Context) {
	var (
		info   createInfo
		lbAddr string
		v      db.Version
	)

	if err := c.BindJSON(&info); err != nil {
		panic("bind: " + err.Error())
	}

	vinfo, err := v.Select(info.Version)
	clusterDNS, _ := utils.GetCidrIpRange(info.ServiceCidr)

	namespace := fmt.Sprintf("%s-%s", info.Project, info.Env)

	kubeControl := control.New("")
	ns, err := kubeControl.Namespace().Apply(&applymetav1.ObjectMetaApplyConfiguration{
		Name: &namespace,
		Labels: map[string]string{
			"project":            info.Project,
			"env":                info.Env,
			"kubernetes":         vinfo.Kubernetes,
			"etcd":               vinfo.Etcd,
			"containerd":         vinfo.Containerd,
			"runc":               vinfo.Runc,
			"registry":           info.Registry,
			"nodePort":           info.NodePort,
			"serviceSubnet":      strings.Replace(info.ServiceCidr, "/", "-", 1),
			"podSubnet":          strings.Replace(info.PodCidr, "/", "-", 1),
			"network":            info.Network,
			"fieldManager":       "control-plane",
			"networkVersion":     info.NetworkVersion,
			"pause":              vinfo.Pause,
			"clusterDNS":         clusterDNS,
			"CoreDNS":            vinfo.Coredns,
			"metrics-server":     vinfo.MetricsServer,
			"kube-state-metrics": vinfo.KubeStateMetrics,
		},
	})
	if err != nil {
		panic(err)
	}

	// kube-apiserver svc
	apiSvc, err := kubeControl.Service().Apply(ns.Name, "kube-apiserver", &applycorev1.ServiceSpecApplyConfiguration{
		Ports: []applycorev1.ServicePortApplyConfiguration{
			{
				Name: func() *string {
					mode := "https"
					return &mode
				}(),
				Port: func() *int32 {
					mode := int32(6443)
					return &mode
				}(),
				TargetPort: &intstr.IntOrString{
					Type:   0,
					IntVal: 6443,
					StrVal: "6443",
				},
			},
		},
		Selector: map[string]string{
			"app": "kube-apiserver",
		},
		Type: func() *corev1.ServiceType {
			name := corev1.ServiceTypeLoadBalancer
			return &name
		}(),
	})
	if err != nil {
		panic(err)
	}

	timeout := time.After(time.Minute * 2)
	finish := make(chan bool)
	count := 1
	go func() {
		for {
			select {
			case <-timeout:
				kubeControl.Service().Delete(namespace, apiSvc.Name)
				kubeControl.Namespace().Delete(ns.Name)
				fmt.Println("wait timeout, delete service")
				err = errors.New("wait timeout")
				finish <- true
				return
			default:
				svc, err := kubeControl.Service().Get(namespace, apiSvc.Name)
				if err != nil {
					panic(err.Error())
				}
				if len(svc.Status.LoadBalancer.Ingress) > 0 {
					lbAddr = svc.Status.LoadBalancer.Ingress[0].IP
					log.Printf("Service external IP is: %s\n", svc.Status.LoadBalancer.Ingress[0].IP)
					finish <- true
					return
				}
				log.Println("Waiting for external IP...")
				count++
			}
			time.Sleep(time.Second * 1)
		}
	}()

	<-finish

	// update loadBalancer to namespace labels
	patchBytes, _ := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]string{
				"loadBalancer": lbAddr,
			},
		},
	})
	kubeControl.Namespace().Patch(ns.Name, types.MergePatchType, patchBytes)

	// ca configmap
	var clusterCa *corev1.ConfigMap
	clusterCa, err = kubeControl.ConfigMaps().Get(ns.Name, "cluster-ca")
	if err != nil {
		pki := cert.New()
		tl := pki.GenerateAll(10, info.Project, info.Env, lbAddr, clusterDNS)
		clusterCa, err = kubeControl.ConfigMaps().Apply(ns.Name, "cluster-ca", map[string]string{
			"sa.pub":                       tl.SaPub,
			"sa.key":                       tl.SaKey,
			"ca.crt":                       tl.CaCrt,
			"ca.key":                       tl.CaKey,
			"apiserver.crt":                tl.ApiServerCrt,
			"apiserver.key":                tl.ApiServerKey,
			"apiserver-kubelet-client.crt": tl.ApiserverKubeletClientCrt,
			"apiserver-kubelet-client.key": tl.ApiserverKubeletClientKey,
			"apiserver-etcd-client.crt":    tl.ApiserverEtcdClientCrt,
			"apiserver-etcd-client.key":    tl.ApiserverEtcdClientKey,
			"kube-controller-manager.crt":  tl.ControllerManagerCrt,
			"kube-controller-manager.key":  tl.ControllerManagerKey,
			"kube-scheduler.crt":           tl.SchedulerCrt,
			"kube-scheduler.key":           tl.SchedulerKey,
			"admin.crt":                    tl.AdminCrt,
			"admin.key":                    tl.AdminKey,
			"etcd.crt":                     tl.EtcdCrt,
			"etcd.key":                     tl.EtcdKey,
			"etcd-server.crt":              tl.EtcdServerCrt,
			"etcd-server.key":              tl.EtcdServerKey,
			"etcd-peer.crt":                tl.EtcdPeerCrt,
			"etcd-peer.key":                tl.EtcdPeerKey,
			"etcd-healthcheck-client.crt":  tl.EtcdHealthcheckClientCrt,
			"etcd-healthcheck-client.key":  tl.EtcdHealthcheckClientKey,
			"front-proxy-ca.crt":           tl.FrontProxyCrt,
			"front-proxy-ca.key":           tl.FrontProxyKey,
			"front-proxy-client.crt":       tl.FrontProxyClientCrt,
			"front-proxy-client.key":       tl.FrontProxyClientKey,
		})
		if err != nil {
			panic(err)
		}
	}

	kubeconfig, err := cert.CreateKubeconfigFileForRestConfig(rest.Config{
		Host: fmt.Sprintf("https://%s:6443", lbAddr),
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: false,
			CAData:   []byte(clusterCa.Data["ca.crt"]),
			CertData: []byte(clusterCa.Data["admin.crt"]),
			KeyData:  []byte(clusterCa.Data["admin.key"]),
		},
	}, fmt.Sprintf("./kubeconfig/%s-%s.kubeconfig", info.Project, info.Env))

	kubeControl.ConfigMaps().Apply(ns.Name, "remote-access", map[string]string{
		"remote-access.kubeconfig": string(kubeconfig),
	})

	volumeMount := []applycorev1.VolumeMountApplyConfiguration{
		// sa pub
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/sa.pub"
				return &v
			}(),
			SubPath: func() *string {
				v := "sa.pub"
				return &v
			}(),
		},
		// sa key
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/sa.key"
				return &v
			}(),
			SubPath: func() *string {
				v := "sa.key"
				return &v
			}(),
		},
		// k8s ca crt
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/ca.crt"
				return &v
			}(),
			SubPath: func() *string {
				v := "ca.crt"
				return &v
			}(),
		},
		// k8s ca key
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/ca.key"
				return &v
			}(),
			SubPath: func() *string {
				v := "ca.key"
				return &v
			}(),
		},

		// k8s apiserver crt
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/apiserver.crt"
				return &v
			}(),
			SubPath: func() *string {
				v := "apiserver.crt"
				return &v
			}(),
		},
		// k8s apiserver key
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/apiserver.key"
				return &v
			}(),
			SubPath: func() *string {
				v := "apiserver.key"
				return &v
			}(),
		},

		// k8s apiserver-kubelet-client crt
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/apiserver-kubelet-client.crt"
				return &v
			}(),
			SubPath: func() *string {
				v := "apiserver-kubelet-client.crt"
				return &v
			}(),
		},
		// k8s apiserver-kubelet-client key
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/apiserver-kubelet-client.key"
				return &v
			}(),
			SubPath: func() *string {
				v := "apiserver-kubelet-client.key"
				return &v
			}(),
		},

		// k8s apiserver-etcd-client crt
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/apiserver-etcd-client.crt"
				return &v
			}(),
			SubPath: func() *string {
				v := "apiserver-etcd-client.crt"
				return &v
			}(),
		},
		// k8s apiserver-etcd-client key
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/apiserver-etcd-client.key"
				return &v
			}(),
			SubPath: func() *string {
				v := "apiserver-etcd-client.key"
				return &v
			}(),
		},

		// k8s kube-controller-manager crt
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/kube-controller-manager.crt"
				return &v
			}(),
			SubPath: func() *string {
				v := "kube-controller-manager.crt"
				return &v
			}(),
		},
		// k8s kube-controller-manager key
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/kube-controller-manager.key"
				return &v
			}(),
			SubPath: func() *string {
				v := "kube-controller-manager.key"
				return &v
			}(),
		},

		// k8s kube-scheduler crt
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/kube-scheduler.crt"
				return &v
			}(),
			SubPath: func() *string {
				v := "kube-scheduler.crt"
				return &v
			}(),
		},
		// k8s kube-scheduler key
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/kube-scheduler.key"
				return &v
			}(),
			SubPath: func() *string {
				v := "kube-scheduler.key"
				return &v
			}(),
		},

		// front-proxy ca crt
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/front-proxy-ca.crt"
				return &v
			}(),
			SubPath: func() *string {
				v := "front-proxy-ca.crt"
				return &v
			}(),
		},
		// front-proxy ca key
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/front-proxy-ca.key"
				return &v
			}(),
			SubPath: func() *string {
				v := "front-proxy-ca.key"
				return &v
			}(),
		},
		// front-proxy client crt
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/front-proxy-client.crt"
				return &v
			}(),
			SubPath: func() *string {
				v := "front-proxy-client.crt"
				return &v
			}(),
		},
		// front-proxy client key
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/front-proxy-client.key"
				return &v
			}(),
			SubPath: func() *string {
				v := "front-proxy-client.key"
				return &v
			}(),
		},

		// etcd ca crt
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/etcd/ca.crt"
				return &v
			}(),
			SubPath: func() *string {
				v := "etcd.crt"
				return &v
			}(),
		},
		// etcd ca key
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/etcd/ca.key"
				return &v
			}(),
			SubPath: func() *string {
				v := "etcd.key"
				return &v
			}(),
		},
		// etcd server crt
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/etcd/server.crt"
				return &v
			}(),
			SubPath: func() *string {
				v := "etcd-server.crt"
				return &v
			}(),
		},
		// etcd server key
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/etcd/server.key"
				return &v
			}(),
			SubPath: func() *string {
				v := "etcd-server.key"
				return &v
			}(),
		},
		// etcd peer crt
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/etcd/peer.crt"
				return &v
			}(),
			SubPath: func() *string {
				v := "etcd-peer.crt"
				return &v
			}(),
		},
		// etcd peer key
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/etcd/peer.key"
				return &v
			}(),
			SubPath: func() *string {
				v := "etcd-peer.key"
				return &v
			}(),
		},
		// etcd healthcheck-client crt
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/etcd/healthcheck-client.crt"
				return &v
			}(),
			SubPath: func() *string {
				v := "etcd-healthcheck-client.crt"
				return &v
			}(),
		},
		// etcd healthcheck-client key
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/etcd/healthcheck-client.key"
				return &v
			}(),
			SubPath: func() *string {
				v := "etcd-healthcheck-client.key"
				return &v
			}(),
		},
	}

	// etcd sa
	etcdSA, err := kubeControl.ServiceAccount().Apply(ns.Name, "etcd")
	if err != nil {
		panic(err)
	}

	// etcd role
	etcdRole, err := kubeControl.Roles().Apply(ns.Name, "application:control-plane:etcd", []applyrbacv1.PolicyRuleApplyConfiguration{
		{
			Verbs: []string{
				"patch",
				"get",
				"create",
			},
			APIGroups: []string{
				"",
			},
			Resources: []string{
				"secrets",
				"configmaps",
			},
		},
		{
			Verbs: []string{
				"get",
			},
			APIGroups: []string{
				"apps",
			},
			Resources: []string{
				"statefulsets",
			},
		},
	})
	if err != nil {
		panic(err)
	}

	// etcd rolebinding
	_, err = kubeControl.RoleBindings().Apply(ns.Name, "application:control-plane:etcd", []applyrbacv1.SubjectApplyConfiguration{{
		Kind: func() *string {
			name := "ServiceAccount"
			return &name
		}(),
		Name:      &etcdSA.Name,
		Namespace: &namespace,
	}}, &applyrbacv1.RoleRefApplyConfiguration{
		APIGroup: func() *string {
			name := "rbac.authorization.k8s.io"
			return &name
		}(),
		Kind: func() *string {
			name := "Role"
			return &name
		}(),
		Name: &etcdRole.Name,
	})
	if err != nil {
		panic(err)
	}

	// etcd svc
	etcdSvc, err := kubeControl.Service().Apply(ns.Name, "etcd", &applycorev1.ServiceSpecApplyConfiguration{
		ClusterIP: nil,
		PublishNotReadyAddresses: func() *bool {
			v := true
			return &v
		}(),
		Selector: map[string]string{
			"app": "etcd",
		},
		Ports: []applycorev1.ServicePortApplyConfiguration{
			{
				Name: func() *string {
					mode := "etcd-client"
					return &mode
				}(),
				Port: func() *int32 {
					mode := int32(2379)
					return &mode
				}(),
				TargetPort: &intstr.IntOrString{
					Type:   0,
					IntVal: 2379,
					StrVal: "2379",
				},
			},
			{
				Name: func() *string {
					mode := "etcd-member"
					return &mode
				}(),
				Port: func() *int32 {
					mode := int32(2380)
					return &mode
				}(),
				TargetPort: &intstr.IntOrString{
					Type:   0,
					IntVal: 2380,
					StrVal: "2380",
				},
			},
			{
				Name: func() *string {
					mode := "etcd-metrics"
					return &mode
				}(),
				Port: func() *int32 {
					mode := int32(2381)
					return &mode
				}(),
				TargetPort: &intstr.IntOrString{
					Type:   0,
					IntVal: 2381,
					StrVal: "2381",
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	// etcd sts
	etcdvolumeMount := append(volumeMount, applycorev1.VolumeMountApplyConfiguration{
		Name: func() *string {
			name := "etcd-vol"
			return &name
		}(),
		MountPath: func() *string {
			name := "/var/lib/etcd"
			return &name
		}(),
		SubPath: func() *string {
			name := "data"
			return &name
		}(),
	})
	etcdvolumeMount = append(volumeMount, applycorev1.VolumeMountApplyConfiguration{
		Name: func() *string {
			v := "etcd-vol"
			return &v
		}(),
		MountPath: func() *string {
			v := "/var/lib/cache"
			return &v
		}(),
		SubPath: func() *string {
			v := "cache"
			return &v
		}(),
	})
	_, err = kubeControl.StatefulSets().Apply(ns.Name, "etcd", &applyappsv1.StatefulSetSpecApplyConfiguration{
		ServiceName: &etcdSvc.Name,
		Replicas: func() *int32 {
			i := int32(1)
			return &i
		}(),
		Selector: &applymetav1.LabelSelectorApplyConfiguration{
			MatchLabels: map[string]string{
				"app": "etcd",
			},
		},
		Template: &applycorev1.PodTemplateSpecApplyConfiguration{
			ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
				Name: func() *string {
					name := "etcd"
					return &name
				}(),
				Namespace: &ns.Name,
				Labels: map[string]string{
					"app":     "etcd",
					"project": ns.Labels["project"],
					"env":     ns.Labels["env"],
				},
			},
			Spec: &applycorev1.PodSpecApplyConfiguration{
				ServiceAccountName: &etcdSA.Name,
				Tolerations: []applycorev1.TolerationApplyConfiguration{
					{
						Key: func() *string {
							v := "node.kubernetes.io/not-ready"
							return &v
						}(),
						Operator: func() *corev1.TolerationOperator {
							v := corev1.TolerationOpExists
							return &v
						}(),
						Effect: func() *corev1.TaintEffect {
							v := corev1.TaintEffectNoExecute
							return &v
						}(),
						TolerationSeconds: func() *int64 {
							v := int64(1)
							return &v
						}(),
					},
					{
						Key: func() *string {
							v := "node.kubernetes.io/unreachable"
							return &v
						}(),
						Operator: func() *corev1.TolerationOperator {
							v := corev1.TolerationOpExists
							return &v
						}(),
						Effect: func() *corev1.TaintEffect {
							v := corev1.TaintEffectNoExecute
							return &v
						}(),
						TolerationSeconds: func() *int64 {
							v := int64(1)
							return &v
						}(),
					},
				},
				Volumes: []applycorev1.VolumeApplyConfiguration{
					// cluster ca configmap
					{
						Name: &clusterCa.Name,
						VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
							ConfigMap: &applycorev1.ConfigMapVolumeSourceApplyConfiguration{
								LocalObjectReferenceApplyConfiguration: applycorev1.LocalObjectReferenceApplyConfiguration{
									Name: &clusterCa.Name,
								},
								DefaultMode: func() *int32 {
									v := int32(0755)
									return &v
								}(),
							},
						},
					},
				},
				Affinity: &applycorev1.AffinityApplyConfiguration{
					PodAntiAffinity: &applycorev1.PodAntiAffinityApplyConfiguration{
						PreferredDuringSchedulingIgnoredDuringExecution: []applycorev1.WeightedPodAffinityTermApplyConfiguration{
							{
								Weight: func() *int32 {
									v := int32(100)
									return &v
								}(),
								PodAffinityTerm: &applycorev1.PodAffinityTermApplyConfiguration{
									LabelSelector: &applymetav1.LabelSelectorApplyConfiguration{
										MatchExpressions: []applymetav1.LabelSelectorRequirementApplyConfiguration{
											{
												Key: func() *string {
													v := "app"
													return &v
												}(),
												Operator: func() *metav1.LabelSelectorOperator {
													v := metav1.LabelSelectorOpIn
													return &v
												}(),
												Values: []string{
													"etcd",
													"kube-apiserver",
													"kube-controller-manager",
													"kube-scheduler",
												},
											},
										},
									},
									Namespaces: []string{namespace},
									TopologyKey: func() *string {
										v := "kubernetes.io/hostname"
										return &v
									}(),
								},
							},
						},
						RequiredDuringSchedulingIgnoredDuringExecution: []applycorev1.PodAffinityTermApplyConfiguration{
							{
								LabelSelector: &applymetav1.LabelSelectorApplyConfiguration{
									MatchExpressions: []applymetav1.LabelSelectorRequirementApplyConfiguration{
										{
											Key: func() *string {
												v := "app"
												return &v
											}(),
											Operator: func() *metav1.LabelSelectorOperator {
												v := metav1.LabelSelectorOpIn
												return &v
											}(),
											Values: []string{
												"etcd",
											},
										},
									},
								},
								Namespaces: []string{
									namespace,
								},
								TopologyKey: func() *string {
									v := "kubernetes.io/hostname"
									return &v
								}(),
							},
						},
					},
				},
				Containers: []applycorev1.ContainerApplyConfiguration{
					{
						Name: func() *string {
							name := "etcd"
							return &name
						}(),
						Image: func() *string {
							a := fmt.Sprintf("buxiaomo/etcd:%s", ns.Labels["etcd"])
							return &a
						}(),
						ImagePullPolicy: func() *corev1.PullPolicy {
							v := corev1.PullAlways
							return &v
						}(),
						Ports: []applycorev1.ContainerPortApplyConfiguration{
							{
								Name: func() *string {
									a := "etcd-client"
									return &a
								}(),
								ContainerPort: func() *int32 {
									a := int32(2379)
									return &a
								}(),
								Protocol: func() *corev1.Protocol {
									a := corev1.ProtocolTCP
									return &a
								}(),
							},
							{
								Name: func() *string {
									a := "etcd-member"
									return &a
								}(),
								ContainerPort: func() *int32 {
									a := int32(2380)
									return &a
								}(),
								Protocol: func() *corev1.Protocol {
									a := corev1.ProtocolTCP
									return &a
								}(),
							},
							{
								Name: func() *string {
									a := "etcd-metrics"
									return &a
								}(),
								ContainerPort: func() *int32 {
									a := int32(2381)
									return &a
								}(),
								Protocol: func() *corev1.Protocol {
									a := corev1.ProtocolTCP
									return &a
								}(),
							},
						},
						Resources: &applycorev1.ResourceRequirementsApplyConfiguration{
							Limits: &corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("230m"),
								corev1.ResourceMemory: resource.MustParse("700Mi"),
							},
							Requests: &corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("25m"),
								corev1.ResourceMemory: resource.MustParse("70Mi"),
							},
						},
						VolumeMounts: etcdvolumeMount,
						Env: []applycorev1.EnvVarApplyConfiguration{
							{
								Name: func() *string {
									v := "NAMESPACE"
									return &v
								}(),
								ValueFrom: &applycorev1.EnvVarSourceApplyConfiguration{
									FieldRef: &applycorev1.ObjectFieldSelectorApplyConfiguration{
										FieldPath: func() *string {
											v := "metadata.namespace"
											return &v
										}(),
									},
								},
							},
						},
						LivenessProbe: &applycorev1.ProbeApplyConfiguration{
							ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
								HTTPGet: &applycorev1.HTTPGetActionApplyConfiguration{
									Path: func() *string {
										a := "/health"
										return &a
									}(),
									Port: &intstr.IntOrString{
										Type:   0,
										IntVal: 2381,
										StrVal: "2381",
									},
									Scheme: func() *corev1.URIScheme {
										a := corev1.URISchemeHTTP
										return &a
									}(),
								},
							},
							InitialDelaySeconds: func() *int32 {
								a := int32(10)
								return &a
							}(),
							TimeoutSeconds: func() *int32 {
								a := int32(15)
								return &a
							}(),
							PeriodSeconds: func() *int32 {
								a := int32(10)
								return &a
							}(),
							FailureThreshold: func() *int32 {
								a := int32(8)
								return &a
							}(),
						},
						ReadinessProbe: &applycorev1.ProbeApplyConfiguration{
							ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
								HTTPGet: &applycorev1.HTTPGetActionApplyConfiguration{
									Path: func() *string {
										a := "/health"
										return &a
									}(),
									Port: &intstr.IntOrString{
										Type:   0,
										IntVal: 2381,
										StrVal: "2381",
									},
									Scheme: func() *corev1.URIScheme {
										a := corev1.URISchemeHTTP
										return &a
									}(),
								},
							},
							InitialDelaySeconds: func() *int32 {
								a := int32(5)
								return &a
							}(),
							TimeoutSeconds: func() *int32 {
								a := int32(5)
								return &a
							}(),
							PeriodSeconds: func() *int32 {
								a := int32(5)
								return &a
							}(),
						},
						Lifecycle: &applycorev1.LifecycleApplyConfiguration{
							PreStop: &applycorev1.LifecycleHandlerApplyConfiguration{
								Exec: &applycorev1.ExecActionApplyConfiguration{
									Command: []string{
										"/bin/sh",
										"-cx",
										"/usr/local/bin/prestop.sh",
									},
								},
							},
						},
						Command: []string{
							"/bin/sh",
							"-ecx",
							"/usr/local/bin/entrypoint.sh",
						},
					},
				},
			},
		},
		VolumeClaimTemplates: []applycorev1.PersistentVolumeClaimApplyConfiguration{
			{
				TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{},
				ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
					Name: func() *string {
						name := "etcd-vol"
						return &name
					}(),
				},
				Spec: &applycorev1.PersistentVolumeClaimSpecApplyConfiguration{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: &applycorev1.VolumeResourceRequirementsApplyConfiguration{
						Requests: &corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("5Gi"),
						},
					},
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	// kube-apiserver cm
	// todo add
	kubeApiserverCM, err := kubeControl.ConfigMaps().Apply(ns.Name, "kube-apiserver", kubeApiserverCfg())
	if err != nil {
		panic(err)
	}

	// kube-apiserver deployment
	apiservervolumeMount := append(volumeMount, applycorev1.VolumeMountApplyConfiguration{
		Name: &kubeApiserverCM.Name,
		MountPath: func() *string {
			v := "/etc/kubernetes/encryption-config.yaml"
			return &v
		}(),
		SubPath: func() *string {
			v := "encryption-config.yaml"
			return &v
		}(),
	})
	apiservervolumeMount = append(apiservervolumeMount, applycorev1.VolumeMountApplyConfiguration{
		Name: &kubeApiserverCM.Name,
		MountPath: func() *string {
			v := "/etc/kubernetes/audit-policy-minimal.yaml"
			return &v
		}(),
		SubPath: func() *string {
			v := "audit-policy-minimal.yaml"
			return &v
		}(),
	})

	_, err = kubeControl.Deployment().Apply(ns.Name, "kube-apiserver", &applyappsv1.DeploymentSpecApplyConfiguration{
		Replicas: func() *int32 {
			mode := int32(1)
			return &mode
		}(),
		Selector: &applymetav1.LabelSelectorApplyConfiguration{
			MatchLabels: map[string]string{
				"app": "kube-apiserver",
			},
		},
		Template: &applycorev1.PodTemplateSpecApplyConfiguration{
			ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
				Name: func() *string {
					a := "kube-apiserver"
					return &a
				}(),
				Namespace: &namespace,
				Labels: map[string]string{
					"app":     "kube-apiserver",
					"project": ns.Labels["project"],
					"env":     ns.Labels["env"],
				},
			},
			Spec: &applycorev1.PodSpecApplyConfiguration{
				Tolerations: []applycorev1.TolerationApplyConfiguration{
					{
						Key: func() *string {
							v := "node.kubernetes.io/not-ready"
							return &v
						}(),
						Operator: func() *corev1.TolerationOperator {
							v := corev1.TolerationOpExists
							return &v
						}(),
						Effect: func() *corev1.TaintEffect {
							v := corev1.TaintEffectNoExecute
							return &v
						}(),
						TolerationSeconds: func() *int64 {
							v := int64(1)
							return &v
						}(),
					},
					{
						Key: func() *string {
							v := "node.kubernetes.io/unreachable"
							return &v
						}(),
						Operator: func() *corev1.TolerationOperator {
							v := corev1.TolerationOpExists
							return &v
						}(),
						Effect: func() *corev1.TaintEffect {
							v := corev1.TaintEffectNoExecute
							return &v
						}(),
						TolerationSeconds: func() *int64 {
							v := int64(1)
							return &v
						}(),
					},
				},
				Affinity: &applycorev1.AffinityApplyConfiguration{
					PodAntiAffinity: &applycorev1.PodAntiAffinityApplyConfiguration{
						PreferredDuringSchedulingIgnoredDuringExecution: []applycorev1.WeightedPodAffinityTermApplyConfiguration{
							{
								Weight: func() *int32 {
									v := int32(100)
									return &v
								}(),
								PodAffinityTerm: &applycorev1.PodAffinityTermApplyConfiguration{
									LabelSelector: &applymetav1.LabelSelectorApplyConfiguration{
										MatchExpressions: []applymetav1.LabelSelectorRequirementApplyConfiguration{
											{
												Key: func() *string {
													v := "app"
													return &v
												}(),
												Operator: func() *metav1.LabelSelectorOperator {
													v := metav1.LabelSelectorOpIn
													return &v
												}(),
												Values: []string{
													"etcd",
													"kube-apiserver",
													"kube-controller-manager",
													"kube-scheduler",
												},
											},
										},
									},
									Namespaces: []string{namespace},
									TopologyKey: func() *string {
										v := "kubernetes.io/hostname"
										return &v
									}(),
								},
							},
						},
						RequiredDuringSchedulingIgnoredDuringExecution: []applycorev1.PodAffinityTermApplyConfiguration{
							{
								LabelSelector: &applymetav1.LabelSelectorApplyConfiguration{
									MatchExpressions: []applymetav1.LabelSelectorRequirementApplyConfiguration{
										{
											Key: func() *string {
												v := "app"
												return &v
											}(),
											Operator: func() *metav1.LabelSelectorOperator {
												v := metav1.LabelSelectorOpIn
												return &v
											}(),
											Values: []string{
												"kube-apiserver",
											},
										},
									},
								},
								Namespaces: []string{
									namespace,
								},
								TopologyKey: func() *string {
									v := "kubernetes.io/hostname"
									return &v
								}(),
							},
						},
					},
				},
				Volumes: []applycorev1.VolumeApplyConfiguration{
					{
						Name: func() *string {
							a := "kube-apiserver"
							return &a
						}(),
						VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
							ConfigMap: &applycorev1.ConfigMapVolumeSourceApplyConfiguration{
								LocalObjectReferenceApplyConfiguration: applycorev1.LocalObjectReferenceApplyConfiguration{
									Name: &kubeApiserverCM.Name,
								},
								DefaultMode: func() *int32 {
									a := int32(0755)
									return &a
								}(),
							},
						},
					},
					{
						Name: &clusterCa.Name,
						VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
							ConfigMap: &applycorev1.ConfigMapVolumeSourceApplyConfiguration{
								LocalObjectReferenceApplyConfiguration: applycorev1.LocalObjectReferenceApplyConfiguration{
									Name: &clusterCa.Name,
								},
								DefaultMode: func() *int32 {
									v := int32(0755)
									return &v
								}(),
							},
						},
					},
				},
				InitContainers: []applycorev1.ContainerApplyConfiguration{
					{
						Name: func() *string {
							v := "wait-for-etcd"
							return &v
						}(),
						Image: func() *string {
							v := "busybox:1.36.1-glibc"
							return &v
						}(),
						ImagePullPolicy: func() *corev1.PullPolicy {
							a := corev1.PullIfNotPresent
							return &a
						}(),
						Command: []string{
							"sh",
							"-c",
							fmt.Sprintf(`for i in $(seq 1 300);do
  telnet etcd-0.%s.%s 2381 2> /dev/null
  if [ $? = "0" ];then
	  echo "etcd is initialized"
	  exit 0
  fi
  echo "etcd service is not initialized, waiting 1s... ($i/300)"
  sleep 1
done`, etcdSvc.Name, etcdSvc.Namespace),
						},
					},
				},
				Containers: []applycorev1.ContainerApplyConfiguration{
					{
						Name: func() *string {
							a := "kube-apiserver"
							return &a
						}(),
						Image: func() *string {
							a := fmt.Sprintf("%s/kube-apiserver:%s", info.Registry, vinfo.Kubernetes)
							return &a
						}(),
						ImagePullPolicy: func() *corev1.PullPolicy {
							a := corev1.PullIfNotPresent
							return &a
						}(),
						Command: []string{
							"kube-apiserver",
							"--anonymous-auth=false",
							"--authorization-mode=Node,RBAC",
							fmt.Sprintf("--advertise-address=%s", lbAddr),
							"--enable-aggregator-routing=true",
							"--allow-privileged=true",
							"--client-ca-file=/etc/kubernetes/pki/ca.crt",
							"--enable-bootstrap-token-auth",
							"--storage-backend=etcd3",
							"--etcd-cafile=/etc/kubernetes/pki/etcd/ca.crt",
							"--etcd-certfile=/etc/kubernetes/pki/apiserver-etcd-client.crt",
							"--etcd-keyfile=/etc/kubernetes/pki/apiserver-etcd-client.key",
							fmt.Sprintf("--etcd-servers=https://etcd-0.%s.%s:2379", etcdSvc.Name, etcdSvc.Namespace),
							"--kubelet-client-certificate=/etc/kubernetes/pki/apiserver-kubelet-client.crt",
							"--kubelet-client-key=/etc/kubernetes/pki/apiserver-kubelet-client.key",
							"--proxy-client-cert-file=/etc/kubernetes/pki/front-proxy-client.crt",
							"--proxy-client-key-file=/etc/kubernetes/pki/front-proxy-client.key",
							"--requestheader-allowed-names=front-proxy-client",
							"--requestheader-client-ca-file=/etc/kubernetes/pki/front-proxy-ca.crt",
							"--requestheader-extra-headers-prefix=X-Remote-Extra-",
							"--requestheader-group-headers=X-Remote-Group",
							"--requestheader-username-headers=X-Remote-User",
							"--secure-port=6443",
							"--service-account-issuer=https://kubernetes.default.svc.cluster.local",
							"--service-account-key-file=/etc/kubernetes/pki/sa.pub",
							"--service-account-signing-key-file=/etc/kubernetes/pki/sa.key",
							fmt.Sprintf("--service-cluster-ip-range=%s", info.ServiceCidr),
							"--tls-cert-file=/etc/kubernetes/pki/apiserver.crt",
							"--tls-private-key-file=/etc/kubernetes/pki/apiserver.key",
							"--audit-log-path=/var/log/kubernetes/audit.log",
							"--audit-policy-file=/etc/kubernetes/audit-policy-minimal.yaml",
							"--audit-log-format=json",
							"--audit-log-maxage=30",
							"--audit-log-maxbackup=10",
							"--audit-log-maxsize=100",
							"--encryption-provider-config=/etc/kubernetes/encryption-config.yaml",
							"--event-ttl=4h",
							"--anonymous-auth=false",
							"--kubelet-preferred-address-types=InternalIP,ExternalIP,Hostname",
							fmt.Sprintf("--service-node-port-range=%s", info.NodePort),
							"--runtime-config=api/all=true",
							"--profiling=false",
							"--enable-admission-plugins=ServiceAccount,NamespaceLifecycle,NodeRestriction,LimitRanger,PersistentVolumeClaimResize,DefaultStorageClass,DefaultTolerationSeconds,MutatingAdmissionWebhook,ValidatingAdmissionWebhook,ResourceQuota,Priority",
							"--tls-cipher-suites=TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,TLS_RSA_WITH_AES_256_GCM_SHA384,TLS_RSA_WITH_AES_128_GCM_SHA256",
							"--v=1",
						},
						Resources: &applycorev1.ResourceRequirementsApplyConfiguration{
							Limits: &corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("150m"),
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
							Requests: &corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("15m"),
								corev1.ResourceMemory: resource.MustParse("300Mi"),
							},
						},
						Ports: []applycorev1.ContainerPortApplyConfiguration{
							{
								Name: func() *string {
									a := "https"
									return &a
								}(),
								ContainerPort: func() *int32 {
									a := int32(6443)
									return &a
								}(),
								Protocol: func() *corev1.Protocol {
									a := corev1.ProtocolTCP
									return &a
								}(),
							},
						},
						SecurityContext: &applycorev1.SecurityContextApplyConfiguration{
							SeccompProfile: &applycorev1.SeccompProfileApplyConfiguration{
								Type: func() *corev1.SeccompProfileType {
									v := corev1.SeccompProfileTypeRuntimeDefault
									return &v
								}(),
							},
						},
						VolumeMounts: apiservervolumeMount,
						LivenessProbe: &applycorev1.ProbeApplyConfiguration{
							ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
								TCPSocket: &applycorev1.TCPSocketActionApplyConfiguration{
									Port: &intstr.IntOrString{
										Type:   0,
										IntVal: 6443,
										StrVal: "6443",
									},
								},
							},
							InitialDelaySeconds: func() *int32 {
								a := int32(10)
								return &a
							}(),
							TimeoutSeconds: func() *int32 {
								a := int32(15)
								return &a
							}(),
							PeriodSeconds: func() *int32 {
								a := int32(10)
								return &a
							}(),
							FailureThreshold: func() *int32 {
								a := int32(8)
								return &a
							}(),
						},
						ReadinessProbe: &applycorev1.ProbeApplyConfiguration{
							ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
								TCPSocket: &applycorev1.TCPSocketActionApplyConfiguration{
									Port: &intstr.IntOrString{
										Type:   0,
										IntVal: 6443,
										StrVal: "6443",
									},
								},
							},
							TimeoutSeconds: func() *int32 {
								a := int32(5)
								return &a
							}(),
							PeriodSeconds: func() *int32 {
								a := int32(5)
								return &a
							}(),
							FailureThreshold: func() *int32 {
								a := int32(5)
								return &a
							}(),
						},
					},
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	// kubeconfig cm
	kubeconfigCM, err := kubeControl.ConfigMaps().Apply(ns.Name, "kubeconfig", map[string]string{
		"kube-proxy.kubeconfig": `apiVersion: v1
clusters:
  - cluster:
      certificate-authority: /etc/kubernetes/pki/ca.crt
      server: https://kube-apiserver:6443
    name: kubernetes
contexts:
  - context:
      cluster: kubernetes
      user: kube-proxy
    name: kubernetes
current-context: kubernetes
kind: Config
preferences: {}
users:
  - name: kube-proxy
    user:
      client-certificate: /etc/kubernetes/pki/kube-proxy.crt
      client-key: /etc/kubernetes/pki/kube-proxy.key`,
		"kube-controller-manager.kubeconfig": `apiVersion: v1
clusters:
  - cluster:
      certificate-authority: /etc/kubernetes/pki/ca.crt
      server: https://kube-apiserver:6443
    name: kubernetes
contexts:
  - context:
      cluster: kubernetes
      user: system:kube-controller-manager
    name: system:kube-controller-manager@kubernetes
current-context: system:kube-controller-manager@kubernetes
kind: Config
preferences: {}
users:
  - name: system:kube-controller-manager
    user:
      client-certificate: /etc/kubernetes/pki/kube-controller-manager.crt
      client-key: /etc/kubernetes/pki/kube-controller-manager.key`,
		"kube-scheduler.kubeconfig": `apiVersion: v1
clusters:
  - cluster:
      certificate-authority: /etc/kubernetes/pki/ca.crt
      server: https://kube-apiserver:6443
    name: kubernetes
contexts:
  - context:
      cluster: kubernetes
      user: system:kube-scheduler
    name: system:kube-scheduler@kubernetes
current-context: system:kube-scheduler@kubernetes
kind: Config
preferences: {}
users:
  - name: system:kube-scheduler
    user:
      client-certificate: /etc/kubernetes/pki/kube-scheduler.crt
      client-key: /etc/kubernetes/pki/kube-scheduler.key`,
	})
	if err != nil {
		panic(err)
	}

	// kube-controller-manager svc
	_, err = kubeControl.Service().Apply(ns.Name, "kube-controller-manager", &applycorev1.ServiceSpecApplyConfiguration{
		Ports: []applycorev1.ServicePortApplyConfiguration{
			{
				Name: func() *string {
					mode := "https"
					return &mode
				}(),
				Port: func() *int32 {
					mode := int32(10257)
					return &mode
				}(),
				TargetPort: &intstr.IntOrString{
					Type:   0,
					IntVal: 10257,
					StrVal: "10257",
				},
			},
		},
		Selector: map[string]string{
			"app": "kube-controller-manager",
		},
		Type: func() *corev1.ServiceType {
			name := corev1.ServiceTypeClusterIP
			return &name
		}(),
	})
	if err != nil {
		panic(err)
	}

	// kube-controller-manager deployment
	controllermanagervolumeMount := append(volumeMount, applycorev1.VolumeMountApplyConfiguration{
		Name: &kubeconfigCM.Name,
		MountPath: func() *string {
			v := "/etc/kubernetes/kube-controller-manager.kubeconfig"
			return &v
		}(),
		SubPath: func() *string {
			v := "kube-controller-manager.kubeconfig"
			return &v
		}(),
		ReadOnly: func() *bool {
			v := true
			return &v
		}(),
	})
	_, err = kubeControl.Deployment().Apply(ns.Name, "kube-controller-manager", &applyappsv1.DeploymentSpecApplyConfiguration{
		Replicas: func() *int32 {
			mode := int32(1)
			return &mode
		}(),
		Selector: &applymetav1.LabelSelectorApplyConfiguration{
			MatchLabels: map[string]string{
				"app": "kube-controller-manager",
			},
		},
		Template: &applycorev1.PodTemplateSpecApplyConfiguration{
			ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
				Name: func() *string {
					a := "kube-controller-manager"
					return &a
				}(),
				Namespace: &namespace,
				Labels: map[string]string{
					"app":     "kube-controller-manager",
					"project": ns.Labels["project"],
					"env":     ns.Labels["env"],
				},
			},
			Spec: &applycorev1.PodSpecApplyConfiguration{
				Tolerations: []applycorev1.TolerationApplyConfiguration{
					{
						Key: func() *string {
							v := "node.kubernetes.io/not-ready"
							return &v
						}(),
						Operator: func() *corev1.TolerationOperator {
							v := corev1.TolerationOpExists
							return &v
						}(),
						Effect: func() *corev1.TaintEffect {
							v := corev1.TaintEffectNoExecute
							return &v
						}(),
						TolerationSeconds: func() *int64 {
							v := int64(1)
							return &v
						}(),
					},
					{
						Key: func() *string {
							v := "node.kubernetes.io/unreachable"
							return &v
						}(),
						Operator: func() *corev1.TolerationOperator {
							v := corev1.TolerationOpExists
							return &v
						}(),
						Effect: func() *corev1.TaintEffect {
							v := corev1.TaintEffectNoExecute
							return &v
						}(),
						TolerationSeconds: func() *int64 {
							v := int64(1)
							return &v
						}(),
					},
				},
				Affinity: &applycorev1.AffinityApplyConfiguration{
					PodAntiAffinity: &applycorev1.PodAntiAffinityApplyConfiguration{
						PreferredDuringSchedulingIgnoredDuringExecution: []applycorev1.WeightedPodAffinityTermApplyConfiguration{
							{
								Weight: func() *int32 {
									v := int32(100)
									return &v
								}(),
								PodAffinityTerm: &applycorev1.PodAffinityTermApplyConfiguration{
									LabelSelector: &applymetav1.LabelSelectorApplyConfiguration{
										MatchExpressions: []applymetav1.LabelSelectorRequirementApplyConfiguration{
											{
												Key: func() *string {
													v := "app"
													return &v
												}(),
												Operator: func() *metav1.LabelSelectorOperator {
													v := metav1.LabelSelectorOpIn
													return &v
												}(),
												Values: []string{
													"etcd",
													"kube-apiserver",
													"kube-controller-manager",
													"kube-scheduler",
												},
											},
										},
									},
									Namespaces: []string{namespace},
									TopologyKey: func() *string {
										v := "kubernetes.io/hostname"
										return &v
									}(),
								},
							},
						},
						RequiredDuringSchedulingIgnoredDuringExecution: []applycorev1.PodAffinityTermApplyConfiguration{
							{
								LabelSelector: &applymetav1.LabelSelectorApplyConfiguration{
									MatchExpressions: []applymetav1.LabelSelectorRequirementApplyConfiguration{
										{
											Key: func() *string {
												v := "app"
												return &v
											}(),
											Operator: func() *metav1.LabelSelectorOperator {
												v := metav1.LabelSelectorOpIn
												return &v
											}(),
											Values: []string{
												"kube-controller-manager",
											},
										},
									},
								},
								Namespaces: []string{
									namespace,
								},
								TopologyKey: func() *string {
									v := "kubernetes.io/hostname"
									return &v
								}(),
							},
						},
					},
				},
				Volumes: []applycorev1.VolumeApplyConfiguration{
					{
						Name: &kubeconfigCM.Name,
						VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
							ConfigMap: &applycorev1.ConfigMapVolumeSourceApplyConfiguration{
								LocalObjectReferenceApplyConfiguration: applycorev1.LocalObjectReferenceApplyConfiguration{
									Name: &kubeconfigCM.Name,
								},
							},
						},
					},
					// cluster ca comfigmap
					{
						Name: &clusterCa.Name,
						VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
							ConfigMap: &applycorev1.ConfigMapVolumeSourceApplyConfiguration{
								LocalObjectReferenceApplyConfiguration: applycorev1.LocalObjectReferenceApplyConfiguration{
									Name: &clusterCa.Name,
								},
								DefaultMode: func() *int32 {
									v := int32(0755)
									return &v
								}(),
							},
						},
					},
				},
				InitContainers: []applycorev1.ContainerApplyConfiguration{
					{
						Name: func() *string {
							v := "wait-for-apiserver"
							return &v
						}(),
						Image: func() *string {
							v := "buxiaomo/curl:8.2.1"
							return &v
						}(),
						ImagePullPolicy: func() *corev1.PullPolicy {
							a := corev1.PullIfNotPresent
							return &a
						}(),
						Command: []string{
							"sh",
							"-c",
							`for i in $(seq 1 300);do
  curl -s -k https://kube-apiserver:6443 -o /dev/null
  if [ $? = "0" ];then
	  echo "kube-apiserver is initialized"
	  exit 0
  fi
  echo "kube-apiserver service is not initialized, waiting 1s... ($i/300)"
  sleep 1
done`,
						},
					},
				},
				Containers: []applycorev1.ContainerApplyConfiguration{
					{
						Name: func() *string {
							a := "kube-controller-manager"
							return &a
						}(),
						Image: func() *string {
							a := fmt.Sprintf("%s/kube-controller-manager:%s", info.Registry, vinfo.Kubernetes)
							return &a
						}(),
						ImagePullPolicy: func() *corev1.PullPolicy {
							v := corev1.PullAlways
							return &v
						}(),
						Command: []string{
							"kube-controller-manager",
							"--bind-address=0.0.0.0",
							"--allocate-node-cidrs=true",
							"--authentication-kubeconfig=/etc/kubernetes/kube-controller-manager.kubeconfig",
							"--authorization-kubeconfig=/etc/kubernetes/kube-controller-manager.kubeconfig",
							"--client-ca-file=/etc/kubernetes/pki/ca.crt",
							fmt.Sprintf("--cluster-cidr=%s", info.PodCidr),
							"--cluster-name=kubernetes",
							"--cluster-signing-cert-file=/etc/kubernetes/pki/ca.crt",
							"--cluster-signing-key-file=/etc/kubernetes/pki/ca.key",
							"--controllers=*,bootstrapsigner,tokencleaner",
							"--kubeconfig=/etc/kubernetes/kube-controller-manager.kubeconfig",
							"--leader-elect=true",
							"--secure-port=10257",
							"--requestheader-client-ca-file=/etc/kubernetes/pki/front-proxy-ca.crt",
							"--root-ca-file=/etc/kubernetes/pki/ca.crt",
							"--service-account-private-key-file=/etc/kubernetes/pki/sa.key",
							fmt.Sprintf("--service-cluster-ip-range=%s", info.ServiceCidr),
							"--use-service-account-credentials=true",
							"--tls-cert-file=/etc/kubernetes/pki/kube-controller-manager.crt",
							"--tls-private-key-file=/etc/kubernetes/pki/kube-controller-manager.key",
							"--feature-gates=RotateKubeletServerCertificate=true",
							"--terminated-pod-gc-threshold=12500",
							"--node-monitor-period=5s",
							"--node-monitor-grace-period=40s",
							"--profiling=false",
							"--kube-api-qps=100",
							"--kube-api-burst=100",
							"--tls-cipher-suites=TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,TLS_RSA_WITH_AES_256_GCM_SHA384,TLS_RSA_WITH_AES_128_GCM_SHA256",
						},
						Resources: &applycorev1.ResourceRequirementsApplyConfiguration{
							Limits: &corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
							Requests: &corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("100Mi"),
							},
						},
						Ports: []applycorev1.ContainerPortApplyConfiguration{
							{
								Name: func() *string {
									a := "https"
									return &a
								}(),
								ContainerPort: func() *int32 {
									a := int32(10257)
									return &a
								}(),
								Protocol: func() *corev1.Protocol {
									a := corev1.ProtocolTCP
									return &a
								}(),
							},
						},
						SecurityContext: &applycorev1.SecurityContextApplyConfiguration{
							SeccompProfile: &applycorev1.SeccompProfileApplyConfiguration{
								Type: func() *corev1.SeccompProfileType {
									v := corev1.SeccompProfileTypeRuntimeDefault
									return &v
								}(),
							},
						},
						VolumeMounts: controllermanagervolumeMount,
						LivenessProbe: &applycorev1.ProbeApplyConfiguration{
							ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
								TCPSocket: &applycorev1.TCPSocketActionApplyConfiguration{
									Port: &intstr.IntOrString{
										Type:   0,
										IntVal: 10257,
										StrVal: "10257",
									},
								},
							},
							InitialDelaySeconds: func() *int32 {
								a := int32(10)
								return &a
							}(),
							TimeoutSeconds: func() *int32 {
								a := int32(15)
								return &a
							}(),
							PeriodSeconds: func() *int32 {
								a := int32(10)
								return &a
							}(),
							FailureThreshold: func() *int32 {
								a := int32(8)
								return &a
							}(),
						},
					},
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	// kube-scheduler svc
	_, err = kubeControl.Service().Apply(ns.Name, "kube-scheduler", &applycorev1.ServiceSpecApplyConfiguration{
		Ports: []applycorev1.ServicePortApplyConfiguration{
			{
				Name: func() *string {
					mode := "https"
					return &mode
				}(),
				Port: func() *int32 {
					mode := int32(10259)
					return &mode
				}(),
				TargetPort: &intstr.IntOrString{
					Type:   0,
					IntVal: 10259,
					StrVal: "10259",
				},
			},
		},
		Selector: map[string]string{
			"app": "kube-scheduler",
		},
		Type: func() *corev1.ServiceType {
			name := corev1.ServiceTypeClusterIP
			return &name
		}(),
	})
	if err != nil {
		panic(err)
	}

	// kube-scheduler deployment
	schedulervolumeMount := append(volumeMount, applycorev1.VolumeMountApplyConfiguration{
		Name: &kubeconfigCM.Name,
		MountPath: func() *string {
			v := "/etc/kubernetes/kube-scheduler.kubeconfig"
			return &v
		}(),
		SubPath: func() *string {
			v := "kube-scheduler.kubeconfig"
			return &v
		}(),
		ReadOnly: func() *bool {
			v := true
			return &v
		}(),
	})
	_, err = kubeControl.Deployment().Apply(ns.Name, "kube-scheduler", &applyappsv1.DeploymentSpecApplyConfiguration{
		Replicas: func() *int32 {
			mode := int32(1)
			return &mode
		}(),
		Selector: &applymetav1.LabelSelectorApplyConfiguration{
			MatchLabels: map[string]string{
				"app": "kube-scheduler",
			},
		},
		Template: &applycorev1.PodTemplateSpecApplyConfiguration{
			ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
				Name: func() *string {
					a := "kube-scheduler"
					return &a
				}(),
				Namespace: &namespace,
				Labels: map[string]string{
					"app":     "kube-scheduler",
					"project": ns.Labels["project"],
					"env":     ns.Labels["env"],
				},
			},
			Spec: &applycorev1.PodSpecApplyConfiguration{
				Tolerations: []applycorev1.TolerationApplyConfiguration{
					{
						Key: func() *string {
							v := "node.kubernetes.io/not-ready"
							return &v
						}(),
						Operator: func() *corev1.TolerationOperator {
							v := corev1.TolerationOpExists
							return &v
						}(),
						Effect: func() *corev1.TaintEffect {
							v := corev1.TaintEffectNoExecute
							return &v
						}(),
						TolerationSeconds: func() *int64 {
							v := int64(1)
							return &v
						}(),
					},
					{
						Key: func() *string {
							v := "node.kubernetes.io/unreachable"
							return &v
						}(),
						Operator: func() *corev1.TolerationOperator {
							v := corev1.TolerationOpExists
							return &v
						}(),
						Effect: func() *corev1.TaintEffect {
							v := corev1.TaintEffectNoExecute
							return &v
						}(),
						TolerationSeconds: func() *int64 {
							v := int64(1)
							return &v
						}(),
					},
				},
				Affinity: &applycorev1.AffinityApplyConfiguration{
					PodAntiAffinity: &applycorev1.PodAntiAffinityApplyConfiguration{
						PreferredDuringSchedulingIgnoredDuringExecution: []applycorev1.WeightedPodAffinityTermApplyConfiguration{
							{
								Weight: func() *int32 {
									v := int32(100)
									return &v
								}(),
								PodAffinityTerm: &applycorev1.PodAffinityTermApplyConfiguration{
									LabelSelector: &applymetav1.LabelSelectorApplyConfiguration{
										MatchExpressions: []applymetav1.LabelSelectorRequirementApplyConfiguration{
											{
												Key: func() *string {
													v := "app"
													return &v
												}(),
												Operator: func() *metav1.LabelSelectorOperator {
													v := metav1.LabelSelectorOpIn
													return &v
												}(),
												Values: []string{
													"etcd",
													"kube-apiserver",
													"kube-controller-manager",
													"kube-scheduler",
												},
											},
										},
									},
									Namespaces: []string{namespace},
									TopologyKey: func() *string {
										v := "kubernetes.io/hostname"
										return &v
									}(),
								},
							},
						},
						RequiredDuringSchedulingIgnoredDuringExecution: []applycorev1.PodAffinityTermApplyConfiguration{
							{
								LabelSelector: &applymetav1.LabelSelectorApplyConfiguration{
									MatchExpressions: []applymetav1.LabelSelectorRequirementApplyConfiguration{
										{
											Key: func() *string {
												v := "app"
												return &v
											}(),
											Operator: func() *metav1.LabelSelectorOperator {
												v := metav1.LabelSelectorOpIn
												return &v
											}(),
											Values: []string{
												"kube-scheduler",
											},
										},
									},
								},
								Namespaces: []string{
									namespace,
								},
								TopologyKey: func() *string {
									v := "kubernetes.io/hostname"
									return &v
								}(),
							},
						},
					},
				},
				InitContainers: []applycorev1.ContainerApplyConfiguration{
					{
						Name: func() *string {
							v := "wait-for-apiserver"
							return &v
						}(),
						Image: func() *string {
							v := "buxiaomo/curl:8.2.1"
							return &v
						}(),
						ImagePullPolicy: func() *corev1.PullPolicy {
							a := corev1.PullIfNotPresent
							return &a
						}(),
						Command: []string{
							"sh",
							"-c",
							`for i in $(seq 1 300);do
  curl -s -k https://kube-apiserver:6443 -o /dev/null
  if [ $? = "0" ];then
	  echo "kube-apiserver is initialized"
	  exit 0
  fi
  echo "kube-apiserver service is not initialized, waiting 1s... ($i/300)"
  sleep 1
done`,
						},
					},
				},
				Volumes: []applycorev1.VolumeApplyConfiguration{
					{
						Name: &kubeconfigCM.Name,
						VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
							ConfigMap: &applycorev1.ConfigMapVolumeSourceApplyConfiguration{
								LocalObjectReferenceApplyConfiguration: applycorev1.LocalObjectReferenceApplyConfiguration{
									Name: &kubeconfigCM.Name,
								},
								//DefaultMode: func() *int32 {
								//	a := int32(0755)
								//	return &a
								//}(),
							},
						},
					},
					{
						Name: &clusterCa.Name,
						VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
							ConfigMap: &applycorev1.ConfigMapVolumeSourceApplyConfiguration{
								LocalObjectReferenceApplyConfiguration: applycorev1.LocalObjectReferenceApplyConfiguration{
									Name: &clusterCa.Name,
								},
								DefaultMode: func() *int32 {
									v := int32(0755)
									return &v
								}(),
							},
						},
					},
				},
				Containers: []applycorev1.ContainerApplyConfiguration{
					{
						Name: func() *string {
							a := "kube-scheduler"
							return &a
						}(),
						Image: func() *string {
							v := fmt.Sprintf("%s/kube-scheduler:%s", info.Registry, vinfo.Kubernetes)
							return &v
						}(),
						ImagePullPolicy: func() *corev1.PullPolicy {
							v := corev1.PullAlways
							return &v
						}(),
						Command: []string{
							"kube-scheduler",
							"--bind-address=0.0.0.0",
							"--authentication-kubeconfig=/etc/kubernetes/kube-scheduler.kubeconfig",
							"--authorization-kubeconfig=/etc/kubernetes/kube-scheduler.kubeconfig",
							"--kubeconfig=/etc/kubernetes/kube-scheduler.kubeconfig",
							"--leader-elect=true",
							"--secure-port=10259",
							"--client-ca-file=/etc/kubernetes/pki/ca.crt",
							"--tls-cert-file=/etc/kubernetes/pki/kube-scheduler.crt",
							"--tls-private-key-file=/etc/kubernetes/pki/kube-scheduler.key",
							"--requestheader-allowed-names=aggregator",
							"--requestheader-client-ca-file=/etc/kubernetes/pki/front-proxy-ca.crt",
							"--requestheader-extra-headers-prefix=X-Remote-Extra-",
							"--requestheader-group-headers=X-Remote-Group",
							"--requestheader-username-headers=X-Remote-User",
							"--profiling=false",
							"--kube-api-qps=100",
							"--v=1",
							"--tls-cipher-suites=TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,TLS_RSA_WITH_AES_256_GCM_SHA384,TLS_RSA_WITH_AES_128_GCM_SHA256",
						},
						Resources: &applycorev1.ResourceRequirementsApplyConfiguration{
							Limits: &corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("50m"),
								corev1.ResourceMemory: resource.MustParse("400Mi"),
							},
							Requests: &corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("5m"),
								corev1.ResourceMemory: resource.MustParse("40Mi"),
							},
						},
						Ports: []applycorev1.ContainerPortApplyConfiguration{
							{
								Name: func() *string {
									a := "https"
									return &a
								}(),
								ContainerPort: func() *int32 {
									a := int32(10259)
									return &a
								}(),
								Protocol: func() *corev1.Protocol {
									a := corev1.ProtocolTCP
									return &a
								}(),
							},
						},
						SecurityContext: &applycorev1.SecurityContextApplyConfiguration{
							SeccompProfile: &applycorev1.SeccompProfileApplyConfiguration{
								Type: func() *corev1.SeccompProfileType {
									v := corev1.SeccompProfileTypeRuntimeDefault
									return &v
								}(),
							},
						},
						VolumeMounts: schedulervolumeMount,
						LivenessProbe: &applycorev1.ProbeApplyConfiguration{
							ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
								TCPSocket: &applycorev1.TCPSocketActionApplyConfiguration{
									Port: &intstr.IntOrString{
										Type:   0,
										IntVal: 10259,
										StrVal: "10259",
									},
								},
							},
							InitialDelaySeconds: func() *int32 {
								a := int32(10)
								return &a
							}(),
							TimeoutSeconds: func() *int32 {
								a := int32(15)
								return &a
							}(),
							PeriodSeconds: func() *int32 {
								a := int32(10)
								return &a
							}(),
							FailureThreshold: func() *int32 {
								a := int32(8)
								return &a
							}(),
						},
					},
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	go plugin(kubeControl, info, namespace)

	c.JSON(http.StatusOK, gin.H{
		"cmd": nil,
		"msg": nil,
	})
	return
}

func ClusterDelete(c *gin.Context) {
	namespace := c.Query("name")
	kubeControl := control.New("")

	ns, err := kubeControl.Namespace().Get(namespace)
	if err != nil {
		panic(err)
	}

	if ns.Labels["prometheus"] == "true" {
		kubeControl.Deployment().Delete(ns.Name, "prometheus")
		kubeControl.ClusterRoleBindings().Delete(fmt.Sprintf("application:control-plane:%s:prometheus", namespace))
		kubeControl.ClusterRoles().Delete(fmt.Sprintf("application:control-plane:%s:prometheus", namespace))
		kubeControl.ServiceAccount().Delete(ns.Name, "prometheus")
		kubeControl.ConfigMaps().Delete(ns.Name, "prometheus")
	}

	am := appmarket.New("")
	if am.Chart().Get(fmt.Sprintf("event-exporter-%s", ns.Name)) {
		am.Chart().UnInstall(fmt.Sprintf("event-exporter-%s", ns.Name))
	}

	kubeControl.Deployment().Delete(ns.Name, "kube-scheduler")
	kubeControl.Deployment().Delete(ns.Name, "kube-controller-manager")
	kubeControl.Deployment().Delete(ns.Name, "kube-apiserver")
	kubeControl.StatefulSets().Delete(ns.Name, "etcd")
	kubeControl.Service().Delete(ns.Name, "kube-apiserver")
	kubeControl.Service().Delete(ns.Name, "etcd")
	kubeControl.ConfigMaps().Delete(ns.Name, "kubeconfig")
	kubeControl.ConfigMaps().Delete(ns.Name, "kube-apiserver")
	//kubeControl.Pvc().Delete(ns.Name, "pki-vol")
	kubeControl.Roles().Delete(ns.Name, "application:control-plane:etcd")
	kubeControl.ServiceAccount().Delete(ns.Name, "etcd")
	kubeControl.Namespace().Delete(ns.Name)

	filename := fmt.Sprintf("./kubeconfig/%s.kubeconfig", ns.Name)
	_, err = os.Stat(filename)
	if err == nil {
		_ = os.RemoveAll(filename)
	}

	c.JSON(http.StatusOK, gin.H{
		"cmd": nil,
		"msg": nil,
	})
}

//
//func ClusterReDeploy(c *gin.Context) {
//	namespace := c.Query("namespace")
//	name := c.Query("name")
//	kok := control.New("")
//	e := kok.DeletePod(namespace, name)
//	if e != nil {
//		c.JSON(http.StatusOK, gin.H{
//			"cmd": nil,
//			"msg": e.Error(),
//		})
//		return
//	}
//	c.JSON(http.StatusOK, gin.H{
//		"cmd": nil,
//		"msg": nil,
//	})
//	return
//}

func ClusterEnableHA(c *gin.Context) {
	name := c.Query("name")
	kubeControl := control.New("")

	stsPatchBytes, _ := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": 3,
		},
	})
	if _, err := kubeControl.StatefulSets().Patch(name, "etcd", stsPatchBytes); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"cmd": nil,
			"msg": err.Error(),
		})
		return
	}

	deployPatchBytes, _ := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": 2,
		},
	})
	if _, err := kubeControl.Deployment().Patch(name, "kube-apiserver", deployPatchBytes); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"cmd": nil,
			"msg": err.Error(),
		})
		return
	}
	if _, err := kubeControl.Deployment().Patch(name, "kube-controller-manager", deployPatchBytes); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"cmd": nil,
			"msg": err.Error(),
		})
		return
	}
	if _, err := kubeControl.Deployment().Patch(name, "kube-scheduler", deployPatchBytes); err != nil {
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

func ClusterLog(c *gin.Context) {
	name := c.Query("name")
	kubeControl := control.New("")
	sa, err := kubeControl.ServiceAccount().Apply(name, "event-exporter")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"cmd": nil,
			"msg": err.Error(),
		})
		return
	}

	cr, err := kubeControl.ClusterRoles().Apply("application:control-plane:event-exporter", []applyrbacv1.PolicyRuleApplyConfiguration{
		{
			Verbs:     []string{"get", "watch", "list"},
			APIGroups: []string{"*"},
			Resources: []string{"*"},
		},
		{
			Verbs:     []string{"*"},
			APIGroups: []string{"coordination.k8s.io"},
			Resources: []string{"leases"},
		},
	})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"cmd": nil,
			"msg": err.Error(),
		})
		return
	}

	_, err = kubeControl.ClusterRoleBindings().Apply("application:control-plane:event-exporter", []applyrbacv1.SubjectApplyConfiguration{
		{
			Kind: func() *string {
				v := "ServiceAccount"
				return &v
			}(),
			Name:      &sa.Name,
			Namespace: &name,
		},
	}, &applyrbacv1.RoleRefApplyConfiguration{
		APIGroup: func() *string {
			v := "rbac.authorization.k8s.io"
			return &v
		}(),
		Kind: func() *string {
			v := "ClusterRole"
			return &v
		}(),
		Name: &cr.Name,
	})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"cmd": nil,
			"msg": err.Error(),
		})
		return
	}

}
