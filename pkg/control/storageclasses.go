package control

import (
	"context"
	v1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type storageclasse struct {
	clientset kubernetes.Interface
	ctx       context.Context
}

func (ck Kc) StorageClasses() storageclasse {
	return storageclasse{
		ck.clientset,
		context.TODO(),
	}
}

func (t storageclasse) List() (*v1.StorageClassList, error) {
	return t.clientset.StorageV1().StorageClasses().List(context.TODO(), metav1.ListOptions{
		TypeMeta: metav1.TypeMeta{},
	})
}
