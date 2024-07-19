package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/spf13/viper"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/watch"
	applyappsv1 "k8s.io/client-go/applyconfigurations/apps/v1"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	applynetworkingv1 "k8s.io/client-go/applyconfigurations/networking/v1"
	applypolicyv1 "k8s.io/client-go/applyconfigurations/policy/v1"
	applyrbacv1 "k8s.io/client-go/applyconfigurations/rbac/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"kok/pkg/utils"
	"kok/pkg/version"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func CreateKubeconfig(kubeconfig string) (config *rest.Config, err error) {
	if kubeconfig == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}
	_, err = os.Stat(kubeconfig)
	if err == nil {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
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

func New(kubeconfig string) *Kok {
	config, err := CreateKubeconfig(kubeconfig)
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

func (c Kok) ClusterInfo() {

}

func (c Kok) ScaleService(namespace string) (err error) {
	stsPatchBytes, _ := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": 3,
		},
	})
	_, err = c.clientset.AppsV1().StatefulSets(namespace).Patch(context.TODO(), "etcd", types.MergePatchType, stsPatchBytes, metav1.PatchOptions{})
	if err != nil {
		return err
	}

	deployPatchBytes, _ := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": 2,
		},
	})
	_, err = c.clientset.AppsV1().Deployments(namespace).Patch(context.TODO(), "kube-apiserver", types.MergePatchType, deployPatchBytes, metav1.PatchOptions{})
	if err != nil {
		return err
	}

	_, err = c.clientset.AppsV1().Deployments(namespace).Patch(context.TODO(), "kube-controller-manager", types.MergePatchType, deployPatchBytes, metav1.PatchOptions{})
	if err != nil {
		return err
	}

	_, err = c.clientset.AppsV1().Deployments(namespace).Patch(context.TODO(), "kube-scheduler", types.MergePatchType, deployPatchBytes, metav1.PatchOptions{})
	if err != nil {
		return err
	}

	return err
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

func (c Kok) CreateNS(name string, labels map[string]string) (namespace NameSpace, err error) {
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
				Name:   &name,
				Labels: labels,
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

func (c NameSpace) CreateSA(name string) error {
	_, err := c.clientset.CoreV1().ServiceAccounts(c.Name).Apply(context.TODO(), &applycorev1.ServiceAccountApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				name := "ServiceAccount"
				return &name
			}(),
			APIVersion: func() *string {
				name := "v1"
				return &name
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Name:      &name,
			Namespace: &c.Name,
			Labels: map[string]string{
				"app": "control-plane",
			},
		},
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
	return err
}

func (c NameSpace) CreateRBAC() error {
	_, err := c.clientset.RbacV1().Roles(c.Name).Apply(context.TODO(), &applyrbacv1.RoleApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				name := "Role"
				return &name
			}(),
			APIVersion: func() *string {
				name := "rbac.authorization.k8s.io/v1"
				return &name
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Name: func() *string {
				name := "application:control-plane"
				return &name
			}(),
			Namespace: &c.Name,
			Labels: map[string]string{
				"app": "control-plane",
			},
		},
		Rules: []applyrbacv1.PolicyRuleApplyConfiguration{
			{
				Verbs: []string{
					"patch",
					"get",
					"create",
				},
				APIGroups: []string{""},
				Resources: []string{
					"secrets",
				},
			},
		},
	},
		metav1.ApplyOptions{
			FieldManager: "kok",
		})
	if err != nil {
		return err
	}
	_, err = c.clientset.RbacV1().RoleBindings(c.Name).Apply(context.TODO(), &applyrbacv1.RoleBindingApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				name := "RoleBinding"
				return &name
			}(),
			APIVersion: func() *string {
				name := "rbac.authorization.k8s.io/v1"
				return &name
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Name: func() *string {
				name := "application:control-plane"
				return &name
			}(),
			Namespace: &c.Name,
			Labels: map[string]string{
				"app": "control-plane",
			},
		},
		Subjects: []applyrbacv1.SubjectApplyConfiguration{
			{
				Kind: func() *string {
					name := "ServiceAccount"
					return &name
				}(),
				APIGroup: func() *string {
					name := ""
					return &name
				}(),
				Name: func() *string {
					name := "control-plane"
					return &name
				}(),
				Namespace: &c.Name,
			},
		},
		RoleRef: &applyrbacv1.RoleRefApplyConfiguration{
			APIGroup: func() *string {
				name := "rbac.authorization.k8s.io"
				return &name
			}(),
			Kind: func() *string {
				name := "Role"
				return &name
			}(),
			Name: func() *string {
				name := "application:control-plane"
				return &name
			}(),
		},
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
	return err
}

func (c NameSpace) CreateIngress(dn string) (result *networkingv1.Ingress, err error) {
	return c.clientset.NetworkingV1().Ingresses(c.Name).Apply(context.TODO(), &applynetworkingv1.IngressApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				name := "Ingress"
				return &name
			}(),
			APIVersion: func() *string {
				name := "networking.k8s.io/v1"
				return &name
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Labels: map[string]string{
				"app": "control-plane",
			},
			Name: func() *string {
				name := "control-plane"
				return &name
			}(),
			Namespace: &c.Name,
		},
		Spec: &applynetworkingv1.IngressSpecApplyConfiguration{
			//IngressClassName: nil,
			//DefaultBackend: &applynetworkingv1.IngressBackendApplyConfiguration{
			//	Service: &applynetworkingv1.IngressServiceBackendApplyConfiguration{
			//		Name: func() *string {
			//			name := "control-plane"
			//			return &name
			//		}(),
			//		Port: &applynetworkingv1.ServiceBackendPortApplyConfiguration{
			//			Name: func() *string {
			//				name := "https"
			//				return &name
			//			}(),
			//			Number: func() *int32 {
			//				name := int32(6443)
			//				return &name
			//			}(),
			//		},
			//	},
			//},
			TLS: []applynetworkingv1.IngressTLSApplyConfiguration{
				{
					Hosts: []string{
						fmt.Sprintf("%s.%s", c.Name, dn),
					},
					SecretName: func() *string {
						name := "control-plane"
						return &name
					}(),
				},
			},
			Rules: []applynetworkingv1.IngressRuleApplyConfiguration{
				{
					Host: func() *string {
						name := fmt.Sprintf("%s.%s", c.Name, dn)
						return &name
					}(),
					IngressRuleValueApplyConfiguration: applynetworkingv1.IngressRuleValueApplyConfiguration{
						HTTP: &applynetworkingv1.HTTPIngressRuleValueApplyConfiguration{
							Paths: []applynetworkingv1.HTTPIngressPathApplyConfiguration{
								{
									Path: func() *string {
										name := "/"
										return &name
									}(),
									PathType: func() *networkingv1.PathType {
										name := networkingv1.PathTypePrefix
										return &name
									}(),
									Backend: &applynetworkingv1.IngressBackendApplyConfiguration{
										Service: &applynetworkingv1.IngressServiceBackendApplyConfiguration{
											Name: func() *string {
												name := "control-plane"
												return &name
											}(),
											Port: &applynetworkingv1.ServiceBackendPortApplyConfiguration{
												Number: func() *int32 {
													name := int32(6443)
													return &name
												}(),
											},
										},
										Resource: nil,
									},
								},
							},
						},
					},
				},
			},
		},
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
}

func (c Kok) NamespaceList() (*corev1.NamespaceList, error) {
	return c.clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{
		LabelSelector: "fieldManager=control-plane",
	})
}

func (c Kok) GetNamespace(name string) (*corev1.Namespace, error) {
	return c.clientset.CoreV1().Namespaces().Get(context.TODO(), name, metav1.GetOptions{})
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
				name := corev1.ServiceTypeLoadBalancer
				//name := corev1.ServiceTypeClusterIP
				return &name
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

func (c NameSpace) CresteSTS(registry, ver string) {
	v := version.GetVersion(ver)
	c.clientset.AppsV1().StatefulSets(c.Name).Apply(context.TODO(), &applyappsv1.StatefulSetApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				name := "StatefulSet"
				return &name
			}(),
			APIVersion: func() *string {
				name := "apps/v1"
				return &name
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Name: func() *string {
				name := "etcd"
				return &name
			}(),
			Namespace: &c.Name,
			Labels: map[string]string{
				"app": "etcd",
			},
		},
		Spec: &applyappsv1.StatefulSetSpecApplyConfiguration{
			Replicas: func() *int32 {
				i := int32(3)
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
					Namespace: &c.Name,
					Labels: map[string]string{
						"app": "etcd",
					},
				},
				Spec: &applycorev1.PodSpecApplyConfiguration{
					Volumes: []applycorev1.VolumeApplyConfiguration{
						{
							Name: func() *string {
								name := "etcd-vol"
								return &name
							}(),
							VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
								PersistentVolumeClaim: &applycorev1.PersistentVolumeClaimVolumeSourceApplyConfiguration{
									ClaimName: func() *string {
										name := "etcd-vol"
										return &name
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
								a := fmt.Sprintf("%s/etcd:%s", registry, v["etcd"])
								return &a
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
							Lifecycle: &applycorev1.LifecycleApplyConfiguration{
								PreStop: &applycorev1.LifecycleHandlerApplyConfiguration{
									Exec: &applycorev1.ExecActionApplyConfiguration{
										Command: []string{
											"/bin/sh",
											"-ec",
											"|",
											`export ETCDCTL_API=3
MEMBER_MAX=$(etcdctl --cacert /etc/kubernetes/pki/etcd/ca.crt --cert /etc/kubernetes/pki/etcd/healthcheck-client.crt --key /etc/kubernetes/pki/etcd/healthcheck-client.key member list | wc -l)
EPS=""
for i in $(seq 0 $((${MEMBER_MAX} - 1))); do
    EPS="${EPS}${EPS:+,}http://etcd-${i}.etcd:2379"
done

member_hash() {
    etcdctl member list | grep http://$(hostname).etcd:2380 | cut -d':' -f1 | cut -d'[' -f1
}

SET_ID=${HOSTNAME##*[^0-9]}
if [ $CLUSTER_SIZE -lt $INITIAL_CLUSTER_SIZE ]
  echo "Removing ${HOSTNAME} from etcd cluster"
  ETCDCTL_ENDPOINT=${EPS} etcdctl member remove $(member_hash)
  if [ $? -eq 0 ]; then
      rm -rf /var/run/etcd/*
  fi
fi`,
										},
									},
								},
							},
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
							Resources:     nil,
							RestartPolicy: nil,
							VolumeMounts: []applycorev1.VolumeMountApplyConfiguration{
								{
									Name: func() *string {
										name := "etcd-vol"
										return &name
									}(),
									MountPath: func() *string {
										name := "/var/lib/etcd"
										return &name
									}(),
									SubPath: func() *string {
										name := "data-vol"
										return &name
									}(),
								},
								{
									Name: func() *string {
										name := "etcd-vol"
										return &name
									}(),
									MountPath: func() *string {
										name := "/etc/kubernetes/pki"
										return &name
									}(),
									SubPath: func() *string {
										name := "pki-vol"
										return &name
									}(),
								},
							},
						},
					},
					EphemeralContainers:           nil,
					RestartPolicy:                 nil,
					TerminationGracePeriodSeconds: nil,
					ActiveDeadlineSeconds:         nil,
					DNSPolicy:                     nil,
					NodeSelector:                  nil,
					ServiceAccountName:            nil,
					DeprecatedServiceAccount:      nil,
					AutomountServiceAccountToken:  nil,
					NodeName:                      nil,
					HostNetwork:                   nil,
					HostPID:                       nil,
					HostIPC:                       nil,
					ShareProcessNamespace:         nil,
					SecurityContext:               nil,
					ImagePullSecrets:              nil,
					Hostname:                      nil,
					Subdomain:                     nil,
					Affinity:                      nil,
					SchedulerName:                 nil,
					Tolerations:                   nil,
					HostAliases:                   nil,
					PriorityClassName:             nil,
					Priority:                      nil,
					DNSConfig:                     nil,
					ReadinessGates:                nil,
					RuntimeClassName:              nil,
					EnableServiceLinks:            nil,
					PreemptionPolicy:              nil,
					Overhead:                      nil,
					TopologySpreadConstraints:     nil,
					SetHostnameAsFQDN:             nil,
					OS:                            nil,
					HostUsers:                     nil,
					SchedulingGates:               nil,
					ResourceClaims:                nil,
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
								corev1.ResourceStorage: resource.MustParse("1Gi"),
							},
						},
					},
				},
			},
			//ServiceName:                          nil,
			//PodManagementPolicy:                  nil,
			//UpdateStrategy:                       nil,
			//RevisionHistoryLimit:                 nil,
			//MinReadySeconds:                      nil,
			//PersistentVolumeClaimRetentionPolicy: nil,
			//Ordinals:                             nil,
		},
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
}

func (c NameSpace) CreateDeploy(name, registry, ver string, externalIp *string, serviceCidr, podCidr, nodePort string) {
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
						ServiceAccountName: func() *string {
							name := "control-plane"
							return &name
						}(),
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

func (c Kok) DeleteAll(namespace string) (err error) {
	err = c.clientset.AppsV1().Deployments(namespace).Delete(context.TODO(), "kube-scheduler", metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	err = c.clientset.AppsV1().Deployments(namespace).Delete(context.TODO(), "kube-controller-manager", metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	err = c.clientset.AppsV1().Deployments(namespace).Delete(context.TODO(), "kube-apiserver", metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	err = c.clientset.AppsV1().StatefulSets(namespace).Delete(context.TODO(), "etcd", metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	err = c.clientset.CoreV1().Services(namespace).Delete(context.TODO(), "kube-apiserver", metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	err = c.clientset.CoreV1().Services(namespace).Delete(context.TODO(), "etcd", metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	err = c.clientset.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), "kubeconfig", metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	err = c.clientset.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), "kube-apiserver", metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	err = c.clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(context.TODO(), "pki-vol", metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	//err = c.clientset.NetworkingV1().Ingresses(namespace).Delete(context.TODO(), "control-plane", metav1.DeleteOptions{})
	//if err != nil {
	//	panic(err.Error())
	//}
	//
	//err = c.clientset.CoreV1().Secrets(namespace).Delete(context.TODO(), "control-plane", metav1.DeleteOptions{})
	//if err != nil {
	//	panic(err.Error())
	//}

	err = c.clientset.RbacV1().RoleBindings(namespace).Delete(context.TODO(), "application:control-plane:etcd", metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	err = c.clientset.RbacV1().Roles(namespace).Delete(context.TODO(), "application:control-plane:etcd", metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	err = c.clientset.CoreV1().ServiceAccounts(namespace).Delete(context.TODO(), "etcd", metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	err = c.clientset.PolicyV1().PodDisruptionBudgets(namespace).Delete(context.TODO(), "etcd", metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	err = c.clientset.CoreV1().Namespaces().Delete(context.TODO(), namespace, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	return nil
}

func (c Kok) Node() error {
	w, err := c.clientset.CoreV1().Nodes().Watch(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	ch := w.ResultChan()
	for event := range ch {
		switch event.Type {
		case watch.Added, watch.Modified:
			node := event.Object.(*corev1.Node)
			for _, condition := range node.Status.Conditions {
				if condition.Type == "Ready" && condition.Status != "True" {

					podList, _ := c.clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
						FieldSelector: "spec.nodeName=" + node.Name,
						LabelSelector: "app=etcd",
					})
					for _, pod := range podList.Items {
						ns, _ := c.GetNamespace(pod.Namespace)
						if ns.Labels["fieldManager"] == "control-plane" {
							err := c.DeletePod(pod.Namespace, pod.Name)
							fmt.Printf("Node %s is NotReady, delete pod: %s in namesapce: %s, err: %v\n", node.Name, pod.Name, pod.Namespace, err)
						}
					}
				}
			}
		case watch.Deleted:
			fmt.Printf("Node deleted: %s\n", event.Object.(*corev1.Node).GetName())
			fmt.Println(event.Object.(*corev1.Node).Status)
		case watch.Error:
			fmt.Printf("Error: %v\n", event.Object)
		}
	}
	return err
}

func (c Kok) DeletePod(namespace, name string) (err error) {
	_, err = c.clientset.CoreV1().Pods(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err == nil {
		c.clientset.CoreV1().Pods(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{
			TypeMeta: metav1.TypeMeta{},
			GracePeriodSeconds: func() *int64 {
				v := int64(0)
				return &v
			}(),
			PropagationPolicy: func() *metav1.DeletionPropagation {
				v := metav1.DeletePropagationForeground
				return &v
			}(),
		})
		return nil
	}
	return err
	//c.clientset.AppsV1().Deployments(namespace).Delete(context.TODO(), "control-plane", metav1.DeleteOptions{})
	//c.clientset.CoreV1().Services(namespace).Delete(context.TODO(), "control-plane", metav1.DeleteOptions{})
	//c.clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(context.TODO(), "control-plane", metav1.DeleteOptions{})
	//c.clientset.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), "kube-proxy", metav1.DeleteOptions{})
	//c.clientset.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), "kubeconfig", metav1.DeleteOptions{})
	//c.clientset.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), "kube-apiserver", metav1.DeleteOptions{})
	//c.clientset.CoreV1().Namespaces().Delete(context.TODO(), namespace, metav1.DeleteOptions{})
}

func (c Kok) CreateAll(registry, ver, project, env, nodePort, serviceSubnet, podSubnet string) (err error) {
	v := version.GetVersion(ver)
	namespace := fmt.Sprintf("%s-%s", project, env)
	clusterDNS, _ := utils.GetCidrIpRange(strings.Replace(serviceSubnet, "-", "/", 1))
	lbAddr := ""

	// kube-apiserver svc
	kubeapiserverSvc, err := c.clientset.CoreV1().Services(namespace).Apply(context.TODO(), &applycorev1.ServiceApplyConfiguration{
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
				v := "kube-apiserver"
				return &v
			}(),
			Namespace: &namespace,
		},
		Spec: &applycorev1.ServiceSpecApplyConfiguration{
			Ports: []applycorev1.ServicePortApplyConfiguration{
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
			},
			Selector: map[string]string{
				"app": "kube-apiserver",
			},
			Type: func() *corev1.ServiceType {
				name := corev1.ServiceTypeLoadBalancer
				return &name
			}(),
		},
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
	if err != nil {
		return err
	}

	timeout := time.After(time.Minute * 2)
	finish := make(chan bool)
	count := 1
	go func() {
		for {
			select {
			case <-timeout:
				c.clientset.CoreV1().Services(namespace).Delete(context.TODO(), kubeapiserverSvc.Name, metav1.DeleteOptions{})
				fmt.Println("wait timeout, delete service")
				err = errors.New("wait timeout")
				finish <- true
				return
			default:
				svc, err := c.clientset.CoreV1().Services(namespace).Get(context.TODO(), kubeapiserverSvc.Name, metav1.GetOptions{})
				if err != nil {
					fmt.Println(err.Error())
					//panic(err.Error())
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
	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]string{
				"loadBalancer": lbAddr,
			},
		},
	}
	patchBytes, _ := json.Marshal(patch)
	_, err = c.clientset.CoreV1().Namespaces().Patch(context.TODO(), namespace, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return err
	}

	// etcd sa
	etcdSa, err := c.clientset.CoreV1().ServiceAccounts(namespace).Apply(context.TODO(), &applycorev1.ServiceAccountApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				name := "ServiceAccount"
				return &name
			}(),
			APIVersion: func() *string {
				name := "v1"
				return &name
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Name: func() *string {
				name := "etcd"
				return &name
			}(),
			Namespace: &namespace,
		},
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
	if err != nil {
		return err
	}

	// etcd role
	etcdRole, err := c.clientset.RbacV1().Roles(namespace).Apply(context.TODO(), &applyrbacv1.RoleApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				name := "Role"
				return &name
			}(),
			APIVersion: func() *string {
				name := "rbac.authorization.k8s.io/v1"
				return &name
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Name: func() *string {
				name := "application:control-plane:etcd"
				return &name
			}(),
			Namespace: &namespace,
		},
		Rules: []applyrbacv1.PolicyRuleApplyConfiguration{
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
		},
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
	if err != nil {
		return err
	}

	// etcd rolebinding
	_, err = c.clientset.RbacV1().RoleBindings(namespace).Apply(context.TODO(), &applyrbacv1.RoleBindingApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				name := "RoleBinding"
				return &name
			}(),
			APIVersion: func() *string {
				name := "rbac.authorization.k8s.io/v1"
				return &name
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Name: func() *string {
				name := "application:control-plane:etcd"
				return &name
			}(),
			Namespace: &namespace,
		},
		Subjects: []applyrbacv1.SubjectApplyConfiguration{
			{
				Kind: func() *string {
					name := "ServiceAccount"
					return &name
				}(),
				Name:      &etcdSa.Name,
				Namespace: &namespace,
			},
		},
		RoleRef: &applyrbacv1.RoleRefApplyConfiguration{
			APIGroup: func() *string {
				name := "rbac.authorization.k8s.io"
				return &name
			}(),
			Kind: func() *string {
				name := "Role"
				return &name
			}(),
			Name: &etcdRole.Name,
		},
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
	if err != nil {
		return err
	}

	// pki pvc
	pkiPvc, err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).Apply(context.TODO(), &applycorev1.PersistentVolumeClaimApplyConfiguration{
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
			Name: func() *string {
				name := "pki-vol"
				return &name
			}(),
			Namespace: &namespace,
		},
		Spec: &applycorev1.PersistentVolumeClaimSpecApplyConfiguration{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			Resources: &applycorev1.VolumeResourceRequirementsApplyConfiguration{
				Requests: &corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1G"),
				},
			},
		},
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
	if err != nil {
		return err
	}

	// etcd svc
	etcdSvc, err := c.clientset.CoreV1().Services(namespace).Apply(context.TODO(), &applycorev1.ServiceApplyConfiguration{
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
				name := "etcd"
				return &name
			}(),
			Namespace: &namespace,
		},
		Spec: &applycorev1.ServiceSpecApplyConfiguration{
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
		},
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
	if err != nil {
		return err
	}

	// etcd pdb
	_, err = c.clientset.PolicyV1().PodDisruptionBudgets(namespace).Apply(context.TODO(),
		&applypolicyv1.PodDisruptionBudgetApplyConfiguration{
			TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
				Kind: func() *string {
					v := "PodDisruptionBudget"
					return &v
				}(),
				APIVersion: func() *string {
					v := "policy/v1"
					return &v
				}(),
			},
			ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
				Name: func() *string {
					v := "etcd"
					return &v
				}(),
				Namespace: &namespace,
			},
			Spec: &applypolicyv1.PodDisruptionBudgetSpecApplyConfiguration{
				MinAvailable: &intstr.IntOrString{
					Type:   0,
					IntVal: 1,
					StrVal: "1",
				},
				Selector: &applymetav1.LabelSelectorApplyConfiguration{
					MatchLabels: map[string]string{
						"app": "etcd",
					},
				},
			},
		},
		metav1.ApplyOptions{
			FieldManager: "kok",
		})
	if err != nil {
		return err
	}

	// etcd sts
	_, err = c.clientset.AppsV1().StatefulSets(namespace).Apply(context.TODO(), &applyappsv1.StatefulSetApplyConfiguration{
		TypeMetaApplyConfiguration: applymetav1.TypeMetaApplyConfiguration{
			Kind: func() *string {
				name := "StatefulSet"
				return &name
			}(),
			APIVersion: func() *string {
				name := "apps/v1"
				return &name
			}(),
		},
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Name: func() *string {
				name := "etcd"
				return &name
			}(),
			Namespace: &namespace,
			Labels: map[string]string{
				"app": "etcd",
			},
		},
		Spec: &applyappsv1.StatefulSetSpecApplyConfiguration{
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
					Namespace: &namespace,
					Labels: map[string]string{
						"app":          "etcd",
						"FieldManager": "kok",
					},
				},
				Spec: &applycorev1.PodSpecApplyConfiguration{
					ServiceAccountName: &etcdSa.Name,
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
						{
							Name: func() *string {
								name := "pki-vol"
								return &name
							}(),
							VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
								PersistentVolumeClaim: &applycorev1.PersistentVolumeClaimVolumeSourceApplyConfiguration{
									ClaimName: &pkiPvc.Name,
								},
							},
						},
					},
					InitContainers: []applycorev1.ContainerApplyConfiguration{
						{
							Name: func() *string {
								v := "init-pki"
								return &v
							}(),
							Image: func() *string {
								v := "buxiaomo/kube-pki:1.0.0"
								//v := "buxiaomo/openssl:3.3.1"
								return &v
							}(),
							ImagePullPolicy: func() *corev1.PullPolicy {
								v := corev1.PullAlways
								return &v
							}(),
							//WorkingDir: func() *string {
							//	v := "/etc/kubernetes/pki"
							//	return &v
							//}(),
							VolumeMounts: []applycorev1.VolumeMountApplyConfiguration{
								{
									Name: &pkiPvc.Name,
									MountPath: func() *string {
										v := "/etc/kubernetes/pki"
										return &v
									}(),
								},
							},
							Env: []applycorev1.EnvVarApplyConfiguration{
								{
									Name: func() *string {
										v := "EXTERNAL_IP"
										return &v
									}(),
									Value: &lbAddr,
								},
								{
									Name: func() *string {
										v := "PROJECT"
										return &v
									}(),
									Value: &project,
								},
								{
									Name: func() *string {
										v := "ENV"
										return &v
									}(),
									Value: &env,
								},
								{
									Name: func() *string {
										v := "WEBHOOK_URL"
										return &v
									}(),
									Value: func() *string {
										a := viper.GetString("WEBHOOK_URL")
										return &a
									}(),
								},
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
								{
									Name: func() *string {
										v := "CLUSTERDNS"
										return &v
									}(),
									Value: &clusterDNS,
								},
							},
						},
						{
							Name: func() *string {
								v := "cp-pki"
								return &v
							}(),
							Image: func() *string {
								v := "alpine:3.20.1"
								return &v
							}(),
							ImagePullPolicy: func() *corev1.PullPolicy {
								v := corev1.PullAlways
								return &v
							}(),
							VolumeMounts: []applycorev1.VolumeMountApplyConfiguration{
								{
									Name: func() *string {
										v := "etcd-vol"
										return &v
									}(),
									MountPath: func() *string {
										v := "/etc/kubernetes/pki/etcd"
										return &v
									}(),
									SubPath: func() *string {
										v := "pki-vol"
										return &v
									}(),
								},
								{
									Name: &pkiPvc.Name,
									MountPath: func() *string {
										v := "/mnt/pki-vol"
										return &v
									}(),
								},
							},
							Command: []string{
								"sh",
								"-xc",
								"cp -vfr /mnt/pki-vol/etcd/* /etc/kubernetes/pki/etcd",
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
								a := fmt.Sprintf("buxiaomo/etcd:%s", v["etcd"])
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
							VolumeMounts: []applycorev1.VolumeMountApplyConfiguration{
								{
									Name: func() *string {
										name := "etcd-vol"
										return &name
									}(),
									MountPath: func() *string {
										name := "/var/lib/etcd"
										return &name
									}(),
									SubPath: func() *string {
										name := "data-vol"
										return &name
									}(),
								},
								{
									Name: func() *string {
										v := "etcd-vol"
										return &v
									}(),
									MountPath: func() *string {
										name := "/etc/kubernetes/pki/etcd"
										return &name
									}(),
									SubPath: func() *string {
										name := "pki-vol"
										return &name
									}(),
								},
							},
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
							RestartPolicy: nil,
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
		},
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
	if err != nil {
		return err
	}

	// kube-apiserver cm
	kubeApiserverCM, err := c.clientset.CoreV1().ConfigMaps(namespace).Apply(context.TODO(), &applycorev1.ConfigMapApplyConfiguration{
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
			Namespace: &namespace,
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
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
	if err != nil {
		return err
	}

	// kube-apiserver deployment
	_, err = c.clientset.AppsV1().Deployments(namespace).Apply(context.TODO(), &applyappsv1.DeploymentApplyConfiguration{
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
			Name: func() *string {
				v := "kube-apiserver"
				return &v
			}(),
			Namespace: &namespace,
			Labels: map[string]string{
				"app": "kube-apiserver",
			},
		},
		Spec: &applyappsv1.DeploymentSpecApplyConfiguration{
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
						"version": v["kubernetes"],
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
								a := "pki-vol"
								return &a
							}(),
							VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
								PersistentVolumeClaim: &applycorev1.PersistentVolumeClaimVolumeSourceApplyConfiguration{
									ClaimName: func() *string {
										a := "pki-vol"
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
										Name: &kubeApiserverCM.Name,
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
								fmt.Sprintf("--service-cluster-ip-range=%s", serviceSubnet),
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
							Resources: &applycorev1.ResourceRequirementsApplyConfiguration{
								Limits: &corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("150m"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
								Requests: &corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("15m"),
									corev1.ResourceMemory: resource.MustParse("200Mi"),
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
							VolumeMounts: []applycorev1.VolumeMountApplyConfiguration{
								{
									Name: &pkiPvc.Name,
									MountPath: func() *string {
										a := "/etc/kubernetes/pki"
										return &a
									}(),
								},
								{
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
								{
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
		},
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
	if err != nil {
		return err
	}

	// kubeconfigCM
	kubeconfigCM, err := c.clientset.CoreV1().ConfigMaps(namespace).Apply(context.TODO(), &applycorev1.ConfigMapApplyConfiguration{
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
				a := "kubeconfig"
				return &a
			}(),
			Namespace: &namespace,
		},
		Data: map[string]string{
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
		},
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
	if err != nil {
		return err
	}

	// kube-controller-manager deployment
	_, err = c.clientset.AppsV1().Deployments(namespace).Apply(context.TODO(), &applyappsv1.DeploymentApplyConfiguration{
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
			Name: func() *string {
				v := "kube-controller-manager"
				return &v
			}(),
			Namespace: &namespace,
			Labels: map[string]string{
				"app": "kube-controller-manager",
			},
		},
		Spec: &applyappsv1.DeploymentSpecApplyConfiguration{
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
						"version": v["kubernetes"],
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
							Name: func() *string {
								a := "pki-vol"
								return &a
							}(),
							VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
								PersistentVolumeClaim: &applycorev1.PersistentVolumeClaimVolumeSourceApplyConfiguration{
									ClaimName: func() *string {
										a := "pki-vol"
										return &a
									}(),
								},
							},
						},
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
								a := fmt.Sprintf("%s/kube-controller-manager:%s", registry, v["kubernetes"])
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
								fmt.Sprintf("--cluster-cidr=%s", podSubnet),
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
								fmt.Sprintf("--service-cluster-ip-range=%s", serviceSubnet),
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
									corev1.ResourceMemory: resource.MustParse("800Mi"),
								},
								Requests: &corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("80Mi"),
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
							VolumeMounts: []applycorev1.VolumeMountApplyConfiguration{
								{
									Name: &pkiPvc.Name,
									MountPath: func() *string {
										a := "/etc/kubernetes/pki"
										return &a
									}(),
								},
								{
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
								},
							},
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
		},
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
	if err != nil {
		return err
	}

	// kube-scheduler deployment
	_, err = c.clientset.AppsV1().Deployments(namespace).Apply(context.TODO(), &applyappsv1.DeploymentApplyConfiguration{
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
			Name: func() *string {
				v := "kube-scheduler"
				return &v
			}(),
			Namespace: &namespace,
			Labels: map[string]string{
				"app": "kube-scheduler",
			},
		},
		Spec: &applyappsv1.DeploymentSpecApplyConfiguration{
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
						"version": v["kubernetes"],
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
							Name: func() *string {
								a := "pki-vol"
								return &a
							}(),
							VolumeSourceApplyConfiguration: applycorev1.VolumeSourceApplyConfiguration{
								PersistentVolumeClaim: &applycorev1.PersistentVolumeClaimVolumeSourceApplyConfiguration{
									ClaimName: func() *string {
										a := "pki-vol"
										return &a
									}(),
								},
							},
						},
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
					},
					Containers: []applycorev1.ContainerApplyConfiguration{
						{
							Name: func() *string {
								a := "kube-scheduler"
								return &a
							}(),
							Image: func() *string {
								v := fmt.Sprintf("%s/kube-scheduler:%s", registry, v["kubernetes"])
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
									corev1.ResourceMemory: resource.MustParse("200Mi"),
								},
								Requests: &corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("5m"),
									corev1.ResourceMemory: resource.MustParse("20Mi"),
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
							VolumeMounts: []applycorev1.VolumeMountApplyConfiguration{
								{
									Name: &pkiPvc.Name,
									MountPath: func() *string {
										a := "/etc/kubernetes/pki"
										return &a
									}(),
								},
								{
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
								},
							},
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
		},
	}, metav1.ApplyOptions{
		FieldManager: "kok",
	})
	if err != nil {
		return err
	}

	return nil
}

func (c Kok) GetDeployment(namespace, name string) (*v1.Deployment, error) {
	return c.clientset.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c Kok) GetConfigMap(namespace, name string) (*corev1.ConfigMap, error) {
	//return c.clientset.AppsV1().(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	return c.clientset.CoreV1().ConfigMaps(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c Kok) GetDaemonSets(namespace, name string) (*v1.DaemonSet, error) {
	return c.clientset.AppsV1().DaemonSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c Kok) DeploymentStatus(namespace, name string) (*v1.Deployment, error) {
	return c.clientset.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}
