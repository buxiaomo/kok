package control

import (
	"context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type secrets struct {
	clientset kubernetes.Interface
	ctx       context.Context
}

func (t secrets) Get(namespace, name string) (result *v1.Secret, err error) {
	return t.clientset.CoreV1().Secrets(namespace).Get(t.ctx, name, metav1.GetOptions{})
}

func (ck Kc) Secrets() secrets {
	return secrets{
		ck.clientset,
		context.TODO(),
	}
}
