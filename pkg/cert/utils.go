package cert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"time"
)

func genRsaKey() (prvkey, pubkey string) {
	// 生成私钥文件
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	derStream := x509.MarshalPKCS1PrivateKey(privateKey)
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: derStream,
	}
	tmp := pem.EncodeToMemory(block)
	prvkey = string(tmp)

	publicKey := &privateKey.PublicKey
	derPkix, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		panic(err)
	}
	block = &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: derPkix,
	}
	tmp = pem.EncodeToMemory(block)
	pubkey = string(tmp)
	return
}

func signature(CN string, NotBefore, NotAfter time.Time) (crt, key string) {
	pk, _ := rsa.GenerateKey(rand.Reader, 2048)
	tmp := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(pk)})

	key = string(tmp)

	k8sCrt, _ := generateCert(CN, NotBefore, NotAfter, pk)
	tmp = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: k8sCrt})
	crt = string(tmp)
	return
}

func generateCert(CN string, NotBefore time.Time, NotAfter time.Time, publicKey *rsa.PrivateKey) ([]byte, error) {
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	rootTemplate := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: CN,
		},
		NotBefore:             NotBefore,
		NotAfter:              NotAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{CN},
	}
	return x509.CreateCertificate(rand.Reader, &rootTemplate, &rootTemplate, &publicKey.PublicKey, publicKey)
}

func signatureFromCA(caCrt string, caKey string, csr *x509.Certificate) (crt, key string) {
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	csr.SerialNumber = serialNumber

	ca_Crt, ca_Key, err := loadPair(caCrt, caKey)

	serverPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	// 定义服务器证书模板
	serverCertBytes, err := x509.CreateCertificate(rand.Reader, csr, ca_Crt, &serverPriv.PublicKey, ca_Key)
	if err != nil {
		panic(err)
	}

	k := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverPriv)})
	key = string(k)

	c := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertBytes})
	crt = string(c)
	return
}

func loadPair(certFile, keyFile string) (caCert *x509.Certificate, caKey *rsa.PrivateKey, err error) {
	if len(certFile) == 0 && len(keyFile) == 0 {
		return nil, nil, errors.New("cert or key has not provided")
	}

	caCertBlock, _ := pem.Decode([]byte(certFile))
	caCert, err = x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		panic(err)
	}

	caKeyBlock, _ := pem.Decode([]byte(keyFile))
	caKey, err = x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes) // 正确解析 RSA 私钥
	if err != nil {
		panic(err)
	}
	return
}
