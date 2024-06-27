package control

import (
	"context"
	"fmt"
	"github.com/spf13/viper"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	applyappsv1 "k8s.io/client-go/applyconfigurations/apps/v1"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"kok/pkg/version"
	"log"
	"os"
	"path/filepath"
	"time"
)

func CreateKubeconfig() (config *rest.Config, err error) {
	KubeconfigPath := ""
	if home := homedir.HomeDir(); home != "" {
		KubeconfigPath = filepath.Join(home, ".kube", "config")
	}
	_, err = os.Stat(KubeconfigPath)
	if err == nil {
		config, err = clientcmd.BuildConfigFromFlags("", KubeconfigPath)
		return
	} else {
		config, err = rest.InClusterConfig()
		return
	}
}

type Kok struct {
	clientset kubernetes.Interface
	//namespace string
	version  map[string]string
	registry string
}

type NameSpace struct {
	Name      string
	clientset kubernetes.Interface
}

func New() *Kok {
	config, err := CreateKubeconfig()
	if err != nil {
		panic(err.Error())
	}
	c, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	return &Kok{
		clientset: c,
		//namespace: namespace,
		//registry: registry,
		//version:  v,
	}
}

func (c Kok) HasDefaultSC() bool {
	l, err := c.clientset.StorageV1().StorageClasses().List(context.TODO(), metav1.ListOptions{
		TypeMeta: metav1.TypeMeta{},
	})
	if err != nil {
		panic(err.Error())
	}
	for _, sc := range l.Items {
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			return true
		}
	}
	return false
}

func (c Kok) CreateNS(name string) (namespace NameSpace, err error) {
	ns, err := c.clientset.CoreV1().Namespaces().Apply(
		context.TODO(),
		&applycorev1.NamespaceApplyConfiguration{
			TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
				Kind: func() *string {
					a := "Namespace"
					return &a
				}(),
				APIVersion: func() *string {
					a := "v1"
					return &a
				}(),
			},
			ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
				Name: &name,
				Labels: map[string]string{
					"app": "control-plane",
				},
			},
		},
		metav1.ApplyOptions{
			FieldManager: "kok",
		})
	if err != nil {
		log.Printf(err.Error())
	}
	return NameSpace{
		Name:      ns.Name,
		clientset: c.clientset,
	}, err
	//return ns.Name
}

func (c Kok) GetInstance() (*corev1.PodList, error) {
	return c.clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		LabelSelector: "app=control-plane",
	})
}
func (c NameSpace) CreateKubeconfig() error {
	_, err := c.clientset.CoreV1().ConfigMaps(c.Name).Apply(
		context.TODO(),
		&applycorev1.ConfigMapApplyConfiguration{
			TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
				Kind: func() *string {
					kind := "ConfigMap"
					return &kind
				}(),
				APIVersion: func() *string {
					kind := "v1"
					return &kind
				}(),
			},
			ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
				Labels: map[string]string{
					"app": "control-plane",
				},
				Name: func() *string {
					a := "kubeconfig"
					return &a
				}(),
				Namespace: &c.Name,
			},
			Data: map[string]string{
				"kube-proxy.kubeconfig": `apiVersion: v1
clusters:
  - cluster:
      certificate-authority: /etc/kubernetes/pki/ca.crt
      server: https://127.0.0.1:6443
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
      server: https://127.0.0.1:6443
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
      server: https://127.0.0.1:6443
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
			},
		},
		metav1.ApplyOptions{
			FieldManager: "kok",
		})
	return err
}

func (c NameSpace) CreateKubeproxyConfig() error {
	_, err := c.clientset.CoreV1().ConfigMaps(c.Name).Apply(
		context.TODO(),
		&applycorev1.ConfigMapApplyConfiguration{
			TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
				Kind: func() *string {
					kind := "ConfigMap"
					return &kind
				}(),
				APIVersion: func() *string {
					kind := "v1"
					return &kind
				}(),
			},
			ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
				Name: func() *string {
					a := "kube-proxy"
					return &a
				}(),
				Labels: map[string]string{
					"app": "control-plane",
				},
				Namespace: &c.Name,
			},
			Data: map[string]string{
				"kube-proxy.yaml": `apiVersion: kubeproxy.config.k8s.io/v1alpha1
kind: KubeProxyConfiguration
bindAddress: "0.0.0.0"
metricsBindAddress: "0.0.0.0:10249"
clientConnection:
  acceptContentTypes: ""
  burst: 10
  contentType: application/vnd.kubernetes.protobuf
  kubeconfig: /etc/kubernetes/kube-proxy.kubeconfig
  qps: 5
clusterCIDR: "10.96.0.0/12"
configSyncPeriod: 15m0s
conntrack:
  max: null
  maxPerCore: 32768
  min: 131072
  tcpCloseWaitTimeout: 1h0m0s
  tcpEstablishedTimeout: 24h0m0s
enableProfiling: false
healthzBindAddress: 127.0.0.1:10256
iptables:
  masqueradeAll: true
  masqueradeBit: 14
  minSyncPeriod: 0s
  syncPeriod: 30s
ipvs:
  strictARP: true
  excludeCIDRs: null
  minSyncPeriod: 5s
  scheduler: "wrr"
  syncPeriod: 30s
mode: "ipvs"
nodePortAddresses: null
oomScoreAdj: -999
portRange: ""
resourceContainer: /kube-proxy
udpIdleTimeout: 250ms
`,
			},
		},
		metav1.ApplyOptions{
			FieldManager: "kok",
		})
	return err
}

func (c NameSpace) CreateKubeApiserverConfig() error {
	_, err := c.clientset.CoreV1().ConfigMaps(c.Name).Apply(
		context.TODO(),
		&applycorev1.ConfigMapApplyConfiguration{
			TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
				Kind: func() *string {
					kind := "ConfigMap"
					return &kind
				}(),
				APIVersion: func() *string {
					kind := "v1"
					return &kind
				}(),
			},
			ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
				Name: func() *string {
					a := "kube-apiserver"
					return &a
				}(),
				Namespace: &c.Name,
				Labels: map[string]string{
					"app": "control-plane",
				},
			},
			Data: map[string]string{
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
			},
		},
		metav1.ApplyOptions{
			FieldManager: "kok",
		})
	return err
}

func (c NameSpace) CreatePVC(name string) error {
	//name := "nfs-client"
	_, err := c.clientset.CoreV1().PersistentVolumeClaims(c.Name).Apply(context.TODO(),
		&applycorev1.PersistentVolumeClaimApplyConfiguration{
			TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
				Kind: func() *string {
					kind := "PersistentVolumeClaim"
					return &kind
				}(),
				APIVersion: func() *string {
					kind := "v1"
					return &kind
				}(),
			},
			ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
				Name:      &name,
				Namespace: &c.Name,
				Labels: map[string]string{
					"app": "control-plane",
				},
			},
			Spec: &applycorev1.PersistentVolumeClaimSpecApplyConfiguration{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: &applycorev1.VolumeResourceRequirementsApplyConfiguration{
					//Limits: nil,
					Requests: &corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("10G"),
					},
				},
				//StorageClassName: &name,
			},
		},
		metav1.ApplyOptions{
			FieldManager: "kok",
		},
	)
	return err
}

func (c NameSpace) CreateSvc() *string {
	_, err := c.clientset.CoreV1().Services(c.Name).Apply(context.TODO(), &applycorev1.ServiceApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				kind := "Service"
				return &kind
			}(),
			APIVersion: func() *string {
				APIVersion := "v1"
				return &APIVersion
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Name: func() *string {
				name := "control-plane"
				return &name
			}(),
			Namespace: &c.Name,
			Labels: map[string]string{
				"app": "control-plane",
			},
		},
		Spec: &applycorev1.ServiceSpecApplyConfiguration{
			Ports: []applycorev1.ServicePortApplyConfiguration{
				{
					Name: func() *string {
						mode := "http"
						return &mode
					}(),
					Port: func() *int32 {
						mode := int32(80)
						return &mode
					}(),
					TargetPort: &intstr.IntOrString{
						Type:   0,
						IntVal: 80,
						StrVal: "80",
					},
				},
				{
					Name: func() *string {
						mode := "https"
						return &mode
					}(),
					Port: func() *int32 {
						mode := int32(443)
						return &mode
					}(),
					TargetPort: &intstr.IntOrString{
						Type:   0,
						IntVal: 6443,
						StrVal: "6443",
					},
				},
				{
					Name: func() *string {
						mode := "kube-apiserver"
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
				{
					Name: func() *string {
						mode := "kube-controller-manager"
						return &mode
					}(),
					Port: func() *int32 {
						mode := int32(10252)
						return &mode
					}(),
					TargetPort: &intstr.IntOrString{
						Type:   0,
						IntVal: 10252,
						StrVal: "10252",
					},
				},
				{
					Name: func() *string {
						mode := "kube-scheduler"
						return &mode
					}(),
					Port: func() *int32 {
						mode := int32(10251)
						return &mode
					}(),
					TargetPort: &intstr.IntOrString{
						Type:   0,
						IntVal: 10251,
						StrVal: "10251",
					},
				},
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
			Selector: map[string]string{
				"app": "control-plane",
			},
			Type: func() *corev1.ServiceType {
				x := corev1.ServiceTypeLoadBalancer
				return &x
			}(),
		},
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
	if err != nil {
		log.Printf("Create svc error: %s", err.Error())
		return nil
	}
	for {
		svc, err := c.clientset.CoreV1().Services(c.Name).Get(context.TODO(), "control-plane", metav1.GetOptions{})
		if err != nil {
			panic(err.Error())
		}
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			log.Printf("Service external IP is: %s\n", svc.Status.LoadBalancer.Ingress[0].IP)
			return &svc.Status.LoadBalancer.Ingress[0].IP
		}
		log.Println("Waiting for external IP...")
		time.Sleep(10 * time.Second)
	}

	return nil
}

func (c NameSpace) CreateDeploy(name, registry, ver string, externalIp *string, serviceCidr, podCidr, nodePort string) {
	fmt.Println(viper.GetString("WEBHOOK_URL"))
	v := version.GetVersion(ver)
	_, err := c.clientset.AppsV1().Deployments(c.Name).Apply(
		context.TODO(),
		&applyappsv1.DeploymentApplyConfiguration{
			TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
				Kind: func() *string {
					a := "Deployment"
					return &a
				}(),
				APIVersion: func() *string {
					a := "apps/v1"
					return &a
				}(),
			},
			ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
				Name:      &name,
				Namespace: &c.Name,
				Labels: map[string]string{
					"app": "control-plane",
				},
			},
			Spec: &applyappsv1.DeploymentSpecApplyConfiguration{
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
						"app": "control-plane",
					},
				},
				Template: &applycorev1.PodTemplateSpecApplyConfiguration{
					ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
						Name: func() *string {
							a := "control-plane"
							return &a
						}(),
						Namespace: &c.Name,
						Labels: map[string]string{
							"app":     "control-plane",
							"version": v["kubernetes"],
						},
					},
					Spec: &applycorev1.PodSpecApplyConfiguration{
						Volumes: []applycorev1.VolumeApplyConfiguration{
							{
								Name: func() *string {
									a := "control-plane-vol"
									return &a
								}(),
								VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
									PersistentVolumeClaim: &applycorev1.PersistentVolumeClaimVolumeSourceApplyConfiguration{
										ClaimName: func() *string {
											a := "control-plane-vol"
											return &a
										}(),
									},
								},
							},
							{
								Name: func() *string {
									a := "kube-apiserver"
									return &a
								}(),
								VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
									ConfigMap: &applycorev1.ConfigMapVolumeSourceApplyConfiguration{
										LocalObjectReferenceApplyConfiguration: applycorev1.LocalObjectReferenceApplyConfiguration{
											Name: func() *string {
												a := "kube-apiserver"
												return &a
											}(),
										},
										DefaultMode: func() *int32 {
											a := int32(0755)
											return &a
										}(),
									},
								},
							},
							{
								Name: func() *string {
									a := "kube-proxy"
									return &a
								}(),
								VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
									ConfigMap: &applycorev1.ConfigMapVolumeSourceApplyConfiguration{
										LocalObjectReferenceApplyConfiguration: applycorev1.LocalObjectReferenceApplyConfiguration{
											Name: func() *string {
												a := "kube-proxy"
												return &a
											}(),
										},
										DefaultMode: func() *int32 {
											a := int32(0755)
											return &a
										}(),
									},
								},
							},
							{
								Name: func() *string {
									a := "kubeconfig"
									return &a
								}(),
								VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
									ConfigMap: &applycorev1.ConfigMapVolumeSourceApplyConfiguration{
										LocalObjectReferenceApplyConfiguration: applycorev1.LocalObjectReferenceApplyConfiguration{
											Name: func() *string {
												a := "kubeconfig"
												return &a
											}(),
										},
										DefaultMode: func() *int32 {
											a := int32(0755)
											return &a
										}(),
									},
								},
							},
						},
						InitContainers: []applycorev1.ContainerApplyConfiguration{
							{
								Name: func() *string {
									a := "init-pki"
									return &a
								}(),
								Image: func() *string {
									a := "buxiaomo/kube-pki:1.0"
									return &a
								}(),
								ImagePullPolicy: func() *corev1.PullPolicy {
									a := corev1.PullAlways
									return &a
								}(),
								Env: []applycorev1.EnvVarApplyConfiguration{
									{
										Name: func() *string {
											a := "EXTERNAL_IP"
											return &a
										}(),
										Value: externalIp,
									},
									{
										Name: func() *string {
											a := "WEBHOOK_URL"
											return &a
										}(),
										Value: func() *string {
											a := fmt.Sprintf("%s/kubeconfig", viper.GetString("WEBHOOK_URL"))
											return &a
										}(),
									},
									{
										Name: func() *string {
											a := "PROJECT_NAME"
											return &a
										}(),
										Value: &c.Name,
									},
									{
										Name: func() *string {
											a := "serviceSubnet"
											return &a
										}(),
										Value: &serviceCidr,
									},
								},
								VolumeMounts: []applycorev1.VolumeMountApplyConfiguration{
									{
										Name: func() *string {
											a := "control-plane-vol"
											return &a
										}(),
										MountPath: func() *string {
											a := "/etc/kubernetes/pki"
											return &a
										}(),
										SubPath: func() *string {
											a := "pki-vol"
											return &a
										}(),
									},
								},
							},
						},
						Containers: []applycorev1.ContainerApplyConfiguration{
							// nginx
							{
								Name: func() *string {
									a := "nginx"
									return &a
								}(),
								Image: func() *string {
									a := "nginx:1.27.0-alpine"
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
											a := int32(80)
											return &a
										}(),
										Protocol: func() *corev1.Protocol {
											a := corev1.ProtocolTCP
											return &a
										}(),
									},
								},
								LivenessProbe: &applycorev1.ProbeApplyConfiguration{
									ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
										TCPSocket: &applycorev1.TCPSocketActionApplyConfiguration{
											Port: &intstr.IntOrString{
												Type:   0,
												IntVal: 80,
												StrVal: "80",
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
								Resources: &applycorev1.ResourceRequirementsApplyConfiguration{
									Limits: nil,
									Requests: &corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("100Mi"),
									},
								},
								VolumeMounts: []applycorev1.VolumeMountApplyConfiguration{
									{
										Name: func() *string {
											a := "control-plane-vol"
											return &a
										}(),
										MountPath: func() *string {
											a := "/usr/share/nginx/html"
											return &a
										}(),
										SubPath: func() *string {
											a := "pki-vol"
											return &a
										}(),
									},
								},
							},
							// etcd
							{
								Name: func() *string {
									a := "etcd"
									return &a
								}(),
								Image: func() *string {
									a := fmt.Sprintf("%s/etcd:%s", registry, v["etcd"])
									return &a
								}(),
								ImagePullPolicy: func() *corev1.PullPolicy {
									a := corev1.PullIfNotPresent
									return &a
								}(),
								Command: []string{
									"etcd",
									"--name=etcd-0",
									"--data-dir=/var/lib/etcd",
									"--listen-client-urls=https://127.0.0.1:2379,https://127.0.0.1:2379",
									"--advertise-client-urls=https://127.0.0.1:2379",
									"--listen-peer-urls=https://127.0.0.1:2380",
									"--initial-advertise-peer-urls=https://127.0.0.1:2380",
									"--initial-cluster=etcd-0=https://127.0.0.1:2380",
									"--initial-cluster-token=kubernetes-etcd-cluster",
									"--client-cert-auth=true",
									"--trusted-ca-file=/etc/kubernetes/pki/etcd/ca.crt",
									"--cert-file=/etc/kubernetes/pki/etcd/server.crt",
									"--key-file=/etc/kubernetes/pki/etcd/server.key",
									"--peer-client-cert-auth=true",
									"--peer-trusted-ca-file=/etc/kubernetes/pki/etcd/ca.crt",
									"--peer-cert-file=/etc/kubernetes/pki/etcd/peer.crt",
									"--peer-key-file=/etc/kubernetes/pki/etcd/peer.key",
									"--listen-metrics-urls=http://0.0.0.0:2381",
									"--auto-compaction-retention=1",
									"--max-request-bytes=33554432",
									"--quota-backend-bytes=8589934592",
									"--enable-v2=false",
									"--snapshot-count=10000",
									"--cipher-suites=TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_RSA_WITH_AES_128_CBC_SHA,TLS_RSA_WITH_AES_128_GCM_SHA256,TLS_RSA_WITH_AES_256_CBC_SHA,TLS_RSA_WITH_AES_256_GCM_SHA384",
								},
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
								StartupProbe: &applycorev1.ProbeApplyConfiguration{
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
										a := int32(24)
										return &a
									}(),
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
								Resources: &applycorev1.ResourceRequirementsApplyConfiguration{
									Limits: nil,
									Requests: &corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("100Mi"),
									},
								},
								VolumeMounts: []applycorev1.VolumeMountApplyConfiguration{
									{
										Name: func() *string {
											a := "control-plane-vol"
											return &a
										}(),
										MountPath: func() *string {
											a := "/var/lib/etcd"
											return &a
										}(),
										SubPath: func() *string {
											a := "etcd-vol"
											return &a
										}(),
									}, {
										Name: func() *string {
											a := "control-plane-vol"
											return &a
										}(),
										MountPath: func() *string {
											a := "/etc/kubernetes/pki"
											return &a
										}(),
										SubPath: func() *string {
											a := "pki-vol"
											return &a
										}(),
									},
								},
							},
							// kube-apiserver
							{
								Name: func() *string {
									a := "kube-apiserver"
									return &a
								}(),
								Image: func() *string {
									a := fmt.Sprintf("%s/kube-apiserver:%s", registry, v["kubernetes"])
									return &a
								}(),
								ImagePullPolicy: func() *corev1.PullPolicy {
									a := corev1.PullIfNotPresent
									return &a
								}(),
								Command: []string{
									"kube-apiserver",
									"--anonymous-auth=true",
									//"--authorization-mode=AlwaysAllow",
									"--authorization-mode=Node,RBAC",
									fmt.Sprintf("--advertise-address=%s", *externalIp),
									"--enable-aggregator-routing=true",
									"--allow-privileged=true",
									"--client-ca-file=/etc/kubernetes/pki/ca.crt",
									"--enable-bootstrap-token-auth",
									"--storage-backend=etcd3",
									"--etcd-cafile=/etc/kubernetes/pki/etcd/ca.crt",
									"--etcd-certfile=/etc/kubernetes/pki/apiserver-etcd-client.crt",
									"--etcd-keyfile=/etc/kubernetes/pki/apiserver-etcd-client.key",
									"--etcd-servers=https://127.0.0.1:2379",
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
									fmt.Sprintf("--service-cluster-ip-range=%s", serviceCidr),
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
									fmt.Sprintf("--service-node-port-range=%s", nodePort),
									"--runtime-config=api/all=true",
									"--profiling=false",
									"--enable-admission-plugins=ServiceAccount,NamespaceLifecycle,NodeRestriction,LimitRanger,PersistentVolumeClaimResize,DefaultStorageClass,DefaultTolerationSeconds,MutatingAdmissionWebhook,ValidatingAdmissionWebhook,ResourceQuota,Priority",
									"--tls-cipher-suites=TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,TLS_RSA_WITH_AES_256_GCM_SHA384,TLS_RSA_WITH_AES_128_GCM_SHA256",
									"--v=1",
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
								StartupProbe: &applycorev1.ProbeApplyConfiguration{
									ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
										//HTTPGet: &applycorev1.HTTPGetActionApplyConfiguration{
										//	Path: func() *string {
										//		a := "/livez"
										//		return &a
										//	}(),
										//	Port: &intstr.IntOrString{
										//		Type:   0,
										//		IntVal: 6443,
										//		StrVal: "6443",
										//	},
										//	Scheme: func() *corev1.URIScheme {
										//		a := corev1.URISchemeHTTPS
										//		return &a
										//	}(),
										//},
										TCPSocket: &applycorev1.TCPSocketActionApplyConfiguration{
											Port: &intstr.IntOrString{
												Type:   0,
												IntVal: 6443,
												StrVal: "6443",
											},
										},
									},
									TimeoutSeconds: func() *int32 {
										a := int32(15)
										return &a
									}(),
									PeriodSeconds: func() *int32 {
										a := int32(10)
										return &a
									}(),
									FailureThreshold: func() *int32 {
										a := int32(24)
										return &a
									}(),
									InitialDelaySeconds: func() *int32 {
										a := int32(10)
										return &a
									}(),
								},
								LivenessProbe: &applycorev1.ProbeApplyConfiguration{
									ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
										TCPSocket: &applycorev1.TCPSocketActionApplyConfiguration{
											Port: &intstr.IntOrString{
												Type:   0,
												IntVal: 6443,
												StrVal: "6443",
											},
										},
										//HTTPGet: &applycorev1.HTTPGetActionApplyConfiguration{
										//	Path: func() *string {
										//		a := "/livez"
										//		return &a
										//	}(),
										//	Port: &intstr.IntOrString{
										//		Type:   0,
										//		IntVal: 6443,
										//		StrVal: "6443",
										//	},
										//	Scheme: func() *corev1.URIScheme {
										//		a := corev1.URISchemeHTTPS
										//		return &a
										//	}(),
										//},
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
										//HTTPGet: &applycorev1.HTTPGetActionApplyConfiguration{
										//	Path: func() *string {
										//		a := "/readyz"
										//		return &a
										//	}(),
										//	Port: &intstr.IntOrString{
										//		Type:   0,
										//		IntVal: 6443,
										//		StrVal: "6443",
										//	},
										//	Scheme: func() *corev1.URIScheme {
										//		a := corev1.URISchemeHTTPS
										//		return &a
										//	}(),
										//},
									},
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
								Resources: &applycorev1.ResourceRequirementsApplyConfiguration{
									Limits: nil,
									Requests: &corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse("250m"),
									},
								},
								VolumeMounts: []applycorev1.VolumeMountApplyConfiguration{
									{
										Name: func() *string {
											a := "control-plane-vol"
											return &a
										}(),
										MountPath: func() *string {
											a := "/etc/kubernetes/pki"
											return &a
										}(),
										SubPath: func() *string {
											a := "pki-vol"
											return &a
										}(),
									},
									{
										Name: func() *string {
											a := "kube-apiserver"
											return &a
										}(),
										MountPath: func() *string {
											a := "/etc/kubernetes/encryption-config.yaml"
											return &a
										}(),
										SubPath: func() *string {
											a := "encryption-config.yaml"
											return &a
										}(),
									}, {
										Name: func() *string {
											a := "kube-apiserver"
											return &a
										}(),
										MountPath: func() *string {
											a := "/etc/kubernetes/audit-policy-minimal.yaml"
											return &a
										}(),
										SubPath: func() *string {
											a := "audit-policy-minimal.yaml"
											return &a
										}(),
									},
								},
							},
							// kube-controller-manager
							{
								Name: func() *string {
									a := "kube-controller-manager"
									return &a
								}(),
								Image: func() *string {
									a := fmt.Sprintf("%s/kube-controller-manager:%s", registry, v["kubernetes"])
									return &a
								}(),
								ImagePullPolicy: func() *corev1.PullPolicy {
									a := corev1.PullIfNotPresent
									return &a
								}(),
								Command: []string{
									"kube-controller-manager",
									"--bind-address=0.0.0.0",
									"--allocate-node-cidrs=true",
									"--authentication-kubeconfig=/etc/kubernetes/kube-controller-manager.kubeconfig",
									"--authorization-kubeconfig=/etc/kubernetes/kube-controller-manager.kubeconfig",
									"--client-ca-file=/etc/kubernetes/pki/ca.crt",
									fmt.Sprintf("--cluster-cidr=%s", podCidr),
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
									fmt.Sprintf("--service-cluster-ip-range=%s", serviceCidr),
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
								SecurityContext: &applycorev1.SecurityContextApplyConfiguration{
									SeccompProfile: &applycorev1.SeccompProfileApplyConfiguration{
										Type: func() *corev1.SeccompProfileType {
											a := corev1.SeccompProfileTypeRuntimeDefault
											return &a
										}(),
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
								LivenessProbe: &applycorev1.ProbeApplyConfiguration{
									ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
										//TCPSocket: &applycorev1.TCPSocketActionApplyConfiguration{
										//	Port: &intstr.IntOrString{
										//		Type:   0,
										//		IntVal: 10257,
										//		StrVal: "10257",
										//	},
										//},
										HTTPGet: &applycorev1.HTTPGetActionApplyConfiguration{
											Path: func() *string {
												a := "/healthz"
												return &a
											}(),
											Port: &intstr.IntOrString{
												Type:   0,
												IntVal: 10257,
												StrVal: "10257",
											},
											Scheme: func() *corev1.URIScheme {
												a := corev1.URISchemeHTTPS
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
										//TCPSocket: &applycorev1.TCPSocketActionApplyConfiguration{
										//	Port: &intstr.IntOrString{
										//		Type:   0,
										//		IntVal: 10257,
										//		StrVal: "10257",
										//	},
										//},
										HTTPGet: &applycorev1.HTTPGetActionApplyConfiguration{
											Path: func() *string {
												a := "/healthz"
												return &a
											}(),
											Port: &intstr.IntOrString{
												Type:   0,
												IntVal: 10257,
												StrVal: "10257",
											},
											Scheme: func() *corev1.URIScheme {
												a := corev1.URISchemeHTTPS
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
								StartupProbe: &applycorev1.ProbeApplyConfiguration{
									ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
										//TCPSocket: &applycorev1.TCPSocketActionApplyConfiguration{
										//	Port: &intstr.IntOrString{
										//		Type:   0,
										//		IntVal: 10257,
										//		StrVal: "10257",
										//	},
										//},
										HTTPGet: &applycorev1.HTTPGetActionApplyConfiguration{
											Path: func() *string {
												a := "/healthz"
												return &a
											}(),
											Port: &intstr.IntOrString{
												Type:   0,
												IntVal: 10257,
												StrVal: "10257",
											},
											Scheme: func() *corev1.URIScheme {
												a := corev1.URISchemeHTTPS
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
										a := int32(24)
										return &a
									}(),
								},
								Resources: &applycorev1.ResourceRequirementsApplyConfiguration{
									Limits: nil,
									Requests: &corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse("200m"),
									},
								},
								VolumeMounts: []applycorev1.VolumeMountApplyConfiguration{
									{
										Name: func() *string {
											a := "control-plane-vol"
											return &a
										}(),
										MountPath: func() *string {
											a := "/etc/kubernetes/pki"
											return &a
										}(),
										SubPath: func() *string {
											a := "pki-vol"
											return &a
										}(),
									}, {
										Name: func() *string {
											a := "kubeconfig"
											return &a
										}(),
										MountPath: func() *string {
											a := "/etc/kubernetes/kube-controller-manager.kubeconfig"
											return &a
										}(),
										SubPath: func() *string {
											a := "kube-controller-manager.kubeconfig"
											return &a
										}(),
										ReadOnly: new(bool),
									},
								},
							},
							// kube-scheduler
							{
								Name: func() *string {
									a := "kube-scheduler"
									return &a
								}(),
								Image: func() *string {
									a := fmt.Sprintf("%s/kube-scheduler:%s", registry, v["kubernetes"])
									return &a
								}(),
								ImagePullPolicy: func() *corev1.PullPolicy {
									a := corev1.PullIfNotPresent
									return &a
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
								SecurityContext: &applycorev1.SecurityContextApplyConfiguration{
									SeccompProfile: &applycorev1.SeccompProfileApplyConfiguration{
										Type: func() *corev1.SeccompProfileType {
											a := corev1.SeccompProfileTypeRuntimeDefault
											return &a
										}(),
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
								StartupProbe: &applycorev1.ProbeApplyConfiguration{
									ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
										//TCPSocket: &applycorev1.TCPSocketActionApplyConfiguration{
										//	Port: &intstr.IntOrString{
										//		Type:   0,
										//		IntVal: 10257,
										//		StrVal: "10257",
										//	},
										//},
										HTTPGet: &applycorev1.HTTPGetActionApplyConfiguration{
											Path: func() *string {
												a := "/healthz"
												return &a
											}(),
											Port: &intstr.IntOrString{
												Type:   0,
												IntVal: 10257,
												StrVal: "10257",
											},
											Scheme: func() *corev1.URIScheme {
												a := corev1.URISchemeHTTPS
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
										a := int32(24)
										return &a
									}(),
								},
								LivenessProbe: &applycorev1.ProbeApplyConfiguration{
									ProbeHandlerApplyConfiguration: applycorev1.ProbeHandlerApplyConfiguration{
										//TCPSocket: &applycorev1.TCPSocketActionApplyConfiguration{
										//	Port: &intstr.IntOrString{
										//		Type:   0,
										//		IntVal: 10259,
										//		StrVal: "10259",
										//	},
										//},
										HTTPGet: &applycorev1.HTTPGetActionApplyConfiguration{
											Path: func() *string {
												a := "/healthz"
												return &a
											}(),
											Port: &intstr.IntOrString{
												Type:   0,
												IntVal: 10259,
												StrVal: "10259",
											},
											Scheme: func() *corev1.URIScheme {
												a := corev1.URISchemeHTTPS
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
										//TCPSocket: &applycorev1.TCPSocketActionApplyConfiguration{
										//	Port: &intstr.IntOrString{
										//		Type:   0,
										//		IntVal: 10259,
										//		StrVal: "10259",
										//	},
										//},
										HTTPGet: &applycorev1.HTTPGetActionApplyConfiguration{
											Path: func() *string {
												a := "/healthz"
												return &a
											}(),
											Port: &intstr.IntOrString{
												Type:   0,
												IntVal: 10259,
												StrVal: "10259",
											},
											Scheme: func() *corev1.URIScheme {
												a := corev1.URISchemeHTTPS
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
								Resources: &applycorev1.ResourceRequirementsApplyConfiguration{
									Limits: nil,
									Requests: &corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse("200m"),
									},
								},
								VolumeMounts: []applycorev1.VolumeMountApplyConfiguration{
									{
										Name: func() *string {
											a := "control-plane-vol"
											return &a
										}(),
										MountPath: func() *string {
											a := "/etc/kubernetes/pki"
											return &a
										}(),
										SubPath: func() *string {
											a := "pki-vol"
											return &a
										}(),
									}, {
										Name: func() *string {
											a := "kubeconfig"
											return &a
										}(),
										MountPath: func() *string {
											a := "/etc/kubernetes/kube-scheduler.kubeconfig"
											return &a
										}(),
										SubPath: func() *string {
											a := "kube-scheduler.kubeconfig"
											return &a
										}(),
										ReadOnly: new(bool),
									},
								},
							},
						},
					},
				},
			},
		},
		metav1.ApplyOptions{
			FieldManager: "kok",
		})
	if err != nil {
		panic(err.Error())
	}
}

func (c Kok) DeleteAll(namespace string) {
	err := c.clientset.AppsV1().Deployments(namespace).Delete(context.TODO(), "control-plane", metav1.DeleteOptions{})
	if err != nil {
		panic(err.Error())
	}

	err = c.clientset.CoreV1().Services(namespace).Delete(context.TODO(), "control-plane", metav1.DeleteOptions{})
	if err != nil {
		panic(err.Error())
	}

	err = c.clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(context.TODO(), "control-plane-vol", metav1.DeleteOptions{})
	if err != nil {
		panic(err.Error())
	}

	err = c.clientset.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), "kube-proxy", metav1.DeleteOptions{})
	if err != nil {
		panic(err.Error())
	}

	err = c.clientset.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), "kubeconfig", metav1.DeleteOptions{})
	if err != nil {
		panic(err.Error())
	}

	err = c.clientset.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), "kube-apiserver", metav1.DeleteOptions{})
	if err != nil {
		panic(err.Error())
	}

	err = c.clientset.CoreV1().Namespaces().Delete(context.TODO(), namespace, metav1.DeleteOptions{})
	if err != nil {
		panic(err.Error())
	}
}

func (c Kok) DeletePod(namespace, name string) error {
	return c.clientset.CoreV1().Pods(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	//c.clientset.AppsV1().Deployments(namespace).Delete(context.TODO(), "control-plane", metav1.DeleteOptions{})
	//c.clientset.CoreV1().Services(namespace).Delete(context.TODO(), "control-plane", metav1.DeleteOptions{})
	//c.clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(context.TODO(), "control-plane", metav1.DeleteOptions{})
	//c.clientset.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), "kube-proxy", metav1.DeleteOptions{})
	//c.clientset.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), "kubeconfig", metav1.DeleteOptions{})
	//c.clientset.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), "kube-apiserver", metav1.DeleteOptions{})
	//c.clientset.CoreV1().Namespaces().Delete(context.TODO(), namespace, metav1.DeleteOptions{})
}

func (c Kok) Status(namespace string) (*v1.Deployment, error) {
	return c.clientset.AppsV1().Deployments(namespace).Get(context.TODO(), "control-plane", metav1.GetOptions{})
}
