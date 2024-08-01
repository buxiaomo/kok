package control

import (
	"context"
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
