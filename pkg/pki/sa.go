package pki

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

type sa struct {
	path string
}

func (app pki) Sa() sa {
	return sa{
		path: app.path,
	}
}

func (app sa) Signature() {
	// 生成私钥文件
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	derStream := x509.MarshalPKCS1PrivateKey(privateKey)

	sk, err := os.Create(fmt.Sprintf("%s/sa.key", app.path))
	if err != nil {
		panic(err)
	}
	pem.Encode(sk, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: derStream,
	})
	sk.Close()

	pub, err := os.Create(fmt.Sprintf("%s/sa.pub", app.path))
	publicKey := &privateKey.PublicKey
	derPkix, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		panic(err)
	}
	pem.Encode(pub, &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: derPkix,
	})
	pub.Close()
}
