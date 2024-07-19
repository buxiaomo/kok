package control

import (
	"context"
	"crypto/sha256"
	"fmt"
	"go.etcd.io/etcd/client/pkg/v3/fileutil"
	"go.etcd.io/etcd/client/pkg/v3/transport"
	clientv3 "go.etcd.io/etcd/client/v3"
	"io"
	"log"
	"os"
	"time"
)

type etcd struct{}

func (o etcd) New() etcd {
	return etcd{}

}

func (o etcd) Snapshot() (err error) {
	tlsInfo := transport.TLSInfo{
		TrustedCAFile: "etcdCa",
		CertFile:      "etcdCert",
		KeyFile:       "etcdCertKey",
	}
	tlsConfig, err := tlsInfo.ClientConfig()
	if err != nil {
		fmt.Printf("tlsconfig failed, err:%v\n", err)
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints: []string{
			"https://127.0.0.1:2379",
		},
		DialTimeout: 5 * time.Second,
		TLS:         tlsConfig,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer cli.Close()

	partpath := "aaa.part"

	var f *os.File
	f, err = os.OpenFile(partpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileutil.PrivateFileMode)

	var rd io.ReadCloser
	rd, err = cli.Snapshot(context.TODO())
	if err != nil {
		return err
	}

	size, err := io.Copy(f, rd)
	if err != nil {
		return err
	}
	if !hasChecksum(size) {
		return fmt.Errorf("sha256 checksum not found [bytes: %d]", size)
	}
	if err = fileutil.Fsync(f); err != nil {
		return err
	}

	return
}

func hasChecksum(n int64) bool {
	// 512 is chosen because it's a minimum disk sector size
	// smaller than (and multiplies to) OS page size in most systems
	return (n % 512) == sha256.Size
}
