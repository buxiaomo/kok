package pki

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"
)

type ca struct {
	path string
}

func (app pki) Ca() ca {
	return ca{
		path: app.path,
	}
}

func (app ca) Signature(years int, cn, name string) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}

	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	derBytes, err := x509.CreateCertificate(rand.Reader,
		&x509.Certificate{
			SerialNumber: serialNumber,
			Subject: pkix.Name{
				CommonName: cn,
			},
			Issuer: pkix.Name{
				CommonName: cn,
			},
			NotBefore:             time.Now(),
			NotAfter:              time.Now().AddDate(years, 0, 0),
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
			BasicConstraintsValid: true,
			IsCA:                  true,
		},
		&x509.Certificate{
			SerialNumber: serialNumber,
			Subject: pkix.Name{
				CommonName: cn,
			},
			Issuer: pkix.Name{
				CommonName: cn,
			},
			NotBefore:             time.Now(),
			NotAfter:              time.Now().AddDate(years, 0, 0),
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
			BasicConstraintsValid: true,
			IsCA:                  true,
		}, &priv.PublicKey, priv)
	if err != nil {
		panic(err)
	}

	certOut, err := os.Create(fmt.Sprintf("%s/%s.crt", app.path, name))
	if err != nil {
		panic(err)
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()

	// 保存私钥到文件
	keyOut, err := os.Create(fmt.Sprintf("%s/%s.key", app.path, name))
	if err != nil {
		panic(err)
	}

	pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	keyOut.Close()
}
