package control

import (
	"context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

func (ck Kc) Nodes() nodes {
	return nodes{
		ck.clientset,
		context.TODO(),
	}
}

type nodes struct {
	clientset kubernetes.Interface
	ctx       context.Context
}

func (t nodes) Watch() (watch.Interface, error) {
	return t.clientset.CoreV1().Nodes().Watch(t.ctx, metav1.ListOptions{})
}

func (t nodes) List() (*v1.NodeList, error) {
	return t.clientset.CoreV1().Nodes().List(t.ctx, metav1.ListOptions{})
}
