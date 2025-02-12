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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	applyappsv1 "k8s.io/client-go/applyconfigurations/apps/v1"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	applyrbacv1 "k8s.io/client-go/applyconfigurations/rbac/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	db "kok/models"
	"kok/pkg/appmarket"
	"kok/pkg/cert"
	"kok/pkg/control"
	"kok/pkg/utils"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"
	"time"
)

func Kubeconfig(c *gin.Context) {
	name := c.Query("name")
	kubeControl := control.New("")
	if name != "all" {
		ns, err := kubeControl.Namespace().Get(name)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"msg": err.Error()})
			return
		}
		cm, _ := kubeControl.ConfigMaps().Get(ns.Name, "cluster-ca")

		kubeconfig, _ := cert.CreateKubeconfigFileForRestConfig("kubernetes", "admin", rest.Config{
			Host: fmt.Sprintf("https://%s:6443", ns.Labels["loadBalancer"]),
			TLSClientConfig: rest.TLSClientConfig{
				Insecure: false,
				CAData:   []byte(cm.Data["ca.crt"]),
				CertData: []byte(cm.Data["admin.crt"]),
				KeyData:  []byte(cm.Data["admin.key"]),
			},
		}, "")
		c.String(http.StatusOK, string(kubeconfig))
		return
	}

	if _, err := os.Stat("./data/kubeconfig/all.kubeconfig"); err == nil {
		os.Remove("./data/kubeconfig/all.kubeconfig")
	}
	t, _ := os.Create("./data/kubeconfig/all.kubeconfig")
	t.Close()
	cmd := exec.Command("sh", "-c", "ls ./data/kubeconfig/*.kubeconfig | xargs -I{} sh -c 'kubecm --config ./data/kubeconfig/all.kubeconfig add -cf {} --context-name $(basename {} .kubeconfig)'")
	err := cmd.Run()
	if err != nil {
		klog.Warningf("cmd.Run() failed with %s\n", err)
	}
	f, err := os.ReadFile("./data/kubeconfig/all.kubeconfig")
	c.String(http.StatusOK, string(f))
	return
}

func kubeApiserverCfg(project, env string) map[string]string {
	u, err := url.Parse(viper.GetString("ELASTICSEARCH_URL"))
	if err != nil {
		panic(err)
	}
	return map[string]string{
		"fluent-bit.conf": fmt.Sprintf(`[Service]
    Http_Listen    0.0.0.0
    Http_Port    2020
    Http_Server    true
    Log_Level    error
    Parsers_File    /fluent-bit/etc/parsers.conf
[Input]
    Name    tail
    Path    /var/log/kubernetes/audit.log
    Refresh_Interval    10
    DB    /fluent-bit/devops-kube-audit.db
    DB.Sync    Normal
    Mem_Buf_Limit    500MB
    Tag    devops-kube-audit.*
[Filter]
    Name    parser
    Match    devops-kube-audit.*
    Key_Name    log
    Parser    json
[Filter]
    Name    record_modifier
    Match    devops-kube-audit.*
    Record    clusterName %s-%s
[Filter]
    Name    modify
    Match    devops-kube-audit.*
    Rename    requestReceivedTimestamp    @timestamp
[Output]
    Name    es
    Match_Regex    devops-kube-audit.*
    Host    %s
    Port    %s
    Index    devops-kube-audit-%%Y.%%m.%%d
    Type    _doc
    Time_Key    @timestamp
    Replace_Dots    true
    Trace_Error    true
    Suppress_Type_Name    false
`, project, env, u.Hostname(), u.Port()),
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

func controlLog(ns *corev1.Namespace) error {
	kubeControl := control.New("")
	u, err := url.Parse(viper.GetString("ELASTICSEARCH_URL"))
	if err != nil {
		return err
	}
	port, _ := strconv.Atoi(u.Port())
	cfdo := map[string]interface{}{
		"apiVersion": "fluentd.fluent.io/v1alpha1",
		"kind":       "ClusterOutput",
		"metadata": map[string]interface{}{
			"name": ns.Name,
			"labels": map[string]interface{}{
				"output.fluentd.fluent.io/rule.name": ns.Name,
			},
		},
		"spec": map[string]interface{}{
			"outputs": []map[string]interface{}{
				{
					"elasticsearch": map[string]interface{}{
						"host":           u.Hostname(),
						"port":           port,
						"logstashFormat": true,
						"logstashPrefix": ns.Name,
					},
				},
			},
		},
	}
	err = kubeControl.Crd("").Apply(ns.Name, cfdo, schema.GroupVersionResource{
		Group:    "fluentd.fluent.io",
		Version:  "v1alpha1",
		Resource: "clusteroutputs",
	})
	if err != nil {
		return err
	}

	cfdc := map[string]interface{}{
		"apiVersion": "fluentd.fluent.io/v1alpha1",
		"kind":       "ClusterFluentdConfig",
		"metadata": map[string]interface{}{
			"name": ns.Name,
			"labels": map[string]interface{}{
				"config.fluentd.fluent.io/enabled": "true",
			},
		},
		"spec": map[string]interface{}{
			"watchedNamespaces": []string{
				ns.Name,
			},
			"clusterFilterSelector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"filter.fluentd.fluent.io/rule.name": ns.Name,
				},
			},
			"clusterOutputSelector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"output.fluentd.fluent.io/rule.name": ns.Name,
				},
			},
		},
	}
	err = kubeControl.Crd("").Apply(ns.Name, cfdc,
		schema.GroupVersionResource{
			Group:    "fluentd.fluent.io",
			Version:  "v1alpha1",
			Resource: "clusterfluentdconfigs",
		},
	)
	if err != nil {
		return err
	}
	return nil
}

func plugin(info createInfo, namespace string) {
	err := waitForClusterReady(namespace)

	kubeControl := control.New("")

	ns, _ := kubeControl.Namespace().Get(namespace)
	if err == nil {
		remoteAppMarket := appmarket.New(fmt.Sprintf("./data/kubeconfig/%s.kubeconfig", namespace))

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
		case "cilium":
			remoteAppMarket.Chart().Install("kube-system", "cilium", "cilium", false, info.NetworkVersion, map[string]interface{}{
				"installNoConntrackIptablesRules": true,
				"externalIPs": map[string]interface{}{
					"enabled": true,
				},
				"nodePort": map[string]interface{}{
					"enabled": true,
				},
				"hostPort": map[string]interface{}{
					"enabled": true,
				},
				"socketLB": map[string]interface{}{
					"enabled": true,
				},
				"loadBalancer": map[string]interface{}{
					"mode":         "snat",
					"acceleration": "native",
				},
				"bpf": map[string]interface{}{
					"masquerade": true,
				},
				"ipam": map[string]interface{}{
					"operator": map[string]interface{}{
						"clusterPoolIPv4PodCIDRList": []string{info.PodCidr},
						"clusterPoolIPv4MaskSize":    24,
					},
				},
				"bandwidthManager": map[string]interface{}{
					"bbr": true,
				},
				"cni": map[string]interface{}{
					"chainingMode": "portmap",
				},
				"cgroup": map[string]interface{}{
					"autoMount": map[string]interface{}{
						"enabled": false,
					},
				},
				"securityContext": map[string]interface{}{
					"privileged": true,
				},
				"hubble": map[string]interface{}{
					"enabled": true,
					"ui": map[string]interface{}{
						"enabled": true,
					},
					"relay": map[string]interface{}{
						"enabled": true,
					},
					"metrics": map[string]interface{}{
						"enableOpenMetrics": true,
					},
				},
				"prometheus": map[string]interface{}{
					"enabled": true,
				},
				"operator": map[string]interface{}{
					"prometheus": map[string]interface{}{
						"enabled": true,
					},
				},
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

		remoteAppMarket.Chart().Install("kube-system", "metrics-server", "metrics-server", false, ns.Labels["metrics-server"], map[string]interface{}{
			"replicaCount": 1,
		})

		//remoteAppMarket.Chart().Install("kube-system", "kubernetes-dashboard", "kubernetes-dashboard", false, ns.Labels["dashboard"], map[string]interface{}{
		//	"kong": map[string]interface{}{
		//		"proxy": map[string]interface{}{
		//			"type": "NodePort",
		//			"http": map[string]interface{}{
		//				"enabled": true,
		//			},
		//		},
		//	},
		//})

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

func waitForClusterReady(namespace string) (err error) {
	kubeControl := control.New("")
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
	remoteKubeControl := control.New(fmt.Sprintf("./data/kubeconfig/%s.kubeconfig", name))

	ns, err := localkubeControl.Namespace().Get(name)

	remoteAppMarket := appmarket.New(fmt.Sprintf("./data/kubeconfig/%s.kubeconfig", name))
	err = remoteAppMarket.Chart().Install("kube-system", "prometheus", "prometheus", false, "2.54.1", map[string]interface{}{
		"replicaCount": 1,
		"remoteWrite":  viper.GetString("PROMETHEUS_URL"),
		"clusterName":  ns.Labels["project"],
		"clusterEnv":   ns.Labels["env"],
	})
	if err != nil {
		panic(fmt.Sprintf("Install proetheus on the remote cluster err: %s", err))
	}

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

type upgradeInfo struct {
	Kubernetes     string `json:"kubernetes" binding:"required"`
	Network        string `json:"network" binding:"required"`
	NetworkVersion string `json:"networkVersion" binding:"required"`
	//Containerd string `json:"containerd" binding:"required"`
}

func UpgradeCluster(namespace string, info upgradeInfo) {
	kubeControl := control.New("")
	ns, err := kubeControl.Namespace().Get(namespace)
	if err != nil {
		panic(err.Error())
	}
	var (
		v              db.Version
		patchBytes     []byte
		networkVersion string
	)
	if info.Kubernetes != ns.Labels["kubernetes"] {
		versionInfo, _ := v.Select(info.Kubernetes)
		if info.NetworkVersion == "None" {
			networkVersion = ns.Labels["networkVersion"]
		} else {
			networkVersion = info.NetworkVersion
		}
		patchBytes, _ = json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels": map[string]string{
					"kubernetes":         fmt.Sprintf("%s-%s", ns.Labels["kubernetes"], info.Kubernetes),
					"containerd":         versionInfo.Containerd,
					"etcd":               versionInfo.Etcd,
					"CoreDNS":            versionInfo.Coredns,
					"dashboard":          versionInfo.Dashboard,
					"kube-state-metrics": versionInfo.KubeStateMetrics,
					"metrics-server":     versionInfo.MetricsServer,
					"pause":              versionInfo.Pause,
					"runc":               versionInfo.Runc,
					"networkVersion":     networkVersion,
				},
			},
		})
		ns, _ = kubeControl.Namespace().Patch(namespace, types.MergePatchType, patchBytes)

		clusterCa, _ := kubeControl.ConfigMaps().Get(namespace, "cluster-ca")
		etcdSvc, _ := kubeControl.Service().Get(namespace, "etcd")
		etcdSA, _ := kubeControl.ServiceAccount().Get(namespace, "etcd")
		kubeApiserverCM, _ := kubeControl.ConfigMaps().Get(namespace, "kube-apiserver")
		kubeconfigCM, _ := kubeControl.ConfigMaps().Get(namespace, "kubeconfig")

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
			{
				Name: &clusterCa.Name,
				ReadOnly: func() *bool {
					v := true
					return &v
				}(),
				MountPath: func() *string {
					v := "/etc/kubernetes/pki/admin.crt"
					return &v
				}(),
				SubPath: func() *string {
					v := "admin.crt"
					return &v
				}(),
			},
			{
				Name: &clusterCa.Name,
				ReadOnly: func() *bool {
					v := true
					return &v
				}(),
				MountPath: func() *string {
					v := "/etc/kubernetes/pki/admin.key"
					return &v
				}(),
				SubPath: func() *string {
					v := "admin.key"
					return &v
				}(),
			},
		}

		// etcd sts
		etcdvolumeMount := append(volumeMount,
			applycorev1.VolumeMountApplyConfiguration{
				Name: func() *string {
					v := "data-vol"
					return &v
				}(),
				MountPath: func() *string {
					v := "/var/lib/etcd"
					return &v
				}(),
			}, applycorev1.VolumeMountApplyConfiguration{
				Name: func() *string {
					v := "cache-vol"
					return &v
				}(),
				MountPath: func() *string {
					v := "/var/lib/cache"
					return &v
				}(),
			},
		)
		_, err := kubeControl.StatefulSets().Apply(ns.Name, "etcd", &applyappsv1.StatefulSetSpecApplyConfiguration{
			//PodManagementPolicy: func() *v1.PodManagementPolicyType {
			//	v := v1.ParallelPodManagement
			//	return &v
			//}(),
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
						"version": ns.Labels["etcd"],
					},
				},
				Spec: &applycorev1.PodSpecApplyConfiguration{
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
										v := int32(0644)
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
								a := fmt.Sprintf("buxiaomo/etcd:%s", versionInfo.Etcd)
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
							//LivenessProbe: &applycorev1.ProbeApplyConfiguration{
							//	ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
							//		HTTPGet: &applycorev1.HTTPGetActionApplyConfiguration{
							//			Path: func() *string {
							//				a := "/health"
							//				return &a
							//			}(),
							//			Port: &intstr.IntOrString{
							//				Type:   0,
							//				IntVal: 2381,
							//				StrVal: "2381",
							//			},
							//			Scheme: func() *corev1.URIScheme {
							//				a := corev1.URISchemeHTTP
							//				return &a
							//			}(),
							//		},
							//	},
							//	InitialDelaySeconds: func() *int32 {
							//		a := int32(10)
							//		return &a
							//	}(),
							//	TimeoutSeconds: func() *int32 {
							//		a := int32(15)
							//		return &a
							//	}(),
							//	PeriodSeconds: func() *int32 {
							//		a := int32(10)
							//		return &a
							//	}(),
							//	FailureThreshold: func() *int32 {
							//		a := int32(8)
							//		return &a
							//	}(),
							//},
							//ReadinessProbe: &applycorev1.ProbeApplyConfiguration{
							//	ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
							//		HTTPGet: &applycorev1.HTTPGetActionApplyConfiguration{
							//			Path: func() *string {
							//				a := "/health"
							//				return &a
							//			}(),
							//			Port: &intstr.IntOrString{
							//				Type:   0,
							//				IntVal: 2381,
							//				StrVal: "2381",
							//			},
							//			Scheme: func() *corev1.URIScheme {
							//				a := corev1.URISchemeHTTP
							//				return &a
							//			}(),
							//		},
							//	},
							//	InitialDelaySeconds: func() *int32 {
							//		a := int32(5)
							//		return &a
							//	}(),
							//	TimeoutSeconds: func() *int32 {
							//		a := int32(5)
							//		return &a
							//	}(),
							//	PeriodSeconds: func() *int32 {
							//		a := int32(5)
							//		return &a
							//	}(),
							//},
							//Lifecycle: &applycorev1.LifecycleApplyConfiguration{
							//	PreStop: &applycorev1.LifecycleHandlerApplyConfiguration{
							//		Exec: &applycorev1.ExecActionApplyConfiguration{
							//			Command: []string{
							//				"/bin/sh",
							//				"-cx",
							//				"/usr/local/bin/prestop.sh",
							//			},
							//		},
							//	},
							//},
							Command: []string{
								"/bin/sh",
								"-ecx",
								"/usr/local/bin/entrypoint.sh",
							},
						},
					},
					ServiceAccountName: &etcdSA.Name,
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
				},
			},
			VolumeClaimTemplates: []applycorev1.PersistentVolumeClaimApplyConfiguration{
				{
					ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
						Name: func() *string {
							v := "data-vol"
							return &v
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
				{
					ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
						Name: func() *string {
							v := "cache-vol"
							return &v
						}(),
					},
					Spec: &applycorev1.PersistentVolumeClaimSpecApplyConfiguration{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: &applycorev1.VolumeResourceRequirementsApplyConfiguration{
							Requests: &corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("1Gi"),
							},
						},
					},
				},
			},
		})
		if err != nil {
			panic(err)
		}
		if err := kubeControl.WaitForStatefulSetUpdate(ns.Name, "etcd"); err != nil {
			klog.Warning(err.Error())
		}

		// kube-apiserver deployment
		apiservervolumeMount := append(volumeMount,
			applycorev1.VolumeMountApplyConfiguration{
				Name: &kubeApiserverCM.Name,
				MountPath: func() *string {
					v := "/etc/kubernetes/encryption-config.yaml"
					return &v
				}(),
				SubPath: func() *string {
					v := "encryption-config.yaml"
					return &v
				}(),
			},
			applycorev1.VolumeMountApplyConfiguration{
				Name: &kubeApiserverCM.Name,
				MountPath: func() *string {
					v := "/etc/kubernetes/audit-policy-minimal.yaml"
					return &v
				}(),
				SubPath: func() *string {
					v := "audit-policy-minimal.yaml"
					return &v
				}(),
			},
			applycorev1.VolumeMountApplyConfiguration{
				Name: func() *string {
					v := "audit-log-dir"
					return &v
				}(),
				MountPath: func() *string {
					v := "/var/log/kubernetes/"
					return &v
				}(),
			},
		)
		apiserverDeploy, err := kubeControl.Deployment().Apply(ns.Name, "kube-apiserver", &applyappsv1.DeploymentSpecApplyConfiguration{
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
						v := "kube-apiserver"
						return &v
					}(),
					Namespace: &namespace,
					Labels: map[string]string{
						"app":     "kube-apiserver",
						"project": ns.Labels["project"],
						"env":     ns.Labels["env"],
						"version": info.Kubernetes,
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
						// audit-log-dir
						{
							Name: func() *string {
								v := "audit-log-dir"
								return &v
							}(),
							VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
								EmptyDir: &applycorev1.EmptyDirVolumeSourceApplyConfiguration{
									//Medium: nil,
									SizeLimit: func() *resource.Quantity {
										v := resource.MustParse("150M")
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
								a := fmt.Sprintf("%s/kube-apiserver:%s", ns.Labels["registry"], versionInfo.Kubernetes)
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
								fmt.Sprintf("--advertise-address=%s", ns.Labels["loadBalancer"]),
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
								fmt.Sprintf("--service-cluster-ip-range=%s", strings.Replace(ns.Labels["serviceSubnet"], "-", "/", 1)),
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
								fmt.Sprintf("--service-node-port-range=%s", ns.Labels["nodePort"]),
								"--runtime-config=api/all=true",
								"--profiling=false",
								"--enable-admission-plugins=ServiceAccount,NamespaceLifecycle,NodeRestriction,LimitRanger,PersistentVolumeClaimResize,DefaultStorageClass,DefaultTolerationSeconds,MutatingAdmissionWebhook,ValidatingAdmissionWebhook,ResourceQuota,Priority",
								"--tls-cipher-suites=TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,TLS_RSA_WITH_AES_256_GCM_SHA384,TLS_RSA_WITH_AES_128_GCM_SHA256",
								"--v=1",
							},
							Resources: &applycorev1.ResourceRequirementsApplyConfiguration{
								Limits: &corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
								Requests: &corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("20m"),
									corev1.ResourceMemory: resource.MustParse("250Mi"),
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
						{
							Name: func() *string {
								v := "fluent-bit"
								return &v
							}(),
							Image: func() *string {
								v := "fluent/fluent-bit:3.1.8"
								return &v
							}(),
							VolumeMounts: []applycorev1.VolumeMountApplyConfiguration{
								{
									Name: &kubeApiserverCM.Name,
									MountPath: func() *string {
										v := "/fluent-bit/etc/fluent-bit.conf"
										return &v
									}(),
									SubPath: func() *string {
										v := "fluent-bit.conf"
										return &v
									}(),
								},
								{
									Name: func() *string {
										v := "audit-log-dir"
										return &v
									}(),
									MountPath: func() *string {
										v := "/var/log/kubernetes/"
										return &v
									}(),
								},
							},
						},
					},
				},
			},
		})
		if err != nil {
			panic(err)
		}
		if err := kubeControl.WaitForDeploymentUpdate(ns.Name, apiserverDeploy); err != nil {
			klog.Warning(err.Error())
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
		controllermanagerDeploy, err := kubeControl.Deployment().Apply(ns.Name, "kube-controller-manager", &applyappsv1.DeploymentSpecApplyConfiguration{
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
						"version": info.Kubernetes,
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
								a := fmt.Sprintf("%s/kube-controller-manager:%s", ns.Labels["registry"], info.Kubernetes)
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
								fmt.Sprintf("--cluster-cidr=%s", strings.Replace(ns.Labels["podSubnet"], "-", "/", 1)),
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
								fmt.Sprintf("--service-cluster-ip-range=%s", strings.Replace(ns.Labels["serviceSubnet"], "-", "/", 1)),
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
									corev1.ResourceCPU:    resource.MustParse("15m"),
									corev1.ResourceMemory: resource.MustParse("150Mi"),
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
		if err := kubeControl.WaitForDeploymentUpdate(ns.Name, controllermanagerDeploy); err != nil {
			klog.Warning(err.Error())
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
		schedulerDeploy, err := kubeControl.Deployment().Apply(ns.Name, "kube-scheduler", &applyappsv1.DeploymentSpecApplyConfiguration{
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
						"version": info.Kubernetes,
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
								v := fmt.Sprintf("%s/kube-scheduler:%s", ns.Labels["registry"], info.Kubernetes)
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
		if err := kubeControl.WaitForDeploymentUpdate(ns.Name, schedulerDeploy); err != nil {
			klog.Warning(err.Error())
		}

		remoteAppMarket := appmarket.New(fmt.Sprintf("./data/kubeconfig/%s.kubeconfig", namespace))

		if info.NetworkVersion != "None" {
			// 升级 cni
			switch ns.Labels["network"] {
			case "flannel":
				remoteAppMarket.Chart().Install("kube-system", "flannel", "flannel", false, info.NetworkVersion, map[string]interface{}{
					"subNet": strings.Replace(ns.Labels["podSubnet"], "-", "/", 1),
				})
			case "calico":
				remoteAppMarket.Chart().Install("kube-system", "calico", "calico", false, info.NetworkVersion, map[string]interface{}{
					"subNet": strings.Replace(ns.Labels["podSubnet"], "-", "/", 1),
				})
			case "canal":
				remoteAppMarket.Chart().Install("kube-system", "canal", "canal", false, info.NetworkVersion, map[string]interface{}{
					"subNet": strings.Replace(ns.Labels["podSubnet"], "-", "/", 1),
				})
			case "antrea":
				remoteAppMarket.Chart().Install("kube-system", "antrea", "antrea", false, info.NetworkVersion, map[string]interface{}{
					"subNet": strings.Replace(ns.Labels["podSubnet"], "-", "/", 1),
				})
			case "cilium":
				remoteAppMarket.Chart().Install("kube-system", "cilium", "cilium", false, info.NetworkVersion, map[string]interface{}{
					"installNoConntrackIptablesRules": true,
					"externalIPs": map[string]interface{}{
						"enabled": true,
					},
					"nodePort": map[string]interface{}{
						"enabled": true,
					},
					"hostPort": map[string]interface{}{
						"enabled": true,
					},
					"socketLB": map[string]interface{}{
						"enabled": true,
					},
					"loadBalancer": map[string]interface{}{
						"mode":         "snat",
						"acceleration": "native",
					},
					"bpf": map[string]interface{}{
						"masquerade": true,
					},
					"ipam": map[string]interface{}{
						"operator": map[string]interface{}{
							"clusterPoolIPv4PodCIDRList": []string{strings.Replace(ns.Labels["podSubnet"], "-", "/", 1)},
							"clusterPoolIPv4MaskSize":    24,
						},
					},
					"bandwidthManager": map[string]interface{}{
						"bbr": true,
					},
					"cni": map[string]interface{}{
						"chainingMode": "portmap",
					},
					"cgroup": map[string]interface{}{
						"autoMount": map[string]interface{}{
							"enabled": false,
						},
					},
					"securityContext": map[string]interface{}{
						"privileged": true,
					},
					"hubble": map[string]interface{}{
						"enabled": true,
						"ui": map[string]interface{}{
							"enabled": true,
						},
						"relay": map[string]interface{}{
							"enabled": true,
						},
						"metrics": map[string]interface{}{
							"enableOpenMetrics": true,
						},
					},
					"prometheus": map[string]interface{}{
						"enabled": true,
					},
					"operator": map[string]interface{}{
						"prometheus": map[string]interface{}{
							"enabled": true,
						},
					},
				})
			case "none":
				fmt.Println("network plugin is none, skip..")
			}
		}

		// 升级 coredns
		remoteAppMarket.Chart().Install("kube-system", "coredns", "coredns", false, ns.Labels["CoreDNS"], map[string]interface{}{
			"replicaCount": 1,
			"clusterIP":    ns.Labels["clusterDNS"],
		})

		// 升级 kube-state-metrics
		remoteAppMarket.Chart().Install("kube-system", "kube-state-metrics", "kube-state-metrics", false, ns.Labels["kube-state-metrics"], map[string]interface{}{
			"replicaCount": 1,
		})

		// 升级 metrics-server
		remoteAppMarket.Chart().Install("kube-system", "metrics-server", "metrics-server", false, ns.Labels["metrics-server"], map[string]interface{}{
			"replicaCount": 1,
		})

		patchBytes, _ = json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels": map[string]string{
					"kubernetes": info.Kubernetes,
				},
			},
		})
		kubeControl.Namespace().Patch(namespace, types.MergePatchType, patchBytes)
	}
}

func ClusterUpgrade(c *gin.Context) {
	namespace := c.Query("name")
	var (
		info upgradeInfo
	)
	if err := c.BindJSON(&info); err != nil {
		panic("bind: " + err.Error())
	}
	go UpgradeCluster(namespace, info)
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

	remoteKubeControl := control.New(fmt.Sprintf("./data/kubeconfig/%s.kubeconfig", name))
	cd, _ := remoteKubeControl.Deployment().Get("kube-system", "coredns")
	ms, _ := remoteKubeControl.Deployment().Get("kube-system", "metrics-server")
	nw, _ := remoteKubeControl.DaemonSets().Get("kube-system", networkName)
	node, _ := remoteKubeControl.Nodes().List()
	c.JSON(http.StatusOK, gin.H{
		"etcd": map[string]interface{}{
			"version": ns.Labels["etcd"],
		},
		"kubernetes": map[string]interface{}{
			"version": ns.Labels["kubernetes"],
		},
		"cri": map[string]interface{}{
			"version": ns.Labels["containerd"],
		},
		"coredns": map[string]interface{}{
			"version": ns.Labels["CoreDNS"],
			"status":  fmt.Sprintf("%d/%d", cd.Status.AvailableReplicas, cd.Status.Replicas),
		},
		"metricsServer": fmt.Sprintf("%d/%d", ms.Status.AvailableReplicas, ms.Status.Replicas),
		"network": map[string]string{
			"name":   ns.Labels["network"],
			"status": fmt.Sprintf("%d/%d", nw.Status.CurrentNumberScheduled, nw.Status.DesiredNumberScheduled),
		},
		"node": node.Items,
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
		Version := ns.Labels["kubernetes"]
		if strings.Contains(ns.Labels["kubernetes"], "-") {
			Version = strings.Replace(ns.Labels["kubernetes"], "-", " -> ", -1)
		}

		instance = append(instance, map[string]interface{}{
			"Name":           ns.Name,
			"Version":        Version,
			"Status":         ns.Status.Phase,
			"Network":        ns.Labels["network"],
			"NetworkVersion": ns.Labels["networkVersion"],
			"LoadBalancer":   ns.Labels["loadBalancer"],
			"Components":     components,
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
	if err != nil {
		panic(err)
	}
	kubernetesAddr, _ := utils.GetCidrIpRange(info.ServiceCidr)      // 10.96.0.1 use by ca
	clusterDNS := utils.Increment(kubernetesAddr, int64(1)).String() // 10.96.0.2 use by coredns

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
			"dashboard":          vinfo.Dashboard,
			"istio-injection":    viper.GetString("ISTIO_INJECTION"),
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
		tl := pki.GenerateAll(10, info.Project, info.Env, lbAddr, kubernetesAddr)
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

	kubeconfig, err := cert.CreateKubeconfigFileForRestConfig(ns.Name, fmt.Sprintf("%s-admin", ns.Name), rest.Config{
		Host: fmt.Sprintf("https://%s:6443", lbAddr),
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: false,
			CAData:   []byte(clusterCa.Data["ca.crt"]),
			CertData: []byte(clusterCa.Data["admin.crt"]),
			KeyData:  []byte(clusterCa.Data["admin.key"]),
		},
	}, fmt.Sprintf("./data/kubeconfig/%s-%s.kubeconfig", info.Project, info.Env))

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
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/admin.crt"
				return &v
			}(),
			SubPath: func() *string {
				v := "admin.crt"
				return &v
			}(),
		},
		{
			Name: &clusterCa.Name,
			ReadOnly: func() *bool {
				v := true
				return &v
			}(),
			MountPath: func() *string {
				v := "/etc/kubernetes/pki/admin.key"
				return &v
			}(),
			SubPath: func() *string {
				v := "admin.key"
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
	etcdvolumeMount := append(volumeMount,
		applycorev1.VolumeMountApplyConfiguration{
			Name: func() *string {
				v := "data-vol"
				return &v
			}(),
			MountPath: func() *string {
				v := "/var/lib/etcd"
				return &v
			}(),
		},
		applycorev1.VolumeMountApplyConfiguration{
			Name: func() *string {
				v := "cache-vol"
				return &v
			}(),
			MountPath: func() *string {
				v := "/var/lib/cache"
				return &v
			}(),
		},
	)
	_, err = kubeControl.StatefulSets().Apply(ns.Name, "etcd", &applyappsv1.StatefulSetSpecApplyConfiguration{
		//PodManagementPolicy: func() *v1.PodManagementPolicyType {
		//	v := v1.ParallelPodManagement
		//	return &v
		//}(),
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
					"version": ns.Labels["etcd"],
				},
			},
			Spec: &applycorev1.PodSpecApplyConfiguration{
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
									v := int32(0644)
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
						//LivenessProbe: &applycorev1.ProbeApplyConfiguration{
						//	ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
						//		HTTPGet: &applycorev1.HTTPGetActionApplyConfiguration{
						//			Path: func() *string {
						//				a := "/health"
						//				return &a
						//			}(),
						//			Port: &intstr.IntOrString{
						//				Type:   0,
						//				IntVal: 2381,
						//				StrVal: "2381",
						//			},
						//			Scheme: func() *corev1.URIScheme {
						//				a := corev1.URISchemeHTTP
						//				return &a
						//			}(),
						//		},
						//	},
						//	InitialDelaySeconds: func() *int32 {
						//		a := int32(10)
						//		return &a
						//	}(),
						//	TimeoutSeconds: func() *int32 {
						//		a := int32(15)
						//		return &a
						//	}(),
						//	PeriodSeconds: func() *int32 {
						//		a := int32(10)
						//		return &a
						//	}(),
						//	FailureThreshold: func() *int32 {
						//		a := int32(8)
						//		return &a
						//	}(),
						//},
						//ReadinessProbe: &applycorev1.ProbeApplyConfiguration{
						//	ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
						//		HTTPGet: &applycorev1.HTTPGetActionApplyConfiguration{
						//			Path: func() *string {
						//				a := "/health"
						//				return &a
						//			}(),
						//			Port: &intstr.IntOrString{
						//				Type:   0,
						//				IntVal: 2381,
						//				StrVal: "2381",
						//			},
						//			Scheme: func() *corev1.URIScheme {
						//				a := corev1.URISchemeHTTP
						//				return &a
						//			}(),
						//		},
						//	},
						//	InitialDelaySeconds: func() *int32 {
						//		a := int32(5)
						//		return &a
						//	}(),
						//	TimeoutSeconds: func() *int32 {
						//		a := int32(5)
						//		return &a
						//	}(),
						//	PeriodSeconds: func() *int32 {
						//		a := int32(5)
						//		return &a
						//	}(),
						//},
						//Lifecycle: &applycorev1.LifecycleApplyConfiguration{
						//	PreStop: &applycorev1.LifecycleHandlerApplyConfiguration{
						//		Exec: &applycorev1.ExecActionApplyConfiguration{
						//			Command: []string{
						//				"/bin/sh",
						//				"-cx",
						//				"/usr/local/bin/prestop.sh",
						//			},
						//		},
						//	},
						//},
						Command: []string{
							"/bin/sh",
							"-ecx",
							"/usr/local/bin/entrypoint.sh",
						},
					},
				},
				ServiceAccountName: &etcdSA.Name,
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
			},
		},
		VolumeClaimTemplates: []applycorev1.PersistentVolumeClaimApplyConfiguration{
			{
				ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
					Name: func() *string {
						v := "data-vol"
						return &v
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
			{
				ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
					Name: func() *string {
						v := "cache-vol"
						return &v
					}(),
				},
				Spec: &applycorev1.PersistentVolumeClaimSpecApplyConfiguration{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: &applycorev1.VolumeResourceRequirementsApplyConfiguration{
						Requests: &corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
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
	kubeApiserverCM, err := kubeControl.ConfigMaps().Apply(ns.Name, "kube-apiserver", kubeApiserverCfg(info.Project, info.Env))
	if err != nil {
		panic(err)
	}

	// kube-apiserver deployment
	apiservervolumeMount := append(volumeMount,
		applycorev1.VolumeMountApplyConfiguration{
			Name: &kubeApiserverCM.Name,
			MountPath: func() *string {
				v := "/etc/kubernetes/encryption-config.yaml"
				return &v
			}(),
			SubPath: func() *string {
				v := "encryption-config.yaml"
				return &v
			}(),
		},
		applycorev1.VolumeMountApplyConfiguration{
			Name: &kubeApiserverCM.Name,
			MountPath: func() *string {
				v := "/etc/kubernetes/audit-policy-minimal.yaml"
				return &v
			}(),
			SubPath: func() *string {
				v := "audit-policy-minimal.yaml"
				return &v
			}(),
		},
		applycorev1.VolumeMountApplyConfiguration{
			Name: func() *string {
				v := "audit-log-dir"
				return &v
			}(),
			MountPath: func() *string {
				v := "/var/log/kubernetes/"
				return &v
			}(),
		},
	)

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
					v := "kube-apiserver"
					return &v
				}(),
				Namespace: &namespace,
				Labels: map[string]string{
					"app":     "kube-apiserver",
					"project": ns.Labels["project"],
					"env":     ns.Labels["env"],
					"version": ns.Labels["kubernetes"],
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
					// audit-log-dir
					{
						Name: func() *string {
							v := "audit-log-dir"
							return &v
						}(),
						VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
							EmptyDir: &applycorev1.EmptyDirVolumeSourceApplyConfiguration{
								//Medium: nil,
								SizeLimit: func() *resource.Quantity {
									v := resource.MustParse("150M")
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
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
							Requests: &corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("20m"),
								corev1.ResourceMemory: resource.MustParse("250Mi"),
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
					{
						Name: func() *string {
							v := "fluent-bit"
							return &v
						}(),
						Image: func() *string {
							v := "fluent/fluent-bit:3.1.8"
							return &v
						}(),
						VolumeMounts: []applycorev1.VolumeMountApplyConfiguration{
							{
								Name: &kubeApiserverCM.Name,
								MountPath: func() *string {
									v := "/fluent-bit/etc/fluent-bit.conf"
									return &v
								}(),
								SubPath: func() *string {
									v := "fluent-bit.conf"
									return &v
								}(),
							},
							{
								Name: func() *string {
									v := "audit-log-dir"
									return &v
								}(),
								MountPath: func() *string {
									v := "/var/log/kubernetes/"
									return &v
								}(),
							},
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
					"version": ns.Labels["kubernetes"],
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
								corev1.ResourceCPU:    resource.MustParse("15m"),
								corev1.ResourceMemory: resource.MustParse("150Mi"),
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
					"version": ns.Labels["kubernetes"],
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

	err = controlLog(ns)

	if err != nil {
		fmt.Println(fmt.Sprintf("crd error: %s", err.Error()))
	}

	go plugin(info, ns.Name)

	//// todo create kibana index pattern
	//kib := kibana.New(viper.GetString("KIBANA_URL"))
	//kib.Index().Create()

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
		kubeControl.Service().Delete(ns.Name, "prometheus")
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
	kubeControl.Service().Delete(ns.Name, "kube-controller-manager")
	kubeControl.Service().Delete(ns.Name, "kube-scheduler")

	kubeControl.ConfigMaps().Delete(ns.Name, "kubeconfig")
	kubeControl.ConfigMaps().Delete(ns.Name, "kube-apiserver")
	kubeControl.ConfigMaps().Delete(ns.Name, "remote-access")
	kubeControl.ConfigMaps().Delete(ns.Name, "cluster-ca")

	kubeControl.Roles().Delete(ns.Name, "application:control-plane:etcd")
	kubeControl.RoleBindings().Delete(ns.Name, "application:control-plane:etcd")
	kubeControl.ServiceAccount().Delete(ns.Name, "etcd")
	kubeControl.Namespace().Delete(ns.Name)

	kubeControl.Crd("").Delete(ns.Name, schema.GroupVersionResource{
		Group:    "fluentd.fluent.io",
		Version:  "v1alpha1",
		Resource: "clusteroutputs",
	})
	kubeControl.Crd("").Delete(ns.Name, schema.GroupVersionResource{
		Group:    "fluentd.fluent.io",
		Version:  "v1alpha1",
		Resource: "clusterfluentdconfigs",
	})

	filename := fmt.Sprintf("./data/kubeconfig/%s.kubeconfig", ns.Name)
	_, err = os.Stat(filename)
	if err == nil {
		_ = os.Remove(filename)
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
	remoteAppMarket := appmarket.New(fmt.Sprintf("./data/kubeconfig/%s.kubeconfig", name))
	err := remoteAppMarket.Chart().Install("infra", "fluent-operator", "fluent-operator", false, "3.1.0", map[string]interface{}{
		"containerRuntime": "containerd",
		"fluentbit": map[string]interface{}{
			"enable": false,
			"input": map[string]interface{}{
				"tail": map[string]interface{}{
					"enable": false,
				},
				"systemd": map[string]interface{}{
					"enable": false,
				},
			},
			"filter": map[string]interface{}{
				"kubernetes": map[string]interface{}{
					"enable": false,
				},
				"containerd": map[string]interface{}{
					"enable": false,
				},
				"systemd": map[string]interface{}{
					"enable": false,
				},
			},
		},
	})
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
}
