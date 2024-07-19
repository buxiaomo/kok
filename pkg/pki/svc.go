package pki

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
)

type svc struct {
	path string
	ca   string
	key  string
}

func (app pki) Svc(ca, key string) svc {
	return svc{
		path: app.path,
		ca:   ca,
		key:  key,
	}
}

func (app svc) Signature(name, cn string) {
	caCert, caKey, err := loadCA(fmt.Sprintf("%s/%s", app.path, app.ca), fmt.Sprintf("%s/%s", app.path, app.key))
	if err != nil {
		panic(err)
	}
	userPriv, _ := rsa.GenerateKey(rand.Reader, 2048)
	userPub := &userPriv.PublicKey
}

func loadCA(ca, key string) (*x509.Certificate, *rsa.PrivateKey, error) {
	caCertPEM, err := ioutil.ReadFile(ca)
	if err != nil {
		return nil, nil, err
	}

	caCertBlock, _ := pem.Decode(caCertPEM)
	if caCertBlock == nil || caCertBlock.Type != "CERTIFICATE" {
		return nil, nil, err
	}

	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}

	caKeyPEM, err := ioutil.ReadFile(key)
	if err != nil {
		return nil, nil, err
	}

	caKeyBlock, _ := pem.Decode(caKeyPEM)
	if caKeyBlock == nil || caKeyBlock.Type != "RSA PRIVATE KEY" {
		return nil, nil, err
	}

	caKey, err := x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}

	return caCert, caKey, nil
}
