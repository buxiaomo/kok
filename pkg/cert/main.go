package cert

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"math"
	"math/big"
	"net"
	"time"
)

type TlsList struct {
	SaPub                     string
	SaKey                     string
	CaCrt                     string
	CaKey                     string
	ApiServerCrt              string
	ApiServerKey              string
	ApiserverKubeletClientCrt string
	ApiserverKubeletClientKey string
	ApiserverEtcdClientCrt    string
	ApiserverEtcdClientKey    string
	ControllerManagerCrt      string
	ControllerManagerKey      string
	SchedulerCrt              string
	SchedulerKey              string
	AdminCrt                  string
	AdminKey                  string
	EtcdCrt                   string
	EtcdKey                   string
	EtcdServerCrt             string
	EtcdServerKey             string
	EtcdPeerCrt               string
	EtcdPeerKey               string
	EtcdHealthcheckClientCrt  string
	EtcdHealthcheckClientKey  string
	FrontProxyCrt             string
	FrontProxyKey             string
	FrontProxyClientCrt       string
	FrontProxyClientKey       string
}

type cert struct{}

func New() *cert {
	return &cert{}
}

func (p cert) GenerateAll(y time.Duration, Project, Env, ExternalIp, ClusterDNS string) *TlsList {
	tl := &TlsList{}
	NotBefore := time.Now()
	NotAfter := NotBefore.Add(time.Hour * 24 * 36 * y)
	tl.SaKey, tl.SaPub = genRsaKey()
	tl.CaCrt, tl.CaKey = signature("kubernetes-ca", NotBefore, NotAfter)
	tl.AdminCrt, tl.AdminKey = signatureFromCA(tl.CaCrt, tl.CaKey, &x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{"system:masters"},
			CommonName:   "kubernetes-admin",
		},
		NotBefore:   NotBefore,
		NotAfter:    NotAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	})

	tl.SchedulerCrt, tl.SchedulerKey = signatureFromCA(tl.CaCrt, tl.CaKey, &x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{"system:kube-scheduler"},
			CommonName:   "system:kube-scheduler",
		},
		NotBefore:   NotBefore,
		NotAfter:    NotAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})

	tl.ControllerManagerCrt, tl.ControllerManagerKey = signatureFromCA(tl.CaCrt, tl.CaKey, &x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{"system:kube-controller-manager"},
			CommonName:   "system:kube-controller-manager",
		},
		NotBefore:   NotBefore,
		NotAfter:    NotAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})

	tl.ApiServerCrt, tl.ApiServerKey = signatureFromCA(tl.CaCrt, tl.CaKey, &x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{"Kubernetes"},
			CommonName:   "kube-apiserver",
		},
		NotBefore:   NotBefore,
		NotAfter:    NotAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{"kube-apiserver", "localhost", "kubernetes", "kubernetes.default", "kubernetes.default.svc", "kubernetes.default.svc.cluster", "kubernetes.default.svc.cluster.local"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1"), net.ParseIP(ExternalIp), net.ParseIP(ClusterDNS)},
	})

	tl.EtcdCrt, tl.EtcdKey = signature("etcd-ca", NotBefore, NotAfter)
	tl.EtcdServerCrt, tl.EtcdServerKey = signatureFromCA(tl.EtcdCrt, tl.EtcdKey, &x509.Certificate{
		NotBefore:   NotBefore,
		NotAfter:    NotAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames: []string{
			fmt.Sprintf("etcd-0.etcd.%s-%s", Project, Env),
			fmt.Sprintf("etcd-1.etcd.%s-%s", Project, Env),
			fmt.Sprintf("etcd-2.etcd.%s-%s", Project, Env),
			fmt.Sprintf("etcd-3.etcd.%s-%s", Project, Env),
			fmt.Sprintf("etcd-4.etcd.%s-%s", Project, Env),
		},
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
			net.ParseIP("::1"),
		},
	})
	tl.EtcdPeerCrt, tl.EtcdPeerKey = signatureFromCA(tl.EtcdCrt, tl.EtcdKey, &x509.Certificate{
		NotBefore:   NotBefore,
		NotAfter:    NotAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames: []string{
			fmt.Sprintf("etcd-0.etcd.%s-%s", Project, Env),
			fmt.Sprintf("etcd-1.etcd.%s-%s", Project, Env),
			fmt.Sprintf("etcd-2.etcd.%s-%s", Project, Env),
			fmt.Sprintf("etcd-3.etcd.%s-%s", Project, Env),
			fmt.Sprintf("etcd-4.etcd.%s-%s", Project, Env),
		},
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
			net.ParseIP("::1"),
		},
	})
	tl.EtcdHealthcheckClientCrt, tl.EtcdHealthcheckClientKey = signatureFromCA(tl.EtcdCrt, tl.EtcdKey, &x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{"system:masters"},
			CommonName:   "kube-etcd-healthcheck-client",
		},
		NotBefore:   NotBefore,
		NotAfter:    NotAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	tl.ApiserverKubeletClientCrt, tl.ApiserverKubeletClientKey = signatureFromCA(tl.EtcdCrt, tl.EtcdKey, &x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{"system:masters"},
			CommonName:   "kube-apiserver-kubelet-client",
		},
		NotBefore:   NotBefore,
		NotAfter:    NotAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	tl.ApiserverEtcdClientCrt, tl.ApiserverEtcdClientKey = signatureFromCA(tl.EtcdCrt, tl.EtcdKey, &x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{"system:masters"},
			CommonName:   "apiserver-etcd-client",
		},
		NotBefore:   NotBefore,
		NotAfter:    NotAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})

	tl.FrontProxyCrt, tl.FrontProxyKey = signature("front-proxy-ca", NotBefore, NotAfter)
	tl.FrontProxyClientCrt, tl.FrontProxyClientKey = signatureFromCA(tl.FrontProxyCrt, tl.FrontProxyKey, &x509.Certificate{
		NotBefore:   NotBefore,
		NotAfter:    NotAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})

	return tl
}

func CreateCACert() (*x509.Certificate, crypto.Signer, error) {
	key, err := GeneratePrivateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unable to create private key while generating CA certificate")
	}

	//serial, err := cryptorand.Int(cryptorand.Reader, new(big.Int).SetInt64(math.MaxInt64-1))
	serial, err := rand.Int(rand.Reader, new(big.Int).SetInt64(math.MaxInt64-1))
	if err != nil {
		return nil, nil, err
	}

	serial = new(big.Int).Add(serial, big.NewInt(1))
	keyUsage := x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "cfg.CommonName",
			Organization: []string{"cfg.Organization"},
		},
		//DNSNames:              []string{cfg.CommonName},
		//NotBefore:             notBefore,
		//NotAfter:              notAfter,
		KeyUsage:              keyUsage,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	certDERBytes, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, key.Public(), key)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(certDERBytes)
	return cert, key, err
}

func GeneratePrivateKey(random io.Reader, bits int) (crypto.Signer, error) {
	return rsa.GenerateKey(random, bits)
}

func CreateKubeconfigFileForRestConfig(restConfig rest.Config, path string) ([]byte, error) {
	clusters := make(map[string]*clientcmdapi.Cluster)
	clusters["kubernetes"] = &clientcmdapi.Cluster{
		Server:                   restConfig.Host,
		CertificateAuthorityData: restConfig.CAData,
	}
	contexts := make(map[string]*clientcmdapi.Context)
	contexts["default"] = &clientcmdapi.Context{
		Cluster:  "kubernetes",
		AuthInfo: "admin",
	}
	authinfos := make(map[string]*clientcmdapi.AuthInfo)
	authinfos["admin"] = &clientcmdapi.AuthInfo{
		ClientCertificateData: restConfig.CertData,
		ClientKeyData:         restConfig.KeyData,
	}
	clientConfig := clientcmdapi.Config{
		Kind:           "Config",
		APIVersion:     "v1",
		Clusters:       clusters,
		Contexts:       contexts,
		CurrentContext: "default",
		AuthInfos:      authinfos,
	}
	//kubeConfigFile, _ := os.CreateTemp("", "kubeconfig")
	_ = clientcmd.WriteToFile(clientConfig, path)
	return clientcmd.Write(clientConfig)
	//return kubeConfigFile.Name()
}
